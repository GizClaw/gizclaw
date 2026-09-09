package doubaoast

import (
	"context"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
)

type bufferStream struct {
	*streamkit.Output
}

func newBufferStream(size int, contexts ...context.Context) *bufferStream {
	var ctx context.Context
	if len(contexts) > 0 {
		ctx = contexts[0]
	}
	return &bufferStream{Output: streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: size, LogContext: ctx})}
}

func chunkInputStreamID(chunk *genx.MessageChunk, fallback string) string {
	if chunk != nil && chunk.Ctrl != nil {
		if streamID := strings.TrimSpace(chunk.Ctrl.StreamID); streamID != "" {
			return streamID
		}
	}
	return strings.TrimSpace(fallback)
}
