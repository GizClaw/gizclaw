//go:build gizclaw_e2e

package rpc_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	cgointernal "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cgo/internal"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

const cSDKFirmwareID = "devkit-firmware-main"

func TestCSDKPing(t *testing.T) {
	runCSDKRPC(t, "ping", cgointernal.CSDKPing)
}

func TestCSDKServerRuntime(t *testing.T) {
	runCSDKRPC(t, "server-runtime", cgointernal.CSDKServerRuntime)
}

func TestCSDKServerStatus(t *testing.T) {
	runCSDKRPC(t, "server-status", cgointernal.CSDKServerStatus)
}

func TestCSDKSpeedTest(t *testing.T) {
	runCSDKRPC(t, "speed-test", cgointernal.CSDKSpeedTest)
}

func TestCSDKServerInitiatedPing(t *testing.T) {
	fixture := cgointernal.NewServerRPCFixture(t)
	response, err := fixture.Ping("server-ping")
	if err != nil {
		t.Fatal(err)
	}
	if response.ServerTime <= 0 {
		t.Fatalf("server_time = %d", response.ServerTime)
	}
}

func TestCSDKServerInitiatedSpeedTest(t *testing.T) {
	tests := []struct {
		name string
		up   int64
		down int64
	}{
		{name: "zero"},
		{name: "upload-only", up: 32*1024 + 7},
		{name: "download-only", down: 32*1024 + 11},
		{name: "full-duplex", up: 64*1024 + 3, down: 64*1024 + 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := cgointernal.NewServerRPCFixture(t)
			uploaded, downloaded, err := fixture.SpeedTest("server-speed-"+tc.name, tc.up, tc.down)
			if err != nil {
				t.Fatal(err)
			}
			if uploaded != tc.up || downloaded != tc.down {
				t.Fatalf("transferred up=%d down=%d, want up=%d down=%d", uploaded, downloaded, tc.up, tc.down)
			}
		})
	}
}

func TestCSDKFirmwareRPC(t *testing.T) {
	runRegisteredCSDKRPC(t, "firmware-rpc", cgointernal.CSDKFirmwareRPC)
}

func TestCSDKFirmwareRPCMaximumID(t *testing.T) {
	h := clitest.NewSetupHarness(t, "cgo-rpc-firmware-maximum-id")
	identityDir := cgointernal.SharedIdentityDir(t, h, "GIZCLAW_E2E_PEER_IDENTITY", "peer")
	cgointernal.AssertServerAvailable(t, identityDir)

	adminDir := cgointernal.SharedIdentityDir(t, h, "GIZCLAW_E2E_ADMIN_IDENTITY", "admin")
	h.SetContextDirAlias("admin-a", adminDir)
	admin := h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	suffix := strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
	firmwareID := strings.Repeat("f", 256-len(suffix)) + suffix
	want := cgointernal.FirmwareConfig{
		Channel: rpcpb.FirmwareChannelName_FIRMWARE_CHANNEL_NAME_STABLE,
		URL:     "https://firmware.example.invalid/devkit/maximum-id.tar.zlib",
		SHA256:  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Size:    4096,
	}
	created, err := api.CreateFirmwareWithResponse(ctx, adminhttp.FirmwareUpsert{
		Id: firmwareID,
		Slots: apitypes.FirmwareSlots{Stable: apitypes.FirmwareSlot{Package: &apitypes.FirmwarePackage{
			Url: want.URL, Sha256: want.SHA256, Size: want.Size,
		}}},
	})
	if err != nil {
		t.Fatalf("create maximum-ID Firmware: %v", err)
	}
	if created.JSON200 == nil {
		t.Fatalf("create maximum-ID Firmware status %d: %s", created.StatusCode(), strings.TrimSpace(string(created.Body)))
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = api.DeleteFirmwareWithResponse(cleanupCtx, created.JSON200.Id)
	}()

	registrationToken := createCSDKRegistrationToken(t, h, "firmware-rpc-maximum-id", &firmwareID)
	cgointernal.CSDKFirmwareRPCPackage(t, identityDir, registrationToken, want)
}

func TestCSDKFirmwareRequiresBinding(t *testing.T) {
	h := clitest.NewSetupHarness(t, "cgo-rpc-firmware-unbound")
	contextName := "cgo-firmware-unbound"
	h.CreateContext(contextName).MustSucceed(t)
	identityDir := filepath.Join(h.XDGConfigHome, "gizclaw", contextName)
	cgointernal.AssertServerAvailable(t, identityDir)
	registrationToken := createCSDKRegistrationToken(t, h, "firmware-unbound", nil)
	client, err := cgointernal.NewClient(identityDir)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Register(registrationToken); err != nil {
		t.Fatal(err)
	}

	_, err = client.GetFirmware(rpcpb.FirmwareChannelName_FIRMWARE_CHANNEL_NAME_STABLE)
	requireRPCError(t, err, rpcpb.RpcErrorCode_RPC_ERROR_CODE_NOT_FOUND, "firmware is not bound to peer")
}

func runCSDKRPC(t *testing.T, scenario string, run func(t *testing.T, identityDir string)) {
	t.Helper()
	_ = scenario
	h := clitest.NewSetupHarness(t, "cgo-rpc")
	identityDir := cgointernal.SharedIdentityDir(t, h, "GIZCLAW_E2E_PEER_IDENTITY", "peer")
	cgointernal.AssertServerAvailable(t, identityDir)
	run(t, identityDir)
}

func runRegisteredCSDKRPC(t *testing.T, scenario string, run func(t *testing.T, identityDir, registrationToken string)) {
	t.Helper()
	h := clitest.NewSetupHarness(t, "cgo-rpc")
	identityDir := cgointernal.SharedIdentityDir(t, h, "GIZCLAW_E2E_PEER_IDENTITY", "peer")
	cgointernal.AssertServerAvailable(t, identityDir)
	firmwareID := cSDKFirmwareID
	registrationToken := createCSDKRegistrationToken(t, h, scenario, &firmwareID)
	run(t, identityDir, registrationToken)
}

func createCSDKRegistrationToken(t *testing.T, h *clitest.Harness, scenario string, firmwareID *string) string {
	t.Helper()
	adminDir := cgointernal.SharedIdentityDir(t, h, "GIZCLAW_E2E_ADMIN_IDENTITY", "admin")
	h.SetContextDirAlias("admin-a", adminDir)
	admin := h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	profileName := "cgo-firmware"
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
		t.Fatalf("put C SDK RuntimeProfile: %v", err)
	}
	tokenName := "cgo-" + scenario
	if err := clitest.DeleteRegistrationTokenByID(ctx, api, tokenName); err != nil {
		t.Fatalf("retire C SDK RegistrationToken: %v", err)
	}
	if firmwareID != nil {
		firmware, found, resolveErr := clitest.FirmwareByID(ctx, api, *firmwareID)
		if resolveErr != nil || !found {
			t.Fatalf("resolve C SDK Firmware %q: found=%v err=%v", *firmwareID, found, resolveErr)
		}
		firmwareID = &firmware.Id
	}
	tokenResp, err := api.CreateRegistrationTokenWithResponse(ctx, adminhttp.RegistrationTokenUpsert{
		Id: tokenName, Token: tokenName, RuntimeProfileId: profile.Id, FirmwareId: firmwareID,
	})
	if err != nil {
		t.Fatalf("create C SDK RegistrationToken: %v", err)
	}
	if tokenResp.JSON200 == nil || tokenResp.JSON200.Token == "" {
		t.Fatalf("create C SDK RegistrationToken status %d: %s", tokenResp.StatusCode(), strings.TrimSpace(string(tokenResp.Body)))
	}
	return tokenResp.JSON200.Token
}

func requireRPCError(t *testing.T, err error, code rpcpb.RpcErrorCode, message string) {
	t.Helper()
	var rpcErr *cgointernal.RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("error = %v, want C SDK RPC error", err)
	}
	if rpcErr.Code != code || rpcErr.Message != message {
		t.Fatalf("RPC error = (%d, %q), want (%d, %q)", rpcErr.Code, rpcErr.Message, code, message)
	}
}
