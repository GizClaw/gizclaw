package streamkit

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// TTSStreamingSession accepts incremental text whose sentence boundaries are
// owned by the provider. Flush and Finish wait until the corresponding audio
// has crossed emit, preserving message identity and final-delivery backpressure.
// Close must cancel IO before joining the session's workers.
type TTSStreamingSession interface {
	SendText(context.Context, string) error
	Flush(context.Context) error
	Finish(context.Context) error
	Close() error
	Done() <-chan struct{}
	Err() error // valid after Done closes
}

// TTSStreamingFactory starts one invocation-local provider session. emit must
// be called serially with the session context, which must derive from the
// factory context. Cancelling that context releases delivery before Close
// joins workers; emit may block until downstream delivery or cancellation.
type TTSStreamingFactory func(context.Context, TTSMeta, func(context.Context, []byte) error) (TTSStreamingSession, error)

// NewStreamingTTSStream manages routes, normalization and message lifecycle
// without client sentence segmentation or synthesis batching. A transformer
// chooses this helper for a provider with native streaming text input, or
// NewMeasuredTTSStream for client-segmented synthesis requests.
func NewStreamingTTSStream(ctx context.Context, input genx.Stream, config OutputConfig, mimeType string, open TTSStreamingFactory) *Output {
	invocation := NewInvocation(ctx, config)
	go runTTS(invocation, input, mimeType, func(state *ttsStreamState, invocation *Invocation, mimeType string) error {
		return state.runStreaming(invocation, mimeType, open)
	})
	return invocation.Output()
}

func (s *ttsStreamState) runStreaming(invocation *Invocation, mimeType string, open TTSStreamingFactory) (resultErr error) {
	var session TTSStreamingSession
	defer func() {
		if session != nil {
			if err := session.Close(); resultErr == nil {
				resultErr = err
			}
		}
	}()
	var meta TTSMeta
	var emissionMeta atomic.Pointer[TTSMeta]
	normalizer := ttsTextNormalizer{preserveNewlines: true}
	var prefix strings.Builder
	turnDirty := false
	providerError := func(err error) error {
		if s.ctx.Err() != nil {
			return context.Cause(s.ctx)
		}
		return genx.ClassifyFailure(err, genx.FailureClassProvider)
	}
	send := func(text string) error {
		if text == "" {
			return nil
		}
		if session == nil {
			prefix.WriteString(text)
			if !hasReadableTTSSpokenText(prefix.String()) {
				if prefix.Len() > 65536 {
					return errors.New("streamkit: unreadable TTS prefix exceeds 64 KiB")
				}
				return nil
			}
			var err error
			session, err = open(s.ctx, meta, func(ctx context.Context, data []byte) error {
				current := emissionMeta.Load()
				return emitTTSAudio(ctx, invocation, s.response, &genx.MessageChunk{
					Part: &genx.Blob{MIMEType: mimeType, Data: data},
					Ctrl: &genx.StreamCtrl{MessageID: current.MessageID},
				})
			})
			if err != nil {
				return providerError(err)
			}
			text = prefix.String()
			prefix.Reset()
		}
		if err := session.SendText(s.ctx, text); err != nil {
			return providerError(err)
		}
		turnDirty = true
		return nil
	}
	for {
		var providerDone <-chan struct{}
		if session != nil {
			providerDone = session.Done()
		}
		select {
		case <-s.ctx.Done():
			return context.Cause(s.ctx)
		case <-providerDone:
			err := session.Err()
			if err == nil {
				err = io.ErrUnexpectedEOF
			}
			return providerError(err)
		case next := <-s.input:
			meta = next.meta
			copyMeta := meta
			emissionMeta.Store(&copyMeta)
			if err := send(normalizer.WriteString(next.text)); err != nil {
				return err
			}
			if !next.finish {
				continue
			}
			if err := send(normalizer.Finish()); err != nil {
				return err
			}
			if session != nil && (turnDirty || next.endStream) {
				var err error
				if next.endStream {
					err = session.Finish(s.ctx)
				} else {
					err = session.Flush(s.ctx)
				}
				if err != nil {
					return providerError(err)
				}
			}
			turnDirty = false
			prefix.Reset()
			if next.messageEnd {
				if err := emitTTSAudio(s.ctx, invocation, s.response, &genx.MessageChunk{
					Part: &genx.Blob{MIMEType: mimeType},
					Ctrl: &genx.StreamCtrl{MessageID: meta.MessageID, MessageEnd: true},
				}); err != nil {
					return err
				}
			}
			if next.endStream {
				return invocation.FinishResponse(s.response, "")
			}
		}
	}
}
