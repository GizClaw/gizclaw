//go:build gizclaw_e2e

package connect_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestRegistrationDoesNotProjectFirmware(t *testing.T) {
	h := clitest.NewSetupHarness(t, "304-firmware-shared-download")
	h.InstallFixedAdminContext("admin-a").MustSucceed(t)
	h.CreateContext("device-a").MustSucceed(t)
	h.RegisterContext("device-a", "--sn", "shared-firmware-device").MustSucceed(t)
	token := createRuntimeProfileRegistrationToken(t, h)

	list := h.RunCLI("connect", "firmware", "list", "--context", "device-a", "--registration-token", token)
	list.MustSucceed(t)
	assertOutputContains(t, list.Stdout, `"items":[]`, `"has_next":false`)

	getMain := h.RunCLI("connect", "firmware", "get", "--firmware-id", "devkit-firmware-main", "--context", "device-a", "--registration-token", token)
	if getMain.Err == nil {
		t.Fatalf("registration unexpectedly projected Firmware:\n%s", getMain.Stdout)
	}

	download := h.RunCLI("connect", "firmware", "download", "--firmware-id", "devkit-firmware-main", "--channel", "stable", "--path", "MANIFEST.txt", "--output", h.SandboxDir+"/MANIFEST.txt", "--context", "device-a", "--registration-token", token)
	if download.Err == nil {
		t.Fatalf("registration unexpectedly allowed Firmware download:\n%s", download.Stdout)
	}
}

func createRuntimeProfileRegistrationToken(t *testing.T, h *clitest.Harness) string {
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
		Name: tokenName, RuntimeProfileName: profileName,
	})
	if err != nil {
		t.Fatalf("create RegistrationToken: %v", err)
	}
	if tokenResp.JSON200 == nil || tokenResp.JSON200.Token == "" {
		t.Fatalf("create RegistrationToken: err=%v status=%d body=%s", err, tokenResp.StatusCode(), strings.TrimSpace(string(tokenResp.Body)))
	}
	return tokenResp.JSON200.Token
}

func assertOutputContains(t *testing.T, output string, values ...string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(output, value) {
			t.Fatalf("output missing %s:\n%s", value, output)
		}
	}
}
