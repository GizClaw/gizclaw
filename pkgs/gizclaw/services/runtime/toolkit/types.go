package toolkit

import (
	"encoding/json"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
)

type ToolType string

const (
	ToolTypeHTTPRequest ToolType = "http_request"
	ToolTypeClientRPC   ToolType = "client_rpc"
)

// Tool is the persisted configuration for one caller-identified capability.
type Tool struct {
	ID          string            `json:"id"`
	InvokeName  string            `json:"invoke_name"`
	Type        ToolType          `json:"type"`
	Description *string           `json:"description,omitempty"`
	Enabled     bool              `json:"enabled"`
	Version     *string           `json:"version,omitempty"`
	InputSchema jsonschema.Schema `json:"input_schema"`
	Triggers    []ToolTrigger     `json:"triggers,omitempty"`
	Metadata    json.RawMessage   `json:"metadata,omitempty"`
	HTTP        *HTTPRequest      `json:"http,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
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

type HTTPRequest struct {
	URL                string                `json:"url"`
	Method             string                `json:"method"`
	Auth               HTTPAuth              `json:"auth"`
	Headers            map[string]string     `json:"headers,omitempty"`
	Query              []HTTPArgumentBinding `json:"query,omitempty"`
	Body               []HTTPArgumentBinding `json:"body,omitempty"`
	ResponsePointer    *string               `json:"response_pointer,omitempty"`
	SuccessStatusCodes []int                 `json:"success_status_codes,omitempty"`
	Timeout            time.Duration         `json:"timeout"`
	MaxResponseBytes   int64                 `json:"max_response_bytes"`
}

type HTTPArgumentBinding struct {
	ArgumentPointer string `json:"argument_pointer"`
	Target          string `json:"target"`
	Required        bool   `json:"required"`
}

type HTTPAuth struct {
	Method      string  `json:"method"`
	BearerToken *string `json:"bearer_token,omitempty"`
	Header      *string `json:"header,omitempty"`
	APIKey      *string `json:"api_key,omitempty"`
	Credential  *string `json:"credential,omitempty"`
	Region      *string `json:"region,omitempty"`
	Service     *string `json:"service,omitempty"`
	Action      *string `json:"action,omitempty"`
	Version     *string `json:"version,omitempty"`
}

type ToolKit struct {
	Tools []Tool
}
