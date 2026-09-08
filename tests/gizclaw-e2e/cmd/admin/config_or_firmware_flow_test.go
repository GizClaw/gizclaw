//go:build gizclaw_e2e

package admin_test

import (
	"fmt"
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
  "id":"devkit",
  "slots":{"stable":{},"beta":{},"develop":{}}
}`)
	firmware := h.RunCLI("admin", "firmwares", "create", "-f", firmwarePath, "--context", "admin-a")
	firmware.MustSucceed(t)
	firmwareID := adminCreatedResourceID(t, firmware.Stdout)

	workflowPath := filepath.Join(h.SandboxDir, "echo-workflow.json")
	writeAdminFixture(t, workflowPath, `{
		"apiVersion":"gizclaw.admin/v1alpha1",
		"kind":"Workflow",
		"metadata":{"id":"device-echo"},
		"spec":{"driver":"flowcraft","flowcraft":{"graph":{
			"name":"device-echo-passthrough",
			"entry":"passthrough",
			"nodes":[{"id":"passthrough","type":"passthrough","publish":true}],
			"edges":[{"from":"passthrough","to":"__end__"}]
		}}}
	}`)
	workflow := h.RunCLI("admin", "apply", "-f", workflowPath, "--context", "admin-a")
	workflow.MustSucceed(t)
	workflowID := adminAppliedResourceID(t, workflow.Stdout)

	profilePath := filepath.Join(h.SandboxDir, "runtime-profile.json")
	writeAdminFixture(t, profilePath, fmt.Sprintf(`{
  "id":"device-default",
  "spec":{
    "resources":{},
    "workflows":{"collections":{"assistants":{"echo":{"resource_id":%q,"i18n":{"en":{"display_name":"Echo"},"zh-CN":{"display_name":"回声"}}}}}}
  }
}`, workflowID))
	profile := h.RunCLI("admin", "runtime-profiles", "create", "-f", profilePath, "--context", "admin-a")
	profile.MustSucceed(t)
	profileID := adminCreatedResourceID(t, profile.Stdout)
	assertContains(t, profile.Stdout, `"resource_id":"`+workflowID+`"`)

	tokenPath := filepath.Join(h.SandboxDir, "registration-token.json")
	writeAdminFixture(t, tokenPath, fmt.Sprintf(`{
  "id":"device-default",
  "runtime_profile_id":%q,
  "firmware_id":%q,
  "token":"e2e-device-default-token"
}`, profileID, firmwareID))
	created := h.RunCLI("admin", "registration-tokens", "create", "-f", tokenPath, "--context", "admin-a")
	created.MustSucceed(t)
	tokenID := adminCreatedResourceID(t, created.Stdout)
	assertContains(t, created.Stdout, `"token":"`, `"runtime_profile_id":"`+profileID+`"`, `"firmware_id":"`+firmwareID+`"`)
	if strings.Contains(created.Stdout, `"channel"`) {
		t.Fatalf("registration token persisted a Firmware channel:\n%s", created.Stdout)
	}

	got := h.RunCLI("admin", "registration-tokens", "get", tokenID, "--context", "admin-a")
	got.MustSucceed(t)
	assertContains(t, got.Stdout, `"token":"e2e-device-default-token"`, `"firmware_id":"`+firmwareID+`"`)

	h.RunCLI("admin", "registration-tokens", "delete", tokenID, "--context", "admin-a").MustSucceed(t)
	h.RunCLI("admin", "runtime-profiles", "delete", profileID, "--context", "admin-a").MustSucceed(t)
}

func writeAdminFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
