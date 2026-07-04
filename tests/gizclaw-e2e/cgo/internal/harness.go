//go:build gizclaw_e2e

package internal

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func SharedIdentityDir(t *testing.T, h *clitest.Harness, contextEnv, defaultContext string) string {
	t.Helper()
	identitiesHome := os.Getenv("GIZCLAW_E2E_IDENTITIES_HOME")
	if identitiesHome == "" {
		identitiesHome = filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "identities")
	}
	contextName := os.Getenv(contextEnv)
	if contextName == "" {
		contextName = defaultContext
	}
	return filepath.Join(identitiesHome, contextName)
}

func AssertServerAvailable(t *testing.T, identityDir string) {
	t.Helper()
	endpoint := ReadEndpoint(t, filepath.Join(identityDir, "config.yaml"))
	client := http.Client{Timeout: time.Second}
	resp, err := client.Get("http://" + endpoint + "/server-info")
	if err != nil {
		t.Fatalf("gizclaw e2e setup server is required at %s; run ./tests/gizclaw-e2e/run_tests.sh: %v", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("server-info status=%d at %s", resp.StatusCode, endpoint)
	}
}

func ReadEndpoint(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "endpoint:"
	for _, line := range splitLines(string(data)) {
		trimmed := trim(line)
		if len(trimmed) >= len(prefix) && trimmed[:len(prefix)] == prefix {
			return trim(trimmed[len(prefix):])
		}
	}
	t.Fatalf("missing endpoint in %s", path)
	return ""
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start <= len(s) {
		out = append(out, s[start:])
	}
	return out
}

func trim(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '"' || s[start] == '\'') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r' || s[end-1] == '"' || s[end-1] == '\'') {
		end--
	}
	return s[start:end]
}
