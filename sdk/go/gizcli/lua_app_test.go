package gizcli

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

func TestLuaAppProviderDiscoveryAndParameters(t *testing.T) {
	device := &Client{}
	var got *rpcpb.ClientLuaAppRunRequest
	if err := device.HandleClientTool(rpcpb.ClientTool_CLIENT_TOOL_LUA_APP_RUN, func(_ context.Context, message proto.Message) (proto.Message, error) {
		got = proto.CloneOf(message.(*rpcpb.ClientLuaAppRunRequest))
		return &rpcpb.ClientLuaAppRunResponse{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	list := deviceControlDispatch(t, device, rpcapi.RPCMethodClientToolV0List, nil)
	listed, err := list.Result.AsClientToolV0ListResponse()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range listed.Tools {
		if tool == rpcpb.ClientTool_CLIENT_TOOL_LUA_APP_RUN {
			found = true
		}
	}
	if !found {
		t.Fatal("installed launch handler is not advertised")
	}
	request := &rpcpb.ClientLuaAppRunRequest{AppId: "tetris", Params: map[string]string{"level": "2", "mode": "single"}}
	response := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_LUA_APP_RUN, func(p *rpcapi.RPCPayload) error { return p.FromClientLuaAppRunRequest(request) })
	if response.Error != nil || !proto.Equal(got, request) {
		t.Fatalf("response=%v got=%v", response, got)
	}
	got = nil
	request.Params["level"] = "bad\x00value"
	response = deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_LUA_APP_RUN, func(p *rpcapi.RPCPayload) error { return p.FromClientLuaAppRunRequest(request) })
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeInvalidArgument || got != nil {
		t.Fatal("invalid arguments reached launch handler")
	}
	missing := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_LUA_APP_INSTALL, func(p *rpcapi.RPCPayload) error {
		return p.FromClientLuaAppInstallRequest(&rpcpb.ClientLuaAppInstallRequest{Url: "https://apps.test/tetris.lua-app.tar.zlib"})
	})
	if missing.Error == nil || missing.Error.Code != rpcapi.StatusCodeUnimplemented {
		t.Fatalf("missing=%v", missing)
	}
}
