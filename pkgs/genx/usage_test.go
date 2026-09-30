package genx

import (
	"context"
	"slices"
	"testing"
)

func TestRecordUsageDeliversNonZeroRecordsToContextRecorder(t *testing.T) {
	RecordUsage(context.Background(), UsageRecord{Provider: "p", Input: 1})

	var outer, inner []UsageRecord
	ctx := WithUsageRecorder(context.Background(), func(record UsageRecord) { outer = append(outer, record) })
	ctx = WithUsageRecorder(ctx, func(record UsageRecord) { inner = append(inner, record) })
	want := UsageRecord{Provider: "p", Model: "m", Modality: UsageModalityText, Unit: UsageUnitToken, Output: 3}
	RecordUsage(ctx, UsageRecord{Provider: "p", Model: "m"}, want)

	if len(outer) != 0 {
		t.Fatalf("replaced recorder got %+v, want nothing", outer)
	}
	if !slices.Equal(inner, []UsageRecord{want}) {
		t.Fatalf("recorder got %+v, want only %+v", inner, want)
	}
}

func TestTokenUsageRecordsSplitsModalitiesAndCache(t *testing.T) {
	// A realtime duplex report: input totals include audio and cached parts.
	got := TokenUsage{
		InputTokens: 2073, InputAudioTokens: 52, CachedTextTokens: 8, CachedAudioTokens: 2,
		OutputTokens: 182, OutputAudioTokens: 135,
	}.Records("volc", "model")
	want := []UsageRecord{
		{Provider: "volc", Model: "model", Modality: UsageModalityText, Unit: UsageUnitToken, Input: 2013, CachedInput: 8, Output: 47},
		{Provider: "volc", Model: "model", Modality: UsageModalityAudio, Unit: UsageUnitToken, Input: 50, CachedInput: 2, Output: 135},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Records() = %+v, want %+v", got, want)
	}

	inconsistent := TokenUsage{InputTokens: 1, InputAudioTokens: 5, OutputTokens: 1, OutputAudioTokens: 3}.Records("p", "m")
	if inconsistent[0].Input != 0 || inconsistent[0].Output != 0 {
		t.Fatalf("inconsistent text record = %+v, want zero input and output", inconsistent[0])
	}
}
