package metrics

import (
	"context"
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
