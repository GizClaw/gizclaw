//go:build gizclaw_e2e

package admin_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestAdminRuntimeProfileRegistrationTokenFlow(t *testing.T) {
	h := clitest.NewHarness(t, "503-admin-runtime-profile-flow")
	h.StartServerFromFixture("server_config.yaml")
	h.CreateAdminContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)

	firmwarePath := filepath.Join(h.SandboxDir, "firmware.json")
	writeAdminFixture(t, firmwarePath, `{
  "name":"devkit",
  "slots":{"stable":{},"beta":{},"develop":{},"pending":{}}
}`)
	h.RunCLI("admin", "firmwares", "put", "devkit", "-f", firmwarePath, "--context", "admin-a").MustSucceed(t)

	profilePath := filepath.Join(h.SandboxDir, "runtime-profile.json")
	writeAdminFixture(t, profilePath, `{
  "name":"device-default",
  "spec":{"resources":{"models":{"primary":"model-default"},"pet_defs":{"tragon":"petdef-tragon"}}}
}`)
	profile := h.RunCLI("admin", "runtime-profiles", "create", "-f", profilePath, "--context", "admin-a")
	profile.MustSucceed(t)
	assertContains(t, profile.Stdout, `"models":{"primary":"model-default"}`, `"pet_defs":{"tragon":"petdef-tragon"}`)

	tokenPath := filepath.Join(h.SandboxDir, "registration-token.json")
	writeAdminFixture(t, tokenPath, `{
  "name":"device-default",
  "runtime_profile_name":"device-default"
}`)
	created := h.RunCLI("admin", "registration-tokens", "create", "-f", tokenPath, "--context", "admin-a")
	created.MustSucceed(t)
	assertContains(t, created.Stdout, `"token":"`, `"runtime_profile_name":"device-default"`)
	if strings.Contains(created.Stdout, `"firmware_name"`) {
		t.Fatalf("registration token retained Firmware coupling:\n%s", created.Stdout)
	}

	got := h.RunCLI("admin", "registration-tokens", "get", "device-default", "--context", "admin-a")
	got.MustSucceed(t)
	if strings.Contains(got.Stdout, `"token"`) {
		t.Fatalf("registration token metadata leaked raw token:\n%s", got.Stdout)
	}

	h.RunCLI("admin", "registration-tokens", "delete", "device-default", "--context", "admin-a").MustSucceed(t)
	h.RunCLI("admin", "runtime-profiles", "delete", "device-default", "--context", "admin-a").MustSucceed(t)
}

func writeAdminFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
