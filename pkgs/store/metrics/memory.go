package metrics

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MemoryStore keeps metric samples in process memory.
type MemoryStore struct {
	mu     sync.RWMutex
	series map[string]*memorySeries
	closed bool
}

type memorySeries struct {
	name   string
	labels map[string]string
	points []Point
}

type memorySelector struct {
	name     string
	matchers []memoryMatcher
}

type memoryMatcher struct {
	name  string
	op    MatchOp
	value string
	re    *regexp.Regexp
}

// NewMemoryStore creates an in-process metrics store for tests and embedded
// single-process integrations.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{series: make(map[string]*memorySeries)}
}

func (s *MemoryStore) Append(ctx context.Context, samples []Sample) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(samples) == 0 {
		return nil
	}
	if s == nil {
		return fmt.Errorf("metrics: memory store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("metrics: memory store is closed")
	}
	for _, sample := range samples {
		if err := validateSample(sample); err != nil {
			return err
		}
		labels := cloneLabels(sample.Labels)
		key := memorySeriesKey(sample.Name, labels)
		series := s.series[key]
		if series == nil {
			series = &memorySeries{name: sample.Name, labels: labels}
			s.series[key] = series
		}
		series.points = append(series.points, Point{Timestamp: sample.Timestamp.UTC(), Value: sample.Value})
		slices.SortFunc(series.points, func(a, b Point) int {
			return a.Timestamp.Compare(b.Timestamp)
		})
	}
	return nil
}

func (s *MemoryStore) Query(ctx context.Context, query Query) (SeriesSet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateQueryExpression(query.Expression); err != nil {
		return nil, err
	}
	selector, err := parseMemorySelector(query.Expression)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, fmt.Errorf("metrics: memory store is nil")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, fmt.Errorf("metrics: memory store is closed")
	}
	out := SeriesSet{}
	for _, series := range s.series {
		if !selector.matches(series) {
			continue
		}
		point, ok := latestPoint(series.points, query.Time)
		if !ok {
			continue
		}
		out = append(out, Series{Name: series.name, Labels: cloneLabels(series.labels), Points: []Point{point}})
	}
	sortSeries(out)
	return out, nil
}

func (s *MemoryStore) QueryRange(ctx context.Context, query RangeQuery) (SeriesSet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateQueryExpression(query.Expression); err != nil {
		return nil, err
	}
	if query.Start.IsZero() {
		return nil, fmt.Errorf("metrics: range query start is zero")
	}
	if query.End.IsZero() {
		return nil, fmt.Errorf("metrics: range query end is zero")
	}
	if query.End.Before(query.Start) {
		return nil, fmt.Errorf("metrics: range query end is before start")
	}
	if query.Step <= 0 {
		return nil, fmt.Errorf("metrics: range query step must be > 0")
	}
	selector, err := parseMemorySelector(query.Expression)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, fmt.Errorf("metrics: memory store is nil")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, fmt.Errorf("metrics: memory store is closed")
	}
	out := SeriesSet{}
	for _, series := range s.series {
		if !selector.matches(series) {
			continue
		}
		points := pointsInRange(series.points, query.Start, query.End)
		if len(points) == 0 {
			continue
		}
		out = append(out, Series{Name: series.name, Labels: cloneLabels(series.labels), Points: points})
	}
	sortSeries(out)
	return out, nil
}

func (s *MemoryStore) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.series = nil
	return nil
}

func latestPoint(points []Point, at time.Time) (Point, bool) {
	if len(points) == 0 {
		return Point{}, false
	}
	if at.IsZero() {
		return points[len(points)-1], true
	}
	for i := len(points) - 1; i >= 0; i-- {
		if !points[i].Timestamp.After(at) {
			return points[i], true
		}
	}
	return Point{}, false
}

func pointsInRange(points []Point, start, end time.Time) []Point {
	out := make([]Point, 0, len(points))
	for _, point := range points {
		if point.Timestamp.Before(start) || point.Timestamp.After(end) {
			continue
		}
		out = append(out, point)
	}
	return out
}

func parseMemorySelector(expr string) (memorySelector, error) {
	expr = strings.TrimSpace(expr)
	open := strings.IndexByte(expr, '{')
	if open < 0 {
		if err := ValidateMetricName(expr); err != nil {
			return memorySelector{}, err
		}
		return memorySelector{name: expr}, nil
	}
	if !strings.HasSuffix(expr, "}") {
		return memorySelector{}, fmt.Errorf("metrics: unsupported memory query expression %q", expr)
	}
	name := strings.TrimSpace(expr[:open])
	if err := ValidateMetricName(name); err != nil {
		return memorySelector{}, err
	}
	body := strings.TrimSpace(expr[open+1 : len(expr)-1])
	selector := memorySelector{name: name}
	if body == "" {
		return selector, nil
	}
	for _, part := range splitSelectorMatchers(body) {
		matcher, err := parseMemoryMatcher(part)
		if err != nil {
			return memorySelector{}, err
		}
		selector.matchers = append(selector.matchers, matcher)
	}
	return selector, nil
}

func splitSelectorMatchers(body string) []string {
	var out []string
	start := 0
	inQuote := false
	escaped := false
	for i, r := range body {
		switch {
		case escaped:
			escaped = false
		case r == '\\':
			escaped = true
		case r == '"':
			inQuote = !inQuote
		case r == ',' && !inQuote:
			out = append(out, strings.TrimSpace(body[start:i]))
			start = i + 1
		}
	}
	out = append(out, strings.TrimSpace(body[start:]))
	return out
}

func parseMemoryMatcher(text string) (memoryMatcher, error) {
	for _, op := range []MatchOp{MatchNotRegexp, MatchRegexp, MatchNotEqual, MatchEqual} {
		idx := strings.Index(text, string(op))
		if idx < 0 {
			continue
		}
		name := strings.TrimSpace(text[:idx])
		if err := ValidateLabelName(name); err != nil {
			return memoryMatcher{}, err
		}
		value, err := strconv.Unquote(strings.TrimSpace(text[idx+len(op):]))
		if err != nil {
			return memoryMatcher{}, fmt.Errorf("metrics: invalid label matcher value %q: %w", text, err)
		}
		matcher := memoryMatcher{name: name, op: op, value: value}
		if op == MatchRegexp || op == MatchNotRegexp {
			re, err := regexp.Compile(value)
			if err != nil {
				return memoryMatcher{}, fmt.Errorf("metrics: invalid label matcher regexp %q: %w", value, err)
			}
			matcher.re = re
		}
		return matcher, nil
	}
	return memoryMatcher{}, fmt.Errorf("metrics: invalid label matcher %q", text)
}

func (s memorySelector) matches(series *memorySeries) bool {
	if series == nil || series.name != s.name {
		return false
	}
	for _, matcher := range s.matchers {
		value := series.labels[matcher.name]
		switch matcher.op {
		case MatchEqual:
			if value != matcher.value {
				return false
			}
		case MatchNotEqual:
			if value == matcher.value {
				return false
			}
		case MatchRegexp:
			if !matcher.re.MatchString(value) {
				return false
			}
		case MatchNotRegexp:
			if matcher.re.MatchString(value) {
				return false
			}
		}
	}
	return true
}

func memorySeriesKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	parts := make([]string, 0, len(keys)+1)
	parts = append(parts, name)
	for _, key := range keys {
		parts = append(parts, key+"="+labels[key])
	}
	return strings.Join(parts, "\xff")
}

func cloneLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	out := make(map[string]string, len(labels))
	for key, value := range labels {
		out[key] = value
	}
	return out
}

func sortSeries(series SeriesSet) {
	slices.SortFunc(series, func(a, b Series) int {
		if a.Name != b.Name {
			return strings.Compare(a.Name, b.Name)
		}
		return strings.Compare(memorySeriesKey("", a.Labels), memorySeriesKey("", b.Labels))
	})
}
