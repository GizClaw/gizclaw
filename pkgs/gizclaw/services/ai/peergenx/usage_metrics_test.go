package peergenx

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizmetrics"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

func TestNestedUsageScopeReportsTokenMetricsOnce(t *testing.T) {
	store := metrics.NewMemoryStore()
	stop, err := gizmetrics.InstallStore(store, gizmetrics.WithFlushInterval(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stop(context.Background()) })
	var caller, outer, inner int
	parent := genx.WithUsageRecorder(t.Context(), func(genx.UsageRecord) { caller++ })
	ctx := providerUsageContext(parent, func(genx.UsageRecord) { outer++ })
	ctx = providerUsageContext(ctx, func(genx.UsageRecord) { inner++ })
	genx.RecordUsage(ctx, genx.UsageRecord{Provider: "volc", Model: "billing-model", Modality: genx.UsageModalityText, Unit: genx.UsageUnitToken, Input: 7, CachedInput: 3, Output: 4})
	if caller != 1 || outer != 0 || inner != 1 {
		t.Fatalf("caller=%d outer=%d inner=%d", caller, outer, inner)
	}
	if err := stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	series, err := store.Latest(t.Context(), metrics.LatestQuery{Selector: metrics.Selector{Name: "genx_model_tokens_total"}, At: time.Now(), Lookback: time.Minute})
	if err != nil || len(series) != 3 {
		t.Fatalf("series=%v error=%v, want three token types", series, err)
	}
	want := map[string]float64{"input": 7, "cached_input": 3, "output": 4}
	for _, item := range series {
		if item.Points[0].Value != want[item.Labels["token_type"]] {
			t.Fatalf("nested forwarding counted tokens more than once: %+v", item)
		}
	}
}
