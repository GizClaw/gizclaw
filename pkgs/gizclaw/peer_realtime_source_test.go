package gizclaw

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
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

func TestPeerRealtimeSourceWithoutLifecycleUsesUnderlyingStream(t *testing.T) {
	source := newPeerRealtimeSourceWithLifecycle(nil, genx.WithRealtimeStreamDelay(0))
	input, err := source.OpenAgentInput(t.Context())
	if err != nil {
		t.Fatalf("OpenAgentInput() error = %v", err)
	}
	if input != source.current {
		t.Fatalf("disabled input type = %T, want underlying %T", input, source.current)
	}
	chunk := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "input", BeginOfStream: true}}
	if err := source.Push(t.Context(), chunk); err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	got, err := input.Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if got != chunk {
		t.Fatalf("Next() = %p, want %p", got, chunk)
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

func TestPeerRealtimeSourceRecordsTransformOnlyAfterAgentConsumes(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-consume", "peer-consume")
	source := newPeerRealtimeSourceWithLifecycle(lifecycle, genx.WithRealtimeStreamDelay(0))
	input, err := source.OpenAgentInput(t.Context())
	if err != nil {
		t.Fatalf("OpenAgentInput() error = %v", err)
	}
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input-consume", nil))
	chunk := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "input-consume", BeginOfStream: true}}
	if err := source.Push(t.Context(), chunk); err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	assertNoTurnStage(t, capture, "agent_transform_started")

	consumed := make(chan error, 1)
	go func() {
		_, err := input.Next()
		consumed <- err
	}()
	select {
	case err := <-consumed:
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Agent input consumption timed out")
	}

	var transform map[string]any
	for _, record := range capturedTurnLifecycleRecords(t, capture) {
		attrs := lifecycleRecordAttrs(record)
		if attrs["stage"] == "agent_transform_started" {
			transform = attrs
		}
	}
	if transform["turn_index"] != uint64(1) {
		t.Fatalf("transform lifecycle = %#v", transform)
	}
}

func TestPeerRealtimeSourceRetainsTurnUntilDelayedConsumptionAfterEOS(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-delayed", "peer-delayed")
	source := newPeerRealtimeSourceWithLifecycle(lifecycle, genx.WithRealtimeStreamDelay(0))
	input, err := source.OpenAgentInput(t.Context())
	if err != nil {
		t.Fatalf("OpenAgentInput() error = %v", err)
	}
	const streamID = "input-delayed"
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, streamID, nil))
	chunk := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: streamID, BeginOfStream: true}}
	if err := source.Push(t.Context(), chunk); err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_EOS, streamID, nil))
	assertNoTurnStage(t, capture, "agent_transform_started")

	if _, err := input.Next(); err != nil {
		t.Fatalf("delayed Next() error = %v", err)
	}
	for _, record := range capturedTurnLifecycleRecords(t, capture) {
		attrs := lifecycleRecordAttrs(record)
		if attrs["stage"] == "agent_transform_started" && attrs["turn_index"] == uint64(1) {
			return
		}
	}
	t.Fatal("agent_transform_started was not correlated after input EOS")
}

func assertNoTurnStage(t *testing.T, capture *slogCapture, stage string) {
	t.Helper()
	for _, record := range capturedTurnLifecycleRecords(t, capture) {
		if attrs := lifecycleRecordAttrs(record); attrs["stage"] == stage {
			t.Fatalf("unexpected stage %q before boundary: %#v", stage, attrs)
		}
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

func TestPeerRealtimeSourceReportsReplacedAudioRouteBeforeReturning(t *testing.T) {
	var got peerAudioInputRoute
	source := newPeerRealtimeSourceWithRouteReplacement(nil, func(_ context.Context, route peerAudioInputRoute) error {
		got = route
		return nil
	}, genx.WithRealtimeStreamDelay(0))
	if _, err := source.OpenAgentInput(t.Context()); err != nil {
		t.Fatalf("OpenAgentInput(first) error = %v", err)
	}
	if err := source.Push(t.Context(), &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus; rate=16000"},
		Ctrl: &genx.StreamCtrl{StreamID: "old-audio", BeginOfStream: true},
	}); err != nil {
		t.Fatalf("Push(BOS) error = %v", err)
	}
	if _, err := source.OpenAgentInput(t.Context()); err != nil {
		t.Fatalf("OpenAgentInput(reload) error = %v", err)
	}
	if got.streamID != "old-audio" || got.mimeType != "audio/opus" {
		t.Fatalf("replaced route = %#v", got)
	}
}

func TestPeerRealtimeSourceFailsOpenWhenRouteReplacementFails(t *testing.T) {
	wantErr := errors.New("event stream failed")
	var callbacks atomic.Int32
	source := newPeerRealtimeSourceWithRouteReplacement(nil, func(context.Context, peerAudioInputRoute) error {
		callbacks.Add(1)
		return wantErr
	}, genx.WithRealtimeStreamDelay(0))
	if _, err := source.OpenAgentInput(t.Context()); err != nil {
		t.Fatalf("OpenAgentInput(first) error = %v", err)
	}
	if err := source.Push(t.Context(), &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: "old-audio", BeginOfStream: true},
	}); err != nil {
		t.Fatalf("Push(BOS) error = %v", err)
	}
	if input, err := source.OpenAgentInput(t.Context()); !errors.Is(err, wantErr) || input != nil {
		t.Fatalf("OpenAgentInput(reload) = (%T, %v), want (nil, %v)", input, err, wantErr)
	}
	if source.current != nil {
		t.Fatal("failed replacement left a new Agent input active")
	}
	if _, err := source.OpenAgentInput(t.Context()); err != nil {
		t.Fatalf("OpenAgentInput(after failed replacement) error = %v", err)
	}
	if got := callbacks.Load(); got != 1 {
		t.Fatalf("replacement callbacks after retry = %d, want 1", got)
	}
}

func TestPeerRealtimeSourceDoesNotReportBOSRejectedWithoutActiveInput(t *testing.T) {
	var callbacks atomic.Int32
	source := newPeerRealtimeSourceWithRouteReplacement(nil, func(context.Context, peerAudioInputRoute) error {
		callbacks.Add(1)
		return nil
	}, genx.WithRealtimeStreamDelay(0))
	err := source.Push(t.Context(), &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: "never-admitted", BeginOfStream: true},
	})
	if !errors.Is(err, agenthost.ErrNoActiveInput) {
		t.Fatalf("Push(without input) error = %v, want %v", err, agenthost.ErrNoActiveInput)
	}
	if _, err := source.OpenAgentInput(t.Context()); err != nil {
		t.Fatalf("OpenAgentInput(first) error = %v", err)
	}
	if _, err := source.OpenAgentInput(t.Context()); err != nil {
		t.Fatalf("OpenAgentInput(second) error = %v", err)
	}
	if got := callbacks.Load(); got != 0 {
		t.Fatalf("replacement callbacks = %d, want 0", got)
	}
}

func TestPeerRealtimeSourceDoesNotReportInactiveOrDuplicateRoute(t *testing.T) {
	var routes []peerAudioInputRoute
	source := newPeerRealtimeSourceWithRouteReplacement(nil, func(_ context.Context, route peerAudioInputRoute) error {
		routes = append(routes, route)
		return nil
	}, genx.WithRealtimeStreamDelay(0))
	if _, err := source.OpenAgentInput(t.Context()); err != nil {
		t.Fatalf("OpenAgentInput(first) error = %v", err)
	}
	if _, err := source.OpenAgentInput(t.Context()); err != nil {
		t.Fatalf("OpenAgentInput(without route) error = %v", err)
	}
	if err := source.Push(t.Context(), &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: "only-route", BeginOfStream: true},
	}); err != nil {
		t.Fatalf("Push(BOS) error = %v", err)
	}
	if _, err := source.OpenAgentInput(t.Context()); err != nil {
		t.Fatalf("OpenAgentInput(replace route) error = %v", err)
	}
	if _, err := source.OpenAgentInput(t.Context()); err != nil {
		t.Fatalf("OpenAgentInput(after replacement) error = %v", err)
	}
	if len(routes) != 1 || routes[0].streamID != "only-route" {
		t.Fatalf("replacement callbacks = %#v", routes)
	}
}

func TestPeerRealtimeSourceConcurrentOpenReportsRouteOnce(t *testing.T) {
	var callbacks atomic.Int32
	source := newPeerRealtimeSourceWithRouteReplacement(nil, func(_ context.Context, route peerAudioInputRoute) error {
		if route.streamID != "active-route" || route.mimeType != "audio/opus" {
			t.Errorf("replacement route = %#v", route)
		}
		callbacks.Add(1)
		return nil
	}, genx.WithRealtimeStreamDelay(0))
	if _, err := source.OpenAgentInput(t.Context()); err != nil {
		t.Fatalf("OpenAgentInput(first) error = %v", err)
	}
	if err := source.Push(t.Context(), &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus; rate=48000"},
		Ctrl: &genx.StreamCtrl{StreamID: "active-route", BeginOfStream: true},
	}); err != nil {
		t.Fatalf("Push(BOS) error = %v", err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := source.OpenAgentInput(t.Context()); err != nil {
				t.Errorf("OpenAgentInput(concurrent) error = %v", err)
			}
		})
	}
	wg.Wait()
	if got := callbacks.Load(); got != 1 {
		t.Fatalf("replacement callbacks = %d, want 1", got)
	}
}
