package gizedge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/giztunnel"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

type gatewayAllowAllPolicy struct{}

type gatewayHandshakeTimeoutError struct{}

func (gatewayHandshakeTimeoutError) Error() string   { return "handshake timeout" }
func (gatewayHandshakeTimeoutError) Timeout() bool   { return true }
func (gatewayHandshakeTimeoutError) Temporary() bool { return true }

func (gatewayAllowAllPolicy) AllowPeer(giznet.PublicKey) bool { return true }
func (gatewayAllowAllPolicy) AllowService(giznet.PublicKey, uint64) bool {
	return true
}

const gatewayBenchmarkBytesPerStream = 8 * 1024 * 1024

type contextDialGiznetConn struct {
	*failingGiznetConn
	dialContext func(context.Context, uint64) (net.Conn, error)
}

func TestLogUpstreamICEIsAddressFree(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	logUpstreamICE("gateway", "3", 3, &upstreamRelayAttempt{member: 1}, &gizwebrtc.ICECandidatePairObservation{
		Local: gizwebrtc.ICECandidateObservation{
			Type: "relay", Protocol: "udp", AddressFamily: "ipv4", Component: 1,
		},
		Remote: gizwebrtc.ICECandidateObservation{
			Type: "host", Protocol: "udp", AddressFamily: "ipv4", Component: 1,
		},
		State: "succeeded", Nominated: true,
	})

	got := output.String()
	for _, want := range []string{
		`msg="edge: upstream ICE selected"`, "upstream_kind=gateway", "upstream_id=3",
		"local_candidate_type=relay", "remote_candidate_type=host", "nominated=true", "relay_member=1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("log output %q does not contain %q", got, want)
		}
	}
	for _, forbidden := range []string{"192.0.2.10", "3478", "shared-secret", "turn:"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("log output contains sensitive network value %q: %q", forbidden, got)
		}
	}
}

func (c *contextDialGiznetConn) DialContext(ctx context.Context, service uint64) (net.Conn, error) {
	return c.dialContext(ctx, service)
}

func TestGatewayBridgesServiceAndPacketOverSharedUpstream(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	edgeKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	upstreamListener, err := (&gizwebrtc.ListenConfig{
		SecurityPolicy: gatewayAllowAllPolicy{},
	}).Listen(serverKey)
	if err != nil {
		t.Fatal(err)
	}
	defer upstreamListener.Close()
	upstreamHTTP := httptest.NewServer(upstreamListener.SignalingHandler())
	defer upstreamHTTP.Close()
	upstreamURL, err := url.Parse(upstreamHTTP.URL)
	if err != nil {
		t.Fatal(err)
	}
	upstreamAccepted := make(chan giznet.Conn, 1)
	go func() {
		conn, acceptErr := upstreamListener.Accept()
		if acceptErr == nil {
			upstreamAccepted <- conn
		}
	}()

	gatewayConfig := defaultGatewayConfig()
	gatewayConfig.Enabled = true
	gatewayConfig.ICEUDPListen = "127.0.0.1:0"
	gatewayConfig.MaxSessions = 4
	gatewayConfig.MaxUpstreams = 1
	gatewayConfig.SessionsPerUpstream = 4
	gatewayConfig.StreamsPerUpstream = 8
	gatewayConfig.MaxPendingHandshakes = 4
	cfg := Config{
		KeyPair: edgeKey,
		Upstream: UpstreamConfig{
			Endpoint:  upstreamHTTP.URL,
			PublicKey: serverKey.Public,
		},
		Gateway: gatewayConfig,
	}
	gateway, err := newGateway(t.Context(), cfg, upstreamURL, nil)
	if err != nil {
		t.Fatalf("newGateway error = %v", err)
	}
	defer gateway.Close()

	var upstreamConn giznet.Conn
	select {
	case upstreamConn = <-upstreamAccepted:
	case <-time.After(5 * time.Second):
		t.Fatal("gateway upstream did not connect")
	}
	defer upstreamConn.Close()
	packetMux := giztunnel.NewPacketMux(upstreamConn)
	defer packetMux.Close()
	go func() {
		buf := make([]byte, 64*1024)
		for {
			protocol, n, readErr := upstreamConn.Read(buf)
			if readErr != nil {
				return
			}
			if protocol == giznet.ProtocolTunnelPacket {
				_ = packetMux.HandlePacket(buf[:n])
			}
		}
	}()

	logicalAccepted := make(chan *giztunnel.Conn, 1)
	go func() {
		stream, acceptErr := upstreamConn.ListenService(gizclaw.ServiceEdgeTunnel).Accept()
		if acceptErr != nil {
			return
		}
		logical, _, acceptErr := giztunnel.Accept(
			context.Background(),
			stream,
			packetMux,
			func(open giztunnel.OpenRequest) error {
				if !open.ClientPublicKey.Equal(clientKey.Public) ||
					!open.EdgePublicKey.Equal(edgeKey.Public) ||
					!open.ServerPublicKey.Equal(serverKey.Public) {
					t.Errorf("unexpected delegated identities: %+v", open)
				}
				return nil
			},
			giztunnel.Config{
				PeerPublicKey: clientKey.Public,
				AllowRemoteService: func(uint64) bool {
					return true
				},
			},
		)
		if acceptErr == nil {
			logicalAccepted <- logical
		}
	}()

	edgeHTTP := httptest.NewServer(gateway.Handler(http.NotFoundHandler()))
	defer edgeHTTP.Close()
	clientListener, clientConn, err := gizwebrtc.Dial(
		context.Background(),
		clientKey,
		edgeKey.Public,
		gizwebrtc.DialConfig{
			SignalingURL:   edgeHTTP.URL + gizwebrtc.SignalingPath,
			SecurityPolicy: gatewayAllowAllPolicy{},
		},
	)
	if err != nil {
		t.Fatalf("client Dial error = %v", err)
	}
	defer clientListener.Close()
	defer clientConn.Close()

	var logical *giztunnel.Conn
	select {
	case logical = <-logicalAccepted:
	case <-time.After(5 * time.Second):
		t.Fatal("logical client was not accepted")
	}
	defer logical.Close()

	serviceListener := logical.ListenService(gizclaw.ServicePeerRPC)
	clientStream, err := clientConn.Dial(gizclaw.ServicePeerRPC)
	if err != nil {
		t.Fatalf("client service Dial error = %v", err)
	}
	serverStream, err := serviceListener.Accept()
	if err != nil {
		t.Fatalf("logical service Accept error = %v", err)
	}
	defer clientStream.Close()
	defer serverStream.Close()
	if err := rpcapi.WriteFrame(clientStream, rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: []byte("rpc")}); err != nil {
		t.Fatal(err)
	}
	if err := rpcapi.WriteFrame(clientStream, rpcapi.Frame{Type: rpcapi.FrameTypeEOS}); err != nil {
		t.Fatal(err)
	}
	requestFrame, err := rpcapi.ReadFrame(serverStream)
	if err != nil || requestFrame.Type != rpcapi.FrameTypeBinary || string(requestFrame.Payload) != "rpc" {
		t.Fatalf("logical request frame = %+v, %v", requestFrame, err)
	}
	requestEOS, err := rpcapi.ReadFrame(serverStream)
	if err != nil || requestEOS.Type != rpcapi.FrameTypeEOS {
		t.Fatalf("logical request EOS = %+v, %v", requestEOS, err)
	}
	if err := rpcapi.WriteFrame(serverStream, rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: []byte("ok")}); err != nil {
		t.Fatal(err)
	}
	if err := rpcapi.WriteFrame(serverStream, rpcapi.Frame{Type: rpcapi.FrameTypeEOS}); err != nil {
		t.Fatal(err)
	}
	responseFrame, err := rpcapi.ReadFrame(clientStream)
	if err != nil || responseFrame.Type != rpcapi.FrameTypeBinary || string(responseFrame.Payload) != "ok" {
		t.Fatalf("client response frame = %+v, %v", responseFrame, err)
	}
	responseEOS, err := rpcapi.ReadFrame(clientStream)
	if err != nil || responseEOS.Type != rpcapi.FrameTypeEOS {
		t.Fatalf("client response EOS = %+v, %v", responseEOS, err)
	}

	if _, err := clientConn.Write(0x42, []byte("packet")); err != nil {
		t.Fatal(err)
	}
	packet := make([]byte, 16)
	protocol, n, err := logical.Read(packet)
	if err != nil || protocol != 0x42 || string(packet[:n]) != "packet" {
		t.Fatalf("logical packet = %x %q %v", protocol, packet[:n], err)
	}
	if _, err := logical.Write(0x43, []byte("reply")); err != nil {
		t.Fatal(err)
	}
	protocol, n, err = clientConn.Read(packet)
	if err != nil || protocol != 0x43 || string(packet[:n]) != "reply" {
		t.Fatalf("client packet = %x %q %v", protocol, packet[:n], err)
	}

	opusFrame := []byte{0x00, 0xaa, 0xbb}
	if _, err := clientConn.Write(giznet.ProtocolOpusPacket, opusFrame); err != nil {
		t.Fatal(err)
	}
	protocol, n, err = logical.Read(packet)
	if err != nil || protocol != giznet.ProtocolOpusPacket ||
		string(packet[:n]) != string(opusFrame) {
		t.Fatalf("logical opus = %x %v %v", protocol, packet[:n], err)
	}
}

func TestGatewayPoolLeastActiveAndCumulativeRotation(t *testing.T) {
	cfg := Config{Gateway: GatewayConfig{
		MaxUpstreams:        2,
		SessionsPerUpstream: 2,
		StreamsPerUpstream:  3,
	}}
	first := &gatewayUpstream{}
	second := &gatewayUpstream{}
	pool := &gatewayPool{cfg: cfg, entries: []*gatewayUpstream{first, second}}
	first.pool = pool
	second.pool = pool
	first.opened = 2

	selected, release, err := pool.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if selected != first || first.state != gatewayUpstreamDraining {
		t.Fatalf("selected=%p state=%d, want first draining", selected, first.state)
	}
	release()
	if len(pool.entries) != 1 || pool.entries[0] != second {
		t.Fatalf("rotated entries = %+v", pool.entries)
	}
}

func TestGatewayPoolExpandsAtSessionCapacity(t *testing.T) {
	cfg := Config{Gateway: GatewayConfig{
		MaxUpstreams:        2,
		SessionsPerUpstream: 2,
		StreamsPerUpstream:  8,
	}}
	first := &gatewayUpstream{active: 2, opened: 2}
	var created atomic.Int32
	pool := &gatewayPool{
		cfg:     cfg,
		entries: []*gatewayUpstream{first},
		newUpstream: func(context.Context) (*gatewayUpstream, error) {
			created.Add(1)
			return &gatewayUpstream{}, nil
		},
	}
	first.pool = pool

	selected, release, err := pool.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if selected == first || created.Load() != 1 || len(pool.entries) != 2 || selected.active != 1 {
		t.Fatalf("capacity growth selected=%p first=%p created=%d entries=%d active=%d",
			selected, first, created.Load(), len(pool.entries), selected.active)
	}
}

func TestGatewayPoolReusesExistingSessionCapacity(t *testing.T) {
	cfg := Config{Gateway: GatewayConfig{
		MaxUpstreams:        2,
		SessionsPerUpstream: 2,
		StreamsPerUpstream:  4,
	}}
	first := &gatewayUpstream{active: 1, opened: 1}
	attempts := 0
	pool := &gatewayPool{
		cfg:     cfg,
		entries: []*gatewayUpstream{first},
		newUpstream: func(context.Context) (*gatewayUpstream, error) {
			attempts++
			return nil, errors.New("unexpected growth")
		},
	}
	first.pool = pool

	selected, release, err := pool.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if selected != first || first.active != 2 {
		t.Fatalf("selected=%p first=%p active=%d, want existing association at capacity",
			selected, first, first.active)
	}
	defer release()
	if selected != first || attempts != 0 {
		t.Fatalf("selected=%p first=%p attempts=%d, want existing association without growth",
			selected, first, attempts)
	}
}

func TestGatewayPoolWarmsBoundedAssociations(t *testing.T) {
	const maxUpstreams = 16
	first := &gatewayUpstream{}
	var created atomic.Int32
	pool := &gatewayPool{
		cfg:     Config{Gateway: GatewayConfig{MaxUpstreams: maxUpstreams}},
		entries: []*gatewayUpstream{first},
		newUpstream: func(context.Context) (*gatewayUpstream, error) {
			created.Add(1)
			return &gatewayUpstream{}, nil
		},
	}
	first.pool = pool

	if err := pool.warm(context.Background()); err != nil {
		t.Fatal(err)
	}
	if created.Load() != gatewayPoolWarmUpstreams-1 || len(pool.entries) != gatewayPoolWarmUpstreams {
		t.Fatalf("warmup created=%d entries=%d, want %d/%d",
			created.Load(), len(pool.entries), gatewayPoolWarmUpstreams-1, gatewayPoolWarmUpstreams)
	}
}

func TestGatewayPoolWarmRequiresTargetAssociations(t *testing.T) {
	first := &gatewayUpstream{}
	pool := &gatewayPool{
		cfg:     Config{Gateway: GatewayConfig{MaxUpstreams: gatewayPoolWarmUpstreams}},
		entries: []*gatewayUpstream{first},
		newUpstream: func(context.Context) (*gatewayUpstream, error) {
			return nil, errors.New("dial unavailable")
		},
	}
	first.pool = pool

	if err := pool.warm(context.Background()); err == nil || !strings.Contains(err.Error(), "warm gateway upstream") {
		t.Fatalf("warm error = %v, want contextual dial failure", err)
	}
	if len(pool.entries) != 1 {
		t.Fatalf("failed warmup entries = %d, want initial association only", len(pool.entries))
	}
}

func TestGatewayPoolReplenishesFailedWarmAssociation(t *testing.T) {
	entries := make([]*gatewayUpstream, gatewayPoolWarmUpstreams)
	for index := range entries {
		entries[index] = &gatewayUpstream{}
	}
	firstAttempt := make(chan struct{})
	replenished := make(chan struct{})
	var attempts atomic.Int32
	pool := &gatewayPool{
		ctx:     t.Context(),
		cfg:     Config{Gateway: GatewayConfig{MaxUpstreams: 16}},
		entries: entries,
		newUpstream: func(context.Context) (*gatewayUpstream, error) {
			if attempts.Add(1) == 1 {
				close(firstAttempt)
				return nil, errors.New("transient dial failure")
			}
			close(replenished)
			return &gatewayUpstream{}, nil
		},
	}
	for _, entry := range entries {
		entry.pool = pool
	}

	pool.markFailed(entries[0], "test_failure", false)
	select {
	case <-firstAttempt:
	case <-time.After(time.Second):
		t.Fatal("warm pool did not start replacement association")
	}
	select {
	case <-replenished:
	case <-time.After(2 * time.Second):
		t.Fatal("warm pool did not retry replacement association")
	}
	deadline := time.Now().Add(time.Second)
	for {
		pool.mu.Lock()
		ready := len(pool.entries) == gatewayPoolWarmUpstreams && pool.growthDone == nil
		pool.mu.Unlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("warm pool did not finish replacing failed association")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestGatewayPoolDrainingPreservesPinnedSessionsAndLiveCap(t *testing.T) {
	firstConn := &failingGiznetConn{state: giznet.PeerStateEstablished}
	first := &gatewayUpstream{conn: firstConn, active: 2}
	second := &gatewayUpstream{}
	created := make(chan struct{}, 1)
	pool := &gatewayPool{
		ctx: t.Context(),
		cfg: Config{Gateway: GatewayConfig{
			MaxUpstreams:        2,
			SessionsPerUpstream: 4,
			StreamsPerUpstream:  8,
		}},
		entries: []*gatewayUpstream{first, second},
		newUpstream: func(context.Context) (*gatewayUpstream, error) {
			created <- struct{}{}
			return &gatewayUpstream{}, nil
		},
	}
	first.pool = pool
	second.pool = pool

	if !pool.markDraining(first, "test_service_open") {
		t.Fatal("selectable entry did not transition to draining")
	}
	if firstConn.closed || len(pool.entries) != 2 {
		t.Fatalf("draining entry closed=%t entries=%d, want pinned entry preserved", firstConn.closed, len(pool.entries))
	}
	select {
	case <-created:
		t.Fatal("pool exceeded max-upstreams while draining entry was pinned")
	case <-time.After(20 * time.Millisecond):
	}
	selected, release, err := pool.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if selected != second {
		t.Fatalf("selected entry = %p, want healthy alternate %p", selected, second)
	}
	release()
	pool.release(first)
	if firstConn.closed || first.active != 1 {
		t.Fatalf("first release closed=%t active=%d, want one pinned session", firstConn.closed, first.active)
	}
	pool.release(first)
	if !firstConn.closed || first.active != 0 {
		t.Fatalf("final release closed=%t active=%d, want retired entry closed", firstConn.closed, first.active)
	}
	select {
	case <-created:
	case <-time.After(time.Second):
		t.Fatal("pool did not replenish after the draining entry released")
	}
}

func TestGatewayPoolTerminalFailureTransitionIsIdempotent(t *testing.T) {
	selector, err := newUpstreamRelaySelector(testUpstreamRelayConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	conn := &failingGiznetConn{state: giznet.PeerStateEstablished}
	pool := &gatewayPool{}
	entry := &gatewayUpstream{
		pool:         pool,
		conn:         conn,
		relayAttempt: selector.markSuccess(0),
	}
	pool.entries = []*gatewayUpstream{entry}
	var transitions atomic.Int32
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			if pool.markFailed(entry, "test_terminal", true) {
				transitions.Add(1)
			}
		})
	}
	workers.Wait()
	if transitions.Load() != 1 || entry.state != gatewayUpstreamFailed || !conn.closed {
		t.Fatalf("transitions=%d state=%d closed=%t, want one failed close", transitions.Load(), entry.state, conn.closed)
	}
	if selector.members[0].failures != 1 {
		t.Fatalf("relay failures = %d, want 1", selector.members[0].failures)
	}
}

func TestGatewayPoolDrainingTransitionIsIdempotent(t *testing.T) {
	conn := &failingGiznetConn{state: giznet.PeerStateEstablished}
	pool := &gatewayPool{}
	entry := &gatewayUpstream{
		pool:   pool,
		conn:   conn,
		active: 8,
	}
	pool.entries = []*gatewayUpstream{entry}
	var transitions atomic.Int32
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			if pool.markDraining(entry, "test_nonterminal") {
				transitions.Add(1)
			}
		})
	}
	workers.Wait()
	if transitions.Load() != 1 || entry.state != gatewayUpstreamDraining {
		t.Fatalf("transitions=%d state=%d, want one draining transition", transitions.Load(), entry.state)
	}
	if conn.closed || len(pool.entries) != 1 {
		t.Fatalf("closed=%t entries=%d, want pinned draining entry preserved", conn.closed, len(pool.entries))
	}
}

func TestGatewayClassifiesPreSessionFailuresWithoutPenalizingRelayForDraining(t *testing.T) {
	selector, err := newUpstreamRelaySelector(testUpstreamRelayConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	gateway := &Gateway{ctx: ctx}
	pool := &gatewayPool{}
	entry := &gatewayUpstream{
		pool:         pool,
		conn:         &failingGiznetConn{state: giznet.PeerStateEstablished},
		relayAttempt: selector.markSuccess(0),
	}
	pool.entries = []*gatewayUpstream{entry}
	if !gateway.classifyServiceOpenFailure(ctx, entry, gizwebrtc.ErrServiceOpen, false) {
		t.Fatal("pre-open DataChannel error was not retryable")
	}
	if entry.state != gatewayUpstreamDraining || selector.members[0].failures != 0 {
		t.Fatalf("state=%d relay failures=%d, want draining without relay penalty", entry.state, selector.members[0].failures)
	}

	serviceOpenTimeout := &gatewayUpstream{
		pool: pool,
		conn: &failingGiznetConn{state: giznet.PeerStateEstablished},
	}
	pool.entries = append(pool.entries, serviceOpenTimeout)
	if !gateway.classifyServiceOpenFailure(ctx, serviceOpenTimeout, context.DeadlineExceeded, true) {
		t.Fatal("complete service-open timeout was not retryable")
	}
	if serviceOpenTimeout.state != gatewayUpstreamDraining {
		t.Fatalf("service-open timeout state=%d, want draining", serviceOpenTimeout.state)
	}

	truncatedServiceOpen := &gatewayUpstream{
		pool: pool,
		conn: &failingGiznetConn{state: giznet.PeerStateEstablished},
	}
	pool.entries = append(pool.entries, truncatedServiceOpen)
	if gateway.classifyServiceOpenFailure(ctx, truncatedServiceOpen, context.DeadlineExceeded, false) ||
		truncatedServiceOpen.state != gatewayUpstreamSelectable {
		t.Fatal("overall-budget-truncated service-open timeout changed healthy upstream eligibility")
	}

	streamClosed := &gatewayUpstream{
		pool: pool,
		conn: &failingGiznetConn{state: giznet.PeerStateEstablished},
	}
	pool.entries = append(pool.entries, streamClosed)
	if !gateway.classifySessionHandshakeFailure(ctx, streamClosed, fmt.Errorf("read session response: %w", io.EOF), false) {
		t.Fatal("pre-accept stream close was not retryable")
	}
	if streamClosed.state != gatewayUpstreamDraining {
		t.Fatalf("pre-accept stream close state=%d, want draining", streamClosed.state)
	}

	handshakeTimeout := &gatewayUpstream{
		pool: pool,
		conn: &failingGiznetConn{state: giznet.PeerStateEstablished},
	}
	pool.entries = append(pool.entries, handshakeTimeout)
	if !gateway.classifySessionHandshakeFailure(ctx, handshakeTimeout, gatewayHandshakeTimeoutError{}, true) {
		t.Fatal("complete pre-accept handshake timeout was not retryable")
	}
	if handshakeTimeout.state != gatewayUpstreamDraining {
		t.Fatalf("pre-accept handshake timeout state=%d, want draining", handshakeTimeout.state)
	}

	truncatedHandshake := &gatewayUpstream{
		pool: pool,
		conn: &failingGiznetConn{state: giznet.PeerStateEstablished},
	}
	pool.entries = append(pool.entries, truncatedHandshake)
	if gateway.classifySessionHandshakeFailure(ctx, truncatedHandshake, gatewayHandshakeTimeoutError{}, false) ||
		truncatedHandshake.state != gatewayUpstreamSelectable {
		t.Fatal("overall-budget-truncated handshake timeout changed healthy upstream eligibility")
	}

	canceledCtx, cancelAttempt := context.WithCancel(context.Background())
	cancelAttempt()
	healthy := &gatewayUpstream{
		pool: pool,
		conn: &failingGiznetConn{state: giznet.PeerStateEstablished},
	}
	pool.entries = append(pool.entries, healthy)
	if gateway.classifySessionHandshakeFailure(canceledCtx, healthy, context.Canceled, false) ||
		healthy.state != gatewayUpstreamSelectable {
		t.Fatal("caller cancellation changed healthy upstream eligibility")
	}
	if gateway.classifySessionHandshakeFailure(ctx, healthy, giztunnel.ErrSessionRejected, false) ||
		healthy.state != gatewayUpstreamSelectable {
		t.Fatal("explicit session rejection changed healthy upstream eligibility")
	}
}

func TestGatewayRetriesSameClientThroughAlternateBeforeSessionAcceptance(t *testing.T) {
	edgeKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	firstConn := &contextDialGiznetConn{
		failingGiznetConn: &failingGiznetConn{state: giznet.PeerStateEstablished},
		dialContext: func(context.Context, uint64) (net.Conn, error) {
			return nil, gizwebrtc.ErrServiceOpen
		},
	}
	serverStream := make(chan net.Conn, 1)
	secondConn := &contextDialGiznetConn{
		failingGiznetConn: &failingGiznetConn{state: giznet.PeerStateEstablished},
		dialContext: func(context.Context, uint64) (net.Conn, error) {
			client, server := net.Pipe()
			serverStream <- server
			return client, nil
		},
	}
	first := &gatewayUpstream{id: 1, conn: firstConn, active: 1}
	second := &gatewayUpstream{
		id:      2,
		conn:    secondConn,
		packets: giztunnel.NewPacketMux(&failingGiznetConn{state: giznet.PeerStateEstablished}),
	}
	defer second.packets.Close()
	pool := &gatewayPool{
		cfg: Config{Gateway: GatewayConfig{
			MaxUpstreams:              2,
			SessionsPerUpstream:       2,
			StreamsPerUpstream:        8,
			SessionBufferBytes:        1 << 20,
			DelegatedEnvelopeValidity: 30 * time.Second,
		}},
		entries: []*gatewayUpstream{first, second},
	}
	first.pool = pool
	second.pool = pool
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gateway := &Gateway{
		ctx:    ctx,
		cancel: cancel,
		cfg: Config{
			KeyPair:  edgeKey,
			Upstream: UpstreamConfig{PublicKey: serverKey.Public},
			Gateway:  pool.cfg.Gateway,
		},
		pool:     pool,
		sessions: make(map[*gatewaySession]struct{}),
	}
	admission := &gatewayAdmission{
		gateway:         gateway,
		clientKey:       clientKey.Public,
		remoteAddr:      "test-client",
		upstream:        first,
		releaseUpstream: func() { pool.release(first) },
	}
	admission.state.Store(1)
	gateway.active = 1
	accepted := make(chan struct {
		open giztunnel.OpenRequest
		err  error
	}, 1)
	go func() {
		stream := <-serverStream
		packetMux := giztunnel.NewPacketMux(&failingGiznetConn{state: giznet.PeerStateEstablished})
		defer packetMux.Close()
		logical, open, acceptErr := giztunnel.Accept(ctx, stream, packetMux, nil, giztunnel.Config{})
		accepted <- struct {
			open giztunnel.OpenRequest
			err  error
		}{open: open, err: acceptErr}
		if acceptErr == nil {
			_ = logical.Close()
		}
	}()
	client := &failingGiznetConn{
		readErr:   giznet.ErrConnClosed,
		state:     giznet.PeerStateEstablished,
		publicKey: clientKey.Public,
	}
	gateway.handleClient(client, admission)
	select {
	case result := <-accepted:
		if result.err != nil {
			t.Fatalf("alternate Accept error = %v", result.err)
		}
		if !result.open.ClientPublicKey.Equal(clientKey.Public) {
			t.Fatalf("alternate client key = %s, want %s", result.open.ClientPublicKey, clientKey.Public)
		}
	case <-time.After(time.Second):
		t.Fatal("alternate did not accept the same client")
	}
	if first.state != gatewayUpstreamDraining || !firstConn.closed {
		t.Fatalf("first state=%d closed=%t, want retired draining entry", first.state, firstConn.closed)
	}
	if admission.upstream != second || second.active != 1 {
		t.Fatalf("admission upstream=%p second=%p active=%d", admission.upstream, second, second.active)
	}
	admission.releaseActive()
	if second.active != 0 || gateway.active != 0 {
		t.Fatalf("release left second active=%d gateway active=%d", second.active, gateway.active)
	}
}

func TestGatewayPoolCancelStopsGrowthAndWaiters(t *testing.T) {
	poolCtx, cancelPool := context.WithCancel(context.Background())
	cfg := Config{Gateway: GatewayConfig{
		MaxUpstreams:        2,
		SessionsPerUpstream: 2,
		StreamsPerUpstream:  4,
	}}
	first := &gatewayUpstream{active: 2, opened: 2}
	growthStarted := make(chan struct{})
	pool := &gatewayPool{
		ctx:     poolCtx,
		cfg:     cfg,
		entries: []*gatewayUpstream{first},
		newUpstream: func(ctx context.Context) (*gatewayUpstream, error) {
			close(growthStarted)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	first.pool = pool

	firstResult := make(chan error, 1)
	go func() {
		_, _, err := pool.acquire(context.Background())
		firstResult <- err
	}()
	select {
	case <-growthStarted:
	case <-time.After(time.Second):
		t.Fatal("pool growth did not start")
	}

	waiterResult := make(chan error, 1)
	go func() {
		_, _, err := pool.acquire(context.Background())
		waiterResult <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cancelPool()

	select {
	case err := <-firstResult:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("growth error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("growth did not stop after pool cancellation")
	}
	select {
	case err := <-waiterResult:
		if !errors.Is(err, giznet.ErrConnClosed) {
			t.Fatalf("waiter error = %v, want connection closed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("growth waiter did not stop after pool cancellation")
	}
}

func BenchmarkGatewayServiceThroughput(b *testing.B) {
	for _, tc := range []struct {
		name         string
		clients      int
		maxUpstreams int
	}{
		{name: "one_client/one_upstream", clients: 1, maxUpstreams: 1},
		{name: "three_clients/one_upstream", clients: 3, maxUpstreams: 1},
		{name: "three_clients/sharded_upstreams", clients: 3, maxUpstreams: 3},
		{name: "ten_clients/one_upstream", clients: 10, maxUpstreams: 1},
		{name: "ten_clients/sharded_upstreams", clients: 10, maxUpstreams: 10},
	} {
		b.Run(tc.name, func(b *testing.B) {
			streams := openGatewayThroughputStreams(b, tc.clients, tc.maxUpstreams)
			payload := bytes.Repeat([]byte("gateway-throughput"), gatewayBenchmarkBytesPerStream/len("gateway-throughput")+1)
			payload = payload[:gatewayBenchmarkBytesPerStream]

			b.SetBytes(int64(len(payload) * len(streams)))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				observation, err := transferGatewayStreams(streams, payload)
				if err != nil {
					b.Fatal(err)
				}
				b.ReportMetric(observation.minMbps, "min-client-Mbps")
				b.ReportMetric(observation.maxMbps, "max-client-Mbps")
			}
		})
	}
}

type gatewayThroughputStream struct {
	client net.Conn
	server net.Conn
}

type gatewayThroughputObservation struct {
	minMbps float64
	maxMbps float64
}

type acceptedGatewayLogical struct {
	key     giznet.PublicKey
	logical *giztunnel.Conn
	err     error
}

func openGatewayThroughputStreams(tb testing.TB, clients, maxUpstreams int) []gatewayThroughputStream {
	tb.Helper()
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		tb.Fatal(err)
	}
	edgeKey, err := giznet.GenerateKeyPair()
	if err != nil {
		tb.Fatal(err)
	}
	upstreamListener, err := (&gizwebrtc.ListenConfig{
		SecurityPolicy: gatewayAllowAllPolicy{},
	}).Listen(serverKey)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = upstreamListener.Close() })
	upstreamHTTP := httptest.NewServer(upstreamListener.SignalingHandler())
	tb.Cleanup(upstreamHTTP.Close)
	upstreamURL, err := url.Parse(upstreamHTTP.URL)
	if err != nil {
		tb.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	tb.Cleanup(cancel)
	logicalCh := make(chan acceptedGatewayLogical, clients)
	go acceptGatewayBenchmarkUpstreams(ctx, upstreamListener, logicalCh)

	gatewayConfig := defaultGatewayConfig()
	gatewayConfig.Enabled = true
	gatewayConfig.ICEUDPListen = "127.0.0.1:0"
	gatewayConfig.MaxSessions = clients
	gatewayConfig.MaxUpstreams = maxUpstreams
	gatewayConfig.SessionsPerUpstream = clients
	gatewayConfig.StreamsPerUpstream = clients * 2
	gatewayConfig.MaxPendingHandshakes = clients
	gatewayConfig.DrainTimeout = time.Second
	cfg := Config{
		KeyPair: edgeKey,
		Upstream: UpstreamConfig{
			Endpoint:  upstreamHTTP.URL,
			PublicKey: serverKey.Public,
		},
		Gateway: gatewayConfig,
	}
	gateway, err := newGateway(ctx, cfg, upstreamURL, nil)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = gateway.Close() })
	edgeHTTP := httptest.NewServer(gateway.Handler(http.NotFoundHandler()))
	tb.Cleanup(edgeHTTP.Close)

	clientConns := make(map[giznet.PublicKey]giznet.Conn, clients)
	for range clients {
		clientKey, err := giznet.GenerateKeyPair()
		if err != nil {
			tb.Fatal(err)
		}
		dialCtx, cancelDial := context.WithTimeout(ctx, 15*time.Second)
		clientListener, clientConn, err := gizwebrtc.Dial(
			dialCtx,
			clientKey,
			edgeKey.Public,
			gizwebrtc.DialConfig{
				SignalingURL:   edgeHTTP.URL + gizwebrtc.SignalingPath,
				SecurityPolicy: gatewayAllowAllPolicy{},
			},
		)
		cancelDial()
		if err != nil {
			tb.Fatal(err)
		}
		tb.Cleanup(func() {
			_ = clientConn.Close()
			_ = clientListener.Close()
		})
		clientConns[clientKey.Public] = clientConn
	}

	logicals := make(map[giznet.PublicKey]*giztunnel.Conn, clients)
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	for len(logicals) < clients {
		select {
		case accepted := <-logicalCh:
			if accepted.err != nil {
				tb.Fatal(accepted.err)
			}
			logicals[accepted.key] = accepted.logical
			tb.Cleanup(func() { _ = accepted.logical.Close() })
		case <-timer.C:
			tb.Fatalf("accepted %d of %d logical gateway sessions", len(logicals), clients)
		}
	}

	streams := make([]gatewayThroughputStream, 0, clients)
	for key, clientConn := range clientConns {
		logical := logicals[key]
		service := logical.ListenService(gizclaw.ServicePeerRPC)
		serverStreamCh := make(chan struct {
			stream net.Conn
			err    error
		}, 1)
		go func() {
			stream, err := service.Accept()
			serverStreamCh <- struct {
				stream net.Conn
				err    error
			}{stream: stream, err: err}
		}()
		clientStream, err := clientConn.Dial(gizclaw.ServicePeerRPC)
		if err != nil {
			tb.Fatal(err)
		}
		var serverStream net.Conn
		select {
		case accepted := <-serverStreamCh:
			if accepted.err != nil {
				tb.Fatal(accepted.err)
			}
			serverStream = accepted.stream
		case <-time.After(5 * time.Second):
			tb.Fatal("accept benchmark service stream timed out")
		}
		tb.Cleanup(func() {
			_ = clientStream.Close()
			_ = serverStream.Close()
		})
		streams = append(streams, gatewayThroughputStream{
			client: clientStream,
			server: serverStream,
		})
	}
	return streams
}

func acceptGatewayBenchmarkUpstreams(
	ctx context.Context,
	listener *gizwebrtc.Listener,
	logicalCh chan<- acceptedGatewayLogical,
) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go acceptGatewayBenchmarkSessions(ctx, conn, logicalCh)
	}
}

func acceptGatewayBenchmarkSessions(
	ctx context.Context,
	conn giznet.Conn,
	logicalCh chan<- acceptedGatewayLogical,
) {
	packetMux := giztunnel.NewPacketMux(conn)
	defer packetMux.Close()
	go func() {
		buf := make([]byte, 64*1024)
		for {
			protocol, n, err := conn.Read(buf)
			if err != nil {
				return
			}
			if protocol == giznet.ProtocolTunnelPacket {
				_ = packetMux.HandlePacket(buf[:n])
			}
		}
	}()
	service := conn.ListenService(gizclaw.ServiceEdgeTunnel)
	for {
		stream, err := service.Accept()
		if err != nil {
			return
		}
		go func() {
			logical, open, err := giztunnel.Accept(
				ctx,
				stream,
				packetMux,
				func(giztunnel.OpenRequest) error { return nil },
				giztunnel.Config{
					AllowRemoteService: func(uint64) bool { return true },
				},
			)
			accepted := acceptedGatewayLogical{logical: logical, err: err}
			if err == nil {
				accepted.key = open.ClientPublicKey
			}
			select {
			case logicalCh <- accepted:
			case <-ctx.Done():
				if logical != nil {
					_ = logical.Close()
				}
			}
		}()
	}
}

func transferGatewayStreams(
	streams []gatewayThroughputStream,
	payload []byte,
) (gatewayThroughputObservation, error) {
	var wg sync.WaitGroup
	errCh := make(chan error, len(streams)*2)
	start := make(chan struct{})
	durations := make([]time.Duration, len(streams))
	for i, stream := range streams {
		wg.Go(func() {
			<-start
			started := time.Now()
			if _, err := io.CopyN(io.Discard, stream.server, int64(len(payload))); err != nil {
				errCh <- fmt.Errorf("stream %d read: %w", i, err)
			}
			durations[i] = time.Since(started)
		})
		wg.Go(func() {
			<-start
			if _, err := io.Copy(stream.client, bytes.NewReader(payload)); err != nil {
				errCh <- fmt.Errorf("stream %d write: %w", i, err)
			}
		})
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		return gatewayThroughputObservation{}, err
	}
	observation := gatewayThroughputObservation{}
	for i, duration := range durations {
		mbps := float64(len(payload)*8) / duration.Seconds() / 1_000_000
		if i == 0 || mbps < observation.minMbps {
			observation.minMbps = mbps
		}
		if mbps > observation.maxMbps {
			observation.maxMbps = mbps
		}
	}
	return observation, nil
}

func TestGatewayAdmissionMatchesAcceptedClientIdentity(t *testing.T) {
	firstKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	secondKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gateway := &Gateway{
		ctx:             ctx,
		cancel:          cancel,
		pending:         2,
		admissions:      make(map[giznet.PublicKey][]*gatewayAdmission),
		admissionNotify: make(chan struct{}),
	}
	first := &gatewayAdmission{gateway: gateway, clientKey: firstKey.Public}
	second := &gatewayAdmission{gateway: gateway, clientKey: secondKey.Public}
	if !gateway.enqueueAdmission(first) || !gateway.enqueueAdmission(second) {
		t.Fatal("enqueueAdmission failed")
	}
	got, err := gateway.claimAdmission(secondKey.Public)
	if err != nil {
		t.Fatal(err)
	}
	if got != second {
		t.Fatalf("claimed admission = %p, want %p", got, second)
	}
	got.releaseActive()
	got, err = gateway.claimAdmission(firstKey.Public)
	if err != nil {
		t.Fatal(err)
	}
	if got != first {
		t.Fatalf("claimed admission = %p, want %p", got, first)
	}
	got.releaseActive()
}

func TestGatewayAdmissionRejectsCapacityBeforeHandshake(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := Config{Gateway: GatewayConfig{
		MaxSessions:          1,
		MaxUpstreams:         1,
		SessionsPerUpstream:  1,
		MaxPendingHandshakes: 1,
	}}
	pool := &gatewayPool{ctx: ctx, cfg: cfg}
	entry := &gatewayUpstream{pool: pool}
	pool.entries = []*gatewayUpstream{entry}
	gateway := &Gateway{
		ctx:             ctx,
		cancel:          cancel,
		cfg:             cfg,
		pool:            pool,
		admissions:      make(map[giznet.PublicKey][]*gatewayAdmission),
		admissionNotify: make(chan struct{}),
	}
	admission, err := gateway.reserveAdmission()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.reserveAdmission(); !errors.Is(err, ErrGatewayOverCapacity) {
		t.Fatalf("second reserve error = %v, want over capacity", err)
	}
	admission.releasePending()
	if _, err := gateway.reserveAdmission(); err != nil {
		t.Fatalf("reserve after release error = %v", err)
	}
}

func TestGatewayBurstSCTPProfileIsAdmissionBounded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := Config{Gateway: GatewayConfig{
		MaxSessions:          gatewayClientBurstSCTPLimit + 2,
		MaxUpstreams:         1,
		SessionsPerUpstream:  gatewayClientBurstSCTPLimit + 2,
		MaxPendingHandshakes: gatewayClientBurstSCTPLimit + 2,
	}}
	pool := &gatewayPool{ctx: ctx, cfg: cfg}
	pool.entries = []*gatewayUpstream{{pool: pool}}
	gateway := &Gateway{
		ctx:             ctx,
		cancel:          cancel,
		cfg:             cfg,
		pool:            pool,
		admissions:      make(map[giznet.PublicKey][]*gatewayAdmission),
		admissionNotify: make(chan struct{}),
	}

	reservations := make([]*gatewayAdmission, 0, gatewayClientBurstSCTPLimit+1)
	for range gatewayClientBurstSCTPLimit + 1 {
		admission, err := gateway.reserveAdmission()
		if err != nil {
			t.Fatal(err)
		}
		reservations = append(reservations, admission)
	}
	if !reservations[gatewayClientBurstSCTPLimit-1].burstSCTP {
		t.Fatal("last in-budget admission did not receive the burst profile")
	}
	if reservations[gatewayClientBurstSCTPLimit].burstSCTP {
		t.Fatal("out-of-budget admission received the burst profile")
	}

	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	reservations[0].clientKey = clientKey.Public
	requestCtx := context.WithValue(ctx, gatewayAdmissionContextKey{}, reservations[0])
	if !gateway.allowBurstSCTP(requestCtx, clientKey.Public) {
		t.Fatal("matching in-budget signaling request did not select the burst profile")
	}
	if gateway.allowBurstSCTP(requestCtx, giznet.PublicKey{}) {
		t.Fatal("mismatched signaling identity selected the burst profile")
	}

	reservations[0].releasePending()
	replacement, err := gateway.reserveAdmission()
	if err != nil {
		t.Fatal(err)
	}
	if !replacement.burstSCTP {
		t.Fatal("released burst-profile budget was not reusable")
	}
	for _, admission := range reservations[1:] {
		admission.releasePending()
	}
	replacement.releasePending()
	if gateway.burstSCTP != 0 {
		t.Fatalf("burst SCTP reservations after release = %d, want 0", gateway.burstSCTP)
	}
}

func TestGatewayUpstreamReportsOnlyUnplannedPhysicalRelayFailure(t *testing.T) {
	for _, test := range []struct {
		name          string
		cancelContext bool
		wantFailures  uint32
	}{
		{name: "physical failure", wantFailures: 1},
		{name: "shutdown", cancelContext: true, wantFailures: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			selector, err := newUpstreamRelaySelector(testUpstreamRelayConfig(t))
			if err != nil {
				t.Fatalf("newUpstreamRelaySelector error = %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			if test.cancelContext {
				cancel()
			} else {
				defer cancel()
			}
			pool := &gatewayPool{ctx: ctx}
			entry := &gatewayUpstream{
				pool:         pool,
				conn:         &failingGiznetConn{readErr: giznet.ErrConnClosed},
				relayAttempt: selector.markSuccess(0),
			}
			pool.entries = []*gatewayUpstream{entry}

			entry.readPackets()

			if got := selector.members[0].failures; got != test.wantFailures {
				t.Fatalf("relay failure count = %d, want %d", got, test.wantFailures)
			}
		})
	}
}

func TestGatewayRejectsUnidentifiedSignalingBeforeListener(t *testing.T) {
	for _, test := range []struct {
		name      string
		publicKey string
	}{
		{name: "missing"},
		{name: "malformed", publicKey: "not-a-public-key"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fallbackCalled := false
			gateway := &Gateway{}
			handler := gateway.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				fallbackCalled = true
			}))
			req := httptest.NewRequest(http.MethodPost, gizwebrtc.SignalingPath, nil)
			if test.publicKey != "" {
				req.Header.Set("X-Giznet-Public-Key", test.publicKey)
			}
			resp := httptest.NewRecorder()

			handler.ServeHTTP(resp, req)

			if fallbackCalled {
				t.Fatal("gateway forwarded an unidentified signaling offer")
			}
			if resp.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", resp.Code, http.StatusBadRequest)
			}
			var body map[string]string
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["error"] != "invalid_public_key" {
				t.Fatalf("error = %q, want invalid_public_key", body["error"])
			}
			if gateway.pending != 0 || gateway.active != 0 {
				t.Fatalf("unidentified request changed capacity: pending=%d active=%d",
					gateway.pending, gateway.active)
			}
		})
	}
}

func TestGatewayServerInfoTransportRemovesAuthoritativeICE(t *testing.T) {
	body := `{"public_key":"server","endpoint":"server:9820","signaling_path":"/offer","ice":{"udp":true,"tcp":true},"ice_servers":[{"urls":["turn:server"]}]}`
	resp := &http.Response{
		Body:   io.NopCloser(strings.NewReader(body)),
		Header: make(http.Header),
	}
	err := rewriteServerInfo(resp, "edge:9821", &serverInfoTransport{
		Mode:          "edge-gateway",
		Endpoint:      "edge:9821",
		PublicKey:     "edge-key",
		SignalingPath: "/webrtc/v1/offer",
	})
	if err != nil {
		t.Fatal(err)
	}
	var info map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	if _, ok := info["ice_servers"]; ok {
		t.Fatal("gateway server-info retained authoritative ICE servers")
	}
	transport, ok := info["transport"].(map[string]any)
	if !ok || transport["public_key"] != "edge-key" {
		t.Fatalf("transport = %#v", info["transport"])
	}
}
