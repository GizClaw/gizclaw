//go:build gizclaw_sdk_e2e

package server

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMonitorSignedSDKGiztest(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	runMonitorSignedGiztest(t, func(ctx context.Context, file, report string) ([]byte, error) {
		command := exec.CommandContext(ctx, "node", "--experimental-strip-types", filepath.Join(root, "tests/gizclaw-e2e/js/giztest/index.ts"), "run", file, "--parallel", "1", "--output", report)
		command.Dir = root
		return command.CombinedOutput()
	})
}

// A missing runner, skipped document/step, failed assertion or failed cleanup
// fails the lane. Flutter has no telemetry sender; Go/JS/C execute KV packets.
func TestDeviceManagementSDKGiztests(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, runner := range []struct {
		name      string
		command   []string
		telemetry bool
	}{
		{"JavaScript", []string{"node", "--experimental-strip-types", filepath.Join(root, "tests/gizclaw-e2e/js/giztest/index.ts")}, true},
		{"C", []string{os.Getenv("GIZCLAW_MANAGEMENT_C_RUNNER"), "test"}, true},
		{"Flutter", []string{os.Getenv("GIZCLAW_MANAGEMENT_FLUTTER_RUNNER")}, false},
	} {
		t.Run(runner.name, func(t *testing.T) {
			if runner.command[0] == "" {
				t.Fatal("native SDK runner executable is required")
			}
			runDeviceManagementGiztests(t, false, runner.telemetry, func(ctx context.Context, file, report string) ([]byte, error) {
				args := append(append([]string{}, runner.command[1:]...), "run", file, "--parallel", "1", "--output", report)
				cmd := exec.CommandContext(ctx, runner.command[0], args...)
				cmd.Dir = root
				return cmd.CombinedOutput()
			})
		})
	}
}
