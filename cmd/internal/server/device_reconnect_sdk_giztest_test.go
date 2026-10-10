//go:build gizclaw_sdk_e2e

package server

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestDeviceReconnectSDKGiztest verifies that C reconnect finishes only after
// the Server can serve device control through the replacement connection.
func TestDeviceReconnectSDKGiztest(t *testing.T) {
	binary := os.Getenv("GIZCLAW_DEVICE_C_RUNNER")
	if binary == "" {
		t.Fatal("native C runner executable is required")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	runDeviceReconnectGiztest(t, func(ctx context.Context, file, report string) ([]byte, error) {
		command := exec.CommandContext(ctx, binary, "test", "run", file, "--output", report)
		command.Dir = root
		return command.CombinedOutput()
	})
}
