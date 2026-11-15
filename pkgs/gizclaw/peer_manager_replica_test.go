package gizclaw

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerpresence"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerroute"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/tests/testsupport/kvfixture"
)

type bootstrapReadView struct {
	kv.Store
	authority kv.Store
}

// CreateIfAbsent preserves the borrowed authority's atomic creation capability.
func (s bootstrapReadView) CreateIfAbsent(ctx context.Context, guard kv.Entry, entries []kv.Entry) ([]byte, bool, error) {
	return kv.CreateIfAbsent(ctx, s.authority, guard, entries)
}

func (s bootstrapReadView) CompareAndMutate(ctx context.Context, key kv.Key, expected []byte, entries []kv.Entry, deletes []kv.Key) (bool, error) {
	return kv.CompareAndMutate(ctx, s.authority, key, expected, entries, deletes)
}

type unavailableReconnectAuthority struct {
	kv.Store
	reads, writes atomic.Int32
}

func (s *unavailableReconnectAuthority) Get(context.Context, kv.Key) ([]byte, error) {
	s.reads.Add(1)
	return nil, errors.New("center unavailable")
}

func (s *unavailableReconnectAuthority) ApplyMutation(context.Context, kv.Mutation) (bool, error) {
	s.writes.Add(1)
	return false, errors.New("center unavailable")
}

func (s *unavailableReconnectAuthority) CreateIfAbsent(context.Context, kv.Entry, []kv.Entry) ([]byte, bool, error) {
	s.writes.Add(1)
	return nil, false, errors.New("center unavailable")
}

func (s *unavailableReconnectAuthority) CompareAndMutate(context.Context, kv.Key, []byte, []kv.Entry, []kv.Key) (bool, error) {
	s.writes.Add(1)
	return false, errors.New("center unavailable")
}

func TestWarmedPeerReconnectAndEdgeRoleReadsAvoidCenter(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	base := kv.NewMemory(nil)
	home, key, edge := giznet.PublicKey{111}, giznet.PublicKey{112}, giznet.PublicKey{113}
	peers := &peer.Server{Store: kv.Prefixed(base, kv.Key{"records"})}
	for _, record := range []apitypes.Peer{
		{PublicKey: key.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusActive},
		{PublicKey: edge.String(), Role: apitypes.PeerRoleEdgeNode, Status: apitypes.PeerRegistrationStatusActive},
	} {
		if _, err := peers.SavePeer(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	routes := &peerroute.Server{Store: kv.Prefixed(base, kv.Key{"routes"}), Peers: peers, ServerPublicKey: home, ServerEndpoint: "server-a:9820"}
	if _, err := routes.Assign(ctx, key, nil); err != nil {
		t.Fatal(err)
	}
	center := &unavailableReconnectAuthority{Store: base}
	var ordinary, primary atomic.Int32
	view := bootstrapReadView{Store: kvfixture.ReadView(center, base, &ordinary, &primary), authority: center}
	peers.Store = kv.Prefixed(view, kv.Key{"records"})
	routes.Store = kv.Prefixed(view, kv.Key{"routes"})
	manager := NewManager(peers)
	manager.PeerRoutes = routes
	service := &PeerService{manager: manager}
	policy := (*ServerSecurityPolicy)(&Server{manager: manager})
	for range 2 {
		for _, stream := range []uint64{ServiceEdgeHTTP, ServiceEdgeRPC} {
			if !policy.AllowService(edge, stream) {
				t.Fatalf("warmed Edge role denied service %x", stream)
			}
		}
		if !service.allowEdgeSignalingPeer(ctx, key) {
			t.Fatal("warmed Peer signaling denied")
		}
		if _, err := routes.Resolve(ctx, key); err != nil {
			t.Fatalf("warmed route resolution: %v", err)
		}
		conn := &testGiznetConn{publicKey: key}
		if _, err := manager.activatePeer(ctx, conn); err != nil {
			t.Fatalf("warmed Peer activation: %v", err)
		}
		manager.SetPeerDown(key, conn)
	}
	if center.reads.Load() != 0 || center.writes.Load() != 0 || primary.Load() != 0 || ordinary.Load() == 0 {
		t.Fatalf("warmed reconnect reached center: reads=%d writes=%d views=%d/%d", center.reads.Load(), center.writes.Load(), ordinary.Load(), primary.Load())
	}
}

type blockedReconnectAuthority struct {
	*unavailableReconnectAuthority
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockedReconnectAuthority) ApplyMutation(ctx context.Context, _ kv.Mutation) (bool, error) {
	s.writes.Add(1)
	s.once.Do(func() { close(s.entered) })
	select {
	case <-s.release:
		return false, errors.New("center unavailable")
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func TestWarmedPeerReconnectDoesNotWaitForPresenceWriter(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	base := kv.NewMemory(nil)
	home, key := giznet.PublicKey{114}, giznet.PublicKey{115}
	peers := &peer.Server{Store: base}
	if _, err := peers.EnsureConnectedPeer(ctx, key); err != nil {
		t.Fatal(err)
	}
	routes := &peerroute.Server{Store: base, Peers: peers, ServerPublicKey: home, ServerEndpoint: "server-a:9820"}
	if _, err := routes.Assign(ctx, key, nil); err != nil {
		t.Fatal(err)
	}
	center := &blockedReconnectAuthority{unavailableReconnectAuthority: &unavailableReconnectAuthority{Store: base}, entered: make(chan struct{}), release: make(chan struct{})}
	view := bootstrapReadView{Store: kvfixture.ReadView(center, base, nil, nil), authority: center}
	peers.Store, routes.Store = view, view
	manager := NewManager(peers)
	manager.PeerRoutes = routes
	manager.Presence = peerpresence.New(kv.Prefixed(view, kv.Key{"presence"}), home)
	if err := manager.Presence.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer func() { close(center.release); _ = manager.Presence.Close() }()
	first := &testGiznetConn{publicKey: key}
	if _, err := manager.activatePeer(ctx, first); err != nil {
		t.Fatal(err)
	}
	select {
	case <-center.entered:
	case <-ctx.Done():
		t.Fatal("presence writer did not reach blocked center")
	}
	manager.SetPeerDown(key, first)
	second := &testGiznetConn{publicKey: key}
	if _, err := manager.activatePeer(ctx, second); err != nil {
		t.Fatalf("reconnect waited for blocked presence writer: %v", err)
	}
	if current, online := manager.Peer(key); !online || current != second || center.reads.Load() != 0 {
		t.Fatalf("reconnect failed while center was blocked: online=%v reads=%d", online, center.reads.Load())
	}
}

func TestActivatingPeerRoutesKnownCreationMissingFromReplica(t *testing.T) {
	ctx := t.Context()
	authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	var ordinary, primary atomic.Int32
	view := bootstrapReadView{Store: kvfixture.ReadView(authority, replica, &ordinary, &primary), authority: authority}
	home, key := giznet.PublicKey{101}, giznet.PublicKey{102}
	peers := &peer.Server{Store: view}
	routes := &peerroute.Server{Store: view, Peers: peers, ServerPublicKey: home, ServerEndpoint: "server-a:9820"}
	state := &activePeer{}
	manager := &Manager{Peers: peers, PeerRoutes: routes, peers: map[giznet.PublicKey]*activePeer{key: state}}
	role, err := manager.ensureActivatingPeer(ctx, key, state, nil)
	if err != nil || role != apitypes.PeerRoleClient {
		t.Fatalf("known creation failed to bootstrap assignment: role=%s error=%v", role, err)
	}
	assignment, err := routes.Lookup(kv.WithPrimaryReads(ctx), key)
	if err != nil || assignment.ServerPublicKey != home.String() || assignment.Version != 1 {
		t.Fatalf("committed assignment = %+v, %v", assignment, err)
	}
	before := primary.Load()
	if _, err := peers.LoadPeer(ctx, key); !errors.Is(err, peer.ErrPeerNotFound) {
		t.Fatalf("ordinary view unexpectedly changed: %v", err)
	}
	if primary.Load() != before || ordinary.Load() == 0 {
		t.Fatalf("bootstrap pinned later reads: ordinary=%d primary=%d/%d", ordinary.Load(), before, primary.Load())
	}
}
