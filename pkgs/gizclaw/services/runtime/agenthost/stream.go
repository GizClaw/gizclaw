package agenthost

import (
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type leaseStream struct {
	genx.Stream
	once    sync.Once
	release func()
}

var _ OutputObservationStream = (*leaseStream)(nil)
var _ outputObservationAbandoner = (*leaseStream)(nil)
var _ OutputProductionObserver = (*leaseStream)(nil)

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

func (s *leaseStream) DeferOutputObservation() {
	if observer, ok := s.Stream.(OutputObservationStream); ok {
		observer.DeferOutputObservation()
	}
}

func (s *leaseStream) ObserveOutput(chunk *genx.MessageChunk) {
	if observer, ok := s.Stream.(OutputObservationStream); ok {
		observer.ObserveOutput(chunk)
	}
}

func (s *leaseStream) AbandonOutputObservation(chunk *genx.MessageChunk) {
	if abandoner, ok := s.Stream.(outputObservationAbandoner); ok {
		abandoner.AbandonOutputObservation(chunk)
	}
}

func (s *leaseStream) SetOutputProductionObserver(observe func(*genx.MessageChunk)) {
	if observer, ok := s.Stream.(OutputProductionObserver); ok {
		observer.SetOutputProductionObserver(observe)
	}
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
