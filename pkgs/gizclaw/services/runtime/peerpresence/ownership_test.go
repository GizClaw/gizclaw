package peerpresence

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/tests/testsupport/kvfixture"
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

// stalePresenceStore routes ordinary reads to an unchanged snapshot while conditional
// mutations execute against a real in-memory authority.
type stalePresenceStore struct {
	kv.Store
	authority kv.Store
	ordinary  atomic.Int32
	primary   atomic.Int32
	attempts  atomic.Int32
	rejected  atomic.Int32
	apply     func(context.Context, kv.Mutation) (bool, error)
}

func newStalePresenceStore(t *testing.T, authority, replica kv.Store) *stalePresenceStore {
	t.Helper()
	s := &stalePresenceStore{authority: authority}
	s.Store = kvfixture.ReadView(authority, replica, &s.ordinary, &s.primary)
	return s
}
func (s *stalePresenceStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
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
func (s *stalePresenceStore) BatchSet(ctx context.Context, entries []kv.Entry) error {
	return s.authority.BatchSet(ctx, entries)
}

func TestFirstClaimUsesOrdinaryOwnerThenPrimaryAfterRejection(t *testing.T) {
	ctx, peer := t.Context(), giznet.PublicKey{95}
	authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	previous := []byte("previous-connection")
	if err := authority.Set(ctx, ownerKey(peer), previous); err != nil {
		t.Fatal(err)
	}
	store := newStalePresenceStore(t, authority, replica)
	service := startService(t, store, 1)
	publisher := service.NewPublisher(peer)
	store.apply = func(ctx context.Context, mutation kv.Mutation) (bool, error) {
		if len(mutation.Entries) > 0 && mutation.Entries[0].Key.String() == peerKey(peer).String() && store.attempts.Load() == 2 && (store.ordinary.Load() != 1 || store.primary.Load() != 0 || mutation.Conditions[1].Expected != nil) {
			t.Errorf("first claim bypassed reader: reads=%d/%d expected=%q", store.ordinary.Load(), store.primary.Load(), mutation.Conditions[1].Expected)
		}
		return authority.ApplyMutation(ctx, mutation)
	}
	want := Snapshot{Online: true, LastSeenAt: time.Now().UTC(), WorkspaceName: "room"}
	if err := service.writeTransition(ctx, publisher, want); err != nil {
		t.Fatal(err)
	}
	if !publisher.ownership.claimed || publisher.ownership.fenced || store.attempts.Load() != 3 || store.rejected.Load() != 1 || store.primary.Load() != 1 {
		t.Fatalf("claim fallback: owner=%+v attempts=%d rejected=%d primary=%d", publisher.ownership, store.attempts.Load(), store.rejected.Load(), store.primary.Load())
	}
	value, err := authority.Get(ctx, peerKey(peer))
	if err != nil {
		t.Fatal(err)
	}
	var got record
	if err := json.Unmarshal(value, &got); err != nil || got.Snapshot != want {
		t.Fatalf("committed snapshot=%+v %v", got, err)
	}
}

func TestStaleOwnerDoesNotRebaseUncertainClaim(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "uncommitted", true: "committed"}[committed], func(t *testing.T) {
			ctx, peer := t.Context(), giznet.PublicKey{96}
			authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
			previous := []byte("original-owner")
			for _, store := range []kv.Store{authority, replica} {
				if err := store.Set(ctx, ownerKey(peer), previous); err != nil {
					t.Fatal(err)
				}
			}
			store := newStalePresenceStore(t, authority, replica)
			service := startService(t, store, 1)
			publisher := service.NewPublisher(peer)
			lost := errors.New("claim result uncertain")
			store.apply = func(ctx context.Context, mutation kv.Mutation) (bool, error) {
				if store.attempts.Load() == 2 {
					if committed {
						if _, err := authority.ApplyMutation(ctx, mutation); err != nil {
							return false, err
						}
					}
					return false, lost
				}
				return authority.ApplyMutation(ctx, mutation)
			}
			want := Snapshot{Online: true, LastSeenAt: time.Now().UTC()}
			if err := service.writeTransition(ctx, publisher, want); !errors.Is(err, lost) {
				t.Fatalf("uncertain error=%v", err)
			}
			if store.attempts.Load() != 2 || store.primary.Load() != 0 {
				t.Fatalf("uncertain claim replayed: %d/%d", store.attempts.Load(), store.primary.Load())
			}
			foreign := []byte("newer-owner")
			if err := authority.Set(ctx, ownerKey(peer), foreign); err != nil {
				t.Fatal(err)
			}
			if err := service.writeTransition(ctx, publisher, want); err != nil {
				t.Fatal(err)
			}
			current, err := authority.Get(ctx, ownerKey(peer))
			if err != nil || string(current) != string(foreign) || !publisher.ownership.fenced || string(publisher.ownership.expected) != string(previous) || store.attempts.Load() != 3 {
				t.Fatalf("uncertain claim reclaimed: current=%q owner=%+v attempts=%d err=%v", current, publisher.ownership, store.attempts.Load(), err)
			}
		})
	}
}

func TestOfflineLostClaimResponseRechecksStaleOwner(t *testing.T) {
	ctx, peer := t.Context(), giznet.PublicKey{97}
	authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	store := newStalePresenceStore(t, authority, replica)
	service := startService(t, store, 1)
	publisher := service.NewPublisher(peer)
	lost := errors.New("claim response lost")
	store.apply = func(ctx context.Context, mutation kv.Mutation) (bool, error) {
		matched, err := authority.ApplyMutation(ctx, mutation)
		if store.attempts.Load() == 2 && err == nil && matched {
			return false, lost
		}
		return matched, err
	}
	if err := service.writeTransition(ctx, publisher, Snapshot{Online: true}); !errors.Is(err, lost) {
		t.Fatalf("claim error=%v", err)
	}
	if err := service.writeTransition(ctx, publisher, Snapshot{}); err != nil {
		t.Fatal(err)
	}
	value, err := authority.Get(ctx, peerKey(peer))
	if err != nil {
		t.Fatal(err)
	}
	var got record
	if err := json.Unmarshal(value, &got); err != nil || got.Online || store.ordinary.Load() != 2 || store.primary.Load() != 1 {
		t.Fatalf("offline recovery=%+v err=%v reads=%d/%d", got, err, store.ordinary.Load(), store.primary.Load())
	}
}

type presenceReadHookStore struct {
	kv.Store
	get      func(context.Context, kv.Key) ([]byte, error)
	batchGet func(context.Context, []kv.Key) []kv.GetResult
}

func (s presenceReadHookStore) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	if s.get != nil {
		return s.get(ctx, key)
	}
	return s.Store.Get(ctx, key)
}

func (s presenceReadHookStore) BatchGet(ctx context.Context, keys []kv.Key) []kv.GetResult {
	if s.batchGet != nil {
		return s.batchGet(ctx, keys)
	}
	return s.Store.BatchGet(ctx, keys)
}

func TestAdminFirstClaimAcrossDifferentServersCannotHideActiveReplacement(t *testing.T) {
	for _, stall := range []string{"owner-read", "first-publication"} {
		t.Run(stall, func(t *testing.T) {
			base, peer := kv.NewMemory(nil), giznet.PublicKey{98}
			// Active Admin identities may connect to different Server keys;
			// Client fixed-home routing does not serialize these publishers.
			if err := base.Set(t.Context(), ownerKey(peer), []byte("previous-owner")); err != nil {
				t.Fatal(err)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			var blocked, released sync.Once
			unblock := func() { released.Do(func() { close(release) }) }
			defer unblock()
			wait := func(ctx context.Context) {
				blocked.Do(func() {
					close(entered)
					select {
					case <-release:
					case <-ctx.Done():
					}
				})
			}
			var store kv.Store = base
			if stall == "owner-read" {
				store = presenceReadHookStore{Store: base, get: func(ctx context.Context, key kv.Key) ([]byte, error) {
					value, err := base.Get(ctx, key)
					if key.String() == ownerKey(peer).String() {
						wait(ctx) // Retain the pre-handoff observation while stalled.
					}
					return value, err
				}}
			} else {
				store = mutationHookStore{Store: base, apply: func(ctx context.Context, mutation kv.Mutation) (bool, error) {
					if len(mutation.Entries) > 0 && mutation.Entries[0].Key.String() == peerKey(peer).String() {
						wait(ctx)
					}
					return base.ApplyMutation(ctx, mutation)
				}}
			}
			a, b := startService(t, store, 1), startService(t, base, 2)
			old, replacement := a.NewPublisher(peer), b.NewPublisher(peer)
			initial := Snapshot{Online: true, LastSeenAt: time.Now().UTC(), WorkspaceName: "old"}
			old.Publish(initial)
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("first claim did not reach its stall")
			}
			old.Publish(Snapshot{LastSeenAt: initial.LastSeenAt.Add(time.Second)})
			want := Snapshot{Online: true, LastSeenAt: initial.LastSeenAt.Add(2 * time.Second), WorkspaceName: "active-admin"}
			replacement.Publish(want)
			awaitWriter(t, b, peer)
			unblock()
			awaitWriter(t, a, peer)
			awaitSnapshot(t, b, peer, want)
			if !old.ownership.fenced || old.ownership.claimed {
				t.Fatalf("late first claim was not fenced: %+v", old.ownership)
			}
			old.Publish(initial)
			awaitWriter(t, a, peer)
			awaitSnapshot(t, b, peer, want)
		})
	}
}

func TestUncertainReservationRetainsOriginalCondition(t *testing.T) {
	for _, committed := range []bool{false, true} {
		for _, handoff := range []bool{false, true} {
			name := map[bool]string{false: "uncommitted", true: "committed"}[committed] + map[bool]string{false: "/retry", true: "/handoff"}[handoff]
			t.Run(name, func(t *testing.T) {
				ctx, peer := t.Context(), giznet.PublicKey{99}
				authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
				previous := []byte("previous-reservation")
				if err := authority.Set(ctx, claimKey(peer), previous); err != nil {
					t.Fatal(err)
				}
				store := newStalePresenceStore(t, authority, replica)
				a := startService(t, store, 1)
				publisher := a.NewPublisher(peer)
				lost := errors.New("reservation response lost")
				store.apply = func(ctx context.Context, mutation kv.Mutation) (bool, error) {
					if store.attempts.Load() == 2 {
						if string(mutation.Conditions[1].Expected) != string(previous) {
							t.Errorf("reservation rebased to %q before uncertain commit", mutation.Conditions[1].Expected)
						}
						if committed {
							if _, err := authority.ApplyMutation(ctx, mutation); err != nil {
								return false, err
							}
						}
						return false, lost
					}
					return authority.ApplyMutation(ctx, mutation)
				}
				want := Snapshot{Online: true, LastSeenAt: time.Now().UTC(), WorkspaceName: "room"}
				if err := a.writeTransition(ctx, publisher, want); !errors.Is(err, lost) {
					t.Fatalf("reservation error = %v", err)
				}
				if store.ordinary.Load() != 0 || publisher.ownership.initialized {
					t.Fatal("owner was read before reservation was established")
				}
				var replacement *Publisher
				if handoff {
					b := startService(t, authority, 2)
					replacement = b.NewPublisher(peer)
					if err := b.writeTransition(ctx, replacement, want); err != nil {
						t.Fatal(err)
					}
				}
				if err := a.writeTransition(ctx, publisher, want); err != nil {
					t.Fatal(err)
				}
				if string(publisher.ownership.reservation.expected) != string(previous) || publisher.ownership.fenced != handoff {
					t.Fatalf("reservation retry = %+v", publisher.ownership)
				}
				current, err := authority.Get(ctx, ownerKey(peer))
				winner := publisher
				if handoff {
					winner = replacement
					if store.ordinary.Load() != 0 {
						t.Fatal("fenced reservation proceeded to an owner read")
					}
				}
				if err != nil || string(current) != winner.service.generation+"/"+winner.token {
					t.Fatalf("reservation winner = %q, %v", current, err)
				}
			})
		}
	}
}

func TestUncertainClaimRecoversCommittedOwnerDespiteNewPendingReservation(t *testing.T) {
	base, peer := kv.NewMemory(nil), giznet.PublicKey{100}
	lost := errors.New("claim response lost")
	var once sync.Once
	store := mutationHookStore{Store: base, apply: func(ctx context.Context, mutation kv.Mutation) (bool, error) {
		matched, err := base.ApplyMutation(ctx, mutation)
		if matched && err == nil && len(mutation.Entries) == 2 {
			once.Do(func() { err = lost })
		}
		return matched, err
	}}
	a, b := startService(t, store, 1), startService(t, base, 2)
	publisher, next := a.NewPublisher(peer), b.NewPublisher(peer)
	want := Snapshot{Online: true, LastSeenAt: time.Now().UTC()}
	if err := a.writeTransition(t.Context(), publisher, want); !errors.Is(err, lost) {
		t.Fatalf("claim error = %v", err)
	}
	if err := b.reserveClaim(t.Context(), next, []byte(b.generation+"/"+next.token)); err != nil {
		t.Fatal(err)
	}
	want.WorkspaceName = "still-current"
	if err := a.writeTransition(t.Context(), publisher, want); err != nil {
		t.Fatal(err)
	}
	if !publisher.ownership.claimed || publisher.ownership.fenced {
		t.Fatalf("committed owner was revoked by pending reservation: %+v", publisher.ownership)
	}
	awaitSnapshot(t, a, peer, want)
	want.WorkspaceName = "replacement"
	if err := b.writeTransition(t.Context(), next, want); err != nil {
		t.Fatal(err)
	}
	if err := a.writeTransition(t.Context(), publisher, Snapshot{}); err != nil {
		t.Fatal(err)
	}
	awaitSnapshot(t, a, peer, want)
}
