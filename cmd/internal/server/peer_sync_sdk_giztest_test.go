//go:build gizclaw_sdk_e2e

package server

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestPeerSyncSDKGiztests uses the same scenario and real Server as the Go lane.
// The Flutter runner must be a built native executable, not a Dart-only test.
func TestPeerSyncSDKGiztests(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, runner := range []struct {
		name    string
		command []string
	}{
		{"JavaScript", []string{"node", "--experimental-strip-types", filepath.Join(root, "tests/gizclaw-e2e/js/giztest/index.ts")}},
		{"Flutter", []string{os.Getenv("GIZCLAW_SYNC_FLUTTER_RUNNER")}},
	} {
		t.Run(runner.name, func(t *testing.T) {
			if runner.command[0] == "" {
				t.Fatal("native runner executable is required")
			}
			runPeerSyncGiztest(t, func(ctx context.Context, file, report string) ([]byte, error) {
				args := append(append([]string{}, runner.command[1:]...), "run", file, "--parallel", "1", "--output", report)
				command := exec.CommandContext(ctx, runner.command[0], args...)
				command.Dir = root
				return command.CombinedOutput()
			})
		})
	}
}
