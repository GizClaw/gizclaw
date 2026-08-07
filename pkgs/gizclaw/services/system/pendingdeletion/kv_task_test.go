package pendingdeletion

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

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
