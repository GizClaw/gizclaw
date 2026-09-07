//go:build gizclaw_e2e

package telemetry_test

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	cgointernal "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cgo/internal"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestCSDKTelemetryPersistsForAdminQueries(t *testing.T) {
	h := clitest.NewSetupHarness(t, "cgo-telemetry")
	identityDir := cgointernal.SharedIdentityDir(t, h, "GIZCLAW_E2E_PEER_IDENTITY", "peer")
	cgointernal.AssertServerAvailable(t, identityDir)
	h.SetContextDirAlias("cgo-telemetry-peer", identityDir)

	client, err := cgointernal.NewClient(identityDir)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.SendFullTelemetry(); err != nil {
		t.Fatal(err)
	}

	adminDir := cgointernal.SharedIdentityDir(t, h, "GIZCLAW_E2E_ADMIN_IDENTITY", "admin")
	h.SetContextDirAlias("cgo-telemetry-admin", adminDir)
	admin := h.ConnectClientFromContext("cgo-telemetry-admin")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	peerKey := h.ContextPublicKey("cgo-telemetry-peer")
	fields := "battery.percent,gnss.latitude,network.rssi_dbm,system.temperature_c"
	var latest *adminhttp.GetPeerTelemetryLatestResponse
	deadline := time.Now().Add(10 * time.Second)
	for {
		latest, err = api.GetPeerTelemetryLatestWithResponse(
			ctx,
			peerKey,
			&adminhttp.GetPeerTelemetryLatestParams{Fields: &fields},
		)
		if err == nil && latest.JSON200 != nil &&
			hasTelemetryValue(latest.JSON200.Values, apitypes.PeerTelemetryFieldBatteryPercent, 91) {
			break
		}
		if time.Now().After(deadline) {
			if err != nil {
				t.Fatalf("latest C SDK telemetry: %v", err)
			}
			if latest == nil {
				t.Fatal("latest C SDK telemetry returned nil response")
			}
			t.Fatalf(
				"latest C SDK telemetry status=%d body=%s",
				latest.StatusCode(),
				strings.TrimSpace(string(latest.Body)),
			)
		}
		time.Sleep(100 * time.Millisecond)
	}

	requireTelemetryValue(t, latest.JSON200.Values, apitypes.PeerTelemetryFieldBatteryPercent, 91)
	requireTelemetryValue(t, latest.JSON200.Values, apitypes.PeerTelemetryFieldGnssLatitude, 31.2304)
	requireTelemetryValue(t, latest.JSON200.Values, apitypes.PeerTelemetryFieldNetworkRssiDbm, -67)
	requireTelemetryValue(t, latest.JSON200.Values, apitypes.PeerTelemetryFieldSystemTemperatureC, 36.5)

	// The cellular identity is not a metric; it rides on the owner-scoped
	// PeerStatus that server.status.get returns, with per-field observation
	// timestamps under details.telemetry_status.
	var status rpcpb.ServerGetStatusResponse
	statusDeadline := time.Now().Add(10 * time.Second)
	for {
		if err := client.CallRPC(rpcpb.RpcMethod_RPC_METHOD_SERVER_STATUS_GET, &rpcpb.ServerGetStatusRequest{}, &status); err != nil {
			t.Fatalf("server.status.get: %v", err)
		}
		if status.GetValue().GetNetworkImei() == cgointernal.FullTelemetryNetworkIMEI &&
			status.GetValue().GetNetworkImsi() == cgointernal.FullTelemetryNetworkIMSI {
			break
		}
		if time.Now().After(statusDeadline) {
			t.Fatalf("server.status.get did not expose the C SDK network identity: network_imei set=%t network_imsi set=%t",
				status.GetValue().NetworkImei != nil, status.GetValue().NetworkImsi != nil)
		}
		time.Sleep(100 * time.Millisecond)
	}
	telemetryStatus := status.GetValue().GetDetails().GetFields()["telemetry_status"].GetStructValue().GetFields()
	for _, key := range []string{"network_imei_at_unix_ms", "network_imsi_at_unix_ms"} {
		if _, ok := telemetryStatus[key]; !ok {
			t.Fatalf("server.status.get details.telemetry_status missing %s: %v", key, telemetryStatus)
		}
	}
}

func hasTelemetryValue(values []apitypes.PeerTelemetryValue, field apitypes.PeerTelemetryField, want float64) bool {
	for _, value := range values {
		if value.Field == field && math.Abs(value.Value-want) < 0.000001 {
			return true
		}
	}
	return false
}

func requireTelemetryValue(t *testing.T, values []apitypes.PeerTelemetryValue, field apitypes.PeerTelemetryField, want float64) {
	t.Helper()
	for _, value := range values {
		if value.Field != field {
			continue
		}
		if math.Abs(value.Value-want) >= 0.000001 {
			t.Fatalf("latest %s = %v, want %v", field, value.Value, want)
		}
		return
	}
	t.Fatalf("latest C SDK telemetry missing %s in %+v", field, values)
}
