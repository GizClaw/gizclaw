package agenthost

import (
	"sync"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
)

type leaseStream struct {
	genx.Stream
	once    sync.Once
	release func()
}

func (s *leaseStream) Next() (*genx.MessageChunk, error) {
	chunk, err := s.Stream.Next()
	if err != nil {
		s.releaseOnce()
	}
	return chunk, err
}

func (s *leaseStream) Close() error {
	err := s.Stream.Close()
	s.releaseOnce()
	return err
}

func (s *leaseStream) CloseWithError(err error) error {
	closeErr := s.Stream.CloseWithError(err)
	s.releaseOnce()
	return closeErr
}

func (s *leaseStream) releaseOnce() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		if s.release != nil {
			s.release()
		}
	})
}
