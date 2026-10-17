package doubaoasr

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

const asrInputQueueChunks = 4096

var errASRInputQueueLimit = errors.New("doubao asr: input queue limit exceeded")

type asrInputRead struct {
	chunk *genx.MessageChunk
	err   error
	bytes int64
}

// asrInput watches replacement BOS while the ASR sender is recovering. Its
// bounded read-ahead lets a replacement cancel a blocked recovery upload.
type asrInput struct {
	ctx    context.Context
	reads  chan asrInputRead
	queued atomic.Int64
	active atomic.Pointer[recoveringSession]
	latest atomic.Pointer[string]
}

func newASRInput(ctx context.Context, input genx.Stream, output *bufferStream) *asrInput {
	s := &asrInput{ctx: ctx, reads: make(chan asrInputRead, asrInputQueueChunks)}
	go func() {
		defer close(s.reads)
		for {
			chunk, err := input.Next()
			var size int64
			if chunk != nil {
				if chunk.IsBeginOfStream() && chunk.Ctrl != nil {
					if id := strings.TrimSpace(chunk.Ctrl.StreamID); id != "" {
						s.latest.Store(&id)
						if active := s.active.Load(); active != nil {
							s.preempt(active)
						}
					}
				}
				if blob, ok := chunk.Part.(*genx.Blob); ok && blob != nil {
					size = int64(len(blob.Data))
				}
			}
			if s.queued.Add(size) > asrRecoveryByteLimit {
				_ = output.Fail(errASRInputQueueLimit)
				return
			}
			select {
			case s.reads <- asrInputRead{chunk: chunk, err: err, bytes: size}:
			case <-ctx.Done():
				return
			default:
				_ = output.Fail(errASRInputQueueLimit)
				return
			}
			if err != nil {
				return
			}
		}
	}()
	return s
}

func (s *asrInput) preempt(active *recoveringSession) {
	latest := s.latest.Load()
	stream, _ := active.route.current()
	if latest != nil && *latest != stream && active.recovering.Load() {
		active.route.interrupt("interrupted")
		active.cancel()
	}
}

func (s *asrInput) activate(active *recoveringSession) {
	s.active.Store(active)
	active.onRetry = func() { s.preempt(active) }
}

func (s *asrInput) next() (*genx.MessageChunk, error) {
	select {
	case read, ok := <-s.reads:
		if !ok {
			if err := s.ctx.Err(); err != nil {
				return nil, err
			}
			return nil, io.EOF
		}
		s.queued.Add(-read.bytes)
		return read.chunk, read.err
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}
