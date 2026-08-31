package peerroute

import (
	"bytes"
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

func BenchmarkAssignResourceContention(b *testing.B) {
	const (
		width = 8
		delay = time.Millisecond
	)
	peers := make(map[giznet.PublicKey]apitypes.Peer, width)
	keys := make([]giznet.PublicKey, width)
	for index := range width {
		keys[index] = giznet.PublicKey{byte(index + 1)}
		peers[keys[index]] = activeClientPeer()
	}
	service := &Server{
		Store:           &delayedPeerRouteStore{Store: kv.NewMemory(nil), delay: delay},
		Peers:           testPeers{items: peers},
		ServerPublicKey: giznet.PublicKey{99},
		ServerEndpoint:  "https://server.example",
	}
	for _, benchmark := range []struct {
		name     string
		parallel bool
		samePeer bool
		global   bool
	}{
		{name: "serial-distinct-8"},
		{name: "parallel-global-distinct-8", parallel: true, global: true},
		{name: "parallel-same-8", parallel: true, samePeer: true},
		{name: "parallel-distinct-8", parallel: true},
	} {
		b.Run(benchmark.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ReportMetric(float64(delay), "store-delay-ns")
			for b.Loop() {
				if err := assignPeerBatch(b.Context(), service, keys, benchmark.parallel, benchmark.samePeer, benchmark.global); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*width), "ns/peer")
		})
	}
}

func assignPeerBatch(ctx context.Context, service *Server, keys []giznet.PublicKey, parallel, samePeer, global bool) error {
	var globalLock sync.Mutex
	assign := func(index int) error {
		key := keys[index]
		if samePeer {
			key = keys[0]
		}
		if global {
			globalLock.Lock()
			defer globalLock.Unlock()
		}
		_, err := service.Assign(ctx, key, nil)
		return err
	}
	if !parallel {
		for index := range keys {
			if err := assign(index); err != nil {
				return err
			}
		}
		return nil
	}
	errs := make([]error, len(keys))
	var wait sync.WaitGroup
	for index := range keys {
		wait.Go(func() { errs[index] = assign(index) })
	}
	wait.Wait()
	return errors.Join(errs...)
}

type delayedPeerRouteStore struct {
	kv.Store
	delay time.Duration
}

func (store *delayedPeerRouteStore) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	timer := time.NewTimer(store.delay)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return store.Store.Get(ctx, key)
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

func TestAssignRaceProducesOnePermanentOwner(t *testing.T) {
	peerKey := giznet.PublicKey{1}
	store := kv.NewMemory(nil)
	peers := testPeers{items: map[giznet.PublicKey]apitypes.Peer{peerKey: activeClientPeer()}}
	servers := []*Server{
		{Store: store, Peers: peers, ServerPublicKey: giznet.PublicKey{8}, ServerEndpoint: "server-a:9820"},
		{Store: store, Peers: peers, ServerPublicKey: giznet.PublicKey{9}, ServerEndpoint: "server-b:9820"},
	}
	errs := make([]error, len(servers))
	assignments := make([]apitypes.PeerAssignment, len(servers))
	var wait sync.WaitGroup
	for index := range servers {
		wait.Go(func() { assignments[index], errs[index] = servers[index].Assign(t.Context(), peerKey, nil) })
	}
	wait.Wait()
	winners := 0
	conflicts := 0
	for _, err := range errs {
		switch {
		case err == nil:
			winners++
		case errors.Is(err, ErrVersionConflict):
			conflicts++
		default:
			t.Fatalf("Assign() unexpected error = %v", err)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("Assign() outcomes = %v, want one winner and one conflict", errs)
	}
	stored, err := servers[0].Lookup(t.Context(), peerKey)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Version != 1 {
		t.Fatalf("stored assignment = %+v, want immutable version 1", stored)
	}
	for index, assignment := range assignments {
		if errs[index] == nil && assignment.ServerPublicKey != stored.ServerPublicKey {
			t.Fatalf("winner = %+v, stored = %+v", assignment, stored)
		}
	}
}

func TestResolveIsReadOnly(t *testing.T) {
	peerKey := giznet.PublicKey{1}
	base := kv.NewMemory(nil)
	store := &countingPeerRouteStore{Store: base}
	service := &Server{
		Store: store, Peers: testPeers{items: map[giznet.PublicKey]apitypes.Peer{peerKey: activeClientPeer()}},
		ServerPublicKey: giznet.PublicKey{8}, ServerEndpoint: "server-a:9820",
	}
	written := apitypes.PeerAssignment{
		PeerPublicKey: peerKey.String(), ServerPublicKey: giznet.PublicKey{9}.String(),
		ServerEndpoint: "server-b:9820", Role: apitypes.PeerRoleClient, Version: 7, UpdatedAt: time.Now(),
	}
	if err := service.put(t.Context(), written); err != nil {
		t.Fatal(err)
	}
	store.writes = 0
	resolved, err := service.Resolve(t.Context(), peerKey)
	if err != nil {
		t.Fatal(err)
	}
	if store.writes != 0 || resolved.ServerPublicKey != written.ServerPublicKey || resolved.Version != written.Version {
		t.Fatalf("Resolve() = %+v with %d writes, want unchanged assignment", resolved, store.writes)
	}
}

func TestAssignRefreshesOnlySameOwnerAndPreservesForeignBytes(t *testing.T) {
	peerKey := giznet.PublicKey{1}
	store := kv.NewMemory(nil)
	peers := testPeers{items: map[giznet.PublicKey]apitypes.Peer{peerKey: activeClientPeer()}}
	owner := &Server{Store: store, Peers: peers, ServerPublicKey: giznet.PublicKey{8}, ServerEndpoint: "server-a:9820"}
	first, err := owner.Assign(t.Context(), peerKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Get(t.Context(), assignmentKey(peerKey.String()))
	if err != nil {
		t.Fatal(err)
	}
	foreign := &Server{Store: store, Peers: peers, ServerPublicKey: giznet.PublicKey{9}, ServerEndpoint: "server-b:9820"}
	if _, err := foreign.Assign(t.Context(), peerKey, nil); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("foreign Assign() error = %v, want conflict", err)
	}
	after, err := store.Get(t.Context(), assignmentKey(peerKey.String()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("foreign Assign() changed persisted assignment bytes")
	}
	owner.ServerEndpoint = "server-a-new:9820"
	refreshed, err := owner.Assign(t.Context(), peerKey, &first.Version)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.ServerPublicKey != first.ServerPublicKey || refreshed.ServerEndpoint != owner.ServerEndpoint || refreshed.Version != first.Version+1 {
		t.Fatalf("refreshed assignment = %+v", refreshed)
	}
	missingVersion := int64(1)
	missingKey := giznet.PublicKey{2}
	owner.Peers = testPeers{items: map[giznet.PublicKey]apitypes.Peer{missingKey: activeClientPeer()}}
	if _, err := owner.Assign(t.Context(), missingKey, &missingVersion); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("versioned missing Assign() error = %v, want conflict", err)
	}
}

type countingPeerRouteStore struct {
	kv.Store
	writes int
}

func (s *countingPeerRouteStore) Set(ctx context.Context, key kv.Key, value []byte) error {
	s.writes++
	return s.Store.Set(ctx, key, value)
}

func (s *countingPeerRouteStore) BatchSet(ctx context.Context, entries []kv.Entry) error {
	s.writes++
	return s.Store.BatchSet(ctx, entries)
}

func (s *countingPeerRouteStore) BatchMutate(ctx context.Context, entries []kv.Entry, keys []kv.Key) error {
	s.writes++
	return s.Store.BatchMutate(ctx, entries, keys)
}

func activeClientPeer() apitypes.Peer {
	return apitypes.Peer{Status: apitypes.PeerRegistrationStatusActive, Role: apitypes.PeerRoleClient}
}
