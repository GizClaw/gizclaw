package gizmetrics

import "context"

// CounterDelta is one non-negative increment with stable, bounded labels.
type CounterDelta struct {
	Value  float64
	Labels []Label
}

// AddPeriodicCounters records counters reported together. After their first
// update, these series publish cumulative totals on every flush, even while
// idle, so rate queries can distinguish silence from inactivity. Ordinary
// AddCounter series publish only after changes. Before installation and after
// shutdown this is a no-op, and it never performs storage I/O.
func AddPeriodicCounters(ctx context.Context, name string, deltas ...CounterDelta) {
	slot := processRecorder.Load()
	if slot == nil {
		return
	}
	slot.recorder.addPeriodicCounters(ctx, name, deltas)
}

// The fast cache is private to an installed recorder and capped at maxSeries.
// Wider label sets use the ordinary validation path. Raw label order is part
// of this cache key; canonical identity and the series limit remain authoritative.
const cachedCounterLabels = 4

type counterCacheKey struct {
	name   string
	labels [cachedCounterLabels]Label
	count  int
}

func counterKey(name string, labels []Label) (counterCacheKey, bool) {
	if len(labels) > cachedCounterLabels {
		return counterCacheKey{}, false
	}
	key := counterCacheKey{name: name, count: len(labels)}
	copy(key.labels[:], labels)
	return key, true
}

func (r *storeRecorder) addPeriodicCounters(ctx context.Context, name string, deltas []CounterDelta) {
	// Once registered, a batch reuses canonical labels and series handles and
	// commits all its increments under one short state lock.
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	for _, delta := range deltas {
		key, cacheable := counterKey(name, delta.Labels)
		if !cacheable || !finite(delta.Value) || delta.Value < 0 || r.counterCache[key] == nil {
			r.mu.Unlock()
			r.registerPeriodicCounters(ctx, name, deltas)
			return
		}
	}
	overflow := false
	for _, delta := range deltas {
		key, _ := counterKey(name, delta.Labels)
		series := r.counterCache[key]
		next := series.value + delta.Value
		if !finite(next) {
			overflow = true
			continue
		}
		series.value = next
		series.version++
		series.dirty = true
	}
	r.mu.Unlock()
	if overflow {
		r.warn(ctx, name, "counter total must remain finite")
	}
}

func (r *storeRecorder) registerPeriodicCounters(ctx context.Context, name string, deltas []CounterDelta) {
	for _, delta := range deltas {
		if !finite(delta.Value) || delta.Value < 0 {
			r.warn(ctx, name, "counter delta must be finite and non-negative")
			continue
		}
		series := r.record(ctx, metricKindPeriodicCounter, name, delta.Value, nil, delta.Labels)
		if series == nil {
			continue
		}
		key, cacheable := counterKey(name, delta.Labels)
		if !cacheable {
			continue
		}
		r.mu.Lock()
		if !r.closed && len(r.counterCache) < r.config.maxSeries {
			if r.counterCache == nil {
				r.counterCache = make(map[counterCacheKey]*metricSeries)
			}
			r.counterCache[key] = series
		}
		r.mu.Unlock()
	}
}
