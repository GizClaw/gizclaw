//go:build gizclaw_e2e

package admin_test

import (
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestAdminWorkspacesUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "508-admin-workspaces")
	h.CreateAdminContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)

	list := h.RunCLI("admin", "workspaces", "list", "--context", "admin-a")
	list.MustSucceed(t)
	if !strings.Contains(list.Stdout, `"name":"workspace-flowcraft-assistant"`) {
		t.Fatalf("workspaces list missing created item:\n%s", list.Stdout)
	}
	for _, want := range []string{`"name":"support-desk-workspace"`, `"name":"workspace-scenario-119"`} {
		if !strings.Contains(list.Stdout, want) {
			t.Fatalf("workspaces list missing %q:\n%s", want, list.Stdout)
		}
	}
	assistantWorkspaceID := adminResourceID(t, list.Stdout, "workspace-flowcraft-assistant")
	supportWorkspaceID := adminResourceID(t, list.Stdout, "support-desk-workspace")
	workflows := h.RunCLI("admin", "workflows", "list", "--context", "admin-a")
	workflows.MustSucceed(t)
	voiceWorkflowID := adminResourceID(t, workflows.Stdout, "flowcraft-voice-assistant")
	chatWorkflowID := adminResourceID(t, workflows.Stdout, "flowcraft-chat-assistant")

	get := h.RunCLI("admin", "workspaces", "get", assistantWorkspaceID, "--context", "admin-a")
	get.MustSucceed(t)
	if !strings.Contains(get.Stdout, `"workflow_id":"`+voiceWorkflowID+`"`) {
		t.Fatalf("workspaces get missing canonical workflow ID:\n%s", get.Stdout)
	}

	rpcGet := h.RunCLI("admin", "workspaces", "get", supportWorkspaceID, "--context", "admin-a")
	rpcGet.MustSucceed(t)
	if !strings.Contains(rpcGet.Stdout, `"workflow_id":"`+chatWorkflowID+`"`) {
		t.Fatalf("workspaces get missing canonical workflow ID:\n%s", rpcGet.Stdout)
	}
}
