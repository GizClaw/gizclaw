package giztestcmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

const testPeerPCMMIME = "audio/pcm;rate=16000;channels=1"

func pcmAssistantChunk(id string, samples int, original []byte, concealed bool) *genx.MessageChunk {
	blob := &genx.Blob{MIMEType: testPeerPCMMIME, Data: make([]byte, samples*2), ConcealedAudio: concealed}
	if original != nil {
		blob.OriginalAudio = &genx.Blob{MIMEType: "audio/opus", Data: original}
	}
	return &genx.MessageChunk{Part: blob, Ctrl: &genx.StreamCtrl{StreamID: id, Label: "assistant"}}
}

func TestFirstPacketCorrelationPCMUsesOnlyRealOriginals(t *testing.T) {
	clock := newPeerPacketClock()
	base := newFakeRelayStream()
	defer base.Close()
	sender := &packetTimedPeerStream{
		peerStream: base, clock: clock, identity: "alice-key", started: time.Now(),
		step: giztest.Step{ID: "send", Client: "alice", PeerStream: &giztest.PeerStreamOperation{MeasureFirstPacket: true}},
	}
	if err := sender.Push(t.Context(), &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: testAudibleOpus(t)}}); err != nil {
		t.Fatal(err)
	}
	marked := nextPush(t, base).Part.(*genx.Blob).Data
	receiver := &packetTimedPeerStream{clock: clock, step: listenChild("listen", "bob", "1s")}
	received := sender.sent.sent.Add(40 * time.Millisecond)
	chunk := pcmAssistantChunk("sfu/alice-key/1", 320, marked, true)
	chunk.Ctrl.Label = "alice-key"
	// Even misleading provenance cannot turn synthesized PCM into delivery proof.
	receiver.observePacketArrival(chunk, received)
	if len(receiver.receipts) != 0 {
		t.Fatal("concealment satisfied original first-packet delivery")
	}
	chunk.Part.(*genx.Blob).ConcealedAudio = false
	receiver.observePacketArrival(chunk, received)
	receiver.observePacketArrival(chunk, received.Add(time.Millisecond))
	if len(receiver.receipts) != 1 {
		t.Fatalf("PCM original correlation receipts = %d, want 1", len(receiver.receipts))
	}
	result := operationResult{evidence: map[string]any{}}
	receiver.addEvidence(&result)
	validatePacketTimingEvidence(t, result.evidence)
	sample := result.evidence["audio_first_packets"].([]any)[0].(map[string]any)
	if sample["latency_ms"] != float64(40) || sample["packet_id"] != sender.sent.id {
		t.Fatalf("PCM first-packet sample = %#v", sample)
	}
}

func TestPeerAudioIntegrityPCMUsesDeliveredLifecycleAndOriginalDigest(t *testing.T) {
	original := []byte{0xf8, 0xff, 0xfe}
	for _, concealed := range []bool{false, true} {
		t.Run(map[bool]string{false: "original", true: "concealed"}[concealed], func(t *testing.T) {
			var integrity peerAudioIntegrity
			begin := pcmAssistantChunk("s", 0, nil, false)
			begin.Ctrl.BeginOfStream = true
			// Lifecycle keys normalize MIME formatting before checking the route.
			begin.Part.(*genx.Blob).MIMEType = "audio/pcm; channels=1; rate=16000"
			integrity.observe(begin)
			payload := pcmAssistantChunk("s", 320, original, concealed)
			integrity.observe(payload)
			end := pcmAssistantChunk("s", 0, nil, false)
			end.Ctrl.EndOfStream = true
			integrity.observe(end)
			before := integrity.summary()["sha256"]
			if integrity.observe(payload) || integrity.ignoredPostEOSPackets != 1 || integrity.violations != 0 {
				t.Fatalf("late PCM payload changed lifecycle: %#v", integrity.summary())
			}
			if integrity.summary()["sha256"] != before {
				t.Fatal("post-EOS PCM changed the original digest")
			}
			if concealed {
				if before != nil {
					t.Fatal("concealment contributed an original digest")
				}
			} else {
				digest := sha256.Sum256(original)
				if before != hex.EncodeToString(digest[:]) {
					t.Fatal("integrity hashed converted PCM instead of its original")
				}
			}
			var missing peerAudioIntegrity
			missing.observe(payload)
			if missing.violations != 1 || missing.violationReasons["payload_without_begin"] != 1 {
				t.Fatalf("missing PCM BOS was hidden or counted twice: %#v", missing.summary())
			}
			payload.Ctrl.StreamID = ""
			missing.observe(payload)
			if missing.violationReasons["missing_stream_id"] != 1 {
				t.Fatal("PCM lifecycle hid a missing stream ID")
			}
		})
	}
}

func TestRebuildPeerStreamOutputPCMPlayoutIncludesConcealment(t *testing.T) {
	original := []byte{0xf8, 0xff, 0xfe}
	start := time.Unix(1, 0)
	var records []peerStreamOutputRecord
	var originalsOnly peerAudioPacing
	appendRecord := func(at time.Duration, samples int, concealed bool) {
		var source []byte
		if !concealed {
			source = original
			originalsOnly.observe(start.Add(at), [][]byte{original})
		}
		chunk := pcmAssistantChunk("kept", samples, source, concealed)
		var classifier peerAudioAudibility
		records = append(records, peerStreamOutputRecord{label: "assistant", streamID: "kept", chunk: chunk, receivedAt: start.Add(at), elapsed: at, audio: classifier.classify(chunk)})
	}
	for at := time.Duration(0); at < 500*time.Millisecond; at += 20 * time.Millisecond {
		appendRecord(at, 320, false)
	}
	for at := 500 * time.Millisecond; at < 2*time.Second; at += 60 * time.Millisecond {
		appendRecord(at, 320, false)
		appendRecord(at+20*time.Millisecond, 640, true)
	}
	// An abandoned response contributes neither originals nor playable PCM.
	abandoned := pcmAssistantChunk("abandoned", 320, original, false)
	records = append(records, peerStreamOutputRecord{label: "assistant", streamID: "abandoned", chunk: abandoned, receivedAt: start.Add(5 * time.Second)})
	out, err := rebuildPeerStreamOutput(records, map[string]bool{"abandoned": true}, true)
	if err != nil {
		t.Fatal(err)
	}
	summary := out.pacing.summary()
	if summary["packets"] != 50 || summary["audio_ms"] != int64(1000) || summary["playout_ms"] != int64(2000) {
		t.Fatalf("original and delivered durations were conflated: %#v", summary)
	}
	if summary["underruns"] != 0 || summary["underrun_ms"] != float64(0) || summary["minimum_buffer_ms"] != float64(480) {
		t.Fatalf("continuous recovered PCM falsely underruns: %#v", summary)
	}
	if originalsOnly.summary()["underruns"] != 12 || originalsOnly.summary()["underrun_ms"] != float64(480) {
		t.Fatal("regression trace no longer exposes the original-only buffer deficit")
	}
	if out.audioBytes != 150 || len(out.assistantPackets) != 50 || out.concealment.fields()["concealed_ms"] != int64(1000) {
		t.Fatalf("concealment altered original provenance: bytes=%d packets=%d concealment=%v", out.audioBytes, len(out.assistantPackets), out.concealment)
	}
	// Real playout stalls must still fail even when recovery supplies samples.
	if err := out.pacing.observeBlob(start.Add(4*time.Second), pcmAssistantChunk("kept", 320, original, false).Part.(*genx.Blob), [][]byte{original}); err != nil {
		t.Fatal(err)
	}
	if out.pacing.summary()["underruns"].(int) == 0 {
		t.Fatal("PCM accounting hid an actual playout stall")
	}
}

func TestPeerStreamPCMPlayoutPreservesOriginalCapture(t *testing.T) {
	for _, mode := range []string{"listen", "invoke"} {
		t.Run(mode, func(t *testing.T) {
			stream := newFakeRelayStream()
			defer stream.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			original := []byte{0xf8, 0xff, 0xfe}
			begin := pcmAssistantChunk("s", 0, nil, false)
			begin.Ctrl.BeginOfStream = true
			end := pcmAssistantChunk("s", 0, nil, false)
			end.Ctrl.EndOfStream = true
			chunks := []*genx.MessageChunk{
				begin, pcmAssistantChunk("s", 320, original, false),
				pcmAssistantChunk("s", 640, nil, true), pcmAssistantChunk("s", 320, original, false),
				end, assistantText("s", "done", true),
			}
			go func() {
				if mode == "invoke" {
					drainPushes(stream, 3)
				}
				for _, chunk := range chunks {
					stream.in <- chunk
				}
			}()
			var observedPCM, concealedPCM int
			observer := func(_, _ string, blob *genx.Blob, _ bool) error {
				if blob != nil {
					if blob.MIMEType != testPeerPCMMIME {
						return errors.New("PCM was relabeled as Opus")
					}
					observedPCM++
					if blob.ConcealedAudio {
						concealedPCM++
					}
				}
				return nil
			}
			var result operationResult
			var err error
			if mode == "listen" {
				result, err = listenPeerStream(ctx, stream, listenStep("25ms"), 1<<20, observer)
			} else {
				result, err = invokePeerStream(ctx, nil, func() (peerStream, error) { return stream, nil }, giztest.Step{ID: "turn", Client: "peer", PeerStream: &giztest.PeerStreamOperation{Mode: "text"}}, "hello", 1<<20, observer)
			}
			if err != nil {
				t.Fatal(err)
			}
			object := result.assertion.(map[string]any)
			pacing := object["audio_pacing"].(map[string]any)
			if pacing["packets"] != 2 || pacing["audio_ms"] != int64(40) || pacing["playout_ms"] != int64(80) || object["audio_bytes"] != 6 {
				t.Fatalf("PCM provenance and playout = %#v", object)
			}
			packets, err := decodeOpusPackets(object["audio"].([]byte))
			if err != nil || len(packets) != 2 || !bytes.Equal(packets[0], original) || !bytes.Equal(packets[1], original) || observedPCM != 3 || concealedPCM != 1 {
				t.Fatal("PCM capture or playback changed original/concealed provenance")
			}
		})
	}
}

type pcmPlaybackOutput struct{ written chan []byte }

func (o *pcmPlaybackOutput) Write(data []byte) (int, error) {
	o.written <- bytes.Clone(data)
	return len(data), nil
}

func (o *pcmPlaybackOutput) Close() error { return nil }

func TestInvokePeerStreamPCMEOSFlushesShortPlaybackBeforeText(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	output := &pcmPlaybackOutput{written: make(chan []byte, 1)}
	session := &playSession{decoder: &fakePlayDecoder{err: errors.New("PCM must not be decoded")}, output: output, out: io.Discard}
	defer session.close()
	stream := newFakeRelayStream()
	original := []byte{0xf8, 0xff, 0xfe}
	go func() {
		drainPushes(stream, 3)
		begin := pcmAssistantChunk("s", 0, nil, false)
		begin.Ctrl.BeginOfStream = true
		stream.in <- begin
		stream.in <- pcmAssistantChunk("s", 320, original, false)
		end := pcmAssistantChunk("s", 0, nil, false)
		end.Ctrl.EndOfStream = true
		stream.in <- end
		select {
		case <-output.written:
			stream.in <- assistantText("s", "done", true)
		case <-ctx.Done():
		}
	}()
	_, err := invokePeerStream(ctx, nil, func() (peerStream, error) { return stream, nil }, giztest.Step{ID: "turn", Client: "peer", PeerStream: &giztest.PeerStreamOperation{Mode: "text"}}, "hello", 0, session.observe)
	if err != nil {
		t.Fatalf("short PCM waited for text completion instead of audio EOS: %v", err)
	}
}

func TestWorkspaceRelayPCMPlayWaitsForAudioAfterTextTerminal(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	first, second := newFakeRelayStream(), newFakeRelayStream()
	var observed, ends int
	done := make(chan error, 1)
	go func() {
		_, err := runWorkspaceRelayWithEvidence(ctx, textRelayOperation(2), first, second, "brief", 0, false, func(_, _ string, blob *genx.Blob, end bool) error {
			if blob != nil {
				observed++
			}
			if end {
				ends++
			}
			return nil
		})
		done <- err
	}()
	drainUserTurn(t, first)
	first.in <- pcmAssistantChunk("audio", 320, []byte{0xf8, 0xff, 0xfe}, false)
	first.in <- assistantText("text", "question", true)
	if chunk := nextPush(t, second); !chunk.IsBeginOfStream() {
		t.Fatal("forwarded question is missing BOS")
	}
	expectUserText(t, second, "question", false)
	select {
	case chunk := <-second.pushes:
		t.Fatalf("PCM play relay completed before trailing audio: %#v", chunk)
	default:
	}
	first.in <- pcmAssistantChunk("audio", 640, nil, true)
	end := pcmAssistantChunk("audio", 0, nil, false)
	end.Ctrl.EndOfStream = true
	first.in <- end
	expectUserText(t, second, "", true)
	second.in <- assistantText("answer", "PASS", true)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if observed != 2 || ends != 1 {
		t.Fatalf("PCM playback observed %d frames and %d ends", observed, ends)
	}
}

func TestWorkspaceRelayPCMPlayWaitsForTextAfterAudioTerminal(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	first, second := newFakeRelayStream(), newFakeRelayStream()
	done := make(chan error, 1)
	go func() {
		_, err := runWorkspaceRelayWithEvidence(ctx, textRelayOperation(2), first, second, "brief", 0, false, func(_, _ string, _ *genx.Blob, _ bool) error { return nil })
		done <- err
	}()
	drainUserTurn(t, first)
	first.in <- pcmAssistantChunk("audio", 320, []byte{0xf8, 0xff, 0xfe}, false)
	first.in <- assistantText("text", "question ", false)
	if chunk := nextPush(t, second); !chunk.IsBeginOfStream() {
		t.Fatal("forwarded question is missing BOS")
	}
	expectUserText(t, second, "question ", false)
	end := pcmAssistantChunk("audio", 0, nil, false)
	end.Ctrl.EndOfStream = true
	first.in <- end
	first.in <- assistantText("text", "tail", false)
	expectUserText(t, second, "tail", false)
	select {
	case chunk := <-second.pushes:
		t.Fatalf("PCM EOS completed the relay before text EOS: %#v", chunk)
	default:
	}
	first.in <- assistantText("text", "", true)
	expectUserText(t, second, "", true)
	second.in <- assistantText("answer", "PASS", true)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestPeerAudioPacingConcealedPCMDoesNotInventOriginalPackets(t *testing.T) {
	var pacing peerAudioPacing
	start := time.Unix(1, 0)
	for at := time.Duration(0); at < 600*time.Millisecond; at += 20 * time.Millisecond {
		blob := pcmAssistantChunk("s", 320, nil, true).Part.(*genx.Blob)
		if err := pacing.observeBlob(start.Add(at), blob, nil); err != nil {
			t.Fatal(err)
		}
	}
	summary := pacing.summary()
	if summary["packets"] != 0 || summary["audio_ms"] != int64(0) || summary["playout_ms"] != int64(600) || summary["underruns"] != 0 {
		t.Fatalf("concealment invented original audio or lost playout: %#v", summary)
	}
}

func TestWorkspaceRelayPCMRetainsWholeRelayByteLimit(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	first, second := newFakeRelayStream(), newFakeRelayStream()
	type outcome struct {
		result operationResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := runWorkspaceRelay(ctx, textRelayOperation(2), first, second, "brief", 0)
		done <- outcome{result, err}
	}()
	drainUserTurn(t, first)
	// Each 9 MiB PCM payload fits the unchanged per-turn byte and duration
	// limits. Together they must exceed the whole-relay byte budget even
	// though neither concealed payload supplies original network bytes.
	first.in <- pcmAssistantChunk("first-audio", 9<<19, nil, true)
	first.in <- assistantText("first-text", "next", true)
	drainUserTurn(t, second)
	second.in <- pcmAssistantChunk("second-audio", 9<<19, nil, true)
	second.in <- assistantText("second-text", "PASS", true)
	got := <-done
	if got.err == nil || !strings.Contains(got.err.Error(), "16777216-byte relay audio limit") {
		t.Fatalf("concealment escaped the whole-relay byte budget: %v", got.err)
	}
	if got.result.evidence["audio_bytes"] != 0 || got.result.evidence["completed_turns"] != 1 {
		t.Fatalf("concealment changed original bytes or turn ownership: %#v", got.result.evidence)
	}
}

func TestWorkspaceRelayPCMRetainsOriginalByteLimit(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	first, second := newFakeRelayStream(), newFakeRelayStream()
	first.in = make(chan *genx.MessageChunk, 9001)
	second.in = make(chan *genx.MessageChunk, 9001)
	done := make(chan error, 1)
	go func() {
		_, err := runWorkspaceRelay(ctx, textRelayOperation(2), first, second, "brief", 0)
		done <- err
	}()
	padded := append([]byte{0xfb, 0x41, 0xff, 0xff, 0xff, 0xff, 0x08, 0xff, 0xfe}, make([]byte, 1024)...)
	drainUserTurn(t, first)
	for range 9000 {
		first.in <- pcmAssistantChunk("first-audio", 320, padded, false)
	}
	first.in <- assistantText("first-text", "next", true)
	drainUserTurn(t, second)
	for range 9000 {
		second.in <- pcmAssistantChunk("second-audio", 320, padded, false)
	}
	second.in <- assistantText("second-text", "PASS", true)
	// The decoded PCM fits 16 MiB, but the original padded packets do not.
	if err := <-done; err == nil || !strings.Contains(err.Error(), "16777216-byte relay audio limit") {
		t.Fatalf("PCM conversion bypassed the original whole-relay byte cap: %v", err)
	}
}

func TestWorkspaceRelayPCMCountsSamplesTowardDurationLimit(t *testing.T) {
	for _, concealed := range []bool{false, true} {
		t.Run(map[bool]string{false: "original", true: "concealed"}[concealed], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			first, second := newFakeRelayStream(), newFakeRelayStream()
			first.in = make(chan *genx.MessageChunk, 29951)
			original := []byte{0xf8, 0xff, 0xfe}
			// 599 seconds of encoded audio leaves the byte budget available;
			// two seconds of delivered PCM must exhaust the duration budget.
			for range 29950 {
				first.in <- assistantBlob("narration", original, false)
			}
			first.in <- pcmAssistantChunk("narration", 32000, nil, concealed)
			op := textRelayOperation(1)
			op.TerminalMedia = "audio"
			_, err := runWorkspaceRelay(ctx, op, first, second, "brief", 0)
			if err == nil || !strings.Contains(err.Error(), "audio duration limit") {
				t.Fatalf("PCM samples escaped the fixed duration guard: %v", err)
			}
		})
	}
}

func TestWorkspaceRelayPCMTurnAudioLimits(t *testing.T) {
	padded := append([]byte{0xfb, 0x41, 0xff, 0xff, 0xff, 0xff, 0x08, 0xff, 0xfe}, make([]byte, 1024)...)
	for _, tc := range []struct {
		name      string
		frames    int
		samples   int
		original  []byte
		concealed bool
		discarded bool
		wantError string
	}{
		{name: "narration beyond non-audio event limit", frames: 6000, samples: 320},
		{name: "concealment beyond non-audio event limit", frames: 6000, samples: 320, concealed: true},
		{name: "empty PCM retains event guard", frames: 4097, wantError: "4096-event"},
		{name: "padded originals retain byte guard", frames: 16300, samples: 320, original: padded, wantError: "relay turn audio limit"},
		{name: "discarded originals retain byte guard", frames: 16300, samples: 320, original: padded, discarded: true, wantError: "relay turn audio limit"},
		{name: "oversized PCM", frames: 1, samples: giztest.MaxRelayAudioBytes/2 + 1, wantError: "relay turn audio limit"},
		{name: "concealment retains byte guard", frames: 1, samples: giztest.MaxRelayAudioBytes/2 + 1, concealed: true, wantError: "relay turn audio limit"},
		{name: "discarded PCM retains byte guard", frames: 1, samples: giztest.MaxRelayAudioBytes/2 + 1, discarded: true, wantError: "relay turn audio limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			first, second := newFakeRelayStream(), newFakeRelayStream()
			first.in = make(chan *genx.MessageChunk, tc.frames+1)
			var original []byte
			if !tc.concealed {
				original = tc.original
				if original == nil {
					original = []byte{0xf8, 0xff, 0xfe}
				}
			}
			for range tc.frames {
				chunk := pcmAssistantChunk("narration", tc.samples, original, tc.concealed)
				if tc.discarded {
					chunk.Ctrl.Error = "interrupted"
				}
				first.in <- chunk
			}
			end := pcmAssistantChunk("narration", 0, nil, false)
			end.Ctrl.EndOfStream = true
			first.in <- end
			op := textRelayOperation(1)
			op.TerminalMedia = "audio"
			result, err := runWorkspaceRelay(ctx, op, first, second, "brief", 0)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v, want %s", err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.evidence["events"] != tc.frames+1 || result.evidence["audio_bytes"] != tc.frames*len(original) {
				t.Fatalf("PCM relay evidence = %#v", result.evidence)
			}
		})
	}
}
