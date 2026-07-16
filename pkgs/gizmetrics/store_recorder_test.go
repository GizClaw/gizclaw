package gizmetrics

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"maps"
	"math"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	storemetrics "github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

type fakeMetricsStore struct {
	mu         sync.Mutex
	batches    [][]storemetrics.Sample
	appendErrs []error
	block      <-chan struct{}
	closeCalls int
}

func (s *fakeMetricsStore) Append(ctx context.Context, samples []storemetrics.Sample) error {
	cloned := cloneSamples(samples)
	s.mu.Lock()
	s.batches = append(s.batches, cloned)
	var err error
	if len(s.appendErrs) > 0 {
		err = s.appendErrs[0]
		s.appendErrs = s.appendErrs[1:]
	}
	block := s.block
	s.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

func (s *fakeMetricsStore) Latest(context.Context, storemetrics.LatestQuery) (storemetrics.SeriesSet, error) {
	return nil, nil
}

func (s *fakeMetricsStore) Range(context.Context, storemetrics.RangeQuery) (storemetrics.SeriesSet, error) {
	return nil, nil
}

func (s *fakeMetricsStore) Aggregate(context.Context, storemetrics.AggregateQuery) (storemetrics.SeriesSet, error) {
	return nil, nil
}

func (s *fakeMetricsStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeCalls++
	return nil
}

func (s *fakeMetricsStore) appendBatches() [][]storemetrics.Sample {
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := make([][]storemetrics.Sample, len(s.batches))
	for index, batch := range s.batches {
		cloned[index] = cloneSamples(batch)
	}
	return cloned
}

func TestStoreRecorderRetainsDirtySeriesAfterFailure(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	appendErr := errors.New("unavailable")
	store := &fakeMetricsStore{appendErrs: []error{appendErr}}
	recorder := newStoreRecorder(store, recorderConfig{flushInterval: time.Hour, appendTimeout: time.Second, maxSeries: 10})
	recorder.addCounter(context.Background(), "retry_total", 2, []Label{{Name: "kind", Value: "test"}})
	if err := recorder.flush(context.Background()); !errors.Is(err, appendErr) {
		t.Fatalf("first flush error = %v, want %v", err, appendErr)
	}
	if !strings.Contains(logs.String(), "gizmetrics: flush failed") || strings.Contains(logs.String(), appendErr.Error()) {
		t.Fatalf("flush warning = %s", logs.String())
	}
	if err := recorder.flush(context.Background()); err != nil {
		t.Fatalf("retry flush error = %v", err)
	}
	batches := store.appendBatches()
	if len(batches) != 2 {
		t.Fatalf("append batches = %d, want 2", len(batches))
	}
	for _, batch := range batches {
		assertMetricSample(t, batch, "retry_total", map[string]string{"kind": "test"}, 2)
	}
	if err := recorder.flush(context.Background()); err != nil {
		t.Fatalf("clean flush error = %v", err)
	}
	if got := len(store.appendBatches()); got != 2 {
		t.Fatalf("append batches after clean flush = %d, want 2", got)
	}
}

func TestStoreRecorderBoundsAppendTimeoutAndRetries(t *testing.T) {
	block := make(chan struct{})
	store := &fakeMetricsStore{block: block}
	recorder := newStoreRecorder(store, recorderConfig{flushInterval: time.Hour, appendTimeout: 10 * time.Millisecond, maxSeries: 10})
	recorder.setGauge(context.Background(), "blocked_gauge", 1, nil)
	if err := recorder.flush(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked flush error = %v, want deadline exceeded", err)
	}
	close(block)
	if err := recorder.flush(context.Background()); err != nil {
		t.Fatalf("retry flush error = %v", err)
	}
	if got := len(store.appendBatches()); got != 2 {
		t.Fatalf("append attempts = %d, want 2", got)
	}
}

func TestStoreRecorderDropsAggregateOverflow(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	store := &fakeMetricsStore{}
	recorder := newStoreRecorder(store, recorderConfig{flushInterval: time.Hour, appendTimeout: time.Second, maxSeries: 10})
	recorder.addCounter(context.Background(), "large_total", math.MaxFloat64, nil)
	recorder.addCounter(context.Background(), "large_total", math.MaxFloat64, nil)
	recorder.observeHistogram(context.Background(), "large_value", math.MaxFloat64, []float64{math.MaxFloat64}, nil)
	recorder.observeHistogram(context.Background(), "large_value", math.MaxFloat64, []float64{math.MaxFloat64}, nil)
	if err := recorder.flush(context.Background()); err != nil {
		t.Fatalf("flush() error = %v", err)
	}
	samples := store.appendBatches()[0]
	assertMetricSample(t, samples, "large_total", map[string]string{}, math.MaxFloat64)
	assertMetricSample(t, samples, "large_value_sum", map[string]string{}, math.MaxFloat64)
	assertMetricSample(t, samples, "large_value_count", map[string]string{}, 1)
	if !strings.Contains(logs.String(), "must remain finite") {
		t.Fatalf("overflow warning = %s", logs.String())
	}
}

func TestStoreRecorderDropsInvalidAndExcessSeriesWithRateLimitedWarning(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	store := &fakeMetricsStore{}
	shutdown, err := InstallStore(store, WithFlushInterval(time.Hour), WithMaxSeries(1))
	if err != nil {
		t.Fatalf("InstallStore() error = %v", err)
	}
	AddCounter(context.Background(), "invalid metric secret-value", 1)
	AddCounter(context.Background(), strings.Repeat("a", maxMetricNameSize+1), 1)
	AddCounter(context.Background(), "valid_total", 1, Label{Name: "kind", Value: "kept"})
	AddCounter(context.Background(), "overflow_total", 1)
	AddCounter(context.Background(), "valid_total", -1, Label{Name: "kind", Value: "kept"})
	SetGauge(context.Background(), "valid_total", 1, Label{Name: "kind", Value: "kept"})
	ObserveHistogram(context.Background(), "histogram", 1, []float64{1, 1})
	ObserveHistogram(context.Background(), "histogram", 1, []float64{1}, Label{Name: "le", Value: "1"})
	AddCounter(context.Background(), "duplicate_total", 1, Label{Name: "kind", Value: "a"}, Label{Name: "kind", Value: "b"})
	SetGauge(context.Background(), "nan_gauge", math.NaN())
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}
	batches := store.appendBatches()
	if len(batches) != 1 || len(batches[0]) != 1 {
		t.Fatalf("appended samples = %#v, want one valid series", batches)
	}
	assertMetricSample(t, batches[0], "valid_total", map[string]string{"kind": "kept"}, 1)
	if got := strings.Count(logs.String(), "gizmetrics: metric update dropped"); got != 1 {
		t.Fatalf("warning count = %d, want 1: %s", got, logs.String())
	}
	if !strings.Contains(logs.String(), "invalid metric name") {
		t.Fatalf("warning missing fixed reason: %s", logs.String())
	}
	if strings.Contains(logs.String(), "secret-value") || strings.Contains(logs.String(), `"metric":`) {
		t.Fatalf("warning leaked invalid metric name: %s", logs.String())
	}
}

func TestStoreRecorderConcurrentRecordingAndShutdown(t *testing.T) {
	store := &fakeMetricsStore{}
	shutdown, err := InstallStore(store, WithFlushInterval(time.Hour), WithMaxSeries(10))
	if err != nil {
		t.Fatalf("InstallStore() error = %v", err)
	}
	start := make(chan struct{})
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			<-start
			for range 1_000 {
				AddCounter(context.Background(), "concurrent_total", 1, Label{Name: "kind", Value: "test"})
			}
		})
	}
	close(start)
	group.Wait()
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}
	batches := store.appendBatches()
	if len(batches) != 1 {
		t.Fatalf("append batches = %d, want 1", len(batches))
	}
	assertMetricSample(t, batches[0], "concurrent_total", map[string]string{"kind": "test"}, 8_000)
}

func TestStoreRecorderShutdownWhileRecording(t *testing.T) {
	store := &fakeMetricsStore{}
	shutdown, err := InstallStore(store, WithFlushInterval(time.Hour), WithMaxSeries(10))
	if err != nil {
		t.Fatalf("InstallStore() error = %v", err)
	}
	AddCounter(context.Background(), "shutdown_race_total", 1)

	stop := make(chan struct{})
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
					AddCounter(context.Background(), "shutdown_race_total", 1)
				}
			}
		})
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}
	close(stop)
	group.Wait()

	batches := store.appendBatches()
	if len(batches) != 1 {
		t.Fatalf("append batches = %d, want 1", len(batches))
	}
	for _, sample := range batches[0] {
		if sample.Name == "shutdown_race_total" && sample.Value >= 1 {
			return
		}
	}
	t.Fatalf("shutdown_race_total not found in %#v", batches[0])
}

func assertMetricSample(t *testing.T, samples []storemetrics.Sample, name string, labels map[string]string, value float64) {
	t.Helper()
	for _, sample := range samples {
		if sample.Name != name || !maps.Equal(sample.Labels, labels) {
			continue
		}
		if math.Abs(sample.Value-value) > 1e-9 {
			t.Fatalf("sample %s%v value = %v, want %v", name, labels, sample.Value, value)
		}
		if sample.Timestamp.IsZero() {
			t.Fatalf("sample %s%v timestamp is zero", name, labels)
		}
		return
	}
	t.Fatalf("sample %s%v not found in %#v", name, labels, samples)
}

func cloneSamples(samples []storemetrics.Sample) []storemetrics.Sample {
	return slices.Collect(func(yield func(storemetrics.Sample) bool) {
		for _, sample := range samples {
			sample.Labels = maps.Clone(sample.Labels)
			if !yield(sample) {
				return
			}
		}
	})
}
