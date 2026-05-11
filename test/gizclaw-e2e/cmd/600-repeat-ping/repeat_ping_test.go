package repeatping_test

import (
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
)

func TestRepeatPingUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "600-repeat-ping")
	h.StartServerFromFixture("server_config.yaml")

	h.CreateContext("client-a").MustSucceed(t)
	for range 10 {
		if _, err := h.RunCLIUntilSuccess("ping", "--context", "client-a"); err != nil {
			t.Fatal(err)
		}
	}
}
