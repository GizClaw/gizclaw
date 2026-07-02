//go:build gizclaw_e2e

package chat

import (
	"net"
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
		t.Skipf("e2e setup server is not available: %v", err)
	}
	for _, path := range paths {
		path := path
		t.Run(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), func(t *testing.T) {
			err := runConfigWithLiveRetry(path, clientContextConfigPath(), selected)
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

func runConfigWithLiveRetry(path, contextConfigPath string, selected workspaceCase) error {
	var err error
	for attempt := 1; attempt <= 5; attempt++ {
		err = runConfig(path, contextConfigPath, selected)
		if err == nil || !isRetryableLiveWorkspaceError(err) {
			return err
		}
		if attempt < 5 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return err
}

func isRetryableLiveWorkspaceError(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "Bad Gateway") ||
		strings.Contains(text, "websocket read: unexpected EOF") ||
		strings.Contains(text, "websocket: close 1006 (abnormal closure): unexpected EOF") ||
		strings.Contains(text, "transport: kcp: timeout") ||
		strings.Contains(text, "response incomplete: length") ||
		strings.Contains(text, "flowcraft: claw event error: recall ingest: extract:") ||
		strings.Contains(text, "speech: POST \"http://gizclaw/v1/audio/speech\": 400 Bad Request") ||
		strings.Contains(text, "doubaospeech: [Server processing timeout] node execution timeout") ||
		strings.Contains(text, "transcript mismatch: similarity")
}

func probeLiveWorkspaceSetup() error {
	contextPath := clientContextConfigPath()
	if contextPath == "" {
		contextPath = defaultClientContextConfigPath()
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
	text := err.Error()
	return strings.Contains(text, "connection refused") ||
		strings.Contains(text, "no such file or directory") ||
		strings.Contains(text, "read context config")
}
