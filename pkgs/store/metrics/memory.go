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
	mu       sync.RWMutex
	series   map[string]*memorySeries
	lookback time.Duration
	closed   bool
}

type memorySeries struct {
	name   string
	labels map[string]string
	points []Point
}

type memorySelector struct {
	aggregation   Aggregation
	rangeFunction memoryRangeFunction
	rangeDuration time.Duration
	rawRange      bool
	timestamp     bool
	name          string
	matchers      []memoryMatcher
}

type memoryRangeFunction string

const (
	memoryRangeAvg   memoryRangeFunction = "avg_over_time"
	memoryRangeMin   memoryRangeFunction = "min_over_time"
	memoryRangeMax   memoryRangeFunction = "max_over_time"
	memoryRangeSum   memoryRangeFunction = "sum_over_time"
	memoryRangeCount memoryRangeFunction = "count_over_time"
	memoryRangeLast  memoryRangeFunction = "last_over_time"
)

type memoryMatcher struct {
	name  string
	op    MatchOp
	value string
	re    *regexp.Regexp
}

const MemoryStoreDefaultLookback = 5 * time.Minute

// NewMemoryStore creates an in-process metrics store for tests and embedded
// single-process integrations.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		series:   make(map[string]*memorySeries),
		lookback: MemoryStoreDefaultLookback,
	}
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
	}
	for _, sample := range samples {
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
	evalTime := query.Time.UTC()
	if evalTime.IsZero() {
		evalTime = time.Now().UTC()
	}
	if s == nil {
		return nil, fmt.Errorf("metrics: memory store is nil")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, fmt.Errorf("metrics: memory store is closed")
	}
	if selector.rangeFunction != "" {
		out := SeriesSet{}
		for _, series := range s.series {
			if !selector.matches(series) {
				continue
			}
			points := series.points
			if selector.timestamp {
				points = timestampPoints(points)
			}
			point, ok := rangeFunctionPoint(points, evalTime.Add(-selector.rangeDuration), evalTime, selector.rangeFunction, evalTime, true)
			if !ok {
				continue
			}
			out = append(out, Series{Name: string(selector.rangeFunction), Labels: cloneLabels(series.labels), Points: []Point{point}})
		}
		sortSeries(out)
		return out, nil
	}
	if selector.rawRange {
		out := SeriesSet{}
		for _, series := range s.series {
			if !selector.matches(series) {
				continue
			}
			points := pointsInWindow(series.points, evalTime.Add(-selector.rangeDuration), evalTime)
			if len(points) == 0 {
				continue
			}
			out = append(out, Series{Name: series.name, Labels: cloneLabels(series.labels), Points: points})
		}
		sortSeries(out)
		return out, nil
	}
	if selector.timestamp {
		out := SeriesSet{}
		for _, series := range s.series {
			if !selector.matches(series) {
				continue
			}
			point, ok := latestPoint(series.points, evalTime, s.lookback)
			if !ok {
				continue
			}
			out = append(out, Series{Name: "timestamp", Labels: cloneLabels(series.labels), Points: []Point{{
				Timestamp: evalTime,
				Value:     float64(point.Timestamp.UnixMilli()) / float64(time.Second/time.Millisecond),
			}}})
		}
		sortSeries(out)
		return out, nil
	}
	if selector.aggregation != "" {
		values := []float64{}
		for _, series := range s.series {
			if !selector.matches(series) {
				continue
			}
			point, ok := latestPoint(series.points, evalTime, s.lookback)
			if ok {
				values = append(values, point.Value)
			}
		}
		if len(values) == 0 {
			return SeriesSet{}, nil
		}
		return SeriesSet{{Name: string(selector.aggregation), Points: []Point{{Timestamp: evalTime, Value: aggregateMemoryValues(selector.aggregation, values)}}}}, nil
	}

	out := SeriesSet{}
	for _, series := range s.series {
		if !selector.matches(series) {
			continue
		}
		point, ok := latestPoint(series.points, evalTime, s.lookback)
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
	if selector.rangeFunction != "" {
		out := SeriesSet{}
		for _, series := range s.series {
			if !selector.matches(series) {
				continue
			}
			points := []Point{}
			for ts := query.Start.UTC(); !ts.After(query.End); ts = ts.Add(query.Step) {
				sourcePoints := series.points
				if selector.timestamp {
					sourcePoints = timestampPoints(sourcePoints)
				}
				point, ok := rangeFunctionPoint(sourcePoints, ts.Add(-selector.rangeDuration), ts, selector.rangeFunction, ts, false)
				if ok {
					points = append(points, point)
				}
			}
			if len(points) == 0 {
				continue
			}
			out = append(out, Series{Name: string(selector.rangeFunction), Labels: cloneLabels(series.labels), Points: points})
		}
		sortSeries(out)
		return out, nil
	}
	if selector.aggregation != "" {
		points := []Point{}
		for ts := query.Start.UTC(); !ts.After(query.End); ts = ts.Add(query.Step) {
			values := []float64{}
			for _, series := range s.series {
				if !selector.matches(series) {
					continue
				}
				point, ok := latestPoint(series.points, ts, s.lookback)
				if ok {
					values = append(values, point.Value)
				}
			}
			if len(values) > 0 {
				points = append(points, Point{Timestamp: ts, Value: aggregateMemoryValues(selector.aggregation, values)})
			}
		}
		if len(points) == 0 {
			return SeriesSet{}, nil
		}
		return SeriesSet{{Name: string(selector.aggregation), Points: points}}, nil
	}

	out := SeriesSet{}
	for _, series := range s.series {
		if !selector.matches(series) {
			continue
		}
		points := pointsInRange(series.points, query.Start.UTC(), query.End.UTC(), query.Step, s.lookback)
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

func latestPoint(points []Point, at time.Time, lookback time.Duration) (Point, bool) {
	if len(points) == 0 {
		return Point{}, false
	}
	for i := len(points) - 1; i >= 0; i-- {
		if !points[i].Timestamp.After(at) {
			if lookback > 0 && at.Sub(points[i].Timestamp) > lookback {
				return Point{}, false
			}
			return points[i], true
		}
	}
	return Point{}, false
}

func pointsInRange(points []Point, start, end time.Time, step time.Duration, lookback time.Duration) []Point {
	out := []Point{}
	for ts := start; !ts.After(end); ts = ts.Add(step) {
		point, ok := latestPoint(points, ts, lookback)
		if ok {
			out = append(out, Point{Timestamp: ts, Value: point.Value})
		}
	}
	return out
}

func timestampPoints(points []Point) []Point {
	out := make([]Point, 0, len(points))
	for _, point := range points {
		out = append(out, Point{
			Timestamp: point.Timestamp,
			Value:     float64(point.Timestamp.UnixMilli()) / float64(time.Second/time.Millisecond),
		})
	}
	return out
}

func rangeFunctionPoint(points []Point, start, end time.Time, fn memoryRangeFunction, timestamp time.Time, preserveLastTimestamp bool) (Point, bool) {
	window := pointsInWindow(points, start, end)
	if len(window) == 0 {
		return Point{}, false
	}
	if fn == memoryRangeLast {
		point := window[len(window)-1]
		if !preserveLastTimestamp {
			point.Timestamp = timestamp
		}
		return point, true
	}
	values := make([]float64, 0, len(window))
	for _, point := range window {
		values = append(values, point.Value)
	}
	aggregation, ok := rangeFunctionAggregation(fn)
	if !ok {
		return Point{}, false
	}
	return Point{Timestamp: timestamp, Value: aggregateMemoryValues(aggregation, values)}, true
}

func pointsInWindow(points []Point, start, end time.Time) []Point {
	out := []Point{}
	for _, point := range points {
		if !point.Timestamp.After(start) {
			continue
		}
		if point.Timestamp.After(end) {
			break
		}
		out = append(out, point)
	}
	return out
}

func rangeFunctionAggregation(fn memoryRangeFunction) (Aggregation, bool) {
	switch fn {
	case memoryRangeAvg:
		return AggregationAvg, true
	case memoryRangeMin:
		return AggregationMin, true
	case memoryRangeMax:
		return AggregationMax, true
	case memoryRangeSum:
		return AggregationSum, true
	case memoryRangeCount:
		return AggregationCount, true
	default:
		return "", false
	}
}

func parseMemorySelector(expr string) (memorySelector, error) {
	expr = strings.TrimSpace(expr)
	rangeFunction, inner, duration, ok, err := parseMemoryRangeFunction(expr)
	if err != nil {
		return memorySelector{}, err
	}
	if ok {
		selector, err := parseMemorySelector(inner)
		if err != nil {
			return memorySelector{}, err
		}
		if selector.aggregation != "" || selector.rangeFunction != "" || selector.rawRange {
			return memorySelector{}, fmt.Errorf("metrics: nested range expression %q is unsupported", expr)
		}
		selector.rangeFunction = rangeFunction
		selector.rangeDuration = duration
		return selector, nil
	}
	inner, duration, ok, err = parseMemoryRawRangeSelector(expr)
	if err != nil {
		return memorySelector{}, err
	}
	if ok {
		selector, err := parseMemorySelector(inner)
		if err != nil {
			return memorySelector{}, err
		}
		if selector.aggregation != "" || selector.rangeFunction != "" || selector.rawRange || selector.timestamp {
			return memorySelector{}, fmt.Errorf("metrics: nested range expression %q is unsupported", expr)
		}
		selector.rawRange = true
		selector.rangeDuration = duration
		return selector, nil
	}
	inner, ok, err = parseMemoryTimestamp(expr)
	if err != nil {
		return memorySelector{}, err
	}
	if ok {
		selector, err := parseMemorySelector(inner)
		if err != nil {
			return memorySelector{}, err
		}
		if selector.aggregation != "" || selector.rangeFunction != "" || selector.rawRange || selector.timestamp {
			return memorySelector{}, fmt.Errorf("metrics: nested timestamp expression %q is unsupported", expr)
		}
		selector.timestamp = true
		return selector, nil
	}
	aggregation, inner, ok, err := parseMemoryAggregation(expr)
	if err != nil {
		return memorySelector{}, err
	}
	if ok {
		selector, err := parseMemorySelector(inner)
		if err != nil {
			return memorySelector{}, err
		}
		if selector.aggregation != "" || selector.rangeFunction != "" || selector.rawRange || selector.timestamp {
			return memorySelector{}, fmt.Errorf("metrics: nested aggregate expression %q is unsupported", expr)
		}
		selector.aggregation = aggregation
		return selector, nil
	}
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

func parseMemoryRangeFunction(expr string) (memoryRangeFunction, string, time.Duration, bool, error) {
	open := strings.IndexByte(expr, '(')
	if open < 0 || !strings.HasSuffix(expr, ")") {
		return "", "", 0, false, nil
	}
	name := strings.TrimSpace(expr[:open])
	fn := memoryRangeFunction(name)
	switch fn {
	case memoryRangeAvg, memoryRangeMin, memoryRangeMax, memoryRangeSum, memoryRangeCount, memoryRangeLast:
	default:
		return "", "", 0, false, nil
	}
	inner := strings.TrimSpace(expr[open+1 : len(expr)-1])
	rangeOpen := strings.LastIndexByte(inner, '[')
	if rangeOpen < 0 || !strings.HasSuffix(inner, "]") {
		return "", "", 0, true, fmt.Errorf("metrics: range function %q requires selector[duration]", expr)
	}
	selector := strings.TrimSpace(inner[:rangeOpen])
	durationText := strings.TrimSpace(inner[rangeOpen+1 : len(inner)-1])
	if beforeResolution, _, ok := strings.Cut(durationText, ":"); ok {
		durationText = strings.TrimSpace(beforeResolution)
	}
	duration, err := time.ParseDuration(durationText)
	if err != nil || duration <= 0 {
		return "", "", 0, true, fmt.Errorf("metrics: invalid range duration %q", durationText)
	}
	return fn, selector, duration, true, nil
}

func parseMemoryRawRangeSelector(expr string) (string, time.Duration, bool, error) {
	rangeOpen := strings.LastIndexByte(expr, '[')
	if rangeOpen < 0 || !strings.HasSuffix(expr, "]") {
		return "", 0, false, nil
	}
	selector := strings.TrimSpace(expr[:rangeOpen])
	durationText := strings.TrimSpace(expr[rangeOpen+1 : len(expr)-1])
	if selector == "" || durationText == "" {
		return "", 0, true, fmt.Errorf("metrics: invalid range selector %q", expr)
	}
	duration, err := time.ParseDuration(durationText)
	if err != nil || duration <= 0 {
		return "", 0, true, fmt.Errorf("metrics: invalid range duration %q", durationText)
	}
	return selector, duration, true, nil
}

func parseMemoryTimestamp(expr string) (string, bool, error) {
	open := strings.IndexByte(expr, '(')
	if open < 0 || !strings.HasSuffix(expr, ")") {
		return "", false, nil
	}
	if strings.TrimSpace(expr[:open]) != "timestamp" {
		return "", false, nil
	}
	inner := strings.TrimSpace(expr[open+1 : len(expr)-1])
	if inner == "" {
		return "", true, fmt.Errorf("metrics: timestamp expression %q is empty", expr)
	}
	return inner, true, nil
}

func parseMemoryAggregation(expr string) (Aggregation, string, bool, error) {
	open := strings.IndexByte(expr, '(')
	if open < 0 || !strings.HasSuffix(expr, ")") {
		return "", "", false, nil
	}
	name := strings.TrimSpace(expr[:open])
	inner := strings.TrimSpace(expr[open+1 : len(expr)-1])
	if inner == "" {
		return "", "", false, fmt.Errorf("metrics: aggregate expression %q is empty", expr)
	}
	aggregation := Aggregation(name)
	switch aggregation {
	case AggregationAvg, AggregationMin, AggregationMax, AggregationSum, AggregationCount:
		return aggregation, inner, true, nil
	default:
		return "", "", false, nil
	}
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
	idx, op, ok := memoryMatcherOperator(text)
	if !ok {
		return memoryMatcher{}, fmt.Errorf("metrics: invalid label matcher %q", text)
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
		re, err := regexp.Compile("^(?:" + value + ")$")
		if err != nil {
			return memoryMatcher{}, fmt.Errorf("metrics: invalid label matcher regexp %q: %w", value, err)
		}
		matcher.re = re
	}
	return matcher, nil
}

func memoryMatcherOperator(text string) (int, MatchOp, bool) {
	inQuote := false
	escaped := false
	for i := 0; i < len(text); i++ {
		ch := text[i]
		switch {
		case escaped:
			escaped = false
			continue
		case ch == '\\':
			escaped = true
			continue
		case ch == '"':
			inQuote = !inQuote
			continue
		case inQuote:
			continue
		}
		for _, op := range []MatchOp{MatchNotRegexp, MatchRegexp, MatchNotEqual, MatchEqual} {
			if strings.HasPrefix(text[i:], string(op)) {
				return i, op, true
			}
		}
	}
	return -1, "", false
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

func aggregateMemoryValues(aggregation Aggregation, values []float64) float64 {
	switch aggregation {
	case AggregationAvg:
		return aggregateMemoryValues(AggregationSum, values) / float64(len(values))
	case AggregationMin:
		out := values[0]
		for _, value := range values[1:] {
			if value < out {
				out = value
			}
		}
		return out
	case AggregationMax:
		out := values[0]
		for _, value := range values[1:] {
			if value > out {
				out = value
			}
		}
		return out
	case AggregationCount:
		return float64(len(values))
	default:
		var out float64
		for _, value := range values {
			out += value
		}
		return out
	}
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
