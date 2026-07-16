package metrics

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sort"
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

// NewMemoryStore creates an empty in-process metrics store.
func NewMemoryStore() *MemoryStore { return &MemoryStore{series: make(map[string]*memorySeries)} }

// Append stores samples in memory.
func (s *MemoryStore) Append(ctx context.Context, samples []Sample) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("metrics: memory store is nil")
	}
	for _, sample := range samples {
		if err := validateSample(sample); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("metrics: memory store is closed")
	}
	for _, sample := range samples {
		labels := cloneLabels(sample.Labels)
		key := memorySeriesKey(sample.Name, labels)
		ms := s.series[key]
		if ms == nil {
			ms = &memorySeries{name: sample.Name, labels: labels}
			s.series[key] = ms
		}
		ms.points = append(ms.points, Point{Timestamp: sample.Timestamp.UTC(), Value: sample.Value})
		slices.SortFunc(ms.points, func(a, b Point) int { return a.Timestamp.Compare(b.Timestamp) })
	}
	return nil
}

// Latest returns the newest matching in-memory sample.
func (s *MemoryStore) Latest(ctx context.Context, q LatestQuery) (SeriesSet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateLatestQuery(q); err != nil {
		return nil, err
	}
	return s.read(func(ms *memorySeries) ([]Point, bool) {
		p, ok := latestPointInclusive(ms.points, q.At.UTC(), q.Lookback)
		return []Point{p}, ok
	}, q.Selector)
}

// Range evaluates last-sample windows over in-memory samples.
func (s *MemoryStore) Range(ctx context.Context, q RangeQuery) (SeriesSet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateRangeQuery(q); err != nil {
		return nil, err
	}
	times := []time.Time{}
	start, end := q.Start.UTC(), q.End.UTC()
	times = append(times, start)
	for t := start.Add(q.Step); !t.After(end); t = t.Add(q.Step) {
		times = append(times, t)
	}
	if times[len(times)-1].Before(end) {
		times = append(times, end)
	}
	return s.read(func(ms *memorySeries) ([]Point, bool) {
		out := []Point{}
		for i, t := range times {
			if i == 0 {
				if p, ok := exactPoint(ms.points, t); ok {
					out = append(out, Point{Timestamp: t, Value: p.Value})
				}
				continue
			}
			if p, ok := latestPoint(ms.points, t, q.Step); ok {
				out = append(out, Point{Timestamp: t, Value: p.Value})
			}
		}
		return out, len(out) > 0
	}, q.Selector)
}

// Aggregate evaluates bucket operations over in-memory samples.
func (s *MemoryStore) Aggregate(ctx context.Context, q AggregateQuery) (SeriesSet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateAggregateQuery(q); err != nil {
		return nil, err
	}
	start, end := q.Start.UTC(), q.End.UTC()
	return s.read(func(ms *memorySeries) ([]Point, bool) {
		out := []Point{}
		first := true
		for bs := start; !bs.After(end); bs = bs.Add(q.Bucket) {
			be := bs.Add(q.Bucket)
			if be.After(end) {
				be = end
			}
			vals := []Point{}
			for _, p := range ms.points {
				if p.Timestamp.Before(bs) || (!first && p.Timestamp.Equal(bs)) || p.Timestamp.After(be) {
					continue
				}
				vals = append(vals, p)
			}
			if len(vals) > 0 {
				out = append(out, Point{Timestamp: bs, Value: aggregatePoints(q.Operation, vals)})
			}
			first = false
			if be.Equal(end) {
				break
			}
		}
		return out, len(out) > 0
	}, q.Selector)
}
func (s *MemoryStore) read(fn func(*memorySeries) ([]Point, bool), sel Selector) (SeriesSet, error) {
	if s == nil {
		return nil, fmt.Errorf("metrics: memory store is nil")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, fmt.Errorf("metrics: memory store is closed")
	}
	out := SeriesSet{}
	for _, ms := range s.series {
		if !matchesSelector(ms, sel) {
			continue
		}
		if points, ok := fn(ms); ok {
			out = append(out, Series{Name: ms.name, Labels: cloneLabels(ms.labels), Points: points})
		}
	}
	sortSeries(out)
	return out, nil
}

// Close prevents further reads and writes.
func (s *MemoryStore) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}
func latestPoint(points []Point, at time.Time, lookback time.Duration) (Point, bool) {
	start := at.Add(-lookback)
	for i := len(points) - 1; i >= 0; i-- {
		p := points[i]
		if p.Timestamp.After(at) {
			continue
		}
		if !p.Timestamp.After(start) {
			return Point{}, false
		}
		return p, true
	}
	return Point{}, false
}
func latestPointInclusive(points []Point, at time.Time, lookback time.Duration) (Point, bool) {
	start := at.Add(-lookback)
	for i := len(points) - 1; i >= 0; i-- {
		p := points[i]
		if p.Timestamp.After(at) {
			continue
		}
		if p.Timestamp.Before(start) {
			return Point{}, false
		}
		return p, true
	}
	return Point{}, false
}
func exactPoint(points []Point, at time.Time) (Point, bool) {
	i, _ := slices.BinarySearchFunc(points, Point{Timestamp: at}, func(a, b Point) int { return a.Timestamp.Compare(b.Timestamp) })
	if i < len(points) && points[i].Timestamp.Equal(at) {
		return points[i], true
	}
	return Point{}, false
}
func aggregatePoints(op Aggregation, ps []Point) float64 {
	v := ps[0].Value
	switch op {
	case AggregationLast:
		return ps[len(ps)-1].Value
	case AggregationCount:
		return float64(len(ps))
	case AggregationMin:
		for _, p := range ps[1:] {
			v = min(v, p.Value)
		}
		return v
	case AggregationMax:
		for _, p := range ps[1:] {
			v = max(v, p.Value)
		}
		return v
	case AggregationSum, AggregationAvg:
		for _, p := range ps[1:] {
			v += p.Value
		}
		if op == AggregationAvg {
			v /= float64(len(ps))
		}
		return v
	}
	return v
}
func matchesSelector(ms *memorySeries, s Selector) bool {
	if ms.name != s.Name {
		return false
	}
	for _, m := range s.Matchers {
		v := ms.labels[m.Name]
		var ok bool
		switch m.Op {
		case MatchEqual:
			ok = v == m.Value
		case MatchNotEqual:
			ok = v != m.Value
		case MatchRegexp:
			ok = regexp.MustCompile("^(?:" + m.Value + ")$").MatchString(v)
		case MatchNotRegexp:
			ok = !regexp.MustCompile("^(?:" + m.Value + ")$").MatchString(v)
		}
		if !ok {
			return false
		}
	}
	return true
}
func cloneLabels(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}
func memorySeriesKey(name string, labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(name)
	for _, k := range keys {
		fmt.Fprintf(&b, "|%d:%s=%d:%s", len(k), k, len(labels[k]), labels[k])
	}
	return b.String()
}
func sortSeries(s SeriesSet) {
	slices.SortFunc(s, func(a, b Series) int {
		return strings.Compare(memorySeriesKey(a.Name, a.Labels), memorySeriesKey(b.Name, b.Labels))
	})
}
