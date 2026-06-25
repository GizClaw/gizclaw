//go:build gizclaw_e2e

package chat

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestPushToTalkRoundtrip(t *testing.T) {
	runLiveWorkspaceCase(t, workspaceCasePushToTalkRoundtrip, allWorkspaceConfigPaths(t))
}

func allWorkspaceConfigPaths(t testing.TB) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "workspaces", "*.json"))
	if err != nil {
		t.Fatalf("glob workspace configs: %v", err)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		t.Fatal("no workspace configs found under testdata/workspaces")
	}
	return paths
}

func runLiveWorkspaceCase(t *testing.T, selected workspaceCase, paths []string) {
	t.Helper()
	if err := probeLiveWorkspaceSetup(); err != nil {
		if os.Getenv("GIZCLAW_E2E_REQUIRE_LIVE") == "1" {
			t.Fatalf("e2e setup server is not available: %v", err)
		}
		t.Skipf("e2e setup server is not available: %v", err)
	}
	for _, path := range paths {
		path := path
		t.Run(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), func(t *testing.T) {
			err := runConfig(path, clientContextConfigPath(), selected)
			if err == nil {
				return
			}
			if shouldSkipUnavailableSetup(err) {
				t.Skipf("e2e setup server is not available: %v", err)
			}
			t.Fatalf("%s %s: %v", selected, path, err)
		})
	}
}

func probeLiveWorkspaceSetup() error {
	contextPath := clientContextConfigPath()
	if contextPath == "" {
		contextPath = contextConfigDefaultPath
	}
	contextCfg, err := readSetupContextConfig(contextPath)
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("tcp", contextCfg.Server.Addr, 200*time.Millisecond)
	if err != nil {
		return err
	}
	return conn.Close()
}

func shouldSkipUnavailableSetup(err error) bool {
	if os.Getenv("GIZCLAW_E2E_REQUIRE_LIVE") == "1" {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	text := err.Error()
	return strings.Contains(text, "connection refused") ||
		strings.Contains(text, "no such file or directory") ||
		strings.Contains(text, "read context config")
}
