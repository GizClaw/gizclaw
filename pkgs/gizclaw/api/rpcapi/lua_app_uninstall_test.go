package rpcapi

import (
	"strings"
	"testing"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

func TestLuaAppUninstallContract(t *testing.T) {
	tool, err := ClientToolByName("lua.app.uninstall")
	if err != nil || tool != rpcpb.ClientTool_CLIENT_TOOL_LUA_APP_UNINSTALL || int(tool) != 27 {
		t.Fatalf("tool=%v err=%v", tool, err)
	}
	for _, id := range []string{"demo", "a.b-c_1", strings.Repeat("a", 32), "", "../demo", "a/b", "Demo", "a\x00b", strings.Repeat("a", 33), "演示"} {
		request := &rpcpb.ClientLuaAppUninstallRequest{AppId: id}
		valid := id == "demo" || id == "a.b-c_1" || id == strings.Repeat("a", 32)
		if err := ValidateClientToolRequest(request); (err == nil) != valid {
			t.Fatalf("id=%q err=%v", id, err)
		}
	}
	if err := ValidateLuaAppResponse(&rpcpb.ClientLuaAppUninstallResponse{}); err != nil {
		t.Fatal(err)
	}
}
