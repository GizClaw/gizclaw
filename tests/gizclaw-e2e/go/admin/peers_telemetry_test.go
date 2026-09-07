//go:build gizclaw_e2e

package admin_test

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
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

	// The cellular identity is exposed through PeerStatus rather than the
	// metric query enum, and the latest observation wins.
	latestIMSI := peerTelemetryFixtureIMSI(11)
	status := waitForPeerStatusNetworkIdentity(t, ctx, peer, peerTelemetryFixtureIMEI, latestIMSI)
	latestAt := start.Add(11 * 2 * time.Minute)
	requireTelemetryStatusFieldTime(t, status, "network_imei_at_unix_ms", latestAt)
	requireTelemetryStatusFieldTime(t, status, "network_imsi_at_unix_ms", latestAt)

	// An older observation never overwrites the newer stored identity, and a
	// rejected frame (identity on a Wi-Fi route) leaves no partial write.
	staleIMSI := "460009999999999"
	rat := "lte"
	wifi := "wifi"
	staleAt := start.Add(-time.Hour)
	if err := peer.SendTelemetryFrame(&telemetrypb.TelemetryFrame{
		Sequence:         100,
		ObservedAtUnixMs: staleAt.UnixMilli(),
		Observations: []*telemetrypb.Observation{{
			Body: &telemetrypb.Observation_Network{Network: &telemetrypb.NetworkObservation{Rat: &rat, Imsi: &staleIMSI}},
		}},
	}); err != nil {
		t.Fatalf("send stale telemetry frame: %v", err)
	}
	wifiIMEI := "111111111111111"
	if err := peer.SendTelemetryFrame(&telemetrypb.TelemetryFrame{
		Sequence:         101,
		ObservedAtUnixMs: now.UnixMilli(),
		Observations: []*telemetrypb.Observation{{
			Body: &telemetrypb.Observation_Network{Network: &telemetrypb.NetworkObservation{Rat: &wifi, Imei: &wifiIMEI}},
		}},
	}); err != nil {
		t.Fatalf("send wifi telemetry frame: %v", err)
	}
	// A later battery frame proves the stale and rejected frames were processed.
	marker := 99.0
	markerAt := now.Add(time.Second)
	if err := peer.SendTelemetryFrame(&telemetrypb.TelemetryFrame{
		Sequence:         102,
		ObservedAtUnixMs: markerAt.UnixMilli(),
		Observations: []*telemetrypb.Observation{{
			Body: &telemetrypb.Observation_Battery{Battery: &telemetrypb.BatteryObservation{Percent: &marker}},
		}},
	}); err != nil {
		t.Fatalf("send marker telemetry frame: %v", err)
	}
	waitForTelemetryLatest(t, ctx, api, peerKey, "battery.percent", 99)
	status, err = peer.GetServerStatus(ctx, "status-after-stale")
	if err != nil {
		t.Fatalf("server.status.get: %v", err)
	}
	if status.NetworkImsi == nil || *status.NetworkImsi != latestIMSI || status.NetworkImei == nil || *status.NetworkImei != peerTelemetryFixtureIMEI {
		t.Fatalf("stale or rejected frames changed the stored identity: imei set=%t imsi set=%t", status.NetworkImei != nil, status.NetworkImsi != nil)
	}
	requireTelemetryStatusFieldTime(t, status, "network_imsi_at_unix_ms", latestAt)
}

const peerTelemetryFixtureIMEI = "490154203237518"

func peerTelemetryFixtureIMSI(index int) string {
	return fmt.Sprintf("46000123456%04d", index)
}

func waitForPeerStatusNetworkIdentity(t *testing.T, ctx context.Context, peer *gizcli.Client, imei, imsi string) *rpcapi.ServerGetStatusResponse {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		status, err := peer.GetServerStatus(ctx, "status-network-identity")
		if err == nil && status != nil && status.NetworkImei != nil && *status.NetworkImei == imei &&
			status.NetworkImsi != nil && *status.NetworkImsi == imsi {
			return status
		}
		if time.Now().After(deadline) {
			if err != nil {
				t.Fatalf("server.status.get did not expose the network identity: %v", err)
			}
			t.Fatalf("server.status.get did not expose the network identity: imei set=%t imsi set=%t",
				status != nil && status.NetworkImei != nil, status != nil && status.NetworkImsi != nil)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func requireTelemetryStatusFieldTime(t *testing.T, status *rpcapi.ServerGetStatusResponse, key string, want time.Time) {
	t.Helper()
	if status.Details == nil {
		t.Fatalf("status details missing for %s", key)
	}
	fields, _ := (*status.Details)["telemetry_status"].(map[string]any)
	raw, ok := fields[key]
	if !ok {
		t.Fatalf("details.telemetry_status missing %s: %v", key, fields)
	}
	var unixMS int64
	switch v := raw.(type) {
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			t.Fatalf("details.telemetry_status.%s = %q is not unix ms", key, v)
		}
		unixMS = parsed
	case float64:
		unixMS = int64(v)
	default:
		t.Fatalf("details.telemetry_status.%s = %#v", key, raw)
	}
	if unixMS != want.UnixMilli() {
		t.Fatalf("details.telemetry_status.%s = %d, want %d", key, unixMS, want.UnixMilli())
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
	rat := "lte"
	imei := peerTelemetryFixtureIMEI
	imsi := peerTelemetryFixtureIMSI(index)
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
					Rat:         &rat,
					Imei:        &imei,
					Imsi:        &imsi,
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
