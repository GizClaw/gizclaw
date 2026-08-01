package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	projectionArtifactVersion = 1
	projectionHeadroom        = 0.30
	maximumRelativeResidual   = 0.20
	projectedTotalSessions    = 30000
	projectedSessionsPerEdge  = 15000
	projectedUpstreamsPerEdge = 16
	sessionsPerEdgeLimit      = 30000
	sessionsPerUpstreamLimit  = 2048
)

type projectionReport struct {
	Version             int                       `json:"version"`
	GeneratedAt         time.Time                 `json:"generated_at"`
	RepositoryCommit    string                    `json:"repository_commit"`
	Environment         capacityEnvironment       `json:"environment"`
	Inputs              []projectionInput         `json:"inputs"`
	Headroom            float64                   `json:"headroom"`
	MaximumResidual     float64                   `json:"maximum_relative_residual"`
	TotalSessions       int                       `json:"total_sessions"`
	SessionsPerEdge     int                       `json:"sessions_per_edge"`
	UpstreamsPerEdge    int                       `json:"upstreams_per_edge"`
	SessionsPerUpstream float64                   `json:"sessions_per_upstream"`
	Models              map[string]roleProjection `json:"models"`
	Soak                map[string]soakStability  `json:"soak"`
	Budgets             projectionBudgets         `json:"budgets"`
	Qualified           bool                      `json:"qualified"`
	FirstLimitation     string                    `json:"first_limitation,omitempty"`
	Limitations         []string                  `json:"limitations,omitempty"`
}

type projectionInput struct {
	File       string `json:"file"`
	Scenario   string `json:"scenario"`
	Sessions   int    `json:"sessions"`
	Repetition int    `json:"repetition"`
	Soak       bool   `json:"soak"`
}

type roleProjection struct {
	TargetSessions  int           `json:"target_sessions"`
	RSSBytes        resourceModel `json:"rss_bytes"`
	AverageCPUCores resourceModel `json:"average_cpu_cores"`
	OpenFDs         resourceModel `json:"open_fds"`
}

type resourceModel struct {
	Points                  []modelPoint `json:"points"`
	Intercept               float64      `json:"intercept"`
	PerSession              float64      `json:"per_session"`
	Projected               float64      `json:"projected"`
	MaximumRelativeResidual float64      `json:"maximum_relative_residual"`
	Qualified               bool         `json:"qualified"`
	Reason                  string       `json:"reason,omitempty"`
}

type modelPoint struct {
	Sessions int     `json:"sessions"`
	Median   float64 `json:"median"`
	Fitted   float64 `json:"fitted"`
	Residual float64 `json:"relative_residual"`
}

type projectionBudgets struct {
	UsableFraction          float64 `json:"usable_fraction"`
	LoadDriverMemoryBytes   float64 `json:"load_driver_memory_bytes"`
	LoadDriverCPUCores      float64 `json:"load_driver_cpu_cores"`
	LoadDriverOpenFDs       float64 `json:"load_driver_open_fds"`
	RequiredLoadDriverHosts *int    `json:"required_load_driver_hosts"`
	DockerMemoryBytes       float64 `json:"docker_memory_bytes"`
	DockerCPUCores          float64 `json:"docker_cpu_cores"`
	ProjectedDockerRSSBytes float64 `json:"projected_docker_rss_bytes"`
	ProjectedDockerCPUCores float64 `json:"projected_docker_cpu_cores"`
}

type roleObservation struct {
	Sessions        int
	RSSBytes        float64
	AverageCPUCores float64
	OpenFDs         float64
}

type soakStability struct {
	EarlyRSSBytes        float64 `json:"early_rss_bytes"`
	LateRSSBytes         float64 `json:"late_rss_bytes"`
	RSSGrowth            float64 `json:"rss_growth"`
	EarlyOpenFDs         float64 `json:"early_open_fds"`
	LateOpenFDs          float64 `json:"late_open_fds"`
	OpenFDGrowth         float64 `json:"open_fd_growth"`
	EarlyAverageCPUCores float64 `json:"early_average_cpu_cores"`
	LateAverageCPUCores  float64 `json:"late_average_cpu_cores"`
	Qualified            bool    `json:"qualified"`
	Reason               string  `json:"reason,omitempty"`
}

func analyzeCapacityArtifacts(directory string) (projectionReport, error) {
	paths, err := filepath.Glob(filepath.Join(directory, "*.json"))
	if err != nil {
		return projectionReport{}, err
	}
	if len(paths) == 0 {
		return projectionReport{}, fmt.Errorf("no JSON run artifacts in %s", directory)
	}
	slices.Sort(paths)
	runs := make([]artifact, 0, len(paths))
	inputs := make([]projectionInput, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return projectionReport{}, err
		}
		var run artifact
		if err := json.Unmarshal(data, &run); err != nil {
			return projectionReport{}, fmt.Errorf("decode %s: %w", path, err)
		}
		if run.Version != extendedArtifactVersion || run.Extended == nil {
			return projectionReport{}, fmt.Errorf("%s is not an extended version-%d artifact", path, extendedArtifactVersion)
		}
		if !run.Passed {
			return projectionReport{}, fmt.Errorf("%s did not pass its fixed workload", path)
		}
		runs = append(runs, run)
		inputs = append(inputs, projectionInput{
			File: filepath.Base(path), Scenario: run.Config.Scenario,
			Sessions: run.Config.Sessions, Repetition: run.Config.Repetition, Soak: run.Config.Soak,
		})
	}
	if err := validateProjectionRunSet(runs); err != nil {
		return projectionReport{}, err
	}

	environment := runs[0].Extended.Environment
	report := projectionReport{
		Version: projectionArtifactVersion, GeneratedAt: time.Now(),
		RepositoryCommit: environment.RepositoryCommit, Environment: environment,
		Inputs: inputs, Headroom: projectionHeadroom, MaximumResidual: maximumRelativeResidual,
		TotalSessions: projectedTotalSessions, SessionsPerEdge: projectedSessionsPerEdge,
		UpstreamsPerEdge:    projectedUpstreamsPerEdge,
		SessionsPerUpstream: float64(projectedSessionsPerEdge) / projectedUpstreamsPerEdge,
		Models:              make(map[string]roleProjection), Soak: make(map[string]soakStability), Qualified: true,
	}
	for _, role := range []string{"load_driver", "edge", "edge2", "server"} {
		observations, err := observationsForRole(runs, role)
		if err != nil {
			return projectionReport{}, err
		}
		target := projectedTotalSessions
		if role == "edge" || role == "edge2" {
			target = projectedSessionsPerEdge
		}
		projection := roleProjection{
			TargetSessions:  target,
			RSSBytes:        fitResourceModel(observations, target, func(value roleObservation) float64 { return value.RSSBytes }),
			AverageCPUCores: fitResourceModel(observations, target, func(value roleObservation) float64 { return value.AverageCPUCores }),
			OpenFDs:         fitResourceModel(observations, target, func(value roleObservation) float64 { return value.OpenFDs }),
		}
		report.Models[role] = projection
		for _, metric := range []struct {
			name  string
			model resourceModel
		}{
			{name: "rss_bytes", model: projection.RSSBytes},
			{name: "average_cpu_cores", model: projection.AverageCPUCores},
			{name: "open_fds", model: projection.OpenFDs},
		} {
			if !metric.model.Qualified {
				rejectProjection(&report, fmt.Sprintf("%s %s: %s", role, metric.name, metric.model.Reason))
			}
		}
	}
	soakRun := findSoakRun(runs)
	for _, role := range []string{"load_driver", "edge", "edge2", "server"} {
		stability, err := analyzeSoakStability(soakRun, role)
		if err != nil {
			return projectionReport{}, err
		}
		report.Soak[role] = stability
		if !stability.Qualified {
			rejectProjection(&report, role+" soak: "+stability.Reason)
		}
	}
	applyProjectionBudgets(&report, runs)
	if report.SessionsPerEdge > sessionsPerEdgeLimit {
		rejectProjection(&report, fmt.Sprintf(
			"projected %d sessions per Edge exceeds %d",
			report.SessionsPerEdge, sessionsPerEdgeLimit,
		))
	}
	if report.SessionsPerUpstream > sessionsPerUpstreamLimit {
		rejectProjection(&report, fmt.Sprintf(
			"projected %.2f sessions per upstream exceeds %d",
			report.SessionsPerUpstream, sessionsPerUpstreamLimit,
		))
	}
	return report, nil
}

func findSoakRun(runs []artifact) artifact {
	for _, run := range runs {
		if run.Config.Soak {
			return run
		}
	}
	return artifact{}
}

func analyzeSoakStability(run artifact, role string) (soakStability, error) {
	holdRounds := pingRoundsForPhase(run.PingRounds, "hold")
	if !run.Config.Soak || len(holdRounds) == 0 {
		return soakStability{}, errors.New("soak run has no ping-round boundary")
	}
	evidence, ok := run.Extended.Roles[role]
	if !ok {
		return soakStability{}, fmt.Errorf("soak run lacks %s evidence", role)
	}
	holdStart := holdRounds[0].StartedAt
	lastRound := holdRounds[len(holdRounds)-1]
	holdEnd := lastRound.StartedAt.Add(lastRound.Duration)
	window := 10 * time.Minute
	early := resourceWindow(evidence.Samples, holdStart, holdStart.Add(window))
	late := resourceWindow(evidence.Samples, holdEnd.Add(-window), holdEnd)
	if len(early) < 2 || len(late) < 2 {
		return soakStability{}, fmt.Errorf("%s soak lacks two complete 10-minute resource windows", role)
	}
	earlyRSS := medianOf(early, func(value roleResourcePoint) float64 { return float64(value.RSSBytes) })
	lateRSS := medianOf(late, func(value roleResourcePoint) float64 { return float64(value.RSSBytes) })
	earlyFDs := medianOf(early, func(value roleResourcePoint) float64 { return float64(value.OpenFDs) })
	lateFDs := medianOf(late, func(value roleResourcePoint) float64 { return float64(value.OpenFDs) })
	stability := soakStability{
		EarlyRSSBytes: earlyRSS, LateRSSBytes: lateRSS, RSSGrowth: relativeGrowth(earlyRSS, lateRSS),
		EarlyOpenFDs: earlyFDs, LateOpenFDs: lateFDs, OpenFDGrowth: relativeGrowth(earlyFDs, lateFDs),
		EarlyAverageCPUCores: averageCPUCores(early), LateAverageCPUCores: averageCPUCores(late),
		Qualified: true,
	}
	var reasons []string
	if stability.RSSGrowth > maximumRelativeResidual {
		reasons = append(reasons, fmt.Sprintf("RSS growth %.3f exceeds %.2f", stability.RSSGrowth, maximumRelativeResidual))
	}
	if stability.OpenFDGrowth > maximumRelativeResidual {
		reasons = append(reasons, fmt.Sprintf("open-FD growth %.3f exceeds %.2f", stability.OpenFDGrowth, maximumRelativeResidual))
	}
	if len(reasons) > 0 {
		stability.Qualified = false
		stability.Reason = strings.Join(reasons, "; ")
	}
	return stability, nil
}

func resourceWindow(samples []roleResourcePoint, start, end time.Time) []roleResourcePoint {
	window := make([]roleResourcePoint, 0)
	for _, sample := range samples {
		if !sample.At.Before(start) && !sample.At.After(end) {
			window = append(window, sample)
		}
	}
	return window
}

func relativeGrowth(early, late float64) float64 {
	return (late - early) / math.Max(math.Abs(early), 1)
}

func averageCPUCores(samples []roleResourcePoint) float64 {
	if len(samples) < 2 {
		return 0
	}
	duration := samples[len(samples)-1].At.Sub(samples[0].At).Seconds()
	if duration <= 0 {
		return 0
	}
	return max(samples[len(samples)-1].CPUSeconds-samples[0].CPUSeconds, 0) / duration
}

func validateProjectionRunSet(runs []artifact) error {
	expected := map[string]int{"sessions-100": 3, "sessions-500": 3, "sessions-1000": 3, "soak-1000": 1}
	counts := make(map[string]int)
	var environment *capacityEnvironment
	var host *hostSummary
	seenRepetitions := make(map[string]map[int]bool)
	for _, run := range runs {
		if run.Extended == nil || len(run.Extended.Errors) > 0 {
			return errors.New("run contains incomplete extended evidence")
		}
		wantSessions := map[string]int{"sessions-100": 100, "sessions-500": 500, "sessions-1000": 1000, "soak-1000": 1000}[run.Config.Scenario]
		if wantSessions == 0 || run.Config.Sessions != wantSessions {
			return fmt.Errorf("unexpected scenario/session pair %q/%d", run.Config.Scenario, run.Config.Sessions)
		}
		wantSoak := run.Config.Scenario == "soak-1000"
		if run.Config.Soak != wantSoak {
			return fmt.Errorf("scenario %q has soak=%t", run.Config.Scenario, run.Config.Soak)
		}
		if wantSoak && run.Config.Duration != time.Hour {
			return fmt.Errorf("soak duration = %s, want 1h", run.Config.Duration)
		}
		if !wantSoak && run.Config.Duration != 5*time.Minute {
			return fmt.Errorf("step duration = %s, want 5m", run.Config.Duration)
		}
		if run.Config.PingInterval != 30*time.Second || run.Config.Concurrency != 512 || run.Config.SpeedBytes != 0 {
			return errors.New("run does not use the fixed ping, concurrency, and throughput settings")
		}
		wantRamp := map[string]time.Duration{
			"sessions-100": 30 * time.Second, "sessions-500": 150 * time.Second,
			"sessions-1000": 5 * time.Minute, "soak-1000": 5 * time.Minute,
		}[run.Config.Scenario]
		if run.Config.Ramp != wantRamp {
			return fmt.Errorf("scenario %q ramp = %s, want %s", run.Config.Scenario, run.Config.Ramp, wantRamp)
		}
		if run.Config.MaxEstablishmentFailures != 0 || run.Config.MaxPingFailures != 0 ||
			run.Config.MaxP99RTT != 0 || run.Config.MaxPingRoundDuration != 30*time.Second ||
			!run.Config.RequireBalancedEdges || run.Config.MaxSessionsPerEdge != sessionsPerEdgeLimit ||
			run.Config.RequiredUpstreamsPerEdge != projectedUpstreamsPerEdge ||
			run.Config.MaxUpstreamsPerEdge != projectedUpstreamsPerEdge ||
			run.Config.MaxSessionsPerUpstream != sessionsPerUpstreamLimit {
			return errors.New("run does not use the fixed correctness and distribution thresholds")
		}
		if run.Established != run.Config.Sessions || run.EstablishmentFailures != 0 ||
			run.UnexpectedDisconnects != 0 || run.IdentityCrossover || !distributionWithin(run, run.Config) {
			return fmt.Errorf("scenario %q does not satisfy fixed correctness and distribution checks", run.Config.Scenario)
		}
		holdRounds := pingRoundsForPhase(run.PingRounds, "hold")
		minimumRounds := int(run.Config.Duration / run.Config.PingInterval)
		if len(holdRounds) < minimumRounds {
			return fmt.Errorf("scenario %q has %d hold ping rounds, want at least %d", run.Config.Scenario, len(holdRounds), minimumRounds)
		}
		pings := 0
		for _, round := range run.PingRounds {
			if round.Phase != "ramp" && round.Phase != "hold" {
				return fmt.Errorf("scenario %q has unexpected ping phase %q", run.Config.Scenario, round.Phase)
			}
			if round.Attempted <= 0 || round.Attempted > run.Config.Sessions || round.Failures != 0 ||
				round.Duration > run.Config.MaxPingRoundDuration ||
				(round.Phase == "hold" && round.Attempted != run.Config.Sessions) {
				return fmt.Errorf("scenario %q has incomplete or overlapping ping round %d", run.Config.Scenario, round.Round)
			}
			pings += round.Attempted
		}
		if run.PingsAttempted != pings || run.PingFailures != 0 {
			return fmt.Errorf("scenario %q ping totals do not match its round evidence", run.Config.Scenario)
		}
		if seenRepetitions[run.Config.Scenario] == nil {
			seenRepetitions[run.Config.Scenario] = make(map[int]bool)
		}
		if run.Config.Repetition <= 0 || seenRepetitions[run.Config.Scenario][run.Config.Repetition] {
			return fmt.Errorf("scenario %q has invalid or duplicate repetition %d", run.Config.Scenario, run.Config.Repetition)
		}
		seenRepetitions[run.Config.Scenario][run.Config.Repetition] = true
		counts[run.Config.Scenario]++
		current := run.Extended.Environment
		if current.RepositoryCommit == "" || current.HostMemoryBytes == 0 || current.HostOpenFDLimit == 0 ||
			current.Docker.LogicalCPU <= 0 || current.Docker.MemoryBytes == 0 {
			return fmt.Errorf("scenario %q repetition %d lacks a complete environment identity", run.Config.Scenario, run.Config.Repetition)
		}
		if current.RepositoryDirty {
			return fmt.Errorf("scenario %q repetition %d was recorded from a dirty worktree", run.Config.Scenario, run.Config.Repetition)
		}
		if environment == nil {
			environment = &current
		} else if current != *environment {
			return errors.New("run artifacts were produced by different commits or host/Docker budgets")
		}
		currentHost := run.Host
		if currentHost.GOOS == "" || currentHost.GOARCH == "" || currentHost.GoVersion == "" ||
			currentHost.LogicalCPU <= 0 || currentHost.GOMAXPROCS != 8 {
			return fmt.Errorf("scenario %q repetition %d lacks a complete load-driver identity", run.Config.Scenario, run.Config.Repetition)
		}
		if host == nil {
			host = &currentHost
		} else if currentHost != *host {
			return errors.New("run artifacts were produced by different load-driver runtimes")
		}
		for _, role := range []string{"load_driver", "edge", "edge2", "server"} {
			evidence, ok := run.Extended.Roles[role]
			if !ok || evidence.Role != role || evidence.ProcessID <= 0 || evidence.OpenFDLimit == 0 {
				return fmt.Errorf("scenario %q repetition %d lacks complete %s evidence", run.Config.Scenario, run.Config.Repetition, role)
			}
			if err := validateRequiredRoleEvidence(evidence); err != nil {
				return fmt.Errorf("scenario %q repetition %d: %w", run.Config.Scenario, run.Config.Repetition, err)
			}
			if err := validateRoleEvidenceCoverage(run, evidence); err != nil {
				return fmt.Errorf("scenario %q repetition %d: %w", run.Config.Scenario, run.Config.Repetition, err)
			}
			if role != "load_driver" {
				if evidence.ContainerID == "" || evidence.ImageID == "" || evidence.ProcessStartTicks == 0 {
					return fmt.Errorf("scenario %q repetition %d lacks %s container, image, or process identity", run.Config.Scenario, run.Config.Repetition, role)
				}
			}
		}
	}
	for scenario, count := range expected {
		if counts[scenario] != count {
			return fmt.Errorf("scenario %q has %d artifacts, want %d", scenario, counts[scenario], count)
		}
	}
	return nil
}

func pingRoundsForPhase(rounds []pingRoundSummary, phase string) []pingRoundSummary {
	filtered := make([]pingRoundSummary, 0, len(rounds))
	for _, round := range rounds {
		if round.Phase == phase {
			filtered = append(filtered, round)
		}
	}
	return filtered
}

func validateRoleEvidenceCoverage(run artifact, evidence roleResourceEvidence) error {
	holdRounds := pingRoundsForPhase(run.PingRounds, "hold")
	if len(holdRounds) == 0 || len(evidence.Samples) == 0 {
		return fmt.Errorf("%s lacks workload or resource samples", evidence.Role)
	}
	holdStart := holdRounds[0].StartedAt
	lastRound := holdRounds[len(holdRounds)-1]
	holdEnd := lastRound.StartedAt.Add(lastRound.Duration)
	if evidence.Samples[0].At.After(holdStart) || evidence.Samples[len(evidence.Samples)-1].At.Before(holdEnd) {
		return fmt.Errorf("%s resource samples do not cover the complete hold interval", evidence.Role)
	}
	return nil
}

func observationsForRole(runs []artifact, role string) ([]roleObservation, error) {
	bySessions := make(map[int][]roleObservation)
	for _, run := range runs {
		if run.Config.Soak {
			continue
		}
		evidence := run.Extended.Roles[role]
		summary := summarizeRoleResources(evidence.Samples)
		duration := summary.End.At.Sub(summary.Start.At).Seconds()
		if duration <= 0 {
			return nil, fmt.Errorf("%s scenario %q has non-positive sampling duration", role, run.Config.Scenario)
		}
		cpuDelta := summary.End.CPUSeconds - summary.Start.CPUSeconds
		if cpuDelta < 0 {
			return nil, fmt.Errorf("%s scenario %q has decreasing CPU time", role, run.Config.Scenario)
		}
		sessions := run.Config.Sessions
		if role == "edge" || role == "edge2" {
			sessions /= 2
		}
		bySessions[sessions] = append(bySessions[sessions], roleObservation{
			Sessions: sessions, RSSBytes: float64(summary.Peak.RSSBytes),
			AverageCPUCores: cpuDelta / duration, OpenFDs: float64(summary.Peak.OpenFDs),
		})
	}
	keys := make([]int, 0, len(bySessions))
	for sessions := range bySessions {
		keys = append(keys, sessions)
	}
	slices.Sort(keys)
	if len(keys) != 3 {
		return nil, fmt.Errorf("%s has %d session points, want 3", role, len(keys))
	}
	observations := make([]roleObservation, 0, len(keys))
	for _, sessions := range keys {
		values := bySessions[sessions]
		if len(values) != 3 {
			return nil, fmt.Errorf("%s at %d sessions has %d repetitions, want 3", role, sessions, len(values))
		}
		observations = append(observations, roleObservation{
			Sessions:        sessions,
			RSSBytes:        medianOf(values, func(value roleObservation) float64 { return value.RSSBytes }),
			AverageCPUCores: medianOf(values, func(value roleObservation) float64 { return value.AverageCPUCores }),
			OpenFDs:         medianOf(values, func(value roleObservation) float64 { return value.OpenFDs }),
		})
	}
	return observations, nil
}

func medianOf[T any](values []T, selectValue func(T) float64) float64 {
	numbers := make([]float64, len(values))
	for index, value := range values {
		numbers[index] = selectValue(value)
	}
	slices.Sort(numbers)
	return numbers[len(numbers)/2]
}

func fitResourceModel(observations []roleObservation, target int, selectValue func(roleObservation) float64) resourceModel {
	model := resourceModel{Qualified: true}
	var xMean, yMean float64
	for _, observation := range observations {
		xMean += float64(observation.Sessions)
		yMean += selectValue(observation)
	}
	xMean /= float64(len(observations))
	yMean /= float64(len(observations))
	var covariance, variance float64
	for _, observation := range observations {
		xDelta := float64(observation.Sessions) - xMean
		covariance += xDelta * (selectValue(observation) - yMean)
		variance += xDelta * xDelta
	}
	if variance == 0 {
		model.Qualified = false
		model.Reason = "session points do not vary"
		return model
	}
	model.PerSession = covariance / variance
	model.Intercept = yMean - model.PerSession*xMean
	model.Projected = model.Intercept + model.PerSession*float64(target)
	for _, observation := range observations {
		observed := selectValue(observation)
		fitted := model.Intercept + model.PerSession*float64(observation.Sessions)
		residual := relativeResidual(observed, fitted)
		model.MaximumRelativeResidual = max(model.MaximumRelativeResidual, residual)
		model.Points = append(model.Points, modelPoint{
			Sessions: observation.Sessions, Median: observed, Fitted: fitted, Residual: residual,
		})
	}
	var reasons []string
	if model.PerSession < 0 {
		reasons = append(reasons, "negative fitted slope")
	}
	if model.MaximumRelativeResidual > maximumRelativeResidual {
		reasons = append(reasons, fmt.Sprintf("relative residual %.3f exceeds %.2f", model.MaximumRelativeResidual, maximumRelativeResidual))
	}
	if model.Projected < 0 || math.IsNaN(model.Projected) || math.IsInf(model.Projected, 0) {
		reasons = append(reasons, "projected value is not finite and non-negative")
	}
	if len(reasons) > 0 {
		model.Qualified = false
		model.Reason = strings.Join(reasons, "; ")
	}
	return model
}

func relativeResidual(observed, fitted float64) float64 {
	if observed == 0 {
		if fitted == 0 {
			return 0
		}
		return math.Inf(1)
	}
	return math.Abs(fitted-observed) / math.Abs(observed)
}

func applyProjectionBudgets(report *projectionReport, runs []artifact) {
	usable := 1 - projectionHeadroom
	report.Budgets = projectionBudgets{
		UsableFraction:        usable,
		LoadDriverMemoryBytes: float64(report.Environment.HostMemoryBytes) * usable,
		LoadDriverCPUCores:    float64(runtimeLogicalCPU(runs)) * usable,
		LoadDriverOpenFDs:     float64(report.Environment.HostOpenFDLimit) * usable,
		DockerMemoryBytes:     float64(report.Environment.Docker.MemoryBytes) * usable,
		DockerCPUCores:        float64(report.Environment.Docker.LogicalCPU) * usable,
	}
	load := report.Models["load_driver"]
	if load.RSSBytes.Qualified && load.AverageCPUCores.Qualified && load.OpenFDs.Qualified {
		required := max(
			requiredHostCount(load.RSSBytes.Projected, report.Budgets.LoadDriverMemoryBytes),
			requiredHostCount(load.AverageCPUCores.Projected, report.Budgets.LoadDriverCPUCores),
			requiredHostCount(load.OpenFDs.Projected, report.Budgets.LoadDriverOpenFDs),
		)
		report.Budgets.RequiredLoadDriverHosts = &required
	}
	checkBudget(report, "load_driver rss_bytes", load.RSSBytes.Projected, report.Budgets.LoadDriverMemoryBytes)
	checkBudget(report, "load_driver average_cpu_cores", load.AverageCPUCores.Projected, report.Budgets.LoadDriverCPUCores)
	checkBudget(report, "load_driver open_fds", load.OpenFDs.Projected, report.Budgets.LoadDriverOpenFDs)

	for _, role := range []string{"edge", "edge2", "server"} {
		model := report.Models[role]
		report.Budgets.ProjectedDockerRSSBytes += model.RSSBytes.Projected
		report.Budgets.ProjectedDockerCPUCores += model.AverageCPUCores.Projected
		fdLimit := float64(runs[0].Extended.Roles[role].OpenFDLimit) * usable
		checkBudget(report, role+" open_fds", model.OpenFDs.Projected, fdLimit)
	}
	checkBudget(report, "Docker role rss_bytes", report.Budgets.ProjectedDockerRSSBytes, report.Budgets.DockerMemoryBytes)
	checkBudget(report, "Docker role average_cpu_cores", report.Budgets.ProjectedDockerCPUCores, report.Budgets.DockerCPUCores)
}

func requiredHostCount(projected, perHostBudget float64) int {
	if projected <= 0 || perHostBudget <= 0 || math.IsNaN(projected) || math.IsInf(projected, 0) {
		return 0
	}
	return max(int(math.Ceil(projected/perHostBudget)), 1)
}

func runtimeLogicalCPU(runs []artifact) int {
	if len(runs) == 0 {
		return 0
	}
	return runs[0].Host.LogicalCPU
}

func checkBudget(report *projectionReport, label string, projected, budget float64) {
	if budget <= 0 {
		rejectProjection(report, label+": budget unavailable")
		return
	}
	if projected > budget {
		rejectProjection(report, fmt.Sprintf("%s: projected %.2f exceeds 70%% budget %.2f", label, projected, budget))
	}
}

func rejectProjection(report *projectionReport, limitation string) {
	report.Qualified = false
	if report.FirstLimitation == "" {
		report.FirstLimitation = limitation
	}
	report.Limitations = append(report.Limitations, limitation)
}

func writeProjectionArtifact(path string, report projectionReport) error {
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
