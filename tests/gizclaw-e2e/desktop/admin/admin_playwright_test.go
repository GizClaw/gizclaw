//go:build gizclaw_e2e

package admin

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
	desktop "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/desktop"
)

func TestDesktopAdminPlaywright(t *testing.T) {
	h := desktop.NewHarness(t)
	h.RunForShell(t, h.FrontendDir(), "npx", "playwright", "test", "e2e/admin.spec.ts")
}

func TestDesktopAdminTelemetryPlaywright(t *testing.T) {
	server := clitest.NewHarness(t, "desktop-admin-telemetry")
	server.StartServerFromFixture("server_config.yaml")
	adminContext := "desktop-admin-telemetry-admin"
	peerContext := "desktop-admin-telemetry-peer"
	server.InstallFixedAdminContext(adminContext).MustSucceed(t)
	server.CreateContext(peerContext).MustSucceed(t)
	peerKey := server.ContextPublicKey(peerContext)
	server.RegisterContext(peerContext, "--sn", "desktop-admin-telemetry-"+peerKey).MustSucceed(t)

	peer := server.ConnectClientFromContext(peerContext)
	t.Cleanup(func() { peer.Close() })
	now := time.Now().UTC()
	start := now.Add(-40 * time.Minute).Truncate(time.Second)
	for i := 0; i < 12; i++ {
		if err := peer.SendTelemetryFrame(telemetryFixtureFrame(uint32(i+1), start.Add(time.Duration(i)*2*time.Minute), i)); err != nil {
			t.Fatalf("send telemetry frame %d: %v", i, err)
		}
	}

	adminClient := server.ConnectClientFromContext(adminContext)
	t.Cleanup(func() { adminClient.Close() })
	api, err := adminClient.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin API client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	waitForLatestValue(t, ctx, api, peerKey, "battery.percent", 71)

	adminKeyPair := server.ContextKeyPair(adminContext)
	privateKeyBase64 := base64.StdEncoding.EncodeToString(adminKeyPair.Private[:])
	screenshotPath := filepath.Join(server.RepoRoot, "apps", "wails", "frontend", "test-results", "admin-telemetry", "peer-telemetry-tab.png")
	if err := os.MkdirAll(filepath.Dir(screenshotPath), 0o755); err != nil {
		t.Fatalf("create screenshot dir: %v", err)
	}

	t.Setenv("GIZCLAW_E2E_ADMIN_TELEMETRY_CONTEXT_NAME", adminContext)
	t.Setenv("GIZCLAW_E2E_ADMIN_TELEMETRY_ENDPOINT", server.ServerAddr)
	t.Setenv("GIZCLAW_E2E_ADMIN_TELEMETRY_PUBLIC_KEY", server.ContextPublicKey(adminContext))
	t.Setenv("GIZCLAW_E2E_ADMIN_TELEMETRY_PRIVATE_KEY_BASE64", privateKeyBase64)
	t.Setenv("GIZCLAW_E2E_ADMIN_TELEMETRY_PEER_PUBLIC_KEY", peerKey)
	t.Setenv("GIZCLAW_E2E_ADMIN_TELEMETRY_SCREENSHOT", screenshotPath)

	frontend := desktop.NewHarness(t)
	frontend.RunForShell(t, frontend.FrontendDir(), "npx", "playwright", "test", "e2e/admin-telemetry-real.spec.ts")
	if _, err := os.Stat(screenshotPath); err != nil {
		t.Fatalf("telemetry screenshot missing at %s: %v", screenshotPath, err)
	}
	t.Logf("admin telemetry screenshot: %s", screenshotPath)
}

func telemetryFixtureFrame(sequence uint32, at time.Time, index int) *telemetrypb.TelemetryFrame {
	percent := 60 + float64(index)
	charging := index%2 == 0
	voltage := 3700 + float64(index*7)
	altitude := 12 + float64(index)
	accuracy := 3.5 + float64(index)/10
	rssi := -72 + float64(index)
	signal := 2 + float64(index%4)
	connected := true
	uptime := 3600 + float64(index*120)
	freeMemory := 64*1024*1024 - float64(index*128*1024)
	temperature := 35.5 + float64(index)/10
	return &telemetrypb.TelemetryFrame{
		Sequence:         sequence,
		ObservedAtUnixMs: at.UnixMilli(),
		Observations: []*telemetrypb.Observation{
			{
				Body: &telemetrypb.Observation_Battery{Battery: &telemetrypb.BatteryObservation{
					Percent:   &percent,
					Charging:  &charging,
					VoltageMv: &voltage,
				}},
			},
			{
				Body: &telemetrypb.Observation_Gnss{Gnss: &telemetrypb.GnssObservation{
					Latitude:  37.77 + float64(index)/1000,
					Longitude: -122.42 + float64(index)/1000,
					AltitudeM: &altitude,
					AccuracyM: &accuracy,
				}},
			},
			{
				Body: &telemetrypb.Observation_Network{Network: &telemetrypb.NetworkObservation{
					RssiDbm:     &rssi,
					SignalLevel: &signal,
					Connected:   &connected,
				}},
			},
			{
				Body: &telemetrypb.Observation_System{System: &telemetrypb.SystemObservation{
					UptimeSeconds:   &uptime,
					FreeMemoryBytes: &freeMemory,
					TemperatureC:    &temperature,
				}},
			},
		},
	}
}

func waitForLatestValue(t *testing.T, ctx context.Context, api *adminhttp.ClientWithResponses, peerKey, field string, want float64) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		fields := field
		resp, err := api.GetPeerTelemetryLatestWithResponse(ctx, peerKey, &adminhttp.GetPeerTelemetryLatestParams{Fields: &fields})
		if err == nil && resp.JSON200 != nil {
			for _, value := range resp.JSON200.Values {
				if string(value.Field) == field && value.Value == want {
					return
				}
			}
		}
		if time.Now().After(deadline) {
			if err != nil {
				t.Fatalf("latest telemetry did not become ready: %v", err)
			}
			if resp == nil {
				t.Fatalf("latest telemetry did not become ready: nil response")
			}
			t.Fatalf("latest telemetry did not become ready status=%d body=%s", resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
		}
		time.Sleep(100 * time.Millisecond)
	}
}
