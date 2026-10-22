package peerpresence

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

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
