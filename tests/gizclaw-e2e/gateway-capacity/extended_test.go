package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestValidateOptionsRequiresCompleteRoleSamplingIdentity(t *testing.T) {
	opts := validOptionsForTest()
	opts.requireRoleResources = true
	if err := validateOptions(opts); err == nil || !strings.Contains(err.Error(), "docker-project") {
		t.Fatalf("validateOptions error = %v, want Docker identity error", err)
	}
	opts.dockerProject = "capacity-test"
	opts.dockerComposeFile = "compose.yaml"
	if err := validateOptions(opts); err == nil || !strings.Contains(err.Error(), "scenario") {
		t.Fatalf("validateOptions error = %v, want scenario error", err)
	}
	opts.scenario = "sessions-100"
	opts.repetition = 1
	if err := validateOptions(opts); err != nil {
		t.Fatalf("validateOptions = %v", err)
	}
}

func TestWriteDiagnosticHeapProfileIsOptIn(t *testing.T) {
	t.Setenv("GIZCLAW_E2E_GATEWAY_HEAP_PROFILE_DIR", "")
	if err := writeDiagnosticHeapProfile("disabled"); err != nil {
		t.Fatalf("writeDiagnosticHeapProfile(disabled) = %v", err)
	}

	dir := t.TempDir()
	t.Setenv("GIZCLAW_E2E_GATEWAY_HEAP_PROFILE_DIR", dir)
	if err := writeDiagnosticHeapProfile("hold-start"); err != nil {
		t.Fatalf("writeDiagnosticHeapProfile(enabled) = %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "hold-start.pprof"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("diagnostic heap profile is empty")
	}
}

func TestValidateOptionsKeepsPingTimeoutInsideRoundBudget(t *testing.T) {
	opts := validOptionsForTest()
	opts.pingInterval = 30 * time.Second
	opts.maxPingRoundDuration = 30 * time.Second
	opts.pingTimeout = 28 * time.Second
	if err := validateOptions(opts); err != nil {
		t.Fatalf("validateOptions = %v", err)
	}

	opts.pingTimeout = opts.maxPingRoundDuration
	if err := validateOptions(opts); err == nil || !strings.Contains(err.Error(), "less than") {
		t.Fatalf("validateOptions error = %v, want ping timeout budget error", err)
	}

	opts.pingTimeout = 28 * time.Second
	opts.maxPingRoundDuration = 31 * time.Second
	if err := validateOptions(opts); err == nil || !strings.Contains(err.Error(), "must not exceed") {
		t.Fatalf("validateOptions error = %v, want ping interval budget error", err)
	}
}

func TestValidateOptionsRequiresCompleteFinalSpeedContract(t *testing.T) {
	opts := validOptionsForTest()
	opts.minFinalSpeedRetention = 0.8
	if err := validateOptions(opts); err == nil || !strings.Contains(err.Error(), "requires -soak") {
		t.Fatalf("validateOptions error = %v, want soak requirement", err)
	}
	opts.soak = true
	opts.duration = time.Hour
	if err := validateOptions(opts); err == nil || !strings.Contains(err.Error(), "positive -speed-bytes") {
		t.Fatalf("validateOptions error = %v, want speed requirement", err)
	}
	opts.speedBytes = 1 << 20
	if err := validateOptions(opts); err != nil {
		t.Fatalf("validateOptions = %v", err)
	}
	opts.cleanupTimeout = 0
	if err := validateOptions(opts); err == nil || !strings.Contains(err.Error(), "cleanup-timeout") {
		t.Fatalf("validateOptions error = %v, want cleanup timeout requirement", err)
	}
}

func TestParseDockerProcessSample(t *testing.T) {
	at := time.Unix(10, 0)
	got, err := parseDockerProcessSample("10000000000 123 5 4096 30 10 100 7 999 1024 8 9 1000 2000")
	if err != nil {
		t.Fatal(err)
	}
	if got.ProcessID != 123 || got.ProcessStartTicks != 999 || got.OpenFDLimit != 1024 {
		t.Fatalf("process metadata = %+v", got)
	}
	if got.Point.RSSBytes != 20_480 || got.Point.CPUSeconds != 0.4 || got.Point.OpenFDs != 7 {
		t.Fatalf("process point = %+v", got.Point)
	}
	if got.Point.UDPSockets != 8 || got.Point.UDP6Sockets != 9 ||
		got.Point.NetworkRXBytes != 1000 || got.Point.NetworkTXBytes != 2000 {
		t.Fatalf("socket/network point = %+v", got.Point)
	}
	if got.Point.GoHeapAllocBytes != nil || got.Point.GoHeapLiveBytes != nil || got.Point.Goroutines != nil || len(got.Point.UnsupportedMetrics) != 3 {
		t.Fatalf("unsupported Go metrics = %+v", got.Point)
	}
	if got.Point.At != at {
		t.Fatalf("sample timestamp = %s, want %s", got.Point.At, at)
	}
	if _, err := parseDockerProcessSample("123 invalid"); err == nil {
		t.Fatal("parseDockerProcessSample accepted malformed output")
	}
	for name, sample := range map[string]string{
		"timestamp":      "18446744073709551615 1 1 1 1 1 1 1 1 1 1 1 1 1",
		"process ID":     "1 18446744073709551615 1 1 1 1 1 1 1 1 1 1 1 1",
		"open FD count":  "1 1 1 1 1 1 1 18446744073709551615 1 1 1 1 1 1",
		"resident bytes": "1 1 18446744073709551615 2 1 1 1 1 1 1 1 1 1 1",
		"UDP sockets":    "1 1 1 1 1 1 1 1 1 1 18446744073709551615 1 1 1",
	} {
		t.Run(name+" overflow", func(t *testing.T) {
			if _, err := parseDockerProcessSample(sample); err == nil || !strings.Contains(err.Error(), "overflow") {
				t.Fatalf("parseDockerProcessSample error = %v, want overflow", err)
			}
		})
	}
	for name, sample := range map[string]string{
		"timestamp":     "-1 1 1 1 1 1 1 1 1 1 1 1 1 1",
		"process ID":    "1 -1 1 1 1 1 1 1 1 1 1 1 1 1",
		"open FD count": "1 1 1 1 1 1 1 -1 1 1 1 1 1 1",
		"UDP sockets":   "1 1 1 1 1 1 1 1 1 1 -1 1 1 1",
		"UDP6 sockets":  "1 1 1 1 1 1 1 1 1 1 1 -1 1 1",
	} {
		t.Run(name+" negative", func(t *testing.T) {
			if _, err := parseDockerProcessSample(sample); err == nil || !strings.Contains(err.Error(), "non-negative") {
				t.Fatalf("parseDockerProcessSample error = %v, want non-negative", err)
			}
		})
	}
}

func TestDockerProcessSampleScriptReadsLinuxProc(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux /proc is required")
	}
	pidFile := filepath.Join(t.TempDir(), "pid")
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "bash", "-c", dockerProcessSampleScript, "gateway-capacity-sampler-test", pidFile)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = command.Process.Kill()
		_ = command.Wait()
	}()
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() {
		t.Fatalf("sampler produced no output: %v: %s", scanner.Err(), strings.TrimSpace(stderr.String()))
	}
	sample, err := parseDockerProcessSample(scanner.Text())
	if err != nil {
		t.Fatal(err)
	}
	if sample.ProcessID != os.Getpid() || sample.ProcessStartTicks == 0 || sample.OpenFDLimit == 0 {
		t.Fatalf("process sample = %+v", sample)
	}
}

func TestClassifyTCShaping(t *testing.T) {
	if got := classifyTCShaping("qdisc noqueue 0: dev lo root refcnt 2"); got.Status != "inactive" {
		t.Fatalf("ordinary qdisc = %+v", got)
	}
	if got := classifyTCShaping("qdisc netem 8001: dev eth0 root limit 1000 delay 10ms"); got.Status != "active" {
		t.Fatalf("netem qdisc = %+v", got)
	}
}

func TestCountLsofFileDescriptors(t *testing.T) {
	output := "p123\nfcwd\nf0\nf1\nftxt\nfmem\n"
	if got := countLsofFileDescriptors(output); got != 2 {
		t.Fatalf("countLsofFileDescriptors = %d, want 2", got)
	}
}

func TestValidateRequiredRoleEvidenceRejectsFallbackAndMissingFDs(t *testing.T) {
	start := time.Unix(1, 0)
	heap, live, goroutines := uint64(1), uint64(1), 1
	evidence := roleResourceEvidence{Role: "load_driver", Samples: []roleResourcePoint{{
		At: start, RSSBytes: 1, RSSSource: "go_memstats_sys", CPUSecondsSource: "go_runtime", OpenFDs: 1, OpenFDsSource: "lsof_process",
		GoHeapAllocBytes: &heap, GoHeapLiveBytes: &live, Goroutines: &goroutines, SocketSource: "unsupported", NetworkSource: "unsupported",
		UnsupportedMetrics: []string{"udp_sockets", "udp6_sockets", "network_rx_bytes", "network_tx_bytes"},
	}}}
	if err := validateRequiredRoleEvidence(evidence); err == nil || !strings.Contains(err.Error(), "RSS") {
		t.Fatalf("validateRequiredRoleEvidence error = %v, want RSS error", err)
	}
	evidence.Samples[0].RSSSource = "go_runtime_memory_total"
	if err := validateRequiredRoleEvidence(evidence); err == nil || !strings.Contains(err.Error(), "RSS") {
		t.Fatalf("validateRequiredRoleEvidence error = %v, want runtime-memory RSS error", err)
	}
	evidence.Samples[0].RSSSource = "ps_rss_kib"
	evidence.Samples[0].OpenFDs = -1
	evidence.Samples[0].OpenFDsSource = "unsupported"
	if err := validateRequiredRoleEvidence(evidence); err == nil || !strings.Contains(err.Error(), "open-file") {
		t.Fatalf("validateRequiredRoleEvidence error = %v, want open-file error", err)
	}
	evidence.Samples[0].OpenFDs = 1
	evidence.Samples[0].OpenFDsSource = "lsof_process"
	evidence.Samples = append(evidence.Samples, roleResourcePoint{
		At: start.Add(4 * time.Second), RSSBytes: 1, RSSSource: "ps_rss_kib",
		CPUSecondsSource: "go_runtime", OpenFDs: 1, OpenFDsSource: "lsof_process",
		GoHeapAllocBytes: &heap, GoHeapLiveBytes: &live, Goroutines: &goroutines, SocketSource: "unsupported", NetworkSource: "unsupported",
		UnsupportedMetrics: []string{"udp_sockets", "udp6_sockets", "network_rx_bytes", "network_tx_bytes"},
	})
	if err := validateRequiredRoleEvidence(evidence); err == nil || !strings.Contains(err.Error(), "sample gap") {
		t.Fatalf("validateRequiredRoleEvidence error = %v, want sample-gap error", err)
	}
}

func TestValidateRequiredRoleEvidenceRequiresUnsupportedMetricDeclarations(t *testing.T) {
	start := time.Unix(1, 0)
	external := roleResourceEvidence{Role: "edge", Samples: []roleResourcePoint{{
		At: start, RSSBytes: 1, RSSSource: "proc_pid_statm", CPUSecondsSource: "proc_pid_stat",
		OpenFDs: 1, OpenFDsSource: "proc_pid_fd", SocketSource: "proc_pid_net_udp", NetworkSource: "proc_pid_net_dev",
	}}}
	if err := validateRequiredRoleEvidence(external); err == nil || !strings.Contains(err.Error(), "Go runtime declarations") {
		t.Fatalf("validateRequiredRoleEvidence error = %v, want unsupported Go runtime declaration error", err)
	}

	heap, live, goroutines := uint64(1), uint64(1), 1
	loadDriver := roleResourceEvidence{Role: "load_driver", Samples: []roleResourcePoint{{
		At: start, RSSBytes: 1, RSSSource: "ps_rss_kib", CPUSecondsSource: "go_runtime",
		OpenFDs: 1, OpenFDsSource: "lsof_process", GoHeapAllocBytes: &heap, GoHeapLiveBytes: &live, Goroutines: &goroutines,
		SocketSource: "unsupported", NetworkSource: "unsupported",
	}}}
	if err := validateRequiredRoleEvidence(loadDriver); err == nil || !strings.Contains(err.Error(), "unsupported socket or network declarations") {
		t.Fatalf("validateRequiredRoleEvidence error = %v, want unsupported socket declaration error", err)
	}
}

func TestValidateRequiredRoleEvidenceRejectsDecreasingCounters(t *testing.T) {
	start := time.Unix(1, 0)
	points := []roleResourcePoint{
		{
			At: start, RSSBytes: 1, RSSSource: "proc_pid_statm", CPUSeconds: 2, CPUSecondsSource: "proc_pid_stat",
			OpenFDs: 1, OpenFDsSource: "proc_pid_fd", SocketSource: "proc_pid_net_udp",
			NetworkRXBytes: 10, NetworkTXBytes: 10, NetworkSource: "proc_pid_net_dev",
			UnsupportedMetrics: []string{"go_heap_alloc_bytes", "go_heap_live_bytes", "goroutines"},
		},
		{
			At: start.Add(time.Second), RSSBytes: 1, RSSSource: "proc_pid_statm", CPUSeconds: 1, CPUSecondsSource: "proc_pid_stat",
			OpenFDs: 1, OpenFDsSource: "proc_pid_fd", SocketSource: "proc_pid_net_udp",
			NetworkRXBytes: 11, NetworkTXBytes: 11, NetworkSource: "proc_pid_net_dev",
			UnsupportedMetrics: []string{"go_heap_alloc_bytes", "go_heap_live_bytes", "goroutines"},
		},
	}
	evidence := roleResourceEvidence{Role: "edge", Samples: points}
	if err := validateRequiredRoleEvidence(evidence); err == nil || !strings.Contains(err.Error(), "CPU counter decreased") {
		t.Fatalf("validateRequiredRoleEvidence error = %v, want CPU counter error", err)
	}
	evidence.Samples[1].CPUSeconds = 3
	evidence.Samples[1].NetworkRXBytes = 9
	if err := validateRequiredRoleEvidence(evidence); err == nil || !strings.Contains(err.Error(), "network counter decreased") {
		t.Fatalf("validateRequiredRoleEvidence error = %v, want network counter error", err)
	}
}

func TestExtendedSamplerLiveHealthReportsProgress(t *testing.T) {
	now := time.Now()
	sampler := testExtendedSampler(now, time.Second)
	progress, err := sampler.liveHealth(now)
	if err != nil {
		t.Fatal(err)
	}
	if progress.MinimumSamples != 2 || progress.MaximumGap != time.Second || progress.MaximumAge != 0 {
		t.Fatalf("liveHealth progress = %+v", progress)
	}
}

func TestExtendedSamplerLiveHealthRejectsHistoricalGap(t *testing.T) {
	now := time.Now()
	sampler := testExtendedSampler(now, maximumResourceSampleGap+time.Millisecond)
	if _, err := sampler.liveHealth(now); err == nil || !strings.Contains(err.Error(), "sample gap") {
		t.Fatalf("liveHealth error = %v, want sample-gap error", err)
	}
}

func TestExtendedSamplerLiveHealthRejectsStaleStream(t *testing.T) {
	now := time.Now()
	sampler := testExtendedSampler(now.Add(-maximumResourceSampleGap-time.Millisecond), time.Second)
	if _, err := sampler.liveHealth(now); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("liveHealth error = %v, want stale-stream error", err)
	}
}

func TestExtendedSamplerLiveHealthAllowsConcurrentSampleTimestamp(t *testing.T) {
	now := time.Now()
	sampler := testExtendedSampler(now.Add(time.Millisecond), time.Second)
	progress, err := sampler.liveHealth(now)
	if err != nil {
		t.Fatal(err)
	}
	if progress.MaximumAge != 0 {
		t.Fatalf("liveHealth maximum age = %s, want zero", progress.MaximumAge)
	}
}

func TestExtendedSamplerLiveHealthRejectsFutureClockSkew(t *testing.T) {
	now := time.Now()
	sampler := testExtendedSampler(now.Add(maximumResourceSampleFutureSkew+time.Millisecond), time.Second)
	if _, err := sampler.liveHealth(now); err == nil || !strings.Contains(err.Error(), "in the future") {
		t.Fatalf("liveHealth error = %v, want future-clock error", err)
	}
}

func testExtendedSampler(latest time.Time, gap time.Duration) *extendedSamplerState {
	roles := make(map[string]*dockerRoleState)
	for _, role := range []string{"edge", "edge2", "server", "coturn-a", "coturn-b"} {
		roles[role] = &dockerRoleState{role: role, samples: []roleResourcePoint{
			testExternalRoleResourcePoint(latest.Add(-gap)),
			testExternalRoleResourcePoint(latest),
		}}
	}
	return &extendedSamplerState{docker: &dockerResourceSampler{roles: roles}}
}

func testExternalRoleResourcePoint(at time.Time) roleResourcePoint {
	return roleResourcePoint{
		At: at, RSSBytes: 1, RSSSource: "proc_pid_statm", CPUSecondsSource: "proc_pid_stat",
		OpenFDs: 1, OpenFDsSource: "proc_pid_fd", SocketSource: "proc_pid_net_udp", NetworkSource: "proc_pid_net_dev",
		UnsupportedMetrics: []string{"go_heap_alloc_bytes", "go_heap_live_bytes", "goroutines"},
	}
}

func TestRecordDockerRoleSampleRejectsProcessReplacement(t *testing.T) {
	state := &dockerRoleState{role: "edge"}
	first := dockerProcessSample{ProcessID: 10, ProcessStartTicks: 20, OpenFDLimit: 100}
	if err := recordDockerRoleSample(state, first); err != nil {
		t.Fatal(err)
	}
	second := dockerProcessSample{ProcessID: 11, ProcessStartTicks: 21, OpenFDLimit: 100}
	if err := recordDockerRoleSample(state, second); err == nil || !strings.Contains(err.Error(), "process changed") {
		t.Fatalf("recordDockerRoleSample error = %v", err)
	}
	if len(state.samples) != 2 {
		t.Fatalf("recorded samples = %d, want 2", len(state.samples))
	}
}

func TestValidateContainerFinalStateRejectsExitAndRestart(t *testing.T) {
	metadata := containerMetadata{RestartCount: 2}
	metadata.State.Running = true
	metadata.State.Status = "running"
	if err := validateContainerFinalState("server", 2, metadata); err != nil {
		t.Fatal(err)
	}
	metadata.RestartCount = 3
	if err := validateContainerFinalState("server", 2, metadata); err == nil {
		t.Fatal("validateContainerFinalState accepted restart")
	}
	metadata.RestartCount = 2
	metadata.State.Running = false
	metadata.State.Status = "exited"
	if err := validateContainerFinalState("server", 2, metadata); err == nil {
		t.Fatal("validateContainerFinalState accepted exited container")
	}
}

func TestSummarizeRoleResourcesPreservesUnsupportedMetrics(t *testing.T) {
	start := time.Unix(1, 0)
	points := []roleResourcePoint{
		{At: start, RSSBytes: 10, CPUSeconds: 1, OpenFDs: 2, UnsupportedMetrics: []string{"goroutines"}},
		{At: start.Add(time.Second), RSSBytes: 20, CPUSeconds: 2, OpenFDs: 3, UnsupportedMetrics: []string{"goroutines"}},
	}
	got := summarizeRoleResources(points)
	if got.Start.RSSBytes != 10 || got.Peak.RSSBytes != 20 || got.Peak.At != start.Add(time.Second) || got.End.OpenFDs != 3 {
		t.Fatalf("resource summary = %+v", got)
	}
	if len(got.Peak.UnsupportedMetrics) != 1 || got.Peak.UnsupportedMetrics[0] != "goroutines" {
		t.Fatalf("peak unsupported metrics = %v", got.Peak.UnsupportedMetrics)
	}
}

func TestExtendedAcceptanceChecksRoundAndDistribution(t *testing.T) {
	config := artifactConfig{
		Edges: []string{"edge-a", "edge-b"}, RequireBalancedEdges: true,
		MaxSessionsPerEdge: 2, RequiredUpstreamsPerEdge: 2,
		MaxUpstreamsPerEdge: 2, MaxSessionsPerUpstream: 2,
	}
	report := artifact{
		Established:      4,
		EdgeDistribution: map[string]int{"edge-a": 2, "edge-b": 2},
		UpstreamDistribution: map[string]map[string]int{
			"edge-a": {"one": 1, "two": 1},
			"edge-b": {"one": 1, "two": 1},
		},
	}
	if !distributionWithin(report, config) {
		t.Fatal("distributionWithin rejected valid distribution")
	}
	report.EdgeDistribution["edge-a"] = 3
	if distributionWithin(report, config) {
		t.Fatal("distributionWithin accepted an over-limit and imbalanced Edge")
	}
	report.EdgeDistribution["edge-a"] = 2
	report.UpstreamDistribution["edge-a"]["three"] = 1
	if distributionWithin(report, config) {
		t.Fatal("distributionWithin accepted too many upstreams")
	}
	delete(report.UpstreamDistribution["edge-a"], "three")
	report.UpstreamDistribution["edge-a"]["one"] = 3
	if distributionWithin(report, config) {
		t.Fatal("distributionWithin accepted too many sessions on one upstream")
	}
	report.UpstreamDistribution["edge-a"]["one"] = 1
	delete(report.UpstreamDistribution["edge-a"], "two")
	if distributionWithin(report, config) {
		t.Fatal("distributionWithin accepted a session without an upstream assignment")
	}
	if pingRoundsWithin([]pingRoundSummary{{Duration: 31 * time.Second}}, 30*time.Second) {
		t.Fatal("pingRoundsWithin accepted overlapping round")
	}
}

func TestHandleSessionServeExitCountsUnexpectedNilError(t *testing.T) {
	state := &resultState{}
	session := &liveSession{}
	handleSessionServeExit(state, session, 7, nil)
	if state.unexpectedDisconnects != 1 {
		t.Fatalf("unexpected disconnects = %d, want 1", state.unexpectedDisconnects)
	}
	if len(state.errors) != 1 || state.errors[0] != "session 7 disconnected" {
		t.Fatalf("errors = %v", state.errors)
	}
}

func TestHandleSessionServeExitIgnoresIntentionalClose(t *testing.T) {
	state := &resultState{}
	session := &liveSession{}
	session.closed.Store(true)
	handleSessionServeExit(state, session, 7, errors.New("closed"))
	if state.unexpectedDisconnects != 0 || len(state.errors) != 0 {
		t.Fatalf("state after intentional close = %+v", state)
	}
}

func TestPingRoundsForPhase(t *testing.T) {
	rounds := []pingRoundSummary{
		{Phase: "ramp", Round: 0},
		{Phase: "hold", Round: 0},
		{Phase: "ramp", Round: 1},
	}
	got := pingRoundsForPhase(rounds, "ramp")
	if len(got) != 2 || got[0].Round != 0 || got[1].Round != 1 {
		t.Fatalf("ramp rounds = %+v", got)
	}
}

func validOptionsForTest() options {
	return options{
		edges: []string{"edge"}, sessions: 1,
		pingInterval: time.Second, dialTimeout: time.Second,
		pingTimeout: time.Second, speedTimeout: time.Second,
		concurrency: 1, artifactPath: "artifact.json", cleanupTimeout: time.Second,
	}
}
