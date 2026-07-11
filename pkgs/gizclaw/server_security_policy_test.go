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

func TestServerSecurityPolicyAllowsPublicServiceForActivePeer(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}

	service := &peer.Server{Store: mustBadgerInMemory(t, nil)}
	if _, err := service.SavePeer(context.Background(), apitypes.Peer{
		PublicKey:     keyPair.Public.String(),
		Role:          apitypes.PeerRoleClient,
		Status:        apitypes.PeerRegistrationStatusActive,
		Device:        apitypes.DeviceInfo{},
		Configuration: apitypes.Configuration{},
	}); err != nil {
		t.Fatalf("SavePeer error = %v", err)
	}

	policy := testServerSecurityPolicy(service)
	if policy.AllowService(keyPair.Public, ServiceAdminHTTP) {
		t.Fatal("active peer should not allow admin service without admin role")
	}
	if !policy.AllowService(keyPair.Public, ServicePeerHTTP) {
		t.Fatal("active peer should allow server public service")
	}
	if policy.AllowService(keyPair.Public, 0xffff) {
		t.Fatal("active peer should not allow unknown service")
	}
}

func TestServerSecurityPolicyAllowsAdminServiceForActiveAdminPeer(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}

	service := &peer.Server{Store: mustBadgerInMemory(t, nil)}
	if _, err := service.SavePeer(context.Background(), apitypes.Peer{
		PublicKey:     keyPair.Public.String(),
		Role:          apitypes.PeerRoleAdmin,
		Status:        apitypes.PeerRegistrationStatusActive,
		Device:        apitypes.DeviceInfo{},
		Configuration: apitypes.Configuration{},
	}); err != nil {
		t.Fatalf("SavePeer error = %v", err)
	}

	policy := testServerSecurityPolicy(service)
	if !policy.AllowService(keyPair.Public, ServiceAdminHTTP) {
		t.Fatal("active admin peer should allow admin service")
	}
}

func TestServerSecurityPolicyAllowsEdgeRPCOnlyForActiveEdgeNode(t *testing.T) {
	ctx := context.Background()
	edgeKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(edge) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}
	blockedKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(blocked) error = %v", err)
	}

	service := &peer.Server{Store: mustBadgerInMemory(t, nil)}
	for _, item := range []struct {
		key    giznet.PublicKey
		role   apitypes.PeerRole
		status apitypes.PeerRegistrationStatus
	}{
		{key: edgeKey.Public, role: apitypes.PeerRoleEdgeNode, status: apitypes.PeerRegistrationStatusActive},
		{key: clientKey.Public, role: apitypes.PeerRoleClient, status: apitypes.PeerRegistrationStatusActive},
		{key: blockedKey.Public, role: apitypes.PeerRoleEdgeNode, status: apitypes.PeerRegistrationStatusBlocked},
	} {
		if _, err := service.SavePeer(ctx, apitypes.Peer{
			PublicKey:     item.key.String(),
			Role:          item.role,
			Status:        item.status,
			Device:        apitypes.DeviceInfo{},
			Configuration: apitypes.Configuration{},
		}); err != nil {
			t.Fatalf("SavePeer(%s) error = %v", item.key, err)
		}
	}

	policy := testServerSecurityPolicy(service)
	if !policy.AllowService(edgeKey.Public, ServiceEdgeRPC) {
		t.Fatal("active edge-node should allow edge RPC")
	}
	if policy.AllowService(edgeKey.Public, ServiceAdminHTTP) {
		t.Fatal("edge-node should not allow admin HTTP")
	}
	if policy.AllowService(clientKey.Public, ServiceEdgeRPC) {
		t.Fatal("active client should not allow edge RPC")
	}
	if policy.AllowService(blockedKey.Public, ServiceEdgeRPC) {
		t.Fatal("blocked edge-node should not allow edge RPC")
	}
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

func TestServerSecurityPolicyDeniesProtectedServicesForBlockedPeer(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}

	service := &peer.Server{Store: mustBadgerInMemory(t, nil)}
	ctx := context.Background()
	if _, err := service.SavePeer(ctx, apitypes.Peer{
		PublicKey:     keyPair.Public.String(),
		Role:          apitypes.PeerRoleUnspecified,
		Status:        apitypes.PeerRegistrationStatusBlocked,
		Device:        apitypes.DeviceInfo{},
		Configuration: apitypes.Configuration{},
	}); err != nil {
		t.Fatalf("SavePeer error = %v", err)
	}

	policy := testServerSecurityPolicy(service)
	if policy.AllowService(keyPair.Public, ServiceAdminHTTP) {
		t.Fatal("blocked peer should not allow admin service")
	}
	if policy.AllowService(keyPair.Public, 0xffff) {
		t.Fatal("blocked peer should not allow unknown service")
	}
}
