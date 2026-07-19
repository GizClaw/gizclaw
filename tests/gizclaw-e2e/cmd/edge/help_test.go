//go:build gizclaw_e2e

package edge_test

import (
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestEdgeCommandUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "edge")

	for _, args := range [][]string{{"edge", "--help"}, {"edge", "serve", "--help"}} {
		result := h.RunCLI(args...)
		result.MustSucceed(t)
		if !strings.Contains(result.Stdout, "serve") {
			t.Fatalf("command %q help does not describe serve:\n%s", strings.Join(args, " "), result.Stdout)
		}
	}

	missingDir := h.RunCLI("edge", "serve")
	if missingDir.Err == nil || !strings.Contains(missingDir.Stderr, "accepts 1 arg") {
		t.Fatalf("edge serve without a workspace = err %v, stderr %q", missingDir.Err, missingDir.Stderr)
	}
}
