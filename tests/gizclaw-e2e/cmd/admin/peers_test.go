//go:build gizclaw_e2e

package admin_test

import (
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestAdminListPeersUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "501-admin-list-peers")

	h.CreateAdminContext("admin-a").MustSucceed(t)
	h.CreateContext("device-a").MustSucceed(t)
	h.CreateContext("device-b").MustSucceed(t)

	h.RegisterContext("admin-a", "--sn", "admin-list-peers-sn").MustSucceed(t)
	h.RegisterContext("device-a", "--sn", "admin-list-peers-device-a-sn").MustSucceed(t)
	h.RegisterContext("device-b", "--sn", "admin-list-peers-device-b-sn").MustSucceed(t)

	list := h.RunCLI("admin", "peers", "list", "--context", "admin-a")
	list.MustSucceed(t)

	for _, publicKey := range []string{
		h.ContextPublicKey("admin-a"),
		h.ContextPublicKey("device-a"),
		h.ContextPublicKey("device-b"),
	} {
		if !strings.Contains(list.Stdout, publicKey) {
			t.Fatalf("expected admin peer list to include %q:\n%s", publicKey, list.Stdout)
		}
	}
}
