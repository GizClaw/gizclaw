package peertelemetry

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
	"log/slog"
	"math"
	"strings"
	"testing"
	"time"
)

func TestOTAPacketIngestion(t *testing.T) {
	// Protoc fixture also asserted by the C and JavaScript packet encoders.
	payload, err := hex.DecodeString("080710d2091a3908feffffffffffffffff01722c080412056f74612d311a03322e302100000000000000002a0a455f444f574e4c4f4144320774696d656f7574")
	if err != nil {
		t.Fatal(err)
	}
	frame, err := Decode(payload)
	if err != nil {
		t.Fatal(err)
	}
	ota := frame.Observations[0].GetOta()
	if ota == nil || ota.DownloadPercent == nil || *ota.DownloadPercent != 0 || ota.GetErrorMessage() != "timeout" {
		t.Fatalf("OTA fields: %v", ota)
	}
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	peer := testPublicKey(t)
	store := &fakeMetricsStore{}
	service := &Service{Metrics: store}
	if err := service.ReportPacket(context.Background(), peer, payload); err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{"ota_state": "OTA_STATE_FAILED", "update_id": "ota-1", "target_version": "2.0", "download_percent": float64(0), "error_code": "E_DOWNLOAD", "error_message": "timeout", "observed_at_unix_ms": float64(1232), "sequence": float64(7), "peer_public_key": peer.String(), "level": "WARN"} {
		if record[key] != want {
			t.Errorf("%s = %v, want %v", key, record[key], want)
		}
	}
	if len(store.samples) != 0 {
		t.Fatal("OTA created metric series")
	}
	output.Reset()
	frame.Observations = append(frame.Observations, &telemetrypb.Observation{})
	if err := service.Report(context.Background(), peer, frame); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("invalid mixed frame: %v", err)
	}
	if output.Len() != 0 {
		t.Fatal("invalid mixed frame logged partial OTA")
	}
}

func TestOTAValidation(t *testing.T) {
	for _, state := range []telemetrypb.OtaState{1, 2, 3, 4} {
		obs := &telemetrypb.OtaObservation{State: state, UpdateId: "attempt"}
		if state == telemetrypb.OtaState_OTA_STATE_DOWNLOADING {
			obs.DownloadPercent = new(100.0)
		}
		if err := validateOTA(obs); err != nil {
			t.Fatalf("state %v: %v", state, err)
		}
	}
	cases := []*telemetrypb.OtaObservation{
		nil, {}, {State: 99, UpdateId: "attempt"}, {State: 1},
		{State: 1, UpdateId: strings.Repeat("a", 129)},
		{State: 1, UpdateId: "attempt", TargetVersion: new(strings.Repeat("a", 129))},
		{State: 2, UpdateId: "attempt"},
		{State: 2, UpdateId: "attempt", DownloadPercent: new(-1.0)},
		{State: 2, UpdateId: "attempt", DownloadPercent: new(101.0)},
		{State: 2, UpdateId: "attempt", DownloadPercent: new(math.NaN())},
		{State: 2, UpdateId: "attempt", DownloadPercent: new(math.Inf(1))},
		{State: 1, UpdateId: "attempt", ErrorCode: new("unexpected")},
		{State: 4, UpdateId: "attempt", ErrorMessage: new(strings.Repeat("a", 513))},
	}
	for i, obs := range cases {
		frame := &telemetrypb.TelemetryFrame{Observations: []*telemetrypb.Observation{{Body: &telemetrypb.Observation_Ota{Ota: obs}}}}
		if _, _, err := MapFrame(testPublicKey(t), frame, time.Now()); !errors.Is(err, ErrInvalidFrame) {
			t.Errorf("case %d: %v", i, err)
		}
	}
}
