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

func TestRegistrationBindsFirmware(t *testing.T) {
	h := clitest.NewSetupHarness(t, "304-firmware-shared-config")
	h.InstallFixedAdminContext("admin-a").MustSucceed(t)
	h.CreateContext("device-a").MustSucceed(t)
	h.RegisterContext("device-a", "--sn", "shared-firmware-device").MustSucceed(t)
	token := createRuntimeProfileRegistrationToken(t, h)

	tests := []struct {
		channel string
		url     string
		size    string
	}{
		{"stable", "https://firmware.example.invalid/devkit/stable.tar.zlib", `"size":4096`},
		{"beta", "https://firmware.example.invalid/devkit/beta.tar.zlib", `"size":8192`},
		{"develop", "https://firmware.example.invalid/devkit/develop.tar.zlib", `"size":12288`},
	}
	for _, tc := range tests {
		t.Run(tc.channel, func(t *testing.T) {
			result := h.RunCLI("connect", "firmware", "get", "--channel", tc.channel, "--context", "device-a", "--registration-token", token)
			result.MustSucceed(t)
			assertOutputContains(t, result.Stdout, `"channel":"`+tc.channel+`"`, `"url":"`+tc.url+`"`, tc.size)
			if strings.Contains(result.Stdout, `"firmware_name"`) {
				t.Fatalf("firmware response unexpectedly exposes identity:\n%s", result.Stdout)
			}
		})
	}
	legacyChannel := "pen" + "ding"
	rejected := h.RunCLI("connect", "firmware", "get", "--channel", legacyChannel, "--context", "device-a", "--registration-token", token)
	if rejected.Err == nil || !strings.Contains(rejected.Stderr, "channel must be one of stable, beta, develop") {
		t.Fatalf("pending channel result = err:%v stderr:%s", rejected.Err, rejected.Stderr)
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
	profile, err := clitest.UpsertRuntimeProfile(ctx, api, adminhttp.RuntimeProfileUpsert{
		Id: profileName,
		Spec: apitypes.RuntimeProfileSpec{
			Resources: apitypes.RuntimeProfileResources{},
			Workflows: apitypes.RuntimeProfileWorkflows{
				System: apitypes.RuntimeProfileSystemWorkflows{
					Pet: "pet-care",
				},
				Collections: apitypes.RuntimeProfileWorkflowCollections{},
			},
		},
	})
	if err != nil {
		t.Fatalf("put RuntimeProfile: %v", err)
	}
	tokenName := "e2e-firmware-main-token"
	if err := clitest.DeleteRegistrationTokenByID(ctx, api, tokenName); err != nil {
		t.Fatalf("retire RegistrationToken: %v", err)
	}
	firmware, found, err := clitest.FirmwareByID(ctx, api, "devkit-firmware-main")
	if err != nil || !found {
		t.Fatalf("resolve Firmware devkit-firmware-main: found=%v err=%v", found, err)
	}
	firmwareID := firmware.Id
	tokenResp, err := api.CreateRegistrationTokenWithResponse(ctx, adminhttp.RegistrationTokenUpsert{
		Id: tokenName, Token: tokenName, RuntimeProfileId: profile.Id, FirmwareId: &firmwareID,
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
