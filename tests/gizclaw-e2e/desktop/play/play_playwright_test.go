//go:build gizclaw_e2e

package play

import (
	"testing"

	desktop "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/desktop"
)

func TestDesktopPlayPlaywright(t *testing.T) {
	h := desktop.NewHarness(t)
	h.RunForShell(t, h.FrontendDir(), "npx", "playwright", "test", "e2e/play.spec.ts")
}
