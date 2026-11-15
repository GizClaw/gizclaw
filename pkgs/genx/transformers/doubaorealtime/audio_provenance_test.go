package doubaorealtime

import (
	"bytes"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestPCMProvenanceRecordingUsesOriginalMIMEAndClosesInputRoute(t *testing.T) {
	out := newBufferStream(8)
	defer out.Close()
	routes := newDoubaoRealtimeHistoryRoutes()
	original := []byte{0xf8, 0xff, 0xfe}
	real := &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: make([]byte, 640), OriginalAudio: &genx.Blob{MIMEType: "audio/opus", Data: original}}, Ctrl: &genx.StreamCtrl{StreamID: "audio"}}
	concealed := &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: make([]byte, 1280), ConcealedAudio: true}, Ctrl: &genx.StreamCtrl{StreamID: "audio"}}
	if err := routes.push(out, real, "audio"); err != nil {
		t.Fatal(err)
	}
	if err := routes.push(out, concealed, "audio"); err != nil {
		t.Fatal(err)
	}
	if err := routes.close(out, "audio", "audio/pcm"); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		chunk, err := out.Next()
		if err != nil {
			t.Fatal(err)
		}
		blob := chunk.Part.(*genx.Blob)
		if blob.MIMEType != "audio/opus" {
			t.Fatalf("history MIME=%q, want original Opus", blob.MIMEType)
		}
		if i == 1 && !bytes.Equal(blob.Data, original) {
			t.Fatal("recording contains converted or concealed PCM")
		}
		if i == 0 && !chunk.IsBeginOfStream() || i == 2 && !chunk.IsEndOfStream() {
			t.Fatal("original recording MIME route was not closed")
		}
	}
	if len(routes.open) != 0 || len(routes.sources) != 0 {
		t.Fatal("PCM EOS retained original recording route state")
	}
	var history realtimeHistoryAudio
	if err := history.append(real, "audio"); err != nil {
		t.Fatal(err)
	}
	if err := history.append(concealed, "audio"); err != nil {
		t.Fatal(err)
	}
	chunks := history.take("transcript")
	if len(chunks) != 1 || !bytes.Equal(chunks[0].Part.(*genx.Blob).Data, original) {
		t.Fatal("realtime history persisted concealed PCM")
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

func TestPCMProvenanceDoesNotChangeProviderUpload(t *testing.T) {
	input := newDoubaoRealtimeAudioInput("pcm", 16000, 1, false)
	defer input.close()
	data := bytes.Repeat([]byte{1, 0}, 640)
	frames, err := input.prepareFrames(&genx.Blob{MIMEType: "audio/pcm", Data: data, ConcealedAudio: true})
	if err != nil || len(frames) != 1 || !bytes.Equal(frames[0], data) || input.decoder != nil {
		t.Fatalf("plain PCM upload changed or decoded media again: %v", err)
	}
}
