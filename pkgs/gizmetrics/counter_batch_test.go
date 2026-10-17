package gizmetrics

import (
	"context"
	"fmt"
	"math"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	storemetrics "github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

func TestPeriodicCountersReuseCanonicalSeriesAndBoundCache(t *testing.T) {
	store := &fakeMetricsStore{}
	r := newStoreRecorder(store, recorderConfig{appendTimeout: time.Second, maxSeries: 2,
		labels: []Label{{Name: "node_id", Value: "node-a"}}})
	labels := []Label{{Name: "model", Value: "one"}, {Name: "provider", Value: "vendor"}}
	r.addPeriodicCounters(t.Context(), "tokens_total", []CounterDelta{{Value: 2, Labels: labels}})
	// Reordered labels and ordinary updates resolve to the same logical series.
	reordered := []Label{labels[1], labels[0]}
	r.addPeriodicCounters(t.Context(), "tokens_total", []CounterDelta{{Value: 3, Labels: reordered}})
	r.addCounter(t.Context(), "tokens_total", 5, labels)
	for i := range 10 {
		r.addPeriodicCounters(t.Context(), "tokens_total", []CounterDelta{{Value: 1,
			Labels: []Label{{Name: "model", Value: fmt.Sprint(i)}}}})
	}
	if len(r.series) != 2 || len(r.counterCache) > 2 {
		t.Fatalf("series=%d cache=%d, want bounded at 2", len(r.series), len(r.counterCache))
	}
	labels[0].Value = "changed"
	for range 2 {
		if err := r.flush(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	for _, batch := range store.appendBatches() {
		assertMetricSample(t, batch, "tokens_total", map[string]string{
			"node_id": "node-a", "model": "one", "provider": "vendor"}, 10)
	}
}

func TestPeriodicCountersValidateColdAndCachedUpdates(t *testing.T) {
	store := &fakeMetricsStore{}
	r := newStoreRecorder(store, recorderConfig{appendTimeout: time.Second, maxSeries: 20,
		labels: []Label{{Name: "node_id", Value: "node-a"}}})
	r.setGauge(t.Context(), "gauge", 1, nil)
	for range 2 { // Includes hits for a previously validated counter key.
		r.addPeriodicCounters(t.Context(), "tokens_total", []CounterDelta{
			{Value: 1, Labels: []Label{{Name: "kind", Value: "good"}}},
			{Value: -1, Labels: []Label{{Name: "kind", Value: "good"}}},
			{Value: math.NaN()}, {Value: math.Inf(1)},
			{Value: 1, Labels: []Label{{Name: "node_id", Value: "spoofed"}}},
			{Value: 1, Labels: []Label{{Name: "bad-label"}}},
			{Value: 1, Labels: []Label{{Name: "kind"}, {Name: "kind"}}},
		})
		r.addPeriodicCounters(t.Context(), "bad metric", []CounterDelta{{Value: 1}})
		r.addPeriodicCounters(t.Context(), "gauge", []CounterDelta{{Value: 1}})
	}
	r.addPeriodicCounters(t.Context(), "large_total", []CounterDelta{{Value: math.MaxFloat64}})
	r.addPeriodicCounters(t.Context(), "large_total", []CounterDelta{{Value: math.MaxFloat64}})
	if err := r.flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	batch := store.appendBatches()[0]
	if len(batch) != 3 {
		t.Fatalf("unexpected accepted series: %+v", batch)
	}
	assertMetricSample(t, batch, "tokens_total", map[string]string{"node_id": "node-a", "kind": "good"}, 2)
	assertMetricSample(t, batch, "gauge", map[string]string{"node_id": "node-a"}, 1)
	assertMetricSample(t, batch, "large_total", map[string]string{"node_id": "node-a"}, math.MaxFloat64)
}

func TestPeriodicCountersWideLabelsRetainCorrectTotals(t *testing.T) {
	store := &fakeMetricsStore{}
	r := newStoreRecorder(store, recorderConfig{appendTimeout: time.Second, maxSeries: 2})
	labels := []Label{{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}, {Name: "e"}}
	for range 2 {
		r.addPeriodicCounters(t.Context(), "wide_total", []CounterDelta{{Value: 3, Labels: labels}})
	}
	if len(r.counterCache) != 0 {
		t.Fatal("wide labels unexpectedly cached")
	}
	if err := r.flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertMetricSample(t, store.appendBatches()[0], "wide_total", map[string]string{
		"a": "", "b": "", "c": "", "d": "", "e": ""}, 6)
}

func TestPeriodicCounterCacheBelongsToInstalledRecorder(t *testing.T) {
	deltas := []CounterDelta{{Value: 2, Labels: []Label{{Name: "model", Value: "one"}}}}
	AddPeriodicCounters(t.Context(), "tokens_total", deltas...)
	for _, node := range []string{"node-a", "node-b"} {
		store := &fakeMetricsStore{}
		stop, err := InstallStore(store, WithFlushInterval(time.Hour), WithLabels(Label{Name: "node_id", Value: node}))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = stop(context.Background()) })
		old := processRecorder.Load().recorder
		AddPeriodicCounters(t.Context(), "tokens_total", deltas...)
		AddPeriodicCounters(t.Context(), "tokens_total", deltas...)
		if err := stop(context.Background()); err != nil {
			t.Fatal(err)
		}
		// Calls already holding the old slot cannot update it after shutdown.
		old.addPeriodicCounters(t.Context(), "tokens_total", deltas)
		AddPeriodicCounters(t.Context(), "tokens_total", deltas...)
		if err := old.flush(t.Context()); err != nil {
			t.Fatal(err)
		}
		for _, batch := range store.appendBatches() {
			assertMetricSample(t, batch, "tokens_total", map[string]string{"node_id": node, "model": "one"}, 4)
		}
	}
}

func TestPeriodicCountersConcurrentTotalsAndRetry(t *testing.T) {
	store := &fakeMetricsStore{appendErrs: []error{context.DeadlineExceeded}}
	r := newStoreRecorder(store, recorderConfig{appendTimeout: time.Second, maxSeries: 3})
	deltas := []CounterDelta{{Value: 1}, {Value: 2, Labels: []Label{{Name: "kind", Value: "output"}}}}
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			for range 100 {
				r.addPeriodicCounters(t.Context(), "tokens_total", deltas)
			}
		})
	}
	group.Wait()
	if err := r.flush(t.Context()); err == nil {
		t.Fatal("expected append failure")
	}
	if err := r.flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, batch := range store.appendBatches() {
		assertMetricSample(t, batch, "tokens_total", map[string]string{}, 800)
		assertMetricSample(t, batch, "tokens_total", map[string]string{"kind": "output"}, 1600)
	}
}

type notifyingMetricsStore struct {
	fakeMetricsStore
	started chan struct{}
}

func (s *notifyingMetricsStore) Append(ctx context.Context, samples []storemetrics.Sample) error {
	select {
	case s.started <- struct{}{}:
	default:
	}
	return s.fakeMetricsStore.Append(ctx, samples)
}

func TestCachedPeriodicBatchProgressesDuringBlockedFlush(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		store := &notifyingMetricsStore{fakeMetricsStore: fakeMetricsStore{block: release}, started: make(chan struct{}, 1)}
		r := newStoreRecorder(store, recorderConfig{appendTimeout: time.Hour, maxSeries: 3})
		deltas := []CounterDelta{{Value: 2}, {Value: 3, Labels: []Label{{Name: "kind", Value: "output"}}}}
		r.addPeriodicCounters(t.Context(), "tokens_total", deltas)
		flushed := make(chan error, 1)
		go func() { flushed <- r.flush(t.Context()) }()
		<-store.started
		done := make(chan struct{})
		go func() {
			r.addPeriodicCounters(t.Context(), "tokens_total", deltas)
			close(done)
		}()
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Fatal("cached counters waited for backend I/O")
		}
		close(release)
		if err := <-flushed; err != nil {
			t.Fatal(err)
		}
		if err := r.flush(t.Context()); err != nil {
			t.Fatal(err)
		}
		batches := store.appendBatches()
		if len(batches) != 2 {
			t.Fatalf("append batches = %d, want 2", len(batches))
		}
		assertMetricSample(t, batches[0], "tokens_total", map[string]string{}, 2)
		assertMetricSample(t, batches[1], "tokens_total", map[string]string{}, 4)
		assertMetricSample(t, batches[1], "tokens_total", map[string]string{"kind": "output"}, 6)
	})
}
