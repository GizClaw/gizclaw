package gizlog

import (
	"context"
	"log/slog"
	"strconv"
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

func TestMonitorLogKeepsBoundedStructuredFields(t *testing.T) {
	handler := &monitorHandler{}
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "gizclaw: request completed", 0)
	record.AddAttrs(
		slog.String("request_id", "req-1"),
		slog.String("operation", "getPeerRuntime"),
		slog.Int("status", 200),
		slog.Int64("duration_ms", 12),
		slog.String("peer_public_key", "spoofed"),
		slog.String("empty", ""),
		slog.String(strings.Repeat("k", monitorMaxFieldKey+1), "dropped"),
		slog.String("stream_id", strings.Repeat("s", monitorMaxFieldLen+10)),
	)
	for index := range monitorMaxFields + 5 {
		record.AddAttrs(slog.Int("extra"+strconv.Itoa(index), index+1))
	}
	if err := handler.Handle(WithPeerPublicKey(context.Background(), "owner"), record); err != nil {
		t.Fatal(err)
	}
	entries := ReadMonitorLogs("owner")
	entry := entries[len(entries)-1]
	if entry.Fields["request_id"] != "req-1" || entry.Fields["operation"] != "getPeerRuntime" {
		t.Fatalf("missing trace fields: %v", entry.Fields)
	}
	if entry.Fields["status"] != "200" || entry.Fields["duration_ms"] != "12" {
		t.Fatalf("scalar attributes not rendered: %v", entry.Fields)
	}
	if entry.PeerPublicKey != "owner" {
		t.Fatalf("identity came from attributes: %q", entry.PeerPublicKey)
	}
	if _, ok := entry.Fields["empty"]; ok {
		t.Fatal("empty value retained")
	}
	if _, ok := entry.Fields[strings.Repeat("k", monitorMaxFieldKey+1)]; ok {
		t.Fatal("unbounded key retained")
	}
	if len(entry.Fields["stream_id"]) != monitorMaxFieldLen {
		t.Fatalf("unbounded value: %d", len(entry.Fields["stream_id"]))
	}
	if len(entry.Fields) > monitorMaxFields {
		t.Fatalf("unbounded field count: %d", len(entry.Fields))
	}
	entry.Fields["request_id"] = "mutated"
	if ReadMonitorLogs("owner")[len(entries)-1].Fields["request_id"] != "req-1" {
		t.Fatal("retained record is aliased to the caller")
	}
}
