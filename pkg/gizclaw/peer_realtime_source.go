package gizclaw

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/runtime/agenthost"
)

type peerRealtimeSource struct {
	mu      sync.RWMutex
	current *genx.RealtimeStream
	options []genx.RealtimeStreamOption

	audioStreamID string
}

func newPeerRealtimeSource(options ...genx.RealtimeStreamOption) *peerRealtimeSource {
	return &peerRealtimeSource{options: options}
}

func (s *peerRealtimeSource) OpenAgentInput(context.Context) (genx.Stream, error) {
	if s == nil {
		return nil, agenthost.ErrMissingSource
	}
	next := genx.NewRealtimeStream(s.options...)
	s.mu.Lock()
	previous := s.current
	s.current = next
	s.audioStreamID = ""
	s.mu.Unlock()
	if previous != nil {
		_ = previous.Close()
	}
	return next, nil
}

func (s *peerRealtimeSource) Push(ctx context.Context, chunk *genx.MessageChunk) error {
	if s == nil {
		return agenthost.ErrNoActiveInput
	}
	chunk = s.bindAudioStreamID(chunk)
	if chunk == nil {
		return nil
	}
	s.mu.RLock()
	current := s.current
	s.mu.RUnlock()
	if current == nil {
		return agenthost.ErrNoActiveInput
	}
	err := current.Push(ctx, chunk)
	if errors.Is(err, io.ErrClosedPipe) {
		return agenthost.ErrNoActiveInput
	}
	return err
}

func (s *peerRealtimeSource) bindAudioStreamID(chunk *genx.MessageChunk) *genx.MessageChunk {
	if s == nil || chunk == nil {
		return chunk
	}
	blob, _ := chunk.Part.(*genx.Blob)
	if !isOpusBlob(blob) {
		return chunk
	}
	ctrl := chunk.Ctrl
	if ctrl == nil {
		ctrl = &genx.StreamCtrl{}
		next := *chunk
		next.Ctrl = ctrl
		chunk = &next
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if !ctrl.BeginOfStream && (ctrl.StreamID == "" || ctrl.StreamID == "audio") && s.audioStreamID == "" {
		return nil
	}
	if ctrl.BeginOfStream && ctrl.StreamID != "" {
		s.audioStreamID = ctrl.StreamID
	}
	if ctrl.StreamID == "" || ctrl.StreamID == "audio" {
		if s.audioStreamID != "" {
			next := *chunk
			nextCtrl := *ctrl
			nextCtrl.StreamID = s.audioStreamID
			next.Ctrl = &nextCtrl
			chunk = &next
			ctrl = &nextCtrl
		}
	}
	if ctrl.EndOfStream && ctrl.StreamID != "" && ctrl.StreamID == s.audioStreamID {
		s.audioStreamID = ""
	}
	return chunk
}

func (s *peerRealtimeSource) Close() error {
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
