package transformers

import (
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func chunkInputStreamID(chunk *genx.MessageChunk, fallback string) string {
	if chunk != nil && chunk.Ctrl != nil {
		if streamID := strings.TrimSpace(chunk.Ctrl.StreamID); streamID != "" {
			return streamID
		}
	}
	return strings.TrimSpace(fallback)
}

func pcm16LE(samples []int16) []byte {
	data := make([]byte, len(samples)*2)
	for i, sample := range samples {
		data[i*2] = byte(sample)
		data[i*2+1] = byte(uint16(sample) >> 8)
	}
	return data
}
