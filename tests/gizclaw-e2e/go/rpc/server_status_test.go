//go:build gizclaw_e2e

package rpc_test

import (
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestServerStatusRPC(t *testing.T) {
	env := newServerResourceHarness(t)

	empty, err := env.peer.GetServerStatus(env.ctx, "server.status.get.empty")
	if err != nil {
		t.Fatalf("server.status.get empty: %v", err)
	}
	if empty.Volume != nil || empty.BatteryPercent != nil {
		t.Fatalf("empty server.status.get = %#v", empty)
	}

	volume := 42
	battery := 87
	charging := true
	muted := false
	labels := map[string]string{"mode": "rpc-e2e"}
	reportedAt := time.Now().UTC().Truncate(time.Second)
	status := rpcapi.ServerPutStatusRequest{
		Volume:         &volume,
		BatteryPercent: &battery,
		Charging:       &charging,
		Muted:          &muted,
		Labels:         &labels,
		ReportedAt:     &reportedAt,
	}
	updated, err := env.peer.PutServerStatus(env.ctx, "server.status.put", status)
	if err != nil {
		t.Fatalf("server.status.put: %v", err)
	}
	assertPeerStatus(t, updated, status)

	got, err := env.peer.GetServerStatus(env.ctx, "server.status.get.updated")
	if err != nil {
		t.Fatalf("server.status.get updated: %v", err)
	}
	assertPeerStatus(t, got, status)

	badVolume := 101
	if _, err := env.peer.PutServerStatus(env.ctx, "server.status.put.invalid", rpcapi.ServerPutStatusRequest{Volume: &badVolume}); err == nil || !strings.Contains(err.Error(), "volume") {
		t.Fatalf("server.status.put invalid volume error = %v", err)
	}
}

func assertPeerStatus(t *testing.T, got *rpcapi.PeerStatus, want rpcapi.PeerStatus) {
	t.Helper()

	if got == nil {
		t.Fatal("peer status is nil")
	}
	if got.Volume == nil || want.Volume == nil || *got.Volume != *want.Volume {
		t.Fatalf("volume = %#v, want %#v", got.Volume, want.Volume)
	}
	if got.BatteryPercent == nil || want.BatteryPercent == nil || *got.BatteryPercent != *want.BatteryPercent {
		t.Fatalf("battery = %#v, want %#v", got.BatteryPercent, want.BatteryPercent)
	}
	if got.Charging == nil || want.Charging == nil || *got.Charging != *want.Charging {
		t.Fatalf("charging = %#v, want %#v", got.Charging, want.Charging)
	}
	if got.Muted == nil || want.Muted == nil || *got.Muted != *want.Muted {
		t.Fatalf("muted = %#v, want %#v", got.Muted, want.Muted)
	}
	if got.Labels == nil || want.Labels == nil || (*got.Labels)["mode"] != (*want.Labels)["mode"] {
		t.Fatalf("labels = %#v, want %#v", got.Labels, want.Labels)
	}
	if got.ReportedAt == nil || want.ReportedAt == nil || !got.ReportedAt.Equal(*want.ReportedAt) {
		t.Fatalf("reported_at = %#v, want %#v", got.ReportedAt, want.ReportedAt)
	}
}
