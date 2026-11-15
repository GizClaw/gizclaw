package pendingdeletion

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestKVTaskExecutionCountEncoding(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  json.RawMessage
		want int
	}{
		{name: "legacy"},
		{name: "persisted", raw: json.RawMessage("7"), want: 7},
		{name: "negative", raw: json.RawMessage("-1")},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := kv.NewMemory(nil)
			record, err := New(KindPeer, "execution-count", nil, ReasonPeerDelete, struct{}{}, time.Unix(1, 0))
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := CreateOrGet(t.Context(), store, record); err != nil {
				t.Fatal(err)
			}
			raw, err := store.Get(t.Context(), kvTaskKey(record.DeletionID))
			if err != nil {
				t.Fatal(err)
			}
			var state map[string]json.RawMessage
			if err := json.Unmarshal(raw, &state); err != nil {
				t.Fatal(err)
			}
			delete(state, "execution_count")
			if test.raw != nil {
				state["execution_count"] = test.raw
			}
			raw, err = json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Set(t.Context(), kvTaskKey(record.DeletionID), raw); err != nil {
				t.Fatal(err)
			}
			source := KVSource{Store: store, SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
			task, err := source.GetTask(t.Context(), record.DeletionID)
			if test.name == "negative" {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("negative persisted count = %v, want ErrInvalid", err)
				}
				return
			}
			if err != nil || task.ExecutionCount != test.want {
				t.Fatalf("decoded task = %#v, %v", task, err)
			}
			claim, ok, err := source.Claim(t.Context(), Reference{Source: source.Name(), DeletionID: record.DeletionID, MarkerFingerprint: task.MarkerFingerprint}, time.Unix(2, 0), time.Minute)
			if err != nil || !ok || claim.ExecutionCount != test.want {
				t.Fatalf("claim = %#v, %v, %v", claim, ok, err)
			}
			raw, err = store.Get(t.Context(), kvTaskKey(record.DeletionID))
			if err != nil {
				t.Fatal(err)
			}
			var persisted struct {
				ExecutionCount *int `json:"execution_count"`
			}
			if err := json.Unmarshal(raw, &persisted); err != nil {
				t.Fatal(err)
			}
			if persisted.ExecutionCount == nil || *persisted.ExecutionCount != test.want {
				t.Fatalf("encoded execution count not preserved: %s", raw)
			}
		})
	}
}

func TestKVTaskExecutionCountUsesCommittedTransitions(t *testing.T) {
	for _, transition := range []string{"defer", "fail"} {
		for _, outcome := range []string{"cas retry", "rejected", "uncertain"} {
			t.Run(transition+"/"+outcome, func(t *testing.T) {
				writer, replica := kv.NewMemory(nil), kv.NewMemory(nil)
				record, err := New(KindPeer, "count-cas", nil, ReasonPeerDelete, struct{}{}, time.Unix(1, 0))
				if err != nil {
					t.Fatal(err)
				}
				if _, _, err := CreateOrGet(t.Context(), writer, record); err != nil {
					t.Fatal(err)
				}
				authority := KVSource{Store: writer, SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
				task, err := authority.GetTask(t.Context(), record.DeletionID)
				if err != nil {
					t.Fatal(err)
				}
				claim, ok, err := authority.Claim(t.Context(), Reference{Source: authority.Name(), DeletionID: record.DeletionID, MarkerFingerprint: task.MarkerFingerprint}, time.Unix(2, 0), time.Minute)
				if err != nil || !ok {
					t.Fatalf("Claim = %v, %v", ok, err)
				}
				raw, err := encodeKVTaskState(claim.Task)
				if err != nil {
					t.Fatal(err)
				}
				entries, err := KVEntries(record)
				if err != nil {
					t.Fatal(err)
				}
				if err := replica.BatchSet(t.Context(), append(entries, kv.Entry{Key: kvTaskKey(record.DeletionID), Value: raw})); err != nil {
					t.Fatal(err)
				}
				if err := authority.Renew(t.Context(), claim, time.Unix(3, 0), 2*time.Minute); err != nil {
					t.Fatal(err)
				}
				store := &laggingTaskStore{Store: writer, replica: replica, reject: outcome == "rejected"}
				if outcome == "uncertain" {
					store.commitError = errors.New("transition commit response lost")
				}
				source := KVSource{Store: store, SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
				apply := func() error {
					if transition == "defer" {
						return source.Defer(t.Context(), claim, "waiting", "safe wait", time.Unix(10, 0), time.Unix(4, 0))
					}
					return source.Fail(t.Context(), claim, "temporary", "temporary", false, time.Unix(10, 0), time.Unix(4, 0), 3)
				}
				err = apply()
				wantErr, wantCount, wantMutations := error(nil), 1, 2
				if outcome == "rejected" {
					wantErr, wantCount, wantMutations = ErrConflict, 0, 4
				} else if outcome == "uncertain" {
					wantErr = store.commitError
				}
				if !errors.Is(err, wantErr) || store.mutations != wantMutations {
					t.Fatalf("transition = %v, mutations=%d; want %v, %d", err, store.mutations, wantErr, wantMutations)
				}
				if outcome != "rejected" {
					if err := apply(); !errors.Is(err, ErrConflict) {
						t.Fatalf("replayed transition = %v, want ErrConflict", err)
					}
				}
				current, err := authority.GetTask(t.Context(), record.DeletionID)
				if err != nil || current.ExecutionCount != wantCount {
					t.Fatalf("committed execution count = %#v, %v; want %d", current, err, wantCount)
				}
				if outcome == "rejected" && current.Status != StatusRunning || outcome != "rejected" && current.Status != StatusRetryWait {
					t.Fatalf("count/state not committed together: %#v", current)
				}
			})
		}
	}
}

func TestKVTaskDoesNotRebuildMissingState(t *testing.T) {
	store := kv.NewMemory(nil)
	record, err := New(KindPeer, "peer-missing-state", nil, ReasonPeerDelete, struct{}{}, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := CreateOrGet(t.Context(), store, record); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(t.Context(), kvTaskKey(record.DeletionID)); err != nil {
		t.Fatal(err)
	}
	source := KVSource{Store: store, SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
	if _, err := source.GetTask(t.Context(), record.DeletionID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing state = %v", err)
	}
	if _, err := store.Get(t.Context(), kvTaskKey(record.DeletionID)); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("read rebuilt task state: %v", err)
	}
}

func TestKVTaskTransitionsUseAtomicGuards(t *testing.T) {
	for _, fixture := range []struct {
		name string
		new  func(*testing.T) kv.Store
	}{
		{name: "memory", new: func(*testing.T) kv.Store { return kv.NewMemory(nil) }},
		{name: "badger", new: func(t *testing.T) kv.Store {
			store, err := kv.NewBadgerInMemory(nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			return store
		}},
		{name: "prefixed", new: func(*testing.T) kv.Store { return kv.Prefixed(kv.NewMemory(nil), kv.Key{"domain"}) }},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			ctx := context.Background()
			store := fixture.new(t)
			record, err := New(KindPeer, "peer-a", nil, ReasonPeerDelete, map[string]string{"public_key": "peer-a"}, time.Unix(1, 0))
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := CreateOrGet(ctx, store, record); err != nil {
				t.Fatal(err)
			}
			source := KVSource{Store: store, SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
			refs, _, err := source.ScanDue(ctx, time.Unix(2, 0), 10, "")
			if err != nil || len(refs) != 1 {
				t.Fatalf("ScanDue() = %#v, %v", refs, err)
			}
			claim, claimed, err := source.Claim(ctx, refs[0], time.Unix(2, 0), time.Minute)
			if err != nil || !claimed {
				t.Fatalf("Claim() = %#v, %v, %v", claim, claimed, err)
			}
			if _, claimed, err := source.Claim(ctx, refs[0], time.Unix(2, 0), time.Minute); err != nil || claimed {
				t.Fatalf("duplicate Claim() = %v, %v", claimed, err)
			}
			if err := source.Renew(ctx, claim, time.Unix(2, 0), 2*time.Minute); err != nil {
				t.Fatalf("Renew() = %v", err)
			}
			if err := source.Defer(ctx, claim, "waiting", "safe wait", time.Unix(3, 0), time.Unix(2, 0)); err != nil {
				t.Fatalf("Defer() = %v", err)
			}
			if err := source.Defer(ctx, claim, "waiting", "duplicate", time.Unix(3, 0), time.Unix(2, 0)); !errors.Is(err, ErrConflict) {
				t.Fatalf("duplicate Defer() = %v, want ErrConflict", err)
			}
			deferred, err := source.GetTask(ctx, record.DeletionID)
			if err != nil || deferred.Status != StatusRetryWait || deferred.FailureCount != 0 || deferred.ExecutionCount != 1 {
				t.Fatalf("GetTask(deferred) = %#v, %v", deferred, err)
			}
			refs, _, err = source.ScanDue(ctx, time.Unix(4, 0), 10, "")
			if err != nil || len(refs) != 1 {
				t.Fatalf("ScanDue(after defer) = %#v, %v", refs, err)
			}
			claim, claimed, err = source.Claim(ctx, refs[0], time.Unix(4, 0), time.Minute)
			if err != nil || !claimed {
				t.Fatalf("Claim(after defer) = %#v, %v, %v", claim, claimed, err)
			}
			stale := claim
			stale.LeaseToken = "stale"
			if err := source.Fail(ctx, stale, "temporary", "temporary", false, time.Unix(3, 0), time.Unix(2, 0), 3); !errors.Is(err, ErrConflict) {
				t.Fatalf("Fail(stale) = %v, want ErrConflict", err)
			}
			if err := source.Fail(ctx, claim, "temporary", "temporary", false, time.Unix(5, 0), time.Unix(4, 0), 3); err != nil {
				t.Fatalf("Fail(current) = %v", err)
			}
			if err := source.Fail(ctx, claim, "temporary", "duplicate", false, time.Unix(5, 0), time.Unix(4, 0), 3); !errors.Is(err, ErrConflict) {
				t.Fatalf("duplicate Fail() = %v, want ErrConflict", err)
			}
			task, err := source.GetTask(ctx, record.DeletionID)
			if err != nil || task.Status != StatusRetryWait || task.FailureCount != 1 || task.ExecutionCount != 2 {
				t.Fatalf("GetTask() = %#v, %v", task, err)
			}
			refs, _, err = source.ScanDue(ctx, time.Unix(6, 0), 10, "")
			if err != nil || len(refs) != 1 {
				t.Fatalf("ScanDue(retry) = %#v, %v", refs, err)
			}
			claim, claimed, err = source.Claim(ctx, refs[0], time.Unix(6, 0), time.Minute)
			if err != nil || !claimed {
				t.Fatalf("Claim(retry) = %#v, %v, %v", claim, claimed, err)
			}
			if err := source.Renew(ctx, claim, time.Unix(6, 0), 2*time.Minute); err != nil {
				t.Fatalf("Renew(after failure) = %v", err)
			}
			claim, err = source.Checkpoint(ctx, claim, PhaseFinalize, time.Unix(6, 0))
			if err != nil || claim.Phase != PhaseFinalize || claim.ExecutionCount != 2 {
				t.Fatalf("Checkpoint() = %#v, %v", claim, err)
			}
			wrongGeneration := claim
			wrongGeneration.MarkerFingerprint = "wrong"
			if err := source.Fail(ctx, wrongGeneration, "invalid", "invalid", true, time.Unix(6, 0), time.Unix(6, 0), 3); !errors.Is(err, ErrConflict) {
				t.Fatalf("Fail(wrong fingerprint) = %v, want ErrConflict", err)
			}
			if err := source.Fail(ctx, claim, "invalid", "safe message", true, time.Unix(6, 0), time.Unix(6, 0), 3); err != nil {
				t.Fatalf("Fail(terminal) = %v", err)
			}
			if _, err := source.Retry(ctx, record.DeletionID, time.Unix(7, 0)); err != nil {
				t.Fatalf("Retry() = %v", err)
			}
			task, err = source.GetTask(ctx, record.DeletionID)
			if err != nil || task.Status != StatusQueued || task.Phase != PhaseFinalize || task.FailureCount != 0 || task.LastErrorCode != "" || task.ExecutionCount != 3 {
				t.Fatalf("GetTask(after retry) = %#v, %v", task, err)
			}
			if _, err := source.Retry(ctx, record.DeletionID, time.Unix(8, 0)); !errors.Is(err, ErrConflict) {
				t.Fatalf("Retry(queued) = %v, want ErrConflict", err)
			}
		})
	}
}

func TestKVFinalizeUsesMatchingLeaseAndDeletesExactKeys(t *testing.T) {
	for _, fixture := range []struct {
		name string
		new  func(*testing.T) kv.Store
	}{
		{name: "memory", new: func(*testing.T) kv.Store { return kv.NewMemory(nil) }},
		{name: "badger", new: func(t *testing.T) kv.Store {
			store, err := kv.NewBadgerInMemory(nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			return store
		}},
		{name: "prefixed", new: func(*testing.T) kv.Store {
			return kv.Prefixed(kv.NewMemory(nil), kv.Key{"domain"})
		}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			ctx := t.Context()
			store := fixture.new(t)
			source := KVSource{Store: store, SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
			record, err := New(KindPeer, "peer-a", nil, ReasonPeerDelete, map[string]string{"public_key": "peer-a"}, time.Unix(1, 0))
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := CreateOrGet(ctx, store, record); err != nil {
				t.Fatal(err)
			}
			domainKey := kv.Key{"peers", "by-pubkey", "peer-a"}
			foreignKey := kv.Key{"peers", "by-pubkey", "peer-b"}
			if err := store.BatchSet(ctx, []kv.Entry{{Key: domainKey, Value: []byte("a")}, {Key: foreignKey, Value: []byte("b")}}); err != nil {
				t.Fatal(err)
			}
			refs, _, err := source.ScanDue(ctx, time.Unix(2, 0), 1, "")
			if err != nil || len(refs) != 1 {
				t.Fatalf("ScanDue() = %#v, %v", refs, err)
			}
			claim, claimed, err := source.Claim(ctx, refs[0], time.Unix(2, 0), time.Minute)
			if err != nil || !claimed {
				t.Fatalf("Claim() = %#v, %v, %v", claim, claimed, err)
			}

			stale := claim
			stale.LeaseToken = "stale"
			if err := source.Finalize(ctx, stale, time.Unix(3, 0), []kv.Key{domainKey}); !errors.Is(err, ErrConflict) {
				t.Fatalf("Finalize(stale) = %v, want ErrConflict", err)
			}
			if _, err := store.Get(ctx, domainKey); err != nil {
				t.Fatalf("domain key after stale finalize = %v", err)
			}

			if err := source.Finalize(ctx, claim, time.Unix(3, 0), []kv.Key{domainKey}); err != nil {
				t.Fatalf("Finalize() = %v", err)
			}
			for _, key := range []kv.Key{domainKey, byIDKey(record.DeletionID), byLocatorKey(record.Kind, record.ResourceID), kvTaskKey(record.DeletionID)} {
				if _, err := store.Get(ctx, key); !errors.Is(err, kv.ErrNotFound) {
					t.Fatalf("Get(%s) after finalize = %v, want ErrNotFound", key.String(), err)
				}
			}
			if got, err := store.Get(ctx, foreignKey); err != nil || string(got) != "b" {
				t.Fatalf("foreign key = %q, %v", got, err)
			}
			if err := source.Finalize(ctx, claim, time.Unix(3, 0), nil); !errors.Is(err, ErrConflict) {
				t.Fatalf("Finalize(completed) = %v, want ErrConflict", err)
			}
		})
	}
}

func TestKVFinalizeRejectsUnsafeDomainKeys(t *testing.T) {
	ctx := t.Context()
	store := kv.NewMemory(nil)
	source := KVSource{Store: store, SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
	record, err := New(KindPeer, "peer-a", nil, ReasonPeerDelete, struct{}{}, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := CreateOrGet(ctx, store, record); err != nil {
		t.Fatal(err)
	}
	refs, _, err := source.ScanDue(ctx, time.Unix(2, 0), 1, "")
	if err != nil || len(refs) != 1 {
		t.Fatalf("ScanDue() = %#v, %v", refs, err)
	}
	claim, claimed, err := source.Claim(ctx, refs[0], time.Unix(2, 0), time.Minute)
	if err != nil || !claimed {
		t.Fatalf("Claim() = %#v, %v, %v", claim, claimed, err)
	}
	for _, keys := range [][]kv.Key{
		{nil},
		{{"peers", ""}},
		{{"peers", "a"}, {"peers", "a"}},
		{{"pending-deletion", "by-id", record.DeletionID}},
	} {
		if err := source.Finalize(ctx, claim, time.Unix(3, 0), keys); err == nil {
			t.Fatalf("Finalize(%v) error = nil", keys)
		}
	}
	if _, err := source.GetTask(ctx, record.DeletionID); err != nil {
		t.Fatalf("task after rejected finalize = %v", err)
	}
}

func TestConditionalFailedRetryRejectsNewFailureAndGeneration(t *testing.T) {
	for _, change := range []string{"unchanged", "new_failure", "new_generation", "execution_count"} {
		t.Run(change, func(t *testing.T) {
			source := KVSource{Store: kv.NewMemory(nil), SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
			now := time.Now().Add(-time.Minute)
			observed := seedFailedRecoveryTask(t, source, "conditional-recovery", now)
			if change == "execution_count" {
				observed.ExecutionCount--
			} else if change != "unchanged" {
				if _, err := source.Retry(t.Context(), observed.Record.DeletionID, now.Add(2*time.Second)); err != nil {
					t.Fatal(err)
				}
				claim, ok, err := source.Claim(t.Context(), Reference{Source: source.Name(), DeletionID: observed.Record.DeletionID, MarkerFingerprint: observed.MarkerFingerprint}, now.Add(2*time.Second), time.Minute)
				if err != nil || !ok {
					t.Fatal(err)
				}
				if change == "new_generation" {
					if err := source.Finalize(t.Context(), claim, now.Add(2*time.Second), nil); err != nil {
						t.Fatal(err)
					}
					seedFailedRecoveryTask(t, source, "conditional-recovery", now.Add(3*time.Second))
				} else if err := source.Fail(t.Context(), claim, "different_failure", "new safe error", true, now.Add(3*time.Second), now.Add(3*time.Second), 3); err != nil {
					t.Fatal(err)
				}
			}
			before, err := source.GetTask(t.Context(), observed.Record.DeletionID)
			if err != nil {
				t.Fatal(err)
			}
			retried, err := source.RetryFailed(t.Context(), observed, time.Now())
			if change == "unchanged" {
				if err != nil || retried.Status != StatusQueued || retried.Phase != PhaseFinalize || retried.FailureCount != 0 || retried.ExecutionCount != observed.ExecutionCount {
					t.Fatalf("retry=%+v, %v", retried, err)
				}
				if _, err := source.RetryFailed(t.Context(), observed, time.Now()); !errors.Is(err, ErrConflict) {
					t.Fatalf("repeated observation=%v", err)
				}
			} else {
				if !errors.Is(err, ErrConflict) {
					t.Fatalf("stale recovery=%v", err)
				}
				after, err := source.GetTask(t.Context(), observed.Record.DeletionID)
				if err != nil {
					t.Fatal(err)
				}
				if !sameFailedObservation(after, before) {
					t.Fatal("stale probe overwrote new failure")
				}
			}
		})
	}
}

// The replica remains fixed until the test explicitly advances it. Conditional
// mutations always execute against Store, including when the read is stale.
type laggingTaskStore struct {
	kv.Store
	replica     kv.Store
	reads       []bool
	mutations   int
	reject      bool
	commitError error
}

func (s *laggingTaskStore) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	primary := kv.WithPrimaryReads(ctx) == ctx
	s.reads = append(s.reads, primary)
	if primary {
		return s.Store.Get(ctx, key)
	}
	return s.replica.Get(ctx, key)
}

func (s *laggingTaskStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	s.mutations++
	if s.reject {
		return false, nil
	}
	matched, err := s.Store.ApplyMutation(ctx, mutation)
	if err == nil && matched && s.commitError != nil {
		return false, s.commitError
	}
	return matched, err
}

func (s *laggingTaskStore) expectFallback(t *testing.T) {
	t.Helper()
	if len(s.reads) < 3 || s.reads[0] || !s.reads[len(s.reads)-1] {
		t.Fatalf("reads = %v, want ordinary first then primary fallback", s.reads)
	}
	s.reads = nil
}

func TestKVClaimTransitionsWithLaggingLearner(t *testing.T) {
	ctx := t.Context()
	writer, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	record, err := New(KindPeer, "lagging-peer", nil, ReasonPeerDelete, struct{}{}, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	for _, store := range []kv.Store{writer, replica} {
		if _, _, err := CreateOrGet(ctx, store, record); err != nil {
			t.Fatal(err)
		}
	}
	store := &laggingTaskStore{Store: writer, replica: replica}
	source := KVSource{Store: store, SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
	queued, err := source.GetTask(ctx, record.DeletionID)
	if err != nil {
		t.Fatal(err)
	}
	ref := Reference{Source: source.Name(), DeletionID: record.DeletionID, MarkerFingerprint: queued.MarkerFingerprint}
	store.reads = nil
	claim, ok, err := source.Claim(ctx, ref, time.Unix(2, 0), time.Minute)
	if err != nil || !ok {
		t.Fatalf("Claim = %v, %v", ok, err)
	}
	for _, primary := range store.reads {
		if primary {
			t.Fatal("initial claim bypassed learner")
		}
	}
	store.reads = nil
	if _, ok, err := source.Claim(ctx, ref, time.Unix(3, 0), time.Minute); err != nil || ok {
		t.Fatalf("duplicate claim stole live lease: %v, %v", ok, err)
	}
	store.expectFallback(t)
	if err := source.Renew(ctx, claim, time.Unix(3, 0), 2*time.Minute); err != nil {
		t.Fatal(err)
	}
	store.expectFallback(t)
	// Advance only to the original claim: its lease is valid but behind the
	// renewal, so the next transition must recover from a definite CAS miss.
	oldState, err := encodeKVTaskState(claim.Task)
	if err != nil {
		t.Fatal(err)
	}
	if err := replica.Set(ctx, kvTaskKey(record.DeletionID), oldState); err != nil {
		t.Fatal(err)
	}
	claim, err = source.Checkpoint(ctx, claim, PhaseFinalize, time.Unix(4, 0))
	if err != nil {
		t.Fatal(err)
	}
	store.expectFallback(t)
	if err := source.Finalize(ctx, claim, time.Unix(5, 0), nil); err != nil {
		t.Fatal(err)
	}
	store.expectFallback(t)
	if _, err := writer.Get(ctx, kvTaskKey(record.DeletionID)); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("finalized task still present: %v", err)
	}
}

func TestKVClaimRefreshesReferenceMissingFromLearner(t *testing.T) {
	ctx := t.Context()
	writer, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	record, err := New(KindPeer, "new-reference", nil, ReasonPeerDelete, struct{}{}, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := CreateOrGet(ctx, writer, record); err != nil {
		t.Fatal(err)
	}
	store := &laggingTaskStore{Store: writer, replica: replica}
	source := KVSource{Store: store, SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
	refs, _, err := source.ScanDue(kv.WithPrimaryReads(ctx), time.Unix(2, 0), 10, "")
	if err != nil || len(refs) != 1 {
		t.Fatalf("discovery=%v err=%v", refs, err)
	}
	store.reads = nil
	claim, claimed, err := source.Claim(ctx, refs[0], time.Unix(2, 0), time.Minute)
	if err != nil || !claimed || claim.Record.DeletionID != record.DeletionID {
		t.Fatalf("claim=%+v claimed=%v err=%v", claim, claimed, err)
	}
	store.expectFallback(t)
	if _, claimed, err := source.Claim(ctx, refs[0], time.Unix(3, 0), time.Minute); err != nil || claimed {
		t.Fatalf("reference stole a committed lease: claimed=%v err=%v", claimed, err)
	}
}

func TestExplicitRetryRefreshesLaggingTaskBeforeRejecting(t *testing.T) {
	for _, path := range []string{"source", "admin"} {
		for _, observation := range []string{"queued", "running", "retry_wait", "missing-record", "missing-state", "failed"} {
			t.Run(path+"/"+observation, func(t *testing.T) {
				ctx := t.Context()
				writer, replica := kv.NewMemory(nil), kv.NewMemory(nil)
				authority := KVSource{Store: writer, SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
				now := time.Unix(10, 0)
				failed := seedFailedRecoveryTask(t, authority, "explicit-retry", time.Unix(1, 0))
				if observation != "missing-record" {
					if _, _, err := CreateOrGet(ctx, replica, failed.Record); err != nil {
						t.Fatal(err)
					}
					if observation == "missing-state" {
						if err := replica.Delete(ctx, kvTaskKey(failed.Record.DeletionID)); err != nil {
							t.Fatal(err)
						}
					} else {
						stale := failed
						stale.Status = Status(observation)
						if stale.Status == StatusRunning {
							stale.LeaseToken, stale.LeaseDeadline = "old-lease", now.Add(time.Minute)
						}
						raw, err := encodeKVTaskState(stale)
						if err != nil {
							t.Fatal(err)
						}
						if err := replica.Set(ctx, kvTaskKey(failed.Record.DeletionID), raw); err != nil {
							t.Fatal(err)
						}
					}
				}
				store := &laggingTaskStore{Store: writer, replica: replica}
				source := KVSource{Store: store, SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
				registry := NewRegistry()
				if err := registry.Register(source, &registryTestHandler{kind: KindPeer}); err != nil {
					t.Fatal(err)
				}
				wakes := 0
				admin := NewAdmin(registry, func() { wakes++ })
				admin.now = func() time.Time { return now }
				retry := func() (Task, error) { return source.Retry(ctx, failed.Record.DeletionID, now) }
				if path == "admin" {
					retry = func() (Task, error) { return admin.Retry(ctx, source.Name(), failed.Record.DeletionID) }
				}
				retried, err := retry()
				if err != nil {
					t.Fatalf("Retry = %v", err)
				}
				if retried.Status != StatusQueued || retried.Phase != failed.Phase || retried.MarkerFingerprint != failed.MarkerFingerprint || retried.Record.DeletionID != failed.Record.DeletionID || retried.FailureCount != 0 || retried.LastErrorCode != "" || !retried.NextAttemptAt.Equal(now) {
					t.Fatalf("retried task = %+v", retried)
				}
				if store.mutations != 1 || wakes != map[string]int{"source": 0, "admin": 1}[path] {
					t.Fatalf("mutations=%d wakes=%d", store.mutations, wakes)
				}
				if observation == "failed" {
					for _, primary := range store.reads {
						if primary {
							t.Fatal("matching failed task bypassed reader")
						}
					}
				} else {
					store.expectFallback(t)
				}
				// An unchanged learner must not cause a second reset of the budget.
				if _, err := retry(); !errors.Is(err, ErrConflict) {
					t.Fatalf("repeated retry = %v", err)
				}
				// Ordinary Get still observes the learner after the mutation succeeds.
				store.reads = nil
				_, _ = admin.Get(ctx, source.Name(), failed.Record.DeletionID)
				for _, primary := range store.reads {
					if primary {
						t.Fatal("ordinary task query refreshed primary")
					}
				}
			})
		}
	}
}

func TestExplicitRetryPreservesMutationAttemptBounds(t *testing.T) {
	for _, path := range []string{"source", "admin"} {
		for _, outcome := range []string{"rejected", "uncertain"} {
			t.Run(path+"/"+outcome, func(t *testing.T) {
				ctx := t.Context()
				writer := kv.NewMemory(nil)
				authority := KVSource{Store: writer, SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
				failed := seedFailedRecoveryTask(t, authority, "retry-bounds", time.Unix(1, 0))
				store := &laggingTaskStore{Store: writer, replica: writer, reject: outcome == "rejected"}
				wantErr, wantMutations := ErrConflict, 4
				if outcome == "uncertain" {
					store.commitError = errors.New("retry commit response lost")
					wantErr, wantMutations = store.commitError, 1
				}
				source := KVSource{Store: store, SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
				registry := NewRegistry()
				if err := registry.Register(source, &registryTestHandler{kind: KindPeer}); err != nil {
					t.Fatal(err)
				}
				admin := NewAdmin(registry, func() { t.Error("unsuccessful retry woke processor") })
				retry := func() (Task, error) { return source.Retry(ctx, failed.Record.DeletionID, time.Unix(10, 0)) }
				if path == "admin" {
					retry = func() (Task, error) { return admin.Retry(ctx, source.Name(), failed.Record.DeletionID) }
				}
				if _, err := retry(); !errors.Is(err, wantErr) {
					t.Fatalf("Retry = %v, want %v", err, wantErr)
				}
				if store.mutations != wantMutations {
					t.Fatalf("retry mutations = %d, want %d", store.mutations, wantMutations)
				}
				current, err := authority.GetTask(ctx, failed.Record.DeletionID)
				if err != nil || current.MarkerFingerprint != failed.MarkerFingerprint || current.Phase != failed.Phase {
					t.Fatalf("retry changed identity or phase: %+v, %v", current, err)
				}
				if outcome == "rejected" && !sameFailedObservation(current, failed) || outcome == "uncertain" && current.Status != StatusQueued {
					t.Fatalf("unexpected task after %s retry: %+v", outcome, current)
				}
			})
		}
	}
}

func TestKVTaskDoesNotReplayUncertainTransition(t *testing.T) {
	ctx := t.Context()
	writer := kv.NewMemory(nil)
	record, err := New(KindPeer, "uncertain-peer", nil, ReasonPeerDelete, struct{}{}, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := CreateOrGet(ctx, writer, record); err != nil {
		t.Fatal(err)
	}
	store := &laggingTaskStore{Store: writer, replica: writer}
	source := KVSource{Store: store, SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
	task, err := source.GetTask(ctx, record.DeletionID)
	if err != nil {
		t.Fatal(err)
	}
	claim, ok, err := source.Claim(ctx, Reference{Source: "peer", DeletionID: record.DeletionID, MarkerFingerprint: task.MarkerFingerprint}, time.Unix(2, 0), time.Minute)
	if err != nil || !ok {
		t.Fatalf("Claim = %v, %v", ok, err)
	}
	store.commitError = errors.New("commit reply lost")
	store.mutations = 0
	if err := source.Fail(ctx, claim, "temporary", "temporary", false, time.Unix(5, 0), time.Unix(3, 0), 3); !errors.Is(err, store.commitError) {
		t.Fatalf("Fail = %v", err)
	}
	if store.mutations != 1 {
		t.Fatalf("uncertain commit replayed %d times", store.mutations)
	}
}
