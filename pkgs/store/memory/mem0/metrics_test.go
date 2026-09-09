package mem0

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizmetrics"
	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

func TestRecallMetricsIncludeResultWithoutScopeOrQuery(t *testing.T) {
	sink := metrics.NewMemoryStore()
	stop, err := gizmetrics.InstallStore(sink, gizmetrics.WithFlushInterval(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stop(context.Background()) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer server.Close()
	store, err := New(Config{Endpoint: server.URL, Flavor: SelfHosted})
	if err != nil {
		t.Fatal(err)
	}
	query := memorystore.Query{Scope: memorystore.Scope{UserID: "private-user"}, Text: "private-query", Limit: 1}
	if _, err := store.Recall(t.Context(), query); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.Recall(ctx, query); err == nil {
		t.Fatal("canceled recall succeeded")
	}
	if err := stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	series, err := sink.Latest(t.Context(), metrics.LatestQuery{Selector: metrics.Selector{Name: "memory_recall_duration_seconds_count"}, At: time.Now(), Lookback: time.Minute})
	if err != nil || len(series) != 2 {
		t.Fatalf("series=%v err=%v", series, err)
	}
	for _, s := range series {
		if len(s.Labels) != 3 || s.Labels["backend"] != "mem0" || s.Points[0].Value != 1 {
			t.Fatalf("series=%v", s)
		}
	}
}
