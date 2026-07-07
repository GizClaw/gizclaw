//go:build gizclaw_e2e

package connect_test

import (
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestMultiClientHomeIsolationUserStory(t *testing.T) {
	server := clitest.NewSetupHarness(t, "402-multi-client-home-isolation")

	first := clitest.NewHarness(t, "402-multi-client-home-isolation")
	second := clitest.NewHarness(t, "402-multi-client-home-isolation")

	first.CreateContextWith("alpha", server.ServerAddr).MustSucceed(t)
	second.CreateContextWith("beta", server.ServerAddr).MustSucceed(t)

	firstList := first.ListContexts()
	firstList.MustSucceed(t)
	secondList := second.ListContexts()
	secondList.MustSucceed(t)

	if !strings.Contains(firstList.Stdout, "alpha") || strings.Contains(firstList.Stdout, "beta") {
		t.Fatalf("unexpected first home contexts:\n%s", firstList.Stdout)
	}
	if !strings.Contains(secondList.Stdout, "beta") || strings.Contains(secondList.Stdout, "alpha") {
		t.Fatalf("unexpected second home contexts:\n%s", secondList.Stdout)
	}

	if _, err := first.RunCLIUntilSuccess("connect", "ping", "--context", "alpha"); err != nil {
		t.Fatal(err)
	}
	if _, err := second.RunCLIUntilSuccess("connect", "ping", "--context", "beta"); err != nil {
		t.Fatal(err)
	}
}
