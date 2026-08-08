package metrics

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPrometheusLatestTranslatesSelectorPrivately(t *testing.T) {
	t.Parallel()
	queries := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		queries = append(queries, q)
		if !strings.Contains(q, `m{peer="a"}`) {
			t.Errorf("query=%q", q)
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(q, "[1ms]") {
			w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
			return
		}
		if strings.Contains(q, "timestamp(") {
			w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"__name__":"m","peer":"a"},"values":[[100,"99"]]}]}}`))
			return
		}
		w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"__name__":"m","peer":"a"},"values":[[100,"2"]]}]}}`))
	}))
	defer server.Close()
	s, err := NewPrometheusStore(PrometheusConfig{RemoteWriteURL: server.URL, QueryURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Latest(context.Background(), LatestQuery{Selector: Selector{Name: "m", Matchers: []LabelMatcher{{Name: "peer", Op: MatchEqual, Value: "a"}}}, At: time.Unix(101, 0), Lookback: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 2 || !strings.HasPrefix(queries[0], "last_over_time(") || !strings.Contains(queries[1], "timestamp(") || len(got) != 1 || got[0].Points[0].Value != 2 {
		t.Fatalf("queries=%q got=%+v", queries, got)
	}
	if !got[0].Points[0].Timestamp.Equal(time.Unix(99, 0)) || !strings.Contains(queries[0], "[2001ms]") {
		t.Fatalf("lookback boundary was not preserved: queries=%q got=%+v", queries, got)
	}
}

func TestPrometheusRangeAndAggregateAvoidFullWindowMaterialization(t *testing.T) {
	t.Parallel()
	queries := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query().Get("query"))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))
	defer server.Close()
	store, err := NewPrometheusStore(PrometheusConfig{RemoteWriteURL: server.URL, QueryURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Unix(100, 0).UTC()
	selector := Selector{Name: "m"}
	if _, err := store.Range(context.Background(), RangeQuery{Selector: selector, Start: start, End: start.Add(10 * time.Minute), Step: time.Minute}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Aggregate(context.Background(), AggregateQuery{Selector: selector, Start: start, End: start.Add(10 * time.Minute), Bucket: time.Minute, Operation: AggregationAvg}); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(queries, "\n")
	if !strings.Contains(joined, "last_over_time(m[60000ms])") || !strings.Contains(joined, "avg_over_time(m[60000ms])") {
		t.Fatalf("queries do not use PromQL windowing:\n%s", joined)
	}
	if strings.Contains(joined, "m[600000ms]") {
		t.Fatalf("queries materialize the full range:\n%s", joined)
	}
}

func TestPrometheusConfigValidation(t *testing.T) {
	t.Parallel()
	if _, err := NewPrometheusStore(PrometheusConfig{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestPrometheusConnectorReturnsIndependentBorrowedStores(t *testing.T) {
	connector, err := NewPrometheusConnector(PrometheusConfig{
		RemoteWriteURL: "https://prometheus.example.test/api/v1/write",
		QueryURL:       "https://prometheus.example.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := connector.Store()
	if err != nil {
		t.Fatal(err)
	}
	second, err := connector.Store()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || first.client != second.client {
		t.Fatalf("stores do not borrow one connector client: %p, %p", first, second)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := connector.Store(); err != nil {
		t.Fatalf("closing one logical Store invalidated connector: %v", err)
	}
	if err := connector.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPrometheusProviderErrorsDoNotExposeResponseBodies(t *testing.T) {
	const secret = "response-contains-bearer-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, secret, http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)
	store, err := NewPrometheusStore(PrometheusConfig{RemoteWriteURL: server.URL, QueryURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	sample := Sample{Name: "requests", Timestamp: time.Now().UTC(), Value: 1}
	if err := store.Append(context.Background(), []Sample{sample}); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("Append() error = %v", err)
	}
	_, err = store.Latest(context.Background(), LatestQuery{
		Selector: Selector{Name: "requests"}, At: time.Now().UTC(), Lookback: time.Minute,
	})
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("Latest() error = %v", err)
	}
}

func TestPrometheusTransportErrorsRemainInspectableWithoutExposingSecrets(t *testing.T) {
	providerErr := errors.New("transport contains bearer-secret")
	store, err := NewPrometheusStore(PrometheusConfig{
		RemoteWriteURL: "https://prometheus.example.test/write",
		QueryURL:       "https://prometheus.example.test",
		HTTPClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, providerErr
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = store.Append(context.Background(), []Sample{{Name: "requests", Timestamp: time.Now().UTC(), Value: 1}})
	if !errors.Is(err, providerErr) || strings.Contains(err.Error(), "bearer-secret") {
		t.Fatalf("Append() error = %v", err)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
