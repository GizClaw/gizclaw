package streamkit

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type ttsBackpressureKey struct{}

// WithTTSBackpressure makes the shared TTS emitter wait for final delivery of
// each audio chunk. Composition layers must forward deferred observations to
// that boundary. The two bounded synthesis jobs can still prefetch audio.
func WithTTSBackpressure(ctx context.Context) context.Context {
	return context.WithValue(ctx, ttsBackpressureKey{}, true)
}

func emitTTSAudio(ctx context.Context, invocation *Invocation, response *Response, chunk *genx.MessageChunk) error {
	if paced, _ := ctx.Value(ttsBackpressureKey{}).(bool); !paced {
		return invocation.Emit(response, chunk)
	}
	done := make(chan bool, 1)
	if err := invocation.EmitTracked(response, chunk, func(*genx.MessageChunk) {
		done <- true
	}, func(*genx.MessageChunk) {
		done <- false
	}); err != nil {
		return err
	}
	// Never wait while holding an invocation or response mutex: interrupt and
	// cancellation must be able to discard the outstanding delivery.
	select {
	case delivered := <-done:
		if !delivered {
			return ErrInactiveResponse
		}
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}
