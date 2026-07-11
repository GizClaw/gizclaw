//go:build gizclaw_e2e

package admin_test

import (
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestAdminContextProvisionUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "500-admin-context-provision")

	h.CreateAdminContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-context-provision-sn").MustSucceed(t)

	after := h.RunCLI("admin", "peers", "list", "--context", "admin-a")
	after.MustSucceed(t)
}
