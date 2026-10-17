package infraflow

import (
	"errors"
	"strconv"
	"sync"
	"time"

	infraapi "github.com/GizClaw/gizclaw-go/pkgs/infraflow/api"
)

// Aggregator retains one counter cell per configured infrastructure route.
// Call Add with counter deltas or already aggregated totals; no user or session
// identifiers are accepted. The owner schedules Drain once per Window.
type Aggregator struct {
	mu          sync.Mutex
	routes      []infraapi.Flow
	values      [MaxFlows]infraapi.Flow
	environment infraapi.Environment
	simulated   bool
}

// NewAggregator fixes the route set for this producer's lifetime.
func NewAggregator(env infraapi.Environment, routes []infraapi.Flow, simulated bool) (*Aggregator, error) {
	frame := infraapi.Frame{Id: "5000", Environment: env, WindowEnd: 5000, Flows: routes}
	if len(routes) == 0 {
		return nil, errors.New("infraflow: at least one configured route is required")
	}
	if err := validateFrame(frame); err != nil {
		return nil, err
	}
	a := &Aggregator{environment: env, simulated: simulated, routes: append([]infraapi.Flow(nil), routes...)}
	for i, route := range a.routes {
		route.Count, route.Bytes, route.DurationCount, route.DurationSumMs = 0, 0, 0, 0
		a.routes[i] = route
		a.values[i] = route
	}
	return a, nil
}

// Add merges a preaggregated counter delta into a preconfigured route cell.
// Producers read their existing fixed-route counters at window cadence. It never creates
// a new route, key, event object or retained item when activity increases.
func (a *Aggregator) Add(route int, count, bytes int64, durationSumMs float64, durationCount int64) error {
	if route < 0 || route >= len(a.routes) {
		return errors.New("infraflow: unconfigured route")
	}
	value := a.routes[route]
	value.Count, value.Bytes, value.DurationSumMs, value.DurationCount = count, bytes, durationSumMs, durationCount
	if !validFlow(value) {
		return errors.New("infraflow: invalid aggregate observation")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	current := &a.values[route]
	if current.Count > MaxCounter-count || current.Bytes > MaxCounter-bytes || current.DurationCount > MaxCounter-durationCount || current.DurationSumMs > MaxCounter-durationSumMs {
		return errors.New("infraflow: counter capacity exceeded")
	}
	current.Count += count
	current.Bytes += bytes
	current.DurationSumMs += durationSumMs
	current.DurationCount += durationCount
	return nil
}

// Drain atomically takes one fixed-size aggregate frame. It performs no KV I/O.
// The producer remains best effort: a failed append does not accumulate backlog.
func (a *Aggregator) Drain(end time.Time) infraapi.Frame {
	endMs := end.UnixMilli() / Window.Milliseconds() * Window.Milliseconds()
	frame := infraapi.Frame{Id: strconv.FormatInt(endMs, 10), Environment: a.environment, WindowStart: endMs - Window.Milliseconds(), WindowEnd: endMs, Simulated: a.simulated, Flows: make([]infraapi.Flow, len(a.routes))}
	a.mu.Lock()
	for i, route := range a.routes {
		frame.Flows[i] = a.values[i]
		a.values[i] = route
	}
	a.mu.Unlock()
	return frame
}
