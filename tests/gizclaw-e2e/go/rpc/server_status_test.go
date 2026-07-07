//go:build gizclaw_e2e

package rpc_test

import (
	"testing"
	"time"
)

func TestServerStatusRPC(t *testing.T) {
	env := newServerResourceHarness(t)

	battery := 87
	charging := true
	if err := env.peer.SendBatteryTelemetry(battery, charging); err != nil {
		t.Fatalf("send battery telemetry: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, err := env.peer.GetServerStatus(env.ctx, "server.status.get.telemetry")
		if err != nil {
			t.Fatalf("server.status.get telemetry: %v", err)
		}
		if got.BatteryPercent != nil && *got.BatteryPercent == battery &&
			got.Charging != nil && *got.Charging == charging {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("server.status.get did not reflect telemetry: %#v", got)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
