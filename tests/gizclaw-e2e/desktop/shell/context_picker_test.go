//go:build gizclaw_e2e

package shell

import (
	"testing"

	desktop "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/desktop"
)

func TestDesktopContextPickerCreatesAndSelectsContext(t *testing.T) {
	h := desktop.NewHarnessForShell(t)
	h.RunForShell(t, h.WailsDir(), "go", "test", "-run", "TestContextBridgeCreateContext", "./internal/bridge")
}
