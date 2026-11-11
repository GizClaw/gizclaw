package peerpresence

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestIdleWriterPublishesWithoutWaitingForAnotherTransition(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		service := startService(t, kv.NewMemory(nil), 1)
		peer := giznet.PublicKey{2}
		want := Snapshot{Online: true, LastSeenAt: time.Now().UTC(), WorkspaceName: "room"}
		service.NewPublisher(peer).Publish(want)
		// All runnable publication work can finish without advancing the fake
		// clock. A timer used solely to collect a batch leaves this unpublished.
		synctest.Wait()
		got, err := service.Read(t.Context(), peer)
		if err != nil || got != want {
			t.Fatalf("idle writer presence = %+v, %v; want %+v", got, err, want)
		}
	})
}

func TestStandbyWriterProgressesBeforeBlockedCommitDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		peer := giznet.PublicKey{2}
		store := &blockingStore{Store: kv.NewMemory(nil), peer: peer, entered: make(chan struct{}), release: make(chan struct{})}
		service := startService(t, store, 1)
		want := Snapshot{Online: true, LastSeenAt: time.Now().UTC()}
		other := giznet.PublicKey{3}
		publisher := service.NewPublisher(other)
		initial := want
		initial.WorkspaceName = "initial"
		publisher.Publish(initial)
		synctest.Wait()
		service.NewPublisher(peer).Publish(want)
		synctest.Wait()
		publisher.Publish(want)
		time.Sleep(writeTimeout / 4)
		synctest.Wait()
		got, err := service.Read(t.Context(), other)
		if err != nil || got != want {
			t.Fatalf("standby presence = %+v, %v; want %+v", got, err, want)
		}
		if blocked, err := service.Read(t.Context(), peer); err != nil || blocked.Online {
			t.Fatalf("first commit did not remain blocked: %+v, %v", blocked, err)
		}
		close(store.release)
		synctest.Wait()
		if got, err := service.Read(t.Context(), peer); err != nil || got != want {
			t.Fatalf("released presence = %+v, %v; want %+v", got, err, want)
		}
	})
}

func TestStandbyFirstClaimSharesOriginalBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		peer := giznet.PublicKey{2}
		store := &blockingStore{Store: kv.NewMemory(nil), peer: peer, entered: make(chan struct{}), release: make(chan struct{})}
		service := startService(t, store, 1)
		want := Snapshot{Online: true, LastSeenAt: time.Now().UTC()}
		service.NewPublisher(peer).Publish(want)
		synctest.Wait()
		other := giznet.PublicKey{3}
		service.NewPublisher(other).Publish(want)
		// Reservation and first publication each use the standby coordinator;
		// neither receives a fresh per-operation timeout.
		time.Sleep(writeTimeout / 2)
		synctest.Wait()
		if got, err := service.Read(t.Context(), other); err != nil || got != want {
			t.Fatalf("standby first claim = %+v, %v; want %+v", got, err, want)
		}
		close(store.release)
		synctest.Wait()
	})
}

func TestPendingReservationHonorsDeadlineAndClose(t *testing.T) {
	for _, closeService := range []bool{false, true} {
		t.Run(map[bool]string{false: "deadline-and-coalesced-offline", true: "close"}[closeService], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				base, peer := kv.NewMemory(nil), giznet.PublicKey{2}
				entered, returned := make(chan time.Time, 1), make(chan error, 1)
				var attempts, ownerReads atomic.Int32
				reader := presenceReadHookStore{Store: base, get: func(ctx context.Context, key kv.Key) ([]byte, error) {
					if key.String() == ownerKey(peer).String() {
						ownerReads.Add(1)
					}
					return base.Get(ctx, key)
				}}
				store := mutationHookStore{Store: reader, apply: func(ctx context.Context, mutation kv.Mutation) (bool, error) {
					if len(mutation.Entries) > 0 && mutation.Entries[0].Key.String() == claimKey(peer).String() {
						attempts.Add(1)
						deadline, _ := ctx.Deadline()
						entered <- deadline
						<-ctx.Done()
						returned <- ctx.Err()
						return false, ctx.Err()
					}
					return base.ApplyMutation(ctx, mutation)
				}}
				service := startService(t, store, 1)
				publisher := service.NewPublisher(peer)
				started := time.Now()
				publisher.Publish(Snapshot{Online: true})
				synctest.Wait()
				if deadline := <-entered; deadline != started.Add(writeTimeout) {
					t.Fatalf("reservation deadline = %v, want %v", deadline, started.Add(writeTimeout))
				}
				publisher.Publish(Snapshot{})
				wantErr := context.DeadlineExceeded
				if closeService {
					wantErr = context.Canceled
					if err := service.Close(); err != nil {
						t.Fatal(err)
					}
				} else {
					time.Sleep(writeTimeout + time.Second)
				}
				synctest.Wait()
				if err := <-returned; !errors.Is(err, wantErr) {
					t.Fatalf("reservation returned %v, want %v", err, wantErr)
				}
				if attempts.Load() != 1 || ownerReads.Load() != 0 || publisher.ownership.initialized {
					t.Fatalf("coalesced reservation attempted=%d owner reads=%d owner=%+v", attempts.Load(), ownerReads.Load(), publisher.ownership)
				}
				if _, err := base.Get(t.Context(), peerKey(peer)); !errors.Is(err, kv.ErrNotFound) {
					t.Fatalf("unclaimed offline publisher wrote a snapshot: %v", err)
				}
			})
		})
	}
}

type delayedMutationStore struct {
	kv.Store
	active    atomic.Int64
	overlaps  atomic.Int64
	published chan struct{}
}

func (s *delayedMutationStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	if s.active.Add(1) > 1 {
		s.overlaps.Add(1)
	}
	defer s.active.Add(-1)
	time.Sleep(13 * time.Millisecond)
	matched, err := s.Store.ApplyMutation(ctx, mutation)
	if matched && err == nil {
		for _, entry := range mutation.Entries {
			var snapshot Snapshot
			if json.Unmarshal(entry.Value, &snapshot) == nil && !snapshot.LastSeenAt.IsZero() {
				s.published <- struct{}{}
			}
		}
	}
	return matched, err
}

func TestContinuousPublicationCoordinatesLeaseRenewal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &delayedMutationStore{Store: kv.NewMemory(nil), published: make(chan struct{}, 1)}
		service := startService(t, store, 1)
		peer := giznet.PublicKey{2}
		publisher := service.NewPublisher(peer)
		ctx, cancel := context.WithCancel(t.Context())
		go func() {
			for ctx.Err() == nil {
				publisher.Publish(Snapshot{Online: true, LastSeenAt: time.Now().UTC()})
				select {
				case <-ctx.Done():
					return
				case <-store.published:
				}
			}
		}()
		// Cross three renewal ticks and the original lease expiry while
		// publication remains continuously runnable on the same Server.
		time.Sleep(31 * time.Second)
		cancel()
		synctest.Wait()
		if count := store.overlaps.Load(); count != 0 {
			t.Fatalf("lease renewal overlapped %d ordinary state commits", count)
		}
		if got, err := service.Read(t.Context(), peer); err != nil || !got.Online {
			t.Fatalf("continuously publishing Peer lost its lease: %+v, %v", got, err)
		}
	})
}

func TestTransitionBatchIsolatesLostOwnership(t *testing.T) {
	store := kv.NewMemory(nil)
	s := startService(t, store, 1)
	stale, valid := giznet.PublicKey{2}, giznet.PublicKey{3}
	if err := store.Set(t.Context(), ownerKey(stale), []byte("replacement")); err != nil {
		t.Fatal(err)
	}
	requests := make([]transitionMutation, 2)
	for i, key := range []giznet.PublicKey{stale, valid} {
		requests[i] = transitionMutation{ctx: t.Context(), result: make(chan transitionResult, 1), mutation: kv.Mutation{
			Conditions: []kv.Condition{{Key: generationKey(s.server), Expected: []byte(s.generation)}, {Key: ownerKey(key)}},
			Entries:    []kv.Entry{{Key: peerKey(key), Value: []byte("snapshot")}, {Key: ownerKey(key), Value: []byte("claimed")}},
		}}
	}
	s.commitTransitions(t.Context(), requests)
	for i, request := range requests {
		result := <-request.result
		if result.err != nil || result.matched != (i == 1) {
			t.Fatalf("transition %d = %+v", i, result)
		}
	}
	if _, err := store.Get(t.Context(), peerKey(stale)); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("stale connection wrote its snapshot: %v", err)
	}
	if value, err := store.Get(t.Context(), peerKey(valid)); err != nil || string(value) != "snapshot" {
		t.Fatalf("valid connection did not progress: %q %v", value, err)
	}
}

func TestTransitionBatchDoesNotReplayUncertainCommit(t *testing.T) {
	base := kv.NewMemory(nil)
	s := startService(t, base, 1)
	uncertain := errors.New("commit response lost")
	calls := 0
	s.store = mutationHookStore{Store: base, apply: func(ctx context.Context, mutation kv.Mutation) (bool, error) {
		calls++
		matched, err := base.ApplyMutation(ctx, mutation)
		if err != nil || !matched {
			t.Fatalf("commit = %v, %v", matched, err)
		}
		return false, uncertain
	}}
	requests := make([]transitionMutation, 8)
	for i := range requests {
		requests[i] = transitionMutation{ctx: t.Context(), result: make(chan transitionResult, 1), mutation: kv.Mutation{
			Conditions: []kv.Condition{{Key: generationKey(s.server), Expected: []byte(s.generation)}, {Key: ownerKey(giznet.PublicKey{byte(i + 10)})}},
		}}
	}
	s.commitTransitions(t.Context(), requests)
	if calls != 1 {
		t.Fatalf("uncertain commit replayed %d times", calls)
	}
	for _, request := range requests {
		if result := <-request.result; result.matched || !errors.Is(result.err, uncertain) {
			t.Fatalf("uncertain result = %+v", result)
		}
	}
	s.store = base
}

func TestTransitionBatchChecksAndReturnsEveryGuard(t *testing.T) {
	base := kv.NewMemory(nil)
	store := &mutationBudgetStore{Store: base}
	s := New(store, giznet.PublicKey{1})
	t.Cleanup(s.cancel)
	peers := []giznet.PublicKey{{2}, {3}}
	if err := base.BatchSet(t.Context(), []kv.Entry{
		{Key: generationKey(s.server), Value: []byte(s.generation)},
		{Key: claimKey(peers[0]), Value: []byte("new-reservation")},
		{Key: claimKey(peers[1]), Value: []byte("original-reservation")},
	}); err != nil {
		t.Fatal(err)
	}
	requests := make([]transitionMutation, len(peers))
	for i, peer := range peers {
		requests[i] = transitionMutation{ctx: t.Context(), result: make(chan transitionResult, 1), mutation: kv.Mutation{
			Conditions: []kv.Condition{
				{Key: generationKey(s.server), Expected: []byte(s.generation)},
				{Key: ownerKey(peer)},
				{Key: claimKey(peer), Expected: []byte("original-reservation")},
			},
			Entries: []kv.Entry{{Key: peerKey(peer), Value: []byte("snapshot")}},
		}}
	}
	s.commitTransitions(t.Context(), requests)
	for i, request := range requests {
		result := <-request.result
		if result.err != nil || result.matched != (i == 1) {
			t.Fatalf("guarded transition %d = %+v", i, result)
		}
		if i == 0 && (len(result.observed) != 3 || string(result.observed[0].Value) != s.generation || !errors.Is(result.observed[1].Err, kv.ErrNotFound) || string(result.observed[2].Value) != "new-reservation") {
			t.Fatalf("rejected guard observations = %+v", result.observed)
		}
	}
	if store.calls != 2 {
		t.Fatalf("reservation rejection used %d mutations, want 2", store.calls)
	}
	if _, err := base.Get(t.Context(), peerKey(peers[0])); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("fenced reservation published: %v", err)
	}
}

func takeTransition(t *testing.T, service *Service) transitionMutation {
	t.Helper()
	select {
	case request := <-service.mutations:
		return request
	case <-time.After(time.Second):
		t.Fatal("writer did not queue a transition")
		return transitionMutation{}
	}
}

func TestRejectedBatchReusesAuthorityObservationsInWriters(t *testing.T) {
	for _, replacement := range []bool{false, true} {
		t.Run(map[bool]string{false: "stale-owner-recovery", true: "reservation-races-observation"}[replacement], func(t *testing.T) {
			base, replica := kv.NewMemory(nil), kv.NewMemory(nil)
			store := newStalePresenceStore(t, base, replica)
			peers := []giznet.PublicKey{{2}, {3}}
			var primaryGets atomic.Int32
			raced := false
			view := presenceReadHookStore{Store: store,
				get: func(ctx context.Context, key kv.Key) ([]byte, error) {
					if kv.WithPrimaryReads(ctx) == ctx {
						primaryGets.Add(1)
					}
					return store.Get(ctx, key)
				},
				batchGet: func(ctx context.Context, keys []kv.Key) []kv.GetResult {
					values := store.BatchGet(ctx, keys)
					if replacement && !raced && kv.WithPrimaryReads(ctx) == ctx {
						raced = true
						// The observation is valid, but the next mutation must
						// still reject a reservation that changes after the read.
						if err := base.Set(ctx, claimKey(peers[0]), []byte("replacement-reservation")); err != nil {
							t.Fatal(err)
						}
					}
					return values
				},
			}
			service := New(view, giznet.PublicKey{1})
			t.Cleanup(service.cancel)
			if err := base.BatchSet(t.Context(), []kv.Entry{
				{Key: generationKey(service.server), Value: []byte(service.generation)},
				{Key: ownerKey(peers[0]), Value: []byte("legacy-owner-a")},
				{Key: ownerKey(peers[1]), Value: []byte("legacy-owner-b")},
			}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), writeTimeout)
			defer cancel()
			publishers := []*Publisher{service.NewPublisher(peers[0]), service.NewPublisher(peers[1])}
			done := make(chan error, len(publishers))
			for _, publisher := range publishers {
				go func() { done <- service.writeTransition(ctx, publisher, Snapshot{Online: true}) }()
			}
			// Drive complete batches deterministically, including both reservations
			// and the stale first-owner attempts. No real coordinator workers run.
			for range 3 {
				batch := []transitionMutation{takeTransition(t, service), takeTransition(t, service)}
				service.commitTransitions(ctx, batch)
			}
			for range publishers {
				select {
				case err := <-done:
					if err != nil {
						t.Fatal(err)
					}
				case <-ctx.Done():
					t.Fatal("writers did not consume batch observations")
				}
			}
			wantReads, wantMutations := int32(1), int32(3)
			if replacement {
				wantReads, wantMutations = 2, 4
			}
			if store.ordinary.Load() != 2 || store.primary.Load() != wantReads || primaryGets.Load() != 0 || store.attempts.Load() != wantMutations {
				t.Fatalf("ordinary=%d primary=%d primary Gets=%d mutations=%d; want 2/%d/0/%d", store.ordinary.Load(), store.primary.Load(), primaryGets.Load(), store.attempts.Load(), wantReads, wantMutations)
			}
			for i, publisher := range publishers {
				fenced := replacement && i == 0
				if publisher.ownership.fenced != fenced || publisher.ownership.claimed == fenced {
					t.Fatalf("writer %d ownership = %+v", i, publisher.ownership)
				}
			}
		})
	}
}

func TestQueuedTransitionRetainsCallerDeadline(t *testing.T) {
	s := New(kv.NewMemory(nil), giznet.PublicKey{1})
	defer s.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	// No worker: even a buffered request must return on its caller's deadline.
	if result := s.applyTransition(ctx, kv.Mutation{}); !errors.Is(result.err, context.DeadlineExceeded) {
		t.Fatalf("queued transition = %+v", result)
	}
}

type mutationBudgetStore struct {
	kv.Store
	calls int
}

func (s *mutationBudgetStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	s.calls++
	if s.calls > 4 {
		return false, errors.New("conditional mutation budget exceeded")
	}
	return s.Store.ApplyMutation(ctx, mutation)
}

func TestBulkHandoffDoesNotStarveValidTransition(t *testing.T) {
	base := kv.NewMemory(nil)
	store := &mutationBudgetStore{Store: base}
	s := New(store, giznet.PublicKey{1})
	t.Cleanup(s.cancel)
	entries := []kv.Entry{{Key: generationKey(s.server), Value: []byte(s.generation)}}
	requests := make([]transitionMutation, 128)
	for i := range requests {
		peer := giznet.PublicKey{byte(i + 10)}
		if i < len(requests)-1 {
			entries = append(entries, kv.Entry{Key: ownerKey(peer), Value: []byte("replacement")})
		}
		requests[i] = transitionMutation{ctx: t.Context(), result: make(chan transitionResult, 1), mutation: kv.Mutation{
			Conditions: []kv.Condition{{Key: generationKey(s.server), Expected: []byte(s.generation)}, {Key: ownerKey(peer)}},
			Entries:    []kv.Entry{{Key: peerKey(peer), Value: []byte("published")}, {Key: ownerKey(peer), Value: []byte("claimed")}},
		}}
	}
	if err := base.BatchSet(t.Context(), entries); err != nil {
		t.Fatal(err)
	}
	s.commitTransitions(t.Context(), requests)
	for i, request := range requests {
		result := <-request.result
		if result.err != nil || result.matched != (i == len(requests)-1) {
			t.Fatalf("transition %d matched=%v error=%v after %d mutations", i, result.matched, result.err, store.calls)
		}
	}
	if value, err := base.Get(t.Context(), peerKey(giznet.PublicKey{137})); err != nil || string(value) != "published" {
		t.Fatalf("valid Peer did not progress during bulk handoff: %q %v", value, err)
	}
}

func TestRejectedBatchCannotCrossServerGeneration(t *testing.T) {
	store := kv.NewMemory(nil)
	old := startService(t, store, 1)
	_ = startService(t, store, 1)
	requests := make([]transitionMutation, 2)
	for i := range requests {
		peer := giznet.PublicKey{byte(i + 10)}
		requests[i] = transitionMutation{ctx: t.Context(), result: make(chan transitionResult, 1), mutation: kv.Mutation{
			Conditions: []kv.Condition{{Key: generationKey(old.server), Expected: []byte(old.generation)}, {Key: ownerKey(peer)}},
			Entries:    []kv.Entry{{Key: peerKey(peer), Value: []byte("obsolete")}},
		}}
	}
	old.commitTransitions(t.Context(), requests)
	for _, request := range requests {
		if result := <-request.result; result.matched || result.err != nil {
			t.Fatalf("old generation batch = %+v", result)
		}
		if _, err := store.Get(t.Context(), request.mutation.Entries[0].Key); !errors.Is(err, kv.ErrNotFound) {
			t.Fatalf("old generation published a snapshot: %v", err)
		}
	}
}
