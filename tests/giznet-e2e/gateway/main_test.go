package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
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

	_, err = fetchEdges(context.Background(), []string{server.URL, server.URL})
	if err == nil || !strings.Contains(err.Error(), "duplicates transport identity") {
		t.Fatalf("fetchEdges error = %v, want duplicate transport identity", err)
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
		sessions:    2,
		ramp:        time.Hour,
		dialTimeout: time.Second,
		concurrency: 1,
	}
	attempts := 0
	err := establishSessions(
		ctx,
		opts,
		[]edgeMetadata{{endpoint: session.edge}},
		state,
		make(chan struct{}, opts.concurrency),
		func(context.Context, edgeMetadata, int, time.Duration) (*liveSession, error) {
			attempts++
			cancel()
			return session, nil
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
