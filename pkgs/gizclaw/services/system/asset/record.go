package asset

import "time"

// Metadata describes an immutable asset without exposing its object key or backend.
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
