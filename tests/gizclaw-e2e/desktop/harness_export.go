//go:build gizclaw_e2e

package desktop

import "testing"

type HarnessForShell struct {
	harness
}

type Harness = HarnessForShell

func NewHarness(t *testing.T) Harness {
	t.Helper()
	return Harness{harness: newHarness(t)}
}

func NewHarnessForShell(t *testing.T) HarnessForShell {
	t.Helper()
	return NewHarness(t)
}

func (h HarnessForShell) FrontendDir() string {
	return h.repoRoot + "/apps/wails/frontend"
}

func (h HarnessForShell) WailsDir() string {
	return h.wailsDir
}

func (h HarnessForShell) RunForShell(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	return h.run(t, dir, name, args...)
}
