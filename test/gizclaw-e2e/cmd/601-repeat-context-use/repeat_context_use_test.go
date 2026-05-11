package repeatcontextuse_test

import (
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
)

func TestRepeatContextUseUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "601-repeat-context-use")
	h.StartServerFromFixture("server_config.yaml")

	h.CreateContext("alpha").MustSucceed(t)
	h.CreateContext("beta").MustSucceed(t)

	for range 4 {
		h.UseContext("alpha").MustSucceed(t)
		if _, err := h.RunCLIUntilSuccess("ping"); err != nil {
			t.Fatal(err)
		}

		h.UseContext("beta").MustSucceed(t)
		if _, err := h.RunCLIUntilSuccess("ping"); err != nil {
			t.Fatal(err)
		}
	}

	list := h.ListContexts()
	list.MustSucceed(t)
	if !strings.Contains(list.Stdout, "* beta") {
		t.Fatalf("expected beta to remain current:\n%s", list.Stdout)
	}
}
