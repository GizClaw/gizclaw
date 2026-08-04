package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func TestNormalizeHTTPBase(t *testing.T) {
	got, err := normalizeHTTPBase("edge.example:9821/path?ignored=1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://edge.example:9821" {
		t.Fatalf("normalizeHTTPBase = %q", got)
	}
	if _, err := normalizeHTTPBase("ftp://edge.example"); err == nil {
		t.Fatal("normalizeHTTPBase accepted unsupported scheme")
	}
}

func TestSummarizeLatencyUsesNearestRank(t *testing.T) {
	values := make([]time.Duration, 100)
	for index := range values {
		values[index] = time.Duration(index+1) * time.Millisecond
	}
	got := summarizeLatency(values)
	if got.Count != 100 || got.P50 != 50 || got.P95 != 95 || got.P99 != 99 || got.Max != 100 {
		t.Fatalf("summarizeLatency = %+v", got)
	}
}

func TestRunOpusUsesSharedBarrierAndExactAccounting(t *testing.T) {
	const sessionCount = 3
	var entered atomic.Int32
	release := make(chan struct{})
	var releaseOnce sync.Once
	sessions := make([]*liveSession, sessionCount)
	for index := range sessions {
		sessions[index] = &liveSession{
			edge: "edge-a", upstream: "upstream-1",
			packetWriteFn: func(protocol byte, payload []byte) (int, error) {
				if protocol != giznet.ProtocolOpusPacket || len(payload) != 3 {
					t.Fatalf("packet = protocol %d bytes %d", protocol, len(payload))
				}
				if entered.Add(1) == sessionCount {
					releaseOnce.Do(func() { close(release) })
				}
				<-release
				return len(payload), nil
			},
		}
	}
	state := &resultState{sessions: sessions}
	runOpusTest(context.Background(), state, options{
		opusPackets: 2, opusPacketBytes: 3, opusInterval: time.Millisecond,
	})
	if state.opus.Attempted != 6 || state.opus.Completed != 6 || state.opus.Failures != 0 ||
		state.opus.AttemptedBytes != 18 || state.opus.CompletedBytes != 18 {
		t.Fatalf("Opus summary = %+v", state.opus)
	}
	if state.opus.Edge["edge-a"].Completed != 6 ||
		state.opus.Upstream["edge-a"]["upstream-1"].Completed != 6 {
		t.Fatalf("Opus path summaries = edge %+v upstream %+v", state.opus.Edge, state.opus.Upstream)
	}
}

func TestSummarizeNestedPingMetrics(t *testing.T) {
	summaries := summarizeNestedLatencyMap(map[string]map[string][]time.Duration{
		"edge-a": {
			"upstream-1": {time.Millisecond, 3 * time.Millisecond},
		},
	})
	got := summaries["edge-a"]["upstream-1"]
	if got.Count != 2 || got.P50 != 1 || got.P99 != 3 {
		t.Fatalf("nested latency summary = %+v", got)
	}
	if got := countNested(map[string]map[string]int{
		"edge-a": {"upstream-1": 2},
		"edge-b": {"upstream-2": 3},
	}); got != 5 {
		t.Fatalf("countNested = %d, want 5", got)
	}
}

func TestActiveCPUSecondsExcludesIdleCapacity(t *testing.T) {
	if got := activeCPUSeconds(12.5, 9.25); got != 3.25 {
		t.Fatalf("activeCPUSeconds = %f, want 3.25", got)
	}
	if got := activeCPUSeconds(1, 2); got != 0 {
		t.Fatalf("activeCPUSeconds with overestimated idle = %f, want 0", got)
	}
}

func TestReadRuntimeResourceMetrics(t *testing.T) {
	totalMemory, heapAlloc, goroutines, cpuSeconds := readRuntimeResourceMetrics()
	if totalMemory == 0 || heapAlloc == 0 || goroutines <= 0 || cpuSeconds < 0 {
		t.Fatalf(
			"runtime resource metrics = total %d heap %d goroutines %d cpu %f",
			totalMemory,
			heapAlloc,
			goroutines,
			cpuSeconds,
		)
	}
}

func TestNonNegativeFinite(t *testing.T) {
	for _, value := range []float64{0, 0.8, 200} {
		if !nonNegativeFinite(value) {
			t.Fatalf("nonNegativeFinite(%v) = false", value)
		}
	}
	for _, value := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if nonNegativeFinite(value) {
			t.Fatalf("nonNegativeFinite(%v) = true", value)
		}
	}
}

func TestEffectiveGOGC(t *testing.T) {
	t.Setenv("GOGC", "")
	if got := effectiveGOGC(); got != "100" {
		t.Fatalf("effectiveGOGC() = %q, want default 100", got)
	}
	t.Setenv("GOGC", " 200 ")
	if got := effectiveGOGC(); got != "200" {
		t.Fatalf("effectiveGOGC() = %q, want 200", got)
	}
}

func TestSummarizeSpeedRunUsesSharedWallClock(t *testing.T) {
	const bytesPerSession = int64(1_000_000)
	sessions := []*liveSession{
		{edge: "edge-a", upstream: "upstream-1"},
		{edge: "edge-b", upstream: "upstream-2"},
	}
	attempts := []speedAttempt{
		{result: gizcli.SpeedTestResult{
			UpBytes: bytesPerSession, UpDuration: time.Second,
		}},
		{result: gizcli.SpeedTestResult{
			UpBytes: bytesPerSession, UpDuration: 2 * time.Second,
		}},
	}
	got := summarizeSpeedRun(
		time.Unix(1, 0),
		2*time.Second,
		sessions,
		"upload",
		bytesPerSession,
		attempts,
	)
	if got.Completed != 2 || got.Failures != 0 || got.TransferredBytes != 2*bytesPerSession {
		t.Fatalf("speed run completion = %+v", got)
	}
	if got.AggregateMbps != 8 {
		t.Fatalf("aggregate Mbps = %f, want 8", got.AggregateMbps)
	}
	if got.PerSessionMbps.Min != 4 || got.PerSessionMbps.Max != 8 {
		t.Fatalf("per-session Mbps = %+v, want min=4 max=8", got.PerSessionMbps)
	}
	if got.Edge["edge-a"].AggregateMbps != 4 ||
		got.Upstream["edge-b"]["upstream-2"].AggregateMbps != 4 {
		t.Fatalf("path summaries = edge %+v upstream %+v", got.Edge, got.Upstream)
	}
	if got.Edge["edge-a"].PerSessionMbps.P50 != 8 ||
		got.Upstream["edge-b"]["upstream-2"].PerSessionMbps.P50 != 4 {
		t.Fatalf("path rate summaries = edge %+v upstream %+v", got.Edge, got.Upstream)
	}
}

func TestSummarizeSpeedRunRejectsIncompleteDownload(t *testing.T) {
	const bytesPerSession = int64(1_000_000)
	got := summarizeSpeedRun(
		time.Unix(1, 0),
		time.Second,
		[]*liveSession{{edge: "edge-a", upstream: "upstream-1"}},
		"download",
		bytesPerSession,
		[]speedAttempt{{result: gizcli.SpeedTestResult{
			DownBytes: bytesPerSession - 1, DownDuration: time.Second,
		}}},
	)
	if got.Completed != 0 || got.Failures != 1 || got.TransferredBytes != 0 {
		t.Fatalf("incomplete download summary = %+v", got)
	}
	if !strings.Contains(got.Sessions[0].Error, "transferred bytes") {
		t.Fatalf("incomplete download error = %q", got.Sessions[0].Error)
	}
}

func TestMeasureSpeedDirectionStartsConcurrentRunTogether(t *testing.T) {
	const (
		sessionCount    = 3
		bytesPerSession = int64(1_000_000)
	)
	var entered atomic.Int32
	release := make(chan struct{})
	var releaseOnce sync.Once
	sessions := make([]*liveSession, sessionCount)
	for index := range sessions {
		sessions[index] = &liveSession{
			edge:     "edge-a",
			upstream: "upstream-1",
			speedFn: func(ctx context.Context, id string, request rpcapi.SpeedTestRequest) (gizcli.SpeedTestResult, error) {
				if strings.Contains(id, ".concurrent.") {
					if entered.Add(1) == sessionCount {
						releaseOnce.Do(func() { close(release) })
					}
					select {
					case <-release:
					case <-ctx.Done():
						return gizcli.SpeedTestResult{}, ctx.Err()
					}
				}
				return gizcli.SpeedTestResult{
					UpContentLength: request.UpContentLength,
					UpBytes:         request.UpContentLength,
					UpDuration:      time.Millisecond,
					Duration:        time.Millisecond,
				}, nil
			},
		}
	}

	got := measureSpeedDirection(
		context.Background(),
		sessions,
		"upload",
		bytesPerSession,
		bytesPerSession,
		time.Second,
		0,
		0,
		"test",
	)
	if entered.Load() != sessionCount {
		t.Fatalf("concurrent starts = %d, want %d", entered.Load(), sessionCount)
	}
	if !got.Passed || got.Concurrent.Completed != sessionCount {
		t.Fatalf("speed direction = %+v", got)
	}
}

func TestSpeedDirectionPassedRequiresRetentionAndAbsoluteFloor(t *testing.T) {
	summary := speedDirectionSummary{
		Baseline: speedRunSummary{Attempted: 1, Completed: 1},
		Concurrent: speedRunSummary{
			Attempted: 100, Completed: 100, AggregateMbps: 250,
		},
		AggregateToBaselineRatio: 0.79,
	}
	if speedDirectionPassed(summary, 0.8, 200) {
		t.Fatal("speedDirectionPassed accepted retention below the configured floor")
	}
	summary.AggregateToBaselineRatio = 0.8
	summary.Concurrent.AggregateMbps = 199.99
	if speedDirectionPassed(summary, 0.8, 200) {
		t.Fatal("speedDirectionPassed accepted aggregate Mbps below the configured floor")
	}
	summary.Concurrent.AggregateMbps = 200
	if !speedDirectionPassed(summary, 0.8, 200) {
		t.Fatal("speedDirectionPassed rejected both thresholds at their configured floors")
	}
}

func TestSummarizeSpeedRetentionRequiresBothDirections(t *testing.T) {
	initial := speedTestSummary{
		Upload: speedDirectionSummary{Concurrent: speedRunSummary{
			AggregateMbps: 250, PerSessionMbps: rateSummary{P50: 1, P95: 2, P99: 4},
		}},
		Download: speedDirectionSummary{Concurrent: speedRunSummary{
			AggregateMbps: 300, PerSessionMbps: rateSummary{P50: 2, P95: 4, P99: 8},
		}},
	}
	final := speedTestSummary{
		Upload: speedDirectionSummary{Concurrent: speedRunSummary{
			AggregateMbps: 200, PerSessionMbps: rateSummary{P50: 0.8, P95: 1.6, P99: 3.2},
		}, Passed: true},
		Download: speedDirectionSummary{Concurrent: speedRunSummary{
			AggregateMbps: 240, PerSessionMbps: rateSummary{P50: 1.6, P95: 3.2, P99: 6.4},
		}, Passed: true},
	}
	got := summarizeSpeedRetention(initial, final, 0.8)
	if !got.Passed || got.UploadRatio != 0.8 || got.DownloadRatio != 0.8 ||
		got.UploadPerSession.P50 != 0.8 || got.UploadPerSession.P95 != 0.8 ||
		got.UploadPerSession.P99 != 0.8 || got.DownloadPerSession.P50 != 0.8 ||
		got.DownloadPerSession.P95 != 0.8 || got.DownloadPerSession.P99 != 0.8 {
		t.Fatalf("speed retention = %+v", got)
	}
	final.Download.Concurrent.PerSessionMbps.P99 = 6.39
	if got := summarizeSpeedRetention(initial, final, 0.8); got.Passed {
		t.Fatalf("speed retention accepted per-session P99 ratio %+v", got)
	}
	final.Download.Concurrent.PerSessionMbps.P99 = 6.4
	final.Download.Concurrent.AggregateMbps = 239
	if got := summarizeSpeedRetention(initial, final, 0.8); got.Passed {
		t.Fatalf("speed retention accepted download ratio %+v", got)
	}
	final.Download.Concurrent.AggregateMbps = 240
	final.Download.Passed = false
	if got := summarizeSpeedRetention(initial, final, 0.8); got.Passed {
		t.Fatalf("speed retention accepted failed final checkpoint %+v", got)
	}
}

func TestHoldSessionsRecordsFinalRoundWithoutDeadlineOverlap(t *testing.T) {
	state := &resultState{}
	opts := options{
		duration:     12 * time.Millisecond,
		pingInterval: 5 * time.Millisecond,
		pingTimeout:  time.Millisecond,
		concurrency:  1,
	}
	if err := holdSessions(t.Context(), state, opts, make(chan struct{}, 1)); err != nil {
		t.Fatal(err)
	}
	if len(state.pingRounds) != 3 {
		t.Fatalf("ping rounds = %+v, want two hold and one final", state.pingRounds)
	}
	if state.pingRounds[0].Phase != "hold" || state.pingRounds[1].Phase != "hold" ||
		state.pingRounds[2].Phase != "final" {
		t.Fatalf("ping phases = %+v", state.pingRounds)
	}
	if state.holdStartedAt.IsZero() || state.holdFinishedAt.Sub(state.holdStartedAt) < opts.duration {
		t.Fatalf("hold boundaries = %s..%s, want at least %s", state.holdStartedAt, state.holdFinishedAt, opts.duration)
	}
}

func TestInitialWorkloadErrorPreservesAggregateFailureAtErrorLimit(t *testing.T) {
	state := &resultState{
		sessions:             []*liveSession{{}},
		edgeDistribution:     map[string]int{"edge": 1},
		upstreamDistribution: map[string]map[string]int{"edge": {"upstream": 1}},
		speedTest: speedTestSummary{
			Upload:   speedDirectionSummary{Passed: true},
			Download: speedDirectionSummary{Passed: false},
		},
	}
	for index := range 100 {
		state.appendErrorLocked(fmt.Sprintf("session failure %d", index))
	}
	err := initialWorkloadError(state, options{
		edges: []string{"edge"}, sessions: 1, speedBytes: 1, requiredUpstreamsPerEdge: 1,
	})
	if err == nil {
		t.Fatal("initialWorkloadError accepted failed download gate")
	}
	if len(state.errors) != 100 || !strings.HasPrefix(state.errors[0], "initial burst gates failed") {
		t.Fatalf("state errors = %d first %q", len(state.errors), state.errors[0])
	}
}

func TestInitialWorkloadErrorRejectsFailedBurstGate(t *testing.T) {
	state := &resultState{
		sessions:             []*liveSession{{}},
		edgeDistribution:     map[string]int{"edge": 1},
		upstreamDistribution: map[string]map[string]int{"edge": {"upstream": 1}},
		speedTest: speedTestSummary{
			Upload:   speedDirectionSummary{Passed: true},
			Download: speedDirectionSummary{Passed: false},
		},
	}
	opts := options{
		edges:                    []string{"edge"},
		sessions:                 1,
		speedBytes:               1,
		requiredUpstreamsPerEdge: 1,
	}
	if err := initialWorkloadError(state, opts); err == nil {
		t.Fatal("initialWorkloadError accepted failed download gate")
	} else if !strings.Contains(err.Error(), "download_passed=false") || len(state.errors) != 1 {
		t.Fatalf("initialWorkloadError = %v, state errors = %v", err, state.errors)
	}
	state.speedTest.Download.Passed = true
	if err := initialWorkloadError(state, opts); err != nil {
		t.Fatalf("initialWorkloadError = %v", err)
	}
}

func TestCloseSessionsReportsFailureAndIsIdempotent(t *testing.T) {
	state := &resultState{sessions: []*liveSession{{
		closeFn: func() error { return errors.New("close failed") },
	}}}
	first := closeSessions(state, time.Second)
	second := closeSessions(state, time.Second)
	if first.CloseFailures != 1 || first.TimedOut || !first.ServeCompleted || len(first.Errors) != 1 {
		t.Fatalf("cleanup summary = %+v", first)
	}
	if second.CloseFailures != first.CloseFailures || second.StartedAt != first.StartedAt {
		t.Fatalf("idempotent cleanup = first %+v second %+v", first, second)
	}
}

func TestCloseSessionsTimesOut(t *testing.T) {
	release := make(chan struct{})
	completed := make(chan struct{})
	state := &resultState{sessions: []*liveSession{{
		closeFn: func() error {
			<-release
			close(completed)
			return nil
		},
	}}}
	got := closeSessions(state, time.Millisecond)
	close(release)
	if !got.TimedOut || got.ServeCompleted || len(got.Errors) != 1 {
		t.Fatalf("cleanup timeout = %+v", got)
	}
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("timed-out cleanup goroutine did not finish")
	}
}

func TestFetchEdgesRejectsDuplicateTransportIdentity(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	transportKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	var endpoint string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		info := apitypes.ServerInfo{
			PublicKey: serverKey.Public.String(),
			Transport: &apitypes.ServerInfoTransport{
				Endpoint:      endpoint,
				Mode:          apitypes.ServerInfoTransportModeEdgeGateway,
				PublicKey:     transportKey.Public.String(),
				SignalingPath: "/offer",
			},
		}
		if err := json.NewEncoder(w).Encode(info); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	endpoint = server.URL

	_, err = fetchEdges(context.Background(), []string{server.URL, server.URL}, false)
	if err == nil || !strings.Contains(err.Error(), "duplicates transport identity") {
		t.Fatalf("fetchEdges error = %v, want duplicate transport identity", err)
	}
}

func TestFetchEdgesUsesSelectedEndpointForSignaling(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	transportKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		info := apitypes.ServerInfo{
			PublicKey: serverKey.Public.String(),
			Transport: &apitypes.ServerInfoTransport{
				Endpoint:      "https://published.example:9821",
				Mode:          apitypes.ServerInfoTransportModeEdgeGateway,
				PublicKey:     transportKey.Public.String(),
				SignalingPath: "/offer",
			},
		}
		if err := json.NewEncoder(w).Encode(info); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	edges, err := fetchEdges(context.Background(), []string{server.URL}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := edges[0].signalingURL, "https://published.example:9821/offer"; got != want {
		t.Fatalf("default signaling URL = %q, want advertised endpoint %q", got, want)
	}
	edges, err = fetchEdges(context.Background(), []string{server.URL}, true)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := edges[0].signalingURL, server.URL+"/offer"; got != want {
		t.Fatalf("overridden signaling URL = %q, want selected endpoint %q", got, want)
	}
}

func TestEstablishSessionsClosesEstablishedSessionsWhenRampIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	closed := make(chan struct{})
	serveExited := make(chan struct{})
	var closeOnce sync.Once
	session := &liveSession{
		edge:     "edge.example:9821",
		upstream: "1",
		serveFn: func() error {
			<-closed
			close(serveExited)
			return context.Canceled
		},
		closeFn: func() error {
			closeOnce.Do(func() { close(closed) })
			return nil
		},
	}
	state := &resultState{
		edgeDistribution:     make(map[string]int),
		upstreamDistribution: make(map[string]map[string]int),
	}
	opts := options{
		sessions:       2,
		ramp:           time.Hour,
		dialTimeout:    time.Second,
		concurrency:    1,
		cleanupTimeout: time.Second,
	}
	attempts := 0
	err := establishSessions(
		ctx,
		opts,
		[]edgeMetadata{{endpoint: session.edge}},
		state,
		make(chan struct{}, opts.concurrency),
		func(context.Context, edgeMetadata, int, time.Duration) (*liveSession, establishmentSessionResult, error) {
			attempts++
			cancel()
			return session, establishmentSessionResult{}, nil
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("establishSessions error = %v, want context canceled", err)
	}
	if attempts != 1 {
		t.Fatalf("establishment attempts = %d, want 1", attempts)
	}
	if !session.closed.Load() {
		t.Fatal("established session was not closed")
	}
	select {
	case <-serveExited:
	default:
		t.Fatal("session Serve goroutine was still running")
	}
}

func TestPingSessionBatchesPreserveOrderAndConcurrency(t *testing.T) {
	sessions := make([]*liveSession, 1000)
	for index := range sessions {
		sessions[index] = &liveSession{edge: strconv.Itoa(index)}
	}
	batches := pingSessionBatches(sessions, 512)
	if len(batches) != 2 {
		t.Fatalf("batch count = %d, want 2", len(batches))
	}
	if len(batches[0]) != 512 || len(batches[1]) != 488 {
		t.Fatalf("batch sizes = %d/%d, want 512/488", len(batches[0]), len(batches[1]))
	}
	for index, session := range append(batches[0], batches[1]...) {
		if session.edge != strconv.Itoa(index) {
			t.Fatalf("session %d edge = %q", index, session.edge)
		}
	}
	if batches := pingSessionBatches(sessions, 0); batches != nil {
		t.Fatalf("zero-concurrency batches = %v, want nil", batches)
	}
}

func TestCounterDelta(t *testing.T) {
	if got := counterDelta(10, 14); got != 4 {
		t.Fatalf("counterDelta(10, 14) = %d, want 4", got)
	}
	if got := counterDelta(14, 10); got != 0 {
		t.Fatalf("counterDelta(14, 10) = %d, want 0", got)
	}
}
