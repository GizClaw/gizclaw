package doubaoasr

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

// audioPacketBuffer aggregates small live PCM frames into provider-sized
// packets. It is owned by one transformer session and reset at every route
// boundary so audio cannot leak into the next session.
type audioPacketBuffer struct {
	target int
	data   []byte
}

func (b *audioPacketBuffer) reset(target int) {
	b.target = target
	b.data = nil
}

func (b *audioPacketBuffer) append(data []byte, yield func([]byte) bool) {
	if len(data) == 0 {
		return
	}
	if b.target <= 0 {
		yield(data)
		return
	}
	for len(data) > 0 {
		remaining := b.target - len(b.data)
		consumed := min(remaining, len(data))
		b.data = append(b.data, data[:consumed]...)
		data = data[consumed:]
		if len(b.data) != b.target {
			continue
		}
		packet := b.data
		b.data = nil
		if !yield(packet) {
			return
		}
	}
}

func (b *audioPacketBuffer) flush() []byte {
	data := b.data
	b.data = nil
	return data
}
