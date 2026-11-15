//go:build store_e2e

package store_e2e_test

import (
	"context"
	"crypto/rand"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/tikv/client-go/v2/oracle"
	"github.com/tikv/client-go/v2/tikv"
)

type localWriteReadClock struct {
	oracle.Oracle
	frozen atomic.Bool
	cached uint64
}

func (o *localWriteReadClock) GetLowResolutionTimestamp(ctx context.Context, option *oracle.Option) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if o.frozen.Load() {
		return o.cached, nil
	}
	return o.Oracle.GetLowResolutionTimestamp(ctx, option)
}

// freezeTiKVReadClock retains the reader's real cached PD timestamp while
// leaving native Learner synchronization, fresh writer timestamps and SDK
// validation unchanged. It does not pause Raft or alter any business response.
func freezeTiKVReadClock(t *testing.T, client *tikv.KVStore) *localWriteReadClock {
	t.Helper()
	original := client.GetOracle()
	cached, err := original.GetLowResolutionTimestamp(t.Context(), &oracle.Option{TxnScope: oracle.GlobalTxnScope})
	if err != nil || cached == 0 {
		t.Fatalf("cached PD timestamp unavailable: %d, %v", cached, err)
	}
	clock := &localWriteReadClock{Oracle: original, cached: cached}
	clock.frozen.Store(true)
	client.SetOracle(clock)
	t.Cleanup(func() { client.SetOracle(original) })
	return clock
}

// Keep the actual ordinary timestamp fixed before every mutation, even when
// Raft has already replicated it. The real transactional client must publish
// only committed postimages and later hand reads back to the native snapshot.
func testTiKVLocalWrites(t *testing.T, client *tikv.KVStore) {
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	clock := freezeTiKVReadClock(t, client)
	options := kv.TiKVReadOptions{Labels: map[string]string{"region": "local"}}
	root, err := kv.NewTiKVWithClient(client, options, nil)
	if err != nil {
		t.Fatal(err)
	}
	otherRoot, err := kv.NewTiKVWithClient(client, options, nil)
	if err != nil {
		t.Fatal(err)
	}
	prefix := kv.Key{"local-writes-" + rand.Text()}
	store, other := kv.Prefixed(root, prefix), kv.Prefixed(otherRoot, prefix)
	key, index, members, ordered, expiring := kv.Key{"peer"}, kv.Key{"index"}, kv.Key{"members"}, kv.Key{"ordered"}, kv.Key{"expiry"}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = store.BatchDelete(cleanup, []kv.Key{key, index, members, ordered, expiring, {"guard"}, {"forbidden"}})
	})
	for _, value := range []string{"created", "updated"} {
		if err := store.Set(ctx, key, []byte(value)); err != nil {
			t.Fatal(err)
		}
		if got, err := store.Get(ctx, key); err != nil || string(got) != value {
			t.Fatalf("immediate read after %s = %q, %v", value, got, err)
		}
	}
	if _, err := other.Get(ctx, key); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("another instance gained the local postimage: %v", err)
	}
	matched, err := store.ApplyMutation(ctx, kv.Mutation{
		Conditions:        []kv.Condition{{Key: key, Expected: []byte("updated")}},
		Entries:           []kv.Entry{{Key: key, Value: []byte("mutated")}, {Key: index, Value: []byte("indexed")}},
		AddMembers:        []kv.SetMembers{{Key: members, Members: []string{"a", "b"}}},
		AddOrderedMembers: []kv.SetMembers{{Key: ordered, Members: []string{"a", "b", "c", "d"}}},
	})
	if err != nil || !matched {
		t.Fatalf("atomic mutation = %v, %v", matched, err)
	}
	results := store.BatchGet(ctx, []kv.Key{key, index, key, members})
	if string(results[0].Value) != "mutated" || string(results[1].Value) != "indexed" || string(results[2].Value) != "mutated" || !errors.Is(results[3].Err, kv.ErrNotFound) {
		t.Fatalf("immediate batch = %+v", results)
	}
	results[0].Value[0] = 'X'
	if string(results[2].Value) != "mutated" {
		t.Fatal("duplicate batch values share memory")
	}
	if matched, err := kv.CompareAndMutate(ctx, store, key, []byte("wrong"), []kv.Entry{{Key: key, Value: []byte("forbidden")}}, nil); err != nil || matched {
		t.Fatalf("failed CAS = %v, %v", matched, err)
	}
	if got, err := store.Get(ctx, key); err != nil || string(got) != "mutated" {
		t.Fatalf("rejected mutation poisoned cache: %q, %v", got, err)
	}
	if _, created, err := kv.CreateIfAbsent(ctx, store, kv.Entry{Key: kv.Key{"guard"}, Value: []byte("guard")}, nil); err != nil || !created {
		t.Fatalf("conditional create = %v, %v", created, err)
	}
	if _, created, err := kv.CreateIfAbsent(ctx, store, kv.Entry{Key: kv.Key{"guard"}, Value: []byte("replaced")}, []kv.Entry{{Key: kv.Key{"forbidden"}, Value: []byte("not committed")}}); err != nil || created {
		t.Fatalf("conditional conflict = %v, %v", created, err)
	}
	if _, err := store.Get(ctx, kv.Key{"forbidden"}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("conditional conflict published attempted value: %v", err)
	}
	if err := store.RemoveMembers(ctx, members, "a"); err != nil {
		t.Fatal(err)
	}
	if found, err := store.HasMember(ctx, members, "a"); err != nil || found {
		t.Fatalf("removed member = %v, %v", found, err)
	}
	if got, err := store.ListMembers(ctx, members); err != nil || !slices.Equal(got, []string{"b"}) {
		t.Fatalf("member list = %v, %v", got, err)
	}
	if matched, err := store.ApplyMutation(ctx, kv.Mutation{RemoveOrderedMembers: []kv.SetMembers{{Key: ordered, Members: []string{"a", "b"}}}}); err != nil || !matched {
		t.Fatalf("ordered removal = %v, %v", matched, err)
	}
	if got, err := store.RangeOrderedMembers(ctx, ordered, kv.OrderedRange{Limit: 2}); err != nil || !slices.Equal(got, []string{"c", "d"}) {
		t.Fatalf("range after leading removals = %v, %v", got, err)
	}
	if err := store.Delete(ctx, members); err != nil {
		t.Fatal(err)
	}
	if err := store.AddMembers(ctx, members, "new"); err != nil {
		t.Fatal(err)
	}
	if got, err := store.ListMembers(ctx, members); err != nil || !slices.Equal(got, []string{"new"}) {
		t.Fatalf("collection recreation = %v, %v", got, err)
	}
	if err := store.BatchSet(ctx, []kv.Entry{{Key: expiring, Value: []byte("short lived"), Deadline: time.Now().Add(100 * time.Millisecond)}}); err != nil {
		t.Fatal(err)
	}
	timer := time.NewTimer(120 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-timer.C:
	}
	if _, err := store.Get(ctx, expiring); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("cached expiry = %v", err)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, key); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("immediate delete = %v", err)
	}
	// A remote replacement becomes visible only once the ordinary snapshot
	// actually covers it. Explicit primary diagnostics bypass local postimages.
	if err := other.Set(ctx, key, []byte("external")); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Get(kv.WithPrimaryReads(ctx), key); err != nil || string(got) != "external" {
		t.Fatalf("primary read did not bypass local deletion: %q, %v", got, err)
	}
	if _, err := store.Get(ctx, key); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("historical snapshot prematurely replaced local deletion: %v", err)
	}
	clock.frozen.Store(false)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		got, err := store.Get(ctx, key)
		if err == nil && string(got) == "external" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("local deletion did not hand back to the advanced snapshot: %q, %v", got, err)
		case <-ticker.C:
		}
	}
}
