//go:build gizclaw_e2e

package connect_test

import (
	"context"
	"testing"
	"time"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestClientPublicReadSequenceUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "300-client-public-read-sequence")

	h.CreateContext("device-a").MustSucceed(t)
	h.RegisterContext(
		"device-a",
		"--name", "device-a",
		"--sn", "device-a-sn",
		"--manufacturer", "Acme",
		"--model", "Model-A",
	).MustSucceed(t)

	c := h.ConnectClientFromContext("device-a")
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	info, err := c.GetServerInfo(ctx, "server.info.get")
	if err != nil {
		t.Fatalf("get device info: %v", err)
	}
	if info == nil || info.Identifiers == nil || info.Identifiers.Sn == nil || *info.Identifiers.Sn != "device-a-sn" {
		t.Fatalf("expected device info response, got %+v", info)
	}

	if _, err := h.RunCLIUntilSuccess("connect", "ping", "--context", "device-a"); err != nil {
		t.Fatal(err)
	}
}
