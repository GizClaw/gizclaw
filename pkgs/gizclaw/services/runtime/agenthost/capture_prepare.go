package agenthost

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// PrepareSpeechOutput keeps one PTT capture in one response before external
// synthesis. The AST factory injects this runtime adapter into its otherwise
// mode-independent GenX composition. Direct GenX callers retain provider routes.
func PrepareSpeechOutput(ctx context.Context, source genx.Stream) (genx.Stream, error) {
	if source == nil {
		return nil, errors.New("agenthost: speech output is required")
	}
	gate, _ := ctx.Value(activeCaptureOutputKey{}).(*captureOutput)
	if gate == nil {
		return source, nil
	}
	return &capturePreparedOutput{source: source, gate: gate, observer: deferOutputObservation(source), originals: make(map[*genx.MessageChunk]*genx.MessageChunk)}, nil
}

type capturePreparedRoute struct {
	prototype *genx.MessageChunk
	lastByte  byte
	separate  bool
	flushed   bool
}

type capturePreparedOutput struct {
	source   genx.Stream
	gate     *captureOutput
	observer OutputObservationStream
	epoch    *genx.ResponseEpoch
	modelID  string
	routes   map[string]*capturePreparedRoute
	order    []string
	queue    []*genx.MessageChunk
	finished bool
	rtEnded  map[string]bool

	mu        sync.Mutex
	deferred  bool
	closed    bool
	originals map[*genx.MessageChunk]*genx.MessageChunk
}

func (s *capturePreparedOutput) Next() (*genx.MessageChunk, error) {
	for {
		if len(s.queue) != 0 {
			chunk := s.queue[0]
			s.queue = s.queue[1:]
			return chunk, nil
		}
		chunk, err := s.source.Next()
		if err != nil || chunk == nil || chunk.Ctrl == nil {
			return chunk, err
		}
		epoch := chunk.Ctrl.ResponseEpoch
		s.gate.mu.Lock()
		turn := s.gate.turn
		owned := turn != nil && epoch == turn.epoch
		ptt := owned && turn.audio && turn.ptt
		stale := turn != nil && epoch != nil && epoch.InputStreamID() != "" && !owned
		s.gate.mu.Unlock()
		complete := chunk.Role == genx.RoleModel && chunk.Part == nil && chunk.Ctrl.ResponseEpochEnd && !chunk.IsBeginOfStream() && !chunk.IsEndOfStream()
		if stale || complete && !ptt {
			s.abandon(chunk)
			continue
		}
		if !ptt {
			// Realtime subtitles close their text route as soon as confirmed.
			// PTT instead keeps that boundary private for one capture response.
			if _, text := chunk.Part.(genx.Text); text && chunk.Role == genx.RoleModel {
				id := chunk.Ctrl.StreamID
				if chunk.IsEndOfStream() && s.rtEnded[id] && !captureHasPayload(chunk.Part) && chunk.Ctrl.Error == "" && chunk.Ctrl.ErrorCode == "" {
					delete(s.rtEnded, id)
					s.abandon(chunk)
					continue
				}
				if captureHasPayload(chunk.Part) {
					delete(s.rtEnded, id)
				}
				if chunk.Ctrl.MessageEnd {
					if s.rtEnded == nil {
						s.rtEnded = make(map[string]bool)
					}
					s.rtEnded[id] = true
					terminal := chunk.Clone()
					terminal.Ctrl.EndOfStream, terminal.Ctrl.MessageEnd = true, false
					return s.forward(terminal, chunk), nil
				}
			}
			return s.forward(chunk, chunk), nil
		}
		if s.epoch != epoch {
			s.epoch, s.modelID, s.finished = epoch, "", false
			s.routes = make(map[string]*capturePreparedRoute)
			s.order = nil
		}
		if complete {
			if !s.finished {
				s.finish()
			}
			s.abandon(chunk)
			continue
		}
		if s.finished {
			s.abandon(chunk)
			continue
		}
		mime, ok := chunk.MIMEType()
		if !ok {
			return s.forward(chunk, chunk), nil
		}
		key := string(chunk.Role) + "\x00" + chunk.Ctrl.Label + "\x00" + mime
		route := s.routes[key]
		result := chunk.Clone()
		if chunk.Role == genx.RoleModel {
			if s.modelID == "" {
				s.modelID = chunk.Ctrl.StreamID
			}
			result.Ctrl.StreamID = s.modelID
		} else {
			result.Ctrl.StreamID = epoch.InputStreamID()
		}
		if route == nil {
			route = &capturePreparedRoute{prototype: result.Clone()}
			s.routes[key] = route
			s.order = append(s.order, key)
		} else if result.IsBeginOfStream() {
			result.Ctrl.BeginOfStream = false
			if !captureHasPayload(result.Part) {
				s.abandon(chunk)
				continue
			}
		}
		if text, ok := result.Part.(genx.Text); ok && text != "" {
			if route.separate && asciiWordByte(route.lastByte) && asciiWordByte(string(text)[0]) {
				text = " " + text
				result.Part = text
			}
			route.lastByte = string(text)[len(text)-1]
			route.separate, route.flushed = false, false
		}
		if result.Ctrl.MessageEnd {
			route.flushed = true
		}
		if result.IsEndOfStream() && result.Ctrl.Error == "" && result.Ctrl.ErrorCode == "" {
			result.Ctrl.EndOfStream, result.Ctrl.ResponseEpochEnd = false, false
			route.separate = true
			_, text := result.Part.(genx.Text)
			if text && result.Role == genx.RoleModel && !route.flushed {
				result.Ctrl.MessageEnd = true
				route.flushed = true
			} else if !captureHasPayload(result.Part) {
				s.abandon(chunk)
				continue
			}
		}
		return s.forward(result, chunk), nil
	}
}

func (s *capturePreparedOutput) finish() {
	s.finished = true
	// A final audio EOS comes after text, so its delivery waits for mixer drain.
	for _, role := range []genx.Role{genx.RoleUser, genx.RoleModel} {
		for _, audio := range []bool{false, true} {
			for _, key := range s.order {
				route := s.routes[key]
				chunk := route.prototype
				mime, _ := chunk.MIMEType()
				if chunk.Role != role || captureAudioMIME(mime) != audio {
					continue
				}
				terminal := chunk.Clone()
				terminal.Part = emptyCapturePart(chunk.Part)
				terminal.Ctrl.BeginOfStream = false
				terminal.Ctrl.EndOfStream = true
				terminal.Ctrl.MessageEnd = false
				terminal.Ctrl.ResponseEpochEnd = false
				s.queue = append(s.queue, terminal)
			}
		}
	}
	if len(s.queue) != 0 {
		s.queue[len(s.queue)-1].Ctrl.ResponseEpochEnd = true
	}
}

func (s *capturePreparedOutput) forward(chunk, original *genx.MessageChunk) *genx.MessageChunk {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		s.abandon(original)
		return chunk
	}
	deferred := s.deferred
	if deferred {
		s.originals[chunk] = original
	}
	s.mu.Unlock()
	if !deferred && s.observer != nil {
		s.observer.ObserveOutput(original)
	}
	return chunk
}

func (s *capturePreparedOutput) DeferOutputObservation() {
	s.mu.Lock()
	s.deferred = true
	s.mu.Unlock()
}

func (s *capturePreparedOutput) ObserveOutput(chunk *genx.MessageChunk) {
	if original := s.takeOriginal(chunk); original != nil && s.observer != nil {
		s.observer.ObserveOutput(original)
	}
}

func (s *capturePreparedOutput) AbandonOutputObservation(chunk *genx.MessageChunk) {
	if original := s.takeOriginal(chunk); original != nil {
		s.abandon(original)
	}
}

func (s *capturePreparedOutput) takeOriginal(chunk *genx.MessageChunk) *genx.MessageChunk {
	s.mu.Lock()
	defer s.mu.Unlock()
	original := s.originals[chunk]
	delete(s.originals, chunk)
	return original
}

func (s *capturePreparedOutput) abandon(chunk *genx.MessageChunk) {
	if observer, ok := s.source.(outputObservationAbandoner); ok {
		observer.AbandonOutputObservation(chunk)
	}
}

func (s *capturePreparedOutput) Close() error {
	s.abandonPending()
	return s.source.Close()
}
func (s *capturePreparedOutput) CloseWithError(err error) error {
	s.abandonPending()
	return s.source.CloseWithError(err)
}

func (s *capturePreparedOutput) abandonPending() {
	s.mu.Lock()
	s.closed = true
	pending := make([]*genx.MessageChunk, 0, len(s.originals))
	for _, original := range s.originals {
		pending = append(pending, original)
	}
	clear(s.originals)
	s.mu.Unlock()
	for _, original := range pending {
		s.abandon(original)
	}
}

func asciiWordByte(b byte) bool {
	return strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", rune(b))
}
