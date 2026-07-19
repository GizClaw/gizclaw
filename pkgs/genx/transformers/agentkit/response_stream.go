package agentkit

import (
	"fmt"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

const maxRetainedCompletedResponses = 64

// ResponseStream gives model output a response-local StreamID while preserving
// upstream IDs for user/history output. All MIME routes with the same upstream
// response identity share one fresh ID.
type ResponseStream struct {
	source genx.Stream

	mu                  sync.Mutex
	responses           map[string]*responseRouteState
	pendingObservations map[string]*pendingObservation
	observationDeferred bool
	sequence            uint64
}

var _ genx.Stream = (*ResponseStream)(nil)

type responseRouteState struct {
	streamID string
	routes   map[string]bool
	terminal bool
	lastUsed uint64
}

type pendingObservation struct {
	upstreamID string
	count      uint64
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
		source:              source,
		responses:           make(map[string]*responseRouteState),
		pendingObservations: make(map[string]*pendingObservation),
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
	upstreamID := strings.TrimSpace(copyCtrl.StreamID)
	copyCtrl.StreamID = s.responseID(upstreamID, chunk)
	copyChunk.Ctrl = &copyCtrl
	result := &copyChunk
	s.mu.Lock()
	if s.observationDeferred {
		pending := s.pendingObservations[copyCtrl.StreamID]
		if pending == nil {
			pending = &pendingObservation{upstreamID: upstreamID}
			s.pendingObservations[copyCtrl.StreamID] = pending
		}
		pending.count++
	}
	s.mu.Unlock()
	return result, nil
}

// Close closes the wrapped provider output.
func (s *ResponseStream) Close() error {
	if s == nil || s.source == nil {
		return nil
	}
	err := s.source.Close()
	s.clearResponseState()
	return err
}

// CloseWithError closes the wrapped provider output with an error.
func (s *ResponseStream) CloseWithError(err error) error {
	if s == nil || s.source == nil {
		return nil
	}
	closeErr := s.source.CloseWithError(err)
	s.clearState()
	return closeErr
}

// DeferOutputObservation forwards pull-visible observation control to the
// wrapped producer when it supports that optional contract.
func (s *ResponseStream) DeferOutputObservation() {
	if s == nil {
		return
	}
	observer, ok := s.source.(outputObservationStream)
	if !ok {
		return
	}
	s.mu.Lock()
	s.observationDeferred = true
	s.mu.Unlock()
	observer.DeferOutputObservation()
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
		localID := strings.TrimSpace(chunk.Ctrl.StreamID)
		pending := s.pendingObservations[localID]
		if pending != nil {
			if pending.count > 1 {
				pending.count--
			} else {
				delete(s.pendingObservations, localID)
			}
		}
		s.mu.Unlock()
		if pending != nil {
			copyChunk := *chunk
			copyCtrl := *chunk.Ctrl
			copyCtrl.StreamID = pending.upstreamID
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
	s.sequence++
	state := s.responses[key]
	mimeType, hasMIME := chunk.MIMEType()
	if state != nil && !chunk.IsEndOfStream() &&
		(state.terminal || hasMIME && responseRoutesComplete(state)) {
		state = nil
	}
	if state == nil {
		state = &responseRouteState{streamID: genx.NewStreamID(), routes: make(map[string]bool)}
		s.responses[key] = state
	}
	state.lastUsed = s.sequence
	if hasMIME {
		state.routes[mimeType] = chunk.IsEndOfStream()
	} else if chunk.IsEndOfStream() {
		state.terminal = true
	}
	streamID := state.streamID
	if state.terminal {
		delete(s.responses, key)
	} else {
		s.pruneCompletedResponses(key)
	}
	return streamID
}

func (s *ResponseStream) pruneCompletedResponses(currentKey string) {
	for len(s.responses) > maxRetainedCompletedResponses {
		var oldestKey string
		var oldestSequence uint64
		for key, state := range s.responses {
			if key == currentKey || !responseRoutesComplete(state) {
				continue
			}
			if oldestKey == "" || state.lastUsed < oldestSequence {
				oldestKey = key
				oldestSequence = state.lastUsed
			}
		}
		if oldestKey == "" {
			return
		}
		delete(s.responses, oldestKey)
	}
}

func responseRoutesComplete(state *responseRouteState) bool {
	if state == nil {
		return false
	}
	if state.terminal {
		return true
	}
	if len(state.routes) == 0 {
		return false
	}
	for _, done := range state.routes {
		if !done {
			return false
		}
	}
	return true
}

func (s *ResponseStream) clearState() {
	s.mu.Lock()
	clear(s.responses)
	clear(s.pendingObservations)
	s.mu.Unlock()
}

func (s *ResponseStream) clearResponseState() {
	s.mu.Lock()
	clear(s.responses)
	s.mu.Unlock()
}
