package main

import (
	"slices"
	"testing"

	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
	"google.golang.org/protobuf/proto"
)

func TestDriverRunsTelemetrySteps(t *testing.T) {
	if !slices.Contains(driver{}.Operations(), "telemetry") {
		t.Fatalf("Operations() = %v, want telemetry", driver{}.Operations())
	}
}

func TestDecodeTelemetryFrameRejectsEmptyFrames(t *testing.T) {
	if _, err := decodeTelemetryFrame(map[string]any{"observations": []any{}}); err == nil {
		t.Fatal("decode empty observations succeeded")
	}
	if _, err := decodeTelemetryFrame(map[string]any{"observations": []any{map[string]any{}}}); err == nil {
		t.Fatal("decode body-less observation succeeded")
	}
}

// TestEncodeTelemetryMatchesProtoJSON checks that the protojson frame a
// document supplies survives the mapping onto the C SDK structs: the SDK's
// own encoders produce bytes that decode back to the same observations.
func TestEncodeTelemetryMatchesProtoJSON(t *testing.T) {
	frame, err := decodeTelemetryFrame(map[string]any{
		"sequence":            7,
		"observed_at_unix_ms": "1800000000000",
		"observations": []any{
			map[string]any{"activity": map[string]any{"activity": "chat", "detail": "bedtime story"}},
			map[string]any{"system": map[string]any{"firmware_version": ""}},
			map[string]any{"network": map[string]any{"rat": "lte", "rssi_dbm": -90, "signal_level": 3}},
			map[string]any{"battery": map[string]any{"percent": 61}, "observed_at_delta_ms": -2000},
			map[string]any{"gnss": map[string]any{"latitude": 1.5, "longitude": -2.5, "accuracy_m": 4}},
			map[string]any{"audioplayer": map[string]any{
				"state": "error", "repeat": "all", "playlist_length": 1, "playlist_revision": 1,
				"current_index": 0, "position_ms": "12000", "error_code": "FETCH_FAILED",
				"error_message": "audio unavailable",
			}},
			map[string]any{"ota": map[string]any{
				"state": "OTA_STATE_DOWNLOADING", "update_id": "attempt-1", "download_percent": 50,
			}, "observed_at_delta_ms": -10},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	mainPayload, otaPayloads, err := encodeTelemetry(frame)
	if err != nil {
		t.Fatal(err)
	}
	gotMain := new(telemetrypb.TelemetryFrame)
	if err := proto.Unmarshal(mainPayload, gotMain); err != nil {
		t.Fatal(err)
	}
	wantMain := proto.Clone(frame).(*telemetrypb.TelemetryFrame)
	wantMain.Observations = wantMain.Observations[:len(wantMain.Observations)-1]
	if !proto.Equal(gotMain, wantMain) {
		t.Fatalf("main frame = %v, want %v", gotMain, wantMain)
	}
	if len(otaPayloads) != 1 {
		t.Fatalf("OTA frames = %d, want 1", len(otaPayloads))
	}
	gotOTA := new(telemetrypb.TelemetryFrame)
	if err := proto.Unmarshal(otaPayloads[0], gotOTA); err != nil {
		t.Fatal(err)
	}
	wantOTA := &telemetrypb.TelemetryFrame{
		Sequence:         frame.Sequence,
		ObservedAtUnixMs: frame.ObservedAtUnixMs,
		Observations:     frame.Observations[len(frame.Observations)-1:],
	}
	if !proto.Equal(gotOTA, wantOTA) {
		t.Fatalf("OTA frame = %v, want %v", gotOTA, wantOTA)
	}
}

func TestEncodeTelemetryOTAOnlyFrameSendsNoMainFrame(t *testing.T) {
	frame, err := decodeTelemetryFrame(map[string]any{
		"observations": []any{map[string]any{"ota": map[string]any{"state": "OTA_STATE_STARTED", "update_id": "u"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	mainPayload, otaPayloads, err := encodeTelemetry(frame)
	if err != nil {
		t.Fatal(err)
	}
	if mainPayload != nil || len(otaPayloads) != 1 {
		t.Fatalf("main = %v, OTA frames = %d; want no main frame and one OTA frame", mainPayload, len(otaPayloads))
	}
}
