package streamkit

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/streamlog"
)

const (
	defaultTTSSegmentMaxRunes = 80

	// ttsLookahead is how many segments may be synthesized ahead of the one
	// being emitted. Every segment costs a fresh provider request, so a serial
	// pipeline puts one full first-audio latency of silence at each segment
	// boundary — more than a listening client's jitter buffer holds. One
	// segment of lookahead hides that latency behind the preceding segment and
	// caps a turn at two concurrent provider sessions.
	ttsLookahead = 1

	// ttsAudioQueueBytes bounds the audio a lookahead segment may hold before
	// its synthesis blocks. Emission order is fixed, so a segment that finishes
	// early waits here rather than being emitted out of turn. A provider picks
	// its own chunk sizes, so the budget is in bytes: counting chunks would let
	// one stream retain an unbounded amount of provider output.
	ttsAudioQueueBytes = 1 << 20
)

// TTSMeta is immutable input-route metadata supplied to a TTS synthesizer.
type TTSMeta struct {
	Role         genx.Role
	Name         string
	Label        string
	StreamID     string
	SegmentIndex int // one-based synthesis request within the input route
	MessageID    string
}

type ttsStreamState struct {
	meta     TTSMeta // input-loop owned; synthesis jobs receive immutable copies
	response *Response
	ctx      context.Context
	cancel   context.CancelFunc
	input    chan ttsInput
	done     chan struct{}
	err      error // published by closing done
}

// cancelPending abandons unsubmitted text and every batch still in flight. Their goroutines
// observe the cancelled context and exit; audio they still hand over is
// rejected by the retired response.
func (s *ttsStreamState) cancelPending() {
	s.cancel()
}

// ttsSegmentEmitter emits one stream's synthesis batches in order, each as soon as its
// provider produces audio. Emission runs beside the text loop rather than
// waiting for the next segment to be cut: a first segment held until the model
// finishes its second sentence would reach the listener only after that whole
// sentence had been generated. At most ttsLookahead+1 syntheses are in flight,
// the one emitting and the ones synthesized ahead of it.
type ttsSegmentEmitter struct {
	ctx        context.Context
	cancel     context.CancelFunc
	slots      chan struct{}
	queue      chan *ttsSegmentJob
	done       chan struct{}
	failed     chan struct{}
	changed    chan struct{}
	err        error
	segmentErr error
	emitted    atomic.Bool
}

func newTTSSegmentEmitter(ctx context.Context, emit func(context.Context, TTSMeta, []byte) error) *ttsSegmentEmitter {
	ctx, cancel := context.WithCancel(ctx)
	e := &ttsSegmentEmitter{
		ctx:     ctx,
		cancel:  cancel,
		slots:   make(chan struct{}, ttsLookahead+1),
		queue:   make(chan *ttsSegmentJob, ttsLookahead+1),
		done:    make(chan struct{}),
		failed:  make(chan struct{}),
		changed: make(chan struct{}, 1),
	}
	go e.run(emit)
	return e
}

func (e *ttsSegmentEmitter) run(emit func(context.Context, TTSMeta, []byte) error) {
	defer close(e.done)
	for job := range e.queue {
		if e.err != nil || e.ctx.Err() != nil {
			job.cancel()
			<-job.done
		} else if emitErr, synthErr := job.drain(func(data []byte) error {
			err := emit(e.ctx, job.meta, data)
			if err == nil && len(data) > 0 {
				e.emitted.Store(true)
				e.notify()
			}
			return err
		}); emitErr != nil {
			e.fail(emitErr)
		} else if synthErr != nil {
			var segmentErr *TTSSegmentError
			if e.ctx.Err() == nil && errors.As(synthErr, &segmentErr) {
				if e.segmentErr == nil {
					e.segmentErr = genx.ClassifyFailure(synthErr, genx.FailureClassProvider)
				}
			} else {
				e.fail(genx.ClassifyFailure(synthErr, genx.FailureClassProvider))
			}
		}
		<-e.slots
		e.notify()
	}
}

// fail records the first emission or synthesis error and abandons the
// segments queued behind it. Only the run goroutine calls it.
func (e *ttsSegmentEmitter) fail(err error) {
	e.err = err
	close(e.failed)
	e.cancel()
	e.notify()
}

// tryEnqueue reserves a synthesis slot before taking text out of the batch.
// While all slots are occupied, pending text remains available for merging.
func (e *ttsSegmentEmitter) tryEnqueue(start func(context.Context) *ttsSegmentJob) (bool, error) {
	// A failure also cancels ctx, so the recorded failure takes precedence;
	// fail writes err before closing failed.
	select {
	case <-e.failed:
		return false, e.err
	default:
	}
	if err := e.ctx.Err(); err != nil {
		return false, err
	}
	select {
	case e.slots <- struct{}{}:
	default:
		return false, nil
	}
	e.queue <- start(e.ctx)
	return true, nil
}

func (e *ttsSegmentEmitter) notify() {
	select {
	case e.changed <- struct{}{}:
	default:
	}
}

// finish waits until every accepted segment has been emitted.
func (e *ttsSegmentEmitter) finish() error {
	close(e.queue)
	<-e.done
	e.cancel()
	if e.err == nil && !e.emitted.Load() {
		return e.segmentErr
	}
	return e.err
}

// abort abandons every accepted segment without waiting for their providers.
func (e *ttsSegmentEmitter) abort() {
	e.cancel()
	close(e.queue)
}

// ttsSegmentJob is one segment being synthesized ahead of its turn to be
// emitted. Audio accumulates in the queue while an earlier segment is still
// emitting, so the provider's first-audio latency is paid concurrently. The
// queue holds at most ttsAudioQueueBytes; past that the provider callback
// blocks until the consumer catches up or the job is cancelled.
type ttsSegmentJob struct {
	meta   TTSMeta
	cancel context.CancelFunc
	done   chan error

	mu      sync.Mutex
	ready   *sync.Cond
	pending [][]byte
	bytes   int
	sealed  bool
	stopped bool
}

func startTTSSegment(ctx context.Context, segment string, meta TTSMeta, mimeType string, synthesize TTSSynthesizer) *ttsSegmentJob {
	return startMeasuredTTSSegment(ctx, segment, meta, mimeType, synthesize, nil, nil)
}

func startMeasuredTTSSegment(ctx context.Context, segment string, meta TTSMeta, mimeType string, synthesize TTSSynthesizer, measureFactory func() func([]byte) time.Duration, budget *ttsSynthesisBudget) *ttsSegmentJob {
	jobCtx, cancel := context.WithCancel(ctx)
	job := &ttsSegmentJob{meta: meta, cancel: cancel, done: make(chan error, 1)}
	job.ready = sync.NewCond(&job.mu)
	stop := context.AfterFunc(jobCtx, job.stop)
	if budget != nil {
		budget.start()
	}
	go func() {
		started := time.Now()
		var measure func([]byte) time.Duration
		if measureFactory != nil {
			measure = measureFactory()
		}
		firstAudio := true
		emit := func(data []byte) error {
			if err := job.push(data); err != nil {
				return err
			}
			if budget != nil && measure != nil {
				duration := measure(data)
				latency := time.Duration(0)
				if duration > 0 && firstAudio {
					latency, firstAudio = time.Since(started), false
				}
				budget.observe(duration, latency)
			}
			return nil
		}
		err := synthesize(jobCtx, segment, meta, mimeType, emit)
		if budget != nil {
			budget.finish()
		}
		stop()
		job.seal()
		job.done <- err
	}()
	return job
}

// push hands one chunk of provider audio to the queue, blocking while the queue
// is over budget so a fast provider cannot outrun the consumer's memory.
func (j *ttsSegmentJob) push(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	for j.bytes >= ttsAudioQueueBytes && !j.stopped {
		j.ready.Wait()
	}
	if j.stopped {
		return context.Canceled
	}
	j.pending = append(j.pending, bytes.Clone(data))
	j.bytes += len(data)
	j.ready.Broadcast()
	return nil
}

// seal marks the provider finished so a waiting consumer stops.
func (j *ttsSegmentJob) seal() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.sealed = true
	j.ready.Broadcast()
}

// stop releases a producer or consumer blocked on the queue after cancellation.
func (j *ttsSegmentJob) stop() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.stopped = true
	j.ready.Broadcast()
}

// next returns the queue's oldest chunk, waiting for one while the provider is
// still running.
func (j *ttsSegmentJob) next() ([]byte, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for len(j.pending) == 0 && !j.sealed {
		j.ready.Wait()
	}
	if len(j.pending) == 0 {
		return nil, false
	}
	data := j.pending[0]
	j.pending = j.pending[1:]
	j.bytes -= len(data)
	j.ready.Broadcast()
	return data, true
}

// drain emits this segment's audio in order and reports the emission and
// synthesis failures separately, because only the latter is a provider fault.
func (j *ttsSegmentJob) drain(emit func([]byte) error) (error, error) {
	defer j.cancel()
	var emitErr error
	for {
		data, ok := j.next()
		if !ok {
			break
		}
		if emitErr != nil {
			continue
		}
		if err := emit(data); err != nil {
			emitErr = err
			j.cancel()
		}
	}
	return emitErr, <-j.done
}

// TTSSegmentError marks a provider failure confined to one text segment.
// The provider must log it before returning. The pipeline keeps already emitted
// audio and continues later segments. If the entire route emits no audio, it
// ends with the first such error. Cancellation and emit errors must not use it.
type TTSSegmentError struct {
	Err error
}

func (e *TTSSegmentError) Error() string { return e.Err.Error() }
func (e *TTSSegmentError) Unwrap() error { return e.Err }

// TTSSynthesizer streams one submitted text batch to a provider. emit accepts already
// normalized container bytes and attaches canonical route metadata.
type TTSSynthesizer func(context.Context, string, TTSMeta, string, func([]byte) error) error

// NewTTSStream starts the shared client-segmented TTS pipeline. It
// returns immediately; each call owns its invocation, output, segmentation,
// batching, cancellation, and response state. genx.WithTTSBatching controls
// the invocation's request size and collection budgets.
func NewTTSStream(ctx context.Context, input genx.Stream, config OutputConfig, mimeType string, synthesize TTSSynthesizer) *Output {
	return NewMeasuredTTSStream(ctx, input, config, mimeType, synthesize, nil)
}

// NewMeasuredTTSStream supplies one serial audio-duration meter per provider
// request. Meters consume normalized output bytes and return newly available
// media duration; headers and incomplete frames return zero. The shared pipeline
// uses these observations to collect subsequent text within the audio budget.
// Before audio is measured, an available lookahead slot can prefetch the next
// confirmed phrase instead of spending a playback budget that does not exist.
// synthesize must call its audio callback serially. A nil meter falls back to
// collecting until the preceding synthesis returns.
func NewMeasuredTTSStream(ctx context.Context, input genx.Stream, config OutputConfig, mimeType string, synthesize TTSSynthesizer, measureFactory func() func([]byte) time.Duration) *Output {
	invocation := NewInvocation(ctx, config)
	go runTTS(invocation, input, mimeType, func(state *ttsStreamState, invocation *Invocation, mimeType string) error {
		return state.run(invocation, mimeType, synthesize, measureFactory)
	})
	return invocation.Output()
}

type ttsStateRunner func(*ttsStreamState, *Invocation, string) error

func runTTS(invocation *Invocation, input genx.Stream, mimeType string, run ttsStateRunner) {
	ctx := invocation.Context()
	// A provider failure or output cancellation must release a blocked Next.
	inputClosed := make(chan struct{})
	stopInput := context.AfterFunc(ctx, func() {
		defer close(inputClosed)
		_ = input.CloseWithError(context.Cause(ctx))
	})
	inputFinished := false
	defer func() {
		if !inputFinished && !stopInput() {
			<-inputClosed
		}
	}()
	states := make(map[string]*ttsStreamState)
	defer func() {
		for _, state := range states {
			state.cancelPending()
		}
		for _, state := range states {
			<-state.done
		}
	}()

	stateFor := func(chunk *genx.MessageChunk) (*ttsStreamState, error) {
		key := inputStreamID(chunk)
		state := states[key]
		if state == nil {
			meta := TTSMeta{StreamID: key}
			updateTTSMeta(&meta, chunk)
			response, err := invocation.StartResponse(ResponseConfig{
				StreamID: meta.StreamID, Role: meta.Role, Name: meta.Name, Label: meta.Label,
			}, mimeType)
			if err != nil {
				return nil, err
			}
			meta.StreamID = response.StreamID()
			if err := invocation.Emit(response, &genx.MessageChunk{
				Part: &genx.Blob{MIMEType: mimeType},
				Ctrl: &genx.StreamCtrl{BeginOfStream: true, MessageID: meta.MessageID},
			}); err != nil {
				return nil, err
			}
			state = newTTSState(ctx, invocation, response, meta, mimeType, run)
			states[key] = state
		} else {
			meta := state.meta
			updateTTSMeta(&meta, chunk)
			if !sameTTSSpeech(state.meta, meta) {
				if err := state.send(ttsInput{meta: state.meta, finish: true}); err != nil {
					return nil, err
				}
			}
			state.meta = meta
		}
		return state, nil
	}
	closeState := func(key string, state *ttsStreamState) error {
		if err := state.send(ttsInput{meta: state.meta, finish: true, endStream: true}); err != nil {
			return err
		}
		<-state.done
		delete(states, key)
		return state.err
	}

	for {
		chunk, err := streamlog.ReadInput(ctx, input)
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, genx.ErrDone) {
				_ = invocation.Fail(err)
				return
			}
			// The reader already reached its normal terminal boundary. Do not
			// turn successful completion into cancellation of the upstream.
			if !stopInput() {
				<-inputClosed
			}
			inputFinished = true
			for key, state := range states {
				if err := closeState(key, state); err != nil {
					_ = invocation.Fail(err)
					return
				}
			}
			_ = invocation.Close()
			return
		}
		if chunk == nil {
			continue
		}
		key := inputStreamID(chunk)
		if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
			if state := states[key]; state != nil {
				state.cancelPending()
				_ = invocation.Interrupt(state.response, chunk.Ctrl.Error)
				<-state.done
				delete(states, key)
			} else if err := invocation.Output().Push(chunk); err != nil {
				return
			}
			continue
		}
		if text, ok := chunk.Part.(genx.Text); ok {
			state, err := stateFor(chunk)
			if err != nil {
				_ = invocation.Fail(err)
				return
			}
			messageEnd := chunk.Ctrl != nil && chunk.Ctrl.MessageEnd && !chunk.IsEndOfStream()
			if err := state.send(ttsInput{meta: state.meta, text: string(text), finish: messageEnd || chunk.IsEndOfStream(), messageEnd: messageEnd, endStream: chunk.IsEndOfStream()}); err != nil {
				_ = invocation.Fail(err)
				return
			}
			if chunk.IsEndOfStream() {
				<-state.done
				delete(states, key)
				if state.err != nil {
					_ = invocation.Fail(state.err)
					return
				}
			}
			continue
		}
		if chunk.IsEndOfStream() {
			if state := states[key]; state != nil {
				updateTTSMeta(&state.meta, chunk)
				if err := closeState(key, state); err != nil {
					_ = invocation.Fail(err)
					return
				}
			}
		}
		if err := invocation.Output().Push(chunk); err != nil {
			return
		}
	}
}

func inputStreamID(chunk *genx.MessageChunk) string {
	if chunk != nil && chunk.Ctrl != nil {
		return strings.TrimSpace(chunk.Ctrl.StreamID)
	}
	return ""
}

func hasReadableTTSSpokenText(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsSymbol(r) {
			return true
		}
	}
	return false
}

func updateTTSMeta(meta *TTSMeta, chunk *genx.MessageChunk) {
	meta.Role = chunk.Role
	meta.Name = chunk.Name
	if chunk.Ctrl != nil {
		if chunk.Ctrl.MessageID != "" {
			meta.MessageID = chunk.Ctrl.MessageID
		}
		if streamID := strings.TrimSpace(chunk.Ctrl.StreamID); streamID != "" {
			meta.StreamID = streamID
		}
		if chunk.Ctrl.Label != "" {
			meta.Label = chunk.Ctrl.Label
		}
	}
}
