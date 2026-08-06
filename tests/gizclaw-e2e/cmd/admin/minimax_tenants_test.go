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
	if !strings.Contains(list.Stdout, `"id":"minimax-cn"`) {
		t.Fatalf("minimax-cn tenant is not configured in this e2e environment: %s", strings.TrimSpace(list.Stdout))
	}
	tenantID := adminResourceID(t, list.Stdout, "minimax-cn")
	credentials := h.RunCLI("admin", "credentials", "list", "--context", "admin-a")
	credentials.MustSucceed(t)
	credentialID := adminResourceID(t, credentials.Stdout, "minimax-cn-credential")

	get := h.RunCLI("admin", "minimax-tenants", "get", tenantID, "--context", "admin-a")
	get.MustSucceed(t)
	if !strings.Contains(get.Stdout, `"credential_id":"`+credentialID+`"`) {
		t.Fatalf("minimax tenants get missing credential:\n%s", get.Stdout)
	}
}
