package clientpublicreadsequence_test

import (
	"context"
	"testing"
	"time"

	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
)

func TestClientPublicReadSequenceUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "300-client-public-read-sequence")
	h.StartServerFromFixture("server_config.yaml")

	h.CreateContext("device-a").MustSucceed(t)
	h.RegisterContext(
		"device-a",
		"--name", "device-a",
		"--sn", "device-a-sn",
		"--manufacturer", "Acme",
		"--model", "Model-A",
		"--depot", "demo",
		"--firmware-semver", "1.0.0",
	).MustSucceed(t)

	c := h.ConnectClientFromContext("device-a")
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	config, err := c.GetGearConfig(ctx, "gear.config.get")
	if err != nil {
		t.Fatalf("get device config: %v", err)
	}
	if config == nil {
		t.Fatal("expected public config response")
	}

	if _, err := h.RunCLIUntilSuccess("ping", "--context", "device-a"); err != nil {
		t.Fatal(err)
	}
}
