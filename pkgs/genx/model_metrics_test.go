package genx

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizmetrics"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

func TestModelTimingSkipsEmptyAndFailedFirstText(t *testing.T) {
	store := metrics.NewMemoryStore()
	stop, err := gizmetrics.InstallStore(store, gizmetrics.WithFlushInterval(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stop(context.Background()) })
	timing := newModelTiming(t.Context(), "test", "model-a")
	timing.started = time.Now().Add(-time.Second)
	timing.text(" ")
	timing.text("hello")
	timing.text("world")
	timing.finish(nil)
	failed := newModelTiming(t.Context(), "test", "model-a")
	failed.text("")
	failed.finish(context.DeadlineExceeded)
	if err := stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	series, err := store.Latest(t.Context(), metrics.LatestQuery{Selector: metrics.Selector{Name: "genx_model_first_text_seconds_count"}, At: time.Now(), Lookback: time.Minute})
	if err != nil || len(series) != 1 || series[0].Points[0].Value != 1 {
		t.Fatalf("first samples=%v err=%v", series, err)
	}
	series, err = store.Latest(t.Context(), metrics.LatestQuery{Selector: metrics.Selector{Name: "genx_model_request_duration_seconds_count"}, At: time.Now(), Lookback: time.Minute})
	if err != nil || len(series) != 2 {
		t.Fatalf("request results=%v err=%v", series, err)
	}
	for _, s := range series {
		if s.Points[0].Value != 1 || (s.Labels["result"] != "success" && s.Labels["result"] != "timeout") {
			t.Fatalf("labels=%v", s.Labels)
		}
	}
}
