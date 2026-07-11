//go:build gizclaw_e2e

package admin_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestAdminConfigFlowUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "503-admin-config-flow")
	h.StartServerFromFixture("server_config.yaml")

	h.CreateAdminContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)
	h.CreateContext("device-a").MustSucceed(t)
	h.RegisterContext("device-a", "--sn", "device-sn").MustSucceed(t)
	devicePubKey := h.ContextPublicKey("device-a")

	configPath := filepath.Join(h.SandboxDir, "peer-config.json")
	if err := os.WriteFile(configPath, []byte(`{"view":"under-12"}`), 0o644); err != nil {
		t.Fatalf("write peer config: %v", err)
	}
	putConfig := h.RunCLI("admin", "peers", "put-config", devicePubKey, "--file", configPath, "--context", "admin-a")
	putConfig.MustSucceed(t)
	if !strings.Contains(putConfig.Stdout, `"view":"under-12"`) {
		t.Fatalf("expected put-config output to include view:\n%s", putConfig.Stdout)
	}
}
