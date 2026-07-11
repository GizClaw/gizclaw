//go:build gizclaw_e2e

package admin_test

import (
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestAdminMiniMaxTenantsUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "505-admin-minimax-tenants")
	h.CreateAdminContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)

	list := h.RunCLI("admin", "minimax-tenants", "list", "--context", "admin-a")
	list.MustSucceed(t)
	if !strings.Contains(list.Stdout, `"name":"minimax-cn"`) {
		t.Skipf("minimax-cn tenant is not configured in this e2e environment: %s", strings.TrimSpace(list.Stdout))
	}

	get := h.RunCLI("admin", "minimax-tenants", "get", "minimax-cn", "--context", "admin-a")
	get.MustSucceed(t)
	if !strings.Contains(get.Stdout, `"credential_name":"minimax-cn-credential"`) {
		t.Fatalf("minimax tenants get missing credential:\n%s", get.Stdout)
	}
}
