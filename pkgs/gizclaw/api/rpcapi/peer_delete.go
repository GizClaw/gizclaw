package rpcapi

import rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"

type ServerPeerDeleteRequest struct{}

type ServerPeerDeleteResponse struct{}

func (t RPCPayload) AsServerPeerDeleteRequest() (ServerPeerDeleteRequest, error) {
	var value rpcpb.ServerPeerDeleteRequest
	if err := t.decode("ServerPeerDeleteRequest", &value); err != nil {
		return ServerPeerDeleteRequest{}, err
	}
	return ServerPeerDeleteRequest{}, nil
}

func (t *RPCPayload) FromServerPeerDeleteRequest(ServerPeerDeleteRequest) error {
	return t.encode("ServerPeerDeleteRequest", &rpcpb.ServerPeerDeleteRequest{})
}

func (t RPCPayload) AsServerPeerDeleteResponse() (ServerPeerDeleteResponse, error) {
	var value rpcpb.ServerPeerDeleteResponse
	if err := t.decode("ServerPeerDeleteResponse", &value); err != nil {
		return ServerPeerDeleteResponse{}, err
	}
	return ServerPeerDeleteResponse{}, nil
}

func (t *RPCPayload) FromServerPeerDeleteResponse(ServerPeerDeleteResponse) error {
	return t.encode("ServerPeerDeleteResponse", &rpcpb.ServerPeerDeleteResponse{})
}
