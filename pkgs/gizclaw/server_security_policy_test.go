package gizclaw

import (
	"context"
	"errors"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/giznetpb"
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

func TestServerSecurityPolicyBlockedCannotUseHostFallback(t *testing.T) {
	key := giznet.PublicKey{73}
	peers := &peer.Server{Store: kv.NewMemory(nil)}
	if _, err := peers.SavePeer(t.Context(), apitypes.Peer{PublicKey: key.String(), Role: apitypes.PeerRoleAdmin, Status: apitypes.PeerRegistrationStatusBlocked}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	policy := (*ServerSecurityPolicy)(&Server{manager: NewManager(peers), SecurityPolicy: testGiznetSecurityPolicy{
		allowService: func(giznet.PublicKey, uint64) bool { calls++; return true },
	}})
	for _, service := range []uint64{ServicePeerRPC, ServicePeerHTTP, ServicePeerOpenAI, EventStreamAgent, ServiceAdminHTTP, ServiceEdgeHTTP, ServiceEdgeRPC, 0xff} {
		if policy.AllowService(key, service) {
			t.Fatalf("blocked Peer opened service %x", service)
		}
	}
	if calls != 0 {
		t.Fatal("blocked denial reached host fallback")
	}
}

type deadlinePeerStore struct{ kv.Store }

func (s deadlinePeerStore) Get(ctx context.Context, _ kv.Key) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestServerSecurityPolicyLookupFailsClosedAndIsBounded(t *testing.T) {
	for _, store := range []kv.Store{
		&failingGetStore{err: errors.New("store unavailable")},
		deadlinePeerStore{},
	} {
		policy := (*ServerSecurityPolicy)(&Server{manager: NewManager(&peer.Server{Store: store}), SecurityPolicy: testGiznetSecurityPolicy{
			allowService: func(giznet.PublicKey, uint64) bool { t.Error("failed lookup reached fallback"); return true },
		}})
		start := time.Now()
		if policy.AllowService(giznet.PublicKey{74}, ServicePeerRPC) {
			t.Fatal("failed lookup allowed service")
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("service lookup exceeded bound: %s", elapsed)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if NewManager(&peer.Server{Store: kv.NewMemory(nil)}).allowService(ctx, giznet.PublicKey{75}, EventStreamAgent) {
		t.Fatal("cancelled service lookup allowed bootstrap")
	}
}
