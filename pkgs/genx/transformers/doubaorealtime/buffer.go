package doubaorealtime

import (
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/internal/streamkit"
)

type bufferStream struct {
	*streamkit.Output
}

func newBufferStream(size int) *bufferStream {
	return &bufferStream{Output: streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: size})}
}

func (s *bufferStream) setOutputObserver(observe func(*genx.MessageChunk)) {
	if s != nil {
		s.SetOutputObserver(observe)
	}
}

func (s *bufferStream) discard(predicate func(*genx.MessageChunk) bool) int {
	if s == nil || s.Output == nil {
		return 0
	}
	return s.Discard(predicate)
}
