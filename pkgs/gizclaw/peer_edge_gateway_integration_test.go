package gizclaw_test

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizedge"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

type edgeGatewayLifecycleCapture struct {
	mu      sync.Mutex
	records []slog.Record
}

func (*edgeGatewayLifecycleCapture) Enabled(context.Context, slog.Level) bool { return true }
func (h *edgeGatewayLifecycleCapture) WithAttrs([]slog.Attr) slog.Handler     { return h }
func (h *edgeGatewayLifecycleCapture) WithGroup(string) slog.Handler          { return h }
func (h *edgeGatewayLifecycleCapture) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	h.records = append(h.records, record.Clone())
	h.mu.Unlock()
	return nil
}

func TestEdgeGatewayAndServerTunnelShareSessionCorrelation(t *testing.T) {
	edgeKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	server := startEdgeGatewayTestServer(t, edgeKey.Public)

	capture := &edgeGatewayLifecycleCapture{}
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(capture))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	edgeAddr := probe.Addr().String()
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	config := fmt.Sprintf(`identity:
  private-key: %s
webrtc:
  listen: %s
  endpoint: %s
upstreams:
  - endpoint: %s
    public-key: %s
http:
  listeners:
    - listen: %s
gateway:
  enabled: true
  max-sessions: 4
  max-upstreams: 1
  sessions-per-upstream: 4
  channels-per-session: 8
  channels-per-upstream: 12
  max-pending-handshakes: 4
  session-buffer-bytes: 1048576
  idle-timeout: 1m
  drain-timeout: 1s
`, edgeKey.Private.String(), edgeAddr, edgeAddr, server.baseURL, server.server.PublicKey().String(), edgeAddr)
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	edgeCtx, cancelEdge := context.WithCancel(t.Context())
	edgeDone := make(chan error, 1)
	go func() { edgeDone <- gizedge.ServeContext(edgeCtx, root) }()
	t.Cleanup(func() {
		cancelEdge()
		select {
		case err := <-edgeDone:
			if err != nil {
				t.Errorf("edge shutdown error = %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("edge did not stop")
		}
	})

	var clientListener giznet.Listener
	var clientConn giznet.Conn
	if err := waitUntil(10*time.Second, func() error {
		select {
		case serveErr := <-edgeDone:
			return fmt.Errorf("edge exited before client dial: %w", serveErr)
		default:
		}
		listener, conn, dialErr := gizwebrtc.Dial(
			t.Context(), clientKey, edgeKey.Public,
			gizwebrtc.DialConfig{
				SignalingURL:   "http://" + edgeAddr + gizwebrtc.SignalingPath,
				SecurityPolicy: allowAllEdgeGatewayPolicy{},
			},
		)
		if dialErr != nil {
			return dialErr
		}
		clientListener, clientConn = listener, conn
		return nil
	}); err != nil {
		t.Fatalf("dial Edge gateway: %v", err)
	}
	t.Cleanup(func() { _ = clientListener.Close() })
	t.Cleanup(func() { _ = clientConn.Close() })

	var matchedSessionID string
	if err := waitUntil(10*time.Second, func() error {
		edgeSessions, serverSessions := lifecycleSessionsByOwner(capture, clientKey.Public.String())
		for sessionID := range edgeSessions {
			if serverSessions[sessionID] {
				matchedSessionID = sessionID
				return nil
			}
		}
		return fmt.Errorf("no shared session ID: edge=%v server=%v", edgeSessions, serverSessions)
	}); err != nil {
		t.Fatal(err)
	}
	if matchedSessionID == "" {
		t.Fatal("matched an empty tunnel_session_id")
	}
}

type allowAllEdgeGatewayPolicy struct{}

func (allowAllEdgeGatewayPolicy) AllowPeer(giznet.PublicKey) bool { return true }
func (allowAllEdgeGatewayPolicy) AllowService(giznet.PublicKey, uint64) bool {
	return true
}

type edgeGatewayTestServer struct {
	server  *gizclaw.Server
	baseURL string
	errCh   chan error
}

func startEdgeGatewayTestServer(t *testing.T, edgePublicKey giznet.PublicKey) *edgeGatewayTestServer {
	t.Helper()
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	srv := completeExternalTestServer(t, &gizclaw.Server{
		LocalStatic: *serverKey,
		PeerStore:   mustBadgerInMemory(t, nil),
		EdgeNodes:   []giznet.PublicKey{edgePublicKey},
	})
	var signalingServer *httptest.Server
	srv.PeerListenerFactories = []gizclaw.PeerListenerFactory{
		func(opts gizclaw.PeerListenerOptions) (giznet.Listener, error) {
			listener, listenErr := (&gizwebrtc.ListenConfig{
				SecurityPolicy:   opts.SecurityPolicy,
				PeerEventHandler: opts.PeerEventHandler,
			}).Listen(opts.KeyPair)
			if listenErr != nil {
				return nil, listenErr
			}
			signalingServer = httptest.NewServer(listener.SignalingHandler())
			return listener, nil
		},
	}
	if err := srv.Listen(); err != nil {
		t.Fatal(err)
	}
	if signalingServer == nil {
		t.Fatal("missing Server signaling endpoint")
	}
	result := &edgeGatewayTestServer{
		server:  srv,
		baseURL: signalingServer.URL,
		errCh:   make(chan error, 1),
	}
	go func() { result.errCh <- srv.Serve() }()
	t.Cleanup(func() {
		_ = srv.Close()
		signalingServer.Close()
		select {
		case err := <-result.errCh:
			if err != nil {
				t.Errorf("Server shutdown error = %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("Server did not stop")
		}
	})
	return result
}

func lifecycleSessionsByOwner(
	capture *edgeGatewayLifecycleCapture,
	peerPublicKey string,
) (map[string]bool, map[string]bool) {
	capture.mu.Lock()
	records := append([]slog.Record(nil), capture.records...)
	capture.mu.Unlock()
	edgeSessions := make(map[string]bool)
	serverSessions := make(map[string]bool)
	for _, record := range records {
		if record.Message != "gizclaw: peer stream lifecycle" {
			continue
		}
		attrs := make(map[string]any)
		record.Attrs(func(attr slog.Attr) bool {
			attrs[attr.Key] = attr.Value.Any()
			return true
		})
		if attrs["stage"] != "session_accepted" || attrs["peer_public_key"] != peerPublicKey {
			continue
		}
		sessionID, _ := attrs["tunnel_session_id"].(string)
		if sessionID == "" {
			continue
		}
		switch attrs["component"] {
		case "edge_gateway":
			edgeSessions[sessionID] = true
		case "server_tunnel":
			serverSessions[sessionID] = true
		}
	}
	return edgeSessions, serverSessions
}
