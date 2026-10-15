package genx

import (
	"context"
	"fmt"
	"time"
)

// TTSBatchConfig controls text submission in the shared streaming TTS pipeline.
// It does not change Unicode boundary detection or provider speech settings.
type TTSBatchConfig struct {
	// MaxRunes bounds one synthesis request. Zero uses the default of 1000.
	MaxRunes int
	// FirstMinRunes optionally delays the first request until this phrase size.
	// Complete closed sentences submit immediately; other sentence candidates
	// need at most four runes. Zero submits the first confirmed readable segment.
	FirstMinRunes int
	// FirstWait optionally collects more text before the first request.
	// A complete closed sentence can submit earlier. Zero submits immediately.
	FirstWait time.Duration
	// MaxWait optionally caps collection after the first request. Zero adds no
	// fixed cap: generated audio and synthesis latency determine the deadline.
	MaxWait time.Duration
	// SynthesisLead sets a minimum reserve for the next request's first audio.
	// The pipeline also reserves at least the largest first-audio latency it
	// has observed in this route. Zero relies on those observations.
	SynthesisLead time.Duration
	// SafetyMargin is additional time reserved before estimated audio runs out.
	SafetyMargin time.Duration
	// WaitBudget optionally supplies the current remaining time available for
	// collecting text, after allowing for synthesis latency. Zero or a negative
	// value makes the next safe batch urgent. The callback must return promptly;
	// routes can call it concurrently outside locks. It can shorten the automatic
	// audio budget. For formats without duration measurement it supplies the
	// budget directly. MaxWait, when nonzero, remains an upper bound.
	WaitBudget func() time.Duration
}

type ttsBatchConfigKey struct{}

// DefaultTTSBatchConfig submits the first segment immediately, then collects
// within the generated audio's remaining duration, reserving observed synthesis
// latency and 50 ms of margin. Requests contain at most 1000 runes.
func DefaultTTSBatchConfig() TTSBatchConfig {
	return TTSBatchConfig{MaxRunes: 1000, SafetyMargin: 50 * time.Millisecond}
}

// WithTTSBatching configures one invocation, including TTS composed by Audio
// Dock. The config is copied; zero MaxWait leaves the audio budget uncapped.
// MaxRunes must be zero or between 1 and 1,048,576; waits cannot be negative.
func WithTTSBatching(ctx context.Context, config TTSBatchConfig) (context.Context, error) {
	if config.MaxRunes < 0 || config.MaxRunes > 1<<20 {
		return nil, fmt.Errorf("genx: TTS MaxRunes must be between 1 and 1048576, or zero for the default")
	}
	if config.FirstWait < 0 || config.MaxWait < 0 || config.SynthesisLead < 0 || config.SafetyMargin < 0 {
		return nil, fmt.Errorf("genx: TTS batching durations cannot be negative")
	}
	if config.FirstMinRunes < 0 || config.FirstMinRunes > 1<<20 {
		return nil, fmt.Errorf("genx: TTS FirstMinRunes must be between 0 and 1048576")
	}
	if config.MaxRunes == 0 {
		config.MaxRunes = DefaultTTSBatchConfig().MaxRunes
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ttsBatchConfigKey{}, config), nil
}

// TTSBatchingFromContext returns the invocation's immutable batching config,
// or the shared defaults when WithTTSBatching was not used.
func TTSBatchingFromContext(ctx context.Context) TTSBatchConfig {
	if ctx != nil {
		if config, ok := ctx.Value(ttsBatchConfigKey{}).(TTSBatchConfig); ok {
			return config
		}
	}
	return DefaultTTSBatchConfig()
}
