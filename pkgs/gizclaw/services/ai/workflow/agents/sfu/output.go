package sfu

import (
	"io"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// maxQueuedOutputChunks bounds the downlink backlog when the AgentHost
// consumer stalls. Audio chunks beyond the bound are dropped oldest-first;
// control chunks are never dropped so routes always close cleanly.
const maxQueuedOutputChunks = 4096

// outputStream is the push-writable genx.Stream returned by Transform. The
// session owns it: session goroutines push, the AgentHost consumer reads,
// and the session closes it exactly once when the attachment ends.
type outputStream struct {
	mu     sync.Mutex
	cond   *sync.Cond
	queue  []*genx.MessageChunk
	closed bool
	err    error
}

var _ genx.Stream = (*outputStream)(nil)

func newOutputStream() *outputStream {
	s := &outputStream{}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// push enqueues a chunk. It reports false once the stream is closed.
func (s *outputStream) push(chunk *genx.MessageChunk) bool {
	if chunk == nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	if len(s.queue) >= maxQueuedOutputChunks {
		s.dropOldestAudioLocked()
	}
	s.queue = append(s.queue, chunk)
	s.cond.Signal()
	return true
}

func (s *outputStream) dropOldestAudioLocked() {
	for i, queued := range s.queue {
		if queued.Ctrl != nil && (queued.Ctrl.BeginOfStream || queued.Ctrl.EndOfStream) {
			continue
		}
		s.queue = append(s.queue[:i], s.queue[i+1:]...)
		return
	}
}

func (s *outputStream) Next() (*genx.MessageChunk, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.queue) == 0 && !s.closed {
		s.cond.Wait()
	}
	if len(s.queue) > 0 {
		chunk := s.queue[0]
		s.queue[0] = nil
		s.queue = s.queue[1:]
		return chunk, nil
	}
	if s.err != nil {
		return nil, s.err
	}
	return nil, io.EOF
}

func (s *outputStream) Close() error {
	return s.CloseWithError(nil)
}

func (s *outputStream) CloseWithError(err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.err = err
	s.cond.Broadcast()
	return nil
}
