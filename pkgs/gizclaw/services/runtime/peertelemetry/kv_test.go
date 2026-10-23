package peertelemetry

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/peerruntest"
	"google.golang.org/protobuf/proto"
)

func TestKVTelemetryPacketAndFrameAtomicity(t *testing.T) {
	store := peerruntest.New(t)
	peer := testPublicKey(t)
	metrics := &fakeMetricsStore{}
	now := time.Unix(30, 0).UTC()
	service := &Service{Status: StatusSync{Store: store}, Metrics: metrics, Now: func() time.Time { return now }}
	frame := &telemetrypb.TelemetryFrame{ObservedAtUnixMs: 10000, Observations: []*telemetrypb.Observation{{ObservedAtDeltaMs: 1, Body: &telemetrypb.Observation_Kv{Kv: &telemetrypb.KVObservation{Key: "snapshot", JsonValue: new(`{"a":[null,false,0]}`)}}}}}
	if err := service.ReportPacket(t.Context(), peer, marshalFrame(t, frame)); err != nil {
		t.Fatal(err)
	}
	status, err := store.GetStatus(t.Context(), peer)
	if err != nil || status.ReportedKv == nil {
		t.Fatal(err)
	}
	entry := (*status.ReportedKv)["snapshot"]
	if entry.ObservedAt.UnixMilli() != 10001 || string(entry.Value) != `{"a":[null,false,0]}` || len(metrics.samples) != 0 {
		t.Fatal("KV projection changed value, time or metrics")
	}
	before, _ := json.Marshal(status)
	invalid := proto.Clone(frame).(*telemetrypb.TelemetryFrame)
	invalid.ObservedAtUnixMs = 20000
	invalid.Observations = append(invalid.Observations, &telemetrypb.Observation{Body: &telemetrypb.Observation_Kv{Kv: &telemetrypb.KVObservation{Key: "bad", JsonValue: new(`{`)}}})
	if err := service.Report(t.Context(), peer, invalid); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("invalid frame: %v", err)
	}
	after, _ := store.GetStatus(t.Context(), peer)
	encoded, _ := json.Marshal(after)
	if string(encoded) != string(before) {
		t.Fatal("invalid frame partially committed")
	}
	invalid = proto.Clone(frame).(*telemetrypb.TelemetryFrame)
	invalid.ObservedAtUnixMs = now.Add(6 * time.Minute).UnixMilli()
	if err := service.Report(t.Context(), peer, invalid); !errors.Is(err, ErrInvalidFrame) {
		t.Fatal("future timestamp accepted")
	}
}

func TestKVRejectsReceiveTimeReplays(t *testing.T) {
	store := peerruntest.New(t)
	peer := testPublicKey(t)
	service := &Service{Status: StatusSync{Store: store}}
	frame := &telemetrypb.TelemetryFrame{Observations: []*telemetrypb.Observation{{Body: &telemetrypb.Observation_Kv{Kv: &telemetrypb.KVObservation{Key: "x", JsonValue: new("false")}}}}}
	if err := service.Report(t.Context(), peer, frame); !errors.Is(err, ErrInvalidFrame) {
		t.Fatal("KV without stable observation time was accepted")
	}
	state, err := store.GetReportedKV(t.Context(), peer)
	if err != nil || state != nil {
		t.Fatal("invalid observation wrote state")
	}
}

func TestKVFrameBudgets(t *testing.T) {
	for _, tc := range []struct {
		name  string
		kv    *telemetrypb.KVObservation
		valid bool
	}{
		{"null", &telemetrypb.KVObservation{Key: "k", JsonValue: new("null")}, true},
		{"false", &telemetrypb.KVObservation{Key: "k", JsonValue: new("false")}, true},
		{"zero", &telemetrypb.KVObservation{Key: "k", JsonValue: new("0")}, true},
		{"delete", &telemetrypb.KVObservation{Key: "k", Delete: new(true)}, true},
		{"key boundary", &telemetrypb.KVObservation{Key: strings.Repeat("x", 64), JsonValue: new("0")}, true},
		{"key bytes", &telemetrypb.KVObservation{Key: strings.Repeat("中", 22), JsonValue: new("0")}, false},
		{"value boundary", &telemetrypb.KVObservation{Key: "k", JsonValue: new(`"` + strings.Repeat("x", 1022) + `"`)}, true},
		{"value oversized", &telemetrypb.KVObservation{Key: "k", JsonValue: new(`"` + strings.Repeat("x", 1023) + `"`)}, false},
		{"malformed", &telemetrypb.KVObservation{Key: "k", JsonValue: new(`{"x":]}`)}, false},
		{"missing", &telemetrypb.KVObservation{Key: "k"}, false},
		{"false delete", &telemetrypb.KVObservation{Key: "k", Delete: new(false)}, false},
		{"both", &telemetrypb.KVObservation{Key: "k", Delete: new(true), JsonValue: new("null")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := telemetrypb.ValidateKV(tc.kv); (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
		})
	}
	frame := &telemetrypb.TelemetryFrame{}
	for range 16 {
		frame.Observations = append(frame.Observations, &telemetrypb.Observation{Body: &telemetrypb.Observation_Kv{Kv: &telemetrypb.KVObservation{Key: "k", JsonValue: new("0")}}})
	}
	if err := telemetrypb.ValidateKVFrame(frame); err != nil {
		t.Fatal(err)
	}
	frame.Observations = append(frame.Observations, frame.Observations[0])
	if err := telemetrypb.ValidateKVFrame(frame); err == nil {
		t.Fatal("accepted 17 entries")
	}
	frame.Observations = frame.Observations[:4]
	for _, obs := range frame.Observations {
		obs.GetKv().JsonValue = new(`"` + strings.Repeat("x", 1022) + `"`)
	}
	if err := telemetrypb.ValidateKVFrame(frame); err == nil {
		t.Fatal("accepted oversized complete frame")
	}
}

func TestKVExactEncodedFrameBoundary(t *testing.T) {
	frame := &telemetrypb.TelemetryFrame{}
	for i := range 4 {
		count := 1022
		if i == 3 {
			count = 510
		}
		frame.Observations = append(frame.Observations, &telemetrypb.Observation{Body: &telemetrypb.Observation_Kv{Kv: &telemetrypb.KVObservation{Key: "k", JsonValue: new(`"` + strings.Repeat("x", count) + `"`)}}})
	}
	last := frame.Observations[3].GetKv()
	delta := telemetrypb.MaxKVFrameBytes - proto.Size(frame)
	last.JsonValue = new(`"` + strings.Repeat("x", 510+delta) + `"`)
	if proto.Size(frame) != 4096 {
		t.Fatal("fixture is not exactly 4096 bytes")
	}
	if err := telemetrypb.ValidateKVFrame(frame); err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(marshalFrame(t, frame)); err != nil {
		t.Fatal(err)
	}
	*last.JsonValue += " "
	if err := telemetrypb.ValidateKVFrame(frame); err == nil {
		t.Fatal("4097-byte frame accepted")
	}
	if _, err := Decode(marshalFrame(t, frame)); err == nil {
		t.Fatal("4097-byte wire frame accepted")
	}
}
