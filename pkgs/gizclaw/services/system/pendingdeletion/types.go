// Package pendingdeletion defines the durable handoff written when an active
// resource is removed. Processing and cleanup are intentionally owned by the
// follow-up cleanup service.
package pendingdeletion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// DescriptorVersion is the schema version of descriptors produced by this package.
const DescriptorVersion = 1

// Kind identifies the domain that owns cleanup for a deleted resource.
type Kind string

const (
	// KindPeer identifies a deleted Peer registration.
	KindPeer Kind = "peer"
	// KindWorkspace identifies a deleted user Workspace.
	KindWorkspace Kind = "workspace"
	// KindPet identifies a deleted Pet row.
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
)

// Record contains only immutable identifiers required by a later domain
// cleaner. Descriptor must not contain secrets or deleted resource content.
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

// Source is the backend-neutral lookup surface shared by KV and SQL pending
// stores. Claiming, retries, and removal are added by the cleanup processor.
type Source interface {
	Get(context.Context, string) (Record, error)
	HasLocator(context.Context, Locator) (bool, error)
}

// New constructs a validated deletion event with a unique deletion ID.
func New(kind Kind, resourceID string, ownerPublicKey *string, reason Reason, descriptor any, now time.Time) (Record, error) {
	resourceID = strings.TrimSpace(resourceID)
	if !kind.valid() {
		return Record{}, fmt.Errorf("pending deletion: invalid kind %q", kind)
	}
	if resourceID == "" {
		return Record{}, errors.New("pending deletion: empty resource id")
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
	record := Record{
		DeletionID:        uuid.NewString(),
		Kind:              kind,
		ResourceID:        resourceID,
		Reason:            reason,
		DeletedAt:         now.UTC(),
		OwnerPublicKey:    cloneString(ownerPublicKey),
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
	if _, err := uuid.Parse(r.DeletionID); err != nil {
		return fmt.Errorf("pending deletion: invalid deletion id %q: %w", r.DeletionID, err)
	}
	if !r.Kind.valid() {
		return fmt.Errorf("pending deletion: invalid kind %q", r.Kind)
	}
	if strings.TrimSpace(r.ResourceID) == "" {
		return errors.New("pending deletion: empty resource id")
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

func (k Kind) valid() bool {
	switch k {
	case KindPeer, KindWorkspace, KindPet:
		return true
	default:
		return false
	}
}

func (r Reason) valid() bool {
	switch r {
	case ReasonAdminDelete, ReasonPeerDelete, ReasonResourceDelete:
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
