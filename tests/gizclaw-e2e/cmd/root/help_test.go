//go:build gizclaw_e2e

package root_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestRootHelpUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "root")

	result := h.RunCLI("--help")
	result.MustSucceed(t)
	for _, want := range []string{"serve", "service", "context", "gen-key", "migrate", "connect", "admin", "edge"} {
		if !strings.Contains(result.Stdout, want) {
			t.Fatalf("root help missing %q:\n%s", want, result.Stdout)
		}
	}
	if strings.Contains(result.Stdout, "play") {
		t.Fatalf("root help should not include old Play UI command:\n%s", result.Stdout)
	}

	want := []string{"admin", "connect", "context", "edge", "gen-key", "migrate", "serve", "service"}
	got := productCommands(result.Stdout)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("product command inventory = %#v, want %#v\nhelp:\n%s", got, want, result.Stdout)
	}
	for _, command := range got {
		storyPath := filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "cmd", command, "USER_STORIES.md")
		if _, err := os.Stat(storyPath); err != nil {
			t.Fatalf("top-level command %q has no e2e owner at %s: %v", command, storyPath, err)
		}
	}
}

func productCommands(help string) []string {
	lines := strings.Split(help, "\n")
	commands := make([]string, 0, 8)
	inCommands := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "Available Commands:" {
			inCommands = true
			continue
		}
		if !inCommands {
			continue
		}
		if strings.TrimSpace(line) == "" {
			if len(commands) > 0 {
				break
			}
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] == "completion" || fields[0] == "help" {
			continue
		}
		commands = append(commands, fields[0])
	}
	return commands
}
