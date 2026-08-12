package gizclaw

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

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
