//go:build gizclaw_sdk_e2e

package server

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestFirmwareMetadataSDKGiztests runs the committed metadata scenario through
// native SDK transports; missing executables and process failures are errors.
func TestFirmwareMetadataSDKGiztests(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, runner := range []struct {
		name    string
		command []string
	}{
		{"JavaScript", []string{"node", "--experimental-strip-types", filepath.Join(root, "tests/gizclaw-e2e/js/giztest/index.ts")}},
		{"C", []string{os.Getenv("GIZCLAW_FIRMWARE_METADATA_C_RUNNER"), "test"}},
		{"Flutter", []string{os.Getenv("GIZCLAW_FIRMWARE_METADATA_FLUTTER_RUNNER")}},
	} {
		t.Run(runner.name, func(t *testing.T) {
			if runner.command[0] == "" {
				t.Fatal("native runner executable is required")
			}
			runFirmwareMetadataGiztest(t, func(ctx context.Context, file, report string) ([]byte, error) {
				args := append(append([]string{}, runner.command[1:]...), "run", file, "--parallel", "1", "--output", report)
				command := exec.CommandContext(ctx, runner.command[0], args...)
				command.Dir = root
				return command.CombinedOutput()
			})
		})
	}
}
