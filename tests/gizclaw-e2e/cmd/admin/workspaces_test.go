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

	get := h.RunCLI("admin", "workspaces", "get", "workspace-flowcraft-assistant", "--context", "admin-a")
	get.MustSucceed(t)
	if !strings.Contains(get.Stdout, `"workflow_name":"flowcraft-voice-assistant"`) {
		t.Fatalf("workspaces get missing workflow name:\n%s", get.Stdout)
	}

	rpcGet := h.RunCLI("admin", "workspaces", "get", "support-desk-workspace", "--context", "admin-a")
	rpcGet.MustSucceed(t)
	if !strings.Contains(rpcGet.Stdout, `"workflow_name":"flowcraft-support"`) {
		t.Fatalf("workspaces get missing resource workflow name:\n%s", rpcGet.Stdout)
	}
}
