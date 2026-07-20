package dashscoperealtime

import "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/internal/streamkit"

type bufferStream struct {
	*streamkit.Output
}

func newBufferStream(size int) *bufferStream {
	return &bufferStream{Output: streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: size})}
}
