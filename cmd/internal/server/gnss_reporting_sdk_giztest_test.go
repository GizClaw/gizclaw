//go:build gizclaw_sdk_e2e

package server

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestGNSSReportingSDKGiztests requires every native SDK runner to execute every
// shared scenario. Missing runners, skipped steps and failed cleanup fail the test.
func TestGNSSReportingSDKGiztests(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, runner := range []struct {
		name    string
		command []string
	}{
		{"JavaScript", []string{"node", "--experimental-strip-types", filepath.Join(root, "tests/gizclaw-e2e/js/giztest/index.ts")}},
		{"C", []string{os.Getenv("GIZCLAW_GNSS_C_RUNNER"), "test"}},
		{"Flutter", []string{os.Getenv("GIZCLAW_GNSS_FLUTTER_RUNNER")}},
	} {
		t.Run(runner.name, func(t *testing.T) {
			if runner.command[0] == "" {
				t.Fatal("native runner executable is required")
			}
			runGNSSReportingGiztests(t, func(ctx context.Context, file, report string) ([]byte, error) {
				args := append(append([]string{}, runner.command[1:]...), "run", file, "--parallel", "1", "--output", report)
				command := exec.CommandContext(ctx, runner.command[0], args...)
				command.Dir = root
				return command.CombinedOutput()
			})
		})
	}
}
