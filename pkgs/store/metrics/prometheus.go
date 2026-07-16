package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

const (
	remoteWriteVersion = "0.1.0"
	maxErrorBodyBytes  = 4096
)

// PrometheusConfig configures a Prometheus remote-write/query backend.
type PrometheusConfig struct {
	RemoteWriteURL string       `yaml:"remote_write_url"`
	QueryURL       string       `yaml:"query_url"`
	BearerToken    string       `yaml:"bearer_token"`
	HTTPClient     *http.Client `yaml:"-"`
}

// PrometheusStore writes samples through remote write and reads them through
// the Prometheus HTTP query API.
type PrometheusStore struct {
	remoteWriteURL string
	queryURL       string
	bearerToken    string
	client         *http.Client
}

// NewPrometheusStore creates a Prometheus remote-write/query metrics store.
func NewPrometheusStore(cfg PrometheusConfig) (*PrometheusStore, error) {
	remoteWriteURL, err := parseRequiredURL("remote_write_url", cfg.RemoteWriteURL)
	if err != nil {
		return nil, err
	}
	queryURL, err := parseRequiredURL("query_url", cfg.QueryURL)
	if err != nil {
		return nil, err
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &PrometheusStore{
		remoteWriteURL: remoteWriteURL,
		queryURL:       strings.TrimRight(queryURL, "/"),
		bearerToken:    cfg.BearerToken,
		client:         client,
	}, nil
}

// Append writes samples through the Prometheus remote write protocol.
func (s *PrometheusStore) Append(ctx context.Context, samples []Sample) error {
	if len(samples) == 0 {
		return nil
	}
	req := &prompb.WriteRequest{
		Timeseries: make([]prompb.TimeSeries, 0, len(samples)),
	}
	for _, sample := range samples {
		if err := validateSample(sample); err != nil {
			return err
		}
		req.Timeseries = append(req.Timeseries, prompb.TimeSeries{
			Labels:  sampleLabels(sample),
			Samples: []prompb.Sample{{Timestamp: sample.Timestamp.UnixMilli(), Value: sample.Value}},
		})
	}
	body, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("metrics: marshal remote write request: %w", err)
	}
	compressed := snappy.Encode(nil, body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.remoteWriteURL, bytes.NewReader(compressed))
	if err != nil {
		return fmt.Errorf("metrics: create remote write request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("Content-Encoding", "snappy")
	httpReq.Header.Set("X-Prometheus-Remote-Write-Version", remoteWriteVersion)
	s.authorize(httpReq)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("metrics: remote write request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("metrics: remote write status %d: %s", resp.StatusCode, readLimitedBody(resp.Body))
	}
	return nil
}

// Latest returns the newest matching Prometheus sample in the lookback window.
func (s *PrometheusStore) Latest(ctx context.Context, q LatestQuery) (SeriesSet, error) {
	if err := validateLatestQuery(q); err != nil {
		return nil, err
	}
	memory, err := s.loadWindow(ctx, q.Selector, q.At.Add(-q.Lookback), q.At)
	if err != nil {
		return nil, err
	}
	return memory.Latest(ctx, q)
}

// Range evaluates the backend-neutral range semantics over Prometheus samples.
func (s *PrometheusStore) Range(ctx context.Context, q RangeQuery) (SeriesSet, error) {
	if err := validateRangeQuery(q); err != nil {
		return nil, err
	}
	memory, err := s.loadWindow(ctx, q.Selector, q.Start, q.End)
	if err != nil {
		return nil, err
	}
	return memory.Range(ctx, q)
}

// Aggregate evaluates bucketed aggregation over Prometheus samples.
func (s *PrometheusStore) Aggregate(ctx context.Context, q AggregateQuery) (SeriesSet, error) {
	if err := validateAggregateQuery(q); err != nil {
		return nil, err
	}
	memory, err := s.loadWindow(ctx, q.Selector, q.Start, q.End)
	if err != nil {
		return nil, err
	}
	return memory.Aggregate(ctx, q)
}

func (s *PrometheusStore) loadWindow(ctx context.Context, selector Selector, start, end time.Time) (*MemoryStore, error) {
	duration := end.Sub(start)
	duration = max(duration, time.Millisecond)
	expr, err := prometheusSelector(selector)
	if err != nil {
		return nil, err
	}
	series, err := s.query(ctx, "/api/v1/query", url.Values{"query": {expr + "[" + formatPrometheusRangeDuration(duration) + "]"}, "time": {formatPrometheusTime(end)}})
	if err != nil {
		return nil, err
	}
	// A range selector is left-open; fetch the boundary separately so the store contract can include Start.
	boundary, err := s.query(ctx, "/api/v1/query", url.Values{"query": {expr + "[1ms]"}, "time": {formatPrometheusTime(start)}})
	if err != nil {
		return nil, err
	}
	samples := samplesFromSeries(series)
	for _, item := range boundary {
		for _, point := range item.Points {
			if point.Timestamp.Equal(start.UTC()) {
				samples = append(samples, Sample{Name: item.Name, Labels: item.Labels, Timestamp: point.Timestamp, Value: point.Value})
			}
		}
	}
	memory := NewMemoryStore()
	if err := memory.Append(ctx, samples); err != nil {
		return nil, err
	}
	return memory, nil
}

func prometheusSelector(selector Selector) (string, error) {
	if err := validateSelector(selector); err != nil {
		return "", err
	}
	matchers := slices.Clone(selector.Matchers)
	slices.SortFunc(matchers, func(a, b LabelMatcher) int { return strings.Compare(a.Name, b.Name) })
	if len(matchers) == 0 {
		return selector.Name, nil
	}
	parts := make([]string, 0, len(matchers))
	for _, m := range matchers {
		parts = append(parts, m.Name+string(m.Op)+strconv.Quote(m.Value))
	}
	return selector.Name + "{" + strings.Join(parts, ",") + "}", nil
}

func formatPrometheusRangeDuration(d time.Duration) string {
	return strconv.FormatInt(d.Milliseconds(), 10) + "ms"
}
func samplesFromSeries(series SeriesSet) []Sample {
	out := []Sample{}
	for _, item := range series {
		for _, point := range item.Points {
			out = append(out, Sample{Name: item.Name, Labels: item.Labels, Timestamp: point.Timestamp, Value: point.Value})
		}
	}
	return out
}

// Close releases resources. The default HTTP client has no owned resources to
// close, so Close is currently a no-op.
func (s *PrometheusStore) Close() error {
	return nil
}

func (s *PrometheusStore) query(ctx context.Context, path string, values url.Values) (SeriesSet, error) {
	endpoint := s.queryURL + path + "?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("metrics: create query request: %w", err)
	}
	s.authorize(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("metrics: query request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("metrics: query status %d: %s", resp.StatusCode, readLimitedBody(resp.Body))
	}
	var decoded prometheusResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("metrics: decode query response: %w", err)
	}
	if decoded.Status != "success" {
		if decoded.Error != "" {
			return nil, fmt.Errorf("metrics: query failed: %s: %s", decoded.ErrorType, decoded.Error)
		}
		return nil, fmt.Errorf("metrics: query failed with status %q", decoded.Status)
	}
	return decoded.Data.series()
}

func (s *PrometheusStore) authorize(req *http.Request) {
	if s.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.bearerToken)
	}
}

func parseRequiredURL(field, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("metrics: prometheus %s is required", field)
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("metrics: prometheus %s: %w", field, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("metrics: prometheus %s must use http or https", field)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("metrics: prometheus %s requires host", field)
	}
	return parsed.String(), nil
}

func sampleLabels(sample Sample) []prompb.Label {
	labels := make([]prompb.Label, 0, len(sample.Labels)+1)
	labels = append(labels, prompb.Label{Name: "__name__", Value: sample.Name})
	names := make([]string, 0, len(sample.Labels))
	for name := range sample.Labels {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		labels = append(labels, prompb.Label{Name: name, Value: sample.Labels[name]})
	}
	return labels
}

func readLimitedBody(r io.Reader) string {
	data, err := io.ReadAll(io.LimitReader(r, maxErrorBodyBytes))
	if err != nil {
		return "read response body: " + err.Error()
	}
	return strings.TrimSpace(string(data))
}

func formatPrometheusTime(t time.Time) string {
	return strconv.FormatFloat(float64(t.UnixNano())/float64(time.Second), 'f', 3, 64)
}

type prometheusResponse struct {
	Status    string         `json:"status"`
	Data      prometheusData `json:"data"`
	ErrorType string         `json:"errorType"`
	Error     string         `json:"error"`
}

type prometheusData struct {
	ResultType string             `json:"resultType"`
	Result     []prometheusResult `json:"result"`
}

type prometheusResult struct {
	Metric map[string]string `json:"metric"`
	Value  prometheusPoint   `json:"value"`
	Values []prometheusPoint `json:"values"`
}

type prometheusPoint struct {
	Timestamp time.Time
	Value     float64
}

func (p *prometheusPoint) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw) != 2 {
		return fmt.Errorf("prometheus point has %d fields", len(raw))
	}
	var unixSeconds float64
	if err := json.Unmarshal(raw[0], &unixSeconds); err != nil {
		return fmt.Errorf("timestamp: %w", err)
	}
	var valueText string
	if err := json.Unmarshal(raw[1], &valueText); err != nil {
		return fmt.Errorf("value: %w", err)
	}
	value, err := parsePrometheusValue(valueText)
	if err != nil {
		return err
	}
	p.Timestamp = time.Unix(0, int64(unixSeconds*float64(time.Second))).UTC()
	p.Value = value
	return nil
}

func parsePrometheusValue(value string) (float64, error) {
	switch value {
	case "NaN":
		return math.NaN(), nil
	case "+Inf", "Inf":
		return math.Inf(1), nil
	case "-Inf":
		return math.Inf(-1), nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("metrics: parse query value %q: %w", value, err)
	}
	return parsed, nil
}

func (d prometheusData) series() (SeriesSet, error) {
	switch d.ResultType {
	case "vector":
		return seriesFromVector(d.Result)
	case "matrix":
		return seriesFromMatrix(d.Result)
	case "":
		return nil, errors.New("metrics: query response missing resultType")
	default:
		return nil, fmt.Errorf("metrics: unsupported query resultType %q", d.ResultType)
	}
}

func seriesFromVector(results []prometheusResult) (SeriesSet, error) {
	out := make(SeriesSet, 0, len(results))
	for _, result := range results {
		series := seriesFromMetric(result.Metric)
		if result.Value.Timestamp.IsZero() {
			return nil, fmt.Errorf("metrics: vector result for %q missing value", series.Name)
		}
		series.Points = []Point{{Timestamp: result.Value.Timestamp, Value: result.Value.Value}}
		out = append(out, series)
	}
	return out, nil
}

func seriesFromMatrix(results []prometheusResult) (SeriesSet, error) {
	out := make(SeriesSet, 0, len(results))
	for _, result := range results {
		series := seriesFromMetric(result.Metric)
		series.Points = make([]Point, 0, len(result.Values))
		for _, value := range result.Values {
			if value.Timestamp.IsZero() {
				return nil, fmt.Errorf("metrics: matrix result for %q has missing value", series.Name)
			}
			series.Points = append(series.Points, Point{Timestamp: value.Timestamp, Value: value.Value})
		}
		out = append(out, series)
	}
	return out, nil
}

func seriesFromMetric(metric map[string]string) Series {
	labels := make(map[string]string, len(metric))
	maps.Copy(labels, metric)
	name := labels["__name__"]
	delete(labels, "__name__")
	return Series{Name: name, Labels: labels}
}
