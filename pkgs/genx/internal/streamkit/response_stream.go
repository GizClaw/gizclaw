package streamkit

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

const maxRetainedCompletedResponses = 64

// ResponseStream gives model output a response-local StreamID while preserving
// upstream IDs for user and history output. All MIME routes with the same
// upstream response identity share one fresh ID.
type ResponseStream struct {
	source genx.Stream

	mu                  sync.Mutex
	responses           map[string]*responseRouteState
	pendingObservations map[string][]pendingObservation
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
	chunk *genx.MessageChunk
}

type outputObservationStream interface {
	DeferOutputObservation()
	ObserveOutput(*genx.MessageChunk)
}

type outputObservationAbandoner interface {
	AbandonOutputObservation(*genx.MessageChunk)
}

type outputObservationBulkAbandoner interface {
	AbandonDeferredObservations()
}

// NewResponseStream wraps a provider output stream with response-ID isolation.
func NewResponseStream(source genx.Stream) (*ResponseStream, error) {
	if source == nil {
		return nil, fmt.Errorf("streamkit: response source is required")
	}
	return &ResponseStream{
		source:              source,
		responses:           make(map[string]*responseRouteState),
		pendingObservations: make(map[string][]pendingObservation),
	}, nil
}

// Next returns the next chunk, replacing model response IDs with invocation-
// local IDs. The source chunk is never mutated.
func (s *ResponseStream) Next() (*genx.MessageChunk, error) {
	if s == nil || s.source == nil {
		return nil, fmt.Errorf("streamkit: response stream is not initialized")
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
		s.pendingObservations[copyCtrl.StreamID] = append(
			s.pendingObservations[copyCtrl.StreamID],
			pendingObservation{chunk: chunk},
		)
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
// producer receives its original provider StreamID rather than the local ID.
func (s *ResponseStream) ObserveOutput(chunk *genx.MessageChunk) {
	if s == nil || chunk == nil {
		return
	}
	observer, ok := s.source.(outputObservationStream)
	if !ok {
		return
	}
	pending, ok := s.takePendingObservation(chunk)
	if !ok {
		return
	}
	observer.ObserveOutput(pending.chunk)
}

// AbandonOutputObservation releases the wrapped producer's deferred delivery
// acknowledgement without recording the chunk as delivered. Composition
// layers call this for read-ahead output discarded by a later interruption.
func (s *ResponseStream) AbandonOutputObservation(chunk *genx.MessageChunk) {
	if s == nil || chunk == nil {
		return
	}
	abandoner, ok := s.source.(outputObservationAbandoner)
	if !ok {
		return
	}
	pending, ok := s.takePendingObservation(chunk)
	if !ok {
		return
	}
	abandoner.AbandonOutputObservation(pending.chunk)
}

// AbandonAllOutputObservations releases every source chunk already read by
// this wrapper but not acknowledged at a later delivery boundary. It returns
// the local response IDs so a composition layer can reject late chunks from
// those interrupted routes as well.
func (s *ResponseStream) AbandonAllOutputObservations() []string {
	if s == nil {
		return nil
	}
	abandoner, ok := s.source.(outputObservationAbandoner)
	bulkAbandoner, bulkOK := s.source.(outputObservationBulkAbandoner)
	if !ok && !bulkOK {
		return nil
	}
	s.mu.Lock()
	ids := make([]string, 0, len(s.pendingObservations))
	pending := make([]pendingObservation, 0)
	for localID, observations := range s.pendingObservations {
		ids = append(ids, localID)
		pending = append(pending, observations...)
	}
	clear(s.pendingObservations)
	s.mu.Unlock()
	if ok {
		for _, observation := range pending {
			abandoner.AbandonOutputObservation(observation.chunk)
		}
	}
	// A source can dequeue a chunk immediately before this wrapper records its
	// response-local observation. A bulk-capable source has no other consumer,
	// so releasing its remaining deferred observations closes that race without
	// marking discarded output as delivered.
	if bulkOK {
		bulkAbandoner.AbandonDeferredObservations()
	}
	sort.Strings(ids)
	return ids
}

func (s *ResponseStream) takePendingObservation(chunk *genx.MessageChunk) (pendingObservation, bool) {
	if s == nil || chunk == nil || chunk.Ctrl == nil {
		return pendingObservation{}, false
	}
	localID := strings.TrimSpace(chunk.Ctrl.StreamID)
	s.mu.Lock()
	defer s.mu.Unlock()
	pending := s.pendingObservations[localID]
	if len(pending) == 0 {
		return pendingObservation{}, false
	}
	result := pending[0]
	if len(pending) == 1 {
		delete(s.pendingObservations, localID)
	} else {
		var zero pendingObservation
		pending[0] = zero
		s.pendingObservations[localID] = pending[1:]
	}
	return result, true
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
		(state.terminal || hasMIME && !chunk.IsBeginOfStream() && responseRoutesComplete(state)) {
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
