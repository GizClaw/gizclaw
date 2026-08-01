package main

import (
	"bufio"
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

func TestParseDockerProcessSample(t *testing.T) {
	at := time.Unix(10, 0)
	got, err := parseDockerProcessSample("10000000000 123 5 4096 30 10 100 7 999 1024")
	if err != nil {
		t.Fatal(err)
	}
	if got.ProcessID != 123 || got.ProcessStartTicks != 999 || got.OpenFDLimit != 1024 {
		t.Fatalf("process metadata = %+v", got)
	}
	if got.Point.RSSBytes != 20_480 || got.Point.CPUSeconds != 0.4 || got.Point.OpenFDs != 7 {
		t.Fatalf("process point = %+v", got.Point)
	}
	if got.Point.GoHeapAllocBytes != nil || got.Point.Goroutines != nil || len(got.Point.UnsupportedMetrics) != 2 {
		t.Fatalf("unsupported Go metrics = %+v", got.Point)
	}
	if got.Point.At != at {
		t.Fatalf("sample timestamp = %s, want %s", got.Point.At, at)
	}
	if _, err := parseDockerProcessSample("123 invalid"); err == nil {
		t.Fatal("parseDockerProcessSample accepted malformed output")
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
		t.Fatalf("sampler produced no output: %v", scanner.Err())
	}
	sample, err := parseDockerProcessSample(scanner.Text())
	if err != nil {
		t.Fatal(err)
	}
	if sample.ProcessID != os.Getpid() || sample.ProcessStartTicks == 0 || sample.OpenFDLimit == 0 {
		t.Fatalf("process sample = %+v", sample)
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
	evidence := roleResourceEvidence{Role: "load_driver", Samples: []roleResourcePoint{{
		At: start, RSSBytes: 1, RSSSource: "go_memstats_sys", CPUSecondsSource: "go_runtime", OpenFDs: 1, OpenFDsSource: "lsof_process",
	}}}
	if err := validateRequiredRoleEvidence(evidence); err == nil || !strings.Contains(err.Error(), "RSS") {
		t.Fatalf("validateRequiredRoleEvidence error = %v, want RSS error", err)
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
	})
	if err := validateRequiredRoleEvidence(evidence); err == nil || !strings.Contains(err.Error(), "sample gap") {
		t.Fatalf("validateRequiredRoleEvidence error = %v, want sample-gap error", err)
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
		concurrency: 1, artifactPath: "artifact.json",
	}
}
