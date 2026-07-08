//go:build gizclaw_e2e

package admin_test

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestPeerTelemetryAdminQueriesFromProtocolPath(t *testing.T) {
	h := clitest.NewHarness(t, "admin-peer-telemetry")
	h.StartServerFromFixture("server_config.yaml")
	h.InstallFixedAdminContext("admin-telemetry-admin").MustSucceed(t)
	h.CreateContext("admin-telemetry-peer").MustSucceed(t)
	peerKey := h.ContextPublicKey("admin-telemetry-peer")
	h.RegisterContext("admin-telemetry-peer", "--sn", "admin-telemetry-"+peerKey).MustSucceed(t)

	peer := h.ConnectClientFromContext("admin-telemetry-peer")
	t.Cleanup(func() { peer.Close() })

	now := time.Now().UTC()
	start := now.Add(-40 * time.Minute).Truncate(time.Second)
	for i := 0; i < 12; i++ {
		if err := peer.SendTelemetryFrame(peerTelemetryFixtureFrame(uint32(i+1), start.Add(time.Duration(i)*2*time.Minute), i)); err != nil {
			t.Fatalf("send telemetry frame %d: %v", i, err)
		}
	}

	admin := h.ConnectClientFromContext("admin-telemetry-admin")
	t.Cleanup(func() { admin.Close() })
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin API client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	waitForTelemetryLatest(t, ctx, api, peerKey, "battery.percent", 71)

	fields := "battery.percent,gnss.latitude,gnss.longitude,network.rssi_dbm,system.temperature_c"
	latest, err := api.GetPeerTelemetryLatestWithResponse(ctx, peerKey, &adminhttp.GetPeerTelemetryLatestParams{Fields: &fields})
	if err != nil {
		t.Fatalf("latest telemetry: %v", err)
	}
	if latest.JSON200 == nil {
		t.Fatalf("latest telemetry status=%d body=%s", latest.StatusCode(), strings.TrimSpace(string(latest.Body)))
	}
	requireTelemetryLatestField(t, latest.JSON200.Values, apitypes.PeerTelemetryFieldBatteryPercent, 71)
	requireTelemetryLatestField(t, latest.JSON200.Values, apitypes.PeerTelemetryFieldGnssLatitude, 37.781)
	requireTelemetryLatestField(t, latest.JSON200.Values, apitypes.PeerTelemetryFieldNetworkRssiDbm, -61)
	requireTelemetryLatestField(t, latest.JSON200.Values, apitypes.PeerTelemetryFieldSystemTemperatureC, 36.6)

	stepMs := int64((2 * time.Minute).Milliseconds())
	limit := int32(100)
	order := apitypes.PeerTelemetryOrderAsc
	ranged, err := api.QueryPeerTelemetryWithResponse(ctx, peerKey, &adminhttp.QueryPeerTelemetryParams{
		Field:       apitypes.PeerTelemetryFieldBatteryPercent,
		StartTimeMs: start.Add(-time.Minute).UnixMilli(),
		EndTimeMs:   now.Add(time.Minute).UnixMilli(),
		StepMs:      &stepMs,
		Limit:       &limit,
		Order:       &order,
	})
	if err != nil {
		t.Fatalf("range telemetry: %v", err)
	}
	if ranged.JSON200 == nil {
		t.Fatalf("range telemetry status=%d body=%s", ranged.StatusCode(), strings.TrimSpace(string(ranged.Body)))
	}
	if len(ranged.JSON200.Points) < 6 {
		t.Fatalf("range points = %d, want >= 6: %+v", len(ranged.JSON200.Points), ranged.JSON200.Points)
	}

	aggregate, err := api.AggregatePeerTelemetryWithResponse(ctx, peerKey, &adminhttp.AggregatePeerTelemetryParams{
		Field:       apitypes.PeerTelemetryFieldBatteryPercent,
		StartTimeMs: start.Add(-time.Minute).UnixMilli(),
		EndTimeMs:   now.Add(time.Minute).UnixMilli(),
		BucketMs:    int64((10 * time.Minute).Milliseconds()),
		Aggregate:   apitypes.PeerTelemetryAggregateLast,
	})
	if err != nil {
		t.Fatalf("aggregate telemetry: %v", err)
	}
	if aggregate.JSON200 == nil {
		t.Fatalf("aggregate telemetry status=%d body=%s", aggregate.StatusCode(), strings.TrimSpace(string(aggregate.Body)))
	}
	if len(aggregate.JSON200.Points) == 0 {
		t.Fatalf("aggregate returned no buckets")
	}
}

func peerTelemetryFixtureFrame(sequence uint32, at time.Time, index int) *telemetrypb.TelemetryFrame {
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

func waitForTelemetryLatest(t *testing.T, ctx context.Context, api *adminhttp.ClientWithResponses, peerKey, field string, want float64) {
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

func requireTelemetryLatestField(t *testing.T, values []apitypes.PeerTelemetryValue, field apitypes.PeerTelemetryField, want float64) {
	t.Helper()
	for _, value := range values {
		if value.Field == field {
			if math.Abs(value.Value-want) > 0.000001 {
				t.Fatalf("latest %s = %v, want %v", field, value.Value, want)
			}
			return
		}
	}
	t.Fatalf("latest missing %s in %+v", field, values)
}
