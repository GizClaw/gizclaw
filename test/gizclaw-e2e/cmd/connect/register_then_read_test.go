//go:build gizclaw_e2e

package connect_test

import (
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
)

func TestClientRegisterThenReadUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "301-client-register-then-read")

	h.CreateContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "connect-register-read-admin-sn").MustSucceed(t)
	h.ApproveAdminPeer(h.ContextPublicKey("admin-a"))

	h.CreateContext("device-a").MustSucceed(t)
	h.RegisterContext(
		"device-a",
		"--name", "device-a",
		"--sn", "connect-register-read-device-a-sn",
		"--manufacturer", "Acme",
		"--model", "Model-A",
	).MustSucceed(t)

	devicePubKey := h.ContextPublicKey("device-a")

	info := h.RunCLI("admin", "peers", "info", devicePubKey, "--context", "admin-a")
	info.MustSucceed(t)
	for _, fragment := range []string{`"sn":"connect-register-read-device-a-sn"`, `"manufacturer":"Acme"`, `"model":"Model-A"`} {
		if !strings.Contains(info.Stdout, fragment) {
			t.Fatalf("admin info output missing %q:\n%s", fragment, info.Stdout)
		}
	}
}
