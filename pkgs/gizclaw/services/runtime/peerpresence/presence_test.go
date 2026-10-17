package peerpresence

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func startService(t *testing.T, store kv.Store, server byte) *Service {
	t.Helper()
	s := New(store, giznet.PublicKey{server})
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func awaitSnapshot(t *testing.T, s *Service, peer giznet.PublicKey, want Snapshot) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		got, err := s.Read(t.Context(), peer)
		if err != nil {
			t.Fatal(err)
		}
		if got.Online == want.Online && got.WorkspaceName == want.WorkspaceName && got.LastSeenAt.Equal(want.LastSeenAt) {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("presence = %+v, want %+v", got, want)
		case <-ticker.C:
		}
	}
}

func TestSharedTransitionsAndExpiredLease(t *testing.T) {
	store := kv.NewMemory(nil)
	a, b := startService(t, store, 1), startService(t, store, 2)
	peer := giznet.PublicKey{3}
	want := Snapshot{Online: true, LastSeenAt: time.Now().UTC(), WorkspaceName: "room"}
	a.Publish(peer, want)
	awaitSnapshot(t, b, peer, want)
	want.WorkspaceName = ""
	a.Publish(peer, want)
	awaitSnapshot(t, b, peer, want)
	want.WorkspaceName = "another-room"
	a.Publish(peer, want)
	awaitSnapshot(t, b, peer, want)
	// A lease may still be returned by a lagging replica after its wall-clock
	// deadline. A read must reject it without refreshing any Peer record.
	a.cancel()
	value, _ := json.Marshal(lease{Generation: a.generation, ExpiresAt: time.Now().Add(-time.Second)})
	if err := store.Set(t.Context(), leaseKey(a.server), value); err != nil {
		t.Fatal(err)
	}
	want.Online, want.WorkspaceName = false, ""
	awaitSnapshot(t, b, peer, want)
}

func TestRestartFencesOldPublisherAndClose(t *testing.T) {
	store := kv.NewMemory(nil)
	old := startService(t, store, 1)
	peer := giznet.PublicKey{2}
	want := Snapshot{Online: true, LastSeenAt: time.Now().UTC(), WorkspaceName: "old"}
	old.Publish(peer, want)
	awaitSnapshot(t, old, peer, want)
	next := startService(t, store, 1)
	offline := want
	offline.Online, offline.WorkspaceName = false, ""
	awaitSnapshot(t, next, peer, offline)
	want.WorkspaceName = "new"
	next.Publish(peer, want)
	awaitSnapshot(t, next, peer, want)
	old.Publish(peer, offline)
	select {
	case <-old.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("old generation was not fenced")
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	awaitSnapshot(t, next, peer, want)
	if err := next.Start(t.Context()); err == nil {
		t.Fatal("duplicate Start succeeded")
	}
	awaitSnapshot(t, next, peer, want)
}

type blockingStore struct {
	kv.Store
	peer    giznet.PublicKey
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockingStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	if len(mutation.Entries) == 1 && mutation.Entries[0].Key.String() == peerKey(s.peer).String() {
		s.once.Do(func() {
			close(s.entered)
			select {
			case <-s.release:
			case <-ctx.Done():
			}
		})
	}
	return s.Store.ApplyMutation(ctx, mutation)
}

func TestTransitionsCoalesceWithoutBlockingOtherPeers(t *testing.T) {
	peer := giznet.PublicKey{2}
	store := &blockingStore{Store: kv.NewMemory(nil), peer: peer, entered: make(chan struct{}), release: make(chan struct{})}
	s := startService(t, store, 1)
	want := Snapshot{Online: true, LastSeenAt: time.Now().UTC(), WorkspaceName: "old"}
	s.Publish(peer, want)
	select {
	case <-store.entered:
	case <-time.After(time.Second):
		t.Fatal("writer did not start")
	}
	want.WorkspaceName = "next"
	s.Publish(peer, want)
	want.Online, want.WorkspaceName = false, ""
	s.Publish(peer, want)
	other := giznet.PublicKey{3}
	otherState := Snapshot{Online: true, LastSeenAt: want.LastSeenAt}
	s.Publish(other, otherState)
	awaitSnapshot(t, s, other, otherState)
	close(store.release)
	awaitSnapshot(t, s, peer, want)
	// A replacement after the offline writer retires must publish normally.
	want.Online, want.WorkspaceName = true, "replacement"
	s.Publish(peer, want)
	awaitSnapshot(t, s, peer, want)
}

func TestCloseCancelsBlockedWriter(t *testing.T) {
	peer := giznet.PublicKey{2}
	store := &blockingStore{Store: kv.NewMemory(nil), peer: peer, entered: make(chan struct{}), release: make(chan struct{})}
	s := startService(t, store, 1)
	s.Publish(peer, Snapshot{Online: true})
	<-store.entered
	done := make(chan error, 1)
	go func() { done <- s.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel the writer")
	}
}

func TestParentCancellationStopsBlockedWriter(t *testing.T) {
	peer := giznet.PublicKey{2}
	store := &blockingStore{Store: kv.NewMemory(nil), peer: peer, entered: make(chan struct{}), release: make(chan struct{})}
	s := New(store, giznet.PublicKey{1})
	t.Cleanup(func() { _ = s.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	s.Publish(peer, Snapshot{Online: true})
	<-store.entered
	cancel()
	done := make(chan struct{})
	go func() { s.workers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("parent cancellation did not stop presence workers")
	}
}

func TestReadRejectsCorruptionAndReturnsStorageErrors(t *testing.T) {
	store := kv.NewMemory(nil)
	s := startService(t, store, 1)
	peer := giznet.PublicKey{2}
	for _, value := range []string{"{", `{"online":true,"server":"bad:server","generation":"x"}`} {
		if err := store.Set(t.Context(), peerKey(peer), []byte(value)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Read(t.Context(), peer); err == nil {
			t.Fatalf("corrupt record %s was accepted", value)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Read(ctx, peer); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled read = %v", err)
	}
}
