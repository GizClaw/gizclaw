package gizedge

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

// TestUpstreamTransportAcquireSlotSemantics covers the concurrency-slot
// bookkeeping that bounds forwarded requests per upstream association.
func TestUpstreamTransportAcquireSlotSemantics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tr := &upstreamTransport{ctx: ctx}

	releases := make([]func(), 0, maxConcurrentUpstreamRequests)
	for i := range maxConcurrentUpstreamRequests {
		release, err := tr.acquireSlot(context.Background())
		if err != nil {
			t.Fatalf("acquireSlot %d error = %v", i, err)
		}
		releases = append(releases, release)
	}

	// With every slot held, a caller whose request context is canceled must
	// fail fast instead of consuming a slot.
	reqCtx, reqCancel := context.WithCancel(context.Background())
	reqCancel()
	if _, err := tr.acquireSlot(reqCtx); err == nil {
		t.Fatal("acquireSlot returned a slot despite a canceled request context")
	}

	// Releasing one slot admits exactly one waiter.
	releases[0]()
	release, err := tr.acquireSlot(context.Background())
	if err != nil {
		t.Fatalf("acquireSlot after release error = %v", err)
	}
	releases[0] = release

	// A double release must not leak capacity beyond the bound.
	releases[1]()
	releases[1]()
	got := 0
	for {
		r, err := tr.acquireSlot(reqCtxTimeout(t, 200*time.Millisecond))
		if err != nil {
			break
		}
		releases = append(releases, r)
		got++
		if got > maxConcurrentUpstreamRequests {
			t.Fatalf("acquired more than one extra slot after double release")
		}
	}
	if got != 1 {
		t.Fatalf("double release changed available capacity: extra slots = %d, want 1", got)
	}

	// The transport lifetime context unblocks a pending waiter.
	cancel()
	if _, err := tr.acquireSlot(context.Background()); err == nil {
		t.Fatal("acquireSlot returned a slot after the transport context was canceled")
	}
}

func reqCtxTimeout(t *testing.T, d time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

// TestUpstreamTransportBoundsConcurrentForwardedRequests drives real service
// DataChannels over a live gizwebrtc association and asserts that a burst of
// forwarded requests never opens more than maxConcurrentUpstreamRequests
// concurrent upstream streams. This is the Edge-side guard that keeps a burst
// from starving the Server's serial DataChannel accept loop, which the
// production incident wedged permanently.
func TestUpstreamTransportBoundsConcurrentForwardedRequests(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	edgeKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := (&gizwebrtc.ListenConfig{
		SecurityPolicy: gatewayAllowAllPolicy{},
		GatewaySCTPPeer: func(context.Context, giznet.PublicKey) bool {
			return true
		},
	}).Listen(serverKey)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	signaling := httptest.NewServer(listener.SignalingHandler())
	defer signaling.Close()

	accepted := make(chan giznet.Conn, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- conn
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	clientListener, clientConn, err := gizwebrtc.Dial(ctx, edgeKey, serverKey.Public, gizwebrtc.DialConfig{
		SignalingURL:          signaling.URL + gizwebrtc.SignalingPath,
		SecurityPolicy:        gatewayAllowAllPolicy{},
		SCTPReceiveBufferSize: gizwebrtc.GatewaySCTPReceiveBufferSize,
	})
	if err != nil {
		t.Fatalf("Dial error = %v", err)
	}
	defer clientListener.Close()
	defer clientConn.Close()

	var serverConn giznet.Conn
	select {
	case serverConn = <-accepted:
	case <-time.After(10 * time.Second):
		t.Fatal("server did not accept the edge association")
	}
	defer serverConn.Close()

	var inFlight atomic.Int64
	var maxInFlight atomic.Int64
	release := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := inFlight.Add(1)
		for {
			m := maxInFlight.Load()
			if n <= m || maxInFlight.CompareAndSwap(m, n) {
				break
			}
		}
		<-release
		inFlight.Add(-1)
		w.WriteHeader(http.StatusOK)
	})
	server := gizhttp.NewServer(serverConn, gizclaw.ServiceEdgeHTTP, handler)
	go func() { _ = server.Serve() }()
	defer server.Shutdown(context.Background())

	tr := &upstreamTransport{ctx: ctx, conn: clientConn, connEpoch: 1}

	const burst = maxConcurrentUpstreamRequests * 3
	var wg sync.WaitGroup
	respErr := make(chan error, burst)
	for range burst {
		wg.Go(func() {
			req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, "http://gizclaw/probe", nil)
			if reqErr != nil {
				respErr <- reqErr
				return
			}
			resp, rtErr := tr.RoundTrip(req)
			if rtErr != nil {
				respErr <- rtErr
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			respErr <- nil
		})
	}

	// Let the burst pile up against the bound, then confirm the server never
	// saw more than the allowed number of concurrent forwarded requests.
	deadline := time.After(10 * time.Second)
	for inFlight.Load() < maxConcurrentUpstreamRequests {
		select {
		case <-deadline:
			t.Fatalf("only %d concurrent requests reached the server, want %d", inFlight.Load(), maxConcurrentUpstreamRequests)
		case <-time.After(5 * time.Millisecond):
		}
	}
	time.Sleep(200 * time.Millisecond)
	if peak := maxInFlight.Load(); peak > maxConcurrentUpstreamRequests {
		t.Fatalf("concurrent forwarded requests = %d, exceeds bound %d", peak, maxConcurrentUpstreamRequests)
	}

	close(release)
	wg.Wait()
	close(respErr)
	failures := 0
	for err := range respErr {
		if err != nil {
			failures++
		}
	}
	if failures != 0 {
		t.Fatalf("%d/%d forwarded requests failed", failures, burst)
	}
	if peak := maxInFlight.Load(); peak != maxConcurrentUpstreamRequests {
		t.Fatalf("peak concurrency = %d, want exactly the bound %d", peak, maxConcurrentUpstreamRequests)
	}

	// After the burst drains, the association still serves requests: the bound
	// throttles, it does not wedge.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://gizclaw/after", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("post-burst RoundTrip error = %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("post-burst status = %d", resp.StatusCode)
	}
}

// TestRouteResolutionSharesUpstreamConcurrencyBound covers the route RPCs the
// gateway issues per connecting peer. They open service streams on the same
// upstream association as forwarded requests, so they must wait for the same
// slots instead of bypassing the bound during a reconnect storm.
func TestRouteResolutionSharesUpstreamConcurrencyBound(t *testing.T) {
	server := testKeyPair(t, 0xb1).Public
	peer := testKeyPair(t, 0xb2).Public
	conn := &routeRPCGiznetConn{
		failingGiznetConn: &failingGiznetConn{state: giznet.PeerStateEstablished},
		assignment:        &rpcpb.PeerAssignment{PeerPublicKey: peer.String(), ServerPublicKey: server.String()},
	}
	entry := &upstreamTransport{
		ctx:       t.Context(),
		cfg:       Config{selectedUpstream: UpstreamConfig{PublicKey: server}},
		conn:      conn,
		connEpoch: 1,
	}
	resolver := &orderedUpstreamTransport{entries: []*upstreamTransport{entry}}

	releases := make([]func(), 0, maxConcurrentUpstreamRequests)
	for i := range maxConcurrentUpstreamRequests {
		release, err := entry.acquireSlot(t.Context())
		if err != nil {
			t.Fatalf("acquireSlot %d error = %v", i, err)
		}
		releases = append(releases, release)
	}

	if _, err := resolver.resolvePeerAssignment(reqCtxTimeout(t, 100*time.Millisecond), peer); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("resolvePeerAssignment with every slot held error = %v, want context deadline", err)
	}
	if calls := conn.calls.Load(); calls != 0 {
		t.Fatalf("route RPC streams opened while every slot was held = %d, want 0", calls)
	}

	releases[0]()
	assignment, err := resolver.resolvePeerAssignment(reqCtxTimeout(t, 5*time.Second), peer)
	if err != nil {
		t.Fatalf("resolvePeerAssignment after release error = %v", err)
	}
	if assignment.GetServerPublicKey() != server.String() {
		t.Fatalf("assignment server = %q, want %q", assignment.GetServerPublicKey(), server.String())
	}
	if calls := conn.calls.Load(); calls != 1 {
		t.Fatalf("route RPC streams opened = %d, want 1", calls)
	}

	// The RPC returned its slot: exactly one is free again.
	if _, err := entry.acquireSlot(reqCtxTimeout(t, 100*time.Millisecond)); err != nil {
		t.Fatalf("route RPC did not release its slot: %v", err)
	}
	for _, release := range releases[1:] {
		release()
	}
}
