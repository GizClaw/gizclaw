package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFitResourceModelAcceptsLinearEvidence(t *testing.T) {
	observations := []roleObservation{
		{Sessions: 100, RSSBytes: 1100},
		{Sessions: 500, RSSBytes: 1500},
		{Sessions: 1000, RSSBytes: 2000},
	}
	got := fitResourceModel(observations, 30_000, func(value roleObservation) float64 { return value.RSSBytes })
	if !got.Qualified || math.Abs(got.Intercept-1000) > 1e-9 ||
		math.Abs(got.PerSession-1) > 1e-9 || math.Abs(got.Projected-31_000) > 1e-9 {
		t.Fatalf("linear model = %+v", got)
	}
}

func TestFitResourceModelRejectsNonlinearAndNegativeEvidence(t *testing.T) {
	nonlinear := []roleObservation{
		{Sessions: 100, RSSBytes: 100},
		{Sessions: 500, RSSBytes: 110},
		{Sessions: 1000, RSSBytes: 1000},
	}
	got := fitResourceModel(nonlinear, 30_000, func(value roleObservation) float64 { return value.RSSBytes })
	if got.Qualified || !strings.Contains(got.Reason, "residual") {
		t.Fatalf("nonlinear model = %+v", got)
	}
	negative := []roleObservation{
		{Sessions: 100, OpenFDs: 100},
		{Sessions: 500, OpenFDs: 80},
		{Sessions: 1000, OpenFDs: 60},
	}
	got = fitResourceModel(negative, 30_000, func(value roleObservation) float64 { return value.OpenFDs })
	if got.Qualified || !strings.Contains(got.Reason, "negative fitted slope") {
		t.Fatalf("negative model = %+v", got)
	}
}

func TestRelativeResidualPreservesSubCoreScale(t *testing.T) {
	if got := relativeResidual(0.01, 0.02); got != 1 {
		t.Fatalf("relative residual = %v, want 1", got)
	}
	if got := relativeResidual(0, 0.01); !math.IsInf(got, 1) {
		t.Fatalf("zero-observation residual = %v, want +Inf", got)
	}
}

func TestAnalyzeCapacityArtifactsProducesQualifiedProjection(t *testing.T) {
	directory := t.TempDir()
	environment := capacityEnvironment{
		RepositoryCommit: "0123456789abcdef", HostMemoryBytes: 1 << 40, HostOpenFDLimit: 1_000_000,
		Docker: dockerEnvironment{OperatingSystem: "test", OSType: "linux", Architecture: "arm64", LogicalCPU: 100_000, MemoryBytes: 1 << 40},
	}
	scenarios := []struct {
		name     string
		sessions int
		ramp     time.Duration
		hold     time.Duration
		repeats  int
		soak     bool
	}{
		{name: "sessions-100", sessions: 100, ramp: 30 * time.Second, hold: 5 * time.Minute, repeats: 3},
		{name: "sessions-500", sessions: 500, ramp: 150 * time.Second, hold: 5 * time.Minute, repeats: 3},
		{name: "sessions-1000", sessions: 1000, ramp: 5 * time.Minute, hold: 5 * time.Minute, repeats: 3},
		{name: "soak-1000", sessions: 1000, ramp: 5 * time.Minute, hold: time.Hour, repeats: 1, soak: true},
	}
	for _, scenario := range scenarios {
		for repetition := 1; repetition <= scenario.repeats; repetition++ {
			run := syntheticExtendedRun(environment, scenario.name, scenario.sessions, scenario.ramp, scenario.hold, repetition, scenario.soak)
			path := filepath.Join(directory, scenario.name+"-"+strconv.Itoa(repetition)+".json")
			if err := writeArtifact(path, run); err != nil {
				t.Fatal(err)
			}
		}
	}

	report, err := analyzeCapacityArtifacts(directory)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Qualified || report.SessionsPerUpstream != 937.5 {
		t.Fatalf("projection = %+v", report)
	}
	if report.Budgets.RequiredLoadDriverHosts == nil || *report.Budgets.RequiredLoadDriverHosts != 1 {
		t.Fatalf("required load-driver hosts = %v, want 1", report.Budgets.RequiredLoadDriverHosts)
	}
	if got, want := report.Budgets.LoadDriverCPUCores, 5.6; math.Abs(got-want) > 1e-9 {
		t.Fatalf("load-driver CPU budget = %v, want %v", got, want)
	}
	if report.Models["edge"].TargetSessions != 15_000 || report.Models["server"].TargetSessions != 30_000 {
		t.Fatalf("role targets = %+v", report.Models)
	}
	path := filepath.Join(directory, "projection-output")
	if err := writeProjectionArtifact(path, report); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded projectionReport
	if err := json.Unmarshal(data, &decoded); err != nil || !decoded.Qualified {
		t.Fatalf("decode projection: report=%+v err=%v", decoded, err)
	}
}

func TestCheckBudgetRejectsProjectionAboveHeadroom(t *testing.T) {
	report := projectionReport{Qualified: true}
	checkBudget(&report, "memory", 71, 70)
	if report.Qualified || len(report.Limitations) != 1 || report.FirstLimitation != report.Limitations[0] ||
		!strings.Contains(report.Limitations[0], "70% budget") {
		t.Fatalf("budget result = %+v", report)
	}
}

func TestMedianAndAverageCPUCores(t *testing.T) {
	values := []roleObservation{{RSSBytes: 30}, {RSSBytes: 10}, {RSSBytes: 20}}
	if got := medianOf(values, func(value roleObservation) float64 { return value.RSSBytes }); got != 20 {
		t.Fatalf("median = %v, want 20", got)
	}
	start := time.Unix(1, 0)
	points := []roleResourcePoint{{At: start, CPUSeconds: 2}, {At: start.Add(4 * time.Second), CPUSeconds: 10}}
	if got := averageCPUCores(points); got != 2 {
		t.Fatalf("average CPU cores = %v, want 2", got)
	}
}

func TestWriteProjectionArtifactReplacesAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projection.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	report := projectionReport{Version: projectionArtifactVersion, Qualified: true}
	if err := writeProjectionArtifact(path, report); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary artifact remains: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), `"qualified": true`) {
		t.Fatalf("artifact data=%q err=%v", data, err)
	}
}

func TestRequiredHostCountRoundsUp(t *testing.T) {
	if got := requiredHostCount(141, 70); got != 3 {
		t.Fatalf("requiredHostCount = %d, want 3", got)
	}
	if got := requiredHostCount(0, 70); got != 0 {
		t.Fatalf("requiredHostCount for no demand = %d, want 0", got)
	}
}

func TestAnalyzeSoakStabilityRejectsResourceGrowth(t *testing.T) {
	run := syntheticExtendedRun(
		capacityEnvironment{}, "soak-1000", 1000, 5*time.Minute, time.Hour, 1, true,
	)
	evidence := run.Extended.Roles["server"]
	lateStart := run.PingRounds[len(run.PingRounds)-1].StartedAt.Add(-10 * time.Minute)
	for index := range evidence.Samples {
		if !evidence.Samples[index].At.Before(lateStart) {
			evidence.Samples[index].RSSBytes *= 2
			evidence.Samples[index].OpenFDs *= 2
		}
	}
	evidence.Summary = summarizeRoleResources(evidence.Samples)
	run.Extended.Roles["server"] = evidence
	got, err := analyzeSoakStability(run, "server")
	if err != nil {
		t.Fatal(err)
	}
	if got.Qualified || !strings.Contains(got.Reason, "RSS growth") || !strings.Contains(got.Reason, "open-FD growth") {
		t.Fatalf("soak stability = %+v", got)
	}
}

func TestValidateRoleEvidenceCoverageRequiresConfiguredHoldEnd(t *testing.T) {
	run := syntheticExtendedRun(
		capacityEnvironment{}, "sessions-100", 100, 30*time.Second, 5*time.Minute, 1, false,
	)
	evidence := run.Extended.Roles["server"]
	evidence.Samples = evidence.Samples[:len(evidence.Samples)-1]
	if err := validateRoleEvidenceCoverage(run, evidence); err == nil || !strings.Contains(err.Error(), "complete hold interval") {
		t.Fatalf("coverage error = %v, want complete hold interval", err)
	}
}

func syntheticExtendedRun(
	environment capacityEnvironment,
	scenario string,
	sessions int,
	ramp time.Duration,
	hold time.Duration,
	repetition int,
	soak bool,
) artifact {
	roles := make(map[string]roleResourceEvidence)
	for _, role := range []string{"load_driver", "edge", "edge2", "server"} {
		roleSessions := sessions
		if role == "edge" || role == "edge2" {
			roleSessions /= 2
		}
		start := time.Unix(1, 0)
		averageCPU := 0.01 + float64(roleSessions)*0.00001
		points := make([]roleResourcePoint, 0, int(hold/time.Second)+1)
		for elapsed := time.Duration(0); elapsed <= hold; elapsed += time.Second {
			point := roleResourcePoint{
				At: start.Add(elapsed), RSSBytes: uint64(1000 + roleSessions*10),
				RSSSource: "test_process_rss", CPUSeconds: averageCPU * elapsed.Seconds(),
				CPUSecondsSource: "test_process_cpu", OpenFDs: 10 + roleSessions/100,
				OpenFDsSource: "test_process_fds",
			}
			if role != "load_driver" {
				point.SocketSource = "proc_pid_net_udp"
				point.NetworkSource = "proc_pid_net_dev"
			}
			points = append(points, point)
		}
		imageID := ""
		if role != "load_driver" {
			imageID = "sha256:" + role
		}
		containerID := ""
		if role != "load_driver" {
			containerID = "container-" + role
		}
		roles[role] = roleResourceEvidence{
			Role: role, ContainerID: containerID, ImageID: imageID, ProcessID: 1, ProcessStartTicks: 1,
			OpenFDLimit: 1_000_000, Samples: points, Summary: summarizeRoleResources(points),
		}
	}
	run := artifact{
		Version: extendedArtifactVersion,
		Host: hostSummary{
			GOOS: "test", GOARCH: "arm64", GoVersion: "go-test", LogicalCPU: 100_000, GOMAXPROCS: 8, GOGC: "100",
		},
		Config: artifactConfig{
			Edges: []string{"edge-a", "edge-b"}, Sessions: sessions, Ramp: ramp,
			Duration: hold, PingInterval: 30 * time.Second, Concurrency: 512,
			MaxPingRoundDuration: 30 * time.Second, RequireBalancedEdges: true,
			MaxSessionsPerEdge: 30000, RequiredUpstreamsPerEdge: sampledUpstreamsPerEdge,
			MaxUpstreamsPerEdge:    16,
			MaxSessionsPerUpstream: sessionsPerUpstreamLimit,
			Scenario:               scenario, Repetition: repetition, Soak: soak,
		},
		Extended:  &extendedRunEvidence{Environment: environment, Roles: roles},
		Attempted: sessions, Established: sessions,
		EdgeDistribution:     map[string]int{"edge-a": sessions / 2, "edge-b": sessions / 2},
		UpstreamDistribution: syntheticUpstreamDistribution(sessions),
		Passed:               true,
	}
	roundCount := int(hold / (30 * time.Second))
	for round := range roundCount {
		run.PingRounds = append(run.PingRounds, pingRoundSummary{
			Phase: "hold", Round: round, StartedAt: time.Unix(1, 0).Add(time.Duration(round) * 30 * time.Second),
			Duration: time.Second, Attempted: sessions,
		})
		run.PingsAttempted += sessions
	}
	return run
}

func syntheticUpstreamDistribution(sessions int) map[string]map[string]int {
	distribution := map[string]map[string]int{"edge-a": {}, "edge-b": {}}
	for _, edge := range []string{"edge-a", "edge-b"} {
		remaining := sessions / 2
		for index := range sampledUpstreamsPerEdge {
			assigned := remaining / (sampledUpstreamsPerEdge - index)
			distribution[edge][fmt.Sprintf("upstream-%d", index)] = assigned
			remaining -= assigned
		}
	}
	return distribution
}
