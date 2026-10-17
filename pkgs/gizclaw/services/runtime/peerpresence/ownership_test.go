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

type mutationHookStore struct {
	kv.Store
	apply func(context.Context, kv.Mutation) (bool, error)
}

func (s mutationHookStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	return s.apply(ctx, mutation)
}

func awaitWriter(t *testing.T, s *Service, peer giznet.PublicKey) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		s.mu.Lock()
		p := s.pending[peer]
		idle := p == nil || !p.running
		s.mu.Unlock()
		if idle {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("presence writer did not finish")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestCrossServerHandoffFencesInFlightAndLaterTransitions(t *testing.T) {
	base := kv.NewMemory(nil)
	peer := giznet.PublicKey{3}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	store := mutationHookStore{Store: base, apply: func(ctx context.Context, mutation kv.Mutation) (bool, error) {
		if len(mutation.Entries) > 0 && mutation.Entries[0].Key.String() == peerKey(peer).String() {
			var value record
			if err := json.Unmarshal(mutation.Entries[0].Value, &value); err != nil {
				return false, err
			}
			if value.WorkspaceName == "late-room" {
				once.Do(func() {
					close(entered)
					select {
					case <-release:
					case <-ctx.Done():
					}
				})
			}
		}
		return base.ApplyMutation(ctx, mutation)
	}}
	a, b := startService(t, store, 1), startService(t, base, 2)
	aPublisher, bPublisher := a.NewPublisher(peer), b.NewPublisher(peer)
	want := Snapshot{Online: true, LastSeenAt: time.Now().UTC(), WorkspaceName: "old-room"}
	aPublisher.Publish(want)
	awaitSnapshot(t, b, peer, want)
	want.WorkspaceName = "late-room"
	aPublisher.Publish(want)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("old connection update did not reach the write authority")
	}
	want.WorkspaceName = "new-room"
	bPublisher.Publish(want)
	awaitSnapshot(t, b, peer, want)
	close(release)
	awaitWriter(t, a, peer)
	awaitSnapshot(t, b, peer, want)
	aPublisher.Publish(Snapshot{LastSeenAt: want.LastSeenAt.Add(time.Second)})
	awaitWriter(t, a, peer)
	awaitSnapshot(t, b, peer, want)
	// A temporary offline transition can retire the worker, but a later
	// rollback on the same connection must retain its fenced ownership.
	aPublisher.Publish(want)
	awaitWriter(t, a, peer)
	awaitSnapshot(t, b, peer, want)
	// Losing one Peer must not cancel the Server's other publishers.
	other := giznet.PublicKey{4}
	a.NewPublisher(other).Publish(want)
	awaitSnapshot(t, b, other, want)
	// A real reconnect can claim ownership again, even with the same snapshot.
	reconnected := a.NewPublisher(peer)
	reconnected.Publish(want)
	awaitWriter(t, a, peer)
	bPublisher.Publish(Snapshot{LastSeenAt: want.LastSeenAt})
	awaitWriter(t, b, peer)
	awaitSnapshot(t, a, peer, want)
	reconnected.Publish(Snapshot{LastSeenAt: want.LastSeenAt})
	awaitSnapshot(t, b, peer, Snapshot{LastSeenAt: want.LastSeenAt})
}

func TestOfflinePublisherRetainsItsOwnershipCondition(t *testing.T) {
	base := kv.NewMemory(nil)
	a, b := startService(t, base, 1), startService(t, base, 2)
	peer := giznet.PublicKey{3}
	publisher := a.NewPublisher(peer)
	want := Snapshot{Online: true, LastSeenAt: time.Now().UTC(), WorkspaceName: "room"}
	publisher.Publish(want)
	awaitSnapshot(t, b, peer, want)
	publisher.Publish(Snapshot{LastSeenAt: want.LastSeenAt})
	awaitSnapshot(t, b, peer, Snapshot{LastSeenAt: want.LastSeenAt})
	awaitWriter(t, a, peer)
	b.NewPublisher(peer).Publish(want)
	awaitWriter(t, b, peer)
	// Deletion rollback resumes the existing Publisher, not a new claim.
	publisher.Publish(Snapshot{Online: true, LastSeenAt: want.LastSeenAt, WorkspaceName: "old-room"})
	awaitWriter(t, a, peer)
	awaitSnapshot(t, b, peer, want)
}

func TestLostClaimResponseCannotReclaimAnotherServer(t *testing.T) {
	for _, test := range []struct {
		name       string
		handoff    bool
		disconnect bool
	}{
		{name: "retry-own-claim"},
		{name: "disconnect-own-claim", disconnect: true},
		{name: "handoff-online", handoff: true},
		{name: "handoff-offline", handoff: true, disconnect: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := kv.NewMemory(nil)
			peer := giznet.PublicKey{3}
			committed, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			store := mutationHookStore{Store: base, apply: func(ctx context.Context, mutation kv.Mutation) (bool, error) {
				matched, err := base.ApplyMutation(ctx, mutation)
				if len(mutation.Entries) > 1 && mutation.Entries[0].Key.String() == peerKey(peer).String() && matched && err == nil {
					once.Do(func() {
						close(committed)
						select {
						case <-release:
						case <-ctx.Done():
						}
						err = errors.New("claim response lost")
					})
				}
				return matched, err
			}}
			a, b := startService(t, store, 1), startService(t, base, 2)
			aPublisher, bPublisher := a.NewPublisher(peer), b.NewPublisher(peer)
			want := Snapshot{Online: true, LastSeenAt: time.Now().UTC(), WorkspaceName: "room"}
			aPublisher.Publish(want)
			select {
			case <-committed:
			case <-time.After(time.Second):
				t.Fatal("claim did not commit")
			}
			if test.handoff {
				bPublisher.Publish(want)
				awaitWriter(t, b, peer)
			}
			if test.disconnect {
				aPublisher.Publish(Snapshot{LastSeenAt: want.LastSeenAt})
			}
			close(release)
			awaitWriter(t, a, peer)
			if !test.handoff && test.disconnect {
				want.Online, want.WorkspaceName = false, ""
			}
			awaitSnapshot(t, b, peer, want)
		})
	}
}
