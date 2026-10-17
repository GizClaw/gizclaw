package gizmetrics

import (
	"context"
	"errors"
	"math"
	"sync/atomic"
	"testing"
	"time"

	storemetrics "github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

func TestCollectorReportsUnchangedCountersAndShutdownZeroWithNodeIdentity(t *testing.T) {
	store := &fakeMetricsStore{}
	var online atomic.Int64
	online.Store(2)
	shutdown, err := InstallStore(store, WithFlushInterval(time.Hour),
		WithLabels(Label{Name: "node_id", Value: "server-a"}),
		WithCollector(func(time.Time) []storemetrics.Sample {
			return []storemetrics.Sample{{Name: "devices_online", Value: float64(online.Load())}, {Name: "messages_total", Value: 7}}
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shutdown(t.Context()) })
	recorder := processRecorder.Load().recorder.(*storeRecorder)
	AddCounter(t.Context(), "requests_total", 3)
	// A caller cannot spoof the process identity.
	AddCounter(t.Context(), "spoofed_total", 1, Label{Name: "node_id", Value: "server-b"})
	for range 2 {
		if err := recorder.flush(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	online.Store(0)
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	batches := store.appendBatches()
	if len(batches) != 3 {
		t.Fatalf("batches = %d, want periodic, unchanged, and shutdown", len(batches))
	}
	labels := map[string]string{"node_id": "server-a"}
	for index, batch := range batches {
		want := 2.0
		if index == 2 {
			want = 0
		}
		assertMetricSample(t, batch, "devices_online", labels, want)
		assertMetricSample(t, batch, "messages_total", labels, 7)
		for _, sample := range batch {
			if sample.Name == "spoofed_total" || sample.Timestamp.IsZero() {
				t.Fatalf("invalid exported sample: %+v", sample)
			}
		}
	}
	assertMetricSample(t, batches[0], "requests_total", labels, 3)
}

func TestCollectorBoundsSeriesAndRetainsAbsoluteValueAfterStoreFailure(t *testing.T) {
	store := &fakeMetricsStore{appendErrs: []error{errors.New("offline")}}
	var value atomic.Int64
	value.Store(4)
	config := recorderConfig{flushInterval: time.Hour, appendTimeout: time.Second, maxSeries: 1}
	if err := WithCollector(func(time.Time) []storemetrics.Sample {
		return []storemetrics.Sample{
			{Name: "messages_total", Value: float64(value.Load())},
			{Name: "excess", Value: 1},
		}
	})(&config); err != nil {
		t.Fatal(err)
	}
	recorder := newStoreRecorder(store, config)
	if err := recorder.flush(t.Context()); err == nil {
		t.Fatal("expected store failure")
	}
	value.Store(9)
	if err := recorder.flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	batches := store.appendBatches()
	if len(batches) != 2 || len(batches[0]) != 1 || len(batches[1]) != 1 {
		t.Fatalf("unbounded or missing samples: %+v", batches)
	}
	assertMetricSample(t, batches[1], "messages_total", map[string]string{}, 9)
}

func TestCollectorRejectsInvalidSamplesAndOptions(t *testing.T) {
	for _, option := range []Option{WithCollector(nil), WithLabels(Label{Name: "bad-label"}), WithLabels(Label{Name: "le"})} {
		if _, err := InstallStore(&fakeMetricsStore{}, option); err == nil {
			t.Fatal("invalid collector option accepted")
		}
	}
	store := &fakeMetricsStore{}
	shutdown, err := InstallStore(store, WithFlushInterval(time.Hour), WithCollector(func(time.Time) []storemetrics.Sample {
		return []storemetrics.Sample{{Name: "bad name", Value: 1}, {Name: "bad_value", Value: math.NaN()}, {Name: "bad_label", Labels: map[string]string{"bad-label": "value"}, Value: 1}}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.appendBatches()) != 0 {
		t.Fatal("invalid collector data reached storage")
	}
}
