package rpcapi

import rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"

type ServerRegisterRequest struct {
	Token string
}

type ServerRegisterResponse struct {
	RuntimeProfileName string
}

func (t RPCPayload) AsServerRegisterRequest() (ServerRegisterRequest, error) {
	var value rpcpb.ServerRegisterRequest
	if err := t.decode("ServerRegisterRequest", &value); err != nil {
		return ServerRegisterRequest{}, err
	}
	return ServerRegisterRequest{Token: value.GetToken()}, nil
}

func (t *RPCPayload) FromServerRegisterRequest(value ServerRegisterRequest) error {
	return t.encode("ServerRegisterRequest", &rpcpb.ServerRegisterRequest{Token: value.Token})
}

func (t RPCPayload) AsServerRegisterResponse() (ServerRegisterResponse, error) {
	var value rpcpb.ServerRegisterResponse
	if err := t.decode("ServerRegisterResponse", &value); err != nil {
		return ServerRegisterResponse{}, err
	}
	return ServerRegisterResponse{
		RuntimeProfileName: value.GetRuntimeProfileName(),
	}, nil
}

func (t *RPCPayload) FromServerRegisterResponse(value ServerRegisterResponse) error {
	return t.encode("ServerRegisterResponse", &rpcpb.ServerRegisterResponse{
		RuntimeProfileName: value.RuntimeProfileName,
	})
}
