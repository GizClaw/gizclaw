package socialutil

import (
	"context"
	"fmt"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type recoveryReadStore struct {
	kv.Store
	reads int
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
