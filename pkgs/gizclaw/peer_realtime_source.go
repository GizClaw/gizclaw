package gizclaw

import (
	"context"
	"errors"
	"io"
	"mime"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
)

type peerRealtimeSource struct {
	mu      sync.RWMutex
	current *genx.RealtimeStream
	options []genx.RealtimeStreamOption

	audioStreamID        string
	audioMIMEType        string
	lifecycle            *peerStreamLifecycle
	onInputRouteReplaced func(context.Context, peerAudioInputRoute) error
}

type peerAudioInputRoute struct {
	streamID string
	mimeType string
}

func newPeerRealtimeSource(options ...genx.RealtimeStreamOption) *peerRealtimeSource {
	return &peerRealtimeSource{options: options}
}

func newPeerRealtimeSourceWithLifecycle(lifecycle *peerStreamLifecycle, options ...genx.RealtimeStreamOption) *peerRealtimeSource {
	return &peerRealtimeSource{options: options, lifecycle: lifecycle}
}

func newPeerRealtimeSourceWithRouteReplacement(lifecycle *peerStreamLifecycle, onReplaced func(context.Context, peerAudioInputRoute) error, options ...genx.RealtimeStreamOption) *peerRealtimeSource {
	return &peerRealtimeSource{options: options, lifecycle: lifecycle, onInputRouteReplaced: onReplaced}
}

func (s *peerRealtimeSource) OpenAgentInput(ctx context.Context) (genx.Stream, error) {
	if s == nil {
		return nil, agenthost.ErrMissingSource
	}
	next := genx.NewRealtimeStream(s.options...)
	s.mu.Lock()
	previous := s.current
	replacedRoute := peerAudioInputRoute{streamID: s.audioStreamID, mimeType: s.audioMIMEType}
	s.current = next
	s.audioStreamID = ""
	s.audioMIMEType = ""
	s.mu.Unlock()
	if previous != nil {
		_ = previous.Close()
	}
	if replacedRoute.streamID != "" && s.onInputRouteReplaced != nil {
		if err := s.onInputRouteReplaced(ctx, replacedRoute); err != nil {
			s.mu.Lock()
			if s.current == next {
				s.current = nil
			}
			s.mu.Unlock()
			_ = next.Close()
			return nil, err
		}
	}
	if s.lifecycle == nil {
		return next, nil
	}
	s.lifecycle.observeAgentInputOpen()
	return &peerObservedAgentInput{Stream: next, lifecycle: s.lifecycle}, nil
}

// peerObservedAgentInput reports the point at which the selected Agent's
// transformer actually consumes an input chunk. Push observation alone only
// proves that the connection accepted the chunk into its queue.
type peerObservedAgentInput struct {
	genx.Stream
	lifecycle *peerStreamLifecycle
}

func (s *peerObservedAgentInput) Next() (*genx.MessageChunk, error) {
	chunk, err := s.Stream.Next()
	if err == nil {
		s.lifecycle.observeAgentTransformStarted(chunk)
	}
	return chunk, err
}

func (s *peerRealtimeSource) Push(ctx context.Context, chunk *genx.MessageChunk) error {
	if s == nil {
		return agenthost.ErrNoActiveInput
	}
	s.mu.Lock()
	current := s.current
	if current == nil {
		s.mu.Unlock()
		return agenthost.ErrNoActiveInput
	}
	chunk = s.bindAudioStreamIDLocked(chunk)
	s.mu.Unlock()
	if chunk == nil {
		return nil
	}
	err := current.Push(ctx, chunk)
	if errors.Is(err, io.ErrClosedPipe) {
		return agenthost.ErrNoActiveInput
	}
	if err == nil && s.lifecycle != nil {
		s.lifecycle.observeAgentInputPush(chunk)
	}
	return err
}

// bindAudioStreamIDLocked binds continuous audio chunks to the active logical
// route. The caller holds s.mu so route state and the selected input stream
// belong to the same source generation.
func (s *peerRealtimeSource) bindAudioStreamIDLocked(chunk *genx.MessageChunk) *genx.MessageChunk {
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

	if !ctrl.BeginOfStream && (ctrl.StreamID == "" || ctrl.StreamID == "audio") && s.audioStreamID == "" {
		return nil
	}
	if ctrl.BeginOfStream && ctrl.StreamID != "" {
		s.audioStreamID = ctrl.StreamID
		s.audioMIMEType = canonicalAudioMIMEType(blob.MIMEType)
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
		s.audioMIMEType = ""
	}
	return chunk
}

func canonicalAudioMIMEType(value string) string {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil || !strings.HasPrefix(strings.ToLower(mediaType), "audio/") {
		return "audio/opus"
	}
	return strings.ToLower(mediaType)
}

func (s *peerRealtimeSource) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	current := s.current
	s.current = nil
	s.audioStreamID = ""
	s.audioMIMEType = ""
	s.mu.Unlock()
	if current != nil {
		return current.Close()
	}
	return nil
}
