//go:build gizclaw_e2e

package openai_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestAssistantScenariosWithLiveModel runs the Monitor diagnostic assistant's
// scenarios headlessly with a real RuntimeProfile model behind /openai/v1:
// the assistant's tools run against its FakeRuntime while every model call
// goes through the GizClaw OpenAI-compatible tool call contract.
func TestAssistantScenariosWithLiveModel(t *testing.T) {
	env := newOpenAIHarness(t)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("node is required for the assistant scenarios: %v", err)
	}
	packageDir := filepath.Join(env.h.RepoRoot, "web", "assistant")
	if _, err := os.Stat(filepath.Join(env.h.RepoRoot, "node_modules", "@openai", "agents-core")); err != nil {
		t.Fatalf("assistant dependencies are missing; run npm ci at the repository root: %v", err)
	}
	report := filepath.Join(t.TempDir(), "assistant-scenarios.json")
	ctx, cancel := context.WithTimeout(env.ctx, 15*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "--experimental-strip-types", "scripts/run-live-scenarios.ts")
	cmd.Dir = packageDir
	cmd.Env = append(os.Environ(),
		"GIZCLAW_ASSISTANT_BASE_URL="+env.baseURL,
		"GIZCLAW_ASSISTANT_API_KEY="+env.apiKey,
		"GIZCLAW_ASSISTANT_MODEL=llm",
		"GIZCLAW_ASSISTANT_REPORT="+report,
	)
	output, err := cmd.CombinedOutput()
	t.Logf("assistant scenarios:\n%s", output)
	if data, readErr := os.ReadFile(report); readErr == nil {
		t.Logf("assistant scenario report:\n%s", data)
	}
	if err != nil {
		t.Fatalf("assistant scenarios failed: %v", err)
	}
}
