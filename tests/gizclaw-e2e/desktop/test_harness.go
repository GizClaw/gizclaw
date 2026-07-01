//go:build gizclaw_e2e

package desktop

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type harness struct {
	configHome string
	repoRoot   string
	wailsDir   string
}

func newHarness(t *testing.T) harness {
	t.Helper()
	root := repoRoot(t)
	return harness{
		configHome: filepath.Join(t.TempDir(), "desktop-config"),
		repoRoot:   root,
		wailsDir:   filepath.Join(root, "apps", "wails"),
	}
}

func (h harness) run(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIZCLAW_DESKTOP_CONFIG_HOME="+h.configHome)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, output.String())
	}
	return output.String()
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		next := filepath.Dir(wd)
		if next == wd {
			t.Fatalf("repo root not found from %s", wd)
		}
		wd = next
	}
}
