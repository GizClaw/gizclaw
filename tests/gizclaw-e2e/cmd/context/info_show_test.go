//go:build gizclaw_e2e

package context_test

import (
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestContextInfoAndShowUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "104-context-info-show")

	noCurrent := h.RunCLI("context", "info")
	if noCurrent.Err == nil {
		t.Fatalf("context info without an active context should fail:\nstdout:\n%s", noCurrent.Stdout)
	}

	h.CreateContext("alpha").MustSucceed(t)
	h.CreateContextWith("beta", "127.0.0.1:9821").MustSucceed(t)
	h.UseContext("beta").MustSucceed(t)

	info := h.RunCLI("context", "info")
	info.MustSucceed(t)
	for _, want := range []string{`"name":"beta"`, `"current":true`, `"server_address":"127.0.0.1:9821"`} {
		if !strings.Contains(info.Stdout, want) {
			t.Fatalf("context info missing %q:\n%s", want, info.Stdout)
		}
	}

	show := h.RunCLI("context", "show", "alpha")
	show.MustSucceed(t)
	for _, want := range []string{`"name":"alpha"`, `"current":false`, `"server_address":"` + h.ServerAddr + `"`} {
		if !strings.Contains(show.Stdout, want) {
			t.Fatalf("context show missing %q:\n%s", want, show.Stdout)
		}
	}

	serverInfo := h.RunCLI("connect", "server-info", "--context", "alpha")
	serverInfo.MustSucceed(t)
	if !strings.Contains(serverInfo.Stdout, `"public_key":"`+h.ServerPublicKey+`"`) {
		t.Fatalf("server-info missing server public key:\n%s", serverInfo.Stdout)
	}

	missing := h.RunCLI("context", "show", "missing")
	if missing.Err == nil {
		t.Fatalf("context show missing should fail:\nstdout:\n%s", missing.Stdout)
	}

	legacyAlias := h.RunCLI("ctx", "list")
	if legacyAlias.Err == nil {
		t.Fatalf("legacy ctx alias should be removed:\nstdout:\n%s", legacyAlias.Stdout)
	}
}
