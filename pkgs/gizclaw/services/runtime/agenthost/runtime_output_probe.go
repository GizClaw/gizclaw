package agenthost

import (
	"fmt"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// outputProbe wraps the runtime output stream handed to the StreamConsumer and
// records the last chunk that crossed the consumer boundary. When the stream
// ends cleanly while the runtime is still active, the recorded route lets the
// failure name the stream that was in flight instead of only reporting that
// the consumer completed.
type outputProbe struct {
	genx.Stream

	mu     sync.Mutex
	chunks int
	last   outputProbeChunk
	// lastError is the most recent terminal error text observed on any route,
	// which usually names the pipeline stage that failed before the stream end.
	lastError string
}

type outputProbeChunk struct {
	streamID  string
	label     string
	mimeType  string
	role      genx.Role
	name      string
	begin     bool
	end       bool
	errorText string
}

func newOutputProbe(stream genx.Stream) *outputProbe {
	return &outputProbe{Stream: stream}
}

func (p *outputProbe) Next() (*genx.MessageChunk, error) {
	chunk, err := p.Stream.Next()
	if chunk != nil {
		p.record(chunk)
	}
	return chunk, err
}

func (p *outputProbe) record(chunk *genx.MessageChunk) {
	state := outputProbeChunk{role: chunk.Role, name: chunk.Name}
	if mimeType, ok := chunk.MIMEType(); ok {
		state.mimeType = mimeType
	}
	if chunk.Ctrl != nil {
		state.streamID = chunk.Ctrl.StreamID
		state.label = chunk.Ctrl.Label
		state.begin = chunk.Ctrl.BeginOfStream
		state.end = chunk.Ctrl.EndOfStream
		state.errorText = chunk.Ctrl.Error
	}
	p.mu.Lock()
	p.chunks++
	p.last = state
	if state.errorText != "" {
		p.lastError = state.errorText
	}
	p.mu.Unlock()
}

// DeferOutputObservation, ObserveOutput, and AbandonOutputObservation forward
// the optional delivery-acknowledgement protocol so wrapping the stream does
// not change how MixerOutput and the producer account for delivered chunks.
func (p *outputProbe) DeferOutputObservation() {
	if observer, ok := p.Stream.(OutputObservationStream); ok {
		observer.DeferOutputObservation()
	}
}

func (p *outputProbe) ObserveOutput(chunk *genx.MessageChunk) {
	if observer, ok := p.Stream.(OutputObservationStream); ok {
		observer.ObserveOutput(chunk)
	}
}

func (p *outputProbe) AbandonOutputObservation(chunk *genx.MessageChunk) {
	if abandoner, ok := p.Stream.(outputObservationAbandoner); ok {
		abandoner.AbandonOutputObservation(chunk)
	}
}

// summary describes the last observed route for diagnostics.
func (p *outputProbe) summary() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.chunks == 0 {
		return "no output chunk was observed"
	}
	var boundary []string
	if p.last.begin {
		boundary = append(boundary, "bos")
	}
	if p.last.end {
		boundary = append(boundary, "eos")
	}
	summary := fmt.Sprintf(
		"chunks=%d last_stream_id=%q last_label=%q last_mime=%q last_role=%q last_name=%q last_boundary=%q",
		p.chunks, p.last.streamID, p.last.label, p.last.mimeType, p.last.role, p.last.name, strings.Join(boundary, "+"),
	)
	if p.lastError != "" {
		summary += fmt.Sprintf(" last_route_error=%q", p.lastError)
	}
	return summary
}

func (p *outputProbe) logAttrs() []any {
	p.mu.Lock()
	defer p.mu.Unlock()
	return []any{
		"output_chunks", p.chunks,
		"last_stream_id", p.last.streamID,
		"last_label", p.last.label,
		"last_mime", p.last.mimeType,
		"last_route_error", p.lastError,
	}
}
