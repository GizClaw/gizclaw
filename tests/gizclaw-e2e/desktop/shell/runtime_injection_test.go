//go:build gizclaw_e2e

package shell

import (
	"testing"

	desktop "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/desktop"
)

func TestDesktopRuntimeInjectionIncludesInMemoryCredential(t *testing.T) {
	h := desktop.NewHarnessForShell(t)
	h.RunForShell(t, h.WailsDir(), "go", "test", "-run", "TestAppExposesContextRuntimeWithoutServerAccess", ".")
	h.RunForShell(t, h.FrontendDir(), "npm", "test")
}
