package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSummarizeEstablishmentReportsRateLatencyAndPhases(t *testing.T) {
	startedAt := time.Unix(1, 0)
	attempts := make([]establishmentSessionResult, 100)
	for index := range attempts {
		attempts[index] = establishmentSessionResult{
			Index:        99 - index,
			Duration:     time.Duration(index+2) * time.Millisecond,
			DialDuration: time.Duration(index+1) * time.Millisecond,
			Phases: map[string]time.Duration{
				phaseKeyGeneration: time.Duration(index+1) * time.Microsecond,
			},
		}
	}

	got := summarizeEstablishment(startedAt, startedAt.Add(2*time.Second), attempts)
	if got.UsableSessionsPerSecond != 50 {
		t.Fatalf("usable sessions/s = %f, want 50", got.UsableSessionsPerSecond)
	}
	if got.Dial.Count != 100 || got.Dial.P50 != 50 || got.Dial.P95 != 95 || got.Dial.P99 != 99 {
		t.Fatalf("Dial latency = %+v", got.Dial)
	}
	if got.Sessions[0].Index != 0 || got.Sessions[99].Index != 99 {
		t.Fatalf("session order = %d..%d, want 0..99", got.Sessions[0].Index, got.Sessions[99].Index)
	}
	if phase := got.Phases[phaseKeyGeneration]; !phase.Supported || phase.Latency.Count != 100 {
		t.Fatalf("key-generation phase = %+v", phase)
	}
	if phase := got.Phases[phaseClientSCTPConnected]; phase.Supported || phase.Reason == "" {
		t.Fatalf("unsupported SCTP phase = %+v", phase)
	}
}

func TestEstablishmentWithinAppliesRateAndDialGates(t *testing.T) {
	config := artifactConfig{
		MinEstablishmentRate: 20,
		MaxDialP95:           time.Second,
		MaxDialP99:           5 * time.Second,
	}
	passing := establishmentSummary{
		UsableSessionsPerSecond: 20,
		Dial:                    latencySummary{P95: 1000, P99: 5000},
	}
	if !establishmentWithin(passing, config) {
		t.Fatal("establishment at each gate did not pass")
	}
	for name, summary := range map[string]establishmentSummary{
		"rate":           {UsableSessionsPerSecond: 19.99, Dial: passing.Dial},
		"p95":            {UsableSessionsPerSecond: 20, Dial: latencySummary{P95: 1001, P99: 5000}},
		"p95 fractional": {UsableSessionsPerSecond: 20, Dial: latencySummary{P95: 1000.0000001, P99: 5000}},
		"p99":            {UsableSessionsPerSecond: 20, Dial: latencySummary{P95: 1000, P99: 5001}},
		"p99 fractional": {UsableSessionsPerSecond: 20, Dial: latencySummary{P95: 1000, P99: 5000.0000001}},
	} {
		t.Run(name, func(t *testing.T) {
			if establishmentWithin(summary, config) {
				t.Fatalf("establishment passed gate: %+v", summary)
			}
		})
	}
}

func TestParseServerTimingReportsSupportedPhases(t *testing.T) {
	got := parseServerTiming(
		"giz_peer_connection;dur=1.250, ignored;dur=99, " +
			"giz_ice_gathering;dur=1000.125;desc=answer, giz_rewrite_sdp;dur=invalid",
	)
	if got[phaseServerPeerConnection] != 1250*time.Microsecond {
		t.Fatalf("peer-connection timing = %s", got[phaseServerPeerConnection])
	}
	if got[phaseServerICEGathering] != 1000125*time.Microsecond {
		t.Fatalf("ICE-gathering timing = %s", got[phaseServerICEGathering])
	}
	if _, ok := got[phaseServerRewriteSDP]; ok {
		t.Fatal("invalid rewrite timing was retained")
	}
}

func TestEstablishSessionsReleasesBurstTogether(t *testing.T) {
	const sessionCount = 100
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var entered atomic.Int32
	release := make(chan struct{})
	var releaseOnce sync.Once
	state := &resultState{
		edgeDistribution:     make(map[string]int),
		upstreamDistribution: make(map[string]map[string]int),
	}
	opts := options{
		sessions:    sessionCount,
		ramp:        0,
		dialTimeout: time.Second,
		concurrency: sessionCount,
	}
	err := establishSessions(
		ctx,
		opts,
		[]edgeMetadata{{endpoint: "edge-a"}, {endpoint: "edge-b"}},
		state,
		make(chan struct{}, sessionCount),
		func(_ context.Context, edge edgeMetadata, index int, _ time.Duration) (*liveSession, establishmentSessionResult, error) {
			if entered.Add(1) == sessionCount {
				releaseOnce.Do(func() { close(release) })
			}
			select {
			case <-release:
			case <-ctx.Done():
				return nil, establishmentSessionResult{}, ctx.Err()
			}
			closed := make(chan struct{})
			return &liveSession{
					edge:     edge.endpoint,
					upstream: fmt.Sprintf("upstream-%d", index%16),
					serveFn: func() error {
						<-closed
						return context.Canceled
					},
					closeFn: func() error {
						close(closed)
						return nil
					},
				}, establishmentSessionResult{
					Upstream:     fmt.Sprintf("upstream-%d", index%16),
					DialDuration: time.Millisecond,
					Phases:       map[string]time.Duration{phaseClientDial: time.Millisecond},
				}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if entered.Load() != sessionCount || len(state.sessions) != sessionCount {
		t.Fatalf("burst established %d/%d sessions", len(state.sessions), entered.Load())
	}
	if state.establishment.Dial.Count != sessionCount || len(state.establishment.Sessions) != sessionCount {
		t.Fatalf("establishment summary = %+v", state.establishment)
	}
	closeSessions(state, time.Second)
}
