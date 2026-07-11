package toolkit

import (
	"encoding/json"
	"net/url"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/google/jsonschema-go/jsonschema"
)

const (
	ResourceKindTool = apitypes.ACLResourceKindTool
	// ResourceOwnerRole is the reserved ACL role used for primary resource owners.
	ResourceOwnerRole = "resource-owner"
	// ToolOwnerRole is kept for Tool callers but now uses the shared managed-resource owner role.
	ToolOwnerRole = ResourceOwnerRole
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
	ID           string             `json:"id"`
	Name         *string            `json:"name,omitempty"`
	Description  *string            `json:"description,omitempty"`
	Source       ToolSource         `json:"source"`
	Enabled      bool               `json:"enabled"`
	OwnerPeer    *string            `json:"owner_peer,omitempty"`
	Version      *string            `json:"version,omitempty"`
	InputSchema  jsonschema.Schema  `json:"input_schema"`
	OutputSchema *jsonschema.Schema `json:"output_schema,omitempty"`
	Triggers     []ToolTrigger      `json:"triggers,omitempty"`
	Executor     ToolExecutor       `json:"executor"`
	Metadata     json.RawMessage    `json:"metadata,omitempty"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
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

// ResourceOwnerPolicyBindingID returns the deterministic ACL binding ID for one owned resource.
func ResourceOwnerPolicyBindingID(kind apitypes.ACLResourceKind, id string) string {
	return "resource-owner:" + url.PathEscape(string(kind)) + ":" + url.PathEscape(id)
}

// ToolOwnerPolicyBindingID returns the deterministic ACL binding ID for a Tool owner.
func ToolOwnerPolicyBindingID(toolID, owner string) string {
	return ResourceOwnerPolicyBindingID(ResourceKindTool, toolID)
}

// LegacyToolOwnerPolicyBindingID returns the pre-resource-owner binding ID used by older device Tools.
func LegacyToolOwnerPolicyBindingID(toolID, owner string) string {
	return "tool-owner:" + url.PathEscape(toolID) + ":" + url.PathEscape(owner)
}
