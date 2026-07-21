// Code generated from api/proto/rpc/rpc.proto and api/proto/rpc/payload/*.proto; DO NOT EDIT.

package rpcapi

import "github.com/google/jsonschema-go/jsonschema"

const (
	RPCMethodClientToolInvoke RPCMethod = "client.tool.invoke"
	RPCMethodServerToolGet    RPCMethod = "server.tool.get"
	RPCMethodServerToolList   RPCMethod = "server.tool.list"
)

type ToolSource string

const (
	ToolSourceAdmin   ToolSource = "admin"
	ToolSourceBuiltin ToolSource = "builtin"
	ToolSourceDevice  ToolSource = "device"
)

func (e ToolSource) Valid() bool {
	switch e {
	case ToolSourceAdmin, ToolSourceBuiltin, ToolSourceDevice:
		return true
	default:
		return false
	}
}

type ToolExecutorKind string

const (
	ToolExecutorKindBuiltin   ToolExecutorKind = "builtin"
	ToolExecutorKindDeviceRpc ToolExecutorKind = "device_rpc"
)

func (e ToolExecutorKind) Valid() bool {
	switch e {
	case ToolExecutorKindBuiltin, ToolExecutorKindDeviceRpc:
		return true
	default:
		return false
	}
}

type ToolExecutor struct {
	Kind   ToolExecutorKind        `json:"kind"`
	Name   *string                 `json:"name,omitempty"`
	Method *string                 `json:"method,omitempty"`
	PeerId *string                 `json:"peer_id,omitempty"`
	Config *map[string]interface{} `json:"config,omitempty"`
}

type ToolTriggerExample struct {
	Input  string                  `json:"input"`
	Args   *map[string]interface{} `json:"args,omitempty"`
	Output *string                 `json:"output,omitempty"`
}

type ToolTrigger struct {
	Name        string                  `json:"name"`
	Description *string                 `json:"description,omitempty"`
	Patterns    *[]string               `json:"patterns,omitempty"`
	Examples    *[]ToolTriggerExample   `json:"examples,omitempty"`
	Metadata    *map[string]interface{} `json:"metadata,omitempty"`
}

type Tool struct {
	Alias        string                   `json:"alias"`
	I18n         map[string]AliasI18nText `json:"i18n"`
	InputSchema  jsonschema.Schema        `json:"input_schema"`
	OutputSchema *jsonschema.Schema       `json:"output_schema,omitempty"`
}

type ToolListRequest struct {
	Cursor *string `json:"cursor,omitempty"`
	Limit  *int    `json:"limit,omitempty"`
}

type ToolListResponse struct {
	Items                  []Tool  `json:"items"`
	HasNext                bool    `json:"has_next"`
	NextCursor             *string `json:"next_cursor,omitempty"`
	RuntimeProfileName     string  `json:"runtime_profile_name"`
	RuntimeProfileRevision string  `json:"runtime_profile_revision"`
}

type ToolGetRequest struct {
	Alias string `json:"alias"`
}

type ToolGetResponse struct {
	Value                  Tool   `json:"value"`
	RuntimeProfileName     string `json:"runtime_profile_name"`
	RuntimeProfileRevision string `json:"runtime_profile_revision"`
}

type ToolInvokeRequest struct {
	CallId string                 `json:"call_id"`
	ToolId string                 `json:"tool_id"`
	Method string                 `json:"method"`
	Args   map[string]interface{} `json:"args"`
}

type ToolInvokeResponse struct {
	DataJson string `json:"data_json"`
}

func decodeToolPayload[T any](p RPCPayload, name string) (T, error) {
	var out T
	err := p.decode(name, &out)
	return out, err
}

func (p RPCPayload) AsToolListRequest() (ToolListRequest, error) {
	return decodeToolPayload[ToolListRequest](p, "ToolListRequest")
}
func (p *RPCPayload) FromToolListRequest(v ToolListRequest) error {
	return p.encode("ToolListRequest", v)
}
func (p *RPCPayload) MergeToolListRequest(v ToolListRequest) error {
	return p.merge("ToolListRequest", v)
}
func (p RPCPayload) AsToolGetRequest() (ToolGetRequest, error) {
	return decodeToolPayload[ToolGetRequest](p, "ToolGetRequest")
}
func (p *RPCPayload) FromToolGetRequest(v ToolGetRequest) error  { return p.encode("ToolGetRequest", v) }
func (p *RPCPayload) MergeToolGetRequest(v ToolGetRequest) error { return p.merge("ToolGetRequest", v) }
func (p RPCPayload) AsToolInvokeRequest() (ToolInvokeRequest, error) {
	return decodeToolPayload[ToolInvokeRequest](p, "ToolInvokeRequest")
}
func (p *RPCPayload) FromToolInvokeRequest(v ToolInvokeRequest) error {
	return p.encode("ToolInvokeRequest", v)
}
func (p *RPCPayload) MergeToolInvokeRequest(v ToolInvokeRequest) error {
	return p.merge("ToolInvokeRequest", v)
}

func (p RPCPayload) AsToolListResponse() (ToolListResponse, error) {
	return decodeToolPayload[ToolListResponse](p, "ToolListResponse")
}
func (p *RPCPayload) FromToolListResponse(v ToolListResponse) error {
	return p.encode("ToolListResponse", v)
}
func (p *RPCPayload) MergeToolListResponse(v ToolListResponse) error {
	return p.merge("ToolListResponse", v)
}
func (p RPCPayload) AsToolGetResponse() (ToolGetResponse, error) {
	return decodeToolPayload[ToolGetResponse](p, "ToolGetResponse")
}
func (p *RPCPayload) FromToolGetResponse(v ToolGetResponse) error {
	return p.encode("ToolGetResponse", v)
}
func (p *RPCPayload) MergeToolGetResponse(v ToolGetResponse) error {
	return p.merge("ToolGetResponse", v)
}
func (p RPCPayload) AsToolInvokeResponse() (ToolInvokeResponse, error) {
	return decodeToolPayload[ToolInvokeResponse](p, "ToolInvokeResponse")
}
func (p *RPCPayload) FromToolInvokeResponse(v ToolInvokeResponse) error {
	return p.encode("ToolInvokeResponse", v)
}
func (p *RPCPayload) MergeToolInvokeResponse(v ToolInvokeResponse) error {
	return p.merge("ToolInvokeResponse", v)
}
