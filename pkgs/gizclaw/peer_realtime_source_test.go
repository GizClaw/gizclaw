package gizclaw

import (
	"context"
	"log/slog"
	"slices"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
)

func TestPeerRealtimeSourceRecordsFirstOpenAndPushOnce(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-1", "peer-1")
	source := newPeerRealtimeSourceWithLifecycle(lifecycle, genx.WithRealtimeStreamDelay(0))
	if _, err := source.OpenAgentInput(t.Context()); err != nil {
		t.Fatalf("OpenAgentInput(first) error = %v", err)
	}
	if _, err := source.OpenAgentInput(t.Context()); err != nil {
		t.Fatalf("OpenAgentInput(second) error = %v", err)
	}
	chunk := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "untrusted-stream-secret", BeginOfStream: true}}
	if err := source.Push(t.Context(), chunk); err != nil {
		t.Fatalf("Push(first) error = %v", err)
	}
	if err := source.Push(t.Context(), chunk); err != nil {
		t.Fatalf("Push(second) error = %v", err)
	}
	if err := source.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	records := capturedLifecycleRecords(t, capture)
	if len(records) != 2 {
		t.Fatalf("lifecycle records = %d, want one open and one push", len(records))
	}
	if attrs := lifecycleRecordAttrs(records[0]); attrs["stage"] != "agent_input_opened" {
		t.Fatalf("first lifecycle record = %#v", attrs)
	}
	attrs := lifecycleRecordAttrs(records[1])
	if attrs["stage"] != "agent_input_first_push" || attrs["stream_id_hash"] != safeStreamIDHash("untrusted-stream-secret") {
		t.Fatalf("second lifecycle record = %#v", attrs)
	}
}

func TestPeerRealtimeSourceRecordsFirstPushForEachTurn(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-turns", "peer-turns")
	source := newPeerRealtimeSourceWithLifecycle(lifecycle, genx.WithRealtimeStreamDelay(0))
	if _, err := source.OpenAgentInput(t.Context()); err != nil {
		t.Fatalf("OpenAgentInput() error = %v", err)
	}
	for index, streamID := range []string{"input-first", "input-second"} {
		lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, streamID, nil))
		for range 20 {
			if err := source.Push(t.Context(), &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: streamID}}); err != nil {
				t.Fatalf("Push(turn %d) error = %v", index+1, err)
			}
		}
	}
	var pushTurns []uint64
	for _, record := range capturedTurnLifecycleRecords(t, capture) {
		attrs := lifecycleRecordAttrs(record)
		if attrs["stage"] == "agent_input_first_push" {
			pushTurns = append(pushTurns, attrs["turn_index"].(uint64))
		}
	}
	if !slices.Equal(pushTurns, []uint64{1, 2}) {
		t.Fatalf("Agent input push turns = %v, want [1 2]", pushTurns)
	}
}

func TestPeerRealtimeSourceBindsDirectOpusToActiveAudioStream(t *testing.T) {
	ctx := context.Background()
	source := newPeerRealtimeSource(genx.WithRealtimeStreamDelay(0))
	input, err := source.OpenAgentInput(ctx)
	if err != nil {
		t.Fatalf("OpenAgentInput() error = %v", err)
	}

	firstStreamID := "audio-ui-first"
	if err := source.Push(ctx, &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xff}},
		Ctrl: &genx.StreamCtrl{StreamID: "audio"},
	}); err != nil {
		t.Fatalf("Push(pre-BOS audio) error = %v", err)
	}
	if err := source.Push(ctx, &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: firstStreamID, BeginOfStream: true},
	}); err != nil {
		t.Fatalf("Push(BOS) error = %v", err)
	}
	if err := source.Push(ctx, &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0x01}},
		Ctrl: &genx.StreamCtrl{StreamID: "audio"},
	}); err != nil {
		t.Fatalf("Push(audio) error = %v", err)
	}
	if err := source.Push(ctx, &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: firstStreamID, EndOfStream: true},
	}); err != nil {
		t.Fatalf("Push(EOS) error = %v", err)
	}

	wantStreamIDs := []string{firstStreamID, firstStreamID, firstStreamID}
	for i, want := range wantStreamIDs {
		got, err := input.Next()
		if err != nil {
			t.Fatalf("Next(%d) error = %v", i, err)
		}
		if got.Ctrl == nil || got.Ctrl.StreamID != want {
			t.Fatalf("Next(%d) stream id = %#v, want %q", i, got.Ctrl, want)
		}
	}

	if err := source.Push(ctx, &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0x02}},
		Ctrl: &genx.StreamCtrl{StreamID: "audio"},
	}); err != nil {
		t.Fatalf("Push(audio after EOS) error = %v", err)
	}
	secondStreamID := "audio-ui-second"
	if err := source.Push(ctx, &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: secondStreamID, BeginOfStream: true},
	}); err != nil {
		t.Fatalf("Push(second BOS) error = %v", err)
	}
	got, err := input.Next()
	if err != nil {
		t.Fatalf("Next(second BOS) error = %v", err)
	}
	if got.Ctrl == nil || got.Ctrl.StreamID != secondStreamID || !got.Ctrl.BeginOfStream {
		t.Fatalf("second BOS = %#v, want stream id %q", got.Ctrl, secondStreamID)
	}
}

func TestPeerRealtimeSourceOpenAgentInputClearsAudioStreamID(t *testing.T) {
	ctx := context.Background()
	source := newPeerRealtimeSource(genx.WithRealtimeStreamDelay(0))
	if _, err := source.OpenAgentInput(ctx); err != nil {
		t.Fatalf("OpenAgentInput(first) error = %v", err)
	}
	if err := source.Push(ctx, &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: "stale-audio", BeginOfStream: true},
	}); err != nil {
		t.Fatalf("Push(first BOS) error = %v", err)
	}
	input, err := source.OpenAgentInput(ctx)
	if err != nil {
		t.Fatalf("OpenAgentInput(second) error = %v", err)
	}
	if err := source.Push(ctx, &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0x01}},
		Ctrl: &genx.StreamCtrl{StreamID: "audio"},
	}); err != nil {
		t.Fatalf("Push(pre-BOS audio) error = %v", err)
	}
	if err := source.Push(ctx, &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: "fresh-audio", BeginOfStream: true},
	}); err != nil {
		t.Fatalf("Push(fresh BOS) error = %v", err)
	}
	got, err := input.Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if got.Ctrl == nil || got.Ctrl.StreamID != "fresh-audio" || !got.Ctrl.BeginOfStream {
		t.Fatalf("first chunk after reopen = %#v, want fresh BOS", got.Ctrl)
	}
}
