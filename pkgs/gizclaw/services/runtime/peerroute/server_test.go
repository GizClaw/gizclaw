package peerroute

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type testPeers struct {
	items map[giznet.PublicKey]apitypes.Peer
	err   error
}

func (p testPeers) LoadPeer(_ context.Context, publicKey giznet.PublicKey) (apitypes.Peer, error) {
	if p.err != nil {
		return apitypes.Peer{}, p.err
	}
	peer, ok := p.items[publicKey]
	if !ok {
		return apitypes.Peer{}, kv.ErrNotFound
	}
	return peer, nil
}

func TestAssignLookupAndRefresh(t *testing.T) {
	ctx := context.Background()
	peerKey := giznet.PublicKey{1}
	serverKey := giznet.PublicKey{2}
	otherServerKey := giznet.PublicKey{3}
	service := &Server{
		Store:           kv.NewMemory(nil),
		ServerPublicKey: serverKey,
		ServerEndpoint:  "server-a:9820",
		Peers: testPeers{items: map[giznet.PublicKey]apitypes.Peer{
			peerKey: {
				PublicKey:     peerKey.String(),
				Role:          apitypes.PeerRoleClient,
				Status:        apitypes.PeerRegistrationStatusActive,
				Device:        apitypes.DeviceInfo{},
				Configuration: apitypes.Configuration{},
			},
		}},
	}

	created, err := service.Assign(ctx, peerKey, nil)
	if err != nil {
		t.Fatalf("Assign create error = %v", err)
	}
	if created.Version != 1 || created.ServerPublicKey != serverKey.String() || created.ServerEndpoint != "server-a:9820" || created.Role != apitypes.PeerRoleClient {
		t.Fatalf("created assignment = %+v", created)
	}
	existing, err := service.Assign(ctx, peerKey, nil)
	if err != nil {
		t.Fatalf("Assign existing error = %v", err)
	}
	if existing.Version != 1 {
		t.Fatalf("idempotent assign version = %d, want 1", existing.Version)
	}

	service.ServerPublicKey = otherServerKey
	service.ServerEndpoint = "server-b:9820"
	refreshed, err := service.Assign(ctx, peerKey, nil)
	if err != nil {
		t.Fatalf("Assign changed route refresh error = %v", err)
	}
	if refreshed.Version != 2 || refreshed.ServerPublicKey != otherServerKey.String() || refreshed.ServerEndpoint != "server-b:9820" {
		t.Fatalf("refreshed assignment = %+v", refreshed)
	}
	service.ServerEndpoint = "server-c:9820"
	version := refreshed.Version
	refreshed, err = service.Assign(ctx, peerKey, &version)
	if err != nil {
		t.Fatalf("Assign CAS refresh error = %v", err)
	}
	if refreshed.Version != 3 || refreshed.ServerEndpoint != "server-c:9820" {
		t.Fatalf("CAS refreshed assignment = %+v", refreshed)
	}
	loaded, err := service.Lookup(ctx, peerKey)
	if err != nil {
		t.Fatalf("Lookup error = %v", err)
	}
	if loaded != refreshed {
		t.Fatalf("Lookup = %+v, want %+v", loaded, refreshed)
	}
	resolved, err := service.Resolve(ctx, peerKey)
	if err != nil {
		t.Fatalf("Resolve error = %v", err)
	}
	if resolved != refreshed {
		t.Fatalf("Resolve = %+v, want %+v", resolved, refreshed)
	}
}

func TestAssignConflictAndValidation(t *testing.T) {
	ctx := context.Background()
	peerKey := giznet.PublicKey{1}
	service := &Server{
		Store:           kv.NewMemory(nil),
		ServerPublicKey: giznet.PublicKey{2},
		ServerEndpoint:  "server:9820",
		Peers: testPeers{items: map[giznet.PublicKey]apitypes.Peer{
			peerKey: {
				PublicKey:     peerKey.String(),
				Role:          apitypes.PeerRoleClient,
				Status:        apitypes.PeerRegistrationStatusActive,
				Device:        apitypes.DeviceInfo{},
				Configuration: apitypes.Configuration{},
			},
		}},
	}
	if _, err := service.Assign(ctx, peerKey, nil); err != nil {
		t.Fatalf("Assign create error = %v", err)
	}
	stale := int64(99)
	if _, err := service.Assign(ctx, peerKey, &stale); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("Assign stale error = %v, want %v", err, ErrVersionConflict)
	}
	if _, err := service.Assign(ctx, giznet.PublicKey{}, nil); !errors.Is(err, ErrInvalidPublicKey) {
		t.Fatalf("Assign zero public key error = %v, want %v", err, ErrInvalidPublicKey)
	}
	missingRoute := *service
	missingRoute.ServerEndpoint = ""
	if _, err := missingRoute.Assign(ctx, peerKey, nil); !errors.Is(err, ErrMissingRoute) {
		t.Fatalf("Assign missing route error = %v, want %v", err, ErrMissingRoute)
	}
	missingPeer := *service
	missingPeer.Peers = testPeers{}
	if _, err := missingPeer.Assign(ctx, giznet.PublicKey{9}, nil); err == nil {
		t.Fatal("Assign unknown peer succeeded")
	}
	blockedPeer := *service
	blockedPeer.Peers = testPeers{items: map[giznet.PublicKey]apitypes.Peer{
		peerKey: {
			PublicKey:     peerKey.String(),
			Role:          apitypes.PeerRoleClient,
			Status:        apitypes.PeerRegistrationStatusBlocked,
			Device:        apitypes.DeviceInfo{},
			Configuration: apitypes.Configuration{},
		},
	}}
	if _, err := blockedPeer.Assign(ctx, peerKey, nil); !errors.Is(err, ErrPeerInactive) {
		t.Fatalf("Assign blocked peer error = %v, want %v", err, ErrPeerInactive)
	}
	serverPeerKey := giznet.PublicKey{3}
	serverPeer := &Server{
		Store:           kv.NewMemory(nil),
		ServerPublicKey: giznet.PublicKey{2},
		ServerEndpoint:  "server:9820",
	}
	serverPeer.Peers = testPeers{items: map[giznet.PublicKey]apitypes.Peer{
		serverPeerKey: {
			PublicKey:     serverPeerKey.String(),
			Role:          apitypes.PeerRoleServer,
			Status:        apitypes.PeerRegistrationStatusActive,
			Device:        apitypes.DeviceInfo{},
			Configuration: apitypes.Configuration{},
		},
	}}
	if _, err := serverPeer.Assign(ctx, serverPeerKey, nil); !errors.Is(err, ErrPeerNotAssignable) {
		t.Fatalf("Assign server peer error = %v, want %v", err, ErrPeerNotAssignable)
	}
	if _, err := serverPeer.Lookup(ctx, serverPeerKey); !errors.Is(err, ErrAssignmentNotFound) {
		t.Fatalf("Lookup server peer assignment error = %v, want %v", err, ErrAssignmentNotFound)
	}
}

func TestResolveRequiresActiveClientPeer(t *testing.T) {
	ctx := context.Background()
	peerKey := giznet.PublicKey{1}
	peerRecord := apitypes.Peer{
		PublicKey:     peerKey.String(),
		Role:          apitypes.PeerRoleClient,
		Status:        apitypes.PeerRegistrationStatusActive,
		Device:        apitypes.DeviceInfo{},
		Configuration: apitypes.Configuration{},
	}
	service := &Server{
		Store:           kv.NewMemory(nil),
		ServerPublicKey: giznet.PublicKey{2},
		ServerEndpoint:  "server:9820",
		Peers:           testPeers{items: map[giznet.PublicKey]apitypes.Peer{peerKey: peerRecord}},
	}
	if _, err := service.Assign(ctx, peerKey, nil); err != nil {
		t.Fatalf("Assign create error = %v", err)
	}
	peerRecord.Status = apitypes.PeerRegistrationStatusBlocked
	service.Peers = testPeers{items: map[giznet.PublicKey]apitypes.Peer{peerKey: peerRecord}}
	if _, err := service.Resolve(ctx, peerKey); !errors.Is(err, ErrPeerInactive) {
		t.Fatalf("Resolve blocked peer error = %v, want %v", err, ErrPeerInactive)
	}
	peerRecord.Status = apitypes.PeerRegistrationStatusActive
	peerRecord.Role = apitypes.PeerRoleAdmin
	service.Peers = testPeers{items: map[giznet.PublicKey]apitypes.Peer{peerKey: peerRecord}}
	if _, err := service.Resolve(ctx, peerKey); !errors.Is(err, ErrPeerNotAssignable) {
		t.Fatalf("Resolve admin peer error = %v, want %v", err, ErrPeerNotAssignable)
	}
	service.Peers = testPeers{}
	if _, err := service.Resolve(ctx, peerKey); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("Resolve deleted peer error = %v, want %v", err, kv.ErrNotFound)
	}
}

func TestResolveRefreshesStaleAssignment(t *testing.T) {
	ctx := context.Background()
	peerKey := giznet.PublicKey{1}
	service := &Server{
		Store:           kv.NewMemory(nil),
		ServerPublicKey: giznet.PublicKey{2},
		ServerEndpoint:  "server-a:9820",
		Peers: testPeers{items: map[giznet.PublicKey]apitypes.Peer{
			peerKey: {
				PublicKey:     peerKey.String(),
				Role:          apitypes.PeerRoleClient,
				Status:        apitypes.PeerRegistrationStatusActive,
				Device:        apitypes.DeviceInfo{},
				Configuration: apitypes.Configuration{},
			},
		}},
	}
	created, err := service.Assign(ctx, peerKey, nil)
	if err != nil {
		t.Fatalf("Assign create error = %v", err)
	}
	service.ServerPublicKey = giznet.PublicKey{3}
	service.ServerEndpoint = "server-b:9820"
	resolved, err := service.Resolve(ctx, peerKey)
	if err != nil {
		t.Fatalf("Resolve refresh error = %v", err)
	}
	if resolved.Version != created.Version+1 || resolved.ServerPublicKey != service.ServerPublicKey.String() || resolved.ServerEndpoint != "server-b:9820" {
		t.Fatalf("resolved assignment = %+v, created = %+v", resolved, created)
	}

	service.ServerEndpoint = ""
	if _, err := service.Resolve(ctx, peerKey); !errors.Is(err, ErrMissingRoute) {
		t.Fatalf("Resolve missing route error = %v, want %v", err, ErrMissingRoute)
	}
}

func TestLookupNotFoundAndInvalidKey(t *testing.T) {
	service := &Server{Store: kv.NewMemory(nil)}
	if _, err := service.Lookup(context.Background(), giznet.PublicKey{1}); !errors.Is(err, ErrAssignmentNotFound) {
		t.Fatalf("Lookup missing error = %v, want %v", err, ErrAssignmentNotFound)
	}
	if _, err := service.Lookup(context.Background(), giznet.PublicKey{}); !errors.Is(err, ErrInvalidPublicKey) {
		t.Fatalf("Lookup zero key error = %v, want %v", err, ErrInvalidPublicKey)
	}
	if _, err := ParsePublicKey("bad"); !errors.Is(err, ErrInvalidPublicKey) {
		t.Fatalf("ParsePublicKey error = %v, want %v", err, ErrInvalidPublicKey)
	}
}
