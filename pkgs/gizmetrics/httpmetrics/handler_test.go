package httpmetrics

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizmetrics"
	storemetrics "github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

type captureStore struct {
	mu      sync.Mutex
	samples []storemetrics.Sample
}

func (s *captureStore) Append(_ context.Context, samples []storemetrics.Sample) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sample := range samples {
		sample.Labels = maps.Clone(sample.Labels)
		s.samples = append(s.samples, sample)
	}
	return nil
}

func (s *captureStore) Latest(context.Context, storemetrics.LatestQuery) (storemetrics.SeriesSet, error) {
	return nil, nil
}

func (s *captureStore) Range(context.Context, storemetrics.RangeQuery) (storemetrics.SeriesSet, error) {
	return nil, nil
}

func (s *captureStore) Aggregate(context.Context, storemetrics.AggregateQuery) (storemetrics.SeriesSet, error) {
	return nil, nil
}

func (s *captureStore) Close() error { return nil }

func (s *captureStore) snapshot() []storemetrics.Sample {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]storemetrics.Sample, len(s.samples))
	for index, sample := range s.samples {
		sample.Labels = maps.Clone(sample.Labels)
		out[index] = sample
	}
	return out
}

func TestWrapRecordsBoundedRequestMetrics(t *testing.T) {
	store, shutdown := installCaptureStore(t)
	handler := Wrap(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/client-error":
			http.Error(writer, "bad", http.StatusBadRequest)
		case "/server-error":
			http.Error(writer, "boom", http.StatusServiceUnavailable)
		case "/canceled":
			writer.WriteHeader(http.StatusNoContent)
		default:
			_, _ = io.WriteString(writer, "ok")
		}
	}), "peer-http", func(request *http.Request) (string, bool) {
		return strings.TrimPrefix(request.URL.Path, "/"), true
	})

	for _, path := range []string{"/success", "/client-error", "/server-error"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	canceled := httptest.NewRequest(http.MethodGet, "/canceled", nil).WithContext(canceledCtx)
	handler.ServeHTTP(httptest.NewRecorder(), canceled)
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}

	samples := store.snapshot()
	assertSampleValue(t, samples, RequestsTotalMetric, requestLabels("success", "2xx", "success"), 1)
	assertSampleValue(t, samples, RequestsTotalMetric, requestLabels("client-error", "4xx", "client_error"), 1)
	assertSampleValue(t, samples, RequestsTotalMetric, requestLabels("server-error", "5xx", "server_error"), 1)
	assertSampleValue(t, samples, RequestsTotalMetric, requestLabels("canceled", "2xx", "canceled"), 1)
	assertSampleValue(t, samples, ResponseBytesMetric, requestLabels("success", "2xx", "success"), 2)
	assertSampleValue(t, samples, RequestsInFlightMetric, map[string]string{
		"surface": "peer-http", "operation": "success", "method": http.MethodGet,
	}, 0)
	if duration := sampleValue(t, samples, RequestDurationMetric+"_sum", requestLabels("success", "2xx", "success")); duration < 0 {
		t.Fatalf("request duration = %v, want non-negative", duration)
	}
}

func TestWrapRecordsAndRethrowsPanic(t *testing.T) {
	store, shutdown := installCaptureStore(t)
	handler := Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("secret panic value")
	}), "admin-http", func(*http.Request) (string, bool) { return "admin.test", true })

	func() {
		defer func() {
			if recovered := recover(); recovered != "secret panic value" {
				t.Fatalf("recovered panic = %#v", recovered)
			}
		}()
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/ignored", nil))
	}()
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}
	labels := map[string]string{
		"surface": "admin-http", "operation": "admin.test", "method": http.MethodPost,
		"status_class": "unknown", "result": "panic",
	}
	assertSampleValue(t, store.snapshot(), RequestsTotalMetric, labels, 1)
	for _, sample := range store.snapshot() {
		for _, value := range sample.Labels {
			if strings.Contains(value, "secret") {
				t.Fatalf("panic value leaked into labels: %#v", sample)
			}
		}
	}
}

func TestWrapClassifiesResponseWriterFailuresAsTransportErrors(t *testing.T) {
	store, shutdown := installCaptureStore(t)
	handler := Wrap(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/write":
			_, _ = writer.Write([]byte("response"))
		case "/read-from":
			_, _ = io.Copy(writer, io.LimitReader(strings.NewReader("response"), 8))
		}
	}), "peer-http", func(request *http.Request) (string, bool) {
		return strings.TrimPrefix(request.URL.Path, "/"), true
	})

	writeErr := errors.New("secret writer failure")
	for _, path := range []string{"/write", "/read-from"} {
		writer := &failingResponseWriter{header: make(http.Header), err: writeErr}
		handler.ServeHTTP(writer, httptest.NewRequest(http.MethodGet, path, nil))
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}

	for _, operation := range []string{"write", "read-from"} {
		labels := requestLabels(operation, "2xx", "transport_error")
		assertSampleValue(t, store.snapshot(), RequestsTotalMetric, labels, 1)
		assertSampleValue(t, store.snapshot(), ResponseBytesMetric, labels, 0)
	}
	for _, sample := range store.snapshot() {
		for _, value := range sample.Labels {
			if strings.Contains(value, writeErr.Error()) {
				t.Fatalf("write error leaked into labels: %#v", sample)
			}
		}
	}
}

func TestWrapNeverFallsBackToRawRequestIdentity(t *testing.T) {
	store, shutdown := installCaptureStore(t)
	handler := Wrap(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusAccepted)
	}), "bad/surface", func(request *http.Request) (string, bool) {
		return request.URL.Path, true
	})
	request := httptest.NewRequest(http.MethodGet, "/workspace/private-name?token=secret", nil)
	request.Header.Set("X-Request-ID", "request-secret")
	request.Header.Set("X-Public-Key", "peer-secret")
	handler.ServeHTTP(httptest.NewRecorder(), request)
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}
	for _, sample := range store.snapshot() {
		for name, value := range sample.Labels {
			if strings.Contains(value, "private") || strings.Contains(value, "secret") {
				t.Fatalf("unbounded identity leaked through %s: %#v", name, sample)
			}
		}
	}
	assertSampleValue(t, store.snapshot(), RequestsTotalMetric, map[string]string{
		"surface": "unknown", "operation": "unknown", "method": http.MethodGet,
		"status_class": "2xx", "result": "success",
	}, 1)
}

func TestWrapMapsNonstandardMethodToOther(t *testing.T) {
	store, shutdown := installCaptureStore(t)
	handler := Wrap(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}), "peer-http", func(*http.Request) (string, bool) { return "method.test", true })
	request := httptest.NewRequest("BREW", "/", nil)
	handler.ServeHTTP(httptest.NewRecorder(), request)
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}
	assertSampleValue(t, store.snapshot(), RequestsTotalMetric, map[string]string{
		"surface": "peer-http", "operation": "method.test", "method": "OTHER",
		"status_class": "2xx", "result": "success",
	}, 1)
}

func TestBoundedMethodAllowsOnlyStandardMethods(t *testing.T) {
	for _, method := range []string{
		http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodOptions, http.MethodConnect, http.MethodTrace,
	} {
		if got := boundedMethod(method); got != method {
			t.Errorf("boundedMethod(%q) = %q", method, got)
		}
	}
	if got := boundedMethod("BREW"); got != "OTHER" {
		t.Fatalf("boundedMethod(BREW) = %q", got)
	}
}

func TestWrapAggregatesInflightAcrossWrapperInstances(t *testing.T) {
	store := &captureStore{}
	shutdown, err := gizmetrics.InstallStore(store, gizmetrics.WithFlushInterval(5*time.Millisecond), gizmetrics.WithMaxSeries(100))
	if err != nil {
		t.Fatalf("InstallStore() error = %v", err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	next := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		entered <- struct{}{}
		<-release
		writer.WriteHeader(http.StatusNoContent)
	})
	resolve := func(*http.Request) (string, bool) { return "shared.operation", true }
	first := Wrap(next, "peer-http", resolve)
	second := Wrap(next, "peer-http", resolve)

	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		first.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/first", nil))
	}()
	go func() {
		defer group.Done()
		second.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/second", nil))
	}()
	<-entered
	<-entered

	labels := map[string]string{"surface": "peer-http", "operation": "shared.operation", "method": http.MethodGet}
	key := inflightKey{surface: "peer-http", operation: "shared.operation", method: http.MethodGet}
	inflightMu.Lock()
	current := processInflight[key]
	inflightMu.Unlock()
	if current != 2 {
		close(release)
		group.Wait()
		t.Fatalf("process in-flight count = %d, want 2", current)
	}
	deadline := time.Now().Add(time.Second)
	for !hasSampleValue(store.snapshot(), RequestsInFlightMetric, labels, 2) {
		if time.Now().After(deadline) {
			close(release)
			group.Wait()
			t.Fatalf("in-flight samples never reached 2: %#v", store.snapshot())
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(release)
	group.Wait()
	inflightMu.Lock()
	remainingKeys := len(processInflight)
	inflightMu.Unlock()
	if remainingKeys != 0 {
		t.Fatalf("in-flight key count = %d, want 0", remainingKeys)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}
}

func TestWrapPreservesOptionalResponseWriterInterfacesAndStreamingDuration(t *testing.T) {
	store, shutdown := installCaptureStore(t)
	all := newAllResponseWriter()
	handler := Wrap(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		for name, ok := range map[string]bool{
			"flusher":       implements[http.Flusher](writer),
			"hijacker":      implements[http.Hijacker](writer),
			"readerFrom":    implements[io.ReaderFrom](writer),
			"pusher":        implements[http.Pusher](writer),
			"closeNotifier": implements[http.CloseNotifier](writer),
		} {
			if !ok {
				t.Errorf("wrapped writer lost %s", name)
			}
		}
		_, _ = io.Copy(writer, strings.NewReader("streamed"))
		writer.(http.Flusher).Flush()
		time.Sleep(20 * time.Millisecond)
	}), "server-public", func(*http.Request) (string, bool) { return "stream", true })
	handler.ServeHTTP(all, httptest.NewRequest(http.MethodGet, "/stream", nil))

	minimal := &minimalResponseWriter{header: make(http.Header)}
	Wrap(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if implements[http.Flusher](writer) || implements[http.Hijacker](writer) || implements[io.ReaderFrom](writer) || implements[http.Pusher](writer) {
			t.Error("wrapped minimal writer gained an optional interface")
		}
	}), "server-public", nil).ServeHTTP(minimal, httptest.NewRequest(http.MethodGet, "/", nil))

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}
	labels := requestLabels("stream", "2xx", "success")
	labels["surface"] = "server-public"
	assertSampleValue(t, store.snapshot(), ResponseBytesMetric, labels, 8)
	if duration := sampleValue(t, store.snapshot(), RequestDurationMetric+"_sum", labels); duration < 0.015 {
		t.Fatalf("stream duration = %v, want at least 15ms", duration)
	}
}

func installCaptureStore(t *testing.T) (*captureStore, func(context.Context) error) {
	t.Helper()
	store := &captureStore{}
	shutdown, err := gizmetrics.InstallStore(store, gizmetrics.WithFlushInterval(time.Hour), gizmetrics.WithMaxSeries(100))
	if err != nil {
		t.Fatalf("InstallStore() error = %v", err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })
	return store, shutdown
}

func requestLabels(operation string, statusClass string, result string) map[string]string {
	return map[string]string{
		"surface": "peer-http", "operation": operation, "method": http.MethodGet,
		"status_class": statusClass, "result": result,
	}
}

func assertSampleValue(t *testing.T, samples []storemetrics.Sample, name string, labels map[string]string, want float64) {
	t.Helper()
	if got := sampleValue(t, samples, name, labels); got != want {
		t.Fatalf("sample %s%v = %v, want %v", name, labels, got, want)
	}
}

func sampleValue(t *testing.T, samples []storemetrics.Sample, name string, labels map[string]string) float64 {
	t.Helper()
	for _, sample := range samples {
		if sample.Name == name && maps.Equal(sample.Labels, labels) {
			return sample.Value
		}
	}
	t.Fatalf("sample %s%v not found in %#v", name, labels, samples)
	return 0
}

func hasSampleValue(samples []storemetrics.Sample, name string, labels map[string]string, want float64) bool {
	for _, sample := range samples {
		if sample.Name == name && maps.Equal(sample.Labels, labels) && sample.Value == want {
			return true
		}
	}
	return false
}

func implements[T any](value any) bool {
	_, ok := value.(T)
	return ok
}

type minimalResponseWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

type failingResponseWriter struct {
	header http.Header
	err    error
}

func (w *failingResponseWriter) Header() http.Header { return w.header }

func (w *failingResponseWriter) Write([]byte) (int, error) { return 0, w.err }

func (*failingResponseWriter) WriteHeader(int) {}

func (w *failingResponseWriter) ReadFrom(io.Reader) (int64, error) { return 0, w.err }

func (w *minimalResponseWriter) Header() http.Header { return w.header }

func (w *minimalResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(body)
}

func (w *minimalResponseWriter) WriteHeader(status int) { w.status = status }

type allResponseWriter struct {
	*minimalResponseWriter
	flushed bool
}

func newAllResponseWriter() *allResponseWriter {
	return &allResponseWriter{minimalResponseWriter: &minimalResponseWriter{header: make(http.Header)}}
}

func (w *allResponseWriter) Flush() { w.flushed = true }

func (w *allResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errors.New("not connected")
}

func (w *allResponseWriter) ReadFrom(reader io.Reader) (int64, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.ReadFrom(reader)
}

func (w *allResponseWriter) Push(string, *http.PushOptions) error { return nil }

func (w *allResponseWriter) CloseNotify() <-chan bool {
	return make(chan bool)
}
