//go:build gizclaw_e2e

package multiserver_test

import (
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestSFUForwardedOriginalAudioThreshold(t *testing.T) {
	for _, recovered := range []bool{false, true} {
		var silence, tone int
		for range 75 {
			blob := &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}}
			if recovered {
				blob = &genx.Blob{MIMEType: "audio/pcm;rate=16000;channels=1", Data: make([]byte, 640), OriginalAudio: blob}
			}
			silence += forwardedOriginalAudioBytes(blob)
		}
		for _, packet := range opusTonePackets(t, 1500*time.Millisecond) {
			blob := &genx.Blob{MIMEType: "audio/opus", Data: packet}
			if recovered {
				blob = &genx.Blob{MIMEType: "audio/pcm;rate=16000;channels=1", Data: make([]byte, 640), OriginalAudio: blob}
			}
			tone += forwardedOriginalAudioBytes(blob)
		}
		if silence != 225 || silence >= minForwardedToneBytes || tone < minForwardedToneBytes {
			t.Fatalf("recovered=%t: silence=%d tone=%d threshold=%d", recovered, silence, tone, minForwardedToneBytes)
		}
	}
	for _, blob := range []*genx.Blob{
		nil,
		{MIMEType: "audio/pcm", Data: make([]byte, 1920)},
		{MIMEType: "audio/pcm", Data: make([]byte, 1920), ConcealedAudio: true},
		{MIMEType: "audio/pcm", Data: make([]byte, 1920), ConcealedAudio: true, OriginalAudio: &genx.Blob{MIMEType: "audio/opus", Data: make([]byte, 2000)}},
	} {
		if got := forwardedOriginalAudioBytes(blob); got != 0 {
			t.Fatalf("non-original audio counted %d bytes", got)
		}
	}
}
