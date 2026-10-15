package streamkit

import (
	"context"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type ttsInput struct {
	meta       TTSMeta
	text       string
	finish     bool
	messageEnd bool
	endStream  bool
}

func newTTSState(ctx context.Context, invocation *Invocation, response *Response, meta TTSMeta, mimeType string, run ttsStateRunner) *ttsStreamState {
	ctx, cancel := context.WithCancel(ctx)
	s := &ttsStreamState{meta: meta, response: response, ctx: ctx, cancel: cancel, input: make(chan ttsInput, 1), done: make(chan struct{})}
	go func() {
		defer close(s.done)
		defer cancel()
		s.err = run(s, invocation, mimeType)
		if s.err != nil && ctx.Err() == nil {
			_ = invocation.Fail(s.err)
		}
	}()
	return s
}

func (s *ttsStreamState) send(input ttsInput) error {
	select {
	case <-s.done:
		return s.err
	case <-s.ctx.Done():
		return context.Cause(s.ctx)
	case s.input <- input:
		return nil
	}
}

func (s *ttsStreamState) run(invocation *Invocation, mimeType string, synthesize TTSSynthesizer, measureFactory func() func([]byte) time.Duration) error {
	config := genx.TTSBatchingFromContext(s.ctx)
	segmenter := newTTSSentenceSegmenter(min(defaultTTSSegmentMaxRunes, config.MaxRunes))
	batch := ttsTextBatch{config: config}
	budget := newTTSSynthesisBudget(config, measureFactory != nil)
	var emitter *ttsSegmentEmitter
	defer func() {
		if emitter != nil {
			emitter.abort()
		}
	}()
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	var meta TTSMeta
	var ending ttsInput
	var blocked *ttsTextSegment
	segmentIndex := 0
	for {
		// Extract candidates without opening requests. A full pending batch
		// applies bounded backpressure, while output and timers remain active.
		exhausted := false
		for blocked == nil {
			segment, ok := segmenter.nextSegment(ending.finish)
			if !ok {
				exhausted = true
				break
			}
			if !batch.fits(segment) {
				blocked = &segment
				break
			}
			batch.append(segment, time.Now())
			// Startup can submit the first safe phrase immediately. Later
			// candidates remain mergeable while synthesis slots are occupied.
			if config.FirstWait == 0 && segmentIndex == 0 {
				if ready, _ := batch.due(time.Now(), true, false, budget); ready {
					break
				}
			}
		}
		ready, deadline := batch.due(time.Now(), segmentIndex == 0, blocked != nil || ending.finish, budget)
		if ready {
			if !hasReadableTTSSpokenText(batch.text.String()) {
				batch.take()
				continue
			}
			if emitter == nil {
				emitter = newTTSSegmentEmitter(s.ctx, func(ctx context.Context, meta TTSMeta, data []byte) error {
					// Delivery acknowledgement can wait while the consumer feeds
					// or plays this audio. Its elapsed time must consume budget.
					if len(data) != 0 {
						budget.markOutput()
					}
					return emitTTSAudio(ctx, invocation, s.response, &genx.MessageChunk{
						Part: &genx.Blob{MIMEType: mimeType, Data: data},
						Ctrl: &genx.StreamCtrl{MessageID: meta.MessageID},
					})
				})
			}
			accepted, err := emitter.tryEnqueue(func(ctx context.Context) *ttsSegmentJob {
				segmentIndex++
				jobMeta := meta
				jobMeta.SegmentIndex = segmentIndex
				text := batch.take()
				debugTTSSegment(s.ctx, jobMeta, text, ending.finish)
				return startMeasuredTTSSegment(ctx, text, jobMeta, mimeType, synthesize, measureFactory, budget)
			})
			if err != nil {
				return err
			}
			if accepted {
				continue
			}
		}
		if blocked != nil && batch.runes == 0 {
			batch.append(*blocked, time.Now())
			blocked = nil
			continue
		}
		if ending.finish && exhausted && batch.runes == 0 {
			if emitter != nil {
				finished := emitter
				emitter = nil
				if err := finished.finish(); err != nil {
					return err
				}
			}
			if ending.messageEnd {
				if err := emitTTSAudio(s.ctx, invocation, s.response, &genx.MessageChunk{
					Part: &genx.Blob{MIMEType: mimeType},
					Ctrl: &genx.StreamCtrl{MessageID: meta.MessageID, MessageEnd: true},
				}); err != nil {
					return err
				}
			}
			if ending.endStream {
				return invocation.FinishResponse(s.response, "")
			}
			ending = ttsInput{}
		}
		var changed, failed <-chan struct{}
		if emitter != nil {
			changed, failed = emitter.changed, emitter.failed
		}
		var timerC <-chan time.Time
		timer.Stop()
		if !ready && !deadline.IsZero() {
			timer.Reset(max(0, time.Until(deadline)))
			timerC = timer.C
		}
		input := s.input
		if blocked != nil || ending.finish {
			input = nil
		}
		select {
		case <-s.ctx.Done():
			return context.Cause(s.ctx)
		case <-failed:
			return emitter.err
		case <-changed:
		case <-budget.changed:
		case <-timerC:
		case next := <-input:
			meta = next.meta
			segmenter.WriteString(next.text)
			if next.finish {
				ending = next
			}
		}
	}
}

func sameTTSSpeech(a, b TTSMeta) bool {
	return a.MessageID == b.MessageID && a.Role == b.Role && a.Name == b.Name && a.Label == b.Label
}
