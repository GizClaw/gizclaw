package clientpublicretryablereads_test

import (
	"context"
	"testing"
	"time"

	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
)

func TestClientPublicRetryableReadsUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "302-client-public-retryable-reads")
	h.StartServerFromFixture("server_config.yaml")

	h.CreateContext("device-a").MustSucceed(t)
	h.RegisterContext("device-a", "--sn", "device-a-sn").MustSucceed(t)

	for i := range 4 {
		c := h.ConnectClientFromContext("device-a")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		config, err := c.GetGearConfig(ctx, "gear.config.get")
		cancel()
		_ = c.Close()
		if err != nil {
			t.Fatalf("get device config on iteration %d: %v", i, err)
		}
		if config == nil {
			t.Fatalf("expected public config response on iteration %d", i)
		}
		if _, err := h.RunCLIUntilSuccess("ping", "--context", "device-a"); err != nil {
			t.Fatal(err)
		}
	}
}
