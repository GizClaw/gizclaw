package agenthost

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/mcpclient"
)

func TestMCPProfileResolutionAndLiveAuthorization(t *testing.T) {
	for _, change := range []string{"unrelated revision", "source endpoint", "source headers", "source removed", "permission removed", "owner changed", "workflow changed", "profile changed"} {
		t.Run(change, func(t *testing.T) {
			registry := mcpclient.NewRegistry()
			defer registry.Close()
			profile := apitypes.RuntimeProfile{Id: "profile", Revision: "old", Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{McpServers: &map[string]apitypes.RuntimeProfileMCPServer{"knowledge": {Endpoint: "http://127.0.0.1:1/mcp", Headers: &map[string]string{"Authorization": "original"}}}}, Workflows: apitypes.RuntimeProfileWorkflows{"assistant": {ResourceId: "flow", Mcp: &map[string]apitypes.RuntimeProfileMCPSelection{"knowledge": {Resources: new(true)}}}}}}
			ws := apitypes.Workspace{Id: "workspace", WorkflowId: "flow", OwnerPublicKey: new("owner"), Labels: &map[string]string{"workflow_name": "assistant"}}
			items := map[string]apitypes.Workspace{"workspace": ws}
			resolver := ServiceResolver{MCPRegistry: registry, Workspaces: fakeWorkspaceService{items: items}, RuntimeProfileForOwner: func(context.Context, string) (apitypes.RuntimeProfile, error) { return profile, nil }}
			config, err := resolver.resolveMCP(withRuntimeProfile(t.Context(), profile), ws)
			if err != nil || config == nil {
				t.Fatalf("resolve MCP = %v, %v", config, err)
			}
			ctx := WithResourceAccess(t.Context(), "owner", nil, nil)
			if err := config.Authorize(ctx); err != nil {
				t.Fatal(err)
			}
			if err := config.Authorize(WithResourceAccess(t.Context(), "other-owner", nil, nil)); err == nil {
				t.Fatal("MCP crossed owner boundary")
			}
			profile.Revision = "new"
			switch change {
			case "source endpoint":
				binding := (*profile.Spec.Resources.McpServers)["knowledge"]
				binding.Endpoint = "http://127.0.0.1:2/mcp"
				(*profile.Spec.Resources.McpServers)["knowledge"] = binding
			case "source headers":
				binding := (*profile.Spec.Resources.McpServers)["knowledge"]
				(*binding.Headers)["Authorization"] = "rotated"
			case "source removed":
				delete(*profile.Spec.Resources.McpServers, "knowledge")
			case "permission removed":
				binding := profile.Spec.Workflows["assistant"]
				delete(*binding.Mcp, "knowledge")
			case "owner changed":
				ws.OwnerPublicKey = new("other-owner")
				items["workspace"] = ws
			case "workflow changed":
				ws.WorkflowId = "different-flow"
				items["workspace"] = ws
			case "profile changed":
				profile.Id = "different-profile"
			}
			if err := config.Authorize(ctx); (err == nil) != (change == "unrelated revision") {
				t.Fatalf("authorization after %s = %v", change, err)
			}
		})
	}
	resolver := ServiceResolver{}
	if config, err := resolver.resolveMCP(t.Context(), apitypes.Workspace{}); err != nil || config != nil {
		t.Fatalf("unconfigured MCP should not affect Workspace: %v, %v", config, err)
	}
}
