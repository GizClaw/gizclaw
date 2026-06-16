package agenthost

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
)

var ErrNoActiveInput = errors.New("agenthost: no active input stream")

type PushSource struct {
	size int

	mu      sync.RWMutex
	current *InputStream
}

func NewPushSource(size int) *PushSource {
	if size <= 0 {
		size = 1
	}
	return &PushSource{size: size}
}

func (s *PushSource) OpenAgentInput(context.Context) (genx.Stream, error) {
	if s == nil {
		return nil, ErrMissingSource
	}
	next := NewInputStream(s.size)
	s.mu.Lock()
	previous := s.current
	s.current = next
	s.mu.Unlock()
	if previous != nil {
		_ = previous.Close()
	}
	return next, nil
}

func (s *PushSource) Push(ctx context.Context, chunk *genx.MessageChunk) error {
	if s == nil {
		return ErrNoActiveInput
	}
	s.mu.RLock()
	current := s.current
	s.mu.RUnlock()
	if current == nil {
		return ErrNoActiveInput
	}
	err := current.Push(ctx, chunk)
	if errors.Is(err, io.ErrClosedPipe) {
		return ErrNoActiveInput
	}
	return err
}

func (s *PushSource) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	current := s.current
	s.current = nil
	s.mu.Unlock()
	if current != nil {
		return current.Close()
	}
	return nil
}
