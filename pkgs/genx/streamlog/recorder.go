// Package streamlog records bounded text and timing summaries without changing
// GenX delivery, buffering, cancellation, or response ownership.
package streamlog

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	"github.com/GizClaw/gizclaw-go/pkgs/gizmetrics"
)

const maxRoutes = 64
const maxTextBytes = 4096

// Recorder observes one boundary of a GenX pipeline. It retains at most 64
// routes and 4096 bytes of unfinished text per route. Call Close on shutdown.
// Observe accepts text deltas; provider hypotheses must be normalized first.
type Recorder struct {
	ctx           context.Context
	logger        *slog.Logger
	boundary      string
	model         Model
	mu            sync.Mutex
	routes        map[routeKey]*route
	inputs        map[string]inputTiming
	outputLinks   []outputLink
	inputSequence uint64
	sequence      uint64
	closed        bool
}

type outputLink struct{ input, output string }

type inputTiming struct {
	started, ended time.Time
	contentStarted time.Time
	contentMIME    string
	sequence       uint64
}

type routeKey struct {
	stream string
	source string
	role   genx.Role
	epoch  *genx.ResponseEpoch
	mime   string
}

type route struct {
	key      routeKey
	started  time.Time
	sequence uint64
	text     string
	bytes    int64
	chunks   int64
	first    bool
	ended    bool
}

type entry struct {
	ctx        context.Context
	attrs      []slog.Attr
	hasContent bool
}

// New creates an observer. It starts no goroutine and does not own the stream.
func New(ctx context.Context, logger *slog.Logger, boundary string) *Recorder {
	if ctx == nil {
		ctx = context.Background()
	}
	if logger == nil {
		logger = slog.Default()
	}
	metadata, _ := ctx.Value(modelKey{}).(Model)
	return &Recorder{model: metadata, ctx: context.WithoutCancel(ctx), logger: logger, boundary: boundary, routes: make(map[routeKey]*route), inputs: make(map[string]inputTiming)}
}

// ObserveInput marks the input timing origin. Only the actual input stream ID
// or a producer-owned ResponseEpoch can associate later output with this input.
func (r *Recorder) ObserveInput(chunk *genx.MessageChunk) {
	if r == nil || chunk == nil || chunk.Ctrl == nil || chunk.Ctrl.StreamID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	id := chunk.Ctrl.StreamID
	stamp, exists := r.inputs[id]
	if !exists || chunk.IsBeginOfStream() && !stamp.ended.IsZero() {
		if !exists && len(r.inputs) >= maxRoutes {
			var oldest string
			var sequence uint64
			for key, value := range r.inputs {
				if oldest == "" || value.sequence < sequence {
					oldest, sequence = key, value.sequence
				}
			}
			delete(r.inputs, oldest)
		}
		r.inputSequence++
		stamp = inputTiming{started: time.Now(), sequence: r.inputSequence}
	}
	if stamp.contentStarted.IsZero() {
		switch part := chunk.Part.(type) {
		case genx.Text:
			if strings.TrimSpace(string(part)) != "" {
				stamp.contentStarted = time.Now()
				stamp.contentMIME = "text/plain"
			}
		case *genx.Blob:
			if part != nil && len(part.Data) > 0 {
				stamp.contentStarted = time.Now()
				stamp.contentMIME = part.MIMEType
			}
		}
	}
	if chunk.IsEndOfStream() && stamp.ended.IsZero() {
		stamp.ended = time.Now()
	}
	r.inputs[id] = stamp
}

// LinkOutput records an association supplied by the producer that creates a
// response. It never infers ownership from the latest input or changes chunks.
func (r *Recorder) LinkOutput(inputID, outputID string) {
	if r == nil || inputID == "" || outputID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	if len(r.outputLinks) == maxRoutes {
		r.outputLinks = r.outputLinks[1:]
	}
	r.outputLinks = append(r.outputLinks, outputLink{input: inputID, output: outputID})
}

// Observe records a chunk at this boundary. Empty control markers do not count
// as first text/audio. EOS closes its MIME route, not other concurrent routes.
// Logging runs after releasing the state lock.
func (r *Recorder) Observe(chunk *genx.MessageChunk) {
	if r == nil || chunk == nil {
		return
	}
	key := routeKey{role: chunk.Role}
	if chunk.Ctrl != nil {
		key.stream = chunk.Ctrl.StreamID
		key.source = chunk.Ctrl.SourceStreamID
		key.epoch = chunk.Ctrl.ResponseEpoch
	}
	var text string
	var size int
	switch part := chunk.Part.(type) {
	case genx.Text:
		key.mime = "text/plain"
		text = string(part)
		size = len(text)
	case *genx.Blob:
		if part == nil || !strings.HasPrefix(part.MIMEType, "audio/") {
			return
		}
		key.mime = part.MIMEType
		size = len(part.Data)
	default:
		if chunk.IsEndOfStream() {
			r.endControl(chunk)
		}
		return
	}
	now := time.Now()
	var entries []entry
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	state := r.routes[key]
	if state != nil && state.ended {
		if !chunk.IsBeginOfStream() {
			r.mu.Unlock()
			return
		}
		state = nil
	}
	if state == nil {
		if len(r.routes) >= maxRoutes {
			var oldest *route
			for _, candidate := range r.routes {
				if oldest == nil || candidate.sequence < oldest.sequence {
					oldest = candidate
				}
			}
			entries = append(entries, r.end(oldest, now, "capacity")...)
			delete(r.routes, oldest.key)
		}
		r.sequence++
		state = &route{key: key, started: now, sequence: r.sequence}
		r.routes[key] = state
		entries = append(entries, r.event(state, "stream_start", now))
	}
	state.chunks++
	state.bytes += int64(size)
	if size > 0 && (key.mime != "text/plain" || strings.TrimSpace(text) != "") && !state.first {
		state.first = true
		event := "first_audio"
		attrs := []slog.Attr{slog.Int("audio_bytes", size)}
		if key.mime == "text/plain" {
			event = "first_text"
			attrs = []slog.Attr{slog.String("content", boundedText(text))}
		}
		entries = append(entries, r.event(state, event, now, attrs...))
	}
	// Consume incrementally so an unusually large delta cannot enlarge retained
	// state or split UTF-8 when the bounded fallback flushes a long sentence.
	for _, char := range text {
		if len(state.text)+utf8.RuneLen(char) > maxTextBytes {
			entries = append(entries, r.flush(state, now, "size_limit")...)
		}
		state.text += string(char)
		if strings.ContainsRune("。！？.!?\n", char) {
			entries = append(entries, r.flush(state, now, "sentence")...)
		}
	}
	if chunk.IsEndOfStream() {
		entries = append(entries, r.endChunk(state, now, chunk.Ctrl)...)
	}
	r.mu.Unlock()
	r.emit(entries)
}

// Close flushes incomplete sentences and active routes exactly once. It does
// not close or cancel the observed stream.
func (r *Recorder) Close(err error) { r.finish(err, true) }

// Flush ends currently retained routes while keeping the observer available
// for a replacement runtime on the same device connection.
func (r *Recorder) Flush(err error) { r.finish(err, false) }

func (r *Recorder) finish(err error, closeRecorder bool) {
	if r == nil {
		return
	}
	result := "completed"
	if errors.Is(err, context.Canceled) {
		result = "canceled"
	} else if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, genx.ErrDone) {
		result = "error"
	}
	var entries []entry
	r.mu.Lock()
	if !r.closed {
		r.closed = closeRecorder
		now := time.Now()
		for _, state := range r.routes {
			entries = append(entries, r.end(state, now, result)...)
		}
		clear(r.routes)
	}
	r.mu.Unlock()
	r.emit(entries)
}

func (r *Recorder) endChunk(state *route, now time.Time, ctrl *genx.StreamCtrl) []entry {
	result := "completed"
	switch {
	case ctrl.Error == "interrupted":
		result = "interrupted"
	case ctrl.Error == context.Canceled.Error():
		result = "canceled"
	case ctrl.Error != "" || ctrl.ErrorCode != "":
		result = "error"
	}
	entries := r.end(state, now, result)
	if len(entries) > 0 && ctrl.ErrorCode != "" {
		last := &entries[len(entries)-1]
		last.attrs = append(last.attrs, slog.String("error_code", ctrl.ErrorCode))
	}
	return entries
}

func (r *Recorder) end(state *route, now time.Time, result string) []entry {
	if state.ended {
		return nil
	}
	entries := r.flush(state, now, "terminal")
	state.ended = true
	return append(entries, r.event(state, "stream_end", now, slog.String("result", result), slog.Int64("content_bytes", state.bytes), slog.Int64("chunks", state.chunks), slog.Time("ended_at", now)))
}

func (r *Recorder) flush(state *route, now time.Time, reason string) []entry {
	text := state.text
	state.text = ""
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return []entry{r.event(state, "text", now, slog.String("content", text), slog.String("flush_reason", reason))}
}

func (r *Recorder) event(state *route, event string, now time.Time, attrs ...slog.Attr) entry {
	ctx := gizlog.WithStreamID(r.ctx, state.key.stream)
	role := state.key.role.String()
	if state.key.role == genx.RoleModel {
		role = "assistant"
	}
	fields := []slog.Attr{slog.String("boundary", r.boundary), slog.Uint64("segment_index", state.sequence), slog.String("event", event), slog.Time("observed_at", now), slog.String("role", role), slog.String("stream_id", state.key.stream), slog.String("mime_type", state.key.mime), slog.Time("started_at", state.started), slog.Float64("duration_ms", float64(now.Sub(state.started))/float64(time.Millisecond))}
	input := state.key.epoch.InputStreamID()
	if input == "" {
		for _, link := range slices.Backward(r.outputLinks) {
			if link.output == state.key.stream {
				input = link.input
				break
			}
		}
	}
	if state.key.source != "" {
		fields = append(fields, slog.String("source_stream_id", state.key.source))
		if input == "" && state.key.epoch == nil {
			if _, known := r.inputs[state.key.source]; known {
				input = state.key.source
			}
		}
	}
	owner := state.key.stream
	if input != "" {
		owner = input
		fields = append(fields, slog.String("input_stream_id", input))
	}
	if stamp, ok := r.inputs[owner]; ok {
		fields = append(fields, slog.Time("input_started_at", stamp.started), slog.Float64("input_elapsed_ms", float64(now.Sub(stamp.started))/float64(time.Millisecond)))
		if !stamp.contentStarted.IsZero() {
			fields = append(fields, slog.String("input_mime_type", stamp.contentMIME), slog.Float64("input_content_elapsed_ms", float64(now.Sub(stamp.contentStarted))/float64(time.Millisecond)))
		}
		if !stamp.ended.IsZero() {
			fields = append(fields, slog.Time("input_ended_at", stamp.ended), slog.Float64("after_input_end_ms", float64(now.Sub(stamp.ended))/float64(time.Millisecond)))
		}
	}
	fields = append(fields, attrs...)
	return entry{ctx: ctx, attrs: fields, hasContent: state.first}
}

func (r *Recorder) emit(entries []entry) {
	for _, item := range entries {
		r.logger.LogAttrs(item.ctx, slog.LevelInfo, "genx: stream", item.attrs...)
		recordMetrics(item.ctx, item.attrs)
		r.recordStageMetrics(item.ctx, item.attrs, item.hasContent)
	}
}

func boundedText(text string) string {
	if len(text) <= maxTextBytes {
		return text
	}
	end := maxTextBytes
	for !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end]
}

func (r *Recorder) endControl(chunk *genx.MessageChunk) {
	var entries []entry
	r.mu.Lock()
	if !r.closed {
		now := time.Now()
		for key, state := range r.routes {
			if key.stream == chunk.Ctrl.StreamID && key.role == chunk.Role && key.epoch == chunk.Ctrl.ResponseEpoch {
				entries = append(entries, r.endChunk(state, now, chunk.Ctrl)...)
			}
		}
	}
	r.mu.Unlock()
	r.emit(entries)
}

func recordMetrics(ctx context.Context, attrs []slog.Attr) {
	var event, boundary, role string
	var latency, afterEnd float64
	var ended bool
	known := false
	for _, attr := range attrs {
		switch attr.Key {
		case "event":
			event = attr.Value.String()
		case "boundary":
			boundary = attr.Value.String()
		case "role":
			role = attr.Value.String()
		case "after_input_end_ms":
			afterEnd = attr.Value.Float64() / 1000
			ended = true
		case "input_elapsed_ms":
			latency = attr.Value.Float64() / 1000
			known = true
		}
	}
	if !known || (event != "first_text" && event != "first_audio") {
		return
	}
	// Boundary is a code-owned dimension at the installed device integrations.
	switch boundary {
	case "model_output", "peer_delivery":
	default:
		return
	}
	if ended && afterEnd >= 0 {
		gizmetrics.ObserveDuration(ctx, "genx_input_end_to_first_output_seconds", time.Duration(afterEnd*float64(time.Second)), gizmetrics.Label{Name: "boundary", Value: boundary}, gizmetrics.Label{Name: "event", Value: event}, gizmetrics.Label{Name: "role", Value: role})
	}
	gizmetrics.ObserveHistogram(ctx, "genx_input_to_first_output_seconds", latency, []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
		gizmetrics.Label{Name: "boundary", Value: boundary}, gizmetrics.Label{Name: "event", Value: event}, gizmetrics.Label{Name: "role", Value: role})
}
