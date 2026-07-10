//go:build gizclaw_e2e

package rpc_test

import (
	"testing"

	cgointernal "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cgo/internal"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

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
	runCSDKRPC(t, "firmware-rpc", cgointernal.CSDKFirmwareRPC)
}

func TestCSDKFirmwareDownload(t *testing.T) {
	runCSDKRPC(t, "firmware-download", cgointernal.CSDKFirmwareDownload)
}

func runCSDKRPC(t *testing.T, scenario string, run func(t *testing.T, identityDir string)) {
	t.Helper()
	_ = scenario
	h := clitest.NewSetupHarness(t, "cgo-rpc")
	identityDir := cgointernal.SharedIdentityDir(t, h, "GIZCLAW_E2E_PEER_IDENTITY", "peer")
	cgointernal.AssertServerAvailable(t, identityDir)
	run(t, identityDir)
}
