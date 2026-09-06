package pendingdeletion

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

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
			deferred, err := source.GetTask(ctx, record.DeletionID)
			if err != nil || deferred.Status != StatusRetryWait || deferred.FailureCount != 0 {
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
			task, err := source.GetTask(ctx, record.DeletionID)
			if err != nil || task.Status != StatusRetryWait || task.FailureCount != 1 {
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
			claim, err = source.Checkpoint(ctx, claim, PhaseFinalize, time.Unix(6, 0))
			if err != nil || claim.Phase != PhaseFinalize {
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
			if err != nil || task.Status != StatusQueued || task.Phase != PhaseFinalize || task.FailureCount != 0 || task.LastErrorCode != "" {
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
