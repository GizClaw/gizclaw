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
	version := existing.Version
	refreshed, err := service.Assign(ctx, peerKey, &version)
	if err != nil {
		t.Fatalf("Assign refresh error = %v", err)
	}
	if refreshed.Version != 2 || refreshed.ServerPublicKey != otherServerKey.String() || refreshed.ServerEndpoint != "server-b:9820" {
		t.Fatalf("refreshed assignment = %+v", refreshed)
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
