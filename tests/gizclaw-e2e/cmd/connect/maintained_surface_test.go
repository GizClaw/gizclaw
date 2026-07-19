//go:build gizclaw_e2e

package connect_test

import (
	"encoding/json"
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestConnectMaintainedSurfaceUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "306-maintained-surface")
	h.CreateContext("device-a").MustSucceed(t)
	h.RegisterContext("device-a", "--sn", "connect-maintained-surface-device-a-sn").MustSucceed(t)

	runStatus := h.RunCLI("connect", "run-status", "--timeout", "10s", "--context", "device-a")
	runStatus.MustSucceed(t)
	if !json.Valid([]byte(runStatus.Stdout)) {
		t.Fatalf("run-status output is not JSON: %q", runStatus.Stdout)
	}

	speed := h.RunCLI(
		"connect", "test-speed",
		"--up-content-length", "4096",
		"--down-content-length", "4096",
		"--timeout", "10s",
		"--context", "device-a",
	)
	speed.MustSucceed(t)
	for _, want := range []string{"Up Bytes:     4096", "Down Bytes:   4096"} {
		if !strings.Contains(speed.Stdout, want) {
			t.Fatalf("test-speed output missing %q:\n%s", want, speed.Stdout)
		}
	}

	for _, args := range [][]string{
		{"connect", "contact", "--help"},
		{"connect", "friend", "--help"},
		{"connect", "friend-group", "--help"},
		{"connect", "firmware", "--help"},
		{"connect", "gameplay", "--help"},
	} {
		result := h.RunCLI(args...)
		result.MustSucceed(t)
		if !strings.Contains(result.Stdout, "Available Commands:") {
			t.Fatalf("command %q does not expose its maintained subcommands:\n%s", strings.Join(args, " "), result.Stdout)
		}
	}

	missingVoice := h.RunCLI("connect", "say", "hello", "--context", "device-a")
	if missingVoice.Err == nil || !strings.Contains(missingVoice.Stderr, "voice id is required") {
		t.Fatalf("say without --voice = err %v, stderr %q", missingVoice.Err, missingVoice.Stderr)
	}
}
