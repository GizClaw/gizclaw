package dashscoperealtime

import "github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"

type bufferStream struct {
	*streamkit.Output
}

func newBufferStream(size int) *bufferStream {
	return &bufferStream{Output: streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: size})}
}
