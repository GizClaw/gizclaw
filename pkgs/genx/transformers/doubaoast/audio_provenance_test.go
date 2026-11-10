package doubaoast

import (
	"bytes"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestPCMProvenanceHistoryKeepsProviderAudioClock(t *testing.T) {
	history := newASTTranslateHistoryAudioBuffer()
	cursor := time.Duration(0)
	for i, duration := range []time.Duration{20 * time.Millisecond, 40 * time.Millisecond, 20 * time.Millisecond} {
		blob := &genx.Blob{MIMEType: "audio/pcm", Data: make([]byte, int(duration)*32000/int(time.Second)), ConcealedAudio: i == 1}
		if i != 1 {
			blob.OriginalAudio = &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}}
		}
		history.appendChunk(&genx.MessageChunk{Part: blob, Ctrl: &genx.StreamCtrl{Timestamp: int64(10000 - i)}}, "audio", cursor, duration)
		cursor += duration
	}
	if chunks := history.opus.segment(20, 60); len(chunks) != 0 {
		t.Fatal("concealed PCM entered original Opus history")
	}
	chunks := history.opus.segment(60, 80)
	if len(chunks) != 1 || chunks[0].Part.(*genx.Blob).MIMEType != "audio/opus" || chunks[0].Ctrl.Timestamp != 9998 {
		t.Fatal("original after concealment shifted outside provider subtitle range")
	}
	if history.opus.cursor != 80*time.Millisecond {
		t.Fatalf("provider audio cursor=%s", history.opus.cursor)
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

func TestPCMPreparationUploadsConcealedSamplesWithoutDecodingOriginal(t *testing.T) {
	transformer := &Transformer{}
	var decoder *opus.Decoder
	for _, concealed := range []bool{false, true} {
		data := bytes.Repeat([]byte{1, 0}, 320)
		blob := &genx.Blob{MIMEType: "audio/pcm", Data: data, ConcealedAudio: concealed}
		if !concealed {
			blob.OriginalAudio = &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}}
		}
		got, err := transformer.prepareAudioBlob(blob, &decoder)
		if err != nil || !bytes.Equal(got, data) || decoder != nil {
			t.Fatalf("PCM upload re-decoded original or omitted samples: %v", err)
		}
	}
}
