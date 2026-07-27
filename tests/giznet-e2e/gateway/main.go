// Command gateway-load exercises logical GizClaw sessions through one or more
// Edge gateways and writes a machine-readable capacity artifact.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"runtime/metrics"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

const artifactVersion = 1

type options struct {
	edges                    []string
	sessions                 int
	ramp                     time.Duration
	duration                 time.Duration
	pingInterval             time.Duration
	dialTimeout              time.Duration
	pingTimeout              time.Duration
	concurrency              int
	artifactPath             string
	maxEstablishmentFailures int
	maxPingFailures          int
	maxP99RTT                time.Duration
}

type artifact struct {
	Version               int                       `json:"version"`
	StartedAt             time.Time                 `json:"started_at"`
	FinishedAt            time.Time                 `json:"finished_at"`
	Host                  hostSummary               `json:"host"`
	Config                artifactConfig            `json:"config"`
	Attempted             int                       `json:"attempted"`
	Established           int                       `json:"established"`
	EstablishmentFailures int                       `json:"establishment_failures"`
	PingsAttempted        int                       `json:"pings_attempted"`
	PingFailures          int                       `json:"ping_failures"`
	UnexpectedDisconnects int                       `json:"unexpected_disconnects"`
	IdentityCrossover     bool                      `json:"identity_crossover"`
	RTT                   latencySummary            `json:"rtt_ms"`
	BytesPerSession       byteSummary               `json:"bytes_per_session"`
	EdgeDistribution      map[string]int            `json:"edge_distribution"`
	UpstreamDistribution  map[string]map[string]int `json:"upstream_distribution"`
	ResourceUsage         resourceSummary           `json:"resource_usage"`
	Errors                []string                  `json:"errors,omitempty"`
	Passed                bool                      `json:"passed"`
}

type hostSummary struct {
	GOOS       string `json:"goos"`
	GOARCH     string `json:"goarch"`
	GoVersion  string `json:"go_version"`
	LogicalCPU int    `json:"logical_cpu"`
}

type artifactConfig struct {
	Edges                    []string      `json:"edges"`
	Sessions                 int           `json:"sessions"`
	Ramp                     time.Duration `json:"ramp"`
	Duration                 time.Duration `json:"duration"`
	PingInterval             time.Duration `json:"ping_interval"`
	DialTimeout              time.Duration `json:"dial_timeout"`
	PingTimeout              time.Duration `json:"ping_timeout"`
	Concurrency              int           `json:"concurrency"`
	MaxEstablishmentFailures int           `json:"max_establishment_failures"`
	MaxPingFailures          int           `json:"max_ping_failures"`
	MaxP99RTT                time.Duration `json:"max_p99_rtt"`
}

type latencySummary struct {
	Count int     `json:"count"`
	P50   float64 `json:"p50"`
	P95   float64 `json:"p95"`
	P99   float64 `json:"p99"`
	Max   float64 `json:"max"`
}

type byteSummary struct {
	RxTotal uint64  `json:"rx_total"`
	TxTotal uint64  `json:"tx_total"`
	RxMean  float64 `json:"rx_mean"`
	TxMean  float64 `json:"tx_mean"`
}

type resourcePoint struct {
	At             time.Time `json:"at"`
	RSSBytes       uint64    `json:"rss_bytes"`
	CPUSeconds     float64   `json:"cpu_seconds"`
	OpenFDs        int       `json:"open_fds"`
	HeapAllocBytes uint64    `json:"heap_alloc_bytes"`
	Goroutines     int       `json:"goroutines"`
}

type resourceSummary struct {
	Start resourcePoint `json:"start"`
	Peak  resourcePoint `json:"peak"`
	End   resourcePoint `json:"end"`
}

type edgeMetadata struct {
	endpoint      string
	serverKey     giznet.PublicKey
	transportKey  giznet.PublicKey
	signalingURL  string
	authoritative apitypes.ServerInfo
}

type liveSession struct {
	client   *gizcli.Client
	edge     string
	upstream string
	rxBytes  uint64
	txBytes  uint64
	closed   atomic.Bool
	serveFn  func() error
	closeFn  func() error
}

type resultState struct {
	mu                    sync.Mutex
	serveWG               sync.WaitGroup
	sessions              []*liveSession
	rtts                  []time.Duration
	errors                []string
	edgeDistribution      map[string]int
	upstreamDistribution  map[string]map[string]int
	pings                 int
	pingFailures          int
	unexpectedDisconnects int
	identityCrossover     bool
}

type upstreamRecorder struct {
	base http.RoundTripper
	mu   sync.Mutex
	id   string
}

func (r *upstreamRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := r.base.RoundTrip(req)
	if err == nil {
		r.mu.Lock()
		r.id = resp.Header.Get("X-GizClaw-Gateway-Upstream")
		r.mu.Unlock()
	}
	return resp, err
}

func (r *upstreamRecorder) upstreamID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.id
}

func main() {
	opts, err := parseOptions()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	report, err := run(context.Background(), opts)
	if writeErr := writeArtifact(opts.artifactPath, report); writeErr != nil {
		fmt.Fprintln(os.Stderr, writeErr)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("gateway capacity passed: established=%d p99=%.2fms artifact=%s\n",
		report.Established, report.RTT.P99, opts.artifactPath)
}

func parseOptions() (options, error) {
	var rawEdges string
	opts := options{}
	flag.StringVar(&rawEdges, "edges", "", "comma-separated Edge HTTP endpoints")
	flag.IntVar(&opts.sessions, "sessions", 30000, "total logical client sessions")
	flag.DurationVar(&opts.ramp, "ramp", 5*time.Minute, "session establishment ramp")
	flag.DurationVar(&opts.duration, "duration", time.Minute, "hold duration after establishment")
	flag.DurationVar(&opts.pingInterval, "ping-interval", 30*time.Second, "active ping interval")
	flag.DurationVar(&opts.dialTimeout, "dial-timeout", 20*time.Second, "per-session dial timeout")
	flag.DurationVar(&opts.pingTimeout, "ping-timeout", 10*time.Second, "per-ping timeout")
	flag.IntVar(&opts.concurrency, "concurrency", 512, "maximum concurrent dial and ping operations")
	flag.StringVar(&opts.artifactPath, "artifact", "gateway-capacity.json", "capacity artifact path")
	flag.IntVar(&opts.maxEstablishmentFailures, "max-establishment-failures", 0, "accepted dial failures")
	flag.IntVar(&opts.maxPingFailures, "max-ping-failures", 0, "accepted ping failures")
	flag.DurationVar(&opts.maxP99RTT, "max-p99-rtt", 0, "optional maximum p99 ping RTT")
	flag.Parse()

	for edge := range strings.SplitSeq(rawEdges, ",") {
		edge = strings.TrimSpace(edge)
		if edge != "" {
			opts.edges = append(opts.edges, edge)
		}
	}
	switch {
	case len(opts.edges) == 0:
		return options{}, errors.New("-edges is required")
	case opts.sessions <= 0:
		return options{}, errors.New("-sessions must be positive")
	case opts.ramp < 0 || opts.duration < 0:
		return options{}, errors.New("-ramp and -duration must be non-negative")
	case opts.pingInterval <= 0:
		return options{}, errors.New("-ping-interval must be positive")
	case opts.dialTimeout <= 0 || opts.pingTimeout <= 0:
		return options{}, errors.New("-dial-timeout and -ping-timeout must be positive")
	case opts.concurrency <= 0:
		return options{}, errors.New("-concurrency must be positive")
	case strings.TrimSpace(opts.artifactPath) == "":
		return options{}, errors.New("-artifact is required")
	case opts.maxEstablishmentFailures < 0 || opts.maxPingFailures < 0 || opts.maxP99RTT < 0:
		return options{}, errors.New("failure and RTT thresholds must be non-negative")
	}
	return opts, nil
}

func run(ctx context.Context, opts options) (artifact, error) {
	started := time.Now()
	report := artifact{
		Version:   artifactVersion,
		StartedAt: started,
		Host: hostSummary{
			GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
			GoVersion: runtime.Version(), LogicalCPU: runtime.NumCPU(),
		},
		Config: artifactConfig{
			Edges: opts.edges, Sessions: opts.sessions, Ramp: opts.ramp,
			Duration: opts.duration, PingInterval: opts.pingInterval,
			DialTimeout: opts.dialTimeout, PingTimeout: opts.pingTimeout,
			Concurrency:              opts.concurrency,
			MaxEstablishmentFailures: opts.maxEstablishmentFailures,
			MaxPingFailures:          opts.maxPingFailures, MaxP99RTT: opts.maxP99RTT,
		},
		Attempted:            opts.sessions,
		EdgeDistribution:     make(map[string]int),
		UpstreamDistribution: make(map[string]map[string]int),
	}
	resources := newResourceSampler()
	defer resources.stop()

	edges, err := fetchEdges(ctx, opts.edges)
	if err != nil {
		report.Errors = []string{err.Error()}
		report.FinishedAt = time.Now()
		report.ResourceUsage = resources.summary()
		return report, err
	}
	state := &resultState{
		edgeDistribution:     report.EdgeDistribution,
		upstreamDistribution: report.UpstreamDistribution,
	}
	sem := make(chan struct{}, opts.concurrency)
	if err := establishSessions(ctx, opts, edges, state, sem, establish); err != nil {
		return finalize(report, state, resources), err
	}

	pingAll(ctx, state, opts, sem, 0)
	if opts.duration > 0 {
		deadline := time.NewTimer(opts.duration)
		ticker := time.NewTicker(opts.pingInterval)
		defer deadline.Stop()
		defer ticker.Stop()
		round := 1
		for {
			select {
			case <-ctx.Done():
				closeSessions(state)
				return finalize(report, state, resources), ctx.Err()
			case <-deadline.C:
				closeSessions(state)
				final := finalize(report, state, resources)
				return final, acceptanceError(final, opts)
			case <-ticker.C:
				pingAll(ctx, state, opts, sem, round)
				round++
			}
		}
	}
	closeSessions(state)
	final := finalize(report, state, resources)
	return final, acceptanceError(final, opts)
}

type establishSessionFunc func(
	context.Context,
	edgeMetadata,
	int,
	time.Duration,
) (*liveSession, error)

func establishSessions(
	ctx context.Context,
	opts options,
	edges []edgeMetadata,
	state *resultState,
	sem chan struct{},
	establishSession establishSessionFunc,
) error {
	var establishWG sync.WaitGroup
	delay := time.Duration(0)
	if opts.sessions > 1 {
		delay = opts.ramp / time.Duration(opts.sessions-1)
	}
	for i := 0; i < opts.sessions; i++ {
		if i > 0 && delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				establishWG.Wait()
				closeSessions(state)
				return ctx.Err()
			case <-timer.C:
			}
		}
		establishWG.Go(func() {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			edge := edges[i%len(edges)]
			session, dialErr := establishSession(ctx, edge, i, opts.dialTimeout)
			if dialErr != nil {
				state.recordError(fmt.Sprintf("session %d dial via %s: %v", i, edge.endpoint, dialErr))
				return
			}
			state.addSession(session)
			state.serveWG.Go(func() {
				err := session.serve()
				if !session.closed.Load() && err != nil {
					state.recordDisconnect(fmt.Sprintf("session %d disconnected: %v", i, err))
				}
			})
		})
	}
	establishWG.Wait()
	if err := ctx.Err(); err != nil {
		closeSessions(state)
		return err
	}
	return nil
}

func fetchEdges(ctx context.Context, endpoints []string) ([]edgeMetadata, error) {
	edges := make([]edgeMetadata, 0, len(endpoints))
	var serverKey giznet.PublicKey
	for _, endpoint := range endpoints {
		base, err := normalizeHTTPBase(endpoint)
		if err != nil {
			return nil, fmt.Errorf("edge %q: %w", endpoint, err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/server-info", nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("edge %q server-info: %w", endpoint, err)
		}
		var info apitypes.ServerInfo
		decodeErr := json.NewDecoder(resp.Body).Decode(&info)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("edge %q server-info status %s", endpoint, resp.Status)
		}
		if decodeErr != nil {
			return nil, fmt.Errorf("edge %q server-info: %w", endpoint, decodeErr)
		}
		if info.Transport == nil || info.Transport.Mode != "edge-gateway" {
			return nil, fmt.Errorf("edge %q does not advertise edge-gateway transport", endpoint)
		}
		if info.IceServers != nil {
			return nil, fmt.Errorf("edge %q retained authoritative ICE servers", endpoint)
		}
		var authoritative, transport giznet.PublicKey
		if err := authoritative.UnmarshalText([]byte(info.PublicKey)); err != nil {
			return nil, fmt.Errorf("edge %q authoritative public key: %w", endpoint, err)
		}
		if err := transport.UnmarshalText([]byte(info.Transport.PublicKey)); err != nil {
			return nil, fmt.Errorf("edge %q transport public key: %w", endpoint, err)
		}
		if len(edges) > 0 && !serverKey.Equal(authoritative) {
			return nil, errors.New("edges advertise different authoritative Server identities")
		}
		serverKey = authoritative
		transportBase, err := normalizeHTTPBase(info.Transport.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("edge %q transport endpoint: %w", endpoint, err)
		}
		path := info.Transport.SignalingPath
		if !strings.HasPrefix(path, "/") {
			return nil, fmt.Errorf("edge %q transport signaling path must be absolute", endpoint)
		}
		edges = append(edges, edgeMetadata{
			endpoint: endpoint, serverKey: authoritative, transportKey: transport,
			signalingURL: transportBase + path, authoritative: info,
		})
	}
	return edges, nil
}

func normalizeHTTPBase(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", errors.New("empty endpoint")
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "http://" + endpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return "", errors.New("invalid HTTP endpoint")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("unsupported endpoint scheme")
	}
	parsed.Path, parsed.RawQuery, parsed.Fragment = "", "", ""
	return strings.TrimSuffix(parsed.String(), "/"), nil
}

func establish(parent context.Context, edge edgeMetadata, index int, timeout time.Duration) (*liveSession, error) {
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		return nil, err
	}
	recorder := &upstreamRecorder{base: http.DefaultTransport}
	client := &gizcli.Client{
		KeyPair: key,
		DialTransport: func(key *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
			ctx, cancel := context.WithTimeout(parent, timeout)
			defer cancel()
			return gizwebrtc.Dial(ctx, key, edge.transportKey, gizwebrtc.DialConfig{
				SignalingURL:   edge.signalingURL,
				HTTPClient:     &http.Client{Transport: recorder, Timeout: timeout},
				SecurityPolicy: policy,
			})
		},
	}
	if err := client.Dial(edge.serverKey, edge.endpoint); err != nil {
		return nil, err
	}
	upstream := recorder.upstreamID()
	if upstream == "" {
		_ = client.Close()
		return nil, fmt.Errorf("session %d did not receive an upstream assignment", index)
	}
	return &liveSession{client: client, edge: edge.endpoint, upstream: upstream}, nil
}

func pingAll(ctx context.Context, state *resultState, opts options, sem chan struct{}, round int) {
	state.mu.Lock()
	sessions := append([]*liveSession(nil), state.sessions...)
	state.mu.Unlock()
	var wg sync.WaitGroup
	for index, session := range sessions {
		wg.Add(1)
		go func(index int, session *liveSession) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			pingCtx, cancel := context.WithTimeout(ctx, opts.pingTimeout)
			started := time.Now()
			id := fmt.Sprintf("gateway-capacity-%d-%d", round, index)
			_, err := session.client.Ping(pingCtx, id)
			cancel()
			state.recordPing(time.Since(started), err)
		}(index, session)
	}
	wg.Wait()
}

func (s *resultState) addSession(session *liveSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions = append(s.sessions, session)
	s.edgeDistribution[session.edge]++
	if s.upstreamDistribution[session.edge] == nil {
		s.upstreamDistribution[session.edge] = make(map[string]int)
	}
	s.upstreamDistribution[session.edge][session.upstream]++
}

func (s *liveSession) serve() error {
	if s == nil {
		return errors.New("nil live session")
	}
	if s.serveFn != nil {
		return s.serveFn()
	}
	if s.client == nil {
		return errors.New("live session has no client")
	}
	return s.client.Serve()
}

func (s *liveSession) close() error {
	if s == nil {
		return nil
	}
	s.closed.Store(true)
	if s.closeFn != nil {
		return s.closeFn()
	}
	if s.client == nil {
		return nil
	}
	return s.client.Close()
}

func (s *liveSession) peerConn() giznet.Conn {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.PeerConn()
}

func (s *resultState) recordPing(rtt time.Duration, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pings++
	if err == nil {
		s.rtts = append(s.rtts, rtt)
		return
	}
	s.pingFailures++
	if strings.Contains(err.Error(), "response id") {
		s.identityCrossover = true
	}
	s.appendErrorLocked("ping: " + err.Error())
}

func (s *resultState) recordError(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendErrorLocked(message)
}

func (s *resultState) recordDisconnect(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unexpectedDisconnects++
	s.appendErrorLocked(message)
}

func (s *resultState) appendErrorLocked(message string) {
	if len(s.errors) < 100 {
		s.errors = append(s.errors, message)
	}
}

func closeSessions(state *resultState) {
	state.mu.Lock()
	sessions := append([]*liveSession(nil), state.sessions...)
	state.mu.Unlock()
	for _, session := range sessions {
		if conn := session.peerConn(); conn != nil {
			if peer := conn.PeerInfo(); peer != nil {
				session.rxBytes = peer.RxBytes
				session.txBytes = peer.TxBytes
			}
		}
		_ = session.close()
	}
	state.serveWG.Wait()
}

func finalize(report artifact, state *resultState, resources *resourceSampler) artifact {
	state.mu.Lock()
	defer state.mu.Unlock()
	report.FinishedAt = time.Now()
	report.Established = len(state.sessions)
	report.EstablishmentFailures = report.Attempted - report.Established
	report.PingsAttempted = state.pings
	report.PingFailures = state.pingFailures
	report.UnexpectedDisconnects = state.unexpectedDisconnects
	report.IdentityCrossover = state.identityCrossover
	report.RTT = summarizeLatency(state.rtts)
	report.Errors = append([]string(nil), state.errors...)
	var rx, tx uint64
	for _, session := range state.sessions {
		sessionRx, sessionTx := session.rxBytes, session.txBytes
		if conn := session.peerConn(); conn != nil {
			peer := conn.PeerInfo()
			if peer != nil {
				sessionRx = peer.RxBytes
				sessionTx = peer.TxBytes
			}
		}
		rx += sessionRx
		tx += sessionTx
	}
	report.BytesPerSession.RxTotal = rx
	report.BytesPerSession.TxTotal = tx
	if report.Established > 0 {
		report.BytesPerSession.RxMean = float64(rx) / float64(report.Established)
		report.BytesPerSession.TxMean = float64(tx) / float64(report.Established)
	}
	report.ResourceUsage = resources.summary()
	report.Passed = report.EstablishmentFailures <= report.Config.MaxEstablishmentFailures &&
		report.PingFailures <= report.Config.MaxPingFailures &&
		report.UnexpectedDisconnects == 0 && !report.IdentityCrossover &&
		(report.Config.MaxP99RTT == 0 || time.Duration(report.RTT.P99*float64(time.Millisecond)) <= report.Config.MaxP99RTT)
	return report
}

func acceptanceError(report artifact, opts options) error {
	if report.Passed {
		return nil
	}
	return fmt.Errorf(
		"gateway capacity failed: established=%d/%d dial_failures=%d ping_failures=%d disconnects=%d crossover=%t p99=%.2fms thresholds=(%d,%d,%s)",
		report.Established, report.Attempted, report.EstablishmentFailures, report.PingFailures,
		report.UnexpectedDisconnects, report.IdentityCrossover, report.RTT.P99,
		opts.maxEstablishmentFailures, opts.maxPingFailures, opts.maxP99RTT,
	)
}

func summarizeLatency(values []time.Duration) latencySummary {
	if len(values) == 0 {
		return latencySummary{}
	}
	sorted := append([]time.Duration(nil), values...)
	slices.Sort(sorted)
	return latencySummary{
		Count: len(sorted),
		P50:   milliseconds(percentile(sorted, 0.50)),
		P95:   milliseconds(percentile(sorted, 0.95)),
		P99:   milliseconds(percentile(sorted, 0.99)),
		Max:   milliseconds(sorted[len(sorted)-1]),
	}
}

func percentile(sorted []time.Duration, quantile float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	index := int(math.Ceil(quantile*float64(len(sorted)))) - 1
	index = max(index, 0)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func milliseconds(value time.Duration) float64 {
	return float64(value) / float64(time.Millisecond)
}

func writeArtifact(path string, report artifact) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return err
	}
	return nil
}

type resourceSampler struct {
	done chan struct{}
	wg   sync.WaitGroup
	mu   sync.Mutex
	data resourceSummary
}

func newResourceSampler() *resourceSampler {
	s := &resourceSampler{done: make(chan struct{})}
	point := readResourcePoint()
	s.data.Start = point
	s.data.Peak = point
	s.wg.Go(func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.observe(readResourcePoint())
			case <-s.done:
				return
			}
		}
	})
	return s
}

func (s *resourceSampler) observe(point resourcePoint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	improved := false
	if point.RSSBytes > s.data.Peak.RSSBytes {
		s.data.Peak.RSSBytes = point.RSSBytes
		improved = true
	}
	if point.CPUSeconds > s.data.Peak.CPUSeconds {
		s.data.Peak.CPUSeconds = point.CPUSeconds
		improved = true
	}
	if point.OpenFDs > s.data.Peak.OpenFDs {
		s.data.Peak.OpenFDs = point.OpenFDs
		improved = true
	}
	if point.HeapAllocBytes > s.data.Peak.HeapAllocBytes {
		s.data.Peak.HeapAllocBytes = point.HeapAllocBytes
		improved = true
	}
	if point.Goroutines > s.data.Peak.Goroutines {
		s.data.Peak.Goroutines = point.Goroutines
		improved = true
	}
	if improved {
		s.data.Peak.At = point.At
	}
}

func (s *resourceSampler) stop() {
	select {
	case <-s.done:
		return
	default:
		close(s.done)
		s.wg.Wait()
	}
}

func (s *resourceSampler) summary() resourceSummary {
	point := readResourcePoint()
	s.observe(point)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.End = point
	return s.data
}

func readResourcePoint() resourcePoint {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	cpu := []metrics.Sample{{Name: "/cpu/classes/total:cpu-seconds"}}
	metrics.Read(cpu)
	cpuSeconds := 0.0
	if cpu[0].Value.Kind() == metrics.KindFloat64 {
		cpuSeconds = cpu[0].Value.Float64()
	}
	return resourcePoint{
		At: time.Now(), RSSBytes: readRSS(memory.Sys), CPUSeconds: cpuSeconds,
		OpenFDs: readFDCount(), HeapAllocBytes: memory.HeapAlloc,
		Goroutines: runtime.NumGoroutine(),
	}
}

func readRSS(fallback uint64) uint64 {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return fallback
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return fallback
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return fallback
	}
	return pages * uint64(os.Getpagesize())
}

func readFDCount() int {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		entries, err = os.ReadDir("/dev/fd")
	}
	if err != nil {
		return -1
	}
	return len(entries)
}
