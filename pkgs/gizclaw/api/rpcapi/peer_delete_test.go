package rpcapi

import "testing"

func TestServerPeerDeletePayloadCodec(t *testing.T) {
	request := &RPCPayload{}
	if err := request.FromServerPeerDeleteRequest(ServerPeerDeleteRequest{}); err != nil {
		t.Fatalf("FromServerPeerDeleteRequest: %v", err)
	}
	if _, err := request.AsServerPeerDeleteRequest(); err != nil {
		t.Fatalf("AsServerPeerDeleteRequest: %v", err)
	}
	response := &RPCPayload{}
	if err := response.FromServerPeerDeleteResponse(ServerPeerDeleteResponse{}); err != nil {
		t.Fatalf("FromServerPeerDeleteResponse: %v", err)
	}
	if _, err := response.AsServerPeerDeleteResponse(); err != nil {
		t.Fatalf("AsServerPeerDeleteResponse: %v", err)
	}
	protoMethod, err := ProtoMethod(RPCMethodServerPeerDelete)
	if err != nil || int32(protoMethod) != 94 {
		t.Fatalf("ProtoMethod(RPCMethodServerPeerDelete) = %d, %v", protoMethod, err)
	}
}
