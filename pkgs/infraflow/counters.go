package infraflow

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	infraapi "github.com/GizClaw/gizclaw-go/pkgs/infraflow/api"
	monitorapi "github.com/GizClaw/gizclaw-go/pkgs/monitor/api"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// Route configures an infrastructure identity, never a device or session.
type Route struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
}

func (r Route) flow() infraapi.Flow {
	return infraapi.Flow{Source: r.Source, Target: r.Target, Kind: infraapi.Request}
}

// CaptureRoute assigns one local activity owner to one directed route.
// EdgePublicKey selects both trusted Edge HTTP and control RPC completions.
// Store selects Get/BatchGet reads on one logical KV Store. Exactly one is required.
type CaptureRoute struct {
	Route         `yaml:",inline"`
	EdgePublicKey string `yaml:"edge_public_key"`
	Store         string `yaml:"store"`
}

// CaptureConfig fixes the local route cells for the process lifetime.
type CaptureConfig struct {
	Node   string         `yaml:"node"`
	Routes []CaptureRoute `yaml:"routes"`
}

// MaxSamplesPerWindow caps retained samples per route/five-second window. Each
// background window rotation grants this many start credits for bounded sampling.
// This is a visual activity signal, not traffic accounting.
const MaxSamplesPerWindow = 20

// Observer admits a bounded number of observations without waiting. An exhausted
// budget costs one atomic load: no clock read, allocation, mutex or I/O. Budget
// replenishment and window rotation belong to Snapshot, outside business work.
type Observer struct {
	remaining atomic.Int32
	owner     *Counters
	route     int
}

// Begin admits one sample, or immediately skips it. There are no CAS retries.
func (o *Observer) Begin() (time.Time, bool) {
	if o == nil {
		return time.Time{}, false
	}
	remaining := o.remaining.Load()
	if remaining <= 0 || !o.remaining.CompareAndSwap(remaining, remaining-1) {
		return time.Time{}, false
	}
	return time.Now(), true
}

// End records a sampled operation. Contention, a missed window or capacity drops
// the sample; it never waits for snapshots or performs network/storage work.
func (o *Observer) End(started time.Time, bytes int64) {
	if o != nil {
		o.owner.add(o.route, bytes, time.Since(started))
	}
}

// Counters retains current and previous fixed windows of best-effort activity.
// Business observations are sampled, bounded and drop instead of waiting.
type Counters struct {
	mu            sync.Mutex
	environment   infraapi.Environment
	generation    string
	windowEnd     int64
	firstEnd      int64
	previousValid bool
	values        [MaxFlows]infraapi.Flow
	previous      [MaxFlows]infraapi.Flow
	routes        []infraapi.Flow
	observers     []*Observer
	edges         map[string]*Observer
	stores        map[string]*Observer
}

// NewCounters validates and allocates a fixed route set without starting work.
func NewCounters(env infraapi.Environment, cfg CaptureConfig) (*Counters, error) {
	if environmentIndex(env) < 0 || !idPattern.MatchString(cfg.Node) || len(cfg.Routes) == 0 || len(cfg.Routes) > MaxFlows {
		return nil, errors.New("infraflow: invalid capture environment, node or capacity")
	}
	c := &Counters{environment: env, edges: map[string]*Observer{}, stores: map[string]*Observer{}}
	var generation [16]byte
	if _, err := rand.Read(generation[:]); err != nil {
		return nil, err
	}
	c.generation = hex.EncodeToString(generation[:])
	c.windowEnd = time.Now().Truncate(Window).Add(Window).UnixMilli()
	c.firstEnd = c.windowEnd
	seen := map[string]bool{}
	for i, r := range cfg.Routes {
		flow := r.flow()
		if !validFlow(flow) || seen[routeKey(flow)] || (r.EdgePublicKey == "") == (r.Store == "") {
			return nil, errors.New("infraflow: invalid or duplicate capture route")
		}
		seen[routeKey(flow)] = true
		observer := &Observer{owner: c, route: i}
		observer.remaining.Store(MaxSamplesPerWindow)
		c.observers = append(c.observers, observer)
		if r.EdgePublicKey != "" {
			if r.Target != cfg.Node {
				return nil, errors.New("infraflow: Edge route must target the capturing node")
			}
			if _, ok := c.edges[r.EdgePublicKey]; ok {
				return nil, errors.New("infraflow: duplicate Edge counter owner")
			}
			c.edges[r.EdgePublicKey] = observer
		} else {
			if r.Source != cfg.Node {
				return nil, errors.New("infraflow: Store route must originate at the capturing node")
			}
			if _, ok := c.stores[r.Store]; ok {
				return nil, errors.New("infraflow: duplicate Store counter owner")
			}
			c.stores[r.Store] = observer
		}
		c.routes = append(c.routes, flow)
		c.values[i] = flow
		c.previous[i] = flow
	}
	return c, nil
}

// EdgeObserver binds a configured trusted peer once, outside the request path.
func (c *Counters) EdgeObserver(publicKey string) *Observer {
	if c == nil {
		return nil
	}
	return c.edges[publicKey]
}

// StoreObserver binds the configured logical Store's bounded sampling gate.
func (c *Counters) StoreObserver(name string) kv.ReadObserver {
	if c == nil {
		return nil
	}
	observer := c.stores[name]
	if observer == nil {
		return nil
	}
	return observer
}

func (c *Counters) rotate(end int64) {
	if end == c.windowEnd {
		return
	}
	if end < c.windowEnd {
		c.firstEnd = end
	}
	c.previousValid = end > c.firstEnd+Window.Milliseconds()
	for i, route := range c.routes {
		c.previous[i] = route
		if end == c.windowEnd+Window.Milliseconds() {
			c.previous[i] = c.values[i]
		}
		c.values[i] = route
		c.observers[i].remaining.Store(MaxSamplesPerWindow)
	}
	c.windowEnd = end
}

func (c *Counters) add(route int, bytes int64, elapsed time.Duration) {
	duration := max(0, float64(elapsed)/float64(time.Millisecond))
	bytes = max(0, bytes)
	if !c.mu.TryLock() {
		return
	}
	end := time.Now().Truncate(Window).Add(Window).UnixMilli()
	if end != c.windowEnd {
		c.mu.Unlock()
		return
	}
	value := &c.values[route]
	if value.Count < MaxSamplesPerWindow && bytes <= MaxCounter-value.Bytes && duration <= MaxCounter-value.DurationSumMs {
		value.Count++
		value.Bytes += bytes
		value.DurationCount++
		value.DurationSumMs += duration
	}
	c.mu.Unlock()
}

// Snapshot returns the latest completed UTC-aligned five-second window for
// authenticated Monitor. Startup's partial window is unavailable. Missing samples
// and zero-valued cells never claim complete activity or inactivity.
// Snapshotting never drains a window, so readers and retries cannot steal counts.
func (c *Counters) Snapshot() *monitorapi.NodeInfraFlows {
	if c == nil {
		return nil
	}
	flows := make([]infraapi.Flow, len(c.routes))
	c.mu.Lock()
	end := time.Now().Truncate(Window).Add(Window).UnixMilli()
	c.rotate(end)
	completed := c.windowEnd - Window.Milliseconds()
	valid := c.previousValid
	copy(flows, c.previous[:len(c.routes)])
	c.mu.Unlock()
	if !valid {
		return nil
	}
	return &monitorapi.NodeInfraFlows{Environment: monitorapi.NodeInfraFlowsEnvironment(c.environment), Generation: c.generation, WindowStart: completed - Window.Milliseconds(), WindowEnd: completed, Flows: flows}
}
