//go:build gizclaw_e2e

package rpc_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestServerToolRPCIsReadOnlyRuntimeCatalog(t *testing.T) {
	env := newServerResourceHarness(t)
	listed, err := env.peer.ListTools(env.ctx, "tool.list", rpcapi.ToolListRequest{})
	if err != nil {
		t.Fatalf("server.tool.list: %v", err)
	}
	if len(listed.Items) != 0 {
		t.Fatalf("server.tool.list = %#v, want empty unconfigured RuntimeProfile catalog", listed.Items)
	}
	if _, err := env.peer.GetTool(env.ctx, "tool.get.missing", rpcapi.ToolGetRequest{Alias: "missing"}); err == nil {
		t.Fatal("server.tool.get unexpectedly resolved an unconfigured alias")
	}
}
