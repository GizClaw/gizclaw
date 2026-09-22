package rpcapi

import (
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
	"strings"
	"testing"
)

func TestMhsCodecPreservesOneofTypesAndDefaults(t *testing.T) {
	for _, value := range []*rpcpb.MhsValue{
		{Value: &rpcpb.MhsValue_BoolValue{BoolValue: false}},
		{Value: &rpcpb.MhsValue_IntValue{IntValue: 0}},
		{Value: &rpcpb.MhsValue_IntValue{IntValue: MaxMhsInt}},
		{Value: &rpcpb.MhsValue_DoubleValue{DoubleValue: 0}},
		{Value: &rpcpb.MhsValue_StringValue{StringValue: ""}},
		{Value: &rpcpb.MhsValue_StringValue{StringValue: strings.Repeat("x", 256)}},
	} {
		request := &rpcpb.ClientMhsV0WriteRequest{States: []*rpcpb.MhsStateValue{{DeviceId: strings.Repeat("a", 64), State: "level", Value: value}}}
		var payload RPCPayload
		if err := payload.FromClientMhsV0WriteRequest(request); err != nil {
			t.Fatal(err)
		}
		encoded, err := encodeRPCRequestPayload(RPCMethodClientMhsV0Write, &payload)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeRPCRequestPayload(RPCMethodClientMhsV0Write, encoded)
		if err != nil {
			t.Fatal(err)
		}
		got, err := decoded.AsClientMhsV0WriteRequest()
		if err != nil || !proto.Equal(got, request) {
			t.Fatalf("got %v err %v", got, err)
		}
		for _, method := range []RPCMethod{RPCMethodClientMhsV0Read, RPCMethodClientMhsV0Write} {
			var result RPCPayload
			if method == RPCMethodClientMhsV0Read {
				err = result.FromClientMhsV0ReadResponse(&rpcpb.ClientMhsV0ReadResponse{States: request.States})
			} else {
				err = result.FromClientMhsV0WriteResponse(&rpcpb.ClientMhsV0WriteResponse{States: request.States})
			}
			if err != nil {
				t.Fatal(err)
			}
			wire, err := encodeRPCResponsePayload(method, &result)
			if err != nil {
				t.Fatal(err)
			}
			response, err := decodeRPCResponsePayload(method, wire)
			if err != nil {
				t.Fatal(err)
			}
			var states []*rpcpb.MhsStateValue
			if method == RPCMethodClientMhsV0Read {
				v, e := response.AsClientMhsV0ReadResponse()
				err = e
				states = v.GetStates()
			} else {
				v, e := response.AsClientMhsV0WriteResponse()
				err = e
				states = v.GetStates()
			}
			if err != nil || len(states) != 1 || !proto.Equal(states[0], request.States[0]) {
				t.Fatalf("result %v %v", states, err)
			}
		}
	}
	for method, id := range map[RPCMethod]int32{RPCMethodClientMhsV0Read: 133, RPCMethodClientMhsV0Write: 134} {
		got, err := ProtoMethod(method)
		if err != nil || int32(got) != id {
			t.Fatalf("%v = %v, %v", method, got, err)
		}
	}
}
