package streamkit

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"unicode"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/streamlog"
)

const (
	defaultTTSSegmentMaxRunes      = 80
	defaultTTSFirstSegmentMinRunes = 8

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
	Role     genx.Role
	Name     string
	Label    string
	StreamID string
}

type ttsStreamState struct {
	meta      TTSMeta
	segmenter *ttsSentenceSegmenter
	response  *Response
	pending   []*ttsSegmentJob
}

// cancelPending abandons every segment still in flight. Their goroutines
// observe the cancelled context and exit without emitting.
func (s *ttsStreamState) cancelPending() {
	for _, job := range s.pending {
		job.cancel()
	}
	s.pending = nil
}

// ttsSegmentJob is one segment being synthesized ahead of its turn to be
// emitted. Audio accumulates in the queue while an earlier segment is still
// emitting, so the provider's first-audio latency is paid concurrently. The
// queue holds at most ttsAudioQueueBytes; past that the provider callback
// blocks until the consumer catches up or the job is cancelled.
type ttsSegmentJob struct {
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
	jobCtx, cancel := context.WithCancel(ctx)
	job := &ttsSegmentJob{cancel: cancel, done: make(chan error, 1)}
	job.ready = sync.NewCond(&job.mu)
	stop := context.AfterFunc(jobCtx, job.stop)
	go func() {
		err := synthesize(jobCtx, segment, meta, mimeType, job.push)
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

// TTSSynthesizer streams one text segment to a provider. emit accepts already
// normalized container bytes and attaches canonical route metadata.
type TTSSynthesizer func(context.Context, string, TTSMeta, string, func([]byte) error) error

// NewTTSStream starts the shared non-provider-specific TTS pipeline. It
// returns immediately; each call owns its invocation, output, segmentation,
// cancellation, and response state.
func NewTTSStream(ctx context.Context, input genx.Stream, config OutputConfig, mimeType string, synthesize TTSSynthesizer) *Output {
	invocation := NewInvocation(ctx, config)
	go runTTS(invocation, input, mimeType, synthesize)
	return invocation.Output()
}

func runTTS(invocation *Invocation, input genx.Stream, mimeType string, synthesize TTSSynthesizer) {
	ctx := invocation.Context()
	states := make(map[string]*ttsStreamState)
	defer func() {
		for _, state := range states {
			state.cancelPending()
		}
	}()

	stateFor := func(chunk *genx.MessageChunk) (*ttsStreamState, error) {
		streamID := inputStreamID(chunk)
		state := states[streamID]
		if state == nil {
			meta := TTSMeta{StreamID: streamID}
			updateTTSMeta(&meta, chunk)
			response, err := invocation.StartResponse(ResponseConfig{
				StreamID: meta.StreamID,
				Role:     meta.Role,
				Name:     meta.Name,
				Label:    meta.Label,
			}, mimeType)
			if err != nil {
				return nil, err
			}
			meta.StreamID = response.StreamID()
			state = &ttsStreamState{
				meta:      meta,
				segmenter: newTTSSentenceSegmenter(defaultTTSSegmentMaxRunes),
				response:  response,
			}
			states[streamID] = state
			if err := invocation.Emit(response, &genx.MessageChunk{
				Part: &genx.Blob{MIMEType: mimeType},
				Ctrl: &genx.StreamCtrl{BeginOfStream: true},
			}); err != nil {
				delete(states, streamID)
				return nil, err
			}
		} else {
			updateTTSMeta(&state.meta, chunk)
		}
		return state, nil
	}

	drainSegment := func(state *ttsStreamState) error {
		job := state.pending[0]
		state.pending = state.pending[1:]
		emitErr, synthErr := job.drain(func(data []byte) error {
			return invocation.Emit(state.response, &genx.MessageChunk{
				Part: &genx.Blob{MIMEType: mimeType, Data: data},
			})
		})
		if emitErr != nil {
			state.cancelPending()
			return emitErr
		}
		if synthErr != nil {
			state.cancelPending()
			return genx.ClassifyFailure(synthErr, genx.FailureClassProvider)
		}
		return nil
	}

	flushState := func(state *ttsStreamState, all bool) error {
		for _, segment := range state.segmenter.Segments(all) {
			if !hasReadableTTSSpokenText(segment) {
				continue
			}
			debugTTSSegment(ctx, state.meta, segment, all)
			// Synthesis starts as soon as the text is segmented, not when the
			// segment's turn to be emitted arrives, so its first-audio latency
			// runs while the preceding segment is still being delivered.
			state.pending = append(state.pending, startTTSSegment(ctx, segment, state.meta, mimeType, synthesize))
			for len(state.pending) > ttsLookahead {
				if err := drainSegment(state); err != nil {
					return err
				}
			}
		}
		if all {
			for len(state.pending) > 0 {
				if err := drainSegment(state); err != nil {
					return err
				}
			}
		}
		return nil
	}

	closeState := func(key string, state *ttsStreamState, errorText string) error {
		if errorText == "" {
			if err := flushState(state, true); err != nil {
				state.cancelPending()
				return err
			}
		} else {
			state.cancelPending()
		}
		if err := invocation.FinishResponse(state.response, errorText); err != nil {
			return err
		}
		delete(states, key)
		if errorText != "" {
			return errors.New(errorText)
		}
		return nil
	}

	closeAll := func() error {
		for key, state := range states {
			if err := closeState(key, state, ""); err != nil {
				return err
			}
		}
		return nil
	}

	for {
		chunk, err := streamlog.ReadInput(ctx, input)
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, genx.ErrDone) {
				_ = invocation.Fail(err)
				return
			}
			if err := closeAll(); err != nil {
				_ = invocation.Fail(err)
				return
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
				updateTTSMeta(&state.meta, chunk)
				state.cancelPending()
				_ = invocation.Interrupt(state.response, chunk.Ctrl.Error)
				delete(states, key)
			} else if err := invocation.Output().Push(chunk); err != nil {
				return
			}
			continue
		}

		text, isText := chunk.Part.(genx.Text)
		if isText {
			state, err := stateFor(chunk)
			if err != nil {
				_ = invocation.Fail(err)
				return
			}
			if text != "" {
				state.segmenter.WriteString(string(text))
				if err := flushState(state, false); err != nil {
					_ = invocation.Fail(err)
					return
				}
			}
			if chunk.IsEndOfStream() {
				if err := closeState(key, state, ""); err != nil {
					_ = invocation.Fail(err)
					return
				}
			}
			continue
		}

		if chunk.IsEndOfStream() {
			if state := states[key]; state != nil {
				updateTTSMeta(&state.meta, chunk)
				if err := closeState(key, state, ""); err != nil {
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
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return true
		}
	}
	return false
}

func updateTTSMeta(meta *TTSMeta, chunk *genx.MessageChunk) {
	meta.Role = chunk.Role
	meta.Name = chunk.Name
	if chunk.Ctrl != nil {
		if streamID := strings.TrimSpace(chunk.Ctrl.StreamID); streamID != "" {
			meta.StreamID = streamID
		}
		if chunk.Ctrl.Label != "" {
			meta.Label = chunk.Ctrl.Label
		}
	}
}
