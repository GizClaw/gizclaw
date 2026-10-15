package genx

import (
	"context"
	"testing"
	"time"
)

func TestTTSBatchingContext(t *testing.T) {
	defaults := TTSBatchingFromContext(nil)
	if defaults.MaxRunes != 1000 || defaults.FirstMinRunes != 0 || defaults.FirstWait != 0 || defaults.MaxWait != 0 || defaults.SafetyMargin != 50*time.Millisecond {
		t.Fatalf("shared defaults = %+v", defaults)
	}
	parent, cancel := context.WithCancel(t.Context())
	config := TTSBatchConfig{MaxRunes: 120, FirstMinRunes: 12, FirstWait: 20 * time.Millisecond, MaxWait: time.Second, SynthesisLead: 500 * time.Millisecond, SafetyMargin: 40 * time.Millisecond}
	ctx, err := WithTTSBatching(parent, config)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxRunes = 1
	config.SafetyMargin = 0
	got := TTSBatchingFromContext(ctx)
	if got.MaxRunes != 120 || got.FirstMinRunes != 12 || got.FirstWait != 20*time.Millisecond || got.MaxWait != time.Second || got.SynthesisLead != 500*time.Millisecond || got.SafetyMargin != 40*time.Millisecond {
		t.Fatalf("configuration was not copied: %+v", got)
	}
	cancel()
	if ctx.Err() != context.Canceled {
		t.Fatalf("batching lost parent cancellation: %v", ctx.Err())
	}
	ctx, err = WithTTSBatching(nil, TTSBatchConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if got := TTSBatchingFromContext(ctx); got.MaxRunes != 1000 || got.MaxWait != 0 || got.FirstMinRunes != 0 {
		t.Fatalf("explicit zero waits/minimum were not preserved: %+v", got)
	}
	if got := TTSBatchingFromContext(parent); got.MaxRunes != defaults.MaxRunes || got.MaxWait != defaults.MaxWait {
		t.Fatalf("configuration escaped its child context: %+v", got)
	}
}

func TestTTSBatchingRejectsInvalidConfig(t *testing.T) {
	for _, config := range []TTSBatchConfig{
		{MaxRunes: -1}, {MaxRunes: 1<<20 + 1},
		{FirstMinRunes: -1}, {FirstMinRunes: 1<<20 + 1},
		{FirstWait: -time.Nanosecond}, {MaxWait: -time.Nanosecond},
		{SynthesisLead: -time.Nanosecond}, {SafetyMargin: -time.Nanosecond},
	} {
		if _, err := WithTTSBatching(t.Context(), config); err == nil {
			t.Errorf("accepted invalid configuration: %+v", config)
		}
	}
}
