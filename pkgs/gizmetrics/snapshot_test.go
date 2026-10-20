package gizmetrics

import (
	"testing"
	"testing/synctest"
	"time"
)

func TestSnapshotReleasesStateLockAndRetainsConcurrentHistogramUpdates(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newStoreRecorder(&fakeMetricsStore{}, recorderConfig{maxSeries: 10})
		labels := []Label{{Name: "kind", Value: "text"}}
		r.observeHistogram(t.Context(), "latency", 0.25, []float64{0.5, 1}, labels)
		pending := r.snapshot()
		if len(pending) != 1 {
			t.Fatalf("captured series = %d, want 1", len(pending))
		}
		// While the captured batch awaits serialization, new observations can
		// complete, including updates to the same histogram's mutable buckets.
		done := make(chan struct{})
		go func() {
			r.observeHistogram(t.Context(), "latency", 0.75, []float64{0.5, 1}, labels)
			close(done)
		}()
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Fatal("observation blocked on a captured snapshot")
		}
		// Materialization needs only captured state, even if the live recorder
		// lock is unavailable. A same-resource writer must still serialize.
		r.mu.Lock()
		rendered := make(chan struct{})
		go func() {
			defer close(rendered)
			samples := seriesSamples(&pending[0].series, time.Now())
			assertMetricSample(t, samples, "latency_bucket", map[string]string{"kind": "text", "le": "1"}, 1)
			assertMetricSample(t, samples, "latency_sum", map[string]string{"kind": "text"}, 0.25)
		}()
		synctest.Wait()
		select {
		case <-rendered:
		default:
			r.mu.Unlock()
			t.Fatal("materialization waited for recorder state lock")
		}
		blocked := make(chan struct{})
		started := make(chan struct{})
		go func() {
			close(started)
			r.observeHistogram(t.Context(), "latency", 0.5, []float64{0.5, 1}, labels)
			close(blocked)
		}()
		<-started
		select {
		case <-blocked:
			r.mu.Unlock()
			t.Fatal("same-resource update bypassed serialization")
		default:
		}
		r.mu.Unlock()
		<-blocked
		r.markClean(pending) // An older successful append must not lose updates.
		latest := r.snapshot()
		if len(latest) != 1 {
			t.Fatal("concurrent observations were marked clean")
		}
		samples := seriesSamples(&latest[0].series, time.Now())
		assertMetricSample(t, samples, "latency_count", map[string]string{"kind": "text"}, 3)
		assertMetricSample(t, samples, "latency_sum", map[string]string{"kind": "text"}, 1.5)
		assertMetricSample(t, samples, "latency_bucket", map[string]string{"kind": "text", "le": "0.5"}, 2)
		r.markClean(latest)
		if len(r.snapshot()) != 0 {
			t.Fatal("successfully appended unchanged histogram was not cleaned")
		}
	})
}
