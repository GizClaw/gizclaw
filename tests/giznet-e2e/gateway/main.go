// Command gateway-load exercises logical GizClaw sessions through one or more
// Edge gateways and writes a machine-readable capacity artifact.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"maps"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"runtime/metrics"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

const (
	artifactVersion = 7
	maxSpeedBytes   = int64(1 << 30)
)

type options struct {
	edges                    []string
	signalingBaseFromEdge    bool
	sessions                 int
	ramp                     time.Duration
	duration                 time.Duration
	pingInterval             time.Duration
	dialTimeout              time.Duration
	pingTimeout              time.Duration
	speedBytes               int64
	speedBaselineBytes       int64
	speedTimeout             time.Duration
	minSpeedAggregateRatio   float64
	minUploadAggregateMbps   float64
	minDownloadAggregateMbps float64
	minEstablishmentRate     float64
	maxDialP95               time.Duration
	maxDialP99               time.Duration
	concurrency              int
	artifactPath             string
	maxEstablishmentFailures int
	maxPingFailures          int
	maxP99RTT                time.Duration
	maxPingRoundDuration     time.Duration
	requireBalancedEdges     bool
	maxSessionsPerEdge       int
	requiredUpstreamsPerEdge int
	maxUpstreamsPerEdge      int
	maxSessionsPerUpstream   int
	dockerProject            string
	dockerComposeFile        string
	requireRoleResources     bool
	scenario                 string
	repetition               int
	soak                     bool
	analysisDir              string
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
	Establishment         establishmentSummary      `json:"establishment"`
	PingRounds            []pingRoundSummary        `json:"ping_rounds"`
	SpeedTest             speedTestSummary          `json:"speed_test"`
	BytesPerSession       byteSummary               `json:"bytes_per_session"`
	EdgeDistribution      map[string]int            `json:"edge_distribution"`
	UpstreamDistribution  map[string]map[string]int `json:"upstream_distribution"`
	ResourceUsage         resourceSummary           `json:"resource_usage"`
	Extended              *extendedRunEvidence      `json:"extended,omitempty"`
	Errors                []string                  `json:"errors,omitempty"`
	Passed                bool                      `json:"passed"`
}

type hostSummary struct {
	GOOS       string `json:"goos"`
	GOARCH     string `json:"goarch"`
	GoVersion  string `json:"go_version"`
	LogicalCPU int    `json:"logical_cpu"`
	GOMAXPROCS int    `json:"go_max_procs"`
}

type artifactConfig struct {
	Edges                    []string      `json:"edges"`
	SignalingBaseFromEdge    bool          `json:"signaling_base_from_edge"`
	Sessions                 int           `json:"sessions"`
	Ramp                     time.Duration `json:"ramp"`
	Duration                 time.Duration `json:"duration"`
	PingInterval             time.Duration `json:"ping_interval"`
	DialTimeout              time.Duration `json:"dial_timeout"`
	PingTimeout              time.Duration `json:"ping_timeout"`
	SpeedBytes               int64         `json:"speed_bytes"`
	SpeedBaselineBytes       int64         `json:"speed_baseline_bytes"`
	SpeedTimeout             time.Duration `json:"speed_timeout"`
	MinSpeedAggregateRatio   float64       `json:"min_speed_aggregate_ratio"`
	MinUploadAggregateMbps   float64       `json:"min_upload_aggregate_mbps"`
	MinDownloadAggregateMbps float64       `json:"min_download_aggregate_mbps"`
	MinEstablishmentRate     float64       `json:"min_establishment_rate"`
	MaxDialP95               time.Duration `json:"max_dial_p95"`
	MaxDialP99               time.Duration `json:"max_dial_p99"`
	Concurrency              int           `json:"concurrency"`
	MaxEstablishmentFailures int           `json:"max_establishment_failures"`
	MaxPingFailures          int           `json:"max_ping_failures"`
	MaxP99RTT                time.Duration `json:"max_p99_rtt"`
	MaxPingRoundDuration     time.Duration `json:"max_ping_round_duration"`
	RequireBalancedEdges     bool          `json:"require_balanced_edges"`
	MaxSessionsPerEdge       int           `json:"max_sessions_per_edge"`
	RequiredUpstreamsPerEdge int           `json:"required_upstreams_per_edge"`
	MaxUpstreamsPerEdge      int           `json:"max_upstreams_per_edge"`
	MaxSessionsPerUpstream   int           `json:"max_sessions_per_upstream"`
	Scenario                 string        `json:"scenario,omitempty"`
	Repetition               int           `json:"repetition,omitempty"`
	Soak                     bool          `json:"soak,omitempty"`
}

type latencySummary struct {
	Count int     `json:"count"`
	P50   float64 `json:"p50"`
	P95   float64 `json:"p95"`
	P99   float64 `json:"p99"`
	Max   float64 `json:"max"`
}

type pingRoundSummary struct {
	Phase            string                               `json:"phase"`
	Round            int                                  `json:"round"`
	StartedAt        time.Time                            `json:"started_at"`
	Duration         time.Duration                        `json:"duration"`
	Attempted        int                                  `json:"attempted"`
	Failures         int                                  `json:"failures"`
	RTT              latencySummary                       `json:"rtt_ms"`
	EdgeRTT          map[string]latencySummary            `json:"edge_rtt_ms"`
	EdgeFailures     map[string]int                       `json:"edge_failures"`
	UpstreamRTT      map[string]map[string]latencySummary `json:"upstream_rtt_ms"`
	UpstreamFailures map[string]map[string]int            `json:"upstream_failures"`
}

type speedTestSummary struct {
	Upload   speedDirectionSummary `json:"upload"`
	Download speedDirectionSummary `json:"download"`
}

type speedDirectionSummary struct {
	Direction                 string          `json:"direction"`
	BaselineBytesPerSession   int64           `json:"baseline_bytes_per_session"`
	ConcurrentBytesPerSession int64           `json:"concurrent_bytes_per_session"`
	Baseline                  speedRunSummary `json:"baseline"`
	Concurrent                speedRunSummary `json:"concurrent"`
	AggregateToBaselineRatio  float64         `json:"aggregate_to_baseline_ratio"`
	Passed                    bool            `json:"passed"`
}

type speedRunSummary struct {
	StartedAt        time.Time                              `json:"started_at"`
	Duration         time.Duration                          `json:"duration"`
	Attempted        int                                    `json:"attempted"`
	Completed        int                                    `json:"completed"`
	Failures         int                                    `json:"failures"`
	RequestedBytes   int64                                  `json:"requested_bytes"`
	TransferredBytes int64                                  `json:"transferred_bytes"`
	AggregateMbps    float64                                `json:"aggregate_mbps"`
	PerSessionMbps   rateSummary                            `json:"per_session_mbps"`
	Edge             map[string]speedPathSummary            `json:"edge"`
	Upstream         map[string]map[string]speedPathSummary `json:"upstream"`
	Sessions         []speedSessionResult                   `json:"sessions"`
}

type speedPathSummary struct {
	Attempted        int         `json:"attempted"`
	Completed        int         `json:"completed"`
	Failures         int         `json:"failures"`
	TransferredBytes int64       `json:"transferred_bytes"`
	AggregateMbps    float64     `json:"aggregate_mbps"`
	PerSessionMbps   rateSummary `json:"per_session_mbps"`
}

type speedSessionResult struct {
	Index    int           `json:"index"`
	Edge     string        `json:"edge"`
	Upstream string        `json:"upstream"`
	Bytes    int64         `json:"bytes"`
	Duration time.Duration `json:"duration"`
	Mbps     float64       `json:"mbps"`
	Error    string        `json:"error,omitempty"`
}

type rateSummary struct {
	Count int     `json:"count"`
	Min   float64 `json:"min"`
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
	At               time.Time `json:"at"`
	RSSBytes         uint64    `json:"rss_bytes"`
	RSSSource        string    `json:"rss_source"`
	CPUSeconds       float64   `json:"cpu_seconds"`
	CPUSecondsSource string    `json:"cpu_seconds_source"`
	OpenFDs          int       `json:"open_fds"`
	OpenFDsSource    string    `json:"open_fds_source,omitempty"`
	HeapAllocBytes   uint64    `json:"heap_alloc_bytes"`
	Goroutines       int       `json:"goroutines"`
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
	speedFn  func(context.Context, string, rpcapi.SpeedTestRequest) (gizcli.SpeedTestResult, error)
}

type resultState struct {
	mu                    sync.Mutex
	serveWG               sync.WaitGroup
	sessions              []*liveSession
	rtts                  []time.Duration
	pingRounds            []pingRoundSummary
	establishment         establishmentSummary
	speedTest             speedTestSummary
	errors                []string
	edgeDistribution      map[string]int
	upstreamDistribution  map[string]map[string]int
	pings                 int
	pingFailures          int
	unexpectedDisconnects int
	identityCrossover     bool
}

type upstreamRecorder struct {
	base         http.RoundTripper
	mu           sync.Mutex
	id           string
	duration     time.Duration
	serverPhases map[string]time.Duration
}

func (r *upstreamRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	started := time.Now()
	resp, err := r.base.RoundTrip(req)
	duration := time.Since(started)
	r.mu.Lock()
	r.duration += duration
	if err == nil {
		r.id = resp.Header.Get("X-GizClaw-Gateway-Upstream")
		for phase, phaseDuration := range parseServerTiming(resp.Header.Get("Server-Timing")) {
			if r.serverPhases == nil {
				r.serverPhases = make(map[string]time.Duration)
			}
			r.serverPhases[phase] += phaseDuration
		}
	}
	r.mu.Unlock()
	return resp, err
}

func (r *upstreamRecorder) upstreamID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.id
}

func (r *upstreamRecorder) signalingDuration() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.duration
}

func (r *upstreamRecorder) signalingPhases() map[string]time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return maps.Clone(r.serverPhases)
}

func parseServerTiming(header string) map[string]time.Duration {
	phases := make(map[string]time.Duration)
	for metric := range strings.SplitSeq(header, ",") {
		parts := strings.Split(strings.TrimSpace(metric), ";")
		phase, ok := serverTimingPhases[strings.TrimSpace(parts[0])]
		if !ok {
			continue
		}
		for _, parameter := range parts[1:] {
			name, value, found := strings.Cut(strings.TrimSpace(parameter), "=")
			if !found || name != "dur" {
				continue
			}
			milliseconds, err := strconv.ParseFloat(value, 64)
			if err != nil || milliseconds < 0 || math.IsInf(milliseconds, 0) || math.IsNaN(milliseconds) {
				continue
			}
			phases[phase] = durationFromMilliseconds(milliseconds)
		}
	}
	return phases
}

func main() {
	opts, err := parseOptions()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if opts.analysisDir != "" {
		report, analyzeErr := analyzeCapacityArtifacts(opts.analysisDir)
		if analyzeErr != nil {
			fmt.Fprintln(os.Stderr, analyzeErr)
			os.Exit(1)
		}
		if writeErr := writeProjectionArtifact(opts.artifactPath, report); writeErr != nil {
			fmt.Fprintln(os.Stderr, writeErr)
			os.Exit(1)
		}
		fmt.Printf("gateway capacity analysis complete: qualified=%t artifact=%s\n", report.Qualified, opts.artifactPath)
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	report, err := run(ctx, opts)
	if writeErr := writeArtifact(opts.artifactPath, report); writeErr != nil {
		fmt.Fprintln(os.Stderr, writeErr)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf(
		"gateway capacity passed: established=%d establishment=%.2f sessions/s dial_p95=%.2fms dial_p99=%.2fms rtt_p99=%.2fms upload=%.2fMbps download=%.2fMbps artifact=%s\n",
		report.Established,
		report.Establishment.UsableSessionsPerSecond,
		report.Establishment.Dial.P95,
		report.Establishment.Dial.P99,
		report.RTT.P99,
		report.SpeedTest.Upload.Concurrent.AggregateMbps,
		report.SpeedTest.Download.Concurrent.AggregateMbps,
		opts.artifactPath,
	)
}

func parseOptions() (options, error) {
	var rawEdges string
	opts := options{}
	flag.StringVar(&rawEdges, "edges", "", "comma-separated Edge HTTP endpoints")
	flag.BoolVar(&opts.signalingBaseFromEdge, "signaling-base-from-edge", false, "use each -edges address as the signaling base instead of the advertised transport endpoint")
	flag.IntVar(&opts.sessions, "sessions", 30000, "total logical client sessions")
	flag.DurationVar(&opts.ramp, "ramp", 5*time.Minute, "session establishment ramp")
	flag.DurationVar(&opts.duration, "duration", time.Minute, "hold duration after establishment")
	flag.DurationVar(&opts.pingInterval, "ping-interval", 30*time.Second, "active ping interval")
	flag.DurationVar(&opts.dialTimeout, "dial-timeout", 20*time.Second, "per-session dial timeout")
	flag.DurationVar(&opts.pingTimeout, "ping-timeout", 10*time.Second, "per-ping timeout")
	flag.Int64Var(&opts.speedBytes, "speed-bytes", 0, "bytes transferred per session and direction; zero disables throughput testing")
	flag.Int64Var(&opts.speedBaselineBytes, "speed-baseline-bytes", 0, "bytes transferred for each single-session baseline; zero uses -speed-bytes")
	flag.DurationVar(&opts.speedTimeout, "speed-timeout", 2*time.Minute, "timeout for each baseline or concurrent throughput run")
	flag.Float64Var(&opts.minSpeedAggregateRatio, "min-speed-aggregate-ratio", 0, "minimum concurrent aggregate Mbps divided by single-session baseline Mbps")
	flag.Float64Var(&opts.minUploadAggregateMbps, "min-upload-aggregate-mbps", 0, "minimum concurrent upload aggregate Mbps")
	flag.Float64Var(&opts.minDownloadAggregateMbps, "min-download-aggregate-mbps", 0, "minimum concurrent download aggregate Mbps")
	flag.Float64Var(&opts.minEstablishmentRate, "min-establishment-rate", 0, "minimum usable sessions established per second")
	flag.DurationVar(&opts.maxDialP95, "max-dial-p95", 0, "optional maximum p95 usable-session Dial duration")
	flag.DurationVar(&opts.maxDialP99, "max-dial-p99", 0, "optional maximum p99 usable-session Dial duration")
	flag.IntVar(&opts.concurrency, "concurrency", 512, "maximum concurrent dial and ping operations")
	flag.StringVar(&opts.artifactPath, "artifact", "gateway-capacity.json", "capacity artifact path")
	flag.IntVar(&opts.maxEstablishmentFailures, "max-establishment-failures", 0, "accepted dial failures")
	flag.IntVar(&opts.maxPingFailures, "max-ping-failures", 0, "accepted ping failures")
	flag.DurationVar(&opts.maxP99RTT, "max-p99-rtt", 0, "optional maximum p99 ping RTT")
	flag.DurationVar(&opts.maxPingRoundDuration, "max-ping-round-duration", 0, "optional maximum complete ping-round duration")
	flag.BoolVar(&opts.requireBalancedEdges, "require-balanced-edges", false, "require equal session distribution across all Edges")
	flag.IntVar(&opts.maxSessionsPerEdge, "max-sessions-per-edge", 0, "optional configured session limit for each Edge")
	flag.IntVar(&opts.requiredUpstreamsPerEdge, "required-upstreams-per-edge", 0, "optional exact observed upstream associations required per Edge")
	flag.IntVar(&opts.maxUpstreamsPerEdge, "max-upstreams-per-edge", 0, "optional maximum observed upstream associations per Edge")
	flag.IntVar(&opts.maxSessionsPerUpstream, "max-sessions-per-upstream", 0, "optional maximum observed sessions per upstream")
	flag.StringVar(&opts.dockerProject, "docker-project", "", "Docker Compose project for external role sampling")
	flag.StringVar(&opts.dockerComposeFile, "docker-compose-file", "", "Docker Compose file for external role sampling")
	flag.BoolVar(&opts.requireRoleResources, "require-role-resources", false, "require load-driver, Edge, and Server process resource samples")
	flag.StringVar(&opts.scenario, "scenario", "", "extended-capacity scenario identifier")
	flag.IntVar(&opts.repetition, "repetition", 0, "one-based extended-capacity repetition")
	flag.BoolVar(&opts.soak, "soak", false, "mark this run as the long soak")
	flag.StringVar(&opts.analysisDir, "analyze-dir", "", "analyze extended run artifacts in this directory")
	flag.Parse()
	if opts.analysisDir != "" {
		if strings.TrimSpace(opts.artifactPath) == "" {
			return options{}, errors.New("-artifact is required")
		}
		return opts, nil
	}

	for edge := range strings.SplitSeq(rawEdges, ",") {
		edge = strings.TrimSpace(edge)
		if edge != "" {
			opts.edges = append(opts.edges, edge)
		}
	}
	if err := validateOptions(opts); err != nil {
		return options{}, err
	}
	if opts.speedBytes > 0 && opts.speedBaselineBytes == 0 {
		opts.speedBaselineBytes = opts.speedBytes
	}
	return opts, nil
}

func validateOptions(opts options) error {
	switch {
	case len(opts.edges) == 0:
		return errors.New("-edges is required")
	case opts.sessions <= 0:
		return errors.New("-sessions must be positive")
	case opts.ramp < 0 || opts.duration < 0:
		return errors.New("-ramp and -duration must be non-negative")
	case opts.pingInterval <= 0:
		return errors.New("-ping-interval must be positive")
	case opts.dialTimeout <= 0 || opts.pingTimeout <= 0 || opts.speedTimeout <= 0:
		return errors.New("-dial-timeout, -ping-timeout, and -speed-timeout must be positive")
	case opts.speedBytes < 0:
		return errors.New("-speed-bytes must be non-negative")
	case opts.speedBytes > maxSpeedBytes:
		return fmt.Errorf("-speed-bytes must not exceed %d", maxSpeedBytes)
	case opts.speedBaselineBytes < 0 || opts.speedBaselineBytes > maxSpeedBytes:
		return fmt.Errorf("-speed-baseline-bytes must be between 0 and %d", maxSpeedBytes)
	case opts.speedBytes > 0 && int64(opts.sessions) > math.MaxInt64/opts.speedBytes:
		return errors.New("-sessions and -speed-bytes overflow aggregate byte accounting")
	case opts.speedBytes == 0 && opts.speedBaselineBytes > 0:
		return errors.New("-speed-baseline-bytes requires positive -speed-bytes")
	case !nonNegativeFinite(opts.minSpeedAggregateRatio) ||
		!nonNegativeFinite(opts.minUploadAggregateMbps) ||
		!nonNegativeFinite(opts.minDownloadAggregateMbps) ||
		!nonNegativeFinite(opts.minEstablishmentRate):
		return errors.New("speed and establishment-rate thresholds must be finite and non-negative")
	case opts.speedBytes == 0 &&
		(opts.minSpeedAggregateRatio > 0 ||
			opts.minUploadAggregateMbps > 0 ||
			opts.minDownloadAggregateMbps > 0):
		return errors.New("speed thresholds require positive -speed-bytes")
	case opts.concurrency <= 0:
		return errors.New("-concurrency must be positive")
	case opts.maxDialP95 < 0 || opts.maxDialP99 < 0:
		return errors.New("-max-dial-p95 and -max-dial-p99 must be non-negative")
	case opts.maxDialP95 > 0 && opts.maxDialP99 > 0 && opts.maxDialP95 > opts.maxDialP99:
		return errors.New("-max-dial-p95 must not exceed -max-dial-p99")
	case strings.TrimSpace(opts.artifactPath) == "":
		return errors.New("-artifact is required")
	case opts.maxEstablishmentFailures < 0 || opts.maxPingFailures < 0 || opts.maxP99RTT < 0:
		return errors.New("failure and RTT thresholds must be non-negative")
	case opts.maxPingRoundDuration < 0:
		return errors.New("-max-ping-round-duration must be non-negative")
	case opts.maxPingRoundDuration > opts.pingInterval:
		return errors.New("-max-ping-round-duration must not exceed -ping-interval")
	case opts.maxPingRoundDuration > 0 && opts.pingTimeout >= opts.maxPingRoundDuration:
		return errors.New("-ping-timeout must be less than -max-ping-round-duration")
	case opts.maxSessionsPerEdge < 0 || opts.requiredUpstreamsPerEdge < 0 ||
		opts.maxUpstreamsPerEdge < 0 || opts.maxSessionsPerUpstream < 0:
		return errors.New("Edge and upstream limits must be non-negative")
	case opts.maxUpstreamsPerEdge > 0 && opts.requiredUpstreamsPerEdge > opts.maxUpstreamsPerEdge:
		return errors.New("-required-upstreams-per-edge must not exceed -max-upstreams-per-edge")
	case opts.requireRoleResources && (strings.TrimSpace(opts.dockerProject) == "" || strings.TrimSpace(opts.dockerComposeFile) == ""):
		return errors.New("-require-role-resources requires -docker-project and -docker-compose-file")
	case opts.requireRoleResources && (strings.TrimSpace(opts.scenario) == "" || opts.repetition <= 0):
		return errors.New("-require-role-resources requires -scenario and positive -repetition")
	}
	return nil
}

func nonNegativeFinite(value float64) bool {
	return value >= 0 && !math.IsInf(value, 0)
}

func run(ctx context.Context, opts options) (artifact, error) {
	started := time.Now()
	report := artifact{
		Version:   artifactVersion,
		StartedAt: started,
		Host: hostSummary{
			GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
			GoVersion: runtime.Version(), LogicalCPU: runtime.NumCPU(),
			GOMAXPROCS: runtime.GOMAXPROCS(0),
		},
		Config: artifactConfig{
			Edges: opts.edges, SignalingBaseFromEdge: opts.signalingBaseFromEdge,
			Sessions: opts.sessions, Ramp: opts.ramp,
			Duration: opts.duration, PingInterval: opts.pingInterval,
			DialTimeout: opts.dialTimeout, PingTimeout: opts.pingTimeout,
			SpeedBytes: opts.speedBytes, SpeedBaselineBytes: opts.speedBaselineBytes,
			SpeedTimeout:             opts.speedTimeout,
			MinSpeedAggregateRatio:   opts.minSpeedAggregateRatio,
			MinUploadAggregateMbps:   opts.minUploadAggregateMbps,
			MinDownloadAggregateMbps: opts.minDownloadAggregateMbps,
			MinEstablishmentRate:     opts.minEstablishmentRate,
			MaxDialP95:               opts.maxDialP95,
			MaxDialP99:               opts.maxDialP99,
			Concurrency:              opts.concurrency,
			MaxEstablishmentFailures: opts.maxEstablishmentFailures,
			MaxPingFailures:          opts.maxPingFailures,
			MaxP99RTT:                opts.maxP99RTT,
			MaxPingRoundDuration:     opts.maxPingRoundDuration,
			RequireBalancedEdges:     opts.requireBalancedEdges,
			MaxSessionsPerEdge:       opts.maxSessionsPerEdge,
			RequiredUpstreamsPerEdge: opts.requiredUpstreamsPerEdge,
			MaxUpstreamsPerEdge:      opts.maxUpstreamsPerEdge,
			MaxSessionsPerUpstream:   opts.maxSessionsPerUpstream,
			Scenario:                 opts.scenario,
			Repetition:               opts.repetition,
			Soak:                     opts.soak,
		},
		Attempted:            opts.sessions,
		EdgeDistribution:     make(map[string]int),
		UpstreamDistribution: make(map[string]map[string]int),
	}
	resources := newResourceSampler(opts.requireRoleResources)
	defer resources.stop()
	var extended *extendedSamplerState
	if opts.requireRoleResources {
		report.Version = extendedArtifactVersion
		var err error
		extended, err = startExtendedSampler(ctx, opts.dockerProject, opts.dockerComposeFile)
		if err != nil {
			report.Errors = []string{err.Error()}
			report.FinishedAt = time.Now()
			report.ResourceUsage = resources.summary()
			return report, err
		}
	}

	edges, err := fetchEdges(ctx, opts.edges, opts.signalingBaseFromEdge)
	if err != nil {
		report.Errors = []string{err.Error()}
		report.FinishedAt = time.Now()
		report.ResourceUsage = resources.summary()
		if extended != nil {
			report.Extended = extended.finish(context.Background(), resources)
			report.Errors = append(report.Errors, report.Extended.Errors...)
		}
		return report, err
	}
	state := &resultState{
		edgeDistribution:     report.EdgeDistribution,
		upstreamDistribution: report.UpstreamDistribution,
	}
	sem := make(chan struct{}, opts.concurrency)
	if err := establishSessions(ctx, opts, edges, state, sem, establish); err != nil {
		return finalize(report, state, resources, extended), err
	}

	pingAll(ctx, state, opts, sem, "hold", 0)
	if opts.speedBytes > 0 {
		runSpeedTests(ctx, state, opts)
	}
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
				return finalize(report, state, resources, extended), ctx.Err()
			case <-deadline.C:
				closeSessions(state)
				final := finalize(report, state, resources, extended)
				return final, acceptanceError(final, opts)
			case <-ticker.C:
				pingAll(ctx, state, opts, sem, "hold", round)
				round++
			}
		}
	}
	closeSessions(state)
	final := finalize(report, state, resources, extended)
	return final, acceptanceError(final, opts)
}

type establishSessionFunc func(
	context.Context,
	edgeMetadata,
	int,
	time.Duration,
) (*liveSession, establishmentSessionResult, error)

func establishSessions(
	ctx context.Context,
	opts options,
	edges []edgeMetadata,
	state *resultState,
	sem chan struct{},
	establishSession establishSessionFunc,
) error {
	var establishWG sync.WaitGroup
	var attemptsMu sync.Mutex
	attempts := make([]establishmentSessionResult, 0, opts.sessions)
	stopRampPings := func() {}
	if opts.requireRoleResources && opts.ramp > 0 {
		stopRampPings = startRampPings(ctx, state, opts, sem)
	}
	startedAt := time.Now()
	start := make(chan struct{})
	var ready sync.WaitGroup
	if opts.ramp == 0 {
		ready.Add(opts.sessions)
	}
	launch := func(i int) {
		establishWG.Go(func() {
			if opts.ramp == 0 {
				ready.Done()
				select {
				case <-start:
				case <-ctx.Done():
					return
				}
			}
			attempt := establishmentSessionResult{
				Index: i,
				Edge:  edges[i%len(edges)].endpoint,
			}
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				attempt.StartedAt = time.Now()
				attempt.Error = ctx.Err().Error()
				attemptsMu.Lock()
				attempts = append(attempts, attempt)
				attemptsMu.Unlock()
				return
			}
			defer func() { <-sem }()
			attempt.StartedAt = time.Now()
			edge := edges[i%len(edges)]
			session, timing, dialErr := establishSession(ctx, edge, i, opts.dialTimeout)
			attempt.Duration = time.Since(attempt.StartedAt)
			attempt.Upstream = timing.Upstream
			attempt.DialDuration = timing.DialDuration
			attempt.Phases = timing.Phases
			if dialErr != nil {
				attempt.Error = dialErr.Error()
			}
			attemptsMu.Lock()
			attempts = append(attempts, attempt)
			attemptsMu.Unlock()
			if dialErr != nil {
				state.recordError(fmt.Sprintf("session %d dial via %s: %v", i, edge.endpoint, dialErr))
				return
			}
			state.addSession(session)
			state.serveWG.Go(func() {
				err := session.serve()
				handleSessionServeExit(state, session, i, err)
			})
		})
	}
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
				stopRampPings()
				establishWG.Wait()
				state.mu.Lock()
				state.establishment = summarizeEstablishment(startedAt, time.Now(), attempts)
				state.mu.Unlock()
				closeSessions(state)
				return ctx.Err()
			case <-timer.C:
			}
		}
		launch(i)
	}
	if opts.ramp == 0 {
		ready.Wait()
		startedAt = time.Now()
		close(start)
	}
	establishWG.Wait()
	stopRampPings()
	state.mu.Lock()
	state.establishment = summarizeEstablishment(startedAt, time.Now(), attempts)
	state.mu.Unlock()
	if err := ctx.Err(); err != nil {
		closeSessions(state)
		return err
	}
	return nil
}

func startRampPings(ctx context.Context, state *resultState, opts options, sem chan struct{}) func() {
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		ticker := time.NewTicker(opts.pingInterval)
		defer ticker.Stop()
		round := 0
		for {
			select {
			case <-ticker.C:
				pingAll(ctx, state, opts, sem, "ramp", round)
				round++
			case <-done:
				return
			case <-ctx.Done():
				return
			}
		}
	})
	var once sync.Once
	return func() {
		once.Do(func() { close(done) })
		wg.Wait()
	}
}

func handleSessionServeExit(state *resultState, session *liveSession, index int, err error) {
	if session.closed.Load() {
		return
	}
	message := fmt.Sprintf("session %d disconnected", index)
	if err != nil {
		message += ": " + err.Error()
	}
	state.recordDisconnect(message)
}

func fetchEdges(ctx context.Context, endpoints []string, signalingBaseFromEdge bool) ([]edgeMetadata, error) {
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
		for _, existing := range edges {
			if existing.transportKey.Equal(transport) {
				return nil, fmt.Errorf(
					"edge %q duplicates transport identity from %q",
					endpoint,
					existing.endpoint,
				)
			}
		}
		serverKey = authoritative
		transportBase, err := normalizeHTTPBase(info.Transport.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("edge %q transport endpoint: %w", endpoint, err)
		}
		if signalingBaseFromEdge {
			transportBase = base
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

func establish(
	parent context.Context,
	edge edgeMetadata,
	index int,
	timeout time.Duration,
) (*liveSession, establishmentSessionResult, error) {
	timing := establishmentSessionResult{Phases: make(map[string]time.Duration)}
	keyStarted := time.Now()
	key, err := giznet.GenerateKeyPair()
	timing.Phases[phaseKeyGeneration] = time.Since(keyStarted)
	if err != nil {
		return nil, timing, err
	}
	recorder := &upstreamRecorder{base: http.DefaultTransport}
	var transportDuration time.Duration
	var clientTiming gizwebrtc.DialTiming
	client := &gizcli.Client{
		KeyPair: key,
		DialTransport: func(key *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
			ctx, cancel := context.WithTimeout(parent, timeout)
			defer cancel()
			transportStarted := time.Now()
			listener, conn, dialErr := gizwebrtc.Dial(ctx, key, edge.transportKey, gizwebrtc.DialConfig{
				SignalingURL:   edge.signalingURL,
				HTTPClient:     &http.Client{Transport: recorder, Timeout: timeout},
				SecurityPolicy: policy,
				OnTiming: func(observation gizwebrtc.DialTiming) {
					clientTiming = observation
				},
			})
			transportDuration = time.Since(transportStarted)
			return listener, conn, dialErr
		},
	}
	clientDialStarted := time.Now()
	if err := client.Dial(edge.serverKey, edge.endpoint); err != nil {
		timing.DialDuration = time.Since(clientDialStarted)
		recordEstablishmentPhases(&timing, transportDuration, recorder, clientTiming)
		return nil, timing, err
	}
	timing.DialDuration = time.Since(clientDialStarted)
	recordEstablishmentPhases(&timing, transportDuration, recorder, clientTiming)
	upstream := recorder.upstreamID()
	timing.Upstream = upstream
	if upstream == "" {
		_ = client.Close()
		return nil, timing, fmt.Errorf("session %d did not receive an upstream assignment", index)
	}
	return &liveSession{client: client, edge: edge.endpoint, upstream: upstream}, timing, nil
}

func recordEstablishmentPhases(
	timing *establishmentSessionResult,
	transportDuration time.Duration,
	recorder *upstreamRecorder,
	clientTiming gizwebrtc.DialTiming,
) {
	if timing == nil || recorder == nil {
		return
	}
	timing.Phases[phaseClientDial] = timing.DialDuration
	timing.Phases[phaseTransportDial] = transportDuration
	timing.Phases[phaseHTTPSignaling] = recorder.signalingDuration()
	timing.Phases[phaseTransportOther] = max(transportDuration-recorder.signalingDuration(), 0)
	timing.Phases[phaseMandatoryEventStream] = max(timing.Phases[phaseClientDial]-transportDuration, 0)
	timing.Phases[phaseClientPeerConnection] = clientTiming.PeerConnectionConstruction
	timing.Phases[phaseClientOfferCreation] = clientTiming.OfferCreation
	timing.Phases[phaseClientSetLocal] = clientTiming.SetLocalDescription
	timing.Phases[phaseClientICEGathering] = clientTiming.ICEGathering
	timing.Phases[phaseClientSetRemote] = clientTiming.SetRemoteDescription
	timing.Phases[phaseClientICEConnected] = clientTiming.ICEConnected
	timing.Phases[phaseClientDTLSConnected] = clientTiming.DTLSConnected
	timing.Phases[phaseClientDataChannel] = clientTiming.DataChannelReady
	maps.Copy(timing.Phases, recorder.signalingPhases())
}

func pingAll(ctx context.Context, state *resultState, opts options, sem chan struct{}, phase string, round int) {
	state.mu.Lock()
	sessions := append([]*liveSession(nil), state.sessions...)
	state.mu.Unlock()
	started := time.Now()
	var roundMu sync.Mutex
	roundRTTs := make([]time.Duration, 0, len(sessions))
	edgeRTTs := make(map[string][]time.Duration)
	edgeFailures := make(map[string]int)
	upstreamRTTs := make(map[string]map[string][]time.Duration)
	upstreamFailures := make(map[string]map[string]int)
	sessionOffset := 0
	for _, batch := range pingSessionBatches(sessions, opts.concurrency) {
		var wg sync.WaitGroup
		for batchIndex, session := range batch {
			index := sessionOffset + batchIndex
			wg.Add(1)
			go func(index int, session *liveSession) {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					err := ctx.Err()
					state.recordPing(0, err)
					roundMu.Lock()
					edgeFailures[session.edge]++
					if upstreamFailures[session.edge] == nil {
						upstreamFailures[session.edge] = make(map[string]int)
					}
					upstreamFailures[session.edge][session.upstream]++
					roundMu.Unlock()
					return
				}
				defer func() { <-sem }()
				pingCtx, cancel := context.WithTimeout(ctx, opts.pingTimeout)
				pingStarted := time.Now()
				id := fmt.Sprintf("gateway-capacity-%s-%d-%d", phase, round, index)
				rxBefore, txBefore := session.byteCounts()
				_, err := session.client.Ping(pingCtx, id)
				rxAfter, txAfter := session.byteCounts()
				rtt := time.Since(pingStarted)
				cancel()
				if err != nil {
					err = fmt.Errorf(
						"id=%s edge=%s upstream=%s rx_delta=%d tx_delta=%d: %w",
						id,
						session.edge,
						session.upstream,
						counterDelta(rxBefore, rxAfter),
						counterDelta(txBefore, txAfter),
						err,
					)
				}
				state.recordPing(rtt, err)
				roundMu.Lock()
				if upstreamRTTs[session.edge] == nil {
					upstreamRTTs[session.edge] = make(map[string][]time.Duration)
				}
				if upstreamFailures[session.edge] == nil {
					upstreamFailures[session.edge] = make(map[string]int)
				}
				if err == nil {
					roundRTTs = append(roundRTTs, rtt)
					edgeRTTs[session.edge] = append(edgeRTTs[session.edge], rtt)
					upstreamRTTs[session.edge][session.upstream] = append(
						upstreamRTTs[session.edge][session.upstream],
						rtt,
					)
				} else {
					edgeFailures[session.edge]++
					upstreamFailures[session.edge][session.upstream]++
				}
				roundMu.Unlock()
			}(index, session)
		}
		wg.Wait()
		sessionOffset += len(batch)
	}
	state.recordPingRound(pingRoundSummary{
		Phase:            phase,
		Round:            round,
		StartedAt:        started,
		Duration:         time.Since(started),
		Attempted:        len(sessions),
		Failures:         countNested(upstreamFailures),
		RTT:              summarizeLatency(roundRTTs),
		EdgeRTT:          summarizeLatencyMap(edgeRTTs),
		EdgeFailures:     edgeFailures,
		UpstreamRTT:      summarizeNestedLatencyMap(upstreamRTTs),
		UpstreamFailures: upstreamFailures,
	})
}

func pingSessionBatches(sessions []*liveSession, concurrency int) [][]*liveSession {
	if concurrency <= 0 {
		return nil
	}
	batches := make([][]*liveSession, 0, (len(sessions)+concurrency-1)/concurrency)
	for len(sessions) > 0 {
		count := min(len(sessions), concurrency)
		batches = append(batches, sessions[:count])
		sessions = sessions[count:]
	}
	return batches
}

func runSpeedTests(ctx context.Context, state *resultState, opts options) {
	state.mu.Lock()
	sessions := append([]*liveSession(nil), state.sessions...)
	state.mu.Unlock()

	upload := measureSpeedDirection(
		ctx,
		sessions,
		"upload",
		opts.speedBaselineBytes,
		opts.speedBytes,
		opts.speedTimeout,
		opts.minSpeedAggregateRatio,
		opts.minUploadAggregateMbps,
	)
	download := measureSpeedDirection(
		ctx,
		sessions,
		"download",
		opts.speedBaselineBytes,
		opts.speedBytes,
		opts.speedTimeout,
		opts.minSpeedAggregateRatio,
		opts.minDownloadAggregateMbps,
	)

	state.mu.Lock()
	state.speedTest = speedTestSummary{Upload: upload, Download: download}
	for _, direction := range []speedDirectionSummary{upload, download} {
		for _, run := range []speedRunSummary{direction.Baseline, direction.Concurrent} {
			for _, session := range run.Sessions {
				if session.Error != "" {
					state.appendErrorLocked(fmt.Sprintf(
						"speed %s session %d via %s upstream %s: %s",
						direction.Direction,
						session.Index,
						session.Edge,
						session.Upstream,
						session.Error,
					))
				}
			}
		}
		if direction.AggregateToBaselineRatio < opts.minSpeedAggregateRatio {
			state.appendErrorLocked(fmt.Sprintf(
				"speed %s aggregate ratio %.3f is below %.3f",
				direction.Direction,
				direction.AggregateToBaselineRatio,
				opts.minSpeedAggregateRatio,
			))
		}
		minAggregateMbps := opts.minUploadAggregateMbps
		if direction.Direction == "download" {
			minAggregateMbps = opts.minDownloadAggregateMbps
		}
		if direction.Concurrent.AggregateMbps < minAggregateMbps {
			state.appendErrorLocked(fmt.Sprintf(
				"speed %s aggregate %.3f Mbps is below %.3f Mbps",
				direction.Direction,
				direction.Concurrent.AggregateMbps,
				minAggregateMbps,
			))
		}
	}
	state.mu.Unlock()
}

func measureSpeedDirection(
	ctx context.Context,
	sessions []*liveSession,
	direction string,
	baselineBytesPerSession int64,
	concurrentBytesPerSession int64,
	timeout time.Duration,
	minAggregateRatio float64,
	minAggregateMbps float64,
) speedDirectionSummary {
	summary := speedDirectionSummary{
		Direction:                 direction,
		BaselineBytesPerSession:   baselineBytesPerSession,
		ConcurrentBytesPerSession: concurrentBytesPerSession,
	}
	if len(sessions) == 0 {
		return summary
	}
	summary.Baseline = measureSpeedRun(
		ctx,
		sessions[:1],
		direction,
		baselineBytesPerSession,
		timeout,
		"baseline",
	)
	summary.Concurrent = measureSpeedRun(
		ctx,
		sessions,
		direction,
		concurrentBytesPerSession,
		timeout,
		"concurrent",
	)
	if summary.Baseline.AggregateMbps > 0 {
		summary.AggregateToBaselineRatio =
			summary.Concurrent.AggregateMbps / summary.Baseline.AggregateMbps
	}
	summary.Passed = speedDirectionPassed(summary, minAggregateRatio, minAggregateMbps)
	return summary
}

func speedDirectionPassed(
	summary speedDirectionSummary,
	minAggregateRatio float64,
	minAggregateMbps float64,
) bool {
	return summary.Baseline.Failures == 0 &&
		summary.Baseline.Completed == summary.Baseline.Attempted &&
		summary.Concurrent.Failures == 0 &&
		summary.Concurrent.Completed == summary.Concurrent.Attempted &&
		summary.AggregateToBaselineRatio >= minAggregateRatio &&
		summary.Concurrent.AggregateMbps >= minAggregateMbps
}

type speedAttempt struct {
	result gizcli.SpeedTestResult
	err    error
}

func measureSpeedRun(
	ctx context.Context,
	sessions []*liveSession,
	direction string,
	bytesPerSession int64,
	timeout time.Duration,
	runName string,
) speedRunSummary {
	attempts := make([]speedAttempt, len(sessions))
	start := make(chan struct{})
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var wg sync.WaitGroup
	for index, session := range sessions {
		wg.Go(func() {
			<-start
			request := rpcapi.SpeedTestRequest{}
			switch direction {
			case "upload":
				request.UpContentLength = bytesPerSession
			case "download":
				request.DownContentLength = bytesPerSession
			default:
				attempts[index].err = fmt.Errorf("unsupported speed direction %q", direction)
				return
			}
			attempts[index].result, attempts[index].err = session.speedTest(
				runCtx,
				fmt.Sprintf("gateway-capacity.speed.%s.%s.%d", direction, runName, index),
				request,
			)
		})
	}
	started := time.Now()
	close(start)
	wg.Wait()
	elapsed := time.Since(started)
	return summarizeSpeedRun(started, elapsed, sessions, direction, bytesPerSession, attempts)
}

func summarizeSpeedRun(
	started time.Time,
	elapsed time.Duration,
	sessions []*liveSession,
	direction string,
	bytesPerSession int64,
	attempts []speedAttempt,
) speedRunSummary {
	summary := speedRunSummary{
		StartedAt:      started,
		Duration:       elapsed,
		Attempted:      len(sessions),
		RequestedBytes: int64(len(sessions)) * bytesPerSession,
		Edge:           make(map[string]speedPathSummary),
		Upstream:       make(map[string]map[string]speedPathSummary),
		Sessions:       make([]speedSessionResult, len(sessions)),
	}
	rates := make([]float64, 0, len(sessions))
	edgeRates := make(map[string][]float64)
	upstreamRates := make(map[string]map[string][]float64)
	for index, session := range sessions {
		result := speedSessionResult{
			Index: index, Edge: session.edge, Upstream: session.upstream,
		}
		path := summary.Edge[session.edge]
		path.Attempted++
		if summary.Upstream[session.edge] == nil {
			summary.Upstream[session.edge] = make(map[string]speedPathSummary)
		}
		if upstreamRates[session.edge] == nil {
			upstreamRates[session.edge] = make(map[string][]float64)
		}
		upstreamPath := summary.Upstream[session.edge][session.upstream]
		upstreamPath.Attempted++

		if index >= len(attempts) {
			result.Error = "missing speed result"
		} else if attempts[index].err != nil {
			result.Error = attempts[index].err.Error()
		} else {
			switch direction {
			case "upload":
				result.Bytes = attempts[index].result.UpBytes
				result.Duration = attempts[index].result.UpDuration
				result.Mbps = attempts[index].result.UpMbps()
			case "download":
				result.Bytes = attempts[index].result.DownBytes
				result.Duration = attempts[index].result.DownDuration
				result.Mbps = attempts[index].result.DownMbps()
			default:
				result.Error = fmt.Sprintf("unsupported speed direction %q", direction)
			}
			if result.Error == "" && result.Bytes != bytesPerSession {
				result.Error = fmt.Sprintf(
					"transferred bytes %d, want %d",
					result.Bytes,
					bytesPerSession,
				)
			}
			if result.Error == "" && (result.Duration <= 0 || result.Mbps <= 0) {
				result.Error = fmt.Sprintf(
					"invalid duration/rate %s/%.3f Mbps",
					result.Duration,
					result.Mbps,
				)
			}
		}
		if result.Error == "" {
			summary.Completed++
			summary.TransferredBytes += result.Bytes
			path.Completed++
			path.TransferredBytes += result.Bytes
			upstreamPath.Completed++
			upstreamPath.TransferredBytes += result.Bytes
			rates = append(rates, result.Mbps)
			edgeRates[session.edge] = append(edgeRates[session.edge], result.Mbps)
			upstreamRates[session.edge][session.upstream] = append(
				upstreamRates[session.edge][session.upstream],
				result.Mbps,
			)
		} else {
			summary.Failures++
			path.Failures++
			upstreamPath.Failures++
		}
		summary.Edge[session.edge] = path
		summary.Upstream[session.edge][session.upstream] = upstreamPath
		summary.Sessions[index] = result
	}
	if elapsed > 0 {
		summary.AggregateMbps = mbpsForBytes(summary.TransferredBytes, elapsed)
		for edge, path := range summary.Edge {
			path.AggregateMbps = mbpsForBytes(path.TransferredBytes, elapsed)
			path.PerSessionMbps = summarizeRates(edgeRates[edge])
			summary.Edge[edge] = path
		}
		for edge, upstreams := range summary.Upstream {
			for upstream, path := range upstreams {
				path.AggregateMbps = mbpsForBytes(path.TransferredBytes, elapsed)
				path.PerSessionMbps = summarizeRates(upstreamRates[edge][upstream])
				summary.Upstream[edge][upstream] = path
			}
		}
	}
	summary.PerSessionMbps = summarizeRates(rates)
	return summary
}

func mbpsForBytes(bytes int64, duration time.Duration) float64 {
	if bytes <= 0 || duration <= 0 {
		return 0
	}
	return float64(bytes*8) / duration.Seconds() / 1_000_000
}

func summarizeRates(values []float64) rateSummary {
	if len(values) == 0 {
		return rateSummary{}
	}
	sorted := append([]float64(nil), values...)
	slices.Sort(sorted)
	return rateSummary{
		Count: len(sorted),
		Min:   sorted[0],
		P50:   floatPercentile(sorted, 0.50),
		P95:   floatPercentile(sorted, 0.95),
		P99:   floatPercentile(sorted, 0.99),
		Max:   sorted[len(sorted)-1],
	}
}

func floatPercentile(sorted []float64, quantile float64) float64 {
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

func summarizeLatencyMap(values map[string][]time.Duration) map[string]latencySummary {
	summaries := make(map[string]latencySummary, len(values))
	for key, samples := range values {
		summaries[key] = summarizeLatency(samples)
	}
	return summaries
}

func summarizeNestedLatencyMap(
	values map[string]map[string][]time.Duration,
) map[string]map[string]latencySummary {
	summaries := make(map[string]map[string]latencySummary, len(values))
	for key, samples := range values {
		summaries[key] = summarizeLatencyMap(samples)
	}
	return summaries
}

func countNested(values map[string]map[string]int) int {
	total := 0
	for _, entries := range values {
		for _, count := range entries {
			total += count
		}
	}
	return total
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

func (s *liveSession) byteCounts() (uint64, uint64) {
	conn := s.peerConn()
	if conn == nil {
		return 0, 0
	}
	peer := conn.PeerInfo()
	if peer == nil {
		return 0, 0
	}
	return peer.RxBytes, peer.TxBytes
}

func counterDelta(before, after uint64) uint64 {
	if after < before {
		return 0
	}
	return after - before
}

func (s *liveSession) speedTest(
	ctx context.Context,
	id string,
	request rpcapi.SpeedTestRequest,
) (gizcli.SpeedTestResult, error) {
	if s == nil {
		return gizcli.SpeedTestResult{}, errors.New("nil live session")
	}
	if s.speedFn != nil {
		return s.speedFn(ctx, id, request)
	}
	if s.client == nil {
		return gizcli.SpeedTestResult{}, errors.New("live session has no client")
	}
	return s.client.SpeedTest(ctx, id, request)
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

func (s *resultState) recordPingRound(round pingRoundSummary) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pingRounds = append(s.pingRounds, round)
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

func finalize(report artifact, state *resultState, resources *resourceSampler, extended *extendedSamplerState) artifact {
	if extended != nil {
		report.Extended = extended.finish(context.Background(), resources)
	}
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
	report.Establishment = state.establishment
	report.PingRounds = append([]pingRoundSummary(nil), state.pingRounds...)
	report.SpeedTest = state.speedTest
	report.Errors = append([]string(nil), state.errors...)
	if report.Extended != nil {
		report.Errors = append(report.Errors, report.Extended.Errors...)
	}
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
	extendedFailed := report.Extended != nil && len(report.Extended.Errors) > 0
	report.Passed = !extendedFailed && report.EstablishmentFailures <= report.Config.MaxEstablishmentFailures &&
		establishmentWithin(report.Establishment, report.Config) &&
		report.PingFailures <= report.Config.MaxPingFailures &&
		report.UnexpectedDisconnects == 0 && !report.IdentityCrossover &&
		(report.Config.SpeedBytes == 0 ||
			(report.SpeedTest.Upload.Passed && report.SpeedTest.Download.Passed)) &&
		(report.Config.MaxP99RTT == 0 || time.Duration(report.RTT.P99*float64(time.Millisecond)) <= report.Config.MaxP99RTT) &&
		pingRoundsWithin(report.PingRounds, report.Config.MaxPingRoundDuration) &&
		distributionWithin(report, report.Config)
	return report
}

func pingRoundsWithin(rounds []pingRoundSummary, maximum time.Duration) bool {
	if maximum == 0 {
		return true
	}
	for _, round := range rounds {
		if round.Duration > maximum {
			return false
		}
	}
	return true
}

func distributionWithin(report artifact, config artifactConfig) bool {
	for _, edge := range config.Edges {
		if config.MaxSessionsPerEdge > 0 && report.EdgeDistribution[edge] > config.MaxSessionsPerEdge {
			return false
		}
		if config.MaxUpstreamsPerEdge > 0 || config.MaxSessionsPerUpstream > 0 {
			assigned := 0
			for _, sessions := range report.UpstreamDistribution[edge] {
				assigned += sessions
			}
			if assigned != report.EdgeDistribution[edge] {
				return false
			}
		}
	}
	if config.RequireBalancedEdges {
		if len(config.Edges) == 0 || report.Established%len(config.Edges) != 0 {
			return false
		}
		expected := report.Established / len(config.Edges)
		for _, edge := range config.Edges {
			if report.EdgeDistribution[edge] != expected {
				return false
			}
		}
	}
	for _, upstreams := range report.UpstreamDistribution {
		if config.RequiredUpstreamsPerEdge > 0 && len(upstreams) != config.RequiredUpstreamsPerEdge {
			return false
		}
		if config.MaxUpstreamsPerEdge > 0 && len(upstreams) > config.MaxUpstreamsPerEdge {
			return false
		}
		if config.MaxSessionsPerUpstream > 0 {
			for _, sessions := range upstreams {
				if sessions > config.MaxSessionsPerUpstream {
					return false
				}
			}
		}
	}
	return true
}

func acceptanceError(report artifact, opts options) error {
	if report.Passed {
		return nil
	}
	return fmt.Errorf(
		"gateway capacity failed: established=%d/%d dial_failures=%d establishment=(%.2f sessions/s,p95 %.2fms,p99 %.2fms) ping_failures=%d disconnects=%d crossover=%t rtt_p99=%.2fms speed=(upload %.2fMbps %.2fx, download %.2fMbps %.2fx) thresholds=(dial_failures %d,rate %.2f sessions/s,p95 %s,p99 %s,ping_failures %d,rtt_p99 %s,%.2fx,upload %.2fMbps,download %.2fMbps)",
		report.Established, report.Attempted, report.EstablishmentFailures,
		report.Establishment.UsableSessionsPerSecond,
		report.Establishment.Dial.P95,
		report.Establishment.Dial.P99,
		report.PingFailures, report.UnexpectedDisconnects, report.IdentityCrossover, report.RTT.P99,
		report.SpeedTest.Upload.Concurrent.AggregateMbps,
		report.SpeedTest.Upload.AggregateToBaselineRatio,
		report.SpeedTest.Download.Concurrent.AggregateMbps,
		report.SpeedTest.Download.AggregateToBaselineRatio,
		opts.maxEstablishmentFailures, opts.minEstablishmentRate, opts.maxDialP95, opts.maxDialP99,
		opts.maxPingFailures, opts.maxP99RTT,
		opts.minSpeedAggregateRatio,
		opts.minUploadAggregateMbps,
		opts.minDownloadAggregateMbps,
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
	done                   chan struct{}
	wg                     sync.WaitGroup
	mu                     sync.Mutex
	data                   resourceSummary
	points                 []resourcePoint
	requireProcessFallback bool
}

func newResourceSampler(requireProcessFallback bool) *resourceSampler {
	s := &resourceSampler{done: make(chan struct{}), requireProcessFallback: requireProcessFallback}
	point := readResourcePoint(requireProcessFallback)
	s.data.Start = point
	s.data.Peak = point
	s.points = append(s.points, point)
	s.wg.Go(func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.observe(readResourcePoint(s.requireProcessFallback))
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
	s.points = append(s.points, point)
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

func (s *resourceSampler) samples() []resourcePoint {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]resourcePoint(nil), s.points...)
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
	point := readResourcePoint(s.requireProcessFallback)
	s.observe(point)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.End = point
	return s.data
}

func readResourcePoint(requireProcessFallback bool) resourcePoint {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	rssBytes, rssSource := readRSS(memory.Sys, requireProcessFallback)
	openFDs, openFDsSource := readFDCount(requireProcessFallback)
	return resourcePoint{
		At: time.Now(), RSSBytes: rssBytes, RSSSource: rssSource,
		CPUSeconds: readActiveCPUSeconds(), CPUSecondsSource: "go_runtime_total_minus_idle",
		OpenFDs: openFDs, OpenFDsSource: openFDsSource, HeapAllocBytes: memory.HeapAlloc,
		Goroutines: runtime.NumGoroutine(),
	}
}

func readActiveCPUSeconds() float64 {
	samples := []metrics.Sample{
		{Name: "/cpu/classes/total:cpu-seconds"},
		{Name: "/cpu/classes/idle:cpu-seconds"},
	}
	metrics.Read(samples)
	if samples[0].Value.Kind() != metrics.KindFloat64 ||
		samples[1].Value.Kind() != metrics.KindFloat64 {
		return 0
	}
	return activeCPUSeconds(
		samples[0].Value.Float64(),
		samples[1].Value.Float64(),
	)
}

func activeCPUSeconds(total, idle float64) float64 {
	return max(total-idle, 0)
}

func readRSS(fallback uint64, requireProcessFallback bool) (uint64, string) {
	data, err := os.ReadFile("/proc/self/statm")
	if err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 2 {
			pages, parseErr := strconv.ParseUint(fields[1], 10, 64)
			if parseErr == nil {
				return pages * uint64(os.Getpagesize()), "proc_self_statm"
			}
		}
	}
	if requireProcessFallback {
		output, commandErr := boundedProcessCommand("ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid()))
		if commandErr == nil {
			kilobytes, parseErr := strconv.ParseUint(strings.TrimSpace(string(output)), 10, 64)
			if parseErr == nil {
				return kilobytes * 1024, "ps_rss_kib"
			}
		}
	}
	return fallback, "go_memstats_sys"
}

func readFDCount(requireProcessFallback bool) (int, string) {
	entries, err := os.ReadDir("/proc/self/fd")
	source := "proc_self_fd"
	if err != nil {
		entries, err = os.ReadDir("/dev/fd")
		source = "dev_fd"
	}
	if err == nil {
		return len(entries), source
	}
	if requireProcessFallback {
		if count, ok := readNativeFDCount(); ok {
			return count, "native_process"
		}
		output, commandErr := boundedProcessCommand("lsof", "-b", "-nP", "-a", "-p", strconv.Itoa(os.Getpid()), "-Ff")
		if commandErr == nil {
			return countLsofFileDescriptors(string(output)), "lsof_process"
		}
	}
	return -1, "unsupported"
}

func boundedProcessCommand(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}

func countLsofFileDescriptors(output string) int {
	count := 0
	for line := range strings.SplitSeq(output, "\n") {
		if len(line) > 1 && line[0] == 'f' {
			if _, err := strconv.ParseUint(line[1:], 10, 64); err != nil {
				continue
			}
			count++
		}
	}
	return count
}
