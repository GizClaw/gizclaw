package toolkit

import (
	"encoding/json"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

const (
	ResourceKindTool = apitypes.ACLResourceKind("tool")
)

type ToolSource string

const (
	ToolSourceBuiltin ToolSource = "builtin"
	ToolSourceDevice  ToolSource = "device"
	ToolSourceAdmin   ToolSource = "admin"
)

type ToolExecutorKind string

const (
	ToolExecutorKindBuiltin   ToolExecutorKind = "builtin"
	ToolExecutorKindDeviceRPC ToolExecutorKind = "device_rpc"
)

// Tool is the persisted configuration model for one executable capability.
type Tool struct {
	ID           string          `json:"id"`
	Name         *string         `json:"name,omitempty"`
	Description  *string         `json:"description,omitempty"`
	Source       ToolSource      `json:"source"`
	Enabled      bool            `json:"enabled"`
	OwnerPeer    *string         `json:"owner_peer,omitempty"`
	Version      *string         `json:"version,omitempty"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
	Triggers     []ToolTrigger   `json:"triggers,omitempty"`
	Executor     ToolExecutor    `json:"executor"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
	SyncedAt     *time.Time      `json:"synced_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type ToolTrigger struct {
	Name        string               `json:"name"`
	Description *string              `json:"description,omitempty"`
	Patterns    []string             `json:"patterns,omitempty"`
	Examples    []ToolTriggerExample `json:"examples,omitempty"`
	Metadata    json.RawMessage      `json:"metadata,omitempty"`
}

type ToolTriggerExample struct {
	Input  string          `json:"input"`
	Args   json.RawMessage `json:"args,omitempty"`
	Output *string         `json:"output,omitempty"`
}

type ToolExecutor struct {
	Kind   ToolExecutorKind `json:"kind"`
	Name   *string          `json:"name,omitempty"`
	Method *string          `json:"method,omitempty"`
	PeerID *string          `json:"peer_id,omitempty"`
	Config json.RawMessage  `json:"config,omitempty"`
}

type ToolKit struct {
	Tools []Tool
}

func ToolResource(id string) apitypes.ACLResource {
	return apitypes.ACLResource{
		Kind: ResourceKindTool,
		Id:   id,
	}
}
