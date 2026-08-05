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
			"name": "devkit",
			"description": "Devkit firmware line",
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
				},
				"pending": {
					"description": "pending channel",
					"package": {
						"url": "https://downloads.example.com/devkit/pending.tar.zlib",
						"sha256": "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
						"size": 8192
					}
				}
			}
	}`), 0o644); err != nil {
		t.Fatalf("write firmware file: %v", err)
	}

	created := h.RunCLI("admin", "firmwares", "create", "-f", firmwarePath, "--context", "admin-a")
	created.MustSucceed(t)
	firmwareID := adminCreatedResourceID(t, created.Stdout)

	put := h.RunCLI("admin", "firmwares", "put", firmwareID, "-f", firmwarePath, "--context", "admin-a")
	put.MustSucceed(t)
	assertContains(t, put.Stdout, `"url":"https://downloads.example.com/devkit/stable.tar.zlib"`, `"size":4096`)

	list := h.RunCLI("admin", "firmwares", "list", "--context", "admin-a")
	list.MustSucceed(t)
	assertContains(t, list.Stdout, `"name":"devkit"`, `"description":"Devkit firmware line"`)

	get := h.RunCLI("admin", "firmwares", "get", firmwareID, "--context", "admin-a")
	get.MustSucceed(t)
	assertContains(t, get.Stdout,
		`"description":"stable channel"`, `"url":"https://downloads.example.com/devkit/stable.tar.zlib"`,
		`"description":"beta channel"`, `"url":"https://downloads.example.com/devkit/beta.tar.zlib"`,
		`"url":"https://downloads.example.com/devkit/develop.tar.zlib"`,
		`"description":"pending channel"`, `"url":"https://downloads.example.com/devkit/pending.tar.zlib"`,
	)

	release := h.RunCLI("admin", "firmwares", "release", firmwareID, "--context", "admin-a")
	release.MustSucceed(t)
	assertContains(t, release.Stdout, `"stable":{"description":"pending channel","package":{`, `"size":8192`, `"beta":{"description":"stable channel","package":{`)

	rollback := h.RunCLI("admin", "firmwares", "rollback", firmwareID, "--context", "admin-a")
	rollback.MustSucceed(t)
	assertContains(t, rollback.Stdout, `"stable":{"description":"stable channel","package":{`, `"size":4096`)

	resource := h.RunCLI("admin", "show", "Firmware", firmwareID, "--context", "admin-a")
	resource.MustSucceed(t)
	assertContains(t, resource.Stdout, `"kind":"Firmware"`, `"name":"devkit"`, `"url":"https://downloads.example.com/devkit/stable.tar.zlib"`)

	deleted := h.RunCLI("admin", "firmwares", "delete", firmwareID, "--context", "admin-a")
	deleted.MustSucceed(t)
	assertContains(t, deleted.Stdout, `"name":"devkit"`)
}

func assertContains(t *testing.T, output string, values ...string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(output, value) {
			t.Fatalf("output missing %s:\n%s", value, output)
		}
	}
}
