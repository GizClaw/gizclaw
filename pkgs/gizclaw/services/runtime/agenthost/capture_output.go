package agenthost

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

const (
	captureAudioLimit = 2 * time.Minute
	captureBytesLimit = 16 << 20
	captureChunkLimit = 16384
)

var errCaptureOutputLimit = errors.New("agenthost: push-to-talk output buffer limit exceeded")

type captureOutputKey struct{}
type activeCaptureOutputKey struct{}

// captureOutputAgent applies the interaction policy before History observes
// output. Its state belongs to Transform, never to the shared Workspace Agent.
type captureOutputAgent struct {
	Agent
	mode apitypes.WorkspaceInputMode
}

func (a captureOutputAgent) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	gate, pushed := ctx.Value(captureOutputKey{}).(*captureOutput)
	// Nested Workflow calls must create their own attachment policy.
	ctx = context.WithValue(ctx, captureOutputKey{}, (*captureOutput)(nil))
	if a.mode == "" {
		return a.Agent.Transform(ctx, input)
	}
	if input == nil {
		return a.Agent.Transform(ctx, input)
	}
	if !pushed || gate == nil {
		gate = newCaptureOutput()
		pushed = false
	}
	ctx, cancel := context.WithCancelCause(ctx)
	gate.mode = a.mode
	gate.cancel = cancel
	gate.input = input
	gate.enabled = true
	ctx = context.WithValue(ctx, activeCaptureOutputKey{}, gate)
	if !pushed {
		// Direct Host callers have no Service admission boundary. Read ahead at
		// this runtime boundary instead of waiting for the provider's uploader.
		queue := NewInputStream(256)
		captureInput := input
		go func() {
			for {
				chunk, err := captureInput.Next()
				if err != nil {
					gate.endInput()
					_ = queue.CloseWithError(err)
					return
				}
				if err := gate.pushInput(ctx, queue, chunk); err != nil {
					_ = queue.CloseWithError(err)
					return
				}
			}
		}()
		input = queue
	}
	output, err := a.Agent.Transform(ctx, input)
	if err != nil || output == nil {
		cancel(err)
		_ = gate.input.Close()
		if err == nil {
			err = errors.New("agenthost: agent returned nil stream")
		}
		return nil, err
	}
	gate.source = output
	gate.observer = deferOutputObservation(output)
	context.AfterFunc(ctx, func() {
		gate.abort(context.Cause(ctx))
		_ = gate.input.CloseWithError(context.Cause(ctx))
		_ = input.CloseWithError(context.Cause(ctx))
		_ = output.CloseWithError(context.Cause(ctx))
	})
	go gate.copy()
	return gate, nil
}

type captureTurn struct {
	epoch    *genx.ResponseEpoch
	audio    bool
	ptt      bool
	ended    bool
	failed   bool
	bytes    int
	chunks   int
	duration time.Duration
}

type captureOutput struct {
	mode     apitypes.WorkspaceInputMode
	enabled  bool
	cancel   context.CancelCauseFunc
	input    genx.Stream
	source   genx.Stream
	observer OutputObservationStream

	mu       sync.Mutex
	cond     *sync.Cond
	turn     *captureTurn
	retained []*genx.MessageChunk
	ready    []*genx.MessageChunk
	routes   map[string]*genx.MessageChunk
	original map[*genx.MessageChunk]*genx.MessageChunk
	deferred bool
	ended    bool
	err      error
}

func newCaptureOutput() *captureOutput {
	g := &captureOutput{routes: make(map[string]*genx.MessageChunk), original: make(map[*genx.MessageChunk]*genx.MessageChunk)}
	g.cond = sync.NewCond(&g.mu)
	return g
}

// pushInput runs inside Service's existing admission/transition boundary.
// EOS releases output after admission, independently of when AST reads it.
func (g *captureOutput) pushInput(ctx context.Context, input InputPusher, chunk *genx.MessageChunk) error {
	if !g.enabled || chunk == nil {
		return input.Push(ctx, chunk)
	}
	g.mu.Lock()
	var discarded []*genx.MessageChunk
	if chunk.IsBeginOfStream() {
		discarded = append(discarded, g.retained...)
		kept := make([]*genx.MessageChunk, 0, len(g.ready))
		for _, pending := range g.ready {
			if pending.Role == genx.RoleModel {
				discarded = append(discarded, pending)
			} else {
				kept = append(kept, pending)
			}
		}
		g.retained, g.ready = nil, kept
		for _, route := range g.routes {
			terminal := route.Clone()
			terminal.Ctrl.BeginOfStream = false
			terminal.Ctrl.EndOfStream = true
			terminal.Ctrl.Error = "interrupted"
			terminal.Ctrl.ErrorCode = "STREAM_INTERRUPTED"
			terminal.Part = emptyCapturePart(route.Part)
			g.ready = append(g.ready, terminal)
		}
		clear(g.routes)
		mode := chunk.Ctrl.InputMode
		if mode == "" {
			mode = string(g.mode)
		}
		g.turn = &captureTurn{epoch: genx.NewResponseEpoch(chunk.Ctrl.StreamID), ptt: mode != "realtime"}
	}
	turn := g.turn
	if turn != nil && chunk.Ctrl != nil && (chunk.Ctrl.StreamID == turn.epoch.InputStreamID() || chunk.Ctrl.StreamID == "" || chunk.Ctrl.StreamID == "audio") {
		if blob, ok := chunk.Part.(*genx.Blob); ok && blob != nil && captureAudioMIME(blob.MIMEType) {
			turn.audio = true
		}
		chunk = chunk.Clone()
		chunk.Ctrl.ResponseEpoch = turn.epoch
	}
	g.cond.Broadcast()
	g.mu.Unlock()
	g.abandon(discarded)
	err := input.Push(ctx, chunk)
	if err != nil || chunk.Ctrl == nil || !chunk.IsEndOfStream() || turn == nil || chunk.Ctrl.ResponseEpoch != turn.epoch {
		return err
	}
	g.mu.Lock()
	if g.turn == turn {
		turn.ended = true
		if chunk.Ctrl.Error != "" || chunk.Ctrl.ErrorCode != "" {
			if !turn.failed {
				terminal := chunk.Clone()
				terminal.Role = genx.RoleModel
				terminal.Part = genx.Text("")
				terminal.Ctrl.BeginOfStream = false
				terminal.Ctrl.Label = "assistant"
				g.ready = append(g.ready, terminal)
			}
			turn.failed = true
			discarded = g.retained
			g.retained = nil
		} else {
			discarded = nil
			g.ready = append(g.ready, g.retained...)
			g.retained = nil
		}
		g.cond.Broadcast()
	}
	g.mu.Unlock()
	g.abandon(discarded)
	return nil
}

func (g *captureOutput) copy() {
	for {
		chunk, err := g.source.Next()
		if err != nil {
			g.mu.Lock()
			g.ended = true
			var discarded []*genx.MessageChunk
			if !IsStreamDone(err) {
				g.err = err
				discarded, g.retained = g.retained, nil
			}
			g.cond.Broadcast()
			g.mu.Unlock()
			g.abandon(discarded)
			return
		}
		if chunk == nil {
			continue
		}
		g.mu.Lock()
		turn := g.turn
		var discarded []*genx.MessageChunk
		owned, stale := g.ownership(chunk, turn)
		if owned && turn.audio && turn.ptt && (chunk.Ctrl.MessageEnd || chunk.Ctrl.MessageID != "") {
			if chunk.Ctrl.MessageEnd && !chunk.IsBeginOfStream() && !chunk.IsEndOfStream() && !captureHasPayload(chunk.Part) {
				g.mu.Unlock()
				g.abandon([]*genx.MessageChunk{chunk})
				continue
			}
			original := chunk
			chunk = chunk.Clone()
			chunk.Ctrl.MessageEnd = false
			chunk.Ctrl.MessageID = ""
			g.original[chunk] = original
		}
		switch {
		case g.err != nil || stale:
			discarded = append(discarded, chunk)
		case owned && chunk.IsEndOfStream() && (chunk.Ctrl.Error != "" || chunk.Ctrl.ErrorCode != ""):
			turn.failed = true
			discarded, g.retained = g.retained, nil
			if turn.audio && turn.ptt && !turn.ended && captureHasPayload(chunk.Part) {
				discarded = append(discarded, chunk)
				chunk = chunk.Clone()
				chunk.Part = emptyCapturePart(chunk.Part)
			}
			g.ready = append(g.ready, chunk)
		case owned && turn.failed:
			discarded = append(discarded, chunk)
		case owned && turn.audio && turn.ptt && !turn.ended:
			if err := turn.retain(chunk); err != nil {
				turn.failed = true
				discarded, g.retained = append(g.retained, chunk), nil
				terminal := &genx.MessageChunk{Role: genx.RoleModel, Ctrl: &genx.StreamCtrl{StreamID: chunk.Ctrl.StreamID, Label: "assistant", EndOfStream: true, Error: err.Error(), ErrorCause: err, ResponseEpoch: turn.epoch}}
				g.ready = append(g.ready, terminal)
			} else {
				g.retained = append(g.retained, chunk)
			}
		default:
			g.ready = append(g.ready, chunk)
		}
		g.cond.Broadcast()
		g.mu.Unlock()
		g.abandon(discarded)
	}
}

func (g *captureOutput) endInput() {
	g.mu.Lock()
	var discarded []*genx.MessageChunk
	if g.turn != nil && g.turn.ptt && !g.turn.ended {
		g.turn.failed = true
		discarded, g.retained = g.retained, nil
	}
	g.cond.Broadcast()
	g.mu.Unlock()
	g.abandon(discarded)
}

func (g *captureOutput) ownership(chunk *genx.MessageChunk, turn *captureTurn) (owned, stale bool) {
	if turn == nil || chunk.Ctrl == nil {
		return false, false
	}
	epoch := chunk.Ctrl.ResponseEpoch
	if epoch != nil && epoch.InputStreamID() != "" {
		return epoch == turn.epoch, epoch != turn.epoch
	}
	// Older adapters may keep the input route ID without attaching an epoch.
	return chunk.Ctrl.StreamID == turn.epoch.InputStreamID(), false
}

func (t *captureTurn) retain(chunk *genx.MessageChunk) error {
	if chunk.Role != genx.RoleModel {
		return nil
	}
	var size int
	switch part := chunk.Part.(type) {
	case genx.Text:
		size = len(part)
	case *genx.Blob:
		if part != nil {
			size = len(part.Data)
			if strings.EqualFold(strings.SplitN(part.MIMEType, ";", 2)[0], "audio/opus") && size > 0 {
				ticks := codecconv.OpusPacketRTPTicks(part.Data)
				if ticks == 0 {
					ticks = 960
				}
				t.duration += time.Duration(ticks) * time.Second / 48000
			}
		}
	}
	t.bytes += size
	t.chunks++
	if t.duration > captureAudioLimit || t.bytes > captureBytesLimit || t.chunks > captureChunkLimit {
		return fmt.Errorf("%w (audio %s, bytes %d, chunks %d)", errCaptureOutputLimit, t.duration, t.bytes, t.chunks)
	}
	return nil
}

func (g *captureOutput) Next() (*genx.MessageChunk, error) {
	g.mu.Lock()
	for len(g.ready) == 0 {
		if g.err != nil {
			err := g.err
			g.mu.Unlock()
			return nil, err
		}
		if g.ended && len(g.retained) == 0 {
			g.mu.Unlock()
			return nil, io.EOF
		}
		g.cond.Wait()
	}
	chunk := g.ready[0]
	g.ready[0] = nil
	g.ready = g.ready[1:]
	if chunk.Role == genx.RoleModel && chunk.Ctrl != nil {
		mime, _ := chunk.MIMEType()
		key := chunk.Ctrl.StreamID + "\x00" + mime
		if chunk.IsBeginOfStream() {
			g.routes[key] = chunk
		}
		if chunk.IsEndOfStream() {
			if !g.deferred {
				delete(g.routes, key)
			}
		}
	}
	deferred := g.deferred
	g.mu.Unlock()
	if !deferred {
		g.ObserveOutput(chunk)
	}
	return chunk, nil
}

func (g *captureOutput) DeferOutputObservation() {
	g.mu.Lock()
	g.deferred = true
	g.mu.Unlock()
}

func (g *captureOutput) ObserveOutput(chunk *genx.MessageChunk) {
	if chunk != nil && chunk.Role == genx.RoleModel && chunk.IsEndOfStream() {
		mime, _ := chunk.MIMEType()
		key := chunk.Ctrl.StreamID + "\x00" + mime
		g.mu.Lock()
		if route := g.routes[key]; route != nil && route.Ctrl.ResponseEpoch == chunk.Ctrl.ResponseEpoch {
			delete(g.routes, key)
		}
		g.mu.Unlock()
	}
	original := g.takeOriginal(chunk)
	if g.observer != nil {
		g.observer.ObserveOutput(original)
	}
}

func (g *captureOutput) AbandonOutputObservation(chunk *genx.MessageChunk) {
	g.abandon([]*genx.MessageChunk{chunk})
}

func (g *captureOutput) SetOutputProductionObserver(observe func(*genx.MessageChunk)) bool {
	if producer, ok := g.source.(OutputProductionObserver); ok {
		return producer.SetOutputProductionObserver(observe)
	}
	return false
}

func (g *captureOutput) abandon(chunks []*genx.MessageChunk) {
	if len(chunks) == 0 {
		return
	}
	abandoner, ok := g.source.(outputObservationAbandoner)
	for _, chunk := range chunks {
		original := g.takeOriginal(chunk)
		if ok {
			abandoner.AbandonOutputObservation(original)
		}
	}
}

func (g *captureOutput) takeOriginal(chunk *genx.MessageChunk) *genx.MessageChunk {
	g.mu.Lock()
	defer g.mu.Unlock()
	if original := g.original[chunk]; original != nil {
		delete(g.original, chunk)
		return original
	}
	return chunk
}

func (g *captureOutput) abort(err error) {
	g.mu.Lock()
	discarded := append(g.retained, g.ready...)
	for i, chunk := range discarded {
		if original := g.original[chunk]; original != nil {
			discarded[i] = original
			delete(g.original, chunk)
		}
	}
	for _, original := range g.original {
		discarded = append(discarded, original)
	}
	clear(g.original)
	g.retained, g.ready = nil, nil
	g.err = err
	g.ended = true
	g.cond.Broadcast()
	g.mu.Unlock()
	g.abandon(discarded)
}

func (g *captureOutput) Close() error { return g.CloseWithError(io.ErrClosedPipe) }

func (g *captureOutput) CloseWithError(err error) error {
	if err == nil {
		err = io.ErrClosedPipe
	}
	g.abort(err)
	g.cancel(err)
	return nil
}

func captureAudioMIME(mime string) bool {
	mime = strings.ToLower(strings.TrimSpace(mime))
	return strings.HasPrefix(mime, "audio/") || strings.HasPrefix(mime, "application/ogg")
}

func emptyCapturePart(part genx.Part) genx.Part {
	if blob, ok := part.(*genx.Blob); ok && blob != nil {
		return &genx.Blob{MIMEType: blob.MIMEType}
	}
	return genx.Text("")
}

func captureHasPayload(part genx.Part) bool {
	switch part := part.(type) {
	case genx.Text:
		return part != ""
	case *genx.Blob:
		return part != nil && len(part.Data) > 0
	default:
		return part != nil
	}
}
