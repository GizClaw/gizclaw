package doubaoasr

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestHistoryUserAudioEOSChunkDefaultsStreamID(t *testing.T) {
	chunk := historyUserAudioEOSChunk("", "")
	if chunk.Role != genx.RoleUser || chunk.Name != "transcript" {
		t.Fatalf("history EOS route = %#v", chunk)
	}
	if chunk.Ctrl == nil || chunk.Ctrl.Label != genx.HistoryUserAudioLabel || chunk.Ctrl.StreamID != "audio" || !chunk.IsEndOfStream() {
		t.Fatalf("history EOS ctrl = %#v", chunk)
	}
	blob, ok := chunk.Part.(*genx.Blob)
	if !ok || blob.MIMEType != "audio/pcm" {
		t.Fatalf("history EOS blob = %#v", chunk.Part)
	}
}
