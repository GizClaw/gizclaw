//go:build giznet_e2e

package webrtc_test

import (
	"bytes"
	"context"
	"net"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/pion/turn/v4"
	"github.com/pion/webrtc/v4"
)

const (
	turnRealm      = "giznet-e2e"
	turnUsername   = "giznet"
	turnCredential = "giznet-secret"
)

func TestWebRTCTURNRelayPacketAndServiceStream(t *testing.T) {
	relay := startLocalTURN(t)
	serverKey := mustKeyPair(t)
	clientKey := mustKeyPair(t)

	server := startWebRTCServerWithConfig(t, serverKey, gizwebrtc.ListenConfig{
		CipherMode:         gizwebrtc.CipherModePlaintext,
		SecurityPolicy:     allowAllPolicy{},
		ICEServers:         relay.iceServers,
		ICETransportPolicy: webrtc.ICETransportPolicyRelay,
	})
	defer server.Close()

	clientListener, clientConn := dialWebRTCWithConfig(t, clientKey, serverKey.Public, server.signalingURL, gizwebrtc.DialConfig{
		CipherMode:         gizwebrtc.CipherModePlaintext,
		SecurityPolicy:     allowAllPolicy{},
		ICEServers:         relay.iceServers,
		ICETransportPolicy: webrtc.ICETransportPolicyRelay,
	})
	defer clientListener.Close()
	defer clientConn.Close()

	serverConn := acceptConn(t, server.listener)
	defer serverConn.Close()
	waitTURNAllocations(t, relay, 2)

	roundTripPacket(t, clientConn, serverConn, 0x42, []byte("turn relay packet"))

	done := serveEchoService(t, serverConn)
	payload := bytes.Repeat([]byte("turn-relay-stream-"), 1024)
	if got := roundTripStream(t, clientConn, payload); !bytes.Equal(got, payload) {
		t.Fatalf("stream echo len=%d, want %d", len(got), len(payload))
	}
	serverConn.CloseService(echoService)
	waitServerDone(t, done)
}

type localTURNRelay struct {
	packetConn net.PacketConn
	server     *turn.Server
	iceServers []gizwebrtc.ICEServer
}

func startLocalTURN(tb testing.TB) *localTURNRelay {
	tb.Helper()
	packetConn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		tb.Fatalf("listen TURN UDP error = %v", err)
	}
	relay := &localTURNRelay{packetConn: packetConn}
	server, err := turn.NewServer(turn.ServerConfig{
		Realm: turnRealm,
		AuthHandler: func(username, realm string, _ net.Addr) ([]byte, bool) {
			if username != turnUsername || realm != turnRealm {
				return nil, false
			}
			return turn.GenerateAuthKey(username, realm, turnCredential), true
		},
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: packetConn,
				RelayAddressGenerator: &turn.RelayAddressGeneratorNone{
					Address: "127.0.0.1",
				},
			},
		},
	})
	if err != nil {
		_ = packetConn.Close()
		tb.Fatalf("start TURN server error = %v", err)
	}
	relay.server = server
	relay.iceServers = []gizwebrtc.ICEServer{
		{
			URLs:       []string{"turn:" + packetConn.LocalAddr().String() + "?transport=udp"},
			Username:   turnUsername,
			Credential: turnCredential,
		},
	}
	tb.Cleanup(func() {
		if relay.server != nil {
			_ = relay.server.Close()
		}
		if relay.packetConn != nil {
			_ = relay.packetConn.Close()
		}
	})
	return relay
}

func waitTURNAllocations(tb testing.TB, relay *localTURNRelay, min int) {
	tb.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if relay.server.AllocationCount() >= min {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	tb.Fatalf("TURN allocations = %d, want at least %d", relay.server.AllocationCount(), min)
}

func startWebRTCServerWithConfig(tb testing.TB, key *giznet.KeyPair, cfg gizwebrtc.ListenConfig) *webRTCServer {
	tb.Helper()
	listener, err := cfg.Listen(key)
	if err != nil {
		tb.Fatalf("gizwebrtc Listen error = %v", err)
	}
	httpServer := httptest.NewServer(listener.SignalingHandler())
	return &webRTCServer{
		listener:     listener,
		httpServer:   httpServer,
		signalingURL: httpServer.URL + gizwebrtc.SignalingPath,
	}
}

func dialWebRTCWithConfig(tb testing.TB, key *giznet.KeyPair, serverPK giznet.PublicKey, signalingURL string, cfg gizwebrtc.DialConfig) (giznet.Listener, giznet.Conn) {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cfg.SignalingURL = signalingURL
	listener, conn, err := gizwebrtc.Dial(ctx, key, serverPK, cfg)
	if err != nil {
		tb.Fatalf("gizwebrtc Dial error = %v", err)
	}
	return listener, conn
}
