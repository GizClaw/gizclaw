//go:build gizclaw_e2e

package migrate_test

import (
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestServerMigrateWorkspaceUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "204-server-migrate-workspace")
	h.ServerAddr = "127.0.0.1:0"
	h.PrepareServerWorkspaceFromFixture("server_config.yaml")

	for range 2 {
		result := h.RunCLI("migrate", "--workspace", h.ServerWorkspace)
		result.MustSucceed(t)
		if !strings.Contains(result.Stdout, "Migrated workspace ") {
			t.Fatalf("migrate output = %q", result.Stdout)
		}
	}
}
