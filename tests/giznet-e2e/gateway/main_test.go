package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
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
