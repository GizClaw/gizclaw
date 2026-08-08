//go:build gizclaw_e2e

package admin_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestAdminFirmwaresUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "511-admin-firmwares")
	h.StartServerFromFixture("server_config.yaml")
	h.CreateAdminContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)

	firmwarePath := filepath.Join(h.SandboxDir, "firmware.json")
	if err := os.WriteFile(firmwarePath, []byte(`{
			"id": "devkit",
			"description": "Devkit firmware channels",
			"slots": {
				"stable": {
					"description": "stable channel",
					"package": {
						"url": "https://downloads.example.com/devkit/stable.tar.zlib",
						"sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
						"size": 4096
					}
				},
				"beta": {
					"description": "beta channel",
					"package": {
						"url": "https://downloads.example.com/devkit/beta.tar.zlib",
						"sha256": "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
						"size": 6144
					}
				},
				"develop": {
					"package": {
						"url": "https://downloads.example.com/devkit/develop.tar.zlib",
						"sha256": "123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0",
						"size": 7168
					}
				}
			}
	}`), 0o644); err != nil {
		t.Fatalf("write firmware file: %v", err)
	}

	created := h.RunCLI("admin", "firmwares", "create", "-f", firmwarePath, "--context", "admin-a")
	created.MustSucceed(t)
	firmwareID := adminCreatedResourceID(t, created.Stdout)
	if firmwareID != "devkit" {
		t.Fatalf("created firmware ID = %q, want caller ID devkit", firmwareID)
	}

	put := h.RunCLI("admin", "firmwares", "put", firmwareID, "-f", firmwarePath, "--context", "admin-a")
	put.MustSucceed(t)
	assertContains(t, put.Stdout, `"url":"https://downloads.example.com/devkit/stable.tar.zlib"`, `"size":4096`)

	list := h.RunCLI("admin", "firmwares", "list", "--context", "admin-a")
	list.MustSucceed(t)
	assertContains(t, list.Stdout, `"id":"devkit"`, `"description":"Devkit firmware channels"`)
	assertNotContains(t, list.Stdout, `"name":"devkit"`)

	get := h.RunCLI("admin", "firmwares", "get", firmwareID, "--context", "admin-a")
	get.MustSucceed(t)
	assertContains(t, get.Stdout,
		`"description":"stable channel"`, `"url":"https://downloads.example.com/devkit/stable.tar.zlib"`,
		`"description":"beta channel"`, `"url":"https://downloads.example.com/devkit/beta.tar.zlib"`,
		`"url":"https://downloads.example.com/devkit/develop.tar.zlib"`,
	)
	assertNotContains(t, get.Stdout, `"pen`+`ding"`)

	for _, removedCommand := range []string{"release", "rollback"} {
		result := h.RunCLI("admin", "firmwares", removedCommand, firmwareID, "--context", "admin-a")
		if result.Err == nil {
			t.Fatalf("removed %s command succeeded:\n%s", removedCommand, result.Stdout)
		}
	}

	resource := h.RunCLI("admin", "show", "Firmware", firmwareID, "--context", "admin-a")
	resource.MustSucceed(t)
	assertContains(t, resource.Stdout, `"kind":"Firmware"`, `"metadata":{"id":"devkit"}`, `"url":"https://downloads.example.com/devkit/stable.tar.zlib"`)
	assertNotContains(t, resource.Stdout, `"name":"devkit"`)

	deleted := h.RunCLI("admin", "firmwares", "delete", firmwareID, "--context", "admin-a")
	deleted.MustSucceed(t)
	assertContains(t, deleted.Stdout, `"id":"devkit"`)
	assertNotContains(t, deleted.Stdout, `"name":"devkit"`)
}

func assertNotContains(t *testing.T, output string, values ...string) {
	t.Helper()
	for _, value := range values {
		if strings.Contains(output, value) {
			t.Fatalf("output unexpectedly contains %s:\n%s", value, output)
		}
	}
}

func assertContains(t *testing.T, output string, values ...string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(output, value) {
			t.Fatalf("output missing %s:\n%s", value, output)
		}
	}
}
