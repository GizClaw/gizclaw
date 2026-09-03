package giztestcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

func testOggOpus(t *testing.T) ([]byte, [][]byte) {
	t.Helper()
	packets, err := appendRealtimeTailSilence(nil, 40*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	var audio bytes.Buffer
	if err := codecconv.OpusPacketsToOgg(&audio, 16000, 1, packets); err != nil {
		t.Fatal(err)
	}
	return audio.Bytes(), packets
}

func TestAudioInputChunksKeepRealtimeOpen(t *testing.T) {
	for _, tc := range []struct {
		mode    string
		wantEOS bool
	}{
		{mode: "push-to-talk", wantEOS: true},
		{mode: "realtime", wantEOS: false},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			chunks := audioInputChunks(tc.mode, "turn", "audio/opus", [][]byte{{1, 2, 3}})
			last := chunks[len(chunks)-1]
			if got := last.IsEndOfStream(); got != tc.wantEOS {
				t.Fatalf("last chunk EndOfStream = %t, want %t", got, tc.wantEOS)
			}
		})
	}
}

func TestPeerStreamTerminalErrorIsNotInterruption(t *testing.T) {
	chunk := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{Error: "provider quota exceeded"}}
	if got := peerStreamTerminalError(chunk); got != "provider quota exceeded" {
		t.Fatalf("terminal error classification = %q", got)
	}
	chunk.Ctrl.Error = "interrupted"
	if got := peerStreamTerminalError(chunk); got != "" {
		t.Fatalf("interruption classified as terminal error %q", got)
	}
}

func TestPeerStreamAudioCaptureMaxBytes(t *testing.T) {
	vars := mustVariables(t, map[string]giztest.VariableSpec{
		"audio": {Direction: "output", Type: "audio", MaxBytes: 4096},
	})
	for _, tc := range []struct {
		name string
		step giztest.Step
		want int
	}{
		{name: "streamed without buffering", step: giztest.Step{}, want: 0},
		{name: "explicit audio capture", step: giztest.Step{Capture: map[string]string{"audio": "/audio"}}, want: 4096},
		{name: "unrelated capture", step: giztest.Step{Capture: map[string]string{"audio": "/text"}}, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := peerStreamAudioCaptureMaxBytes(tc.step, vars)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("capture max bytes = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestDecodeOpusPacketsRejectsEmptyInput(t *testing.T) {
	if _, err := decodeOpusPackets(nil); err == nil {
		t.Fatal("empty Opus input accepted")
	}
}

func TestDecodeOpusPacketsCopiesRawPacket(t *testing.T) {
	input := []byte{1, 2, 3}
	packets, err := decodeOpusPackets(input)
	if err != nil {
		t.Fatal(err)
	}
	input[0] = 9
	if packets[0][0] != 1 {
		t.Fatal("packet aliases caller input")
	}
}

func TestPeerAudioPacingSummarizesPacketClockAndArrivalGaps(t *testing.T) {
	var pacing peerAudioPacing
	packet := []byte{0xf8}
	started := time.Unix(1, 0)
	for _, offset := range []time.Duration{0, 20 * time.Millisecond, 41 * time.Millisecond, 60 * time.Millisecond} {
		pacing.observe(started.Add(offset), [][]byte{packet})
	}
	summary := pacing.summary()
	if summary["packets"] != 4 || summary["audio_ms"] != int64(80) || summary["target_span_ms"] != int64(60) || summary["receive_span_ms"] != int64(60) {
		t.Fatalf("pacing summary = %#v", summary)
	}
	if summary["mean_packet_ms"] != float64(20) || summary["mean_interval_ms"] != float64(20) || summary["p95_interval_ms"] != float64(21) || summary["max_interval_ms"] != float64(21) || summary["absolute_drift_ms"] != float64(0) {
		t.Fatalf("pacing intervals = %#v", summary)
	}
}

func TestPeerAudioPacingOmitsUnavailableIntervals(t *testing.T) {
	var pacing peerAudioPacing
	if summary := pacing.summary(); summary != nil {
		t.Fatalf("empty pacing summary = %#v, want nil", summary)
	}
	pacing.observe(time.Unix(1, 0), [][]byte{{0xf8}})
	summary := pacing.summary()
	if len(summary) != 2 || summary["packets"] != 1 || summary["audio_ms"] != int64(20) {
		t.Fatalf("single-packet pacing summary = %#v", summary)
	}
}

func TestInvokePeerStreamObservesAssistantOpus(t *testing.T) {
	stream := newFakeRelayStream()
	oggAudio, packets := testOggOpus(t)
	go func() {
		drainPushes(stream, 3)
		stream.in <- assistantText("s1", "done", false)
		stream.in <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/ogg; codecs=opus", Data: oggAudio}, Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant"}}
		stream.in <- assistantText("s1", "", true)
		stream.in <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/ogg; codecs=opus"}, Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant", EndOfStream: true}}
	}()
	var client string
	var role string
	var observed [][]byte
	open := func() (peerStream, error) { return stream, nil }
	result, err := invokePeerStream(context.Background(), nil, open, giztest.Step{
		ID: "turn", Client: "peer", PeerStream: &giztest.PeerStreamOperation{Mode: "text"},
	}, "hello", 0, func(gotClient, gotRole string, gotPacket []byte, _ bool) error {
		client = gotClient
		role = gotRole
		if len(gotPacket) > 0 {
			observed = append(observed, gotPacket)
			gotPacket[0] = 9
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if client != "peer" || role != "assistant" || len(observed) != len(packets) || observed[0][0] != 9 || packets[0][0] == 9 {
		t.Fatalf("observer client=%q role=%q packet_count=%d source_count=%d", client, role, len(observed), len(packets))
	}
	pacing := result.assertion.(map[string]any)["audio_pacing"].(map[string]any)
	if pacing["packets"] != len(packets) || pacing["audio_ms"] != int64(40) {
		t.Fatalf("audio pacing = %#v", pacing)
	}
}

func TestInvokePeerStreamObservesUserBeforeAssistant(t *testing.T) {
	stream := newFakeRelayStream()
	go func() {
		drainPushes(stream, 3)
		stream.in <- assistantText("s1", "done", false)
		stream.in <- assistantBlob("s1", []byte{2}, false)
		stream.in <- assistantText("s1", "", true)
		stream.in <- assistantBlob("s1", nil, true)
	}()
	var roles []string
	_, err := invokePeerStream(context.Background(), nil, func() (peerStream, error) { return stream, nil }, giztest.Step{
		ID: "turn", Client: "peer", PeerStream: &giztest.PeerStreamOperation{Mode: "push-to-talk"},
	}, []byte{1}, 0, func(_ string, role string, _ []byte, end bool) error {
		if end {
			roles = append(roles, role)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(roles, []string{"user", "assistant"}) {
		t.Fatalf("observed roles = %v", roles)
	}
}

func TestInvokePeerStreamDoesNotWaitForUserPlaybackBeforePush(t *testing.T) {
	packets, err := appendRealtimeTailSilence(nil, playStartBuffer)
	if err != nil {
		t.Fatal(err)
	}
	var audio bytes.Buffer
	if err := codecconv.OpusPacketsToOgg(&audio, 16000, 1, packets); err != nil {
		t.Fatal(err)
	}
	output := &closeUnblocksPlayOutput{started: make(chan struct{}), closed: make(chan struct{})}
	session := &playSession{decoder: &fakePlayDecoder{samples: []int16{1}}, output: output}
	t.Cleanup(func() { _ = session.close() })
	stream := newFakeRelayStream()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, invokeErr := invokePeerStream(ctx, nil, func() (peerStream, error) { return stream, nil }, giztest.Step{
			ID: "turn", Client: "peer", PeerStream: &giztest.PeerStreamOperation{Mode: "push-to-talk"},
		}, audio.Bytes(), 0, session.observe)
		result <- invokeErr
	}()
	select {
	case <-output.started:
	case <-time.After(time.Second):
		t.Fatal("user playback did not start")
	}
	select {
	case <-stream.pushes:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("outbound turn waited for blocked local user playback")
	}
	cancel()
	_ = output.Close()
	select {
	case <-result:
	case <-time.After(time.Second):
		t.Fatal("peer stream did not stop after cancellation")
	}
}

func TestInvokePeerStreamPropagatesAudioObserverFailure(t *testing.T) {
	stream := newFakeRelayStream()
	go func() {
		drainPushes(stream, 3)
		stream.in <- assistantBlob("s1", []byte{1}, false)
		stream.in <- assistantText("s1", "done", false)
		stream.in <- assistantText("s1", "", true)
		stream.in <- assistantBlob("s1", nil, true)
	}()
	open := func() (peerStream, error) { return stream, nil }
	_, err := invokePeerStream(context.Background(), nil, open, giztest.Step{
		ID: "turn", Client: "peer", PeerStream: &giztest.PeerStreamOperation{Mode: "text"},
	}, "hello", 0, func(string, string, []byte, bool) error { return errors.New("speaker failed") })
	if err == nil || !strings.Contains(err.Error(), "speaker failed") {
		t.Fatalf("observer error = %v", err)
	}
}

func TestFirstHistoryName(t *testing.T) {
	result := map[string]any{"items": []any{map[string]any{"name": "history-1"}}}
	if got := firstHistoryName(result); got != "history-1" {
		t.Fatalf("firstHistoryName() = %q, want history-1", got)
	}
	if got := firstHistoryName(map[string]any{}); got != "" {
		t.Fatalf("firstHistoryName(empty) = %q", got)
	}
}

func TestStreamIDMatchesDerivedRealtimeSegment(t *testing.T) {
	for _, tc := range []struct {
		actual   string
		expected string
		want     bool
	}{
		{actual: "turn", expected: "turn", want: true},
		{actual: "turn:rt:2", expected: "turn", want: true},
		{actual: "turn-2:rt:1", expected: "turn-1", want: false},
		{actual: "turn", expected: "", want: false},
	} {
		if got := streamIDMatches(tc.actual, tc.expected); got != tc.want {
			t.Errorf("streamIDMatches(%q, %q) = %t, want %t", tc.actual, tc.expected, got, tc.want)
		}
	}
}

func TestAppendRealtimeTailSilence(t *testing.T) {
	input := [][]byte{{1, 2, 3}}
	packets, err := appendRealtimeTailSilence(input, 60*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(packets) != 4 {
		t.Fatalf("packet count = %d, want 4", len(packets))
	}
	if len(packets[1]) == 0 || len(packets[2]) == 0 || len(packets[3]) == 0 {
		t.Fatal("encoded silence contains an empty Opus packet")
	}
	if got := len(input); got != 1 {
		t.Fatalf("input packet slice mutated to length %d", got)
	}
}

// drainPushes waits for the peer_stream input turn to be pushed so a test can
// time stream events relative to the armed inactivity timer.
func drainPushes(stream *fakeRelayStream, count int) {
	for range count {
		select {
		case <-stream.pushes:
		case <-time.After(2 * time.Second):
			return
		}
	}
}

func finishAssistantTurn(stream *fakeRelayStream, id string) {
	stream.in <- assistantText(id, "done", false)
	stream.in <- assistantBlob(id, []byte{1, 2, 3}, false)
	stream.in <- assistantText(id, "", true)
	stream.in <- assistantBlob(id, nil, true)
}

func transcriptText(id, text string, eos bool) *genx.MessageChunk {
	return &genx.MessageChunk{
		Part: genx.Text(text),
		Ctrl: &genx.StreamCtrl{StreamID: id, Label: "transcript", EndOfStream: eos},
	}
}

func invokeFakePeerStream(ctx context.Context, op giztest.PeerStreamOperation, streams ...*fakeRelayStream) (operationResult, error) {
	index := 0
	open := func() (peerStream, error) {
		if index >= len(streams) {
			return nil, fmt.Errorf("unexpected PeerStream open %d", index)
		}
		stream := streams[index]
		index++
		return stream, nil
	}
	return invokePeerStream(ctx, nil, open, giztest.Step{ID: "turn", PeerStream: &op}, "hello", 0)
}

func TestInvokePeerStreamRearmsRetainedRealtimeSession(t *testing.T) {
	stream := newFakeRelayStream()
	sessions := newPeerStreamSessions()
	t.Cleanup(func() { _ = sessions.Close() })
	openCount := 0
	open := func() (peerStream, error) {
		openCount++
		return stream, nil
	}
	requireAudio := false
	firstStep := giztest.Step{ID: "first", Client: "peer", PeerStream: &giztest.PeerStreamOperation{
		Mode: "realtime", Session: "microphone", KeepOpen: true,
		Completion: "first_response", FirstTextTimeout: "1s", RequireAudio: &requireAudio,
	}}
	firstStreamID := make(chan string, 1)
	go func() {
		for i := range 202 {
			chunk := <-stream.pushes
			if i == 0 {
				firstStreamID <- chunk.Ctrl.StreamID
			}
		}
		stream.in <- assistantText("assistant-1", "ready", false)
	}()
	first, err := invokePeerStreamWithSessions(context.Background(), nil, open, sessions, firstStep, []byte{1}, 0)
	if err != nil {
		t.Fatalf("retain realtime session: %v", err)
	}
	oldID := <-firstStreamID
	if first.evidence["session_retained"] != true || openCount != 1 {
		t.Fatalf("first evidence=%#v open_count=%d", first.evidence, openCount)
	}
	stream.in <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{
		StreamID: oldID, Label: "user", EndOfStream: true,
		ErrorCode: "INPUT_ROUTE_RELOADED", Error: "input route reloaded", ErrorRetryable: true,
	}}
	secondStep := giztest.Step{ID: "second", Client: "peer", PeerStream: &giztest.PeerStreamOperation{
		Mode: "realtime", Session: "microphone", AwaitRearm: "INPUT_ROUTE_RELOADED",
		KeepOpen: true, Completion: "first_response", FirstTextTimeout: "1s", RequireAudio: &requireAudio,
	}}
	secondStreamID := make(chan string, 1)
	go func() {
		for i := range 202 {
			chunk := <-stream.pushes
			if i == 0 {
				secondStreamID <- chunk.Ctrl.StreamID
			}
		}
		stream.in <- assistantText("assistant-2", "ready again", false)
	}()
	second, err := invokePeerStreamWithSessions(context.Background(), nil, open, sessions, secondStep, []byte{1}, 0)
	if err != nil {
		t.Fatalf("re-arm retained realtime session: %v", err)
	}
	newID := <-secondStreamID
	if newID == oldID || openCount != 1 || second.evidence["reload_eos_observed"] != true || second.evidence["replacement_bos_sent"] != true || second.evidence["stream_id_changed"] != true || second.evidence["session_connection_reused"] != true || second.evidence["session_retained"] != true {
		t.Fatalf("old_id=%q new_id=%q open_count=%d evidence=%#v", oldID, newID, openCount, second.evidence)
	}
	select {
	case <-stream.closed:
		t.Fatal("re-retained session closed after the second step")
	default:
	}
	stream.in <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{
		StreamID: newID, Label: "user", EndOfStream: true,
		ErrorCode: "INPUT_ROUTE_RELOADED", Error: "input route reloaded", ErrorRetryable: true,
	}}
	thirdStreamID := make(chan string, 1)
	go func() {
		for i := range 202 {
			chunk := <-stream.pushes
			if i == 0 {
				thirdStreamID <- chunk.Ctrl.StreamID
			}
		}
		stream.in <- assistantText("assistant-3", "ready once more", false)
	}()
	third, err := invokePeerStreamWithSessions(context.Background(), nil, open, sessions, giztest.Step{ID: "third", Client: "peer", PeerStream: &giztest.PeerStreamOperation{
		Mode: "realtime", Session: "microphone", AwaitRearm: "INPUT_ROUTE_RELOADED",
		Completion: "first_response", FirstTextTimeout: "1s", RequireAudio: &requireAudio,
	}}, []byte{1}, 0)
	if err != nil {
		t.Fatalf("consume re-retained realtime session: %v", err)
	}
	newestID := <-thirdStreamID
	if newestID == newID || openCount != 1 || third.evidence["stream_id_changed"] != true {
		t.Fatalf("second_id=%q third_id=%q open_count=%d evidence=%#v", newID, newestID, openCount, third.evidence)
	}
	select {
	case <-stream.closed:
	default:
		t.Fatal("consumed session was not closed")
	}
}

func TestInvokePeerStreamAwaitRearmTimesOutWithCausalEvidence(t *testing.T) {
	stream := newFakeRelayStream()
	sessions := newPeerStreamSessions()
	session := newPeerStreamSession("peer", stream)
	session.streamID = "old-route"
	if err := sessions.add("microphone", session); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	result, err := invokePeerStreamWithSessions(ctx, nil, func() (peerStream, error) {
		t.Fatal("await_rearm opened a replacement PeerStream")
		return nil, nil
	}, sessions, giztest.Step{ID: "second", Client: "peer", PeerStream: &giztest.PeerStreamOperation{
		Mode: "realtime", Session: "microphone", AwaitRearm: "INPUT_ROUTE_RELOADED",
	}}, []byte{1}, 0)
	if err == nil || !strings.Contains(err.Error(), "timed out waiting for re-arm INPUT_ROUTE_RELOADED") {
		t.Fatalf("error = %v", err)
	}
	if result.evidence["reload_eos_observed"] != false || result.evidence["replacement_bos_sent"] != false || result.evidence["stream_id_changed"] != false || result.evidence["session_connection_reused"] != true {
		t.Fatalf("evidence = %#v", result.evidence)
	}
	if _, leaked := result.evidence["old_stream_id"]; leaked {
		t.Fatalf("evidence leaked a raw stream ID: %#v", result.evidence)
	}
	select {
	case <-stream.closed:
	default:
		t.Fatal("timed-out retained session was not closed")
	}
}

func TestInvokePeerStreamAwaitRearmPreservesSuccessfulBOSEvidenceOnResponseTimeout(t *testing.T) {
	stream := newFakeRelayStream()
	sessions := newPeerStreamSessions()
	session := newPeerStreamSession("peer", stream)
	session.streamID = "old-route"
	session.startReader()
	if err := sessions.add("microphone", session); err != nil {
		t.Fatal(err)
	}
	stream.in <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{
		StreamID: "old-route", Label: "user", EndOfStream: true,
		ErrorCode: "INPUT_ROUTE_RELOADED", Error: "input route reloaded", ErrorRetryable: true,
	}}
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(context.Canceled)
	firstReplacement := make(chan *genx.MessageChunk, 1)
	go func() {
		for i := range 202 {
			chunk := <-stream.pushes
			if i == 0 {
				firstReplacement <- chunk
			}
		}
		cancel(context.DeadlineExceeded)
	}()
	result, err := invokePeerStreamWithSessions(ctx, nil, func() (peerStream, error) {
		t.Fatal("await_rearm opened a replacement PeerStream")
		return nil, nil
	}, sessions, giztest.Step{ID: "second", Client: "peer", PeerStream: &giztest.PeerStreamOperation{
		Mode: "realtime", Session: "microphone", AwaitRearm: "INPUT_ROUTE_RELOADED",
	}}, []byte{1}, 0)
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("error = %v", err)
	}
	first := <-firstReplacement
	if first.Ctrl == nil || !first.IsBeginOfStream() || first.Ctrl.StreamID == "" || first.Ctrl.StreamID == "old-route" || first.Ctrl.Label != "user" {
		t.Fatalf("first replacement chunk = %#v", first)
	}
	if result.evidence["reload_eos_observed"] != true || result.evidence["replacement_bos_sent"] != true || result.evidence["stream_id_changed"] != true || result.evidence["session_connection_reused"] != true {
		t.Fatalf("evidence = %#v", result.evidence)
	}
	select {
	case <-stream.closed:
	default:
		t.Fatal("response-timeout retained session was not closed")
	}
}

func TestInvokePeerStreamIdleTimeoutAllowsLongReply(t *testing.T) {
	stream := newFakeRelayStream()
	go func() {
		drainPushes(stream, 3)
		for i := range 6 {
			time.Sleep(30 * time.Millisecond)
			stream.in <- assistantText("s1", fmt.Sprintf("part %d", i), false)
		}
		finishAssistantTurn(stream, "s1")
	}()
	result, err := invokeFakePeerStream(context.Background(), giztest.PeerStreamOperation{Mode: "text", IdleTimeout: "120ms"}, stream)
	if err != nil {
		t.Fatalf("long reply failed: %v", err)
	}
	if result.evidence["idle_timeout_ms"] != int64(120) {
		t.Fatalf("idle_timeout_ms = %#v", result.evidence["idle_timeout_ms"])
	}
	if last, _ := result.evidence["last_event_ms"].(int64); last <= 0 {
		t.Fatalf("last_event_ms = %#v", result.evidence["last_event_ms"])
	}
	if _, ok := result.evidence["deadline"]; ok {
		t.Fatalf("passing evidence names a deadline: %#v", result.evidence)
	}
}

func TestInvokePeerStreamIdleTimeoutFailsOnStall(t *testing.T) {
	stream := newFakeRelayStream()
	go func() {
		drainPushes(stream, 3)
		stream.in <- assistantText("s1", "hello", false)
	}()
	result, err := invokeFakePeerStream(context.Background(), giztest.PeerStreamOperation{Mode: "text", IdleTimeout: "50ms"}, stream)
	if err == nil || !strings.HasPrefix(err.Error(), "peer_stream idle timeout exceeded after 50ms") || !strings.Contains(err.Error(), "deadline=idle_timeout") {
		t.Fatalf("error = %v", err)
	}
	if result.evidence["deadline"] != "idle_timeout" || result.evidence["events"] != 1 || result.evidence["idle_timeout_ms"] != int64(50) {
		t.Fatalf("evidence = %#v", result.evidence)
	}
	if _, ok := result.evidence["last_event_ms"].(int64); !ok {
		t.Fatalf("evidence lacks last_event_ms: %#v", result.evidence)
	}
}

func TestInvokePeerStreamStepTimeoutWinsOverIdleTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	stream := newFakeRelayStream()
	go func() {
		drainPushes(stream, 3)
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Millisecond):
				stream.in <- assistantText("s1", "still streaming", false)
			}
		}
	}()
	result, err := invokeFakePeerStream(ctx, giztest.PeerStreamOperation{Mode: "text", IdleTimeout: "1s"}, stream)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "deadline=timeout") {
		t.Fatalf("error = %v", err)
	}
	if result.evidence["deadline"] != "timeout" || result.evidence["idle_timeout_ms"] != int64(1000) {
		t.Fatalf("evidence = %#v", result.evidence)
	}
	if events, _ := result.evidence["events"].(int); events < 2 {
		t.Fatalf("events = %#v, want a streaming reply", result.evidence["events"])
	}
}

func TestInvokePeerStreamWithoutIdleTimeoutWaitsThroughGaps(t *testing.T) {
	stream := newFakeRelayStream()
	go func() {
		drainPushes(stream, 3)
		stream.in <- assistantText("s1", "hello", false)
		time.Sleep(120 * time.Millisecond)
		finishAssistantTurn(stream, "s1")
	}()
	result, err := invokeFakePeerStream(context.Background(), giztest.PeerStreamOperation{Mode: "text"}, stream)
	if err != nil {
		t.Fatalf("reply without idle_timeout failed: %v", err)
	}
	if _, ok := result.evidence["idle_timeout_ms"]; ok {
		t.Fatalf("evidence reports idle_timeout_ms without idle_timeout: %#v", result.evidence)
	}
	if last, _ := result.evidence["last_event_ms"].(int64); last <= 0 {
		t.Fatalf("last_event_ms = %#v", result.evidence["last_event_ms"])
	}
}

func TestInvokePeerStreamFirstResponseReturnsWithoutEOS(t *testing.T) {
	stream := newFakeRelayStream()
	go func() {
		drainPushes(stream, 3)
		stream.in <- transcriptText("user-1", "question", false)
		stream.in <- assistantText("s1", "hello", false)
		stream.in <- assistantBlob("s1", []byte{1, 2, 3}, false)
	}()
	result, err := invokeFakePeerStream(context.Background(), giztest.PeerStreamOperation{
		Mode: "text", Completion: "first_response", FirstTextTimeout: "100ms", FirstAudioTimeout: "150ms",
	}, stream)
	if err != nil {
		t.Fatalf("first response failed: %v", err)
	}
	object, _ := result.assertion.(map[string]any)
	if object["text_eos"] != false || object["audio_eos"] != false {
		t.Fatalf("first response waited for terminal output: %#v", object)
	}
	if object["events"] != 3 || object["first_transcript_ms"] == nil || object["first_text_ms"] == nil || object["first_audio_ms"] == nil {
		t.Fatalf("first response assertion = %#v", object)
	}
	select {
	case <-stream.closed:
	case <-time.After(time.Second):
		t.Fatal("first response did not close its probe stream")
	}
}

func TestInvokePeerStreamFirstResponseSelectedModalities(t *testing.T) {
	textRequired, audioRequired := true, true
	textDisabled, audioDisabled := false, false
	for _, tc := range []struct {
		name string
		op   giztest.PeerStreamOperation
		send func(*fakeRelayStream)
	}{
		{
			name: "text only",
			op: giztest.PeerStreamOperation{
				Mode: "text", Completion: "first_response", FirstTextTimeout: "100ms",
				RequireText: &textRequired, RequireAudio: &audioDisabled,
			},
			send: func(stream *fakeRelayStream) { stream.in <- assistantText("s1", "hello", false) },
		},
		{
			name: "audio only",
			op: giztest.PeerStreamOperation{
				Mode: "text", Completion: "first_response", FirstAudioTimeout: "100ms",
				RequireText: &textDisabled, RequireAudio: &audioRequired,
			},
			send: func(stream *fakeRelayStream) { stream.in <- assistantBlob("s1", []byte{1}, false) },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := newFakeRelayStream()
			go func() {
				drainPushes(stream, 3)
				tc.send(stream)
			}()
			result, err := invokeFakePeerStream(context.Background(), tc.op, stream)
			if err != nil {
				t.Fatalf("selected first response failed: %v", err)
			}
			object := result.assertion.(map[string]any)
			if object["events"] != 1 {
				t.Fatalf("first response assertion = %#v", object)
			}
			if object["first_transcript_ms"] != int64(0) {
				t.Fatalf("missing transcript evidence = %#v, want zero", object["first_transcript_ms"])
			}
		})
	}
}

func TestInvokePeerStreamFirstResponseSelectedModalityDeadlines(t *testing.T) {
	textRequired, audioRequired := true, true
	textDisabled, audioDisabled := false, false
	for _, tc := range []struct {
		name     string
		op       giztest.PeerStreamOperation
		deadline string
	}{
		{
			name: "text only",
			op: giztest.PeerStreamOperation{
				Mode: "text", Completion: "first_response", FirstTextTimeout: "30ms",
				RequireText: &textRequired, RequireAudio: &audioDisabled,
			},
			deadline: "first_text_timeout",
		},
		{
			name: "audio only",
			op: giztest.PeerStreamOperation{
				Mode: "text", Completion: "first_response", FirstAudioTimeout: "30ms",
				RequireText: &textDisabled, RequireAudio: &audioRequired,
			},
			deadline: "first_audio_timeout",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := newFakeRelayStream()
			go drainPushes(stream, 3)
			result, err := invokeFakePeerStream(context.Background(), tc.op, stream)
			if !errors.Is(err, context.DeadlineExceeded) || result.evidence["deadline"] != tc.deadline || !strings.Contains(err.Error(), "deadline="+tc.deadline) {
				t.Fatalf("result = %#v, error = %v", result, err)
			}
		})
	}
}

func TestInvokePeerStreamFirstResponseSelectedModalityHonorsContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	stream := newFakeRelayStream()
	go drainPushes(stream, 3)
	audioDisabled := false
	result, err := invokeFakePeerStream(ctx, giztest.PeerStreamOperation{
		Mode: "text", Completion: "first_response", FirstTextTimeout: "1s", RequireAudio: &audioDisabled,
	}, stream)
	if !errors.Is(err, context.DeadlineExceeded) || result.evidence["deadline"] != "timeout" || !strings.Contains(err.Error(), "deadline=timeout") {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
}

func TestInvokePeerStreamFirstResponseDeadlines(t *testing.T) {
	for _, tc := range []struct {
		name       string
		send       func(*fakeRelayStream)
		textLimit  string
		audioLimit string
		deadline   string
	}{
		{
			name: "text", textLimit: "30ms", audioLimit: "100ms", deadline: "first_text_timeout",
			send: func(stream *fakeRelayStream) { stream.in <- assistantBlob("s1", []byte{1}, false) },
		},
		{
			name: "audio", textLimit: "100ms", audioLimit: "30ms", deadline: "first_audio_timeout",
			send: func(stream *fakeRelayStream) { stream.in <- assistantText("s1", "hello", false) },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := newFakeRelayStream()
			go func() {
				drainPushes(stream, 3)
				tc.send(stream)
			}()
			result, err := invokeFakePeerStream(context.Background(), giztest.PeerStreamOperation{
				Mode: "text", Completion: "first_response", FirstTextTimeout: tc.textLimit, FirstAudioTimeout: tc.audioLimit,
			}, stream)
			if !errors.Is(err, context.DeadlineExceeded) || result.evidence["deadline"] != tc.deadline || !strings.Contains(err.Error(), "deadline="+tc.deadline) {
				t.Fatalf("result = %#v, error = %v", result, err)
			}
		})
	}
}

func TestInvokePeerStreamFirstResponseDeadlineStartsAfterInput(t *testing.T) {
	stream := newFakeRelayStream()
	stream.pushes = make(chan *genx.MessageChunk)
	go func() {
		for range 3 {
			time.Sleep(30 * time.Millisecond)
			<-stream.pushes
		}
		stream.in <- transcriptText("user-1", "question", false)
		stream.in <- assistantText("s1", "hello", false)
	}()
	audioDisabled := false
	result, err := invokeFakePeerStream(context.Background(), giztest.PeerStreamOperation{
		Mode: "text", Completion: "first_response", FirstTextTimeout: "20ms", RequireAudio: &audioDisabled,
	}, stream)
	if err != nil {
		t.Fatalf("input time counted against first response deadline: %v", err)
	}
	if textMS := result.evidence["first_text_ms"].(int64); textMS >= 20 {
		t.Fatalf("first_text_ms = %d, want response-only latency", textMS)
	}
	if transcriptMS := result.evidence["first_transcript_ms"].(int64); transcriptMS >= 20 {
		t.Fatalf("first_transcript_ms = %d, want response-only latency", transcriptMS)
	}
}

func TestInvokePeerStreamTerminalLatencyKeepsOperationClock(t *testing.T) {
	stream := newFakeRelayStream()
	stream.pushes = make(chan *genx.MessageChunk)
	go func() {
		for range 3 {
			time.Sleep(20 * time.Millisecond)
			<-stream.pushes
		}
		finishAssistantTurn(stream, "s1")
	}()
	result, err := invokeFakePeerStream(context.Background(), giztest.PeerStreamOperation{Mode: "text"}, stream)
	if err != nil {
		t.Fatal(err)
	}
	if textMS := result.evidence["first_text_ms"].(int64); textMS < 60 {
		t.Fatalf("terminal first_text_ms = %d, want operation clock including input push", textMS)
	}
}

func TestPeerStreamFirstResponseArrivalWinsSchedulingRace(t *testing.T) {
	for _, tc := range []struct {
		name   string
		chunk  *genx.MessageChunk
		within func(*peerStreamFirstResponseArrivals, time.Duration) bool
	}{
		{name: "text", chunk: assistantText("s1", "hello", false), within: (*peerStreamFirstResponseArrivals).firstTextWithin},
		{name: "audio", chunk: assistantBlob("s1", []byte{1}, false), within: (*peerStreamFirstResponseArrivals).firstAudioWithin},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := newFakeRelayStream()
			defer func() { _ = stream.Close() }()
			arrivals := &peerStreamFirstResponseArrivals{started: time.Now()}
			next := readPeerStream(t.Context(), stream, arrivals)
			stream.in <- tc.chunk
			deadline := time.NewTimer(40 * time.Millisecond)
			defer deadline.Stop()
			for !tc.within(arrivals, 40*time.Millisecond) {
				select {
				case <-deadline.C:
					t.Fatal("reader did not record the response before its deadline")
				default:
					time.Sleep(time.Millisecond)
				}
			}
			time.Sleep(50 * time.Millisecond)
			if !tc.within(arrivals, 40*time.Millisecond) {
				t.Fatal("an on-time queued response became a scheduling timeout")
			}
			if result := <-next; result.chunk != tc.chunk {
				t.Fatalf("queued result = %#v", result)
			}
		})
	}
}

func TestInvokePeerStreamIdleTimeoutRearmsAfterInterrupt(t *testing.T) {
	first := newFakeRelayStream()
	second := newFakeRelayStream()
	go func() {
		drainPushes(first, 3)
		first.in <- assistantText("s1", "first reply", false)
		// The interrupting turn is pushed on the reopened stream; the idle
		// timer must restart from that push, not from the last first-stream event.
		drainPushes(second, 3)
		time.Sleep(110 * time.Millisecond)
		finishAssistantTurn(second, "s2")
	}()
	result, err := invokeFakePeerStream(context.Background(), giztest.PeerStreamOperation{Mode: "text", InterruptAfter: "50ms", IdleTimeout: "150ms"}, first, second)
	if err != nil {
		t.Fatalf("interrupted turn failed: %v", err)
	}
	object, _ := result.assertion.(map[string]any)
	if object["interrupted"] != true {
		t.Fatalf("assertion = %#v", object)
	}
	if result.evidence["idle_timeout_ms"] != int64(150) {
		t.Fatalf("evidence = %#v", result.evidence)
	}
}

func TestInvokePeerStreamIdleTimeoutSuspendedDuringInterruptReplacement(t *testing.T) {
	first := newFakeRelayStream()
	second := newFakeRelayStream()
	// Unbuffered pushes make the replacement turn block until the test drains
	// it, which happens only after the whole idle_timeout has elapsed.
	second.pushes = make(chan *genx.MessageChunk)
	go func() {
		drainPushes(first, 3)
		first.in <- assistantText("s1", "first reply", false)
		time.Sleep(150 * time.Millisecond)
		drainPushes(second, 3)
		finishAssistantTurn(second, "s2")
	}()
	result, err := invokeFakePeerStream(context.Background(), giztest.PeerStreamOperation{Mode: "text", InterruptAfter: "40ms", IdleTimeout: "100ms"}, first, second)
	if err != nil {
		t.Fatalf("blocked replacement push tripped the idle bound: %v", err)
	}
	if object, _ := result.assertion.(map[string]any); object["interrupted"] != true {
		t.Fatalf("assertion = %#v", object)
	}
}

func TestInvokePeerStreamIdleTimeoutAppliesAfterInterruptReplacement(t *testing.T) {
	first := newFakeRelayStream()
	second := newFakeRelayStream()
	go func() {
		drainPushes(first, 3)
		first.in <- assistantText("s1", "first reply", false)
		drainPushes(second, 3)
		// The reopened stream never answers the interrupting turn.
	}()
	result, err := invokeFakePeerStream(context.Background(), giztest.PeerStreamOperation{Mode: "text", InterruptAfter: "40ms", IdleTimeout: "80ms"}, first, second)
	if err == nil || !strings.Contains(err.Error(), "deadline=idle_timeout") || !strings.Contains(err.Error(), "interrupt_sent=true") {
		t.Fatalf("error = %v", err)
	}
	if result.evidence["deadline"] != "idle_timeout" {
		t.Fatalf("evidence = %#v", result.evidence)
	}
}

func listenStep(duration string) giztest.Step {
	return giztest.Step{ID: "listen", Client: "peer", PeerStream: &giztest.PeerStreamOperation{Mode: "listen", Duration: duration}}
}

func TestListenPeerStreamCapturesReceivedOpusFromAnyLabel(t *testing.T) {
	stream := newFakeRelayStream()
	oggAudio, packets := testOggOpus(t)
	wantBytes := 0
	for _, packet := range packets {
		wantBytes += len(packet)
	}
	stream.in <- &genx.MessageChunk{Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "remote-a", Label: "participant-a"}}
	stream.in <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: packets[0]}, Ctrl: &genx.StreamCtrl{StreamID: "remote-a", Label: "participant-a"}}
	stream.in <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: packets[1]}, Ctrl: &genx.StreamCtrl{StreamID: "remote-b", Label: "participant-b"}}
	stream.in <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "text/plain"}, Ctrl: &genx.StreamCtrl{StreamID: "remote-a", Label: "participant-a", EndOfStream: true}}
	started := time.Now()
	result, err := listenPeerStream(context.Background(), stream, listenStep("250ms"), len(oggAudio)*4)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 250*time.Millisecond || elapsed > 3*time.Second {
		t.Fatalf("listen returned after %s, want the declared duration", elapsed)
	}
	object := result.assertion.(map[string]any)
	if object["audio_bytes"] != wantBytes || object["packets"] != 2 || object["events"] != 4 || object["streams"] != 2 {
		t.Fatalf("listen result = %#v", object)
	}
	if first, _ := object["first_audio_ms"].(int64); first < 1 {
		t.Fatalf("first_audio_ms = %v", object["first_audio_ms"])
	}
	if texts, _ := object["text"].([]string); len(texts) != 1 {
		t.Fatalf("text fragments = %#v", object["text"])
	}
	audio, ok := object["audio"].([]byte)
	if !ok || !bytes.HasPrefix(audio, []byte("OggS")) {
		t.Fatalf("captured audio is not Ogg: %#v", object["audio"])
	}
	decoded, err := decodeOpusPackets(audio)
	if err != nil || len(decoded) != 2 {
		t.Fatalf("captured Ogg decodes to %d packets, err %v", len(decoded), err)
	}
	if result.evidence["audio_bytes"] != wantBytes || result.evidence["mode"] != "listen" || result.evidence["duration_ms"] != int64(250) {
		t.Fatalf("listen evidence = %#v", result.evidence)
	}
	if _, present := result.evidence["audio"]; present {
		t.Fatal("listen evidence leaks audio payload")
	}
	if _, present := object["audio_pacing"]; !present {
		t.Fatalf("listen result lacks audio_pacing: %#v", object)
	}
}

func TestListenPeerStreamAcceptsSilence(t *testing.T) {
	stream := newFakeRelayStream()
	result, err := listenPeerStream(context.Background(), stream, listenStep("50ms"), 4096)
	if err != nil {
		t.Fatal(err)
	}
	object := result.assertion.(map[string]any)
	if object["audio_bytes"] != 0 || object["packets"] != 0 || object["events"] != 0 {
		t.Fatalf("silent listen result = %#v", object)
	}
	if _, present := object["audio"]; present {
		t.Fatal("silent listen produced audio")
	}
	if _, present := object["audio_pacing"]; present {
		t.Fatal("silent listen produced audio_pacing")
	}
}

func TestListenPeerStreamEnforcesCaptureBound(t *testing.T) {
	stream := newFakeRelayStream()
	_, packets := testOggOpus(t)
	for _, packet := range packets {
		stream.in <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: packet}, Ctrl: &genx.StreamCtrl{StreamID: "remote", Label: "participant"}}
	}
	_, err := listenPeerStream(context.Background(), stream, listenStep("1s"), len(packets[0]))
	if err == nil || !strings.Contains(err.Error(), "exceeds output variable max_bytes") {
		t.Fatalf("capture bound error = %v", err)
	}
}

func TestListenPeerStreamFailsOnEarlyCloseAndCancellation(t *testing.T) {
	closed := newFakeRelayStream()
	close(closed.in)
	result, err := listenPeerStream(context.Background(), closed, listenStep("1s"), 0)
	if err == nil || !strings.Contains(err.Error(), "closed before the listen duration") || result.evidence["events"] != 0 {
		t.Fatalf("early close error = %v, evidence %#v", err, result.evidence)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	result, err = listenPeerStream(ctx, newFakeRelayStream(), listenStep("10s"), 0)
	if !errors.Is(err, context.DeadlineExceeded) || result.evidence["deadline"] != "timeout" {
		t.Fatalf("cancellation error = %v, evidence %#v", err, result.evidence)
	}
}

func TestInvokePeerStreamListenModeUsesListenPath(t *testing.T) {
	stream := newFakeRelayStream()
	result, err := invokePeerStream(context.Background(), nil, func() (peerStream, error) { return stream, nil }, listenStep("20ms"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.evidence["mode"] != "listen" || len(stream.pushes) != 0 {
		t.Fatalf("listen pushed input or used the wrong path: %#v pushes=%d", result.evidence, len(stream.pushes))
	}
	select {
	case <-stream.closed:
	default:
		t.Fatal("listen stream was not closed")
	}
}

func TestInvokePeerStreamInputSentCompletesAfterEOS(t *testing.T) {
	stream := newFakeRelayStream()
	oggAudio, packets := testOggOpus(t)
	step := giztest.Step{ID: "turn", Client: "peer", PeerStream: &giztest.PeerStreamOperation{Mode: "push-to-talk", Completion: "input_sent"}}
	started := time.Now()
	result, err := invokePeerStream(context.Background(), nil, func() (peerStream, error) { return stream, nil }, step, oggAudio, 0)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("input_sent waited for output")
	}
	if got := len(stream.pushes); got != len(packets)+2 {
		t.Fatalf("pushed chunks = %d, want BOS + %d packets + EOS", got, len(packets))
	}
	var last *genx.MessageChunk
	for len(stream.pushes) > 0 {
		last = <-stream.pushes
	}
	if !last.IsEndOfStream() {
		t.Fatal("input_sent returned before the EOS was pushed")
	}
	object := result.assertion.(map[string]any)
	if object["input_sent"] != true || object["input_packets"] != len(packets) || object["input_ms"] != int64(40) || object["pushed_packets"] != len(packets) || object["audio_bytes"] != 0 {
		t.Fatalf("input_sent result = %#v", object)
	}
	if result.evidence["input_packets"] != len(packets) || result.evidence["input_ms"] != int64(40) {
		t.Fatalf("input_sent evidence = %#v", result.evidence)
	}
}

func TestInvokePeerStreamInputSentRecordsAlreadyArrivedOutput(t *testing.T) {
	stream := newFakeRelayStream()
	oggAudio, packets := testOggOpus(t)
	stream.in <- assistantText("s1", "early", false)
	pushed := make(chan int, 1)
	go func() {
		count := 0
		for {
			select {
			case <-stream.pushes:
				count++
			case <-stream.closed:
				for len(stream.pushes) > 0 {
					<-stream.pushes
					count++
				}
				pushed <- count
				return
			}
		}
	}()
	// Pacing keeps the push loop busy long enough for the reader to deliver
	// the queued assistant text before the last packet is on the wire.
	step := giztest.Step{ID: "turn", Client: "peer", PeerStream: &giztest.PeerStreamOperation{Mode: "realtime", Completion: "input_sent", Pacing: "2ms"}}
	result, err := invokePeerStream(context.Background(), nil, func() (peerStream, error) { return stream, nil }, step, oggAudio, 0)
	if err != nil {
		t.Fatal(err)
	}
	object := result.assertion.(map[string]any)
	if got := <-pushed; got != object["pushed_packets"].(int)+1 {
		t.Fatalf("pushed chunks = %d, want BOS + %v realtime packets without EOS", got, object["pushed_packets"])
	}
	if object["input_packets"] != len(packets) || object["pushed_packets"].(int) <= len(packets) {
		t.Fatalf("realtime input_sent result = %#v", object)
	}
	if texts, _ := object["text"].([]string); len(texts) != 1 || texts[0] != "early" {
		t.Fatalf("already arrived output = %#v", object["text"])
	}
}
