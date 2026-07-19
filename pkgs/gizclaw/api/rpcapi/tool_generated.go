// Code generated from api/proto/rpc/rpc.proto and api/proto/rpc/payload/*.proto; DO NOT EDIT.

package rpcapi

import (
	"time"

	"github.com/google/jsonschema-go/jsonschema"
)

const (
	RPCMethodClientToolInvoke RPCMethod = "client.tool.invoke"
	RPCMethodServerToolCreate RPCMethod = "server.tool.create"
	RPCMethodServerToolDelete RPCMethod = "server.tool.delete"
	RPCMethodServerToolGet    RPCMethod = "server.tool.get"
	RPCMethodServerToolList   RPCMethod = "server.tool.list"
	RPCMethodServerToolPut    RPCMethod = "server.tool.put"
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
	Id             string                  `json:"id"`
	Name           *string                 `json:"name,omitempty"`
	Description    *string                 `json:"description,omitempty"`
	Source         ToolSource              `json:"source"`
	Enabled        *bool                   `json:"enabled,omitempty"`
	OwnerPeer      *string                 `json:"owner_peer,omitempty"`
	OwnerPublicKey *string                 `json:"owner_public_key,omitempty"`
	Version        *string                 `json:"version,omitempty"`
	InputSchema    jsonschema.Schema       `json:"input_schema"`
	OutputSchema   *jsonschema.Schema      `json:"output_schema,omitempty"`
	Triggers       *[]ToolTrigger          `json:"triggers,omitempty"`
	Executor       ToolExecutor            `json:"executor"`
	Metadata       *map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt      time.Time               `json:"created_at"`
	UpdatedAt      time.Time               `json:"updated_at"`
}

type ToolListRequest struct {
	Cursor *string `json:"cursor,omitempty"`
	Limit  *int    `json:"limit,omitempty"`
}

type ToolListResponse struct {
	Items      []Tool  `json:"items"`
	HasNext    bool    `json:"has_next"`
	NextCursor *string `json:"next_cursor,omitempty"`
}

type ToolGetRequest struct {
	Id string `json:"id"`
}

type ToolGetResponse = Tool
type ToolCreateRequest = Tool
type ToolCreateResponse = Tool

type ToolPutRequest struct {
	Id   string `json:"id"`
	Body Tool   `json:"body"`
}

type ToolPutResponse = Tool

type ToolDeleteRequest struct {
	Id string `json:"id"`
}

type ToolDeleteResponse = Tool

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
func (p RPCPayload) AsToolCreateRequest() (ToolCreateRequest, error) {
	return decodeToolPayload[ToolCreateRequest](p, "ToolCreateRequest")
}
func (p *RPCPayload) FromToolCreateRequest(v ToolCreateRequest) error {
	return p.encode("ToolCreateRequest", v)
}
func (p *RPCPayload) MergeToolCreateRequest(v ToolCreateRequest) error {
	return p.merge("ToolCreateRequest", v)
}
func (p RPCPayload) AsToolPutRequest() (ToolPutRequest, error) {
	return decodeToolPayload[ToolPutRequest](p, "ToolPutRequest")
}
func (p *RPCPayload) FromToolPutRequest(v ToolPutRequest) error  { return p.encode("ToolPutRequest", v) }
func (p *RPCPayload) MergeToolPutRequest(v ToolPutRequest) error { return p.merge("ToolPutRequest", v) }
func (p RPCPayload) AsToolDeleteRequest() (ToolDeleteRequest, error) {
	return decodeToolPayload[ToolDeleteRequest](p, "ToolDeleteRequest")
}
func (p *RPCPayload) FromToolDeleteRequest(v ToolDeleteRequest) error {
	return p.encode("ToolDeleteRequest", v)
}
func (p *RPCPayload) MergeToolDeleteRequest(v ToolDeleteRequest) error {
	return p.merge("ToolDeleteRequest", v)
}
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
func (p RPCPayload) AsToolCreateResponse() (ToolCreateResponse, error) {
	return decodeToolPayload[ToolCreateResponse](p, "ToolCreateResponse")
}
func (p *RPCPayload) FromToolCreateResponse(v ToolCreateResponse) error {
	return p.encode("ToolCreateResponse", v)
}
func (p *RPCPayload) MergeToolCreateResponse(v ToolCreateResponse) error {
	return p.merge("ToolCreateResponse", v)
}
func (p RPCPayload) AsToolPutResponse() (ToolPutResponse, error) {
	return decodeToolPayload[ToolPutResponse](p, "ToolPutResponse")
}
func (p *RPCPayload) FromToolPutResponse(v ToolPutResponse) error {
	return p.encode("ToolPutResponse", v)
}
func (p *RPCPayload) MergeToolPutResponse(v ToolPutResponse) error {
	return p.merge("ToolPutResponse", v)
}
func (p RPCPayload) AsToolDeleteResponse() (ToolDeleteResponse, error) {
	return decodeToolPayload[ToolDeleteResponse](p, "ToolDeleteResponse")
}
func (p *RPCPayload) FromToolDeleteResponse(v ToolDeleteResponse) error {
	return p.encode("ToolDeleteResponse", v)
}
func (p *RPCPayload) MergeToolDeleteResponse(v ToolDeleteResponse) error {
	return p.merge("ToolDeleteResponse", v)
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
