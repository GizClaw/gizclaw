//go:build gizclaw_e2e

package admin_test

import (
	"archive/tar"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
				"stable": {"description": "stable channel"},
				"beta": {"description": "beta channel"},
				"develop": {"description": "develop channel"},
				"pending": {"description": "pending channel"}
			}
	}`), 0o644); err != nil {
		t.Fatalf("write firmware file: %v", err)
	}
	appTarPath := filepath.Join(h.SandboxDir, "app.tar")
	writeFirmwareTarFile(t, appTarPath, map[string]string{
		"MANIFEST.txt":            "cmd firmware artifact bundle",
		"firmware/main.bin":       "app firmware payload",
		"firmware/voice_dsp.bin":  "voice dsp firmware payload",
		"assets/icons/status.png": "png firmware asset payload",
		"config/device.json":      `{"modules":["main","voice_dsp"]}`,
	})
	dataTarPath := filepath.Join(h.SandboxDir, "data.tar")
	writeFirmwareTarFile(t, dataTarPath, map[string]string{"assets/data.txt": "data firmware payload"})

	put := h.RunCLI("admin", "firmwares", "put", "devkit", "-f", firmwarePath, "--context", "admin-a")
	put.MustSucceed(t)
	assertContains(t, put.Stdout, `"name":"devkit"`, `"description":"stable channel"`)

	list := h.RunCLI("admin", "firmwares", "list", "--context", "admin-a")
	list.MustSucceed(t)
	assertContains(t, list.Stdout, `"name":"devkit"`, `"description":"Devkit firmware line"`)

	get := h.RunCLI("admin", "firmwares", "get", "devkit", "--context", "admin-a")
	get.MustSucceed(t)
	assertContains(t, get.Stdout, `"name":"devkit"`, `"description":"stable channel"`)

	uploadApp := h.RunCLI("admin", "firmwares", "upload-artifact", "devkit", "--channel", "stable", "-f", appTarPath, "--context", "admin-a")
	uploadApp.MustSucceed(t)
	assertContains(t, uploadApp.Stdout, `"tar_path":"devkit/stable/artifact/artifact.tar"`, `"sha256":`)

	artifactList := h.RunCLI("admin", "firmwares", "artifact", "ls", "devkit", "--channel", "stable", "--path", "firmware", "--context", "admin-a")
	artifactList.MustSucceed(t)
	assertContains(t, artifactList.Stdout, `"path":"firmware/main.bin"`, `"path":"firmware/voice_dsp.bin"`)

	artifactTree := h.RunCLI("admin", "firmwares", "artifact", "tree", "devkit", "--channel", "stable", "--context", "admin-a")
	artifactTree.MustSucceed(t)
	assertContains(t, artifactTree.Stdout, `"path":"assets/icons/status.png"`, `"path":"config/device.json"`, `"path":"firmware/main.bin"`)

	artifactStat := h.RunCLI("admin", "firmwares", "artifact", "stat", "devkit", "--channel", "stable", "--path", "assets/icons/status.png", "--context", "admin-a")
	artifactStat.MustSucceed(t)
	assertContains(t, artifactStat.Stdout, `"path":"assets/icons/status.png"`, `"type":"file"`)

	entryDownloadPath := filepath.Join(h.SandboxDir, "artifact-main.bin")
	artifactDownload := h.RunCLI("admin", "firmwares", "artifact", "dl", "devkit", "--channel", "stable", "--path", "firmware/main.bin", "--output", entryDownloadPath, "--context", "admin-a")
	artifactDownload.MustSucceed(t)
	assertContains(t, artifactDownload.Stdout, `"output":"`+entryDownloadPath+`"`)
	assertFileContent(t, entryDownloadPath, "app firmware payload")

	tarDownloadPath := filepath.Join(h.SandboxDir, "stable-artifact.tar")
	artifactTar := h.RunCLI("admin", "firmwares", "download-artifact", "devkit", "--channel", "stable", "--output", tarDownloadPath, "--context", "admin-a")
	artifactTar.MustSucceed(t)
	assertContains(t, artifactTar.Stdout, `"output":"`+tarDownloadPath+`"`)
	assertTarContains(t, tarDownloadPath, "firmware/main.bin", "assets/icons/status.png", "config/device.json")

	uploadDevelop := h.RunCLI("admin", "firmwares", "upload-artifact", "devkit", "--channel", "develop", "-f", dataTarPath, "--context", "admin-a")
	uploadDevelop.MustSucceed(t)
	assertContains(t, uploadDevelop.Stdout, `"tar_path":"devkit/develop/artifact/artifact.tar"`)

	deleteDevelop := h.RunCLI("admin", "firmwares", "delete-artifact", "devkit", "--channel", "develop", "--context", "admin-a")
	deleteDevelop.MustSucceed(t)
	assertContains(t, deleteDevelop.Stdout, `"name":"devkit"`)

	uploadData := h.RunCLI("admin", "firmwares", "upload-artifact", "devkit", "--channel", "pending", "-f", dataTarPath, "--context", "admin-a")
	uploadData.MustSucceed(t)
	assertContains(t, uploadData.Stdout, `"tar_path":"devkit/pending/artifact/artifact.tar"`, `"sha256":`)

	release := h.RunCLI("admin", "firmwares", "release", "devkit", "--context", "admin-a")
	release.MustSucceed(t)
	assertContains(t, release.Stdout, `"stable":{"artifact":{`, `"tar_path":"devkit/pending/artifact/artifact.tar"`, `"beta":{"artifact":{`, `"tar_path":"devkit/stable/artifact/artifact.tar"`)

	rollback := h.RunCLI("admin", "firmwares", "rollback", "devkit", "--context", "admin-a")
	rollback.MustSucceed(t)
	assertContains(t, rollback.Stdout, `"stable":{"artifact":{`, `"tar_path":"devkit/stable/artifact/artifact.tar"`)

	resource := h.RunCLI("admin", "show", "Firmware", "devkit", "--context", "admin-a")
	resource.MustSucceed(t)
	assertContains(t, resource.Stdout, `"kind":"Firmware"`, `"name":"devkit"`)

	delete := h.RunCLI("admin", "firmwares", "delete", "devkit", "--context", "admin-a")
	delete.MustSucceed(t)
	assertContains(t, delete.Stdout, `"name":"devkit"`)
}

func writeFirmwareTarFile(t *testing.T, filePath string, files map[string]string) {
	t.Helper()
	f, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("create tar %s: %v", filePath, err)
	}
	defer f.Close()
	tw := tar.NewWriter(f)
	modTime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for name, body := range files {
		data := []byte(body)
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(data)), ModTime: modTime}); err != nil {
			t.Fatalf("WriteHeader(%s): %v", name, err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatalf("Write(%s): %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar %s: %v", filePath, err)
	}
}

func assertFileContent(t *testing.T, filePath string, want string) {
	t.Helper()
	payload, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read %s: %v", filePath, err)
	}
	if string(payload) != want {
		t.Fatalf("%s payload = %q, want %q", filePath, string(payload), want)
	}
}

func assertTarContains(t *testing.T, filePath string, paths ...string) {
	t.Helper()
	f, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("open tar %s: %v", filePath, err)
	}
	defer f.Close()
	seen := make(map[string]bool)
	tr := tar.NewReader(f)
	for {
		header, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("read tar %s: %v", filePath, err)
		}
		seen[header.Name] = true
	}
	for _, path := range paths {
		if !seen[path] {
			t.Fatalf("tar %s missing %q; seen=%v", filePath, path, seen)
		}
	}
}

func mustRunCLIJSON[T any](t *testing.T, h *clitest.Harness, args ...string) T {
	t.Helper()
	result, err := h.RunCLIUntilSuccess(args...)
	if err != nil {
		t.Fatalf("%v failed: %v\nstdout:\n%s\nstderr:\n%s", args, err, result.Stdout, result.Stderr)
	}
	var out T
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		t.Fatalf("decode %v output: %v\n%s", args, err, result.Stdout)
	}
	return out
}

func assertContains(t *testing.T, output string, values ...string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(output, value) {
			t.Fatalf("output missing %s:\n%s", value, output)
		}
	}
}
