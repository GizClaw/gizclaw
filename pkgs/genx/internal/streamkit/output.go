package streamkit

import (
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// ErrOutputLimit is returned when queued content exceeds OutputConfig.MaxBytes.
var ErrOutputLimit = errors.New("streamkit: output buffer limit exceeded")

// OutputConfig configures a growable pull-output buffer.
type OutputConfig struct {
	InitialCapacity int
	MaxBytes        int64
	Observe         func(*genx.MessageChunk)
}

type outputEntry struct {
	chunk   *genx.MessageChunk
	bytes   int64
	observe func(*genx.MessageChunk)
	abandon func(*genx.MessageChunk)
}

type deferredObservation struct {
	chunk   *genx.MessageChunk
	observe func(*genx.MessageChunk)
	abandon func(*genx.MessageChunk)
}

// Output is a growable, concurrency-safe GenX Stream. Producers never wait for
// downstream pulls unless memory allocation itself blocks. A positive MaxBytes
// limits queued content bytes and turns overflow into an observable error.
type Output struct {
	mu   sync.Mutex
	cond *sync.Cond

	queue       []outputEntry
	queuedBytes int64
	maxBytes    int64
	closed      bool
	closeErr    error
	done        chan struct{}
	closeOnce   sync.Once

	observationDeferred bool
	observe             func(*genx.MessageChunk)
	deferred            []deferredObservation
	observers           int
	deferredObservers   int
}

var _ genx.Stream = (*Output)(nil)

// NewOutput creates an empty growable output stream.
func NewOutput(config OutputConfig) *Output {
	capacity := max(config.InitialCapacity, 0)
	output := &Output{
		queue:    make([]outputEntry, 0, capacity),
		maxBytes: config.MaxBytes,
		done:     make(chan struct{}),
		observe:  config.Observe,
	}
	output.cond = sync.NewCond(&output.mu)
	return output
}

// Next returns the next queued chunk. Observation happens only after the chunk
// has crossed this pull-visible boundary.
func (o *Output) Next() (*genx.MessageChunk, error) {
	if o == nil {
		return nil, io.EOF
	}
	o.mu.Lock()
	for len(o.queue) == 0 && !o.closed && o.closeErr == nil {
		o.cond.Wait()
	}
	if o.closeErr != nil {
		err := o.closeErr
		o.mu.Unlock()
		return nil, err
	}
	if len(o.queue) == 0 {
		o.mu.Unlock()
		return nil, io.EOF
	}
	entry := o.queue[0]
	var zero outputEntry
	o.queue[0] = zero
	o.queue = o.queue[1:]
	o.queuedBytes -= entry.bytes
	deferred := o.observationDeferred
	observe := entry.observe
	if observe == nil {
		observe = o.observe
	}
	tracked := entry.chunk != nil && observe != nil
	observing := tracked && !deferred
	if tracked {
		o.observers++
		if deferred {
			o.deferredObservers++
			o.deferred = append(o.deferred, deferredObservation{
				chunk: entry.chunk, observe: observe, abandon: entry.abandon,
			})
		}
	}
	o.mu.Unlock()
	if observing {
		func() {
			defer o.finishObservation()
			observe(entry.chunk)
		}()
	}
	return entry.chunk, nil
}

func (o *Output) finishObservation() {
	o.mu.Lock()
	o.observers--
	o.cond.Broadcast()
	o.mu.Unlock()
}

// Push appends a chunk without waiting for a downstream pull.
func (o *Output) Push(chunk *genx.MessageChunk) error {
	return o.PushObserved(chunk, nil)
}

// PushObserved appends a chunk with an optional per-entry delivery observer.
// The observer runs only after the chunk crosses the final pull boundary and
// takes precedence over the Output-wide observer.
func (o *Output) PushObserved(chunk *genx.MessageChunk, observe func(*genx.MessageChunk)) error {
	return o.PushTracked(chunk, observe, nil)
}

// PushTracked appends a chunk with final-delivery and pre-delivery-discard
// callbacks. Composition layers use abandon to release a wrapped producer's
// deferred acknowledgement without recording discarded read-ahead output.
func (o *Output) PushTracked(chunk *genx.MessageChunk, observe, abandon func(*genx.MessageChunk)) error {
	if o == nil {
		return io.ErrClosedPipe
	}
	entry := outputEntry{chunk: chunk, bytes: chunkContentBytes(chunk), observe: observe, abandon: abandon}
	o.mu.Lock()
	if o.closeErr != nil {
		err := o.closeErr
		o.mu.Unlock()
		return err
	}
	if o.closed {
		o.mu.Unlock()
		return io.ErrClosedPipe
	}
	if o.maxBytes > 0 && o.queuedBytes+entry.bytes > o.maxBytes {
		err := fmt.Errorf("%w: queued=%d next=%d max=%d", ErrOutputLimit, o.queuedBytes, entry.bytes, o.maxBytes)
		abandoned := o.closeWithErrorLocked(err)
		if entry.abandon != nil {
			abandoned = append(abandoned, deferredObservation{chunk: entry.chunk, abandon: entry.abandon})
		}
		o.mu.Unlock()
		runAbandonments(abandoned)
		return err
	}
	o.queue = append(o.queue, entry)
	o.queuedBytes += entry.bytes
	o.cond.Signal()
	o.mu.Unlock()
	return nil
}

// Discard removes queued chunks matching predicate while preserving order.
func (o *Output) Discard(predicate func(*genx.MessageChunk) bool) int {
	return len(o.discardChunks(predicate))
}

func (o *Output) discardChunks(predicate func(*genx.MessageChunk) bool) []*genx.MessageChunk {
	if o == nil || predicate == nil {
		return nil
	}
	o.mu.Lock()
	kept := o.queue[:0]
	removed := make([]*genx.MessageChunk, 0)
	abandoned := make([]outputEntry, 0)
	for _, entry := range o.queue {
		if predicate(entry.chunk) {
			o.queuedBytes -= entry.bytes
			removed = append(removed, entry.chunk)
			if entry.abandon != nil {
				abandoned = append(abandoned, entry)
			}
			continue
		}
		kept = append(kept, entry)
	}
	clear(o.queue[len(kept):])
	o.queue = kept
	o.mu.Unlock()
	for _, entry := range abandoned {
		entry.abandon(entry.chunk)
	}
	return removed
}

// Close marks production complete while preserving already queued chunks.
func (o *Output) Close() error {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	if !o.closed && o.closeErr == nil {
		o.closed = true
		o.signalDoneLocked()
		o.cond.Broadcast()
	}
	o.mu.Unlock()
	return nil
}

// CloseWithError terminates the stream and discards queued chunks.
func (o *Output) CloseWithError(err error) error {
	if o == nil {
		return nil
	}
	if err == nil {
		err = io.ErrClosedPipe
	}
	o.mu.Lock()
	abandoned := o.closeWithErrorLocked(err)
	o.mu.Unlock()
	runAbandonments(abandoned)
	return nil
}

func (o *Output) closeWithErrorLocked(err error) []deferredObservation {
	if o.closed || o.closeErr != nil {
		return nil
	}
	abandoned := make([]deferredObservation, 0, len(o.queue)+len(o.deferred))
	for _, entry := range o.queue {
		if entry.abandon != nil {
			abandoned = append(abandoned, deferredObservation{chunk: entry.chunk, abandon: entry.abandon})
		}
	}
	o.closeErr = err
	o.closed = true
	clear(o.queue)
	o.queue = nil
	o.queuedBytes = 0
	abandoned = append(abandoned, o.abandonDeferredObservationsLocked()...)
	o.signalDoneLocked()
	o.cond.Broadcast()
	return abandoned
}

// AbandonDeferredObservations releases delivery acknowledgements that can no
// longer arrive because their final consumer has been cancelled.
func (o *Output) AbandonDeferredObservations() {
	if o == nil {
		return
	}
	o.mu.Lock()
	abandoned := o.abandonDeferredObservationsLocked()
	o.mu.Unlock()
	runAbandonments(abandoned)
}

func (o *Output) abandonDeferredObservationsLocked() []deferredObservation {
	if o.deferredObservers == 0 {
		return nil
	}
	abandoned := o.deferred
	o.observers -= o.deferredObservers
	o.deferredObservers = 0
	o.deferred = nil
	o.cond.Broadcast()
	return abandoned
}

func (o *Output) signalDoneLocked() {
	o.closeOnce.Do(func() { close(o.done) })
}

// Done closes as soon as production is closed or aborted.
func (o *Output) Done() <-chan struct{} {
	if o == nil {
		done := make(chan struct{})
		close(done)
		return done
	}
	return o.done
}

// DeferOutputObservation disables automatic pull observation. Call
// ObserveOutput explicitly after the final consumer successfully receives a
// chunk.
func (o *Output) DeferOutputObservation() {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.observationDeferred = true
	o.mu.Unlock()
}

// ObserveOutput records one successfully delivered chunk.
func (o *Output) ObserveOutput(chunk *genx.MessageChunk) {
	if o == nil || chunk == nil {
		return
	}
	o.mu.Lock()
	var observe func(*genx.MessageChunk)
	index := o.deferredObservationIndexLocked(chunk, true)
	tracked := o.observationDeferred && index >= 0
	if tracked {
		observe = o.deferred[index].observe
		o.removeDeferredObservationLocked(index)
		o.deferredObservers--
	}
	o.mu.Unlock()
	if !tracked {
		return
	}
	defer o.finishObservation()
	if observe != nil {
		observe(chunk)
	}
}

// AbandonOutputObservation releases the delivery acknowledgement associated
// with a chunk that a composition layer read but later discarded before its
// final consumer pulled it. Unlike ObserveOutput, it does not run the
// persistence observer.
func (o *Output) AbandonOutputObservation(chunk *genx.MessageChunk) {
	if o == nil || chunk == nil {
		return
	}
	o.mu.Lock()
	index := o.deferredObservationIndexLocked(chunk, false)
	var abandon func(*genx.MessageChunk)
	if o.observationDeferred && index >= 0 {
		abandon = o.deferred[index].abandon
		o.removeDeferredObservationLocked(index)
		o.deferredObservers--
		o.observers--
		o.cond.Broadcast()
	}
	o.mu.Unlock()
	if abandon != nil {
		abandon(chunk)
	}
}

func runAbandonments(observations []deferredObservation) {
	for _, observation := range observations {
		if observation.abandon != nil {
			observation.abandon(observation.chunk)
		}
	}
}

func (o *Output) deferredObservationIndexLocked(chunk *genx.MessageChunk, allowFIFO bool) int {
	if !o.observationDeferred || len(o.deferred) == 0 {
		return -1
	}
	for index := range o.deferred {
		if o.deferred[index].chunk == chunk {
			return index
		}
	}
	if allowFIFO {
		// Preserve the original FIFO acknowledgement contract for callers that
		// pass a defensive clone rather than the exact chunk returned by Next.
		return 0
	}
	return -1
}

func (o *Output) removeDeferredObservationLocked(index int) {
	copy(o.deferred[index:], o.deferred[index+1:])
	var zero deferredObservation
	o.deferred[len(o.deferred)-1] = zero
	o.deferred = o.deferred[:len(o.deferred)-1]
}

// SetOutputObserver replaces the pull-visible observation callback.
func (o *Output) SetOutputObserver(observe func(*genx.MessageChunk)) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.observe = observe
	o.mu.Unlock()
}

// WaitForObservers waits until every chunk already dequeued by Next has
// completed its pull-visible observation callback, including observations
// deferred to a later delivery boundary. Producers use this after a response
// discard when persistence must include a chunk that Next has already claimed
// but whose observer has not returned yet.
func (o *Output) WaitForObservers() {
	if o == nil {
		return
	}
	o.mu.Lock()
	for o.observers != 0 {
		o.cond.Wait()
	}
	o.mu.Unlock()
}

func chunkContentBytes(chunk *genx.MessageChunk) int64 {
	if chunk == nil {
		return 0
	}
	switch part := chunk.Part.(type) {
	case genx.Text:
		return int64(len(part))
	case *genx.Blob:
		if part != nil {
			return int64(len(part.Data))
		}
	}
	return 0
}
