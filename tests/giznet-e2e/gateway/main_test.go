package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
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
