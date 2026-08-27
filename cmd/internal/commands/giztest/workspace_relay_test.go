package giztest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type fakeRelayStream struct {
	in           chan *genx.MessageChunk
	pushes       chan *genx.MessageChunk
	closeOnce    sync.Once
	closed       chan struct{}
	nextExitGate <-chan struct{}
}

func newFakeRelayStream() *fakeRelayStream {
	return &fakeRelayStream{in: make(chan *genx.MessageChunk, 64), pushes: make(chan *genx.MessageChunk, 64), closed: make(chan struct{})}
}

func (f *fakeRelayStream) Next() (*genx.MessageChunk, error) {
	select {
	case chunk, ok := <-f.in:
		if !ok {
			return nil, io.EOF
		}
		return chunk, nil
	case <-f.closed:
		if f.nextExitGate != nil {
			<-f.nextExitGate
		}
		return nil, io.EOF
	}
}

func (f *fakeRelayStream) Push(ctx context.Context, chunk *genx.MessageChunk) error {
	select {
	case f.pushes <- chunk:
		return nil
	case <-f.closed:
		return fmt.Errorf("push on closed stream")
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (f *fakeRelayStream) Close() error {
	f.closeOnce.Do(func() { close(f.closed) })
	return nil
}

// settle waits until the relay has drained this fake stream's queue, so a
// test can order events across the two independent stream channels.
func (f *fakeRelayStream) settle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for len(f.in) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("relay did not drain the stream queue")
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
}

func assistantText(id, text string, eos bool) *genx.MessageChunk {
	return &genx.MessageChunk{Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: id, Label: "assistant", EndOfStream: eos}}
}

func assistantBlob(id string, data []byte, eos bool) *genx.MessageChunk {
	return &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: data}, Ctrl: &genx.StreamCtrl{StreamID: id, Label: "assistant", EndOfStream: eos}}
}

func nextPush(t *testing.T, stream *fakeRelayStream) *genx.MessageChunk {
	t.Helper()
	select {
	case chunk := <-stream.pushes:
		return chunk
	case <-time.After(2 * time.Second):
		t.Fatal("no forwarded chunk arrived")
		return nil
	}
}

func expectUserText(t *testing.T, stream *fakeRelayStream, text string, eos bool) string {
	t.Helper()
	chunk := nextPush(t, stream)
	if chunk.Role != genx.RoleUser || chunk.Ctrl == nil || chunk.Ctrl.Label != "user" || chunk.Ctrl.StreamID == "" {
		t.Fatalf("forwarded chunk is not receiving-side user input: %#v", chunk)
	}
	if got, _ := chunk.Part.(genx.Text); string(got) != text || chunk.IsEndOfStream() != eos {
		t.Fatalf("forwarded chunk = %q eos=%v, want %q eos=%v", string(got), chunk.IsEndOfStream(), text, eos)
	}
	return chunk.Ctrl.StreamID
}

func drainUserTurn(t *testing.T, stream *fakeRelayStream) []string {
	t.Helper()
	var fragments []string
	for {
		chunk := nextPush(t, stream)
		if chunk.Ctrl == nil || chunk.Ctrl.Label != "user" {
			t.Fatalf("expected user chunk, got %#v", chunk)
		}
		if text, ok := chunk.Part.(genx.Text); ok && string(text) != "" {
			fragments = append(fragments, string(text))
		}
		if chunk.IsEndOfStream() {
			return fragments
		}
	}
}

func textRelayOperation(maxTurns int) *WorkspaceRelayOperation {
	terminal := "tester"
	if maxTurns%2 == 0 {
		terminal = "candidate"
	}
	return &WorkspaceRelayOperation{FirstClient: "tester", SecondClient: "candidate", Media: "text", MaxTurns: maxTurns, TerminalClient: terminal}
}

func TestWorkspaceRelayForwardsTextIncrementally(t *testing.T) {
	tester, candidate := newFakeRelayStream(), newFakeRelayStream()
	op := textRelayOperation(3)
	type outcome struct {
		result operationResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := runWorkspaceRelay(context.Background(), op, tester, candidate, "brief", 0)
		done <- outcome{result, err}
	}()
	// The relay must first deliver the initial input to the tester side.
	if chunk := nextPush(t, tester); !chunk.Ctrl.BeginOfStream {
		t.Fatalf("initial input missing BOS: %#v", chunk)
	}
	inputID := expectUserText(t, tester, "brief", false)
	expectUserText(t, tester, "", true)
	// Tester turn 1 streams two fragments; the first must reach the candidate
	// before the tester's EOS is even produced.
	tester.in <- assistantText("t1", "open ", false)
	if chunk := nextPush(t, candidate); !chunk.Ctrl.BeginOfStream {
		t.Fatalf("forwarded turn missing BOS: %#v", chunk)
	}
	forwardID := expectUserText(t, candidate, "open ", false)
	if forwardID == inputID || forwardID == "t1" {
		t.Fatalf("forwarded stream ID %q was not rewritten", forwardID)
	}
	tester.in <- assistantText("t1", "question", false)
	expectUserText(t, candidate, "question", false)
	tester.in <- assistantText("t1", "", true)
	expectUserText(t, candidate, "", true)
	// Candidate turn 2 answers; the relay forwards it back to the tester.
	candidate.in <- assistantText("c1", "answer", true)
	if chunk := nextPush(t, tester); !chunk.Ctrl.BeginOfStream {
		t.Fatalf("return turn missing BOS: %#v", chunk)
	}
	expectUserText(t, tester, "answer", false)
	expectUserText(t, tester, "", true)
	// Tester turn 3 is terminal: captured, never forwarded again.
	tester.in <- assistantText("t2", "PASS", false)
	tester.in <- assistantText("t2", "", true)
	got := <-done
	if got.err != nil {
		t.Fatalf("relay error = %v", got.err)
	}
	assertion := got.result.assertion.(map[string]any)
	if assertion["completed_turns"] != 3 {
		t.Fatalf("completed_turns = %v", assertion["completed_turns"])
	}
	terminal := assertion["terminal"].(map[string]any)
	if terminal["client"] != "tester" || terminal["text"] != "PASS" {
		t.Fatalf("terminal = %#v", terminal)
	}
	turns := assertion["turns"].(map[string]any)
	testerTurns := turns["tester"].(map[string]any)
	candidateTurns := turns["candidate"].(map[string]any)
	if testerTurns["count"] != 2 || candidateTurns["count"] != 1 {
		t.Fatalf("turn counts = %#v / %#v", testerTurns, candidateTurns)
	}
	runes := testerTurns["text_runes"].(map[string]any)
	if runes["min"] != int64(4) || runes["max"] != int64(13) {
		t.Fatalf("tester text_runes = %#v", runes)
	}
	if _, ok := testerTurns["first_text_ms"].(map[string]any); !ok {
		t.Fatalf("tester first_text_ms missing: %#v", testerTurns)
	}
	select {
	case chunk := <-candidate.pushes:
		t.Fatalf("terminal turn was forwarded: %#v", chunk)
	default:
	}
}

func TestWorkspaceRelayEvidenceExcludesContent(t *testing.T) {
	tester, candidate := newFakeRelayStream(), newFakeRelayStream()
	op := textRelayOperation(2)
	done := make(chan operationResult, 1)
	go func() {
		result, err := runWorkspaceRelay(context.Background(), op, tester, candidate, "secret brief", 0)
		if err != nil {
			t.Errorf("relay error = %v", err)
		}
		done <- result
	}()
	drainUserTurn(t, tester)
	tester.in <- assistantText("t1", "secret question", true)
	drainUserTurn(t, candidate)
	candidate.in <- assistantText("c1", "SECRET VERDICT", true)
	result := <-done
	evidence, err := json.Marshal(result.evidence)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"secret", "SECRET", "brief", "question", "VERDICT"} {
		if strings.Contains(string(evidence), banned) {
			t.Fatalf("evidence leaks content: %s", evidence)
		}
	}
	if result.evidence["terminal_client"] != "candidate" || result.evidence["completed_turns"] != 2 {
		t.Fatalf("evidence = %#v", result.evidence)
	}
}

func TestWorkspaceRelayObservesActiveAssistantOpus(t *testing.T) {
	tester, candidate := newFakeRelayStream(), newFakeRelayStream()
	op := textRelayOperation(1)
	op.TerminalMedia = "audio"
	oggAudio, packets := testOggOpus(t)
	type observation struct {
		client string
		role   string
		packet []byte
	}
	var observations []observation
	done := make(chan error, 1)
	go func() {
		_, err := runWorkspaceRelayWithEvidence(context.Background(), op, tester, candidate, "brief", 0, false, func(client, role string, packet []byte, _ bool) error {
			if len(packet) > 0 {
				observations = append(observations, observation{client: client, role: role, packet: packet})
			}
			return nil
		})
		done <- err
	}()
	drainUserTurn(t, tester)
	tester.in <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/ogg; codecs=opus", Data: oggAudio}, Ctrl: &genx.StreamCtrl{StreamID: "t1", Label: "assistant"}}
	tester.in <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/ogg; codecs=opus"}, Ctrl: &genx.StreamCtrl{StreamID: "t1", Label: "assistant", EndOfStream: true}}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(observations) != len(packets) || observations[0].client != "tester" || observations[0].role != "assistant" || !bytes.Equal(observations[0].packet, packets[0]) {
		t.Fatalf("observations = %#v", observations)
	}
}

func TestWorkspaceRelayFullEvidenceIncludesBoundedTexts(t *testing.T) {
	tester, candidate := newFakeRelayStream(), newFakeRelayStream()
	op := textRelayOperation(2)
	done := make(chan operationResult, 1)
	go func() {
		result, err := runWorkspaceRelayWithEvidence(context.Background(), op, tester, candidate, "private input", 0, true)
		if err != nil {
			t.Errorf("relay error = %v", err)
		}
		done <- result
	}()
	drainUserTurn(t, tester)
	tester.in <- assistantText("t1", "question", true)
	drainUserTurn(t, candidate)
	candidate.in <- assistantText("c1", "FAIL: actual answer", true)
	evidence := (<-done).evidence
	terminal := evidence["terminal"].(map[string]any)
	if terminal["text"] != "FAIL: actual answer" {
		t.Fatalf("terminal evidence = %#v", terminal)
	}
	turns := evidence["turns"].(map[string]any)
	texts := turns["candidate"].(map[string]any)["texts"].([]any)
	if len(texts) != 1 || texts[0] != "FAIL: actual answer" {
		t.Fatalf("candidate texts = %#v", texts)
	}
	data, _ := json.Marshal(evidence)
	if strings.Contains(string(data), "private input") {
		t.Fatalf("full evidence leaks initial input: %s", data)
	}
}

func TestWorkspaceRelayIdleTimeoutAttributesActiveTurn(t *testing.T) {
	tester, candidate := newFakeRelayStream(), newFakeRelayStream()
	op := textRelayOperation(3)
	op.IdleTimeout = "40ms"
	done := make(chan struct {
		result operationResult
		err    error
	}, 1)
	go func() {
		result, err := runWorkspaceRelay(context.Background(), op, tester, candidate, "brief", 0)
		done <- struct {
			result operationResult
			err    error
		}{result, err}
	}()
	drainUserTurn(t, tester)
	got := <-done
	_ = tester.Close()
	_ = candidate.Close()
	if got.err == nil || !errors.Is(got.err, context.DeadlineExceeded) || !strings.Contains(got.err.Error(), "client tester turn 1") {
		t.Fatalf("error = %v", got.err)
	}
	if got.result.evidence["deadline"] != "idle_timeout" || got.result.evidence["active_client"] != "tester" || got.result.evidence["active_turn"] != 1 {
		t.Fatalf("evidence = %#v", got.result.evidence)
	}
	if got.result.evidence["idle_timeout_ms"] != int64(40) || got.result.evidence["observed_text"] != false {
		t.Fatalf("evidence = %#v", got.result.evidence)
	}
}

func TestWorkspaceRelayIdleTimeoutResetsOnActiveProgress(t *testing.T) {
	tester, candidate := newFakeRelayStream(), newFakeRelayStream()
	op := textRelayOperation(2)
	op.IdleTimeout = "100ms"
	done := make(chan error, 1)
	go func() {
		_, err := runWorkspaceRelay(context.Background(), op, tester, candidate, "brief", 0)
		done <- err
	}()
	drainUserTurn(t, tester)
	time.Sleep(60 * time.Millisecond)
	tester.in <- assistantText("t1", "still working", false)
	if chunk := nextPush(t, candidate); !chunk.Ctrl.BeginOfStream {
		t.Fatalf("forwarded turn missing BOS: %#v", chunk)
	}
	expectUserText(t, candidate, "still working", false)
	time.Sleep(60 * time.Millisecond)
	tester.in <- assistantText("t1", "", true)
	expectUserText(t, candidate, "", true)
	candidate.in <- assistantText("c1", "PASS", true)
	if err := <-done; err != nil {
		t.Fatalf("relay failed after active progress reset: %v", err)
	}
}

func TestWorkspaceRelayIdleTimeoutIgnoresInactiveTraffic(t *testing.T) {
	tester, candidate := newFakeRelayStream(), newFakeRelayStream()
	op := textRelayOperation(2)
	op.IdleTimeout = "80ms"
	done := make(chan error, 1)
	go func() {
		_, err := runWorkspaceRelay(context.Background(), op, tester, candidate, "brief", 0)
		done <- err
	}()
	drainUserTurn(t, tester)

	deadline := time.Now().Add(160 * time.Millisecond)
	for time.Now().Before(deadline) {
		candidate.in <- &genx.MessageChunk{Ctrl: &genx.StreamCtrl{Label: "control"}}
		time.Sleep(15 * time.Millisecond)
	}
	if err := <-done; err == nil || !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "client tester turn 1") {
		t.Fatalf("inactive traffic extended idle deadline: %v", err)
	}
}

func TestWorkspaceRelayIdleTimeoutIgnoresDiscardedControlStream(t *testing.T) {
	tester, candidate := newFakeRelayStream(), newFakeRelayStream()
	op := textRelayOperation(3)
	op.IdleTimeout = "80ms"
	done := make(chan error, 1)
	go func() {
		_, err := runWorkspaceRelay(context.Background(), op, tester, candidate, "brief", 0)
		done <- err
	}()
	drainUserTurn(t, tester)

	candidate.in <- assistantText("self-start", "discard me", false)
	candidate.settle(t)
	tester.in <- assistantText("t1", "question", true)
	drainUserTurn(t, candidate)

	deadline := time.Now().Add(160 * time.Millisecond)
	for time.Now().Before(deadline) {
		candidate.in <- &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "self-start", Label: "control"}}
		time.Sleep(15 * time.Millisecond)
	}
	if err := <-done; err == nil || !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "client candidate turn 2") {
		t.Fatalf("discarded control stream extended idle deadline: %v", err)
	}
}

func TestWorkspaceRelayTextCompletesOnAudioTerminal(t *testing.T) {
	tester, candidate := newFakeRelayStream(), newFakeRelayStream()
	op := textRelayOperation(2)
	op.TerminalMedia = "audio"
	done := make(chan operationResult, 1)
	go func() {
		result, err := runWorkspaceRelay(context.Background(), op, tester, candidate, "brief", 0)
		if err != nil {
			t.Errorf("relay error = %v", err)
		}
		done <- result
	}()
	drainUserTurn(t, tester)
	tester.in <- assistantText("t1", "question", false)
	if chunk := nextPush(t, candidate); !chunk.Ctrl.BeginOfStream {
		t.Fatalf("forwarded turn missing BOS: %#v", chunk)
	}
	expectUserText(t, candidate, "question", false)
	tester.in <- assistantText("t1", "", true)
	tester.settle(t)
	select {
	case chunk := <-candidate.pushes:
		t.Fatalf("text EOS completed an audio-terminal turn: %#v", chunk)
	default:
	}
	tester.in <- assistantBlob("t1", nil, true)
	expectUserText(t, candidate, "", true)
	candidate.in <- assistantText("c1", "answer", false)
	candidate.in <- assistantBlob("c1", nil, true)
	result := <-done
	terminal := result.assertion.(map[string]any)["terminal"].(map[string]any)
	if terminal["text"] != "answer" {
		t.Fatalf("terminal = %#v", terminal)
	}
	if result.evidence["completed_turns"] != 2 {
		t.Fatalf("evidence = %#v", result.evidence)
	}
}

func TestWorkspaceRelayFailsOnDisconnectAndStreamError(t *testing.T) {
	t.Run("disconnect", func(t *testing.T) {
		tester, candidate := newFakeRelayStream(), newFakeRelayStream()
		done := make(chan error, 1)
		go func() {
			_, err := runWorkspaceRelay(context.Background(), textRelayOperation(3), tester, candidate, "brief", 0)
			done <- err
		}()
		drainUserTurn(t, tester)
		_ = tester.Close()
		err := <-done
		if err == nil || !strings.Contains(err.Error(), "client tester") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("terminal stream error", func(t *testing.T) {
		tester, candidate := newFakeRelayStream(), newFakeRelayStream()
		done := make(chan error, 1)
		go func() {
			_, err := runWorkspaceRelay(context.Background(), textRelayOperation(3), tester, candidate, "brief", 0)
			done <- err
		}()
		drainUserTurn(t, tester)
		tester.in <- &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "t1", Label: "assistant", Error: "workflow exploded", EndOfStream: true}}
		err := <-done
		if err == nil || !strings.Contains(err.Error(), "terminal stream error") || strings.Contains(err.Error(), "exploded") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestWorkspaceRelayEnforcesFixedLimits(t *testing.T) {
	t.Run("text bytes", func(t *testing.T) {
		tester, candidate := newFakeRelayStream(), newFakeRelayStream()
		done := make(chan error, 1)
		go func() {
			_, err := runWorkspaceRelay(context.Background(), textRelayOperation(3), tester, candidate, "brief", 0)
			done <- err
		}()
		drainUserTurn(t, tester)
		go func() {
			for range 3 {
				select {
				case tester.in <- assistantText("t1", strings.Repeat("x", 512*1024), false):
				case <-tester.closed:
					return
				}
			}
		}()
		go func() { // Keep the receiving side drained so forwarding cannot block.
			for {
				select {
				case <-candidate.pushes:
				case <-candidate.closed:
					return
				}
			}
		}()
		err := <-done
		_ = tester.Close()
		_ = candidate.Close()
		if err == nil || !strings.Contains(err.Error(), "text limit") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("events", func(t *testing.T) {
		tester, candidate := newFakeRelayStream(), newFakeRelayStream()
		done := make(chan error, 1)
		go func() {
			_, err := runWorkspaceRelay(context.Background(), textRelayOperation(3), tester, candidate, "brief", 0)
			done <- err
		}()
		drainUserTurn(t, tester)
		go func() {
			for range relayMaxTurnEvents + 1 {
				select {
				case tester.in <- &genx.MessageChunk{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "t1", Label: "transcript"}}:
				case <-tester.closed:
					return
				}
			}
		}()
		err := <-done
		_ = tester.Close()
		if err == nil || !strings.Contains(err.Error(), "event") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestWorkspaceRelayCancellationClosesCleanly(t *testing.T) {
	tester, candidate := newFakeRelayStream(), newFakeRelayStream()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := runWorkspaceRelay(ctx, textRelayOperation(3), tester, candidate, "brief", 0)
		done <- err
	}()
	drainUserTurn(t, tester)
	cancel()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "cancelled") {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("relay did not stop on cancellation")
	}
}

func TestWorkspaceRelayForwardsAudioIncrementally(t *testing.T) {
	tester, candidate := newFakeRelayStream(), newFakeRelayStream()
	op := textRelayOperation(3)
	op.Media = "audio"
	type outcome struct {
		result operationResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := runWorkspaceRelay(context.Background(), op, tester, candidate, []byte{0x11, 0x22}, 1<<20)
		done <- outcome{result, err}
	}()
	// Initial push-to-talk input: BOS, one packet, EOS.
	if chunk := nextPush(t, tester); !chunk.Ctrl.BeginOfStream {
		t.Fatalf("input missing BOS: %#v", chunk)
	}
	if chunk := nextPush(t, tester); !bytes.Equal(chunk.Part.(*genx.Blob).Data, []byte{0x11, 0x22}) {
		t.Fatalf("input packet = %#v", chunk)
	}
	if chunk := nextPush(t, tester); !chunk.IsEndOfStream() {
		t.Fatalf("input missing EOS: %#v", chunk)
	}
	// Tester audio turn: the first packet is forwarded before the source EOS.
	tester.in <- assistantText("t1-text", "question", true)
	tester.in <- assistantBlob("t1", []byte{0xAA}, false)
	if chunk := nextPush(t, candidate); !chunk.Ctrl.BeginOfStream || chunk.Ctrl.Label != "user" {
		t.Fatalf("forwarded audio missing user BOS: %#v", chunk)
	}
	forwarded := nextPush(t, candidate)
	if forwarded.Role != genx.RoleUser || !bytes.Equal(forwarded.Part.(*genx.Blob).Data, []byte{0xAA}) || forwarded.Ctrl.StreamID == "t1" {
		t.Fatalf("forwarded packet = %#v", forwarded)
	}
	tester.in <- assistantBlob("t1", nil, true)
	if chunk := nextPush(t, candidate); !chunk.IsEndOfStream() {
		t.Fatalf("forwarded audio missing EOS: %#v", chunk)
	}
	// Candidate answers with one packet.
	candidate.in <- assistantText("c1-text", "answer", true)
	candidate.in <- assistantBlob("c1", []byte{0xBB}, false)
	nextPush(t, tester)
	nextPush(t, tester)
	candidate.in <- assistantBlob("c1", nil, true)
	nextPush(t, tester)
	// Terminal tester audio turn is captured, not forwarded.
	tester.in <- assistantText("t2-text", "PASS", true)
	tester.in <- assistantBlob("t2", []byte{0xCC, 0xDD}, false)
	tester.in <- assistantBlob("t2", nil, true)
	got := <-done
	if got.err != nil {
		t.Fatalf("relay error = %v", got.err)
	}
	assertion := got.result.assertion.(map[string]any)
	terminal := assertion["terminal"].(map[string]any)
	audio, ok := terminal["audio"].([]byte)
	if !ok || !bytes.HasPrefix(audio, []byte("OggS")) {
		t.Fatalf("terminal audio = %#v", terminal["audio"])
	}
	if terminal["text"] != "PASS" {
		t.Fatalf("terminal text = %#v", terminal)
	}
	turns := assertion["turns"].(map[string]any)
	testerTurns := turns["tester"].(map[string]any)
	testerTexts := testerTurns["texts"].([]any)
	if len(testerTexts) != 2 || testerTexts[0] != "question" || testerTexts[1] != "PASS" {
		t.Fatalf("tester texts = %#v", testerTexts)
	}
	audioAgg := testerTurns["audio_bytes"].(map[string]any)
	if audioAgg["min"] != int64(1) || audioAgg["max"] != int64(2) {
		t.Fatalf("tester audio_bytes = %#v", audioAgg)
	}
	select {
	case chunk := <-candidate.pushes:
		t.Fatalf("terminal audio was forwarded: %#v", chunk)
	default:
	}
}

func TestWorkspaceRelayDiscardsSelfStartRepliesAndInterruptions(t *testing.T) {
	tester, candidate := newFakeRelayStream(), newFakeRelayStream()
	op := textRelayOperation(3)
	type outcome struct {
		result operationResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := runWorkspaceRelay(context.Background(), op, tester, candidate, "brief", 0)
		done <- outcome{result, err}
	}()
	drainUserTurn(t, tester)
	// The candidate self-starts a greeting before its first turn: fragments,
	// EOS, and an interrupted marker must all be discarded without failing.
	candidate.in <- assistantText("greet", "自我开场白", false)
	candidate.in <- assistantText("greet", "", true)
	candidate.in <- &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "greet", Label: "assistant", Error: "interrupted", EndOfStream: true}}
	candidate.settle(t)
	// Turn 1: tester responds and the relay forwards it.
	tester.in <- assistantText("t1", "question", true)
	drainUserTurn(t, candidate)
	// Turn 2: candidate's first real turn.
	candidate.in <- assistantText("c1", "answer", true)
	drainUserTurn(t, tester)
	// After its first activation, inactive candidate output is a hard failure.
	tester.in <- assistantText("t2", "PASS", true)
	got := <-done
	if got.err != nil {
		t.Fatalf("relay error = %v", got.err)
	}
	assertion := got.result.assertion.(map[string]any)
	if assertion["completed_turns"] != 3 {
		t.Fatalf("completed_turns = %v", assertion["completed_turns"])
	}
	terminal := assertion["terminal"].(map[string]any)
	if terminal["text"] != "PASS" {
		t.Fatalf("terminal = %#v", terminal)
	}
	candidateTurns := assertion["turns"].(map[string]any)["candidate"].(map[string]any)
	if candidateTurns["count"] != 1 {
		t.Fatalf("self-start reply was counted as a turn: %#v", candidateTurns)
	}
}

func TestWorkspaceRelayRejectsInactiveOutputAfterFirstActivation(t *testing.T) {
	tester, candidate := newFakeRelayStream(), newFakeRelayStream()
	op := textRelayOperation(5)
	done := make(chan error, 1)
	go func() {
		_, err := runWorkspaceRelay(context.Background(), op, tester, candidate, "brief", 0)
		done <- err
	}()
	drainUserTurn(t, tester)
	tester.in <- assistantText("t1", "question", true)
	drainUserTurn(t, candidate)
	candidate.in <- assistantText("c1", "answer", true)
	drainUserTurn(t, tester)
	// Candidate already held a turn; more candidate text while the tester is
	// active is turn mixing and must fail.
	candidate.in <- assistantText("c2", "mixed", false)
	err := <-done
	if err == nil || !strings.Contains(err.Error(), "client candidate") || !strings.Contains(err.Error(), "inactive") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "mixed") {
		t.Fatalf("error leaks content: %v", err)
	}
}

func TestWorkspaceRelayInterruptedSelfStartNeverCompletesATurn(t *testing.T) {
	// Replays the live failure: the candidate self-starts (BOS + empty
	// fragment) during turn 1; after the handoff its interrupted terminal
	// arrives while the candidate is ACTIVE and must not complete turn 2.
	tester, candidate := newFakeRelayStream(), newFakeRelayStream()
	op := textRelayOperation(3)
	type outcome struct {
		result operationResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := runWorkspaceRelay(context.Background(), op, tester, candidate, "brief", 0)
		done <- outcome{result, err}
	}()
	drainUserTurn(t, tester)
	// Self-start begins with only a BOS and an empty fragment.
	candidate.in <- &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "selfstart", Label: "assistant", BeginOfStream: true}}
	candidate.in <- assistantText("selfstart", "", false)
	candidate.settle(t)
	// Turn 1 completes; the candidate becomes active.
	tester.in <- assistantText("t1", "question", true)
	drainUserTurn(t, candidate)
	// The stale self-start terminal arrives while the candidate is active.
	candidate.in <- &genx.MessageChunk{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "selfstart", Label: "assistant", EndOfStream: true, Error: "interrupted"}}
	// The candidate's real turn-2 response follows on a new stream.
	candidate.in <- assistantText("c1", "answer", true)
	drainUserTurn(t, tester)
	tester.in <- assistantText("t2", "PASS", true)
	got := <-done
	if got.err != nil {
		t.Fatalf("relay error = %v", got.err)
	}
	assertion := got.result.assertion.(map[string]any)
	if assertion["completed_turns"] != 3 {
		t.Fatalf("completed_turns = %v", assertion["completed_turns"])
	}
	if terminal := assertion["terminal"].(map[string]any); terminal["text"] != "PASS" {
		t.Fatalf("terminal = %#v", terminal)
	}
}

func TestWorkspaceRelayRejectsNonOpusAudioMedia(t *testing.T) {
	cases := map[string]*genx.MessageChunk{
		"pcm packet":     {Part: &genx.Blob{MIMEType: "audio/L16;rate=16000;channels=1", Data: []byte{1, 2}}, Ctrl: &genx.StreamCtrl{StreamID: "t1", Label: "assistant"}},
		"mp3 eos":        {Part: &genx.Blob{MIMEType: "audio/mpeg"}, Ctrl: &genx.StreamCtrl{StreamID: "t1", Label: "assistant", EndOfStream: true}},
		"ogg vorbis bos": {Part: &genx.Blob{MIMEType: "audio/ogg; codecs=vorbis"}, Ctrl: &genx.StreamCtrl{StreamID: "t1", Label: "assistant", BeginOfStream: true}},
		"non-audio blob": {Part: &genx.Blob{MIMEType: "application/octet-stream", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "t1", Label: "assistant"}},
	}
	for name, chunk := range cases {
		t.Run(name, func(t *testing.T) {
			tester, candidate := newFakeRelayStream(), newFakeRelayStream()
			op := textRelayOperation(3)
			op.Media = "audio"
			done := make(chan error, 1)
			go func() {
				_, err := runWorkspaceRelay(context.Background(), op, tester, candidate, []byte{0x11}, 0)
				done <- err
			}()
			for range 3 { // BOS, packet, EOS of the initial input
				nextPush(t, tester)
			}
			tester.in <- chunk
			err := <-done
			if err == nil || !strings.Contains(err.Error(), "unsupported relay media type") || !strings.Contains(err.Error(), "client tester") {
				t.Fatalf("error = %v", err)
			}
			select {
			case forwarded := <-candidate.pushes:
				t.Fatalf("unsupported media was forwarded: %#v", forwarded)
			default:
			}
		})
	}
}

func TestWorkspaceRelayRejectsNonOpusTerminalAudioForTextRelay(t *testing.T) {
	tester, candidate := newFakeRelayStream(), newFakeRelayStream()
	op := textRelayOperation(2)
	op.TerminalMedia = "audio"
	done := make(chan error, 1)
	go func() {
		_, err := runWorkspaceRelay(context.Background(), op, tester, candidate, "brief", 0)
		done <- err
	}()
	drainUserTurn(t, tester)
	tester.in <- &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/mpeg"},
		Ctrl: &genx.StreamCtrl{StreamID: "t1", Label: "assistant", EndOfStream: true},
	}
	err := <-done
	_ = tester.Close()
	_ = candidate.Close()
	if err == nil || !strings.Contains(err.Error(), "unsupported relay media type") {
		t.Fatalf("error = %v", err)
	}
}

func TestRelayOpusMIME(t *testing.T) {
	for mimeType, want := range map[string]bool{
		"audio/opus": true, "audio/opus; rate=48000": true, "audio/opus; codecs=opus": true, "audio/ogg; codecs=opus": true, "audio/ogg;codecs=OPUS": true,
		"audio/opus; codecs=vorbis": false, "audio/opus; codecs=pcm": false,
		"audio/ogg": false, "audio/ogg; codecs=vorbis": false, "audio/mpeg": false, "audio/L16;rate=16000;channels=1": false, "application/ogg": false, "": false,
	} {
		if got := relayOpusMIME(mimeType); got != want {
			t.Fatalf("relayOpusMIME(%q) = %v, want %v", mimeType, got, want)
		}
	}
}
