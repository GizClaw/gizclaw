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
		service.NewPublisher(peer).Publish(want)
		synctest.Wait()
		other := giznet.PublicKey{3}
		service.NewPublisher(other).Publish(want)
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

func TestQueuedTransitionRetainsCallerDeadline(t *testing.T) {
	s := New(kv.NewMemory(nil), giznet.PublicKey{1})
	defer s.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	// No worker: even a buffered request must return on its caller's deadline.
	if _, err := s.applyTransition(ctx, kv.Mutation{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued transition = %v", err)
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
