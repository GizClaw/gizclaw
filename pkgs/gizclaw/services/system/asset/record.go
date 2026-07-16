package asset

import (
	"context"
	"time"
)

// OwnerKind identifies a closed class of structures that may reference assets.
type OwnerKind string

const (
	// OwnerKindResource identifies a declarative Resource as Kind/metadata.name.
	OwnerKindResource OwnerKind = "resource"
	// OwnerKindFriendGroupMessage identifies a FriendGroup message as group-id/message-id.
	OwnerKindFriendGroupMessage OwnerKind = "friend-group-message"
)

// Valid reports whether the owner kind is supported by the contract.
func (k OwnerKind) Valid() bool {
	switch k {
	case OwnerKindResource, OwnerKindFriendGroupMessage:
		return true
	default:
		return false
	}
}

// Owner identifies one structure that may contain asset references.
type Owner struct {
	Kind OwnerKind
	ID   string
}

// Binding is a reverse reference from an asset to an owner.
//
// The associated asset is supplied by the containing response, repository key,
// or service method argument and is deliberately not duplicated here.
type Binding struct {
	Owner Owner
}

// Metadata is the safe public projection for an immutable asset.
type Metadata struct {
	Ref       Ref
	MediaType string
	SizeBytes int64
	SHA256    [32]byte
	CreatedAt time.Time
	ExpiresAt *time.Time
}

// Asset is an immutable AssetService record.
type Asset struct {
	Metadata Metadata
}

// PutRequest controls a streaming asset upload.
type PutRequest struct {
	MediaType string
	MaxBytes  int64
	ExpiresAt *time.Time
}

// OwnerSnapshot is the current state returned by an OwnerResolver.
type OwnerSnapshot struct {
	Exists bool
	Refs   []Ref
}

// OwnerResolver loads an owner and enumerates the asset references in its full structure.
type OwnerResolver interface {
	ResolveAssetOwner(context.Context, Owner) (OwnerSnapshot, error)
}

type recordState string

const (
	stateStaging  recordState = "staging"
	stateReady    recordState = "ready"
	stateDeleting recordState = "deleting"
)

type assetRecord struct {
	SchemaVersion int         `json:"schema_version"`
	ID            string      `json:"id"`
	MediaType     string      `json:"media_type"`
	SizeBytes     int64       `json:"size_bytes"`
	SHA256        string      `json:"sha256"`
	CreatedAt     time.Time   `json:"created_at"`
	ExpiresAt     *time.Time  `json:"expires_at,omitempty"`
	State         recordState `json:"state"`
}

type bindingRecord struct {
	Owner     Owner        `json:"owner"`
	State     bindingState `json:"state"`
	CreatedAt time.Time    `json:"created_at"`
}

type bindingState string

const (
	bindingStatePending bindingState = "pending"
	bindingStateActive  bindingState = "active"
)
