package gizclaw

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/giztunnel"
)

func validTunnelOpen(t *testing.T, now time.Time) (giztunnel.OpenRequest, giznet.PublicKey, giznet.PublicKey) {
	t.Helper()
	client, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	edge, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	server, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	sessionID, err := giztunnel.NewSessionID()
	if err != nil {
		t.Fatal(err)
	}
	return giztunnel.OpenRequest{
		SessionID:       sessionID,
		ClientPublicKey: client.Public,
		EdgePublicKey:   edge.Public,
		ServerPublicKey: server.Public,
		IssuedAtUnix:    now.Unix(),
		ExpiresAtUnix:   now.Add(30 * time.Second).Unix(),
		RemoteAddr:      "198.51.100.1:1234",
	}, edge.Public, server.Public
}

func TestValidateEdgeTunnelOpenIdentityTimeAndReplay(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	valid, edge, server := validTunnelOpen(t, now)
	tests := []struct {
		name   string
		change func(*giztunnel.OpenRequest, *giznet.PublicKey, *giznet.PublicKey)
		want   string
	}{
		{name: "valid"},
		{name: "wrong edge", change: func(_ *giztunnel.OpenRequest, edge *giznet.PublicKey, _ *giznet.PublicKey) {
			*edge = giznet.PublicKey{1}
		}, want: "edge identity mismatch"},
		{name: "wrong server", change: func(_ *giztunnel.OpenRequest, _ *giznet.PublicKey, server *giznet.PublicKey) {
			*server = giznet.PublicKey{1}
		}, want: "server identity mismatch"},
		{name: "expired", change: func(open *giztunnel.OpenRequest, _ *giznet.PublicKey, _ *giznet.PublicKey) {
			open.ExpiresAtUnix = now.Unix()
		}, want: "expired"},
		{name: "future", change: func(open *giztunnel.OpenRequest, _ *giznet.PublicKey, _ *giznet.PublicKey) {
			open.IssuedAtUnix = now.Add(6 * time.Second).Unix()
			open.ExpiresAtUnix = now.Add(30 * time.Second).Unix()
		}, want: "future"},
		{name: "long validity", change: func(open *giztunnel.OpenRequest, _ *giznet.PublicKey, _ *giznet.PublicKey) {
			open.ExpiresAtUnix = now.Add(31 * time.Second).Unix()
		}, want: "exceeds limit"},
		{name: "long remote address", change: func(open *giztunnel.OpenRequest, _ *giznet.PublicKey, _ *giznet.PublicKey) {
			open.RemoteAddr = strings.Repeat("x", edgeTunnelMaxRemoteAddrSize+1)
		}, want: "too long"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			open := valid
			physicalEdge := edge
			serverKey := server
			if tt.change != nil {
				tt.change(&open, &physicalEdge, &serverKey)
			}
			service := &PeerService{manager: &Manager{}}
			err := service.validateEdgeTunnelOpen(now, physicalEdge, serverKey, open)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("validate error = %v", err)
				}
				if err := service.validateEdgeTunnelOpen(now, physicalEdge, serverKey, open); err == nil ||
					!strings.Contains(err.Error(), "replayed") {
					t.Fatalf("replay error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validate error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestManagerAllowsEdgeTunnelOnlyForActiveEdgeNode(t *testing.T) {
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	peers := &peer.Server{Store: mustBadgerInMemory(t, nil)}
	if _, err := peers.EnsureConnectedPeer(context.Background(), key.Public); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(peers)
	if manager.allowService(context.Background(), key.Public, ServiceEdgeTunnel) {
		t.Fatal("client role allowed edge tunnel")
	}
	stored, err := peers.LoadPeer(context.Background(), key.Public)
	if err != nil {
		t.Fatal(err)
	}
	stored.Role = apitypes.PeerRoleEdgeNode
	stored.Status = apitypes.PeerRegistrationStatusActive
	if _, err := peers.SavePeer(context.Background(), stored); err != nil {
		t.Fatal(err)
	}
	if !manager.allowService(context.Background(), key.Public, ServiceEdgeTunnel) {
		t.Fatal("active edge-node did not allow edge tunnel")
	}
	stored.Status = apitypes.PeerRegistrationStatusBlocked
	if _, err := peers.SavePeer(context.Background(), stored); err != nil {
		t.Fatal(err)
	}
	if manager.allowService(context.Background(), key.Public, ServiceEdgeTunnel) {
		t.Fatal("inactive edge-node allowed edge tunnel")
	}
}

func TestManagerKeepsConcurrentEdgeTransportsIndependent(t *testing.T) {
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	peers := &peer.Server{Store: mustBadgerInMemory(t, nil)}
	if _, err := peers.EnsureConnectedPeer(context.Background(), key.Public); err != nil {
		t.Fatal(err)
	}
	stored, err := peers.LoadPeer(context.Background(), key.Public)
	if err != nil {
		t.Fatal(err)
	}
	stored.Role = apitypes.PeerRoleEdgeNode
	stored.Status = apitypes.PeerRegistrationStatusActive
	if _, err := peers.SavePeer(context.Background(), stored); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(peers)
	first := &testGiznetConn{publicKey: key.Public}
	second := &testGiznetConn{publicKey: key.Public}
	if err := manager.activateEdgeTransport(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := manager.activateEdgeTransport(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if got, ok := manager.Peer(key.Public); !ok || got != first {
		t.Fatalf("primary Edge transport = %v, %t", got, ok)
	}
	manager.setEdgeTransportDown(key.Public, first)
	if got, ok := manager.Peer(key.Public); !ok || got != second {
		t.Fatalf("surviving Edge transport = %v, %t", got, ok)
	}
	manager.setEdgeTransportDown(key.Public, second)
	if _, ok := manager.Peer(key.Public); ok {
		t.Fatal("Edge remained online after all transports closed")
	}
}
