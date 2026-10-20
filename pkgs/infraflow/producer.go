package infraflow

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	infraapi "github.com/GizClaw/gizclaw-go/pkgs/infraflow/api"
	monitorapi "github.com/GizClaw/gizclaw-go/pkgs/monitor/api"
)

const collectionTimeout = 2 * time.Second
const maxSnapshotBytes = 64 * 1024

// SourceConfig pins a remote Monitor identity and its complete route ownership.
// Endpoint is an operator-configured absolute /monitor/api/node URL.
type SourceConfig struct {
	Endpoint  string  `yaml:"endpoint"`
	PublicKey string  `yaml:"public_key"`
	Token     string  `yaml:"token" json:"-"`
	Routes    []Route `yaml:"routes"`
}

// ProducerConfig enables the single publisher for one environment. Local capture
// is included automatically; Sources adds explicitly owned remote Server routes.
type ProducerConfig struct {
	Sources []SourceConfig `yaml:"sources"`
}

func (c *Config) validateCollection() error {
	if c.Environment != "" && environmentIndex(c.Environment) < 0 {
		return errors.New("infraflow: invalid environment")
	}
	if c.Capture == nil && c.Producer == nil {
		return nil
	}
	if environmentIndex(c.Environment) < 0 {
		return errors.New("infraflow: collection requires a fixed environment")
	}
	var local *Counters
	var err error
	if c.Capture != nil {
		local, err = NewCounters(c.Environment, *c.Capture)
		if err != nil {
			return err
		}
	}
	if c.Producer != nil {
		if c.Store == "" {
			return errors.New("infraflow: producer requires infra_flow.store")
		}
		_, err = producerSources(c.Environment, local, *c.Producer)
	}
	return err
}

type counterSource struct {
	config SourceConfig
	routes []infraapi.Flow
	offset int
	local  *Counters
}

func producerSources(env infraapi.Environment, local *Counters, cfg ProducerConfig) ([]counterSource, error) {
	if environmentIndex(env) < 0 || len(cfg.Sources) > MaxFlows {
		return nil, errors.New("infraflow: invalid producer environment or source capacity")
	}
	var sources []counterSource
	if local != nil {
		if local.environment != env {
			return nil, errors.New("infraflow: local environment mismatch")
		}
		sources = append(sources, counterSource{local: local, routes: append([]infraapi.Flow(nil), local.routes...)})
	}
	identities := map[string]bool{}
	for _, config := range cfg.Sources {
		u, err := url.Parse(config.Endpoint)
		if err != nil || u.Host == "" || u.User != nil || u.Path != "/monitor/api/node" || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "::1" || u.Hostname() == "localhost"))) {
			return nil, errors.New("infraflow: source requires HTTPS Monitor endpoint (HTTP only on loopback)")
		}
		config.Token = os.ExpandEnv(config.Token)
		if !strings.HasPrefix(config.Token, "gizclaw_mk_") || len(strings.TrimPrefix(config.Token, "gizclaw_mk_")) < 32 || strings.ContainsAny(config.Token, "\r\n") || len(config.Token) > 4096 || config.PublicKey == "" || identities[config.PublicKey] || len(config.Routes) == 0 || len(config.Routes) > MaxFlows {
			return nil, errors.New("infraflow: invalid or duplicate Monitor source identity, token or routes")
		}
		identities[config.PublicKey] = true
		source := counterSource{config: config}
		for _, route := range config.Routes {
			source.routes = append(source.routes, route.flow())
		}
		sources = append(sources, source)
	}
	seen := map[string]bool{}
	total := 0
	for i := range sources {
		sources[i].offset = total
		for _, flow := range sources[i].routes {
			if !validFlow(flow) || seen[routeKey(flow)] {
				return nil, errors.New("infraflow: invalid or duplicate route ownership")
			}
			seen[routeKey(flow)] = true
			total++
		}
	}
	if total == 0 || total > MaxFlows {
		return nil, errors.New("infraflow: producer requires 1..64 routes")
	}
	return sources, nil
}

// Producer collects completed fixed-window counters into existing Aggregator and
// Feed primitives. It borrows Store and has no event queue or catch-up backlog.
type Producer struct {
	feed        *Feed
	aggregate   *Aggregator
	environment infraapi.Environment
	sources     []counterSource
	client      *http.Client
}

// NewProducer validates every directed route has exactly one source. Run owns
// the worker lifecycle; construction does no network I/O and starts no goroutine.
func NewProducer(feed *Feed, env infraapi.Environment, local *Counters, cfg ProducerConfig) (*Producer, error) {
	if feed == nil {
		return nil, errors.New("infraflow: producer requires Feed")
	}
	sources, err := producerSources(env, local, cfg)
	if err != nil {
		return nil, err
	}
	var routes []infraapi.Flow
	for _, source := range sources {
		routes = append(routes, source.routes...)
	}
	aggregate, err := NewAggregator(env, routes, false)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment, ForceAttemptHTTP2: true, IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: time.Second}
	transport.MaxIdleConns = MaxFlows
	transport.MaxConnsPerHost = 1
	transport.MaxResponseHeaderBytes = 8 * 1024
	return &Producer{feed: feed, aggregate: aggregate, environment: env, sources: sources, client: &http.Client{Transport: transport, Timeout: collectionTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// Run publishes only complete aligned windows. Failed/missing/reset sources are
// omitted; idle healthy sources retain zero-valued routes.
// Cancellation joins all bounded scrape work before returning. No shutdown frame
// is emitted, and the borrowed Store remains open.
func (p *Producer) Run(ctx context.Context) {
	defer p.client.CloseIdleConnections()
	next := time.Now().Truncate(Window).Add(Window)
	for {
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		end := time.Now().Truncate(Window)
		if err := p.collect(ctx, end); err != nil && ctx.Err() == nil {
			// Deliberately avoid provider errors, URLs, tokens and payloads in logs.
			slog.WarnContext(ctx, "infraflow: collection window unavailable", "environment", p.environment, "window_end", end.UnixMilli())
			if errors.Is(err, ErrWindowConflict) {
				return
			}
		}
		next = end.Add(Window)
		if !time.Now().Before(next) {
			next = time.Now().Truncate(Window).Add(Window)
		}
	}
}

type sourceResult struct {
	snapshot *monitorapi.NodeInfraFlows
	err      error
}

func (p *Producer) collect(ctx context.Context, end time.Time) error {
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	results := make(chan struct {
		index int
		sourceResult
	}, len(p.sources))
	// Fixed fan-out: at most 64 sources, one request per source and window.
	for i := range p.sources {
		go func(i int) {
			snapshot, err := p.snapshot(ctx, &p.sources[i], end)
			results <- struct {
				index int
				sourceResult
			}{i, sourceResult{snapshot, err}}
		}(i)
	}
	available := make([]bool, MaxFlows)
	var errs []error
	for range p.sources {
		result := <-results
		source := &p.sources[result.index]
		current := result.snapshot
		if result.err != nil {
			errs = append(errs, result.err)
			continue
		}
		for i, flow := range current.Flows {
			route := source.offset + i
			if err := p.aggregate.Add(route, flow.Count, flow.Bytes, flow.DurationSumMs, flow.DurationCount); err != nil {
				errs = append(errs, err)
				continue
			}
			available[route] = true
		}
	}

	frame := p.aggregate.Drain(end)
	flows := frame.Flows[:0]
	for i, flow := range frame.Flows {
		if available[i] {
			flows = append(flows, flow)
		}
	}
	frame.Flows = flows
	if len(flows) == 0 {
		return errors.Join(errs...)
	}
	if parent.Err() != nil {
		return parent.Err()
	}
	appendCtx, appendCancel := context.WithTimeout(parent, time.Second)
	defer appendCancel()
	// One atomic append per window, with a separate bounded write budget.
	if err := p.feed.Append(appendCtx, frame); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (p *Producer) snapshot(ctx context.Context, source *counterSource, end time.Time) (*monitorapi.NodeInfraFlows, error) {
	var snapshot *monitorapi.NodeInfraFlows
	if source.local != nil {
		snapshot = source.local.Snapshot()
	} else {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.config.Endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+source.config.Token)
		response, err := p.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return nil, errors.New("infraflow: Monitor source unavailable")
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, maxSnapshotBytes+1))
		if err != nil || len(data) > maxSnapshotBytes {
			return nil, errors.New("infraflow: oversized or unreadable Monitor snapshot")
		}
		var node monitorapi.NodeSnapshot
		if err := json.Unmarshal(data, &node); err != nil {
			return nil, err
		}
		if node.PublicKey != source.config.PublicKey || node.Role != "server" || time.Since(node.Time) > collectionTimeout || time.Until(node.Time) > collectionTimeout {
			return nil, errors.New("infraflow: stale or mismatched Monitor identity")
		}
		snapshot = node.InfraFlows
	}
	if snapshot == nil || string(snapshot.Environment) != string(p.environment) || len(snapshot.Generation) == 0 || len(snapshot.Generation) > 64 || len(snapshot.Flows) != len(source.routes) || snapshot.WindowEnd != end.UnixMilli() || snapshot.WindowStart != end.Add(-Window).UnixMilli() {
		return nil, errors.New("infraflow: missing or mismatched counter snapshot")
	}
	flows := make([]infraapi.Flow, len(source.routes))
	for i, route := range source.routes {
		index := slices.IndexFunc(snapshot.Flows, func(flow infraapi.Flow) bool { return routeKey(flow) == routeKey(route) })
		if index < 0 || !validFlow(snapshot.Flows[index]) {
			return nil, errors.New("infraflow: counter route mismatch")
		}
		flows[i] = snapshot.Flows[index]
	}
	snapshot.Flows = flows
	return snapshot, nil
}
