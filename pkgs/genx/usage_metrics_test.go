package genx

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizmetrics"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

func TestRecordUsageMetricsSeparateModelsModalitiesAndCachedInput(t *testing.T) {
	store := metrics.NewMemoryStore()
	stop, err := gizmetrics.InstallStore(store, gizmetrics.WithFlushInterval(time.Hour), gizmetrics.WithLabels(gizmetrics.Label{Name: "node_id", Value: "server-a"}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stop(context.Background()) })
	// No product recorder or SQL metering configuration is needed. Provider
	// totals include cache/audio; Records splits them into disjoint quantities.
	RecordUsage(t.Context(), TokenUsage{
		InputTokens: 100, InputAudioTokens: 20, CachedTextTokens: 30, CachedAudioTokens: 5,
		OutputTokens: 40, OutputAudioTokens: 10,
	}.Records("volc", "model-a")...)
	RecordUsage(t.Context(), UsageRecord{Provider: "volc", Model: "model-b", Unit: UsageUnitToken, Modality: UsageModalityText, Input: 2, Output: 3})
	RecordUsage(t.Context(), UsageRecord{Provider: "gemini", Model: "model-a", Unit: UsageUnitToken, Modality: UsageModalityText, Input: 4})
	if err := stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	series, err := store.Latest(t.Context(), metrics.LatestQuery{Selector: metrics.Selector{Name: "genx_model_tokens_total"}, At: time.Now(), Lookback: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[[4]string]float64)
	for _, item := range series {
		if len(item.Labels) != 5 || item.Labels["node_id"] != "server-a" || len(item.Points) != 1 {
			t.Fatalf("unexpected identity or points: %+v", item)
		}
		key := [4]string{item.Labels["provider"], item.Labels["model"], item.Labels["modality"], item.Labels["token_type"]}
		got[key] = item.Points[0].Value
	}
	want := map[[4]string]float64{
		{"volc", "model-a", "text", "input"}: 50, {"volc", "model-a", "text", "cached_input"}: 30, {"volc", "model-a", "text", "output"}: 30,
		{"volc", "model-a", "audio", "input"}: 15, {"volc", "model-a", "audio", "cached_input"}: 5, {"volc", "model-a", "audio", "output"}: 10,
		{"volc", "model-b", "text", "input"}: 2, {"volc", "model-b", "text", "output"}: 3,
		{"gemini", "model-a", "text", "input"}: 4,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("token totals = %v, want %v", got, want)
	}
}

func TestRecordUsageMetricsSkipOtherUnitsAndMalformedRecordsWithoutChangingCallbacks(t *testing.T) {
	store := metrics.NewMemoryStore()
	stop, err := gizmetrics.InstallStore(store, gizmetrics.WithFlushInterval(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stop(context.Background()) })
	base := UsageRecord{Provider: "volc", Model: "model", Modality: UsageModalityText, Unit: UsageUnitToken, Input: 1}
	var observed []UsageRecord
	ctx := WithUsageRecorder(t.Context(), func(record UsageRecord) { observed = append(observed, record) })
	for _, mutate := range []func(*UsageRecord){
		func(r *UsageRecord) { r.Unit = UsageUnitCharacter },
		func(r *UsageRecord) { r.Unit = UsageUnitMillisecond },
		func(r *UsageRecord) { r.Unit = "unknown" },
		func(r *UsageRecord) { r.Modality = "unknown" },
		func(r *UsageRecord) { r.Provider = "" },
		func(r *UsageRecord) { r.Provider = strings.Repeat("p", 65) },
		func(r *UsageRecord) { r.Model = "" },
		func(r *UsageRecord) { r.Model = strings.Repeat("m", 257) },
		func(r *UsageRecord) { r.Model = " model " },
		func(r *UsageRecord) { r.Model = "model\nrequest" },
		func(r *UsageRecord) { r.Model = string([]byte{0xff}) },
		func(r *UsageRecord) { r.Input = -1 },
		func(r *UsageRecord) { r.CachedInput = -1 },
		func(r *UsageRecord) { r.Output = -1 },
	} {
		record := base
		mutate(&record)
		before := len(observed)
		RecordUsage(ctx, record)
		if len(observed) != before+1 || observed[before] != record {
			t.Fatalf("metrics changed callback delivery of %+v", record)
		}
	}
	RecordUsage(nil, base)
	RecordUsage(ctx, UsageRecord{})
	if err := stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	series, err := store.Latest(t.Context(), metrics.LatestQuery{Selector: metrics.Selector{Name: "genx_model_tokens_total"}, At: time.Now(), Lookback: time.Minute})
	if err != nil || len(series) != 0 {
		t.Fatalf("unexpected token series: %v, error=%v", series, err)
	}
	if UsageRecorderFromContext(nil) != nil || UsageRecorderFromContext(t.Context()) != nil {
		t.Fatal("missing callback should be nil")
	}
}

func TestRecordUsageMetricsAggregateConcurrentReportsIntoFixedSeries(t *testing.T) {
	store := metrics.NewMemoryStore()
	stop, err := gizmetrics.InstallStore(store, gizmetrics.WithFlushInterval(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stop(context.Background()) })
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // A reported billable quantity still counts after cancellation.
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			for range 100 {
				RecordUsage(ctx, UsageRecord{Provider: "openai", Model: "model-a", Modality: UsageModalityText, Unit: UsageUnitToken, Input: 3, CachedInput: 1, Output: 2})
			}
		})
	}
	group.Wait()
	if err := stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	series, err := store.Latest(t.Context(), metrics.LatestQuery{Selector: metrics.Selector{Name: "genx_model_tokens_total"}, At: time.Now(), Lookback: time.Minute})
	if err != nil || len(series) != 3 {
		t.Fatalf("series=%v error=%v, want three fixed series", series, err)
	}
	want := map[string]float64{"input": 2400, "cached_input": 800, "output": 1600}
	for _, item := range series {
		if len(item.Labels) != 4 || item.Points[0].Value != want[item.Labels["token_type"]] {
			t.Fatalf("unexpected concurrent total: %+v", item)
		}
	}
}
