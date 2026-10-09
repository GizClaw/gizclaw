package rpcapi

import (
	"strings"
	"testing"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

func TestLuaAppRequestBounds(t *testing.T) {
	for _, request := range []proto.Message{
		&rpcpb.ClientLuaAppListRequest{},
		&rpcpb.ClientLuaAppInstallRequest{Url: "https://apps.test/tetris.lua-app.tar.zlib?token=opaque"},
		&rpcpb.ClientLuaAppRunRequest{AppId: "tetris", Params: map[string]string{"mode": "single", "text": "玩一局", "empty": ""}},
	} {
		if err := ValidateClientToolRequest(request); err != nil {
			t.Fatal(err)
		}
	}
	for _, request := range []proto.Message{
		&rpcpb.ClientLuaAppInstallRequest{Url: "http://apps.test/app"},
		&rpcpb.ClientLuaAppInstallRequest{Url: "https://user:pass@apps.test/app"},
		&rpcpb.ClientLuaAppInstallRequest{Url: "https://apps.test/app#"},
		&rpcpb.ClientLuaAppInstallRequest{Url: "https://apps.test/app", Sha256: new("bad")},
		&rpcpb.ClientLuaAppRunRequest{AppId: "../game"},
		&rpcpb.ClientLuaAppRunRequest{AppId: "Game"},
		&rpcpb.ClientLuaAppRunRequest{AppId: "game", Params: map[string]string{"": "empty key"}},
		&rpcpb.ClientLuaAppRunRequest{AppId: "game", Params: map[string]string{"text": "a\x00b"}},
		&rpcpb.ClientLuaAppRunRequest{AppId: "game", Params: map[string]string{"text": strings.Repeat("字", 342)}},
		&rpcpb.ClientLuaAppRunRequest{AppId: "game", Params: map[string]string{"a": strings.Repeat("a", 1024), "b": strings.Repeat("a", 1024), "c": strings.Repeat("a", 1024), "d": strings.Repeat("a", 1024)}},
	} {
		if ValidateClientToolRequest(request) == nil {
			t.Fatalf("accepted invalid %T", request)
		}
	}
}

func TestLuaAppCatalogAndCodec(t *testing.T) {
	app := &rpcpb.LuaAppInfo{AppId: "tetris", Version: "0.1.0", DisplayName: new("俄罗斯方块")}
	if err := ValidateLuaAppResponse(&rpcpb.ClientLuaAppListResponse{Apps: []*rpcpb.LuaAppInfo{app}}); err != nil {
		t.Fatal(err)
	}
	if ValidateLuaAppResponse(&rpcpb.ClientLuaAppListResponse{Apps: []*rpcpb.LuaAppInfo{app, app}}) == nil {
		t.Fatal("accepted duplicate IDs")
	}
	if ValidateLuaAppResponse(&rpcpb.ClientLuaAppInstallResponse{}) == nil {
		t.Fatal("accepted missing installed app")
	}
	request, err := ClientToolRequestMessage(rpcpb.ClientTool_CLIENT_TOOL_LUA_APP_RUN, map[string]any{"app_id": "tetris", "params": map[string]any{"mode": "single", "level": "2"}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ClientToolRequestFromBytes(rpcpb.ClientTool_CLIENT_TOOL_LUA_APP_RUN, data)
	if err != nil || !proto.Equal(request, decoded) {
		t.Fatalf("decoded=%v err=%v", decoded, err)
	}
}
