package dashscoperealtime

import (
	"context"

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
