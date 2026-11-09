package doubaoasr

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestPCMProvenanceUploadsAllSamplesAndRecordsOnlyOriginals(t *testing.T) {
	session := newFakeDoubaoASRSession()
	transformer := newTransformer(Config{Format: "opus", SampleRate: 16000, Channels: 1, Bits: 16})
	var opened doubaoASRSessionConfig
	transformer.newSession = func(_ context.Context, cfg doubaoASRSessionConfig) (doubaoASRSession, error) {
		opened = cfg
		return session, nil
	}
	input := newBufferStream(8)
	out, err := transformer.Transform(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	originals := [][]byte{{0xf8, 0xff, 0xfe}, {0xf8, 0xff, 0xfe}}
	var want []byte
	for i, samples := range []int{320, 640, 320} {
		data := bytes.Repeat([]byte{byte(i + 1), 0}, samples)
		blob := &genx.Blob{MIMEType: "audio/pcm", Data: data, ConcealedAudio: i == 1}
		if i != 1 {
			blob.OriginalAudio = &genx.Blob{MIMEType: "audio/opus", Data: originals[i/2]}
		}
		want = append(want, data...)
		if err := input.Push(&genx.MessageChunk{Part: blob, Ctrl: &genx.StreamCtrl{StreamID: "audio"}}); err != nil {
			t.Fatal(err)
		}
	}
	_ = input.Close()
	chunks := collectTransformerChunks(t, out)
	if !opened.isPCM() {
		t.Fatalf("provider format=%q, want PCM", opened.format)
	}
	var sent []byte
	for _, send := range session.sends {
		sent = append(sent, send.data...)
	}
	if !bytes.Equal(sent, want) {
		t.Fatal("provider upload omitted or re-decoded media PCM")
	}
	var recorded [][]byte
	for _, chunk := range chunks {
		if chunk.Ctrl != nil && chunk.Ctrl.Label == genx.HistoryUserAudioLabel {
			if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
				if blob.MIMEType != "audio/opus" {
					t.Fatalf("recorded MIME=%q", blob.MIMEType)
				}
				recorded = append(recorded, blob.Data)
			}
		}
	}
	if len(recorded) != 2 || !bytes.Equal(recorded[0], originals[0]) || !bytes.Equal(recorded[1], originals[1]) {
		t.Fatal("recording contains PCM concealment or lost real originals")
	}
}

func TestRecoveredPCMSelectsSessionBeforeEmptyBOSAndAdaptsFormat(t *testing.T) {
	for _, rate := range []int{8000, 16000} {
		for _, channels := range []int{1, 2} {
			t.Run(fmt.Sprintf("%dHz_%dch", rate, channels), func(t *testing.T) {
				session := newFakeDoubaoASRSession()
				transformer := newTransformer(Config{Format: "ogg_opus", SampleRate: rate, Channels: channels, Bits: 16, RealtimePacing: new(false)})
				var opened doubaoASRSessionConfig
				transformer.newSession = func(_ context.Context, cfg doubaoASRSessionConfig) (doubaoASRSession, error) {
					opened = cfg
					return session, nil
				}
				input := newBufferStream(16)
				out, err := transformer.Transform(t.Context(), input)
				if err != nil {
					t.Fatal(err)
				}
				defer out.Close()
				mimeType := "audio/pcm;rate=16000;channels=1"
				if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: mimeType, OriginalAudio: &genx.Blob{MIMEType: "audio/opus"}}, Ctrl: &genx.StreamCtrl{StreamID: "audio", BeginOfStream: true}}); err != nil {
					t.Fatal(err)
				}
				for i, samples := range []int{40, 160, 40, 160} {
					blob := &genx.Blob{MIMEType: mimeType, Data: bytes.Repeat([]byte{byte(i + 1), 0}, samples), ConcealedAudio: i == 1}
					if !blob.ConcealedAudio {
						blob.OriginalAudio = &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}}
					}
					if err := input.Push(&genx.MessageChunk{Part: blob, Ctrl: &genx.StreamCtrl{StreamID: "audio"}}); err != nil {
						t.Fatal(err)
					}
				}
				_ = input.Close()
				collectTransformerChunks(t, out)
				if !opened.isPCM() || opened.sampleRate != rate || opened.channels != channels {
					t.Fatalf("empty converted BOS opened wrong provider format: %#v", opened)
				}
				var uploaded []byte
				for _, send := range session.sends {
					uploaded = append(uploaded, send.data...)
				}
				if want := rate * channels * 2 / 40; len(uploaded) != want {
					t.Fatalf("25ms source uploaded %d bytes, want %d", len(uploaded), want)
				}
				if audioDuration(uploaded, opened) != 25*time.Millisecond {
					t.Fatal("format adaptation distorted provider audio time")
				}
			})
		}
	}
}

func TestHistoryCloneSelectsOriginalBeforeCopyingPCM(t *testing.T) {
	original := &genx.Blob{MIMEType: "audio/opus", Data: []byte{1, 2, 3}, OriginalAudio: &genx.Blob{MIMEType: "audio/opus", Data: []byte{9}}, ConcealedAudio: true}
	blob := &genx.Blob{MIMEType: "audio/pcm", OriginalAudio: original}
	chunk := &genx.MessageChunk{Part: blob, Ctrl: &genx.StreamCtrl{StreamID: "input", Timestamp: 123, BeginOfStream: true, EndOfStream: true, Error: "source"}}
	var cloned *genx.MessageChunk
	withoutPCM := testing.AllocsPerRun(5, func() { cloned = historyUserAudioChunk(chunk, "recording") })
	blob.Data = make([]byte, 128<<10)
	withPCM := testing.AllocsPerRun(5, func() { cloned = historyUserAudioChunk(chunk, "recording") })
	if withPCM > withoutPCM {
		t.Fatalf("history copied discarded PCM: allocations %g > %g", withPCM, withoutPCM)
	}
	recorded := cloned.Part.(*genx.Blob)
	if recorded.OriginalAudio != nil || recorded.ConcealedAudio || !bytes.Equal(recorded.Data, original.Data) {
		t.Fatal("history retained nested process-local provenance")
	}
	recorded.Data[0] = 7
	if original.Data[0] != 1 || cloned.Ctrl == chunk.Ctrl || cloned.Ctrl.StreamID != "recording" || cloned.Ctrl.Timestamp != 123 || cloned.IsBeginOfStream() || cloned.IsEndOfStream() || cloned.Ctrl.Error != "" {
		t.Fatal("history clone lost payload isolation or route metadata")
	}
}

func TestPCMProvenanceHistoryKeepsFractionalSourceClockAcrossResampling(t *testing.T) {
	cfg := doubaoASRSessionConfig{format: "pcm", sampleRate: 8000, channels: 2, bits: 16}
	history := newDoubaoASRHistoryAudioBuffer(cfg)
	for i := range 3 {
		blob := &genx.Blob{MIMEType: "audio/pcm;rate=16000;channels=1", Data: make([]byte, 80), ConcealedAudio: i == 1}
		if !blob.ConcealedAudio {
			blob.OriginalAudio = &genx.Blob{MIMEType: "audio/opus", Data: []byte{byte(i)}}
		}
		// A FIR converter may retain these samples until later calls. That
		// delay must not remove originals or shrink a fractional PLC interval.
		history.appendChunk(&genx.MessageChunk{Part: blob}, "audio", nil, cfg)
	}
	if history.audioCursor != 7500*time.Microsecond {
		t.Fatalf("fractional source cursor = %s", history.audioCursor)
	}
	chunks := history.opusSegment("audio", 5, 8)
	if len(chunks) != 1 || chunks[0].Part.(*genx.Blob).Data[0] != 2 {
		t.Fatal("recorded original moved across fractional concealment")
	}
}

func TestPCMProvenanceHistoryUsesUploadedAudioPositions(t *testing.T) {
	cfg := doubaoASRSessionConfig{format: "pcm", sampleRate: 16000, channels: 1, bits: 16}
	history := newDoubaoASRHistoryAudioBuffer(cfg)
	for i, duration := range []int{20, 40, 20} {
		blob := &genx.Blob{MIMEType: "audio/pcm", Data: make([]byte, duration*32), ConcealedAudio: i == 1}
		if i != 1 {
			blob.OriginalAudio = &genx.Blob{MIMEType: "audio/opus", Data: []byte{byte(i + 1)}}
		}
		history.appendChunk(&genx.MessageChunk{Part: blob, Ctrl: &genx.StreamCtrl{Timestamp: int64(9999 - i)}}, "audio", blob.Data, cfg)
	}
	if chunks := history.opusSegment("audio", 20, 60); len(chunks) != 0 {
		t.Fatal("concealed interval became recorded audio")
	}
	chunks := history.opusSegment("audio", 60, 80)
	if len(chunks) != 1 || chunks[0].Part.(*genx.Blob).Data[0] != 3 {
		t.Fatal("recorded original shifted across the concealed interval")
	}
}
