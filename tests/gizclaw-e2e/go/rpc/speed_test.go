//go:build gizclaw_e2e

package rpc_test

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestRPCSpeedDirections(t *testing.T) {
	h := clitest.NewSetupHarness(t, "client-rpc-speed")
	aliasSetupAdminContext(t, h)
	registerSetupPeer(t, h, "peer-speed", "peer-speed-sn", true)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	peer := h.ConnectClientFromContext("peer-speed")
	t.Cleanup(func() { peer.Close() })
	const payloadBytes = 32 * 1024 * 1024

	for _, tc := range []struct {
		name      string
		upBytes   int64
		downBytes int64
	}{
		{name: "upload", upBytes: payloadBytes},
		{name: "download", downBytes: payloadBytes},
		{name: "bidirectional", upBytes: payloadBytes, downBytes: payloadBytes},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := peer.SpeedTest(
				ctx,
				"all.speed_test.run."+tc.name,
				rpcapi.SpeedTestRequest{
					UpContentLength:   tc.upBytes,
					DownContentLength: tc.downBytes,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if result.UpBytes != tc.upBytes || result.DownBytes != tc.downBytes {
				t.Fatalf("speed bytes = %d/%d, want %d/%d",
					result.UpBytes, result.DownBytes, tc.upBytes, tc.downBytes)
			}
			if result.Duration <= 0 {
				t.Fatalf("total duration = %v, want positive", result.Duration)
			}
			if tc.upBytes > 0 && (result.UpDuration <= 0 || result.UpMbps() <= 0) {
				t.Fatalf("upload measurement = %v/%.2f Mbps, want positive",
					result.UpDuration, result.UpMbps())
			}
			if tc.upBytes == 0 && (result.UpDuration != 0 || result.UpMbps() != 0) {
				t.Fatalf("upload measurement = %v/%.2f Mbps, want zero",
					result.UpDuration, result.UpMbps())
			}
			if tc.downBytes > 0 && (result.DownDuration <= 0 || result.DownMbps() <= 0) {
				t.Fatalf("download measurement = %v/%.2f Mbps, want positive",
					result.DownDuration, result.DownMbps())
			}
			if tc.downBytes == 0 && (result.DownDuration != 0 || result.DownMbps() != 0) {
				t.Fatalf("download measurement = %v/%.2f Mbps, want zero",
					result.DownDuration, result.DownMbps())
			}
		})
	}
}
