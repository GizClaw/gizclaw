package gizclaw

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/giztunnel"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

func TestAcceptedPeerStreamLifecycleIsAlwaysConstructedForAuditContent(t *testing.T) {
	declaration := giztunnel.SessionDeclaration{SessionID: giztunnel.SessionID{1}}
	warnLogger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if lifecycle := newAcceptedPeerStreamLifecycle(warnLogger, declaration); lifecycle == nil {
		t.Fatal("Warn logger did not construct the required audit observer")
	}
}

func TestServeEdgeTunnelCarriesAcceptedSessionCorrelation(t *testing.T) {
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
	edgeConn, serverConn := newTestWebRTCConnPair(
		t,
		serverKey,
		edgeKey,
		testGiznetSecurityPolicy{},
		testGiznetSecurityPolicy{},
	)
	t.Cleanup(func() {
		_ = edgeConn.Close()
		_ = serverConn.Close()
	})
	edgeTransport, ok := edgeConn.(*gizwebrtc.Conn)
	if !ok {
		t.Fatalf("edge connection type = %T", edgeConn)
	}
	router, err := giztunnel.NewRouter(edgeTransport, giztunnel.Config{})
	if err != nil {
		t.Fatalf("NewRouter(edge) error = %v", err)
	}
	t.Cleanup(func() { _ = router.Close() })

	capture := captureSlog(t)
	host := &PeerConn{Conn: serverConn, Service: &PeerService{manager: &Manager{}}}
	serveDone := make(chan error, 1)
	go func() { serveDone <- host.serveEdgeTunnel() }()

	sessionID, err := giztunnel.NewSessionID()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	logical, err := router.Dial(ctx, giztunnel.SessionDeclaration{
		SessionID:       sessionID,
		ClientPublicKey: clientKey.Public,
	})
	if err != nil {
		t.Fatalf("Dial(logical) error = %v", err)
	}
	_ = logical.Close()

	deadline := time.Now().Add(5 * time.Second)
	for {
		records := capturedLifecycleRecords(t, capture)
		if len(records) >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("lifecycle records = %d, want accepted and terminal", len(records))
		}
		time.Sleep(10 * time.Millisecond)
	}
	records := capturedLifecycleRecords(t, capture)
	wantStages := []string{"session_accepted", "terminal"}
	for i, wantStage := range wantStages {
		attrs := lifecycleRecordAttrs(records[i])
		if attrs["component"] != "server_tunnel" || attrs["stage"] != wantStage {
			t.Fatalf("record[%d] = %#v", i, attrs)
		}
		if attrs["tunnel_session_id"] != sessionID.String() || attrs["peer_public_key"] != clientKey.Public.String() {
			t.Fatalf("record[%d] correlation = %#v", i, attrs)
		}
	}

	if host.tunnelRouter != nil {
		_ = host.tunnelRouter.Close()
	}
	_ = serverConn.Close()
	select {
	case <-serveDone:
	case <-time.After(5 * time.Second):
		t.Fatal("serveEdgeTunnel did not stop")
	}
}

func TestEdgeTunnelRemoteServiceUsesLogicalClientIdentity(t *testing.T) {
	client, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{}
	if manager.allowService(context.Background(), client.Public, ServiceEdgeHTTP) {
		t.Fatal("unregistered logical client unexpectedly authorized for edge service")
	}
	if !manager.allowService(context.Background(), client.Public, ServicePeerRPC) {
		t.Fatal("logical client should retain normal peer RPC service")
	}
	if manager.allowActivePeerRole(context.Background(), client.Public, apitypes.PeerRoleEdgeNode) {
		t.Fatal("unregistered logical client unexpectedly has edge role")
	}
}

func TestRetiredEdgeTunnelCarrierIsNotAuthorized(t *testing.T) {
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{}
	if manager.allowService(context.Background(), key.Public, 0x32) {
		t.Fatal("retired ServiceEdgeTunnel carrier is still authorized")
	}
}
