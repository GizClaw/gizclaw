package gizedge

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

// silentUDPProxy sits between the Edge and the upstream ICE socket. Silencing
// a flow drops its DTLS records (and therefore SCTP and every DataChannel)
// in both directions while STUN keeps passing, so ICE consent stays healthy
// and neither side sees a close. This is the production failure: the
// association stops passing traffic without any error.
type silentUDPProxy struct {
	conn   *net.UDPConn
	target *net.UDPAddr

	mu    sync.Mutex
	flows map[string]*silentUDPFlow
	wg    sync.WaitGroup
}

type silentUDPFlow struct {
	client   *net.UDPAddr
	upstream *net.UDPConn
	silent   atomic.Bool
}

func newSilentUDPProxy(t *testing.T, target string) *silentUDPProxy {
	t.Helper()
	targetAddr, err := net.ResolveUDPAddr("udp", target)
	if err != nil {
		t.Fatalf("ResolveUDPAddr error = %v", err)
	}
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("ListenUDP proxy error = %v", err)
	}
	proxy := &silentUDPProxy{conn: conn, target: targetAddr, flows: make(map[string]*silentUDPFlow)}
	proxy.wg.Go(proxy.serveClients)
	t.Cleanup(func() {
		_ = conn.Close()
		proxy.mu.Lock()
		for _, flow := range proxy.flows {
			_ = flow.upstream.Close()
		}
		proxy.mu.Unlock()
		proxy.wg.Wait()
	})
	return proxy
}

func (p *silentUDPProxy) addr() string { return p.conn.LocalAddr().String() }

// isDTLSRecord demultiplexes per RFC 7983: STUN starts with 0-3, DTLS 20-63.
func isDTLSRecord(packet []byte) bool {
	return len(packet) > 0 && packet[0] >= 20 && packet[0] <= 63
}

func (p *silentUDPProxy) serveClients() {
	buf := make([]byte, 64*1024)
	for {
		n, client, err := p.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		flow, err := p.flow(client)
		if err != nil {
			return
		}
		if flow.silent.Load() && isDTLSRecord(buf[:n]) {
			continue
		}
		_, _ = flow.upstream.Write(buf[:n])
	}
}

func (p *silentUDPProxy) flow(client *net.UDPAddr) (*silentUDPFlow, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if flow := p.flows[client.String()]; flow != nil {
		return flow, nil
	}
	upstream, err := net.DialUDP("udp", nil, p.target)
	if err != nil {
		return nil, err
	}
	flow := &silentUDPFlow{client: client, upstream: upstream}
	p.flows[client.String()] = flow
	p.wg.Go(func() {
		buf := make([]byte, 64*1024)
		for {
			n, err := upstream.Read(buf)
			if err != nil {
				return
			}
			if flow.silent.Load() && isDTLSRecord(buf[:n]) {
				continue
			}
			_, _ = p.conn.WriteToUDP(buf[:n], flow.client)
		}
	})
	return flow, nil
}

// silenceExisting silences every current association; later dials get new
// ICE flows and work, as a fresh association did after the production restart.
func (p *silentUDPProxy) silenceExisting() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, flow := range p.flows {
		flow.silent.Store(true)
	}
	return len(p.flows)
}

type syncLogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func captureEdgeLogs(t *testing.T) *syncLogBuffer {
	t.Helper()
	output := &syncLogBuffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return output
}

// startSilenceableUpstream runs an upstream Server whose ICE socket is only
// reachable through a silentUDPProxy and which serves Edge HTTP on every
// accepted association.
func startSilenceableUpstream(t *testing.T, upstreamKey *giznet.KeyPair) (*silentUDPProxy, *httptest.Server) {
	t.Helper()
	probe, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket error = %v", err)
	}
	iceAddr := probe.LocalAddr().String()
	if err := probe.Close(); err != nil {
		t.Fatalf("Close probe error = %v", err)
	}
	proxy := newSilentUDPProxy(t, iceAddr)
	listener, err := (&gizwebrtc.ListenConfig{
		ICEUDPAddr:       iceAddr,
		PublicICEUDPAddr: proxy.addr(),
		ICELite:          true,
		SecurityPolicy: edgeTestSecurityPolicy{
			allowService: func(_ giznet.PublicKey, service uint64) bool {
				return service == gizclaw.ServiceEdgeHTTP
			},
		},
	}).Listen(upstreamKey)
	if err != nil {
		t.Fatalf("Listen upstream error = %v", err)
	}
	var serving sync.WaitGroup
	serving.Go(func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			serving.Go(func() {
				defer conn.Close()
				server := gizhttp.NewServer(conn, gizclaw.ServiceEdgeHTTP, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, _ = w.Write([]byte("ok"))
				}))
				_ = server.Serve()
			})
		}
	})
	signaling := httptest.NewServer(listener.SignalingHandler())
	t.Cleanup(func() {
		signaling.Close()
		_ = listener.Close()
		serving.Wait()
	})
	return proxy, signaling
}

func TestUpstreamTransportEvictsSilentUpstreamAndRedials(t *testing.T) {
	logs := captureEdgeLogs(t)
	edgeKey := testKeyPair(t, 0x7b)
	upstreamKey := testKeyPair(t, 0x7c)
	proxy, signaling := startSilenceableUpstream(t, upstreamKey)

	cfg := Config{
		KeyPair:          edgeKey,
		selectedUpstream: UpstreamConfig{Endpoint: signaling.URL, PublicKey: upstreamKey.Public},
	}
	upstreamURL, err := cfg.selectedUpstreamURL()
	if err != nil {
		t.Fatalf("selectedUpstreamURL error = %v", err)
	}
	transport := &upstreamTransport{
		ctx:         t.Context(),
		cfg:         cfg,
		upstreamURL: upstreamURL,
		liveness: upstreamLivenessConfig{
			interval:       300 * time.Millisecond,
			timeout:        500 * time.Millisecond,
			stallDelay:     200 * time.Millisecond,
			backoffInitial: 50 * time.Millisecond,
		},
	}
	if _, _, err := transport.currentConn(); err != nil {
		t.Fatalf("initial dial error = %v", err)
	}
	transport.startLivenessMonitor()
	defer transport.Close()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	if got := edgeHTTPGetBody(t, client); got != "ok" {
		t.Fatalf("healthy body = %q", got)
	}
	firstConn, firstEpoch := transportConn(t, transport)

	// A request that arrives while the upstream is silent must not hang until
	// the client timeout: the stalled association is detected, evicted and the
	// request is retried on a fresh one.
	if silenced := proxy.silenceExisting(); silenced == 0 {
		t.Fatal("proxy carried no upstream flow")
	}
	started := time.Now()
	if got := edgeHTTPGetBody(t, client); got != "ok" {
		t.Fatalf("body after silent upstream = %q", got)
	}
	if elapsed := time.Since(started); elapsed > 4*time.Second {
		t.Fatalf("request on silent upstream took %s, want fail-over well before the client timeout", elapsed)
	}
	secondConn, secondEpoch := transportConn(t, transport)
	if secondEpoch <= firstEpoch || secondConn == firstConn {
		t.Fatalf("upstream epoch = %d after silence, want a redial past %d", secondEpoch, firstEpoch)
	}
	if info := firstConn.PeerInfo(); info == nil || info.State != giznet.PeerStateOffline {
		t.Fatal("silent upstream association was not closed")
	}

	// With no traffic at all, the periodic probe must still find the stall
	// and bring up a replacement before the next request arrives.
	proxy.silenceExisting()
	deadline := time.Now().Add(5 * time.Second)
	for {
		transport.mu.Lock()
		epoch, conn := transport.connEpoch, transport.conn
		transport.mu.Unlock()
		if epoch > secondEpoch && conn != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("idle silent upstream was not replaced: epoch=%d", epoch)
		}
		time.Sleep(10 * time.Millisecond)
	}
	started = time.Now()
	if got := edgeHTTPGetBody(t, client); got != "ok" {
		t.Fatalf("body after idle replacement = %q", got)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("request after idle replacement took %s", elapsed)
	}

	output := logs.String()
	for _, want := range []string{
		`msg="edge: upstream stalled"`, "trigger=slow_request", "trigger=periodic",
		`msg="edge: upstream evicted"`, "reason=liveness_probe_failed",
		`msg="edge: upstream redialing"`, `msg="edge: upstream ICE selected"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("edge logs do not contain %q:\n%s", want, output)
		}
	}
}

func transportConn(t *testing.T, transport *upstreamTransport) (giznet.Conn, uint64) {
	t.Helper()
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if transport.conn == nil {
		t.Fatal("transport has no upstream connection")
	}
	return transport.conn, transport.connEpoch
}

func TestUpstreamTransportRedialBacksOffWhileUpstreamIsDown(t *testing.T) {
	var dials atomic.Int32
	signaling := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		dials.Add(1)
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer signaling.Close()
	upstreamKey := testKeyPair(t, 0x7d)
	cfg := Config{
		KeyPair:          testKeyPair(t, 0x7e),
		selectedUpstream: UpstreamConfig{Endpoint: signaling.URL, PublicKey: upstreamKey.Public},
	}
	upstreamURL, err := cfg.selectedUpstreamURL()
	if err != nil {
		t.Fatalf("selectedUpstreamURL error = %v", err)
	}
	// Epoch 1 was connected and has been evicted; the Server is now down.
	transport := &upstreamTransport{
		ctx:         t.Context(),
		cfg:         cfg,
		upstreamURL: upstreamURL,
		connEpoch:   1,
		liveness: upstreamLivenessConfig{
			interval:       20 * time.Millisecond,
			backoffInitial: 40 * time.Millisecond,
			backoffMaximum: 160 * time.Millisecond,
		},
	}
	transport.startLivenessMonitor()
	time.Sleep(time.Second)
	if err := transport.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
	// Without backoff the 20 ms interval would redial ~50 times; with a 40 ms
	// initial delay capped at 160 ms it stays near 1s/160ms.
	if got := dials.Load(); got < 3 || got > 15 {
		t.Fatalf("redial attempts in 1s = %d, want bounded exponential backoff", got)
	}
	settled := dials.Load()
	time.Sleep(100 * time.Millisecond)
	if dials.Load() != settled {
		t.Fatal("closed transport kept redialing")
	}
}

type silentProbeConn struct {
	failingGiznetConn
	closed atomic.Bool
}

func (c *silentProbeConn) Close() error {
	c.closed.Store(true)
	return nil
}

func TestGatewayPoolEvictsStalledUpstreamAndReplenishes(t *testing.T) {
	logs := captureEdgeLogs(t)
	dead := &silentProbeConn{failingGiznetConn: failingGiznetConn{state: giznet.PeerStateEstablished}}
	pool := &gatewayPool{
		ctx: t.Context(),
		cfg: Config{Gateway: GatewayConfig{MaxUpstreams: 16, SessionsPerUpstream: 8}},
		newUpstream: func(context.Context) (*gatewayUpstream, error) {
			return &gatewayUpstream{conn: &silentProbeConn{
				failingGiznetConn: failingGiznetConn{state: giznet.PeerStateEstablished},
			}}, nil
		},
		liveness: upstreamLivenessConfig{
			interval: 20 * time.Millisecond,
			timeout:  50 * time.Millisecond,
			probe: func(ctx context.Context, conn giznet.Conn) error {
				if conn == dead {
					<-ctx.Done()
					return ctx.Err()
				}
				return nil
			},
		},
	}
	stalled := &gatewayUpstream{id: 1, pool: pool, conn: dead}
	pool.entries = []*gatewayUpstream{stalled}
	pool.nextID = 1
	go pool.monitorLiveness()

	deadline := time.Now().Add(3 * time.Second)
	for {
		pool.mu.Lock()
		stillSelectable := false
		for _, entry := range pool.entries {
			if entry == stalled {
				stillSelectable = true
			}
		}
		healthy := pool.selectableCountLocked()
		pool.mu.Unlock()
		if !stillSelectable && healthy == gatewayPoolWarmUpstreams && dead.closed.Load() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("stalled upstream selectable=%t healthy=%d, want eviction and %d replacements",
				stillSelectable, healthy, gatewayPoolWarmUpstreams)
		}
		time.Sleep(5 * time.Millisecond)
	}
	upstream, release, err := pool.acquire(t.Context())
	if err != nil {
		t.Fatalf("acquire after eviction error = %v", err)
	}
	release()
	if upstream == stalled {
		t.Fatal("acquire returned the evicted upstream")
	}
	output := logs.String()
	for _, want := range []string{
		`msg="edge: upstream stalled"`, "upstream_kind=gateway",
		`msg="edge: upstream evicted"`, "reason=liveness_probe_failed",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("gateway logs do not contain %q:\n%s", want, output)
		}
	}
}

type rxProgressConn struct {
	failingGiznetConn
	rx       atomic.Uint64
	progress bool
}

func (c *rxProgressConn) PeerInfo() *giznet.PeerInfo {
	if c.progress {
		c.rx.Add(512)
	}
	return &giznet.PeerInfo{State: giznet.PeerStateEstablished, RxBytes: c.rx.Load()}
}

func TestUpstreamLivenessSeparatesSlowFromStalled(t *testing.T) {
	cfg := upstreamLivenessConfig{
		timeout: 20 * time.Millisecond,
		probe: func(ctx context.Context, _ giznet.Conn) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}.withDefaults()

	// Other responses keep arriving: the probe merely queued behind a burst.
	busy := &rxProgressConn{progress: true}
	if err := cfg.check(t.Context(), busy); !errors.Is(err, errUpstreamSlow) || errors.Is(err, errUpstreamLivenessProbe) {
		t.Fatalf("busy upstream check error = %v, want slow without liveness failure", err)
	}
	// Nothing arrives at all: the association is silent.
	silent := &rxProgressConn{}
	if err := cfg.check(t.Context(), silent); !errors.Is(err, errUpstreamLivenessProbe) || errors.Is(err, errUpstreamSlow) {
		t.Fatalf("silent upstream check error = %v, want liveness failure", err)
	}
	// A probe that fails outright, such as a closed association, is a failure.
	cfg.probe = func(context.Context, giznet.Conn) error { return giznet.ErrConnClosed }
	if err := cfg.check(t.Context(), busy); !errors.Is(err, errUpstreamLivenessProbe) {
		t.Fatalf("closed upstream check error = %v, want liveness failure", err)
	}
}

func TestEdgeProxyLogsUpstreamErrorCause(t *testing.T) {
	logs := captureEdgeLogs(t)
	cause := errors.New("edge: dial upstream server: boom")
	handler := newPeerHTTPProxy("", roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, cause
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/login", nil))
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", recorder.Code)
	}
	output := logs.String()
	for _, want := range []string{`msg="gizedge: upstream proxy error"`, "level=WARN", "request_path=/login", "boom"} {
		if !strings.Contains(output, want) {
			t.Fatalf("proxy error log does not contain %q:\n%s", want, output)
		}
	}
	_, _ = io.Copy(io.Discard, recorder.Body)
}
