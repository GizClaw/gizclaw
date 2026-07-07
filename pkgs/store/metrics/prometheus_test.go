package metrics

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

func TestPrometheusStoreAppendWritesRemoteWriteRequest(t *testing.T) {
	var got prompb.WriteRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/write" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-protobuf" {
			t.Fatalf("Content-Type = %q", got)
		}
		if got := r.Header.Get("Content-Encoding"); got != "snappy" {
			t.Fatalf("Content-Encoding = %q", got)
		}
		if got := r.Header.Get("X-Prometheus-Remote-Write-Version"); got != remoteWriteVersion {
			t.Fatalf("remote write version = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		decoded, err := snappy.Decode(nil, body)
		if err != nil {
			t.Fatalf("snappy decode: %v", err)
		}
		if err := proto.Unmarshal(decoded, &got); err != nil {
			t.Fatalf("proto unmarshal: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	store, err := NewPrometheusStore(PrometheusConfig{
		RemoteWriteURL: server.URL + "/api/v1/write",
		QueryURL:       server.URL,
		BearerToken:    "test-token",
		HTTPClient:     server.Client(),
	})
	if err != nil {
		t.Fatalf("NewPrometheusStore: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	ts := time.Unix(123, 456*int64(time.Millisecond)).UTC()
	if err := store.Append(context.Background(), []Sample{{
		Name:      "gizclaw_peer_battery_percent",
		Labels:    map[string]string{"peer_id": "p1", "kind": "battery"},
		Timestamp: ts,
		Value:     82,
	}}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	if len(got.Timeseries) != 1 {
		t.Fatalf("timeseries count = %d", len(got.Timeseries))
	}
	series := got.Timeseries[0]
	wantLabels := []prompb.Label{
		{Name: "__name__", Value: "gizclaw_peer_battery_percent"},
		{Name: "kind", Value: "battery"},
		{Name: "peer_id", Value: "p1"},
	}
	if !proto.Equal(&prompb.TimeSeries{Labels: wantLabels, Samples: series.Samples}, &series) {
		t.Fatalf("labels = %+v, want %+v", series.Labels, wantLabels)
	}
	if len(series.Samples) != 1 {
		t.Fatalf("sample count = %d", len(series.Samples))
	}
	if series.Samples[0].Timestamp != ts.UnixMilli() || series.Samples[0].Value != 82 {
		t.Fatalf("sample = %+v", series.Samples[0])
	}
}

func TestPrometheusStoreAppendValidationAndErrors(t *testing.T) {
	t.Run("empty batch", func(t *testing.T) {
		store := mustPrometheusStore(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("empty append should not issue request")
		})))
		if err := store.Append(context.Background(), nil); err != nil {
			t.Fatalf("Append nil: %v", err)
		}
	})

	t.Run("invalid sample", func(t *testing.T) {
		store := mustPrometheusStore(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
		err := store.Append(context.Background(), []Sample{{Name: "bad-name", Timestamp: time.Now()}})
		if err == nil || !strings.Contains(err.Error(), "invalid metric name") {
			t.Fatalf("Append error = %v", err)
		}
	})

	t.Run("http status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "rejected", http.StatusBadGateway)
		}))
		defer server.Close()
		store := mustPrometheusStore(t, server)
		err := store.Append(context.Background(), []Sample{{Name: "ok_metric", Timestamp: time.Now()}})
		if err == nil || !strings.Contains(err.Error(), "remote write status 502") {
			t.Fatalf("Append error = %v", err)
		}
	})
}

func TestPrometheusStoreQueryVector(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("query"); got != `gizclaw_peer_battery_percent{peer_id="p1"}` {
			t.Fatalf("query = %q", got)
		}
		if got := r.URL.Query().Get("time"); got != "123.456" {
			t.Fatalf("time = %q", got)
		}
		writeJSON(t, w, map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "vector",
				"result": []any{map[string]any{
					"metric": map[string]string{"__name__": "gizclaw_peer_battery_percent", "peer_id": "p1"},
					"value":  []any{123.456, "82"},
				}},
			},
		})
	}))
	defer server.Close()

	store := mustPrometheusStore(t, server)
	got, err := store.Query(context.Background(), Query{
		Expression: `gizclaw_peer_battery_percent{peer_id="p1"}`,
		Time:       time.Unix(123, 456*int64(time.Millisecond)).UTC(),
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("series count = %d", len(got))
	}
	if got[0].Name != "gizclaw_peer_battery_percent" || got[0].Labels["peer_id"] != "p1" {
		t.Fatalf("series = %+v", got[0])
	}
	if len(got[0].Points) != 1 || got[0].Points[0].Value != 82 {
		t.Fatalf("points = %+v", got[0].Points)
	}
}

func TestPrometheusStoreQueryRangeMatrix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query_range" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("start"); got != "10.000" {
			t.Fatalf("start = %q", got)
		}
		if got := r.URL.Query().Get("end"); got != "40.000" {
			t.Fatalf("end = %q", got)
		}
		if got := r.URL.Query().Get("step"); got != "15" {
			t.Fatalf("step = %q", got)
		}
		writeJSON(t, w, map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "matrix",
				"result": []any{map[string]any{
					"metric": map[string]string{"__name__": "gizclaw_peer_gnss_latitude", "peer_id": "p1"},
					"values": []any{
						[]any{10.0, "31.2"},
						[]any{25.0, "31.3"},
					},
				}},
			},
		})
	}))
	defer server.Close()

	store := mustPrometheusStore(t, server)
	got, err := store.QueryRange(context.Background(), RangeQuery{
		Expression: "gizclaw_peer_gnss_latitude",
		Start:      time.Unix(10, 0).UTC(),
		End:        time.Unix(40, 0).UTC(),
		Step:       15 * time.Second,
	})
	if err != nil {
		t.Fatalf("QueryRange: %v", err)
	}
	if len(got) != 1 || len(got[0].Points) != 2 {
		t.Fatalf("result = %+v", got)
	}
	if got[0].Points[0].Value != 31.2 || got[0].Points[1].Value != 31.3 {
		t.Fatalf("points = %+v", got[0].Points)
	}
}

func TestPrometheusStoreQueryErrors(t *testing.T) {
	t.Run("empty expression", func(t *testing.T) {
		store := mustPrometheusStore(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
		if _, err := store.Query(context.Background(), Query{}); err == nil {
			t.Fatal("expected empty expression error")
		}
	})

	t.Run("bad range", func(t *testing.T) {
		store := mustPrometheusStore(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
		_, err := store.QueryRange(context.Background(), RangeQuery{
			Expression: "metric",
			Start:      time.Unix(2, 0),
			End:        time.Unix(1, 0),
			Step:       time.Second,
		})
		if err == nil || !strings.Contains(err.Error(), "end is before start") {
			t.Fatalf("QueryRange error = %v", err)
		}
	})

	t.Run("prometheus error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, map[string]any{"status": "error", "errorType": "bad_data", "error": "parse failed"})
		}))
		defer server.Close()
		store := mustPrometheusStore(t, server)
		_, err := store.Query(context.Background(), Query{Expression: "metric"})
		if err == nil || !strings.Contains(err.Error(), "bad_data: parse failed") {
			t.Fatalf("Query error = %v", err)
		}
	})
}

func TestNewPrometheusStoreRequiresEndpoints(t *testing.T) {
	tests := []PrometheusConfig{
		{QueryURL: "http://example.test"},
		{RemoteWriteURL: "http://example.test/api/v1/write"},
		{RemoteWriteURL: "file:///tmp/write", QueryURL: "http://example.test"},
		{RemoteWriteURL: "http://example.test/api/v1/write", QueryURL: "://bad"},
	}
	for _, cfg := range tests {
		if _, err := NewPrometheusStore(cfg); err == nil {
			t.Fatalf("NewPrometheusStore(%+v) expected error", cfg)
		}
	}
}

func mustPrometheusStore(t *testing.T, server *httptest.Server) *PrometheusStore {
	t.Helper()
	t.Cleanup(server.Close)
	store, err := NewPrometheusStore(PrometheusConfig{
		RemoteWriteURL: server.URL + "/api/v1/write",
		QueryURL:       server.URL,
		HTTPClient:     server.Client(),
	})
	if err != nil {
		t.Fatalf("NewPrometheusStore: %v", err)
	}
	return store
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode json: %v", err)
	}
}
