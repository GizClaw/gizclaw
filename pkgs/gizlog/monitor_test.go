package gizlog

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestMonitorLogBoundsAndPeerIsolation(t *testing.T) {
	handler := &monitorHandler{}
	for range 510 {
		r := slog.NewRecord(time.Now(), slog.LevelInfo, strings.Repeat("m", 5000), 0)
		r.AddAttrs(slog.String("peer_public_key", "monitor-test-owner"))
		if err := handler.Handle(WithPeerPublicKey(context.Background(), "monitor-test-owner"), r); err != nil {
			t.Fatal(err)
		}
	}
	entries := ReadMonitorLogs("monitor-test-owner")
	if len(entries) != 500 {
		t.Fatal(len(entries))
	}
	for _, e := range entries {
		if len(e.Message) > 4096 {
			t.Fatal("unbounded message")
		}
	}
	if len(ReadMonitorLogs("monitor-test-other")) != 0 {
		t.Fatal("cross-peer disclosure")
	}
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "unscoped", 0)
	_ = handler.Handle(context.Background(), r)
	if len(ReadMonitorLogs("monitor-test-owner")) != 499 {
		t.Fatal("unscoped record returned")
	}
}
