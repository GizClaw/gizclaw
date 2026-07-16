package gizmetrics

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"
)

func TestDefaultRecorderIsConcurrentNoop(t *testing.T) {
	if processRecorder.Load() != nil {
		t.Fatal("process recorder unexpectedly installed")
	}
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			for range 1_000 {
				AddCounter(context.Background(), "invalid metric name", -1, Label{Name: "bad-label", Value: "value"})
				SetGauge(context.Background(), "gauge", math.NaN())
				ObserveHistogram(context.Background(), "histogram", 1, nil)
			}
		})
	}
	group.Wait()
	if processRecorder.Load() != nil {
		t.Fatal("no-op recording installed a recorder")
	}
}

func TestInstallStoreLifecycleAndAggregation(t *testing.T) {
	store := &fakeMetricsStore{}
	shutdown, err := InstallStore(store, WithFlushInterval(time.Hour), WithMaxSeries(10))
	if err != nil {
		t.Fatalf("InstallStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = shutdown(context.Background())
	})
	if _, err := InstallStore(&fakeMetricsStore{}); !errors.Is(err, ErrAlreadyInstalled) {
		t.Fatalf("second InstallStore() error = %v, want %v", err, ErrAlreadyInstalled)
	}

	labels := []Label{{Name: "surface", Value: "peer-http"}, {Name: "operation", Value: "login"}}
	AddCounter(context.Background(), "request_total", 1, labels...)
	AddCounter(context.Background(), "request_total", 2, labels...)
	SetGauge(context.Background(), "active_requests", 7, labels...)
	ObserveHistogram(context.Background(), "request_duration_seconds", 0.1, []float64{0.5, 1}, labels...)
	ObserveHistogram(context.Background(), "request_duration_seconds", 0.7, []float64{0.5, 1}, labels...)
	labels[0].Value = "mutated"

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}
	if store.closeCalls != 0 {
		t.Fatalf("store Close calls = %d, want 0", store.closeCalls)
	}
	if got := len(store.appendBatches()); got != 1 {
		t.Fatalf("append batches = %d, want 1", got)
	}
	samples := store.appendBatches()[0]
	assertMetricSample(t, samples, "request_total", map[string]string{"operation": "login", "surface": "peer-http"}, 3)
	assertMetricSample(t, samples, "active_requests", map[string]string{"operation": "login", "surface": "peer-http"}, 7)
	assertMetricSample(t, samples, "request_duration_seconds_bucket", map[string]string{"operation": "login", "surface": "peer-http", "le": "0.5"}, 1)
	assertMetricSample(t, samples, "request_duration_seconds_bucket", map[string]string{"operation": "login", "surface": "peer-http", "le": "1"}, 2)
	assertMetricSample(t, samples, "request_duration_seconds_bucket", map[string]string{"operation": "login", "surface": "peer-http", "le": "+Inf"}, 2)
	assertMetricSample(t, samples, "request_duration_seconds_sum", map[string]string{"operation": "login", "surface": "peer-http"}, 0.8)
	assertMetricSample(t, samples, "request_duration_seconds_count", map[string]string{"operation": "login", "surface": "peer-http"}, 2)

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("second shutdown() error = %v", err)
	}
	AddCounter(context.Background(), "request_total", 100)
	if got := len(store.appendBatches()); got != 1 {
		t.Fatalf("append batches after shutdown = %d, want 1", got)
	}
}

func TestInstallStoreValidatesInputs(t *testing.T) {
	var typedNil *fakeMetricsStore
	if _, err := InstallStore(typedNil); !errors.Is(err, ErrStoreRequired) {
		t.Fatalf("InstallStore(typed nil) error = %v, want %v", err, ErrStoreRequired)
	}
	tests := []struct {
		name   string
		option Option
	}{
		{name: "nil option", option: nil},
		{name: "flush interval", option: WithFlushInterval(0)},
		{name: "append timeout", option: WithAppendTimeout(-time.Second)},
		{name: "max series", option: WithMaxSeries(0)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := InstallStore(&fakeMetricsStore{}, test.option); err == nil {
				t.Fatal("InstallStore() expected validation error")
			}
		})
	}
}
