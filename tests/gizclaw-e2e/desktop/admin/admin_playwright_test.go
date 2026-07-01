//go:build gizclaw_e2e

package admin

import (
	"testing"

	desktop "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/desktop"
)

func TestDesktopAdminPlaywright(t *testing.T) {
	h := desktop.NewHarness(t)
	h.RunForShell(t, h.FrontendDir(), "npx", "playwright", "test", "e2e/admin.spec.ts")
}
