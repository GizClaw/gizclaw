package streamkit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// ErrInactiveResponse reports output for a response that is no longer active.
var ErrInactiveResponse = errors.New("streamkit: response is not active")

// ErrResponseActive reports a duplicate active StreamID.
var ErrResponseActive = errors.New("streamkit: response is already active")

// Invocation owns all mutable stream state for one Transform call. Multiple
// logical responses may be active when an input stream interleaves StreamIDs.
type Invocation struct {
	mu sync.Mutex

	ctx       context.Context
	cancel    context.CancelFunc
	output    *Output
	responses map[string]*Response
	closed    bool
}

// NewInvocation creates an independent invocation and output buffer.
func NewInvocation(parent context.Context, outputConfig OutputConfig) *Invocation {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	invocation := &Invocation{
		ctx:       ctx,
		cancel:    cancel,
		output:    NewOutput(outputConfig),
		responses: make(map[string]*Response),
	}
	if err := parent.Err(); err != nil {
		_ = invocation.Cancel(err)
		return invocation
	}
	go func() {
		select {
		case <-parent.Done():
			_ = invocation.Cancel(parent.Err())
		case <-invocation.output.Done():
			invocation.stopFromOutput()
		}
	}()
	return invocation
}

func (i *Invocation) stopFromOutput() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return
	}
	i.closed = true
	clear(i.responses)
	i.cancel()
}

// Context returns the invocation-local cancellation context.
func (i *Invocation) Context() context.Context {
	if i == nil || i.ctx == nil {
		return context.Background()
	}
	return i.ctx
}

// Output returns the pull stream owned by this invocation.
func (i *Invocation) Output() *Output {
	if i == nil {
		return nil
	}
	return i.output
}

// StartResponse registers a logical response and its known MIME routes.
func (i *Invocation) StartResponse(config ResponseConfig, mimeTypes ...string) (*Response, error) {
	if i == nil {
		return nil, io.ErrClosedPipe
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return nil, io.ErrClosedPipe
	}
	response := NewResponse(config)
	if _, active := i.responses[response.StreamID()]; active {
		return nil, fmt.Errorf("%w: stream_id=%s", ErrResponseActive, response.StreamID())
	}
	for _, mimeType := range mimeTypes {
		response.Declare(mimeType)
	}
	i.responses[response.StreamID()] = response
	return response, nil
}

// Emit appends one chunk for an active response. Missing route metadata is
// filled from ResponseConfig; mismatched or late StreamIDs are rejected.
func (i *Invocation) Emit(response *Response, chunk *genx.MessageChunk) error {
	if i == nil || response == nil || chunk == nil {
		return ErrInactiveResponse
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return ErrInactiveResponse
	}
	if active := i.responses[response.StreamID()]; active != response {
		return ErrInactiveResponse
	}
	chunk = response.applyMetadata(chunk)
	if !response.Accept(chunk) {
		return ErrInactiveResponse
	}
	if err := i.output.Push(chunk); err != nil {
		i.closed = true
		clear(i.responses)
		i.cancel()
		return err
	}
	return nil
}

// FinishResponse emits EOS for each still-open MIME route and retires the
// response. Already emitted per-route EOS chunks are not duplicated.
func (i *Invocation) FinishResponse(response *Response, errorText string) error {
	if i == nil || response == nil {
		return ErrInactiveResponse
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return ErrInactiveResponse
	}
	if active := i.responses[response.StreamID()]; active != response {
		return ErrInactiveResponse
	}
	if err := i.pushTerminalLocked(response.End(errorText)); err != nil {
		return err
	}
	delete(i.responses, response.StreamID())
	return nil
}

// Interrupt discards unpulled chunks for one response, emits terminal EOS for
// each open MIME route, and rejects later events for that response. An empty
// error text is reported as "interrupted".
func (i *Invocation) Interrupt(response *Response, errorText string) error {
	if i == nil || response == nil {
		return ErrInactiveResponse
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return ErrInactiveResponse
	}
	if active := i.responses[response.StreamID()]; active != response {
		return ErrInactiveResponse
	}
	streamID := response.StreamID()
	discarded := i.output.discardChunks(func(chunk *genx.MessageChunk) bool {
		return chunkStreamID(chunk) == streamID
	})
	if strings.TrimSpace(errorText) == "" {
		errorText = "interrupted"
	}
	if err := i.pushTerminalLocked(response.endAfterDiscard(errorText, discarded)); err != nil {
		return err
	}
	delete(i.responses, response.StreamID())
	return nil
}

// Fail emits terminal EOS/error for every active response and closes output
// without discarding already-buffered chunks.
func (i *Invocation) Fail(cause error) error {
	if i == nil {
		return nil
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return nil
	}
	i.closed = true
	i.cancel()
	errorText := "failed"
	if cause != nil {
		errorText = cause.Error()
	}
	for _, response := range i.responses {
		if err := i.pushTerminalLocked(response.End(errorText)); err != nil {
			return err
		}
	}
	clear(i.responses)
	return i.output.Close()
}

// Cancel terminates the complete invocation. Active responses receive
// terminal EOS/error after unpulled output is discarded.
func (i *Invocation) Cancel(cause error) error {
	if i == nil {
		return nil
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return nil
	}
	i.closed = true
	i.cancel()
	i.output.AbandonDeferredObservations()
	discarded := i.output.discardChunks(func(*genx.MessageChunk) bool { return true })
	errorText := "cancelled"
	if cause != nil {
		errorText = cause.Error()
	}
	for _, response := range i.responses {
		responseDiscarded := make([]*genx.MessageChunk, 0, len(discarded))
		for _, chunk := range discarded {
			if chunkStreamID(chunk) == response.StreamID() {
				responseDiscarded = append(responseDiscarded, chunk)
			}
		}
		if err := i.pushTerminalLocked(response.endAfterDiscard(errorText, responseDiscarded)); err != nil {
			return err
		}
	}
	clear(i.responses)
	return i.output.Close()
}

// Close completes the invocation and drains already-buffered output.
func (i *Invocation) Close() error {
	if i == nil {
		return nil
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return nil
	}
	i.closed = true
	i.cancel()
	for _, response := range i.responses {
		if err := i.pushTerminalLocked(response.End("")); err != nil {
			return err
		}
	}
	clear(i.responses)
	return i.output.Close()
}

func (i *Invocation) pushTerminalLocked(chunks []*genx.MessageChunk) error {
	for _, chunk := range chunks {
		if err := i.output.Push(chunk); err != nil {
			i.cancel()
			return fmt.Errorf("streamkit: emit terminal chunk: %w", err)
		}
	}
	return nil
}

func chunkStreamID(chunk *genx.MessageChunk) string {
	if chunk == nil || chunk.Ctrl == nil {
		return ""
	}
	return chunk.Ctrl.StreamID
}
