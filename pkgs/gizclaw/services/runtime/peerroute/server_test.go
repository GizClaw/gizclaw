package peerroute

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/tests/testsupport/kvfixture"
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

func (p testPeers) EnsureAvailable(ctx context.Context, key giznet.PublicKey) error {
	_, err := p.LoadPeer(ctx, key)
	return err
}

// staleRouteStore routes ordinary reads to an unchanged snapshot while conditional
// mutations execute against a real in-memory authority.
type staleRouteStore struct {
	kv.Store
	authority kv.Store
	ordinary  atomic.Int32
	primary   atomic.Int32
	attempts  atomic.Int32
	rejected  atomic.Int32
	apply     func(context.Context, kv.Mutation) (bool, error)
}

func newStaleRouteStore(t *testing.T, authority, replica kv.Store) *staleRouteStore {
	t.Helper()
	s := &staleRouteStore{authority: authority}
	s.Store = kvfixture.ReadView(authority, replica, &s.ordinary, &s.primary)
	return s
}
func (s *staleRouteStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	s.attempts.Add(1)
	var matched bool
	var err error
	if s.apply != nil {
		matched, err = s.apply(ctx, mutation)
	} else {
		matched, err = s.authority.ApplyMutation(ctx, mutation)
	}
	if !matched && err == nil {
		s.rejected.Add(1)
	}
	return matched, err
}
func (s *staleRouteStore) CreateIfAbsent(ctx context.Context, guard kv.Entry, entries []kv.Entry) ([]byte, bool, error) {
	return kv.CreateIfAbsent(ctx, s.authority, guard, entries)
}
func (s *staleRouteStore) CompareAndMutate(ctx context.Context, key kv.Key, expected []byte, entries []kv.Entry, deletes []kv.Key) (bool, error) {
	return s.ApplyMutation(ctx, kv.Mutation{Conditions: []kv.Condition{{Key: key, Expected: expected}}, Entries: entries, DeleteKeys: deletes})
}

func TestAssignmentReadsPreferReplicaAndRetryRejectedVersionAtPrimary(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		t.Run(map[bool]string{false: "newer-version", true: "foreign-home"}[foreign], func(t *testing.T) {
			ctx, key := t.Context(), giznet.PublicKey{93}
			authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
			assignment := apitypes.PeerAssignment{PeerPublicKey: key.String(), ServerPublicKey: giznet.PublicKey{8}.String(), ServerEndpoint: "old", Role: apitypes.PeerRoleClient, Version: 1, UpdatedAt: time.Now()}
			data, err := encodeAssignment(assignment)
			if err != nil {
				t.Fatal(err)
			}
			if err := replica.Set(ctx, assignmentKey(key.String()), data); err != nil {
				t.Fatal(err)
			}
			assignment.Version = 2
			if foreign {
				assignment.ServerPublicKey = giznet.PublicKey{9}.String()
			}
			data, err = encodeAssignment(assignment)
			if err != nil {
				t.Fatal(err)
			}
			if err := authority.Set(ctx, assignmentKey(key.String()), data); err != nil {
				t.Fatal(err)
			}
			store := newStaleRouteStore(t, authority, replica)
			service := &Server{Store: store, Peers: testPeers{items: map[giznet.PublicKey]apitypes.Peer{key: activeClientPeer()}}, ServerPublicKey: giznet.PublicKey{8}, ServerEndpoint: "new"}
			got, err := service.Lookup(ctx, key)
			if err != nil || got.Version != 1 || store.primary.Load() != 0 {
				t.Fatalf("ordinary lookup=%+v %v primary=%d", got, err, store.primary.Load())
			}
			version := int64(1)
			if _, err := service.Assign(ctx, key, &version); !errors.Is(err, ErrVersionConflict) {
				t.Fatalf("stale version accepted: %v", err)
			}
			if store.attempts.Load() != 1 || store.rejected.Load() != 1 || store.primary.Load() != 1 {
				t.Fatalf("fallback attempts=%d rejected=%d primary=%d", store.attempts.Load(), store.rejected.Load(), store.primary.Load())
			}
			after, err := authority.Get(ctx, assignmentKey(key.String()))
			if err != nil || !bytes.Equal(after, data) {
				t.Fatalf("newer assignment changed: %v", err)
			}
			if err := replica.Delete(ctx, assignmentKey(key.String())); err != nil {
				t.Fatal(err)
			}
			got, err = service.Lookup(ctx, key)
			if !errors.Is(err, ErrAssignmentNotFound) || store.primary.Load() != 1 {
				t.Fatalf("missing assignment was refreshed: %+v %v primary=%d", got, err, store.primary.Load())
			}
		})
	}
}

func TestAssignmentNoOpStillChecksExpectedVersion(t *testing.T) {
	ctx, key := t.Context(), giznet.PublicKey{95}
	authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	assignment := apitypes.PeerAssignment{PeerPublicKey: key.String(), ServerPublicKey: giznet.PublicKey{8}.String(), ServerEndpoint: "unchanged", Role: apitypes.PeerRoleClient, Version: 1, UpdatedAt: time.Now()}
	data, err := encodeAssignment(assignment)
	if err != nil {
		t.Fatal(err)
	}
	if err := replica.Set(ctx, assignmentKey(key.String()), data); err != nil {
		t.Fatal(err)
	}
	assignment.Version = 2
	data, err = encodeAssignment(assignment)
	if err != nil {
		t.Fatal(err)
	}
	if err := authority.Set(ctx, assignmentKey(key.String()), data); err != nil {
		t.Fatal(err)
	}
	store := newStaleRouteStore(t, authority, replica)
	service := &Server{Store: store, Peers: testPeers{items: map[giznet.PublicKey]apitypes.Peer{key: activeClientPeer()}}, ServerPublicKey: giznet.PublicKey{8}, ServerEndpoint: "unchanged"}
	oldVersion := int64(1)
	if _, err := service.Assign(ctx, key, &oldVersion); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale no-op accepted obsolete version: %v", err)
	}
	if store.rejected.Load() != 1 || store.primary.Load() != 1 {
		t.Fatalf("no-op conflict did not refresh authority: rejected=%d primary=%d", store.rejected.Load(), store.primary.Load())
	}
	got, err := service.Assign(ctx, key, &assignment.Version)
	if err != nil || got.Version != assignment.Version || !got.UpdatedAt.Equal(assignment.UpdatedAt) {
		t.Fatalf("fresh no-op changed metadata or rejected committed version: %+v %v", got, err)
	}
	if store.ordinary.Load() != 2 || store.primary.Load() != 2 || store.attempts.Load() != 2 {
		t.Fatalf("version fallback was not scoped: reads=%d/%d attempts=%d", store.ordinary.Load(), store.primary.Load(), store.attempts.Load())
	}
	after, err := authority.Get(ctx, assignmentKey(key.String()))
	if err != nil || !bytes.Equal(after, data) {
		t.Fatalf("no-op overwrote assignment: %v", err)
	}
}

func TestAssignmentConflictRetryIsBounded(t *testing.T) {
	ctx, key := t.Context(), giznet.PublicKey{94}
	authority := kv.NewMemory(nil)
	service := &Server{Store: authority, Peers: testPeers{items: map[giznet.PublicKey]apitypes.Peer{key: activeClientPeer()}}, ServerPublicKey: giznet.PublicKey{8}, ServerEndpoint: "old"}
	initial, err := service.Assign(ctx, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	store := newStaleRouteStore(t, authority, authority)
	store.apply = func(context.Context, kv.Mutation) (bool, error) { return false, nil }
	service.Store, service.ServerEndpoint = store, "new"
	if _, err := service.Assign(ctx, key, &initial.Version); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("retry error=%v", err)
	}
	if store.attempts.Load() != 8 {
		t.Fatalf("unbounded retry: %d", store.attempts.Load())
	}
}

type unavailableRouteWriter struct {
	kv.Store
	writes atomic.Int32
}

type routeReadErrorStore struct {
	kv.Store
	reads atomic.Int32
}

func (s *routeReadErrorStore) Get(context.Context, kv.Key) ([]byte, error) {
	s.reads.Add(1)
	return nil, errors.New("authority unavailable")
}

func (s *unavailableRouteWriter) CreateIfAbsent(context.Context, kv.Entry, []kv.Entry) ([]byte, bool, error) {
	s.writes.Add(1)
	return nil, false, errors.New("authority unavailable")
}

func (s *unavailableRouteWriter) CompareAndMutate(context.Context, kv.Key, []byte, []kv.Entry, []kv.Key) (bool, error) {
	s.writes.Add(1)
	return false, errors.New("authority unavailable")
}

func TestUnchangedAssignmentSurvivesWriterOutage(t *testing.T) {
	ctx, key := t.Context(), giznet.PublicKey{96}
	home := giznet.PublicKey{8}
	replica := kv.NewMemory(nil)
	assignment := apitypes.PeerAssignment{PeerPublicKey: key.String(), ServerPublicKey: home.String(), ServerEndpoint: "unchanged", Role: apitypes.PeerRoleClient, Version: 1, UpdatedAt: time.Now()}
	data, err := encodeAssignment(assignment)
	if err != nil {
		t.Fatal(err)
	}
	if err := replica.Set(ctx, assignmentKey(key.String()), data); err != nil {
		t.Fatal(err)
	}
	authority := &routeReadErrorStore{Store: replica}
	store := &unavailableRouteWriter{Store: kvfixture.ReadView(authority, replica, nil, nil)}
	service := &Server{Store: store, Peers: testPeers{items: map[giznet.PublicKey]apitypes.Peer{key: activeClientPeer()}}, ServerPublicKey: home, ServerEndpoint: "unchanged"}
	got, err := service.Assign(ctx, key, nil)
	if err != nil || got.Version != assignment.Version || !got.UpdatedAt.Equal(assignment.UpdatedAt) || store.writes.Load() != 0 || authority.reads.Load() != 0 {
		t.Fatalf("unchanged assignment: %+v %v writes=%d primary=%d", got, err, store.writes.Load(), authority.reads.Load())
	}
}

func TestRouteReadFailuresDoNotRefreshPrimary(t *testing.T) {
	ctx, key := t.Context(), giznet.PublicKey{97}
	for _, primary := range []bool{false, true} {
		ctx := ctx
		if primary {
			ctx = kv.WithPrimaryReads(ctx)
		}
		replica := kv.NewMemory(nil)
		authority := &routeReadErrorStore{Store: replica}
		var ordinaryReads, primaryReads atomic.Int32
		service := &Server{Store: kvfixture.ReadView(authority, replica, &ordinaryReads, &primaryReads), Peers: testPeers{items: map[giznet.PublicKey]apitypes.Peer{key: activeClientPeer()}}, ServerPublicKey: giznet.PublicKey{8}, ServerEndpoint: "home"}
		for _, operation := range []func(context.Context, giznet.PublicKey) (apitypes.PeerAssignment, error){service.Lookup, service.Resolve} {
			ordinaryReads.Store(0)
			primaryReads.Store(0)
			_, err := operation(ctx, key)
			if !primary && !errors.Is(err, ErrAssignmentNotFound) || primary && err == nil {
				t.Fatalf("read error=%v (primary=%v)", err, primary)
			}
			wantOrdinary, wantPrimary := int32(1), int32(0)
			if primary {
				wantOrdinary, wantPrimary = 0, 1
			}
			if ordinaryReads.Load() != wantOrdinary || primaryReads.Load() != wantPrimary {
				t.Fatalf("route read retried: %d/%d", ordinaryReads.Load(), primaryReads.Load())
			}
		}
	}
}

func TestAssignmentMissingReaderStillCreatesAndRefreshesAtomically(t *testing.T) {
	ctx, key := t.Context(), giznet.PublicKey{98}
	authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	store := newStaleRouteStore(t, authority, replica)
	service := &Server{Store: store, Peers: testPeers{items: map[giznet.PublicKey]apitypes.Peer{key: activeClientPeer()}}, ServerPublicKey: giznet.PublicKey{8}, ServerEndpoint: "old"}
	first, err := service.Assign(ctx, key, nil)
	if err != nil || first.Version != 1 {
		t.Fatalf("claim=%+v %v", first, err)
	}
	service.ServerEndpoint = "new"
	second, err := service.Assign(ctx, key, nil)
	if err != nil || second.Version != 2 || second.ServerEndpoint != "new" || store.attempts.Load() != 1 {
		t.Fatalf("refresh=%+v %v conditional writes=%d", second, err, store.attempts.Load())
	}
	before, err := authority.Get(ctx, assignmentKey(key.String()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Assign(ctx, key, &first.Version); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("obsolete version after missing reader accepted: %v", err)
	}
	after, err := authority.Get(ctx, assignmentKey(key.String()))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("obsolete version modified assignment: %v", err)
	}
}
