package repeatserverrestart_test

import (
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
)

func TestRepeatServerRestartUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "602-repeat-server-restart")
	h.StartServerFromFixture("server_config.yaml")

	h.CreateContext("client-a").MustSucceed(t)
	for range 3 {
		if _, err := h.RunCLIUntilSuccess("ping", "--context", "client-a"); err != nil {
			t.Fatal(err)
		}
		h.StopServer()
		h.RestartServer()
	}
	if _, err := h.RunCLIUntilSuccess("ping", "--context", "client-a"); err != nil {
		t.Fatal(err)
	}
}
