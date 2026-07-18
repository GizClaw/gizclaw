//go:build gizclaw_e2e

package connect_test

import (
	"context"
	"testing"
	"time"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestClientPublicRetryableReadsUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "302-client-public-retryable-reads")

	h.CreateContext("device-a").MustSucceed(t)
	h.RegisterContext("device-a", "--sn", "device-a-sn").MustSucceed(t)

	c := h.ConnectClientFromContext("device-a")
	defer func() { _ = c.Close() }()

	for i := range 4 {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		info, err := c.GetServerInfo(ctx, "server.info.get")
		cancel()
		if err != nil {
			t.Fatalf("get device info on iteration %d: %v", i, err)
		}
		if info == nil || info.Identifiers == nil || info.Identifiers.Sn == nil || *info.Identifiers.Sn != "device-a-sn" {
			t.Fatalf("expected device info response on iteration %d, got %+v", i, info)
		}
	}
}
