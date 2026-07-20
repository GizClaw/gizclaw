package dashscoperealtime

import "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/agentkit"

type bufferStream struct {
	*agentkit.Output
}

func newBufferStream(size int) *bufferStream {
	return &bufferStream{Output: agentkit.NewOutput(agentkit.OutputConfig{InitialCapacity: size})}
}
