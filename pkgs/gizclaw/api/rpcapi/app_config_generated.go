// Code generated from api/proto/rpc/rpc.proto and api/proto/rpc/payload/*.proto; DO NOT EDIT.

package rpcapi

const (
	RPCMethodServerAppConfigGet  RPCMethod = "server.app_config.get"
	RPCMethodServerAppConfigList RPCMethod = "server.app_config.list"
)

type AppConfigListRequest struct {
	Cursor *string `json:"cursor,omitempty"`
	Limit  *int    `json:"limit,omitempty"`
}

type AppConfigListResponse struct {
	Keys                   []string `json:"keys"`
	HasNext                bool     `json:"has_next"`
	NextCursor             *string  `json:"next_cursor,omitempty"`
	RuntimeProfileName     string   `json:"runtime_profile_name"`
	RuntimeProfileRevision string   `json:"runtime_profile_revision"`
}

type AppConfigGetRequest struct {
	Key string `json:"key"`
}

type AppConfigGetResponse struct {
	Value                  string `json:"value"`
	RuntimeProfileName     string `json:"runtime_profile_name"`
	RuntimeProfileRevision string `json:"runtime_profile_revision"`
}

func decodeAppConfigPayload[T any](p RPCPayload, name string) (T, error) {
	var out T
	err := p.decode(name, &out)
	return out, err
}

func (p RPCPayload) AsAppConfigListRequest() (AppConfigListRequest, error) {
	return decodeAppConfigPayload[AppConfigListRequest](p, "AppConfigListRequest")
}
func (p *RPCPayload) FromAppConfigListRequest(v AppConfigListRequest) error {
	return p.encode("AppConfigListRequest", v)
}
func (p *RPCPayload) MergeAppConfigListRequest(v AppConfigListRequest) error {
	return p.merge("AppConfigListRequest", v)
}
func (p RPCPayload) AsAppConfigListResponse() (AppConfigListResponse, error) {
	return decodeAppConfigPayload[AppConfigListResponse](p, "AppConfigListResponse")
}
func (p *RPCPayload) FromAppConfigListResponse(v AppConfigListResponse) error {
	return p.encode("AppConfigListResponse", v)
}
func (p *RPCPayload) MergeAppConfigListResponse(v AppConfigListResponse) error {
	return p.merge("AppConfigListResponse", v)
}
func (p RPCPayload) AsAppConfigGetRequest() (AppConfigGetRequest, error) {
	return decodeAppConfigPayload[AppConfigGetRequest](p, "AppConfigGetRequest")
}
func (p *RPCPayload) FromAppConfigGetRequest(v AppConfigGetRequest) error {
	return p.encode("AppConfigGetRequest", v)
}
func (p *RPCPayload) MergeAppConfigGetRequest(v AppConfigGetRequest) error {
	return p.merge("AppConfigGetRequest", v)
}
func (p RPCPayload) AsAppConfigGetResponse() (AppConfigGetResponse, error) {
	return decodeAppConfigPayload[AppConfigGetResponse](p, "AppConfigGetResponse")
}
func (p *RPCPayload) FromAppConfigGetResponse(v AppConfigGetResponse) error {
	return p.encode("AppConfigGetResponse", v)
}
func (p *RPCPayload) MergeAppConfigGetResponse(v AppConfigGetResponse) error {
	return p.merge("AppConfigGetResponse", v)
}
