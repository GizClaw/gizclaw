//go:build gizclaw_e2e

package admin_test

import (
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestAdminLookupPeerUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "502-admin-lookup-peer")

	h.CreateAdminContext("admin-a").MustSucceed(t)
	h.CreateContext("device-a").MustSucceed(t)

	h.RegisterContext("admin-a", "--sn", "admin-lookup-peer-sn").MustSucceed(t)
	h.RegisterContext(
		"device-a",
		"--sn", "admin-lookup-peer-device-a-sn",
		"--manufacturer", "Acme",
		"--model", "Model-A",
	).MustSucceed(t)

	devicePubKey := h.ContextPublicKey("device-a")

	resolve := h.RunCLI("admin", "peers", "resolve-sn", "admin-lookup-peer-device-a-sn", "--context", "admin-a")
	resolve.MustSucceed(t)
	if !strings.Contains(resolve.Stdout, devicePubKey) {
		t.Fatalf("expected resolved public key %q:\n%s", devicePubKey, resolve.Stdout)
	}

	get := h.RunCLI("admin", "peers", "get", devicePubKey, "--context", "admin-a")
	get.MustSucceed(t)
	if !strings.Contains(get.Stdout, `"public_key":"`+devicePubKey+`"`) {
		t.Fatalf("expected get output to include device public key:\n%s", get.Stdout)
	}

	info := h.RunCLI("admin", "peers", "info", devicePubKey, "--context", "admin-a")
	info.MustSucceed(t)
	for _, fragment := range []string{`"sn":"admin-lookup-peer-device-a-sn"`, `"manufacturer":"Acme"`, `"model":"Model-A"`} {
		if !strings.Contains(info.Stdout, fragment) {
			t.Fatalf("expected info output to include %q:\n%s", fragment, info.Stdout)
		}
	}
}
