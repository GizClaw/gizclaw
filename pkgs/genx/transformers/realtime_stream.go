package transformers

import (
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func realtimeChunkInputStreamID(chunk *genx.MessageChunk, fallback string) string {
	if chunk != nil && chunk.Ctrl != nil {
		streamID := strings.TrimSpace(chunk.Ctrl.StreamID)
		if streamID != "" && streamID != "audio" {
			return streamID
		}
	}
	return fallback
}
