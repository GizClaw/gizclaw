package agenthost

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
)

type InputStream struct {
	ch       chan *genx.MessageChunk
	done     chan struct{}
	once     sync.Once
	mu       sync.Mutex
	err      error
	doneFlag atomic.Bool
}

func NewInputStream(size int) *InputStream {
	if size <= 0 {
		size = 1
	}
	return &InputStream{
		ch:   make(chan *genx.MessageChunk, size),
		done: make(chan struct{}),
	}
}

func (s *InputStream) Push(ctx context.Context, chunk *genx.MessageChunk) error {
	if s == nil {
		return io.ErrClosedPipe
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if s.doneFlag.Load() {
		return io.ErrClosedPipe
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return io.ErrClosedPipe
	case s.ch <- chunk:
		if s.doneFlag.Load() {
			return io.ErrClosedPipe
		}
		return nil
	}
}

func (s *InputStream) Next() (*genx.MessageChunk, error) {
	if s == nil {
		return nil, io.ErrClosedPipe
	}
	select {
	case chunk := <-s.ch:
		return chunk, nil
	case <-s.done:
		select {
		case chunk := <-s.ch:
			return chunk, nil
		default:
		}
		s.mu.Lock()
		err := s.err
		s.mu.Unlock()
		if err == nil {
			err = io.EOF
		}
		return nil, err
	}
}

func (s *InputStream) Close() error {
	return s.CloseWithError(io.EOF)
}

func (s *InputStream) CloseWithError(err error) error {
	if s == nil {
		return nil
	}
	if err == nil {
		err = io.ErrClosedPipe
	}
	s.once.Do(func() {
		s.doneFlag.Store(true)
		s.mu.Lock()
		s.err = err
		s.mu.Unlock()
		close(s.done)
	})
	return nil
}

func (s *InputStream) closed() bool {
	if s == nil {
		return true
	}
	return s.doneFlag.Load()
}

func IsStreamDone(err error) bool {
	return errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF)
}
