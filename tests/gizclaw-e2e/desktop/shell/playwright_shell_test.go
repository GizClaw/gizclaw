//go:build gizclaw_e2e

package shell

import (
	"testing"

	desktop "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/desktop"
)

func TestDesktopShellPlaywright(t *testing.T) {
	h := desktop.NewHarnessForShell(t)
	h.RunForShell(t, h.FrontendDir(), "npm", "run", "test:e2e")
}
