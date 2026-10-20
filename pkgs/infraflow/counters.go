package infraflow

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	infraapi "github.com/GizClaw/gizclaw-go/pkgs/infraflow/api"
	monitorapi "github.com/GizClaw/gizclaw-go/pkgs/monitor/api"
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

// Counters retains current and previous fixed windows per configured route. Completions only
// update memory; snapshots, network and KV work never run in the activity path.
type Counters struct {
	mu            sync.Mutex
	environment   infraapi.Environment
	generation    string
	windowEnd     int64
	firstEnd      int64
	valid         bool
	previousValid bool
	values        [MaxFlows]infraapi.Flow
	previous      [MaxFlows]infraapi.Flow
	routes        []infraapi.Flow
	edges         map[string]int
	stores        map[string]int
}

// NewCounters validates and allocates a fixed route set without starting work.
func NewCounters(env infraapi.Environment, cfg CaptureConfig) (*Counters, error) {
	if environmentIndex(env) < 0 || !idPattern.MatchString(cfg.Node) || len(cfg.Routes) == 0 || len(cfg.Routes) > MaxFlows {
		return nil, errors.New("infraflow: invalid capture environment, node or capacity")
	}
	c := &Counters{environment: env, edges: map[string]int{}, stores: map[string]int{}}
	var generation [16]byte
	if _, err := rand.Read(generation[:]); err != nil {
		return nil, err
	}
	c.generation = hex.EncodeToString(generation[:])
	c.windowEnd = time.Now().Truncate(Window).Add(Window).UnixMilli()
	c.firstEnd = c.windowEnd
	c.valid = true
	seen := map[string]bool{}
	for i, r := range cfg.Routes {
		flow := r.flow()
		if !validFlow(flow) || seen[routeKey(flow)] || (r.EdgePublicKey == "") == (r.Store == "") {
			return nil, errors.New("infraflow: invalid or duplicate capture route")
		}
		seen[routeKey(flow)] = true
		if r.EdgePublicKey != "" {
			if r.Target != cfg.Node {
				return nil, errors.New("infraflow: Edge route must target the capturing node")
			}
			if _, ok := c.edges[r.EdgePublicKey]; ok {
				return nil, errors.New("infraflow: duplicate Edge counter owner")
			}
			c.edges[r.EdgePublicKey] = i
		} else {
			if r.Source != cfg.Node {
				return nil, errors.New("infraflow: Store route must originate at the capturing node")
			}
			if _, ok := c.stores[r.Store]; ok {
				return nil, errors.New("infraflow: duplicate Store counter owner")
			}
			c.stores[r.Store] = i
		}
		c.routes = append(c.routes, flow)
		c.values[i] = flow
		c.previous[i] = flow
	}
	return c, nil
}

// Edge records one completed trusted Edge HTTP or control RPC call. Bytes are
// actual application payload bytes read/written, excluding transport overhead.
func (c *Counters) Edge(publicKey string, bytes int64, elapsed time.Duration) {
	if c == nil {
		return
	}
	if route, ok := c.edges[publicKey]; ok {
		c.add(route, bytes, elapsed)
	}
}

// StoreObserver returns a fixed-cell callback, or nil for an unconfigured Store.
func (c *Counters) StoreObserver(name string) func(time.Duration, int64) {
	if c == nil {
		return nil
	}
	route, ok := c.stores[name]
	if !ok {
		return nil
	}
	return func(elapsed time.Duration, bytes int64) { c.add(route, bytes, elapsed) }
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
			c.previousValid = c.previousValid && c.valid
		}
		c.values[i] = route
	}
	c.valid = true
	c.windowEnd = end
}

func (c *Counters) add(route int, bytes int64, elapsed time.Duration) {
	duration := max(0, float64(elapsed)/float64(time.Millisecond))
	bytes = max(0, bytes)
	c.mu.Lock()
	end := time.Now().Truncate(Window).Add(Window).UnixMilli()
	c.rotate(end)
	value := &c.values[route]
	if value.Count == MaxCounter || bytes > MaxCounter-value.Bytes || duration > MaxCounter-value.DurationSumMs {
		c.valid = false
	} else {
		value.Count++
		value.Bytes += bytes
		value.DurationCount++
		value.DurationSumMs += duration
	}
	c.mu.Unlock()
}

// Snapshot returns the latest completed UTC-aligned five-second window for
// authenticated Monitor. Startup's partial window and overflow are unavailable.
// Snapshotting never drains a window, so readers and retries cannot steal counts.
func (c *Counters) Snapshot() *monitorapi.NodeInfraFlows {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	end := time.Now().Truncate(Window).Add(Window).UnixMilli()
	c.rotate(end)
	completed := c.windowEnd - Window.Milliseconds()
	valid := c.previousValid
	flows := append([]infraapi.Flow(nil), c.previous[:len(c.routes)]...)
	c.mu.Unlock()
	if !valid {
		return nil
	}
	return &monitorapi.NodeInfraFlows{Environment: monitorapi.NodeInfraFlowsEnvironment(c.environment), Generation: c.generation, WindowStart: completed - Window.Milliseconds(), WindowEnd: completed, Flows: flows}
}
