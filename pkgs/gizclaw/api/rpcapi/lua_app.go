package rpcapi

import rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"

// AsClientLuaAppListRequest decodes the protobuf payload.
func (t RPCPayload) AsClientLuaAppListRequest() (*rpcpb.ClientLuaAppListRequest, error) {
	body := new(rpcpb.ClientLuaAppListRequest)
	err := t.decode("ClientLuaAppListRequest", body)
	return body, err
}

// FromClientLuaAppListRequest encodes the protobuf payload.
func (t *RPCPayload) FromClientLuaAppListRequest(v *rpcpb.ClientLuaAppListRequest) error {
	return t.encode("ClientLuaAppListRequest", v)
}

// AsClientLuaAppListResponse decodes the protobuf payload.
func (t RPCPayload) AsClientLuaAppListResponse() (*rpcpb.ClientLuaAppListResponse, error) {
	body := new(rpcpb.ClientLuaAppListResponse)
	err := t.decode("ClientLuaAppListResponse", body)
	return body, err
}

// FromClientLuaAppListResponse encodes the protobuf payload.
func (t *RPCPayload) FromClientLuaAppListResponse(v *rpcpb.ClientLuaAppListResponse) error {
	return t.encode("ClientLuaAppListResponse", v)
}

// AsClientLuaAppInstallRequest decodes the protobuf payload.
func (t RPCPayload) AsClientLuaAppInstallRequest() (*rpcpb.ClientLuaAppInstallRequest, error) {
	body := new(rpcpb.ClientLuaAppInstallRequest)
	err := t.decode("ClientLuaAppInstallRequest", body)
	return body, err
}

// FromClientLuaAppInstallRequest encodes the protobuf payload.
func (t *RPCPayload) FromClientLuaAppInstallRequest(v *rpcpb.ClientLuaAppInstallRequest) error {
	return t.encode("ClientLuaAppInstallRequest", v)
}

// AsClientLuaAppInstallResponse decodes the protobuf payload.
func (t RPCPayload) AsClientLuaAppInstallResponse() (*rpcpb.ClientLuaAppInstallResponse, error) {
	body := new(rpcpb.ClientLuaAppInstallResponse)
	err := t.decode("ClientLuaAppInstallResponse", body)
	return body, err
}

// FromClientLuaAppInstallResponse encodes the protobuf payload.
func (t *RPCPayload) FromClientLuaAppInstallResponse(v *rpcpb.ClientLuaAppInstallResponse) error {
	return t.encode("ClientLuaAppInstallResponse", v)
}

// AsClientLuaAppRunRequest decodes the protobuf payload.
func (t RPCPayload) AsClientLuaAppRunRequest() (*rpcpb.ClientLuaAppRunRequest, error) {
	body := new(rpcpb.ClientLuaAppRunRequest)
	err := t.decode("ClientLuaAppRunRequest", body)
	return body, err
}

// FromClientLuaAppRunRequest encodes the protobuf payload.
func (t *RPCPayload) FromClientLuaAppRunRequest(v *rpcpb.ClientLuaAppRunRequest) error {
	return t.encode("ClientLuaAppRunRequest", v)
}

// AsClientLuaAppRunResponse decodes the protobuf payload.
func (t RPCPayload) AsClientLuaAppRunResponse() (*rpcpb.ClientLuaAppRunResponse, error) {
	body := new(rpcpb.ClientLuaAppRunResponse)
	err := t.decode("ClientLuaAppRunResponse", body)
	return body, err
}

// FromClientLuaAppRunResponse encodes the protobuf payload.
func (t *RPCPayload) FromClientLuaAppRunResponse(v *rpcpb.ClientLuaAppRunResponse) error {
	return t.encode("ClientLuaAppRunResponse", v)
}
