//go:build gizclaw_e2e

package connect_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestFirmwareSharedSetupDownload(t *testing.T) {
	h := clitest.NewSetupHarness(t, "304-firmware-shared-download")
	h.InstallFixedAdminContext("admin-a").MustSucceed(t)
	h.CreateContext("device-a").MustSucceed(t)
	h.RegisterContext("device-a", "--sn", "shared-firmware-device").MustSucceed(t)
	token := createFirmwareRegistrationToken(t, h)

	list := h.RunCLI("connect", "firmware", "list", "--context", "device-a", "--registration-token", token)
	list.MustSucceed(t)
	assertOutputContains(t, list.Stdout, `"name":"devkit-firmware-main"`, `"has_next":false`)

	getMain := h.RunCLI("connect", "firmware", "get", "--firmware-id", "devkit-firmware-main", "--context", "device-a", "--registration-token", token)
	getMain.MustSucceed(t)
	assertOutputContains(t, getMain.Stdout, `"name":"devkit-firmware-main"`)

	getOther := h.RunCLI("connect", "firmware", "get", "--firmware-id", "devkit-firmware-079", "--context", "device-a", "--registration-token", token)
	if getOther.Err == nil {
		t.Fatalf("other firmware unexpectedly accessible:\n%s", getOther.Stdout)
	}

	outputPath := filepath.Join(h.SandboxDir, "MANIFEST.txt")
	download := mustRunCLIJSON[firmwareDownloadCLIResponse](t, h, "connect", "firmware", "download", "--firmware-id", "devkit-firmware-main", "--channel", "stable", "--path", "MANIFEST.txt", "--output", outputPath, "--context", "device-a", "--registration-token", token)
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
	binDownload := mustRunCLIJSON[firmwareDownloadCLIResponse](t, h, "connect", "firmware", "download", "--firmware-id", "devkit-firmware-main", "--channel", "stable", "--path", "firmware/main.bin", "--output", binPath, "--context", "device-a", "--registration-token", token)
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

func createFirmwareRegistrationToken(t *testing.T, h *clitest.Harness) string {
	t.Helper()
	admin := h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	profileName := "e2e-firmware-main"
	profileResp, err := api.PutRuntimeProfileWithResponse(ctx, profileName, adminhttp.RuntimeProfileUpsert{
		Name: profileName,
		Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{}},
	})
	if err != nil {
		t.Fatalf("put RuntimeProfile: %v", err)
	}
	if profileResp.JSON200 == nil {
		t.Fatalf("put RuntimeProfile: err=%v status=%d body=%s", err, profileResp.StatusCode(), strings.TrimSpace(string(profileResp.Body)))
	}
	tokenName := "e2e-firmware-main-token"
	_, _ = api.DeleteRegistrationTokenWithResponse(ctx, tokenName)
	tokenResp, err := api.CreateRegistrationTokenWithResponse(ctx, adminhttp.RegistrationTokenUpsert{
		Name: tokenName, FirmwareName: "devkit-firmware-main", RuntimeProfileName: profileName,
	})
	if err != nil {
		t.Fatalf("create RegistrationToken: %v", err)
	}
	if tokenResp.JSON200 == nil || tokenResp.JSON200.Token == nil {
		t.Fatalf("create RegistrationToken: err=%v status=%d body=%s", err, tokenResp.StatusCode(), strings.TrimSpace(string(tokenResp.Body)))
	}
	return *tokenResp.JSON200.Token
}

func assertOutputContains(t *testing.T, output string, values ...string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(output, value) {
			t.Fatalf("output missing %s:\n%s", value, output)
		}
	}
}
