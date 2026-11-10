package sfu

import (
	"bytes"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestOpusPassthroughSelectsOnlyRealOriginalAudio(t *testing.T) {
	original := []byte{0xf8, 0xff, 0xfe}
	blob := &genx.Blob{MIMEType: "audio/pcm;rate=16000;channels=1", Data: make([]byte, 640), OriginalAudio: &genx.Blob{MIMEType: "audio/opus", Data: original}}
	frame, ok := opusFrame(&genx.MessageChunk{Part: blob})
	if !ok || !bytes.Equal(frame, original) {
		t.Fatal("passthrough forwarded decoded PCM as Opus or lost the original")
	}
	blob.ConcealedAudio = true
	if _, ok := opusFrame(&genx.MessageChunk{Part: blob}); ok {
		t.Fatal("concealed audio was spliced into the original encoded stream")
	}
}
