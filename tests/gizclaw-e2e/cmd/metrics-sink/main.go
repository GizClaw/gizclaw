// Command metrics-sink is a queryable Prometheus Remote Write fixture for
// Docker E2E observability acceptance.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"sync"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

type sample struct {
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels"`
	Timestamp int64             `json:"timestamp_ms"`
	Value     float64           `json:"value"`
}

type sink struct {
	mu      sync.Mutex
	samples []sample
}

func (s *sink) write(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("Content-Encoding") != "snappy" || r.Header.Get("Content-Type") != "application/x-protobuf" {
		http.Error(w, "invalid remote write headers", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<20))
	if err != nil {
		http.Error(w, "read request", http.StatusBadRequest)
		return
	}
	decoded, err := snappy.Decode(nil, body)
	if err != nil {
		http.Error(w, "decode snappy", http.StatusBadRequest)
		return
	}
	var request prompb.WriteRequest
	if err := proto.Unmarshal(decoded, &request); err != nil {
		http.Error(w, "decode protobuf", http.StatusBadRequest)
		return
	}

	received := make([]sample, 0, len(request.Timeseries))
	for _, series := range request.Timeseries {
		labels := make(map[string]string, len(series.Labels))
		name := ""
		for _, label := range series.Labels {
			if label.Name == "__name__" {
				name = label.Value
				continue
			}
			labels[label.Name] = label.Value
		}
		if name == "" {
			http.Error(w, "metric name is missing", http.StatusBadRequest)
			return
		}
		for _, point := range series.Samples {
			received = append(received, sample{Name: name, Labels: labels, Timestamp: point.Timestamp, Value: point.Value})
		}
	}
	s.mu.Lock()
	s.samples = append(s.samples, received...)
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *sink) dump(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	items := slices.Clone(s.samples)
	s.mu.Unlock()
	slices.SortFunc(items, func(a, b sample) int {
		if a.Name != b.Name {
			return compare(a.Name, b.Name)
		}
		return compare(fmt.Sprint(a.Labels), fmt.Sprint(b.Labels))
	})
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(items); err != nil {
		log.Printf("encode dump: %v", err)
	}
}

func compare(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func main() {
	receiver := &sink{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/write", receiver.write)
	mux.HandleFunc("GET /dump", receiver.dump)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	log.Fatal(http.ListenAndServe(":9090", mux))
}
