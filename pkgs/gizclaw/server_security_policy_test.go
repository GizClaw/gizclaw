package gizclaw

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/giznetpb"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
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

func TestServerSecurityPolicyAllowsUnknownPeerBootstrapServices(t *testing.T) {
	policy := testServerSecurityPolicy(&peer.Server{Store: kv.NewMemory(nil)})
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

func TestServerSecurityPolicyAllowsGatewaySCTPOnlyForActiveEdgeNode(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	peers := &peer.Server{Store: mustBadgerInMemory(t, nil)}
	if _, err := peers.EnsureConnectedPeer(context.Background(), keyPair.Public); err != nil {
		t.Fatal(err)
	}
	policy := testServerSecurityPolicy(peers)
	if policy.AllowGatewaySCTP(keyPair.Public) {
		t.Fatal("client role allowed gateway SCTP profile")
	}
	stored, err := peers.LoadPeer(context.Background(), keyPair.Public)
	if err != nil {
		t.Fatal(err)
	}
	stored.Role = apitypes.PeerRoleEdgeNode
	stored.Status = apitypes.PeerRegistrationStatusActive
	if _, err := peers.SavePeer(context.Background(), stored); err != nil {
		t.Fatal(err)
	}
	if !policy.AllowGatewaySCTP(keyPair.Public) {
		t.Fatal("active edge-node did not receive gateway SCTP profile")
	}
	stored.Status = apitypes.PeerRegistrationStatusBlocked
	if _, err := peers.SavePeer(context.Background(), stored); err != nil {
		t.Fatal(err)
	}
	if policy.AllowGatewaySCTP(keyPair.Public) {
		t.Fatal("blocked edge-node retained gateway SCTP profile")
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

type admissionForwardingPolicy struct {
	ctx       context.Context
	admission giznet.PeerAdmission
	result    bool
}

func (p *admissionForwardingPolicy) AllowPeer(ctx context.Context, admission giznet.PeerAdmission) bool {
	p.ctx, p.admission = ctx, admission
	return p.result
}
func (*admissionForwardingPolicy) AllowService(giznet.PublicKey, uint64) bool { return false }

func TestServerSecurityPolicyForwardsAdmission(t *testing.T) {
	admission := giznet.PeerAdmission{PublicKey: giznet.PublicKey{1}, Credential: &giznetpb.AdmissionCredential{Version: 1, Type: "custom", Value: "opaque"}}
	if (*ServerSecurityPolicy)(nil).AllowPeer(t.Context(), admission) {
		t.Fatal("nil Server admitted")
	}
	s := &Server{}
	policy := (*ServerSecurityPolicy)(s)
	if !policy.AllowPeer(t.Context(), admission) {
		t.Fatal("default admission changed")
	}
	injected := &admissionForwardingPolicy{}
	s.SecurityPolicy = injected
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	for _, want := range []bool{false, true} {
		injected.result = want
		if policy.AllowPeer(ctx, admission) != want || injected.ctx != ctx || injected.admission.PublicKey != admission.PublicKey || injected.admission.Credential != admission.Credential {
			t.Fatal("admission was not delegated intact")
		}
	}
}

// Even a store whose next read is indefinitely blocked must not affect
// ordinary service authorization, including logical Edge service admission.
func TestOrdinaryPeerServicesDoNotReadSlowStore(t *testing.T) {
	store := &blockingGetStore{Store: kv.NewMemory(nil), entered: make(chan struct{}), release: make(chan struct{})}
	defer close(store.release)
	manager := NewManager(&peer.Server{Store: store})
	policy := (*ServerSecurityPolicy)(&Server{manager: manager})
	done := make(chan bool, 1)
	go func() {
		for _, service := range []uint64{ServicePeerRPC, ServicePeerHTTP, ServicePeerOpenAI, EventStreamAgent} {
			if !policy.AllowService(giznet.PublicKey{73}, service) || !manager.allowService(t.Context(), giznet.PublicKey{74}, service) {
				done <- false
				return
			}
		}
		done <- true
	}()
	select {
	case allowed := <-done:
		if !allowed {
			t.Fatal("slow store denied ordinary Peer service")
		}
	case <-store.entered:
		t.Fatal("ordinary Peer service attempted a storage read")
	case <-time.After(time.Second):
		t.Fatal("ordinary Peer service did not make progress")
	}
	select {
	case <-store.entered:
		t.Fatal("ordinary service queried storage")
	default:
	}
}

type serviceContextStore struct {
	kv.Store
	readContext context.Context
}

func (s *serviceContextStore) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	s.readContext = ctx
	return s.Store.Get(ctx, key)
}

func TestRoleServicesPreserveLookupContextAndBlockedDenial(t *testing.T) {
	for _, test := range []struct {
		service uint64
		role    apitypes.PeerRole
	}{
		{ServiceAdminHTTP, apitypes.PeerRoleAdmin},
		{ServiceEdgeHTTP, apitypes.PeerRoleEdgeNode},
		{ServiceEdgeRPC, apitypes.PeerRoleEdgeNode},
	} {
		key := giznet.PublicKey{75}
		base := kv.NewMemory(nil)
		peers := &peer.Server{Store: base}
		manager := NewManager(peers)
		for _, status := range []apitypes.PeerRegistrationStatus{apitypes.PeerRegistrationStatusActive, apitypes.PeerRegistrationStatusBlocked} {
			peers.Store = base
			if _, err := peers.SavePeer(t.Context(), apitypes.Peer{PublicKey: key.String(), Role: test.role, Status: status}); err != nil {
				t.Fatal(err)
			}
			store := &serviceContextStore{Store: base}
			peers.Store = store
			ctx := context.Background()
			allowed := manager.allowService(ctx, key, test.service)
			if allowed != (status == apitypes.PeerRegistrationStatusActive) {
				t.Fatalf("role service %x with status %s: allowed=%v", test.service, status, allowed)
			}
			if store.readContext != ctx {
				t.Fatal("role service replaced its original lookup context")
			}
			if _, hasDeadline := store.readContext.Deadline(); hasDeadline {
				t.Fatal("role service added a deadline")
			}
		}
	}
}
