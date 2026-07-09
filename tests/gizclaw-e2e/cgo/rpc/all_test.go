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
