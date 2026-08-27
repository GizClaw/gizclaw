package gizedge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
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

type gatewayLogCapture struct {
	mu      sync.Mutex
	records []slog.Record
}

func (*gatewayLogCapture) Enabled(context.Context, slog.Level) bool { return true }
func (h *gatewayLogCapture) WithAttrs([]slog.Attr) slog.Handler     { return h }
func (h *gatewayLogCapture) WithGroup(string) slog.Handler          { return h }
func (h *gatewayLogCapture) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	h.records = append(h.records, record.Clone())
	h.mu.Unlock()
	return nil
}

type gatewayHandshakeTimeoutError struct{}

type gatewayWaitSignalContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *gatewayWaitSignalContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func (gatewayHandshakeTimeoutError) Error() string   { return "handshake timeout" }
func (gatewayHandshakeTimeoutError) Timeout() bool   { return true }
func (gatewayHandshakeTimeoutError) Temporary() bool { return true }

func (gatewayAllowAllPolicy) AllowPeer(giznet.PublicKey) bool { return true }
func (gatewayAllowAllPolicy) AllowService(giznet.PublicKey, uint64) bool {
	return true
}

const gatewayBenchmarkBytesPerStream = 8 * 1024 * 1024

type blockingCloseGiznetConn struct {
	failingGiznetConn
	entered chan<- struct{}
	release <-chan struct{}
}

func (c *blockingCloseGiznetConn) Close() error {
	c.entered <- struct{}{}
	<-c.release
	return nil
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

func TestGatewayBridgeLifecycleResultIsBounded(t *testing.T) {
	tests := []struct {
		name        string
		observation giztunnel.BridgeObservation
		err         error
		wantResult  string
		wantReason  string
	}{
		{name: "success", observation: giztunnel.BridgeObservation{ErrorClass: "clean"}, wantResult: "success", wantReason: "completed"},
		{name: "canceled", observation: giztunnel.BridgeObservation{ErrorClass: "context_canceled"}, wantResult: "canceled", wantReason: "context_canceled"},
		{name: "timeout", observation: giztunnel.BridgeObservation{ErrorClass: "deadline_exceeded"}, wantResult: "timeout", wantReason: "deadline_exceeded"},
		{name: "observed closed compatibility nil", observation: giztunnel.BridgeObservation{ErrorClass: "eof"}, wantResult: "closed", wantReason: "transport_closed"},
		{name: "generic rejection", observation: giztunnel.BridgeObservation{ErrorClass: "eof", OpenRejectionCount: 1}, wantResult: "transport_error", wantReason: "bridge_error"},
		{name: "exact session capacity", observation: giztunnel.BridgeObservation{ErrorClass: "eof", OpenRejectionCount: 1, CapacityScope: "session"}, wantResult: "transport_error", wantReason: "channel_capacity_rejected"},
		{name: "exact association capacity", observation: giztunnel.BridgeObservation{ErrorClass: "closed", OpenRejectionCount: 1, CapacityScope: "association"}, wantResult: "transport_error", wantReason: "channel_capacity_rejected"},
		{name: "transport error", observation: giztunnel.BridgeObservation{ErrorClass: "other"}, err: errors.New("authorization: Bearer secret"), wantResult: "transport_error", wantReason: "bridge_error"},
		{name: "legacy zero observation", err: io.ErrUnexpectedEOF, wantResult: "closed", wantReason: "transport_closed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, reason := gatewayBridgeLifecycleResult(test.observation, test.err)
			if result != test.wantResult || reason != test.wantReason {
				t.Fatalf("gatewayBridgeLifecycleResult() = (%q, %q), want (%q, %q)", result, reason, test.wantResult, test.wantReason)
			}
			if strings.Contains(result+reason, "secret") {
				t.Fatal("gateway lifecycle result exposed a raw error")
			}
		})
	}
}

func TestGatewayBridgeObservationAttrsAreBoundedScalars(t *testing.T) {
	observation := giztunnel.BridgeObservation{
		Path:                        "service",
		Direction:                   "left_to_right",
		Phase:                       "accept_source",
		ErrorClass:                  "eof",
		OpenRejectionCount:          2,
		FirstOpenRejectionDirection: "left_to_right",
		FirstOpenRejectionClass:     "other",
		LastOpenRejectionDirection:  "right_to_left",
		LastOpenRejectionClass:      "buffer_limit",
		CapacityScope:               "session",
		ActiveChannels:              32,
		ChannelLimit:                32,
	}
	attrs := gatewayBridgeObservationAttrs(observation)
	got := make(map[string]any, len(attrs)/2)
	for index := 0; index < len(attrs); index += 2 {
		key, ok := attrs[index].(string)
		if !ok {
			t.Fatalf("attribute key %d has type %T", index, attrs[index])
		}
		got[key] = attrs[index+1]
	}
	want := map[string]any{
		"bridge_path":                           "service",
		"bridge_direction":                      "left_to_right",
		"bridge_phase":                          "accept_source",
		"bridge_error_class":                    "eof",
		"bridge_open_rejection_count":           uint64(2),
		"bridge_first_open_rejection_direction": "left_to_right",
		"bridge_first_open_rejection_class":     "other",
		"bridge_last_open_rejection_direction":  "right_to_left",
		"bridge_last_open_rejection_class":      "buffer_limit",
		"bridge_capacity_scope":                 "session",
		"bridge_active_channels":                32,
		"bridge_channel_limit":                  32,
	}
	if !maps.Equal(got, want) {
		t.Fatalf("gatewayBridgeObservationAttrs() = %#v, want %#v", got, want)
	}
	for _, forbidden := range []string{"authorization", "Bearer", "secret", "service_number", "rpc"} {
		if strings.Contains(fmt.Sprint(attrs), forbidden) {
			t.Fatalf("bridge attributes exposed forbidden value %q: %#v", forbidden, attrs)
		}
	}

	unknownCapacity := gatewayBridgeObservationAttrs(giztunnel.BridgeObservation{
		CapacityScope:  "unknown",
		ActiveChannels: 99,
		ChannelLimit:   100,
	})
	if strings.Contains(fmt.Sprint(unknownCapacity), "bridge_capacity") {
		t.Fatalf("unknown capacity produced exact attributes: %#v", unknownCapacity)
	}
}

func TestGatewaySessionEstablishmentResultIsBounded(t *testing.T) {
	for _, test := range []struct {
		name           string
		err            error
		completeBudget bool
		want           string
	}{
		{name: "canceled", err: context.Canceled, want: "canceled"},
		{name: "deadline", err: context.DeadlineExceeded, want: "timeout"},
		{name: "complete handshake timeout", err: gatewayHandshakeTimeoutError{}, completeBudget: true, want: "timeout"},
		{name: "partial handshake timeout", err: gatewayHandshakeTimeoutError{}, want: "transport_error"},
		{name: "closed", err: io.EOF, want: "closed"},
		{name: "rejected", err: giztunnel.ErrSessionRejected, want: "transport_error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := gatewaySessionEstablishmentResult(test.err, test.completeBudget); got != test.want {
				t.Fatalf("gatewaySessionEstablishmentResult() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestGatewayBridgesServiceAndPacketOverSharedUpstream(t *testing.T) {
	capture := &gatewayLogCapture{}
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(capture))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

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
	gatewayConfig.MaxSessions = 4
	gatewayConfig.MaxUpstreams = 1
	gatewayConfig.SessionsPerUpstream = 4
	gatewayConfig.ChannelsPerSession = 8
	gatewayConfig.ChannelsPerUpstream = 12
	gatewayConfig.MaxPendingHandshakes = 4
	cfg := Config{
		KeyPair:  edgeKey,
		Listen:   "127.0.0.1:0",
		Endpoint: "localhost:0",
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
	serverTransport, ok := upstreamConn.(*gizwebrtc.Conn)
	if !ok {
		t.Fatalf("upstream type = %T", upstreamConn)
	}
	serverRouter, err := giztunnel.NewRouter(serverTransport, giztunnel.Config{
		AcceptSessions:        true,
		MaxChannelsPerSession: 8,
		MaxChannels:           12,
		MaxPendingSessions:    4,
		AllowRemoteService: func(giznet.PublicKey, uint64) bool {
			return true
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer serverRouter.Close()
	go func() {
		buf := make([]byte, 64*1024)
		for {
			protocol, n, readErr := upstreamConn.Read(buf)
			if readErr != nil {
				return
			}
			if protocol == giznet.ProtocolTunnelPacket {
				_ = serverRouter.HandlePacket(buf[:n])
			}
		}
	}()

	logicalAccepted := make(chan *giztunnel.Conn, 1)
	go func() {
		logical, declaration, acceptErr := serverRouter.Accept(context.Background())
		if acceptErr == nil && !declaration.ClientPublicKey.Equal(clientKey.Public) {
			t.Errorf("unexpected declared client identity: %s", declaration.ClientPublicKey)
			_ = logical.Close()
			return
		}
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

	capture.mu.Lock()
	records := append([]slog.Record(nil), capture.records...)
	capture.mu.Unlock()
	wantStages := map[string]bool{
		"session_establishing": false,
		"session_accepted":     false,
		"bridge_started":       false,
	}
	var sessionID string
	for _, record := range records {
		if record.Message != peerStreamLifecycleMessage {
			continue
		}
		attrs := make(map[string]any)
		record.Attrs(func(attr slog.Attr) bool {
			attrs[attr.Key] = attr.Value.Any()
			return true
		})
		stage, ok := attrs["stage"].(string)
		if !ok || !strings.HasPrefix(stage, "session_") && stage != "bridge_started" {
			continue
		}
		if _, ok := wantStages[stage]; !ok {
			continue
		}
		wantStages[stage] = true
		gotSessionID, _ := attrs["tunnel_session_id"].(string)
		if sessionID == "" {
			sessionID = gotSessionID
		}
		if gotSessionID == "" || gotSessionID != sessionID {
			t.Errorf("stage %q tunnel_session_id = %q, want shared %q", stage, gotSessionID, sessionID)
		}
		if attrs["peer_public_key"] != clientKey.Public.String() {
			t.Errorf("stage %q peer_public_key = %#v", stage, attrs["peer_public_key"])
		}
		if _, exists := attrs["remote_addr"]; exists {
			t.Errorf("stage %q exposed remote_addr", stage)
		}
		for key, value := range attrs {
			switch value.(type) {
			case string, int64, uint64, bool:
			default:
				t.Errorf("stage %q attribute %q is non-scalar %T", stage, key, value)
			}
		}
	}
	for stage, found := range wantStages {
		if !found {
			t.Errorf("missing gateway lifecycle stage %q", stage)
		}
	}
}

func TestGatewayPoolSelectsLeastActiveAssociation(t *testing.T) {
	cfg := Config{Gateway: GatewayConfig{
		MaxUpstreams:        2,
		SessionsPerUpstream: 2,
		ChannelsPerUpstream: 6,
	}}
	first := &gatewayUpstream{active: 1}
	second := &gatewayUpstream{}
	pool := &gatewayPool{cfg: cfg, entries: []*gatewayUpstream{first, second}}
	first.pool = pool
	second.pool = pool

	selected, release, err := pool.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if selected != second || second.active != 1 {
		t.Fatalf("selected=%p active=%d, want least-active second", selected, second.active)
	}
	release()
	if len(pool.entries) != 2 || second.active != 0 {
		t.Fatalf("entries=%d second.active=%d", len(pool.entries), second.active)
	}
}

func TestGatewayPoolExpandsAtSessionCapacity(t *testing.T) {
	cfg := Config{Gateway: GatewayConfig{
		MaxUpstreams:        2,
		SessionsPerUpstream: 2,
		ChannelsPerUpstream: 8,
	}}
	first := &gatewayUpstream{active: 2}
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
		ChannelsPerUpstream: 6,
	}}
	first := &gatewayUpstream{active: 1}
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

func TestRetryGatewayStartupRelayWaitsForEligibleMember(t *testing.T) {
	var attempts int
	err := retryGatewayStartupRelay(t.Context(), func(context.Context) error {
		attempts++
		if attempts == 1 {
			return fmt.Errorf("warm gateway upstream: %w", &upstreamRelaysUnavailableError{
				attempts:   2,
				retryAfter: time.Millisecond,
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retryGatewayStartupRelay error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("startup attempts = %d, want 2", attempts)
	}
}

func TestRetryGatewayStartupRelayRetriesInnerDeadline(t *testing.T) {
	var attempts int
	err := retryGatewayStartupRelay(t.Context(), func(context.Context) error {
		attempts++
		if attempts == 1 {
			return fmt.Errorf("warm gateway upstream: %w", context.DeadlineExceeded)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retryGatewayStartupRelay error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("startup attempts = %d, want 2", attempts)
	}
}

func TestRetryGatewayStartupRelayStopsAtOuterDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	var attempts int
	err := retryGatewayStartupRelay(ctx, func(context.Context) error {
		attempts++
		cancel()
		return fmt.Errorf("warm gateway upstream: %w", context.DeadlineExceeded)
	})
	if !errors.Is(err, context.Canceled) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("retryGatewayStartupRelay error = %v, want outer cancellation and inner deadline", err)
	}
	if attempts != 1 {
		t.Fatalf("startup attempts = %d, want 1", attempts)
	}
}

func TestRetryGatewayStartupRelayDoesNotRetryOtherErrors(t *testing.T) {
	want := errors.New("invalid gateway configuration")
	var attempts int
	err := retryGatewayStartupRelay(t.Context(), func(context.Context) error {
		attempts++
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("retryGatewayStartupRelay error = %v, want %v", err, want)
	}
	if attempts != 1 {
		t.Fatalf("startup attempts = %d, want 1", attempts)
	}
}

func TestRetryGatewayStartupRelayStopsWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	want := &upstreamRelaysUnavailableError{attempts: 2, retryAfter: time.Hour}
	var attempts int
	err := retryGatewayStartupRelay(ctx, func(context.Context) error {
		attempts++
		cancel()
		return want
	})
	if !errors.Is(err, want) || !errors.Is(err, context.Canceled) {
		t.Fatalf("retryGatewayStartupRelay error = %v, want relay unavailable and canceled", err)
	}
	if attempts != 1 {
		t.Fatalf("startup attempts = %d, want 1", attempts)
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

func TestGatewayPoolReplenishWarmSignalsEachNewAssociation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	secondDial := make(chan struct{})
	replenished := make(chan struct{})
	var attempts atomic.Int32
	pool := &gatewayPool{
		ctx: ctx,
		cfg: Config{Gateway: GatewayConfig{
			MaxUpstreams:        gatewayPoolWarmUpstreams,
			SessionsPerUpstream: 1,
			ChannelsPerUpstream: 3,
		}},
		newUpstream: func(ctx context.Context) (*gatewayUpstream, error) {
			if attempts.Add(1) == 1 {
				return &gatewayUpstream{}, nil
			}
			close(secondDial)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	done := make(chan struct{})
	pool.growthDone = done
	waiting := make(chan struct{})
	acquireCtx := &gatewayWaitSignalContext{Context: t.Context(), waiting: waiting}
	acquired := make(chan *gatewayUpstream, 1)
	go func() {
		entry, release, err := pool.acquire(acquireCtx)
		if err != nil {
			acquired <- nil
			return
		}
		release()
		acquired <- entry
	}()
	select {
	case <-waiting:
	case <-time.After(time.Second):
		t.Fatal("acquisition did not wait for the active warm replenishment")
	}
	go func() {
		pool.replenishWarm(done)
		close(replenished)
	}()
	select {
	case <-secondDial:
	case <-time.After(time.Second):
		t.Fatal("warm replenishment did not continue toward the target")
	}
	select {
	case entry := <-acquired:
		if entry == nil {
			t.Fatal("waiting acquisition failed after the first replenished association")
		}
	case <-time.After(time.Second):
		t.Fatal("waiting acquisition was not signaled after the first replenished association")
	}
	cancel()
	select {
	case <-replenished:
	case <-time.After(time.Second):
		t.Fatal("warm replenishment did not stop after cancellation")
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
			ChannelsPerUpstream: 12,
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

func TestGatewayClassifiesNativeSessionFailuresWithoutPenalizingRelayForDraining(t *testing.T) {
	selector, err := newUpstreamRelaySelector(testUpstreamRelayConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	gateway := &Gateway{ctx: ctx}
	pool := &gatewayPool{}
	entry := &gatewayUpstream{pool: pool, conn: &failingGiznetConn{state: giznet.PeerStateEstablished}, relayAttempt: selector.markSuccess(0)}
	pool.entries = []*gatewayUpstream{entry}
	if !gateway.classifySessionHandshakeFailure(ctx, entry, gizwebrtc.ErrServiceOpen, false) {
		t.Fatal("native channel open error was not retryable")
	}
	if entry.state != gatewayUpstreamDraining || selector.members[0].failures != 0 {
		t.Fatalf("state=%d relay failures=%d, want draining without relay penalty", entry.state, selector.members[0].failures)
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

func TestGatewayPoolCancelStopsGrowthAndWaiters(t *testing.T) {
	poolCtx, cancelPool := context.WithCancel(context.Background())
	cfg := Config{Gateway: GatewayConfig{
		MaxUpstreams:        2,
		SessionsPerUpstream: 2,
		ChannelsPerUpstream: 6,
	}}
	first := &gatewayUpstream{active: 2}
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
	gatewayConfig.MaxSessions = clients
	gatewayConfig.MaxUpstreams = maxUpstreams
	gatewayConfig.SessionsPerUpstream = clients
	gatewayConfig.ChannelsPerUpstream = clients * 3
	gatewayConfig.MaxPendingHandshakes = clients
	gatewayConfig.DrainTimeout = time.Second
	cfg := Config{
		KeyPair:  edgeKey,
		Listen:   "127.0.0.1:0",
		Endpoint: "localhost:0",
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
	transport, ok := conn.(*gizwebrtc.Conn)
	if !ok {
		logicalCh <- acceptedGatewayLogical{err: fmt.Errorf("upstream type = %T", conn)}
		return
	}
	router, err := giztunnel.NewRouter(transport, giztunnel.Config{
		AcceptSessions: true,
		AllowRemoteService: func(giznet.PublicKey, uint64) bool {
			return true
		},
	})
	if err != nil {
		logicalCh <- acceptedGatewayLogical{err: err}
		return
	}
	defer router.Close()
	go func() {
		buf := make([]byte, 64*1024)
		for {
			protocol, n, err := conn.Read(buf)
			if err != nil {
				return
			}
			if protocol == giznet.ProtocolTunnelPacket {
				_ = router.HandlePacket(buf[:n])
			}
		}
	}()
	for {
		logical, declaration, err := router.Accept(ctx)
		if err != nil {
			return
		}
		go func(logical *giztunnel.Conn, key giznet.PublicKey) {
			accepted := acceptedGatewayLogical{logical: logical, key: key}
			select {
			case logicalCh <- accepted:
			case <-ctx.Done():
				if logical != nil {
					_ = logical.Close()
				}
			}
		}(logical, declaration.ClientPublicKey)
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

func TestGatewayCloseSessionsStartsEveryCloseConcurrently(t *testing.T) {
	const sessionCount = 8
	entered := make(chan struct{}, sessionCount)
	release := make(chan struct{})
	gateway := &Gateway{sessions: make(map[*gatewaySession]struct{}, sessionCount)}
	for range sessionCount {
		conn := &blockingCloseGiznetConn{entered: entered, release: release}
		gateway.sessions[&gatewaySession{client: conn}] = struct{}{}
	}

	done := make(chan struct{})
	go func() {
		gateway.closeSessions()
		close(done)
	}()
	for range sessionCount {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("session closes were serialized")
		}
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("closeSessions did not wait for every close")
	}
}

func TestGatewayPoolCloseStartsEveryUpstreamCloseConcurrently(t *testing.T) {
	const upstreamCount = 4
	entered := make(chan struct{}, upstreamCount)
	release := make(chan struct{})
	pool := &gatewayPool{entries: make([]*gatewayUpstream, 0, upstreamCount)}
	for range upstreamCount {
		conn := &blockingCloseGiznetConn{entered: entered, release: release}
		pool.entries = append(pool.entries, &gatewayUpstream{pool: pool, conn: conn})
	}

	done := make(chan error, 1)
	go func() { done <- pool.Close() }()
	for range upstreamCount {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("upstream closes were serialized")
		}
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pool Close did not wait for every upstream close")
	}
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
	body := `{"public_key":"server","version":"0.2.5","build_commit":"deadbeef","endpoint":"server:9820","signaling_path":"/offer","ice":{"udp":true,"tcp":true},"ice_servers":[{"urls":["turn:server"]}]}`
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
	if info["version"] != "0.2.5" || info["build_commit"] != "deadbeef" {
		t.Fatalf("gateway changed authoritative server metadata: %#v", info)
	}
	transport, ok := info["transport"].(map[string]any)
	if !ok || transport["public_key"] != "edge-key" {
		t.Fatalf("transport = %#v", info["transport"])
	}
}
