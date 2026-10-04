//go:build gizclaw_e2e

package admin_test

import (
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestAdminWorkflowsUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "507-admin-workflows")
	h.CreateAdminContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)

	list := h.RunCLI("admin", "workflows", "list", "--context", "admin-a")
	list.MustSucceed(t)
	if !strings.Contains(list.Stdout, `"id":"eino-voice-assistant"`) {
		t.Fatalf("workflows list missing eino-voice-assistant:\n%s", list.Stdout)
	}
	for _, want := range []string{`"id":"eino-chat-assistant"`, `"id":"eino-scenario-119"`} {
		if !strings.Contains(list.Stdout, want) {
			t.Fatalf("workflows list missing %q:\n%s", want, list.Stdout)
		}
	}
	voiceWorkflowID := adminResourceID(t, list.Stdout, "eino-voice-assistant")
	chatWorkflowID := adminResourceID(t, list.Stdout, "eino-chat-assistant")

	get := h.RunCLI("admin", "workflows", "get", voiceWorkflowID, "--context", "admin-a")
	get.MustSucceed(t)
	if !strings.Contains(get.Stdout, `"driver":"eino"`) {
		t.Fatalf("workflows get missing driver:\n%s", get.Stdout)
	}

	rpcGet := h.RunCLI("admin", "workflows", "get", chatWorkflowID, "--context", "admin-a")
	rpcGet.MustSucceed(t)
	if !strings.Contains(rpcGet.Stdout, `"id":"eino-chat-assistant"`) || !strings.Contains(rpcGet.Stdout, `"driver":"eino"`) {
		t.Fatalf("workflows get missing resource fields:\n%s", rpcGet.Stdout)
	}

}
