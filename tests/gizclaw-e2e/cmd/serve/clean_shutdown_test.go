//go:build gizclaw_e2e

package serve_test

import (
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestServerCleanShutdownUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "203-server-clean-shutdown")
	h.StartServerFromFixture("server_config.yaml")

	h.CreateContext("client-a").MustSucceed(t)
	if _, err := h.RunCLIUntilSuccess("connect", "ping", "--context", "client-a"); err != nil {
		t.Fatal(err)
	}

	h.StopServer()

	offline := h.RunCLI("connect", "ping", "--context", "client-a")
	if offline.Err == nil {
		t.Fatalf("expected ping to fail while server is stopped:\nstdout:\n%s\nstderr:\n%s", offline.Stdout, offline.Stderr)
	}
	offlineOutput := offline.Stderr + offline.Stdout
	if !strings.Contains(offlineOutput, "failed") &&
		!strings.Contains(offlineOutput, "timeout") &&
		!strings.Contains(offlineOutput, "connection refused") {
		t.Fatalf("expected offline ping failure message, got:\nstdout:\n%s\nstderr:\n%s", offline.Stdout, offline.Stderr)
	}

	h.RestartServer()
	if _, err := h.RunCLIUntilSuccess("connect", "ping", "--context", "client-a"); err != nil {
		t.Fatal(err)
	}
}
