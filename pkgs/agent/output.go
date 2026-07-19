package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/buffer"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// Interrupted is the terminal error carried by routes canceled by replacement input.
const Interrupted = "interrupted"

var (
	ErrResponseClosed   = errors.New("agent: response is closed")
	ErrOutputBufferFull = errors.New("agent: output buffer is full")
)

// OutputConfig configures an Agent-owned, non-blocking output queue.
type OutputConfig struct {
	InitialCapacity  int
	MaxBufferedBytes int
	Observe          func(*genx.MessageChunk)
}

// Output is a growable pull stream. Provider workers enqueue immediately and
// never depend on the downstream consumer to provide backpressure.
type Output struct {
	queue *buffer.Buffer[outputItem]

	mu               sync.Mutex
	maxBufferedBytes int
	bufferedBytes    int
	observe          func(*genx.MessageChunk)
	active           *Response
	closed           bool
}

type outputItem struct {
	response *Response
	chunk    *genx.MessageChunk
	size     int
}

// Response owns one assistant response and its fresh StreamID.
type Response struct {
	output   *Output
	ctx      context.Context
	cancel   context.CancelCauseFunc
	streamID string

	closed       bool
	interrupted  bool
	routes       map[string]responseRoute
	observedDone map[string]bool
}

type responseRoute struct {
	mimeType string
	role     genx.Role
	name     string
	label    string
}

// NewOutput constructs an Agent-owned growable pull-output stream.
func NewOutput(cfg OutputConfig) *Output {
	capacity := cfg.InitialCapacity
	if capacity < 0 {
		capacity = 0
	}
	return &Output{
		queue:            buffer.N[outputItem](capacity),
		maxBufferedBytes: cfg.MaxBufferedBytes,
		observe:          cfg.Observe,
	}
}

// Begin starts a new assistant response. Any previous response that has not
// been fully observed is interrupted before the new response can publish.
func (o *Output) Begin(ctx context.Context) (*Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return nil, ErrResponseClosed
	}
	if o.active != nil && !o.active.allRoutesObservedLocked() {
		if err := o.interruptLocked(o.active); err != nil {
			return nil, err
		}
	}
	responseCtx, cancel := context.WithCancelCause(ctx)
	response := &Response{
		output:       o,
		ctx:          responseCtx,
		cancel:       cancel,
		streamID:     genx.NewStreamID(),
		routes:       make(map[string]responseRoute),
		observedDone: make(map[string]bool),
	}
	o.active = response
	return response, nil
}

// Context is canceled when the response finishes, fails, or is interrupted.
func (r *Response) Context() context.Context { return r.ctx }

// StreamID returns the fresh identifier shared by this response's MIME routes.
func (r *Response) StreamID() string { return r.streamID }

// Interrupted reports whether this response was actually interrupted. It is
// useful to distinguish a completed, fully pulled response from one whose
// pending output was discarded by a replacement input.
func (r *Response) Interrupted() bool {
	if r == nil || r.output == nil {
		return false
	}
	r.output.mu.Lock()
	defer r.output.mu.Unlock()
	return r.interrupted
}

// Push publishes user-visible model content. ToolCall chunks are rejected
// because ToolCall and ToolResult traffic is internal to an Agent turn.
func (r *Response) Push(chunk *genx.MessageChunk) error {
	if r == nil || r.output == nil {
		return ErrResponseClosed
	}
	if chunk == nil {
		return nil
	}
	if chunk.ToolCall != nil {
		return fmt.Errorf("agent: tool calls cannot be published on Agent output")
	}
	o := r.output
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed || r.closed || r.interrupted || o.active != r {
		return ErrResponseClosed
	}
	owned := chunk.Clone()
	if owned.Ctrl == nil {
		owned.Ctrl = &genx.StreamCtrl{}
	}
	owned.Ctrl.StreamID = r.streamID
	if owned.Role == "" {
		owned.Role = genx.RoleModel
	}
	if mimeType, ok := owned.MIMEType(); ok {
		r.routes[mimeType] = responseRoute{
			mimeType: mimeType,
			role:     owned.Role,
			name:     owned.Name,
			label:    owned.Ctrl.Label,
		}
	}
	if err := o.addLocked(r, owned); err != nil {
		if errors.Is(err, ErrOutputBufferFull) {
			o.terminateLocked(r, err.Error())
		}
		return err
	}
	return nil
}

// Finish closes every declared MIME route successfully. A route is not final
// for history purposes until its EOS is returned by Output.Next.
func (r *Response) Finish() error {
	return r.finish("")
}

// Fail closes every declared MIME route with the same terminal error. The
// error is a user-visible turn result; it does not corrupt the pull stream.
func (r *Response) Fail(message string) error {
	if message == "" {
		return r.Finish()
	}
	return r.finish(message)
}

func (r *Response) finish(message string) error {
	if r == nil || r.output == nil {
		return ErrResponseClosed
	}
	o := r.output
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed || r.closed || r.interrupted || o.active != r {
		return ErrResponseClosed
	}
	routes := r.sortedOpenRoutesLocked()
	terminal := make([]*genx.MessageChunk, 0, len(routes))
	totalSize := 0
	for _, route := range routes {
		chunk := responseEOS(r.streamID, route, message)
		terminal = append(terminal, chunk)
		totalSize += messageChunkSize(chunk)
	}
	if o.maxBufferedBytes > 0 && o.bufferedBytes+totalSize > o.maxBufferedBytes {
		return fmt.Errorf("%w: buffered=%d next=%d maximum=%d", ErrOutputBufferFull, o.bufferedBytes, totalSize, o.maxBufferedBytes)
	}
	for _, chunk := range terminal {
		if err := o.addLocked(r, chunk); err != nil {
			return err
		}
	}
	r.closed = true
	return nil
}

// Interrupt cancels pending work, discards unpulled chunks for this response,
// and leaves one interrupted EOS for every route not already observed.
func (r *Response) Interrupt() error {
	if r == nil || r.output == nil {
		return nil
	}
	r.output.mu.Lock()
	defer r.output.mu.Unlock()
	return r.output.interruptLocked(r)
}

func (o *Output) Next() (*genx.MessageChunk, error) {
	item, err := o.queue.Next()
	if err != nil {
		if errors.Is(err, buffer.ErrIteratorDone) {
			return nil, io.EOF
		}
		return nil, err
	}
	o.mu.Lock()
	o.bufferedBytes -= item.size
	if o.bufferedBytes < 0 {
		o.bufferedBytes = 0
	}
	if item.response != nil && item.chunk != nil && item.chunk.IsEndOfStream() {
		if mimeType, ok := item.chunk.MIMEType(); ok {
			item.response.observedDone[mimeType] = true
		}
	}
	observe := o.observe
	o.mu.Unlock()
	if observe != nil && item.chunk != nil {
		observe(item.chunk.Clone())
	}
	return item.chunk, nil
}

func (o *Output) Close() error {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return nil
	}
	o.closed = true
	if o.active != nil {
		o.active.cancel(io.EOF)
	}
	o.mu.Unlock()
	return o.queue.CloseWrite()
}

func (o *Output) CloseWithError(err error) error {
	if o == nil {
		return nil
	}
	if err == nil {
		err = io.ErrClosedPipe
	}
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return nil
	}
	o.closed = true
	if o.active != nil {
		o.active.cancel(err)
	}
	o.bufferedBytes = 0
	o.mu.Unlock()
	return o.queue.CloseWithError(err)
}

func (o *Output) addLocked(response *Response, chunk *genx.MessageChunk) error {
	size := messageChunkSize(chunk)
	if o.maxBufferedBytes > 0 && o.bufferedBytes+size > o.maxBufferedBytes {
		return fmt.Errorf("%w: buffered=%d next=%d maximum=%d", ErrOutputBufferFull, o.bufferedBytes, size, o.maxBufferedBytes)
	}
	if err := o.queue.Add(outputItem{response: response, chunk: chunk, size: size}); err != nil {
		return err
	}
	o.bufferedBytes += size
	return nil
}

func (o *Output) interruptLocked(response *Response) error {
	if response == nil || response.interrupted || response.allRoutesObservedLocked() {
		return nil
	}
	response.interrupted = true
	response.closed = true
	response.cancel(errors.New(Interrupted))
	o.queue.RemoveIf(func(item outputItem) bool {
		if item.response != response {
			return false
		}
		o.bufferedBytes -= item.size
		return true
	})
	if o.bufferedBytes < 0 {
		o.bufferedBytes = 0
	}
	routes := response.sortedOpenRoutesLocked()
	if len(routes) == 0 {
		routes = []responseRoute{{mimeType: "text/plain", role: genx.RoleModel}}
	}
	for _, route := range routes {
		if response.observedDone[route.mimeType] {
			continue
		}
		if err := o.addLocked(response, responseEOS(response.streamID, route, Interrupted)); err != nil {
			return err
		}
	}
	return nil
}

func (o *Output) terminateLocked(response *Response, message string) {
	if response == nil || response.closed || response.interrupted {
		return
	}
	response.closed = true
	response.cancel(errors.New(message))
	routes := response.sortedOpenRoutesLocked()
	if len(routes) == 0 {
		routes = []responseRoute{{mimeType: "text/plain", role: genx.RoleModel}}
	}
	for _, route := range routes {
		_ = o.addLocked(response, responseEOS(response.streamID, route, message))
	}
}

func (r *Response) allRoutesObservedLocked() bool {
	if r == nil || len(r.routes) == 0 {
		return r != nil && r.closed
	}
	for mimeType := range r.routes {
		if !r.observedDone[mimeType] {
			return false
		}
	}
	return true
}

func (r *Response) sortedOpenRoutesLocked() []responseRoute {
	routes := make([]responseRoute, 0, len(r.routes))
	for mimeType, route := range r.routes {
		if !r.observedDone[mimeType] {
			routes = append(routes, route)
		}
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].mimeType == routes[j].mimeType {
			return false
		}
		if routes[i].mimeType == "text/plain" {
			return true
		}
		if routes[j].mimeType == "text/plain" {
			return false
		}
		return routes[i].mimeType < routes[j].mimeType
	})
	return routes
}

func responseEOS(streamID string, route responseRoute, message string) *genx.MessageChunk {
	var part genx.Part
	if route.mimeType == "text/plain" {
		part = genx.Text("")
	} else {
		part = &genx.Blob{MIMEType: route.mimeType}
	}
	return &genx.MessageChunk{
		Role: route.role,
		Name: route.name,
		Part: part,
		Ctrl: &genx.StreamCtrl{
			StreamID:    streamID,
			Label:       route.label,
			Error:       message,
			EndOfStream: true,
		},
	}
}

func messageChunkSize(chunk *genx.MessageChunk) int {
	if chunk == nil {
		return 0
	}
	switch part := chunk.Part.(type) {
	case genx.Text:
		return len(part)
	case *genx.Blob:
		if part != nil {
			return len(part.Data)
		}
	}
	return 0
}
