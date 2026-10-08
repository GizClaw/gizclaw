package socialutil

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type recoveryReadStore struct {
	kv.Store
	reads int
}

func TestRecoveryContinuesAfterTimedOutIntentAndRetainsItForRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := kv.NewMemory(nil)
		defer store.Close()
		index := RecoveryIndex{Root: kv.Key{"retirement"}}
		for _, id := range []string{"a", "b", "c"} {
			if _, err := store.ApplyMutation(t.Context(), kv.Mutation{
				Entries: []kv.Entry{{Key: index.recordKey(id), Value: []byte(id)}}, AddMembers: index.Add(id),
			}); err != nil {
				t.Fatal(err)
			}
		}
		var ids []string
		for id, err := range index.IDs(t.Context(), store) {
			if err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		var attempted []string
		started := time.Now()
		err := index.Reconcile(t.Context(), store, func(ctx context.Context, id string) error {
			attempted = append(attempted, id)
			if id == ids[0] {
				<-ctx.Done()
				return ctx.Err()
			}
			_, err := store.ApplyMutation(ctx, kv.Mutation{
				DeleteKeys: []kv.Key{index.recordKey(id)}, RemoveMembers: index.Remove(id),
			})
			return err
		})
		if !errors.Is(err, context.DeadlineExceeded) || !slices.Equal(attempted, ids) {
			t.Fatalf("attempted = %v, error = %v", attempted, err)
		}
		if time.Since(started) != recoveryAttemptTimeout {
			t.Fatal("recovery did not bound the slow attempt")
		}
		var remaining []string
		for id, err := range index.IDs(t.Context(), store) {
			if err != nil {
				t.Fatal(err)
			}
			remaining = append(remaining, id)
		}
		if !slices.Equal(remaining, ids[:1]) {
			t.Fatalf("remaining indexed work = %v, want failed identity only", remaining)
		}
	})
}

func TestRecoveryCancellationStopsBeforeNextIntent(t *testing.T) {
	store := kv.NewMemory(nil)
	defer store.Close()
	index := RecoveryIndex{Root: kv.Key{"retirement"}}
	for _, id := range []string{"a", "b", "c"} {
		if _, err := store.ApplyMutation(t.Context(), kv.Mutation{
			Entries: []kv.Entry{{Key: index.recordKey(id), Value: []byte(id)}}, AddMembers: index.Add(id),
		}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	attempts := 0
	err := index.Reconcile(ctx, store, func(context.Context, string) error {
		attempts++
		cancel()
		return context.Canceled
	})
	if !errors.Is(err, context.Canceled) || attempts != 1 {
		t.Fatalf("attempts = %d, error = %v", attempts, err)
	}
}

func TestRecoveryContinuesPastInvalidBucketAndMember(t *testing.T) {
	store := kv.NewMemory(nil)
	defer store.Close()
	index := RecoveryIndex{Root: kv.Key{"retirement"}}
	if _, err := store.ApplyMutation(t.Context(), kv.Mutation{
		Entries: []kv.Entry{{Key: index.recordKey("valid"), Value: []byte("valid")}},
		AddMembers: append(index.Add("valid"),
			kv.SetMembers{Key: index.directory(), Members: []string{"invalid-bucket"}},
			kv.SetMembers{Key: index.bucket(recoveryBucket("valid")), Members: []string{""}},
		),
	}); err != nil {
		t.Fatal(err)
	}
	var attempted []string
	err := index.Reconcile(t.Context(), store, func(_ context.Context, id string) error {
		attempted = append(attempted, id)
		return nil
	})
	if err != nil || !slices.Equal(attempted, []string{"valid"}) {
		t.Fatalf("attempted = %v, error = %v", attempted, err)
	}
	if present, err := store.HasMember(t.Context(), index.directory(), "invalid-bucket"); err != nil || present {
		t.Fatalf("invalid directory entry remains: %v, %v", present, err)
	}
	if present, err := store.HasMember(t.Context(), index.bucket(recoveryBucket("valid")), ""); err != nil || present {
		t.Fatalf("empty member remains: %v, %v", present, err)
	}
}

func (s *recoveryReadStore) ListMembers(ctx context.Context, key kv.Key) ([]string, error) {
	s.reads++
	return s.Store.ListMembers(ctx, key)
}

func TestRecoveryIndexPartitionsPendingWorkAndRemovesCompletedIDs(t *testing.T) {
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	index := RecoveryIndex{Root: kv.Key{"friend-retirement"}}
	const total = 4096
	for i := range total {
		id := fmt.Sprintf("relation-%d", i)
		ok, err := store.ApplyMutation(t.Context(), kv.Mutation{
			Conditions: []kv.Condition{{Key: kv.Key{"records", id}}},
			Entries:    []kv.Entry{{Key: kv.Key{"records", id}, Value: []byte(id)}}, AddMembers: index.Add(id),
		})
		if err != nil || !ok {
			t.Fatalf("publish: %v, %v", ok, err)
		}
	}
	// A failed publication must not create a phantom recovery entry.
	ok, err := store.ApplyMutation(t.Context(), kv.Mutation{
		Conditions: []kv.Condition{{Key: kv.Key{"records", "relation-0"}}}, AddMembers: index.Add("phantom"),
	})
	if err != nil || ok {
		t.Fatalf("conflicting publication: %v, %v", ok, err)
	}
	for i := range total / 2 {
		id := fmt.Sprintf("relation-%d", i)
		ok, err := store.ApplyMutation(t.Context(), kv.Mutation{
			Conditions: []kv.Condition{{Key: kv.Key{"records", id}, Expected: []byte(id)}},
			DeleteKeys: []kv.Key{{"records", id}}, RemoveMembers: index.Remove(id),
		})
		if err != nil || !ok {
			t.Fatalf("complete: %v, %v", ok, err)
		}
	}
	reader := &recoveryReadStore{Store: store}
	seen := make(map[string]bool)
	for id, err := range index.IDs(t.Context(), reader) {
		if err != nil {
			t.Fatal(err)
		}
		if seen[id] {
			t.Fatalf("duplicate recovery ID %q", id)
		}
		seen[id] = true
	}
	if len(seen) != total/2 || seen["phantom"] {
		t.Fatalf("pending IDs = %d, phantom = %v", len(seen), seen["phantom"])
	}
	for i := total / 2; i < total; i++ {
		if !seen[fmt.Sprintf("relation-%d", i)] {
			t.Fatalf("missing pending ID %d", i)
		}
	}
	if reader.reads > 257 {
		t.Fatalf("set reads = %d; directory plus at most 256 buckets", reader.reads)
	}
	other := RecoveryIndex{Root: kv.Key{"group-retirement"}}
	for id, err := range other.IDs(t.Context(), store) {
		t.Fatalf("foreign work kind returned %q, %v", id, err)
	}
}

func TestRecoveryIndexRejectsMalformedDirectory(t *testing.T) {
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	index := RecoveryIndex{Root: kv.Key{"recovery"}}
	if err := store.AddMembers(t.Context(), index.directory(), "unexpected/key"); err != nil {
		t.Fatal(err)
	}
	var failed bool
	for _, err := range index.IDs(t.Context(), store) {
		failed = err != nil
	}
	if !failed {
		t.Fatal("malformed recovery directory accepted")
	}
}

func TestRecoveryIndexDoesNotShareWorkRecordNamespace(t *testing.T) {
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	index := RecoveryIndex{Root: kv.Key{"social-retirement-intents", "friend-groups"}}
	id := "recovery-buckets"
	key := append(append(kv.Key{}, index.Root...), id)
	ok, err := store.ApplyMutation(t.Context(), kv.Mutation{
		Entries: []kv.Entry{{Key: key, Value: []byte("record")}}, AddMembers: index.Add(id),
	})
	if err != nil || !ok {
		t.Fatalf("publish record and index: %v, %v", ok, err)
	}
	data, err := store.Get(t.Context(), key)
	if err != nil || string(data) != "record" {
		t.Fatalf("record = %q, %v", data, err)
	}
	count := 0
	for got, err := range index.IDs(t.Context(), store) {
		if err != nil || got != id {
			t.Fatalf("indexed ID = %q, %v", got, err)
		}
		count++
	}
	if count != 1 {
		t.Fatalf("indexed count = %d", count)
	}
}
