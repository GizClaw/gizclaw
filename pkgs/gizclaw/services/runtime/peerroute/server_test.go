package peerroute

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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

func TestAssignDoesNotBlockIndependentPeer(t *testing.T) {
	first := giznet.PublicKey{1}
	second := giznet.PublicKey{2}
	base := kv.NewMemory(nil)
	store := &blockingPeerRouteStore{
		Store:   base,
		key:     assignmentKey(first.String()).String(),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	service := &Server{
		Store:           store,
		Peers:           testPeers{items: map[giznet.PublicKey]apitypes.Peer{first: activeClientPeer(), second: activeClientPeer()}},
		ServerPublicKey: giznet.PublicKey{9},
		ServerEndpoint:  "https://server.example",
	}

	type assignResult struct {
		assignment apitypes.PeerAssignment
		err        error
	}
	firstDone := make(chan assignResult, 1)
	go func() {
		assignment, err := service.Assign(t.Context(), first, nil)
		firstDone <- assignResult{assignment: assignment, err: err}
	}()
	<-store.entered

	sameDone := make(chan assignResult, 1)
	go func() {
		assignment, err := service.Assign(t.Context(), first, nil)
		sameDone <- assignResult{assignment: assignment, err: err}
	}()
	secondDone := make(chan assignResult, 1)
	go func() {
		assignment, err := service.Assign(t.Context(), second, nil)
		secondDone <- assignResult{assignment: assignment, err: err}
	}()

	select {
	case got := <-secondDone:
		if got.err != nil {
			t.Fatalf("independent Assign() error = %v", got.err)
		}
		if got.assignment.Version != 1 {
			t.Fatalf("independent assignment version = %d, want 1", got.assignment.Version)
		}
	case <-time.After(time.Second):
		close(store.release)
		t.Fatal("independent Peer could not complete Assign while first Peer Store.Get was blocked")
	}
	select {
	case got := <-sameDone:
		close(store.release)
		t.Fatalf("same Peer completed before first assignment release: %#v", got)
	default:
	}

	close(store.release)
	for name, done := range map[string]<-chan assignResult{"first": firstDone, "same": sameDone} {
		got := <-done
		if got.err != nil {
			t.Fatalf("%s Assign() error = %v", name, got.err)
		}
		if got.assignment.Version != 1 {
			t.Fatalf("%s assignment version = %d, want 1", name, got.assignment.Version)
		}
	}
}

func activeClientPeer() apitypes.Peer {
	return apitypes.Peer{Status: apitypes.PeerRegistrationStatusActive, Role: apitypes.PeerRoleClient}
}

type blockingPeerRouteStore struct {
	kv.Store
	key     string
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (store *blockingPeerRouteStore) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	if key.String() == store.key {
		store.once.Do(func() {
			close(store.entered)
			<-store.release
		})
	}
	return store.Store.Get(ctx, key)
}
