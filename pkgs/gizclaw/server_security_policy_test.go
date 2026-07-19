package gizclaw

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func testServerSecurityPolicy(peers *peer.Server) *ServerSecurityPolicy {
	return (*ServerSecurityPolicy)(&Server{manager: NewManager(peers)})
}

func TestServerSecurityPolicyRequiresAdminRoleForAdminService(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}

	service := &peer.Server{Store: mustBadgerInMemory(t, nil)}
	if _, err := service.EnsureConnectedPeer(context.Background(), keyPair.Public); err != nil {
		t.Fatalf("EnsureConnectedPeer error = %v", err)
	}
	policy := testServerSecurityPolicy(service)
	if policy.AllowService(keyPair.Public, ServiceAdminHTTP) {
		t.Fatal("non-admin peer should not allow admin service")
	}
	stored, err := service.LoadPeer(context.Background(), keyPair.Public)
	if err != nil {
		t.Fatalf("LoadPeer error = %v", err)
	}
	if stored.Role != apitypes.PeerRoleClient {
		t.Fatalf("policy changed stored role to %q", stored.Role)
	}
}

func TestServerSecurityPolicyAllowsPublicServicesWithoutPeerLookup(t *testing.T) {
	policy := (*ServerSecurityPolicy)(&Server{manager: &Manager{}})
	if !policy.AllowService(giznet.PublicKey{}, ServicePeerRPC) {
		t.Fatal("policy should allow rpc service")
	}
	if !policy.AllowService(giznet.PublicKey{}, ServicePeerHTTP) {
		t.Fatal("policy should allow server public service")
	}
	if policy.AllowService(giznet.PublicKey{}, ServiceEdgeHTTP) {
		t.Fatal("policy should not allow edge HTTP service without peer lookup")
	}
}

func TestServerSecurityPolicyDeniesAdminServiceForUnknownPeer(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}

	policy := testServerSecurityPolicy(&peer.Server{Store: mustBadgerInMemory(t, nil)})
	if policy.AllowService(keyPair.Public, ServiceAdminHTTP) {
		t.Fatal("unknown peer should not allow admin service")
	}
}
