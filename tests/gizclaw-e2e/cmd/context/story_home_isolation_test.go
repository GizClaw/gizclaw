//go:build gizclaw_e2e

package context_test

import (
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestContextIsolationBetweenStoryHomesUserStory(t *testing.T) {
	server := clitest.NewSetupHarness(t, "103-context-isolation-between-story-homes")

	first := clitest.NewHarness(t, "103-context-isolation-between-story-homes")
	second := clitest.NewHarness(t, "103-context-isolation-between-story-homes")

	first.CreateContextWith("alpha", server.ServerAddr).MustSucceed(t)
	second.CreateContextWith("beta", server.ServerAddr).MustSucceed(t)

	firstList := first.ListContexts()
	firstList.MustSucceed(t)
	secondList := second.ListContexts()
	secondList.MustSucceed(t)

	if !strings.Contains(firstList.Stdout, "alpha") || strings.Contains(firstList.Stdout, "beta") {
		t.Fatalf("unexpected first home list:\n%s", firstList.Stdout)
	}
	if !strings.Contains(secondList.Stdout, "beta") || strings.Contains(secondList.Stdout, "alpha") {
		t.Fatalf("unexpected second home list:\n%s", secondList.Stdout)
	}

	if _, err := first.RunCLIUntilSuccess("connect", "ping", "--context", "alpha"); err != nil {
		t.Fatal(err)
	}
	if _, err := second.RunCLIUntilSuccess("connect", "ping", "--context", "beta"); err != nil {
		t.Fatal(err)
	}
}
