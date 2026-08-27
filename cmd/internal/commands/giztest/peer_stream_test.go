package giztest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

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
	vars := &variables{values: map[string]value{
		"audio": {spec: VariableSpec{Direction: "output", Type: "audio", MaxBytes: 4096}},
	}}
	for _, tc := range []struct {
		name string
		step Step
		want int
	}{
		{name: "streamed without buffering", step: Step{}, want: 0},
		{name: "explicit audio capture", step: Step{Capture: map[string]string{"audio": "/audio"}}, want: 4096},
		{name: "unrelated capture", step: Step{Capture: map[string]string{"audio": "/text"}}, want: 0},
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

func invokeFakePeerStream(ctx context.Context, op PeerStreamOperation, streams ...*fakeRelayStream) (operationResult, error) {
	index := 0
	open := func() (peerStream, error) {
		if index >= len(streams) {
			return nil, fmt.Errorf("unexpected PeerStream open %d", index)
		}
		stream := streams[index]
		index++
		return stream, nil
	}
	return invokePeerStream(ctx, nil, open, Step{ID: "turn", PeerStream: &op}, "hello", 0)
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
	result, err := invokeFakePeerStream(context.Background(), PeerStreamOperation{Mode: "text", IdleTimeout: "120ms"}, stream)
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
	result, err := invokeFakePeerStream(context.Background(), PeerStreamOperation{Mode: "text", IdleTimeout: "50ms"}, stream)
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
	result, err := invokeFakePeerStream(ctx, PeerStreamOperation{Mode: "text", IdleTimeout: "1s"}, stream)
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
	result, err := invokeFakePeerStream(context.Background(), PeerStreamOperation{Mode: "text"}, stream)
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
		stream.in <- assistantText("s1", "hello", false)
		stream.in <- assistantBlob("s1", []byte{1, 2, 3}, false)
	}()
	result, err := invokeFakePeerStream(context.Background(), PeerStreamOperation{
		Mode: "text", Completion: "first_response", FirstTextTimeout: "100ms", FirstAudioTimeout: "150ms",
	}, stream)
	if err != nil {
		t.Fatalf("first response failed: %v", err)
	}
	object, _ := result.assertion.(map[string]any)
	if object["text_eos"] != false || object["audio_eos"] != false {
		t.Fatalf("first response waited for terminal output: %#v", object)
	}
	if object["events"] != 2 || object["first_text_ms"] == nil || object["first_audio_ms"] == nil {
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
		op   PeerStreamOperation
		send func(*fakeRelayStream)
	}{
		{
			name: "text only",
			op: PeerStreamOperation{
				Mode: "text", Completion: "first_response", FirstTextTimeout: "100ms",
				RequireText: &textRequired, RequireAudio: &audioDisabled,
			},
			send: func(stream *fakeRelayStream) { stream.in <- assistantText("s1", "hello", false) },
		},
		{
			name: "audio only",
			op: PeerStreamOperation{
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
		})
	}
}

func TestInvokePeerStreamFirstResponseSelectedModalityDeadlines(t *testing.T) {
	textRequired, audioRequired := true, true
	textDisabled, audioDisabled := false, false
	for _, tc := range []struct {
		name     string
		op       PeerStreamOperation
		deadline string
	}{
		{
			name: "text only",
			op: PeerStreamOperation{
				Mode: "text", Completion: "first_response", FirstTextTimeout: "30ms",
				RequireText: &textRequired, RequireAudio: &audioDisabled,
			},
			deadline: "first_text_timeout",
		},
		{
			name: "audio only",
			op: PeerStreamOperation{
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
	result, err := invokeFakePeerStream(ctx, PeerStreamOperation{
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
			result, err := invokeFakePeerStream(context.Background(), PeerStreamOperation{
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
		stream.in <- assistantText("s1", "hello", false)
	}()
	audioDisabled := false
	result, err := invokeFakePeerStream(context.Background(), PeerStreamOperation{
		Mode: "text", Completion: "first_response", FirstTextTimeout: "20ms", RequireAudio: &audioDisabled,
	}, stream)
	if err != nil {
		t.Fatalf("input time counted against first response deadline: %v", err)
	}
	if textMS := result.evidence["first_text_ms"].(int64); textMS >= 20 {
		t.Fatalf("first_text_ms = %d, want response-only latency", textMS)
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
	result, err := invokeFakePeerStream(context.Background(), PeerStreamOperation{Mode: "text"}, stream)
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
	result, err := invokeFakePeerStream(context.Background(), PeerStreamOperation{Mode: "text", InterruptAfter: "50ms", IdleTimeout: "150ms"}, first, second)
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
	result, err := invokeFakePeerStream(context.Background(), PeerStreamOperation{Mode: "text", InterruptAfter: "40ms", IdleTimeout: "100ms"}, first, second)
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
	result, err := invokeFakePeerStream(context.Background(), PeerStreamOperation{Mode: "text", InterruptAfter: "40ms", IdleTimeout: "80ms"}, first, second)
	if err == nil || !strings.Contains(err.Error(), "deadline=idle_timeout") || !strings.Contains(err.Error(), "interrupt_sent=true") {
		t.Fatalf("error = %v", err)
	}
	if result.evidence["deadline"] != "idle_timeout" {
		t.Fatalf("evidence = %#v", result.evidence)
	}
}
