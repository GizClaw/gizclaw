// Package pendingdeletion defines durable deletion handoffs and coordinates
// bounded processing. Resource validation and atomic physical removal remain
// owned by registered domain handlers.
package pendingdeletion

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/google/uuid"
)

// DescriptorVersion is the schema version of descriptors produced by this package.
const DescriptorVersion = 1

var deletionIDNamespace = uuid.NewSHA1(
	uuid.NameSpaceURL,
	[]byte("https://github.com/GizClaw/gizclaw/pending-deletion"),
)

// Kind identifies the domain that owns cleanup for a resource pending deletion.
type Kind string

const (
	// KindPeer identifies a Peer registration pending deletion.
	KindPeer Kind = "peer"
	// KindWorkspace identifies a user Workspace pending deletion.
	KindWorkspace Kind = "workspace"
	// KindFriendGroup identifies retired Friend Group data pending deletion.
	KindFriendGroup Kind = "friend_group"
	// KindPet identifies a Pet row pending deletion.
	KindPet Kind = "pet"
)

// Reason identifies the lifecycle operation that produced a deletion event.
type Reason string

const (
	// ReasonAdminDelete identifies an Admin-initiated deletion.
	ReasonAdminDelete Reason = "admin_delete"
	// ReasonPeerDelete identifies authenticated Peer self-deletion.
	ReasonPeerDelete Reason = "peer_delete"
	// ReasonResourceDelete identifies a domain resource deletion.
	ReasonResourceDelete Reason = "resource_delete"
	// ReasonFriendRelationshipDelete identifies Direct Chatroom retirement.
	ReasonFriendRelationshipDelete Reason = "friend_relationship_delete"
	// ReasonFriendGroupDelete identifies Group Chatroom retirement.
	ReasonFriendGroupDelete Reason = "friend_group_delete"
)

// Record contains only immutable identifiers required by a later domain
// cleaner. Descriptor must not contain secrets or resource content.
type Record struct {
	DeletionID        string          `json:"deletion_id"`
	Kind              Kind            `json:"kind"`
	ResourceID        string          `json:"resource_id"`
	Reason            Reason          `json:"reason"`
	DeletedAt         time.Time       `json:"deleted_at"`
	OwnerPublicKey    *string         `json:"owner_public_key,omitempty"`
	DescriptorVersion int             `json:"descriptor_version"`
	Descriptor        json.RawMessage `json:"descriptor"`
}

// Locator identifies deletion events for one logical resource.
type Locator struct {
	Kind           Kind
	ResourceID     string
	OwnerPublicKey *string
}

// LookupSource is the backend-neutral marker lookup surface shared by KV and SQL stores.
type LookupSource interface {
	Get(context.Context, string) (Record, error)
	HasLocator(context.Context, Locator) (bool, error)
}

// New constructs a validated deletion event whose ID is stable for the
// resource locator. Repeated requests for the same resource therefore address
// the same deletion event.
func New(kind Kind, resourceID string, ownerPublicKey *string, reason Reason, descriptor any, now time.Time) (Record, error) {
	ownerPublicKey = cloneString(ownerPublicKey)
	if !kind.valid() {
		return Record{}, fmt.Errorf("pending deletion: invalid kind %q", kind)
	}
	if err := customid.ValidateResourceID(resourceID); err != nil {
		return Record{}, fmt.Errorf("pending deletion: invalid resource id: %w", err)
	}
	if reason == "" {
		return Record{}, errors.New("pending deletion: empty reason")
	}
	data, err := json.Marshal(descriptor)
	if err != nil {
		return Record{}, fmt.Errorf("pending deletion: encode descriptor: %w", err)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	deletionID, err := deletionIDForLocator(kind, resourceID, ownerPublicKey)
	if err != nil {
		return Record{}, err
	}
	record := Record{
		DeletionID:        deletionID,
		Kind:              kind,
		ResourceID:        resourceID,
		Reason:            reason,
		DeletedAt:         now.UTC(),
		OwnerPublicKey:    ownerPublicKey,
		DescriptorVersion: DescriptorVersion,
		Descriptor:        data,
	}
	if err := record.Validate(); err != nil {
		return Record{}, err
	}
	return record, nil
}

// Validate checks the immutable envelope before it crosses a storage boundary.
func (r Record) Validate() error {
	if !r.Kind.valid() {
		return fmt.Errorf("pending deletion: invalid kind %q", r.Kind)
	}
	resourceID := r.ResourceID
	if err := customid.ValidateResourceID(resourceID); err != nil {
		return fmt.Errorf("pending deletion: invalid resource id: %w", err)
	}
	if r.OwnerPublicKey != nil {
		ownerPublicKey := strings.TrimSpace(*r.OwnerPublicKey)
		if ownerPublicKey == "" {
			return errors.New("pending deletion: empty owner public key")
		}
		if ownerPublicKey != *r.OwnerPublicKey {
			return errors.New("pending deletion: non-canonical owner public key")
		}
	}
	expectedDeletionID, err := deletionIDForLocator(r.Kind, resourceID, r.OwnerPublicKey)
	if err != nil {
		return err
	}
	if r.DeletionID != expectedDeletionID {
		if _, err := uuid.Parse(r.DeletionID); err != nil {
			return fmt.Errorf("pending deletion: invalid deletion id %q", r.DeletionID)
		}
	}
	if !r.Reason.valid() {
		return fmt.Errorf("pending deletion: invalid reason %q", r.Reason)
	}
	if r.DeletedAt.IsZero() {
		return errors.New("pending deletion: empty deleted_at")
	}
	if r.DescriptorVersion != DescriptorVersion {
		return fmt.Errorf("pending deletion: unsupported descriptor version %d", r.DescriptorVersion)
	}
	if len(r.Descriptor) == 0 || !json.Valid(r.Descriptor) {
		return errors.New("pending deletion: invalid descriptor JSON")
	}
	return nil
}

func deletionIDForLocator(kind Kind, resourceID string, ownerPublicKey *string) (string, error) {
	if !kind.valid() {
		return "", fmt.Errorf("pending deletion: invalid kind %q", kind)
	}
	if err := customid.ValidateResourceID(resourceID); err != nil {
		return "", fmt.Errorf("pending deletion: invalid resource id: %w", err)
	}
	encode := base64.RawURLEncoding.EncodeToString
	if kind != KindPet {
		locator := string(kind) + "\x00" + encode([]byte(resourceID))
		return uuid.NewSHA1(deletionIDNamespace, []byte(locator)).String(), nil
	}
	if ownerPublicKey == nil || strings.TrimSpace(*ownerPublicKey) == "" {
		return "", errors.New("pending deletion: Pet requires owner public key")
	}
	if *ownerPublicKey != strings.TrimSpace(*ownerPublicKey) {
		return "", errors.New("pending deletion: non-canonical owner public key")
	}
	locator := string(kind) + "\x00" +
		encode([]byte(*ownerPublicKey)) + "\x00" +
		encode([]byte(resourceID))
	return uuid.NewSHA1(deletionIDNamespace, []byte(locator)).String(), nil
}

// DeterministicDeletionID returns the current stable deletion ID for a locator.
func DeterministicDeletionID(locator Locator) (string, error) {
	return deletionIDForLocator(locator.Kind, locator.ResourceID, locator.OwnerPublicKey)
}

// IsDeterministic reports whether record uses the current stable locator ID.
func IsDeterministic(record Record) bool {
	expected, err := DeterministicDeletionID(Locator{
		Kind:           record.Kind,
		ResourceID:     record.ResourceID,
		OwnerPublicKey: record.OwnerPublicKey,
	})
	return err == nil && record.DeletionID == expected
}

// Fingerprint returns a stable digest of every immutable marker field.
func Fingerprint(record Record) (string, error) {
	if err := record.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(struct {
		DeletionID        string          `json:"deletion_id"`
		Kind              Kind            `json:"kind"`
		ResourceID        string          `json:"resource_id"`
		Reason            Reason          `json:"reason"`
		DeletedAt         time.Time       `json:"deleted_at"`
		OwnerPublicKey    *string         `json:"owner_public_key,omitempty"`
		DescriptorVersion int             `json:"descriptor_version"`
		Descriptor        json.RawMessage `json:"descriptor"`
	}{
		DeletionID:        record.DeletionID,
		Kind:              record.Kind,
		ResourceID:        record.ResourceID,
		Reason:            record.Reason,
		DeletedAt:         record.DeletedAt.UTC(),
		OwnerPublicKey:    cloneString(record.OwnerPublicKey),
		DescriptorVersion: record.DescriptorVersion,
		Descriptor:        append(json.RawMessage(nil), record.Descriptor...),
	})
	if err != nil {
		return "", fmt.Errorf("pending deletion: encode marker fingerprint: %w", err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

// StoredFingerprint returns Fingerprint for valid records and a stable
// quarantine fingerprint for malformed persisted records. Sources use the
// latter only so a corrupt legacy marker can be claimed and failed instead of
// blocking migration or every later task in the same source.
func StoredFingerprint(record Record) (string, error) {
	if fingerprint, err := Fingerprint(record); err == nil {
		return fingerprint, nil
	}
	data, err := json.Marshal(struct {
		DeletionID        string  `json:"deletion_id"`
		Kind              Kind    `json:"kind"`
		ResourceID        string  `json:"resource_id"`
		Reason            Reason  `json:"reason"`
		DeletedAt         string  `json:"deleted_at"`
		OwnerPublicKey    *string `json:"owner_public_key,omitempty"`
		DescriptorVersion int     `json:"descriptor_version"`
		Descriptor        string  `json:"descriptor"`
	}{
		DeletionID: record.DeletionID, Kind: record.Kind, ResourceID: record.ResourceID,
		Reason: record.Reason, DeletedAt: record.DeletedAt.UTC().Format(time.RFC3339Nano),
		OwnerPublicKey: cloneString(record.OwnerPublicKey), DescriptorVersion: record.DescriptorVersion,
		Descriptor: string(record.Descriptor),
	})
	if err != nil {
		return "", fmt.Errorf("pending deletion: encode stored marker fingerprint: %w", err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func (k Kind) valid() bool {
	switch k {
	case KindPeer, KindWorkspace, KindFriendGroup, KindPet:
		return true
	default:
		return false
	}
}

func (r Reason) valid() bool {
	switch r {
	case ReasonAdminDelete,
		ReasonPeerDelete,
		ReasonResourceDelete,
		ReasonFriendRelationshipDelete,
		ReasonFriendGroupDelete:
		return true
	default:
		return false
	}
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
