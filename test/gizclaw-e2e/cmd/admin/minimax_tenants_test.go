//go:build gizclaw_e2e

package admin_test

import (
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
)

func TestAdminMiniMaxTenantsUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "505-admin-minimax-tenants")
	h.CreateContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)

	list := h.RunCLI("admin", "minimax-tenants", "list", "--context", "admin-a")
	list.MustSucceed(t)
	if !strings.Contains(list.Stdout, `"name":"minimax-main"`) {
		t.Fatalf("minimax tenants list missing created item:\n%s", list.Stdout)
	}

	get := h.RunCLI("admin", "minimax-tenants", "get", "minimax-main", "--context", "admin-a")
	get.MustSucceed(t)
	if !strings.Contains(get.Stdout, `"credential_name":"minimax-main-credential"`) {
		t.Fatalf("minimax tenants get missing credential:\n%s", get.Stdout)
	}
}
