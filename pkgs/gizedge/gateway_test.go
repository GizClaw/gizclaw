package gizedge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/giztunnel"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

type gatewayAllowAllPolicy struct{}

func (gatewayAllowAllPolicy) AllowPeer(giznet.PublicKey) bool { return true }
func (gatewayAllowAllPolicy) AllowService(giznet.PublicKey, uint64) bool {
	return true
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
	gateway, err := newGateway(t.Context(), cfg, upstreamURL)
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
	if selected != first || !first.draining {
		t.Fatalf("selected=%p draining=%t, want first draining", selected, first.draining)
	}
	release()
	if len(pool.entries) != 1 || pool.entries[0] != second {
		t.Fatalf("rotated entries = %+v", pool.entries)
	}
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
