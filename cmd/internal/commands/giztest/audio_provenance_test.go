package giztestcmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func giztestPCM(data []byte, original []byte, concealed bool) *genx.Blob {
	blob := &genx.Blob{MIMEType: "audio/pcm", Data: data, ConcealedAudio: concealed}
	if original != nil {
		blob.OriginalAudio = &genx.Blob{MIMEType: "audio/opus", Data: original}
	}
	return blob
}

func TestPlaySessionConsumesPCMWithoutCodecAndRecordsOnlyOriginals(t *testing.T) {
	decoder := &fakePlayDecoder{err: errors.New("PCM must not be decoded again")}
	output := &fakePlayOutput{}
	var logs bytes.Buffer
	session := &playSession{decoder: decoder, output: output, out: &logs}
	defer session.close()
	original := []byte{0xf8, 0xff, 0xfe}
	var want []byte
	for i, samples := range []int{320, 640, 320} {
		data := bytes.Repeat([]byte{byte(i + 1), 0}, samples)
		var source []byte
		if i != 1 {
			source = original
		}
		if err := session.observe("peer", "assistant", giztestPCM(data, source, i == 1), false); err != nil {
			t.Fatal(err)
		}
		want = append(want, data...)
	}
	if err := session.observe("peer", "assistant", nil, true); err != nil {
		t.Fatal(err)
	}
	if len(decoder.packets) != 0 || !bytes.Equal(output.Bytes(), want) {
		t.Fatal("PCM was decoded again, omitted, or not played exactly")
	}
	if session.bytes != 6 || len(session.packets) != 2 || !bytes.Equal(session.packets[0], original) {
		t.Fatal("concealment changed the raw recording or network-byte count")
	}
	if !strings.Contains(logs.String(), "packets=2 audio=40ms") || !strings.Contains(logs.String(), "concealed_samples=640 concealed_ms=40") {
		t.Fatalf("playback metrics=%q", logs.String())
	}
}

func TestPlaySessionOwnsRetainedObserverAudio(t *testing.T) {
	for _, tc := range []struct {
		name             string
		pcm              bool
		discardRecording bool
	}{
		{"pcm_recorded", true, false},
		{"pcm_playback_only", true, true},
		{"legacy_recorded", false, false},
		{"legacy_playback_only", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantOriginal := []byte{0xf8, 0xff, 0xfe}
			original := bytes.Clone(wantOriginal)
			wantPCM := []byte{1, 0, 0xfe, 0xff}
			blob := giztestPCM(bytes.Clone(wantPCM), original, false)
			if !tc.pcm {
				blob = &genx.Blob{MIMEType: "audio/opus", Data: original}
			}
			decoder := &fakePlayDecoder{samples: []int16{1, -2}}
			output := &fakePlayOutput{}
			session := &playSession{decoder: decoder, output: output, discardRecording: tc.discardRecording}
			defer session.close()
			if err := observePeerAudioBlob(session.observe, "peer", "assistant", blob, [][]byte{original}); err != nil {
				t.Fatal(err)
			}
			// The callback has returned, but the short utterance is still buffered.
			clear(blob.Data)
			clear(original)
			if err := session.observe("peer", "assistant", nil, true); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(output.Bytes(), wantPCM) {
				t.Fatal("playback retained borrowed caller storage")
			}
			if tc.discardRecording {
				if len(session.packets) != 0 {
					t.Fatal("playback-only session retained a recording")
				}
			} else if len(session.packets) != 1 || !bytes.Equal(session.packets[0], wantOriginal) {
				t.Fatal("recording retained borrowed caller storage")
			}
			if !tc.pcm && (len(decoder.packets) != 1 || !bytes.Equal(decoder.packets[0], wantOriginal)) {
				t.Fatal("legacy playback lost its owned encoded packet")
			}
		})
	}
}

func TestListenPCMCountsRealPacketsSeparatelyFromConcealment(t *testing.T) {
	stream := newFakeRelayStream()
	defer stream.Close()
	original := []byte{0xf8, 0xff, 0xfe}
	for i, samples := range []int{320, 640, 320} {
		var source []byte
		if i != 1 {
			source = original
		}
		stream.in <- &genx.MessageChunk{Part: giztestPCM(make([]byte, samples*2), source, i == 1), Ctrl: &genx.StreamCtrl{StreamID: "remote", Label: "participant"}}
	}
	result, err := listenPeerStream(t.Context(), stream, listenStep("100ms"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	object := result.assertion.(map[string]any)
	if object["packets"] != 2 || object["audio_bytes"] != 6 || object["concealed_samples"] != 640 || object["concealed_ms"] != int64(40) {
		t.Fatalf("PCM listen metrics=%#v", object)
	}
	packets, err := decodeOpusPackets(object["audio"].([]byte))
	if err != nil || len(packets) != 2 || !bytes.Equal(packets[0], original) || !bytes.Equal(packets[1], original) {
		t.Fatal("capture inserted encoded concealment")
	}
}

func TestPCMIntegrityHashesOriginalsAndStillRejectsMissingBOS(t *testing.T) {
	original := []byte{0xf8, 0xff, 0xfe}
	var integrity peerAudioIntegrity
	chunks := []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "s", BeginOfStream: true}},
		{Part: giztestPCM(make([]byte, 640), original, false), Ctrl: &genx.StreamCtrl{StreamID: "s"}},
		{Part: giztestPCM(make([]byte, 1280), nil, true), Ctrl: &genx.StreamCtrl{StreamID: "s"}},
		{Part: giztestPCM(make([]byte, 640), original, false), Ctrl: &genx.StreamCtrl{StreamID: "s"}},
		{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "s", EndOfStream: true}},
	}
	for _, chunk := range chunks {
		if !integrity.observe(chunk) {
			t.Fatal("valid PCM route rejected")
		}
	}
	digest := sha256.Sum256(append(bytes.Clone(original), original...))
	summary := integrity.summary()
	if summary["sha256"] != hex.EncodeToString(digest[:]) || summary["violations"] != 0 || summary["open"] != 0 {
		t.Fatalf("integrity=%#v", summary)
	}
	var missing peerAudioIntegrity
	missing.observe(chunks[2])
	if missing.violations != 1 || missing.violationReasons["payload_without_begin"] != 1 {
		t.Fatal("concealment relaxed the missing-BOS assertion")
	}
	if integrity.observe(chunks[2]) {
		t.Fatal("concealed audio re-entered a completed route")
	}
}

func TestWorkspaceRelayForwardsValidatedPCMAndCapturesOnlyOriginals(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	tester, candidate := newFakeRelayStream(), newFakeRelayStream()
	defer tester.Close()
	defer candidate.Close()
	op := textRelayOperation(2)
	op.Media = "audio"
	type outcome struct {
		result operationResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := runWorkspaceRelay(ctx, op, tester, candidate, []byte{0xf8, 0xff, 0xfe}, 1<<20)
		done <- outcome{result, err}
	}()
	for range 5 {
		_ = nextPush(t, tester)
	}
	original := []byte{0xf8, 0xff, 0xfe}
	data := bytes.Repeat([]byte{1, 0}, 320)
	tester.in <- &genx.MessageChunk{Part: giztestPCM(data, original, false), Ctrl: &genx.StreamCtrl{StreamID: "first", Label: "assistant"}}
	if begin := nextPush(t, candidate); !begin.IsBeginOfStream() || begin.Part.(*genx.Blob).MIMEType != "audio/pcm" {
		t.Fatal("PCM relay lost its declared channel")
	}
	if chunk := nextPush(t, candidate); !bytes.Equal(chunk.Part.(*genx.Blob).Data, data) {
		t.Fatal("PCM relay forwarded original Opus as PCM")
	}
	concealed := make([]byte, 1280)
	tester.in <- &genx.MessageChunk{Part: giztestPCM(concealed, nil, true), Ctrl: &genx.StreamCtrl{StreamID: "first", Label: "assistant"}}
	if chunk := nextPush(t, candidate); !bytes.Equal(chunk.Part.(*genx.Blob).Data, concealed) {
		t.Fatal("relay silently dropped already concealed PCM")
	}
	tester.in <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "first", Label: "assistant", EndOfStream: true}}
	if end := nextPush(t, candidate); !end.IsEndOfStream() || end.Part.(*genx.Blob).MIMEType != "audio/pcm" {
		t.Fatal("PCM terminal did not close forwarded input")
	}
	candidate.in <- &genx.MessageChunk{Part: giztestPCM(data, original, false), Ctrl: &genx.StreamCtrl{StreamID: "final", Label: "assistant"}}
	candidate.in <- &genx.MessageChunk{Part: giztestPCM(concealed, nil, true), Ctrl: &genx.StreamCtrl{StreamID: "final", Label: "assistant"}}
	candidate.in <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "final", Label: "assistant", EndOfStream: true}}
	result := <-done
	if result.err != nil {
		t.Fatal(result.err)
	}
	terminal := result.result.assertion.(map[string]any)["terminal"].(map[string]any)
	packets, err := decodeOpusPackets(terminal["audio"].([]byte))
	if err != nil || len(packets) != 1 || !bytes.Equal(packets[0], original) {
		t.Fatal("relay recording included synthetic PCM")
	}
	if !relayTerminalMedia("audio", "audio/pcm; rate=16000; channels=1") || relayTerminalMedia("audio", "audio/pcm; rate=24000; channels=1") {
		t.Fatal("relay accepted unsupported PCM or lost valid PCM terminal")
	}
}
