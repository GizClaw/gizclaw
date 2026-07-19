package agentkit

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
var ErrInactiveResponse = errors.New("agentkit: response is not active")

// ErrResponseActive reports an attempt to start a second active response.
var ErrResponseActive = errors.New("agentkit: response is already active")

// Invocation owns all mutable stream state for one Transform call.
type Invocation struct {
	mu sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
	output *Output
	active *Response
	closed bool
}

// NewInvocation creates an independent invocation and output buffer. Cancelling
// parent terminates only this invocation.
func NewInvocation(parent context.Context, outputConfig OutputConfig) *Invocation {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	invocation := &Invocation{
		ctx:    ctx,
		cancel: cancel,
		output: NewOutput(outputConfig),
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
		}
	}()
	return invocation
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

// StartResponse creates a fresh active response and declares its known MIME
// routes. The previous response must be completed or interrupted first.
func (i *Invocation) StartResponse(mimeTypes ...string) (*Response, error) {
	if i == nil {
		return nil, io.ErrClosedPipe
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return nil, io.ErrClosedPipe
	}
	if i.active != nil {
		return nil, fmt.Errorf("%w: finish or interrupt the current response first", ErrResponseActive)
	}
	response := NewResponse("")
	for _, mimeType := range mimeTypes {
		response.Declare(mimeType)
	}
	i.active = response
	return response, nil
}

// Emit appends one chunk for the active response. A missing StreamID is filled
// with the response identity; a mismatched or late StreamID is rejected.
func (i *Invocation) Emit(response *Response, chunk *genx.MessageChunk) error {
	if i == nil || response == nil || chunk == nil {
		return ErrInactiveResponse
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed || i.active != response {
		return ErrInactiveResponse
	}
	chunk = responseChunk(response.StreamID(), chunk)
	if !response.Accept(chunk) {
		return ErrInactiveResponse
	}
	if err := i.output.Push(chunk); err != nil {
		i.closed = true
		i.active = nil
		i.cancel()
		return err
	}
	return nil
}

// FinishResponse emits EOS for each still-open MIME route and retires the
// response. Already emitted per-route EOS chunks are not duplicated.
func (i *Invocation) FinishResponse(response *Response, label, errorText string) error {
	if i == nil || response == nil {
		return ErrInactiveResponse
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed || i.active != response {
		return ErrInactiveResponse
	}
	if err := i.pushTerminalLocked(response.End(label, errorText)); err != nil {
		return err
	}
	i.active = nil
	return nil
}

// Interrupt discards unpulled chunks for the active response, emits interrupted
// EOS for each open MIME route, and rejects all later events for that response.
func (i *Invocation) Interrupt(response *Response, label string) error {
	if i == nil || response == nil {
		return ErrInactiveResponse
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed || i.active != response {
		return ErrInactiveResponse
	}
	streamID := response.StreamID()
	discarded := i.output.discardChunks(func(chunk *genx.MessageChunk) bool {
		return chunkStreamID(chunk) == streamID
	})
	if err := i.pushTerminalLocked(response.endAfterDiscard(label, "interrupted", discarded)); err != nil {
		return err
	}
	i.active = nil
	return nil
}

// Cancel terminates the complete invocation. Any active response receives
// terminal EOS/error after its unpulled buffered output is discarded.
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
	discarded := i.output.discardChunks(func(*genx.MessageChunk) bool { return true })
	if i.active != nil {
		errorText := "cancelled"
		if cause != nil {
			errorText = cause.Error()
		}
		if err := i.pushTerminalLocked(i.active.endAfterDiscard("assistant", errorText, discarded)); err != nil {
			return err
		}
		i.active = nil
	}
	return i.output.Close()
}

// Close completes the invocation and drains any already-buffered output.
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
	if i.active != nil {
		if err := i.pushTerminalLocked(i.active.End("assistant", "")); err != nil {
			return err
		}
		i.active = nil
	}
	return i.output.Close()
}

func (i *Invocation) pushTerminalLocked(chunks []*genx.MessageChunk) error {
	for _, chunk := range chunks {
		if err := i.output.Push(chunk); err != nil {
			i.cancel()
			return err
		}
	}
	return nil
}

func responseChunk(streamID string, chunk *genx.MessageChunk) *genx.MessageChunk {
	copyChunk := *chunk
	if chunk.Ctrl == nil {
		copyChunk.Ctrl = &genx.StreamCtrl{StreamID: streamID}
		return &copyChunk
	}
	copyCtrl := *chunk.Ctrl
	if strings.TrimSpace(copyCtrl.StreamID) == "" {
		copyCtrl.StreamID = streamID
	}
	copyChunk.Ctrl = &copyCtrl
	return &copyChunk
}

func chunkStreamID(chunk *genx.MessageChunk) string {
	if chunk == nil || chunk.Ctrl == nil {
		return ""
	}
	return strings.TrimSpace(chunk.Ctrl.StreamID)
}
