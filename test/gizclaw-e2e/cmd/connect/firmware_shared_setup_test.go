//go:build gizclaw_e2e

package connect_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
)

func TestFirmwareSharedSetupDownload(t *testing.T) {
	h := clitest.NewSetupHarness(t, "304-firmware-shared-download")
	h.CreateContext("device-a").MustSucceed(t)
	h.RegisterContext("device-a", "--sn", "shared-firmware-device").MustSucceed(t)
	applyClientView(t, h, h.ContextPublicKey("device-a"))

	list := h.RunCLI("connect", "firmware", "list", "--context", "device-a")
	list.MustSucceed(t)
	assertOutputContains(t, list.Stdout, `"name":"devkit-firmware-000"`, `"has_next":true`)

	getMain := h.RunCLI("connect", "firmware", "get", "--firmware-id", "devkit-firmware-main", "--context", "device-a")
	getMain.MustSucceed(t)
	assertOutputContains(t, getMain.Stdout, `"name":"devkit-firmware-main"`)

	getLast := h.RunCLI("connect", "firmware", "get", "--firmware-id", "devkit-firmware-079", "--context", "device-a")
	getLast.MustSucceed(t)
	assertOutputContains(t, getLast.Stdout, `"name":"devkit-firmware-079"`)

	outputPath := filepath.Join(h.SandboxDir, "MANIFEST.txt")
	download := mustRunCLIJSON[firmwareDownloadCLIResponse](t, h, "connect", "firmware", "download", "--firmware-id", "devkit-firmware-main", "--channel", "stable", "--path", "MANIFEST.txt", "--output", outputPath, "--context", "device-a")
	if download.Bytes <= 0 || download.Metadata.File.Path != "MANIFEST.txt" {
		t.Fatalf("firmware download = %#v", download)
	}
	payload, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read downloaded firmware: %v", err)
	}
	if !bytes.Contains(payload, []byte("gizclaw devkit firmware")) {
		t.Fatalf("downloaded firmware manifest missing text")
	}

	binPath := filepath.Join(h.SandboxDir, "main.bin")
	binDownload := mustRunCLIJSON[firmwareDownloadCLIResponse](t, h, "connect", "firmware", "download", "--firmware-id", "devkit-firmware-main", "--channel", "stable", "--path", "firmware/main.bin", "--output", binPath, "--context", "device-a")
	if binDownload.Bytes <= 0 || binDownload.Metadata.File.Path != "firmware/main.bin" {
		t.Fatalf("firmware bin download = %#v", binDownload)
	}
	binPayload, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("read downloaded firmware bin: %v", err)
	}
	if !bytes.Contains(binPayload, []byte("GIZCLAW_MAIN_FIRMWARE_V1")) {
		t.Fatalf("downloaded firmware bin missing marker")
	}
}

type firmwareDownloadCLIResponse struct {
	Metadata rpcapi.FirmwareFilesDownloadResponse `json:"metadata"`
	Bytes    int64                                `json:"bytes"`
	Output   string                               `json:"output"`
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

func applyClientView(t *testing.T, h *clitest.Harness, peerPublicKey string) {
	t.Helper()

	script := filepath.Join(h.RepoRoot, "test", "gizclaw-e2e", "setup", "apply_client_view.sh")
	cmd := exec.Command(script, peerPublicKey)
	cmd.Dir = h.RepoRoot
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("apply client view: %v\n%s", err, string(output))
	}
}

func assertOutputContains(t *testing.T, output string, values ...string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(output, value) {
			t.Fatalf("output missing %s:\n%s", value, output)
		}
	}
}
