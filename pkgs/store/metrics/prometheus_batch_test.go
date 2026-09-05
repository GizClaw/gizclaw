package metrics

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

func waitPendingAppends(t *testing.T, store *PrometheusStore, count int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		store.batchMu.Lock()
		got := len(store.pending)
		store.batchMu.Unlock()
		if got == count {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pending appends = %d, want %d", got, count)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestPrometheusAppendBatchesAndWaitsForAcceptance(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	defer close(release)
	var requests atomic.Int32
	var sampleCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encoded, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		decoded, err := snappy.Decode(nil, encoded)
		if err != nil {
			t.Error(err)
			return
		}
		var request prompb.WriteRequest
		if err := proto.Unmarshal(decoded, &request); err != nil {
			t.Error(err)
			return
		}
		if len(request.Timeseries) != 1 {
			t.Errorf("series = %d, want 1", len(request.Timeseries))
		}
		for _, series := range request.Timeseries {
			sampleCount.Add(int32(len(series.Samples)))
			for i := 1; i < len(series.Samples); i++ {
				if series.Samples[i-1].Timestamp > series.Samples[i].Timestamp {
					t.Error("unsorted samples")
				}
			}
		}
		requests.Add(1)
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	store, err := NewPrometheusStore(PrometheusConfig{RemoteWriteURL: server.URL, QueryURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	results := make(chan error, 33)
	go func() { results <- store.Append(ctx, []Sample{{Name: "value", Timestamp: time.Unix(1, 0)}}) }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	for i := range 32 {
		go func() { results <- store.Append(ctx, []Sample{{Name: "value", Timestamp: time.Unix(int64(40-i), 0)}}) }()
	}
	waitPendingAppends(t, store, 32)
	select {
	case err := <-results:
		t.Fatalf("Append returned before acceptance: %v", err)
	default:
	}
	release <- struct{}{}
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	release <- struct{}{}
	for range 33 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if requests.Load() != 2 || sampleCount.Load() != 33 {
		t.Fatalf("requests=%d samples=%d", requests.Load(), sampleCount.Load())
	}
}

func TestPrometheusAppendCancellationDoesNotCancelPeers(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	defer close(release)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	store, err := NewPrometheusStore(PrometheusConfig{RemoteWriteURL: server.URL, QueryURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sample := []Sample{{Name: "v", Timestamp: time.Now()}}
	first := make(chan error, 1)
	go func() { first <- store.Append(ctx, sample) }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	canceled, cancelCaller := context.WithCancel(ctx)
	canceledResult := make(chan error, 1)
	healthy := make(chan error, 1)
	go func() { canceledResult <- store.Append(canceled, sample) }()
	go func() { healthy <- store.Append(ctx, sample) }()
	waitPendingAppends(t, store, 2)
	release <- struct{}{}
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	cancelCaller()
	if err := <-canceledResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}
	select {
	case err := <-healthy:
		t.Fatalf("peer returned early: %v", err)
	default:
	}
	release <- struct{}{}
	if err := <-healthy; err != nil {
		t.Fatal(err)
	}
	if err := <-first; err != nil {
		t.Fatal(err)
	}
}

func TestPrometheusCloseCancelsAndJoinsWriter(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(entered)
		<-r.Context().Done()
	}))
	defer server.Close()
	store, err := NewPrometheusStore(PrometheusConfig{RemoteWriteURL: server.URL, QueryURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := make(chan error, 1)
	sample := []Sample{{Name: "v", Timestamp: time.Now()}}
	go func() { result <- store.Append(ctx, sample) }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, errPrometheusClosed) {
		t.Fatalf("close error=%v", err)
	}
	if err := store.Append(ctx, sample); !errors.Is(err, errPrometheusClosed) {
		t.Fatalf("append after close=%v", err)
	}
}

func TestPrometheusAppendBackpressureRespectsDeadline(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(entered)
		select {
		case <-release:
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	store, err := NewPrometheusStore(PrometheusConfig{RemoteWriteURL: server.URL, QueryURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	first := make(chan error, 1)
	samples := make([]Sample, prometheusPendingSamples)
	for i := range samples {
		samples[i] = Sample{Name: "value", Timestamp: time.UnixMilli(int64(i) + 1)}
	}
	go func() { first <- store.Append(ctx, samples) }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	waiting, stop := context.WithTimeout(ctx, 20*time.Millisecond)
	defer stop()
	err = store.Append(waiting, []Sample{{Name: "value", Timestamp: time.Now()}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("admission error=%v", err)
	}
	store.batchMu.Lock()
	queued := len(store.pending)
	store.batchMu.Unlock()
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if queued != 0 {
		t.Fatalf("backpressured call was admitted: %d", queued)
	}
}

func TestPrometheusAppendSharedFailureIsReturnedToEveryCaller(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{}, 2)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		calls.Add(1)
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	store, err := NewPrometheusStore(PrometheusConfig{RemoteWriteURL: server.URL, QueryURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := make(chan error, 3)
	appendOne := func() { result <- store.Append(ctx, []Sample{{Name: "value", Timestamp: time.Now()}}) }
	go appendOne()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	go appendOne()
	go appendOne()
	waitPendingAppends(t, store, 2)
	release <- struct{}{}
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	release <- struct{}{}
	for range 3 {
		if err := <-result; err == nil {
			t.Fatal("backend failure reported success")
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("request calls=%d", calls.Load())
	}
}

func TestPrometheusZeroStoreClose(t *testing.T) {
	var store PrometheusStore
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(context.Background(), []Sample{{Name: "value", Timestamp: time.Now()}}); !errors.Is(err, errPrometheusClosed) {
		t.Fatalf("append after close=%v", err)
	}
}
