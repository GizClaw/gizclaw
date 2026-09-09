package streamlog

import (
	"context"
	"log/slog"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// Model identifies a configured speech model. Kind is asr, tts, ast, or realtime.
// Model is a provider model/version/resource ID, never a request or voice ID.
type Model struct{ Provider, Model, Kind string }
type modelKey struct{}
type stageKey struct{}
type stage struct{ input, output *Recorder }

// StartStage creates bounded input and output recorders for one Transformer
// invocation. It starts no goroutine and preserves the caller's identity.
func StartStage(ctx context.Context, name string, models ...Model) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	metadata := Model{}
	if len(models) > 0 {
		metadata = models[0]
	}
	ctx = context.WithValue(ctx, modelKey{}, metadata)
	logger := slog.Default().With("transformer", name)
	if metadata.Model != "" {
		logger = logger.With("provider", metadata.Provider, "model", metadata.Model)
	}
	return context.WithValue(ctx, stageKey{}, &stage{
		input:  New(ctx, logger, "transformer_input"),
		output: New(ctx, logger, "transformer_output"),
	})
}

// OutputRecorder returns the current stage's pull-boundary recorder, or nil.
func OutputRecorder(ctx context.Context) *Recorder {
	if ctx == nil {
		return nil
	}
	if s, _ := ctx.Value(stageKey{}).(*stage); s != nil {
		return s.output
	}
	return nil
}

// ReadInput observes the original stream without wrapping or changing its
// optional delivery and interruption interfaces. Terminal reads flush text.
func ReadInput(ctx context.Context, input genx.Stream) (*genx.MessageChunk, error) {
	chunk, err := input.Next()
	ObserveInputRead(ctx, chunk, err)
	return chunk, err
}

// ObserveInputRead records a native read that also supports provider-specific
// stop signals. It does not change the read result or its ownership.
func ObserveInputRead(ctx context.Context, chunk *genx.MessageChunk, err error) {
	if ctx == nil {
		return
	}
	if s, _ := ctx.Value(stageKey{}).(*stage); s != nil {
		if chunk != nil {
			s.output.ObserveInput(chunk)
			s.input.Observe(chunk)
		}
		if err != nil {
			s.input.Close(err)
		}
	}
}
