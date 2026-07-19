package agentkit

import (
	"fmt"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// ResponseStream gives model output a response-local StreamID while preserving
// upstream IDs for user/history output. All MIME routes with the same upstream
// response identity share one fresh ID.
type ResponseStream struct {
	source genx.Stream

	mu                 sync.Mutex
	responses          map[string]*responseRouteState
	upstreamByResponse map[string]string
}

var _ genx.Stream = (*ResponseStream)(nil)

type responseRouteState struct {
	streamID string
	routes   map[string]bool
	terminal bool
}

type outputObservationStream interface {
	DeferOutputObservation()
	ObserveOutput(*genx.MessageChunk)
}

// NewResponseStream wraps a provider output stream with response-ID isolation.
func NewResponseStream(source genx.Stream) (*ResponseStream, error) {
	if source == nil {
		return nil, fmt.Errorf("agentkit: response source is required")
	}
	return &ResponseStream{
		source:             source,
		responses:          make(map[string]*responseRouteState),
		upstreamByResponse: make(map[string]string),
	}, nil
}

// Next returns the next chunk, replacing model response IDs with invocation-
// local IDs. The source chunk is never mutated.
func (s *ResponseStream) Next() (*genx.MessageChunk, error) {
	if s == nil || s.source == nil {
		return nil, fmt.Errorf("agentkit: response stream is not initialized")
	}
	chunk, err := s.source.Next()
	if err != nil || chunk == nil || chunk.Role != genx.RoleModel {
		return chunk, err
	}
	copyChunk := *chunk
	copyCtrl := genx.StreamCtrl{}
	if chunk.Ctrl != nil {
		copyCtrl = *chunk.Ctrl
	}
	copyCtrl.StreamID = s.responseID(copyCtrl.StreamID, chunk)
	copyChunk.Ctrl = &copyCtrl
	return &copyChunk, nil
}

// Close closes the wrapped provider output.
func (s *ResponseStream) Close() error {
	if s == nil || s.source == nil {
		return nil
	}
	return s.source.Close()
}

// CloseWithError closes the wrapped provider output with an error.
func (s *ResponseStream) CloseWithError(err error) error {
	if s == nil || s.source == nil {
		return nil
	}
	return s.source.CloseWithError(err)
}

// DeferOutputObservation forwards pull-visible observation control to the
// wrapped producer when it supports that optional contract.
func (s *ResponseStream) DeferOutputObservation() {
	if s == nil {
		return
	}
	if observer, ok := s.source.(outputObservationStream); ok {
		observer.DeferOutputObservation()
	}
}

// ObserveOutput acknowledges a chunk at the final pull boundary. The wrapped
// producer receives its original provider StreamID rather than the response-
// local ID exposed by this stream.
func (s *ResponseStream) ObserveOutput(chunk *genx.MessageChunk) {
	if s == nil || chunk == nil {
		return
	}
	observer, ok := s.source.(outputObservationStream)
	if !ok {
		return
	}
	observed := chunk
	if chunk.Ctrl != nil {
		s.mu.Lock()
		upstream, mapped := s.upstreamByResponse[chunk.Ctrl.StreamID]
		s.mu.Unlock()
		if mapped {
			copyChunk := *chunk
			copyCtrl := *chunk.Ctrl
			copyCtrl.StreamID = upstream
			copyChunk.Ctrl = &copyCtrl
			observed = &copyChunk
		}
	}
	observer.ObserveOutput(observed)
}

func (s *ResponseStream) responseID(upstream string, chunk *genx.MessageChunk) string {
	upstream = strings.TrimSpace(upstream)
	key := upstream
	if key == "" {
		key = "\x00anonymous"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.responses[key]
	mimeType, hasMIME := chunk.MIMEType()
	if state != nil && !chunk.IsEndOfStream() && (state.terminal || hasMIME && state.routes[mimeType] || chunk.IsBeginOfStream() && hasCompletedRoute(state.routes)) {
		state = nil
	}
	if state == nil {
		state = &responseRouteState{streamID: genx.NewStreamID(), routes: make(map[string]bool)}
		s.responses[key] = state
		s.upstreamByResponse[state.streamID] = upstream
	}
	if hasMIME {
		state.routes[mimeType] = chunk.IsEndOfStream()
	} else if chunk.IsEndOfStream() {
		state.terminal = true
	}
	return state.streamID
}

func hasCompletedRoute(routes map[string]bool) bool {
	for _, done := range routes {
		if done {
			return true
		}
	}
	return false
}
