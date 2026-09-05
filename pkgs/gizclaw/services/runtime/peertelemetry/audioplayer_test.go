package peertelemetry

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerrun"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"google.golang.org/protobuf/proto"
)

func TestAudioPlayerTelemetrySnapshotOrdering(t *testing.T) {
	peer := giznet.PublicKey{7}
	base := time.Unix(1700000000, 0)
	player := func(state string, position uint64, delta int32) *telemetrypb.Observation {
		return &telemetrypb.Observation{ObservedAtDeltaMs: delta, Body: &telemetrypb.Observation_Audioplayer{Audioplayer: &telemetrypb.AudioPlayerObservation{State: state, Repeat: "all", CurrentIndex: new(uint32(0)), PlaylistLength: 2, PlaylistRevision: 3, PositionMs: position}}}
	}
	samples, patch, err := MapFrame(peer, &telemetrypb.TelemetryFrame{Observations: []*telemetrypb.Observation{player("playing", 2000, 1000), player("buffering", 0, 0)}}, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 0 {
		t.Fatal("player telemetry created metric series")
	}
	if patch.AudioPlayer == nil || patch.AudioPlayer.PositionMs != 2000 {
		t.Fatalf("patch=%+v", patch)
	}
	store := &memoryStatusStore{status: map[giznet.PublicKey]apitypes.PeerStatus{peer: {Volume: new(35), BatteryPercent: new(80)}}}
	syncer := StatusSync{Store: store}
	if err := syncer.SyncTelemetryStatus(context.Background(), peer, patch); err != nil {
		t.Fatal(err)
	}
	_, stale, err := MapFrame(peer, &telemetrypb.TelemetryFrame{Observations: []*telemetrypb.Observation{player("buffering", 0, 0)}}, base)
	if err != nil {
		t.Fatal(err)
	}
	if err := syncer.SyncTelemetryStatus(context.Background(), peer, stale); err != nil {
		t.Fatal(err)
	}
	got := store.status[peer]
	if got.Audioplayer.State != "playing" || got.Audioplayer.PositionMs != 2000 || *got.Volume != 35 || *got.BatteryPercent != 80 {
		t.Fatalf("snapshot=%+v", got)
	}
	// An unrelated control response preserves the player; an older player status cannot regress it.
	_, err = syncer.ApplyDeviceStatus(context.Background(), peer, apitypes.PeerStatus{Volume: new(40), Audioplayer: stale.AudioPlayer}, base)
	if err != nil {
		t.Fatal(err)
	}
	got = store.status[peer]
	if *got.Volume != 40 || got.Audioplayer.State != "playing" {
		t.Fatalf("control snapshot=%+v", got)
	}
}

func TestAudioPlayerTelemetryErrorAndInvalidFrame(t *testing.T) {
	at := time.Unix(1700000000, 0)
	observation := &telemetrypb.AudioPlayerObservation{State: "error", Repeat: "off", PlaylistLength: 1, ErrorCode: new("FETCH_FAILED"), ErrorMessage: new("audio unavailable")}
	patch, err := mapAudioPlayer(observation, at)
	if err != nil || patch.AudioPlayer.ErrorCode == nil || *patch.AudioPlayer.ErrorCode != "FETCH_FAILED" {
		t.Fatalf("patch=%+v err=%v", patch, err)
	}
	observation.State = "playing"
	if _, err := mapAudioPlayer(observation, at); err == nil {
		t.Fatal("accepted active player without index")
	}
	observation.CurrentIndex = new(uint32(0))
	observation.ErrorCode = nil
	observation.ErrorMessage = nil
	observation.PositionMs = 1 << 53
	if _, err := mapAudioPlayer(observation, at); err == nil {
		t.Fatal("accepted lossy progress")
	}
}

func TestAudioPlayerTelemetryCrossLanguageGolden(t *testing.T) {
	frame := &telemetrypb.TelemetryFrame{Observations: []*telemetrypb.Observation{{Body: &telemetrypb.Observation_Audioplayer{Audioplayer: &telemetrypb.AudioPlayerObservation{State: "playing", CurrentIndex: new(uint32(0)), PositionMs: 1 << 32, Repeat: "all", PlaylistLength: 1, PlaylistRevision: 3}}}}}
	wire, err := proto.Marshal(frame)
	if err != nil {
		t.Fatal(err)
	}
	golden := []byte{0x1a, 0x1c, 0x7a, 0x1a, 0x0a, 0x07, 'p', 'l', 'a', 'y', 'i', 'n', 'g', 0x10, 0, 0x18, 0x80, 0x80, 0x80, 0x80, 0x10, 0x2a, 3, 'a', 'l', 'l', 0x30, 1, 0x38, 3}
	if !bytes.Equal(wire, golden) {
		t.Fatalf("wire=%x", wire)
	}
}

func TestAudioPlayerAndOTASnapshotsCoexist(t *testing.T) {
	ctx := context.Background()
	peer := testPublicKey(t)
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	runtime := &peerrun.Server{Store: store}
	metrics := &fakeMetricsStore{}
	service := &Service{Metrics: metrics, Status: StatusSync{Store: runtime}}
	frame := &telemetrypb.TelemetryFrame{ObservedAtUnixMs: 1700000000000, Observations: []*telemetrypb.Observation{
		{Body: &telemetrypb.Observation_Ota{Ota: &telemetrypb.OtaObservation{State: 2, UpdateId: "ota-1", DownloadPercent: new(50.0)}}},
		{Body: &telemetrypb.Observation_Audioplayer{Audioplayer: &telemetrypb.AudioPlayerObservation{State: "playing", Repeat: "off", CurrentIndex: new(uint32(0)), PlaylistLength: 1, PositionMs: 1000}}},
	}}
	if err := service.Report(ctx, peer, frame); err != nil {
		t.Fatal(err)
	}
	status, err := runtime.GetStatus(ctx, peer)
	if err != nil {
		t.Fatal(err)
	}
	if status.Ota == nil || status.Audioplayer == nil || *status.Ota.DownloadPercent != 50 || status.Audioplayer.PositionMs != 1000 {
		t.Fatalf("snapshots: %+v", status)
	}
	if len(metrics.samples) != 0 {
		t.Fatal("runtime observations created metric series")
	}
	// Unrelated device control must preserve both snapshots.
	status, err = (StatusSync{Store: runtime}).ApplyDeviceStatus(ctx, peer, apitypes.PeerStatus{Volume: new(35)}, time.UnixMilli(frame.ObservedAtUnixMs+1000))
	if err != nil {
		t.Fatal(err)
	}
	if status.Ota == nil || status.Audioplayer == nil || *status.Volume != 35 {
		t.Fatalf("control lost snapshot: %+v", status)
	}
}
