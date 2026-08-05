package kv_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type storeWithoutCreateIfAbsent struct {
	kv.Store
}

func TestSupportsCreateIfAbsent(t *testing.T) {
	supported := kv.NewMemory(nil)
	unsupported := storeWithoutCreateIfAbsent{Store: supported}
	for _, tc := range []struct {
		name  string
		store kv.Store
		want  bool
	}{
		{name: "supported", store: supported, want: true},
		{name: "unsupported", store: unsupported},
		{name: "prefixed supported", store: kv.Prefixed(supported, kv.Key{"supported"}), want: true},
		{name: "prefixed unsupported", store: kv.Prefixed(unsupported, kv.Key{"unsupported"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := kv.SupportsCreateIfAbsent(tc.store); got != tc.want {
				t.Fatalf("SupportsCreateIfAbsent() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSupportsCreateIfAllAbsent(t *testing.T) {
	supported := kv.NewMemory(nil)
	unsupported := storeWithoutCreateIfAbsent{Store: supported}
	for _, tc := range []struct {
		name  string
		store kv.Store
		want  bool
	}{
		{name: "supported", store: supported, want: true},
		{name: "unsupported", store: unsupported},
		{name: "prefixed supported", store: kv.Prefixed(supported, kv.Key{"supported"}), want: true},
		{name: "prefixed unsupported", store: kv.Prefixed(unsupported, kv.Key{"unsupported"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := kv.SupportsCreateIfAllAbsent(tc.store); got != tc.want {
				t.Fatalf("SupportsCreateIfAllAbsent() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSupportsCompareAndMutate(t *testing.T) {
	supported := kv.NewMemory(nil)
	unsupported := storeWithoutCreateIfAbsent{Store: supported}
	for _, tc := range []struct {
		name  string
		store kv.Store
		want  bool
	}{
		{name: "supported", store: supported, want: true},
		{name: "unsupported", store: unsupported},
		{name: "prefixed supported", store: kv.Prefixed(supported, kv.Key{"supported"}), want: true},
		{name: "prefixed unsupported", store: kv.Prefixed(unsupported, kv.Key{"unsupported"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := kv.SupportsCompareAndMutate(tc.store); got != tc.want {
				t.Fatalf("SupportsCompareAndMutate() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCreateIfAbsentRejectsUnsupportedStore(t *testing.T) {
	store := storeWithoutCreateIfAbsent{Store: kv.NewMemory(nil)}
	var _ kv.Store = store
	_, created, err := kv.CreateIfAbsent(
		context.Background(),
		store,
		kv.Entry{Key: kv.Key{"guard"}, Value: []byte("guard")},
		nil,
	)
	if !errors.Is(err, kv.ErrCreateIfAbsentUnsupported) || created {
		t.Fatalf("CreateIfAbsent() = (_, %v, %v), want unsupported error", created, err)
	}
}

func TestCreateIfAbsentCreatesOneAtomicRecord(t *testing.T) {
	for _, fixture := range []struct {
		name string
		new  func(*testing.T) kv.Store
	}{
		{name: "memory", new: func(*testing.T) kv.Store { return kv.NewMemory(nil) }},
		{name: "badger", new: func(t *testing.T) kv.Store { return newTestStore(t, nil) }},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			store := fixture.new(t)
			ctx := context.Background()
			guard := kv.Entry{Key: kv.Key{"pending", "resource"}, Value: []byte("winner")}
			entries := []kv.Entry{{Key: kv.Key{"records", "winner"}, Value: []byte("record")}}

			const callers = 16
			start := make(chan struct{})
			results := make(chan struct {
				existing string
				created  bool
				err      error
			}, callers)
			var group sync.WaitGroup
			for range callers {
				group.Go(func() {
					<-start
					existing, created, err := kv.CreateIfAbsent(ctx, store, guard, entries)
					results <- struct {
						existing string
						created  bool
						err      error
					}{existing: string(existing), created: created, err: err}
				})
			}
			close(start)
			group.Wait()
			close(results)

			created := 0
			for result := range results {
				if result.err != nil {
					t.Fatalf("CreateIfAbsent() error = %v", result.err)
				}
				if result.created {
					created++
					continue
				}
				if result.existing != "winner" {
					t.Fatalf("CreateIfAbsent() existing = %q, want winner", result.existing)
				}
			}
			if created != 1 {
				t.Fatalf("CreateIfAbsent() creators = %d, want 1", created)
			}
			if value, err := store.Get(ctx, guard.Key); err != nil || string(value) != "winner" {
				t.Fatalf("Get(guard) = %q, %v", value, err)
			}
			if value, err := store.Get(ctx, entries[0].Key); err != nil || string(value) != "record" {
				t.Fatalf("Get(record) = %q, %v", value, err)
			}
		})
	}
}

func TestCreateIfAbsentExistingGuardSkipsWriteValidation(t *testing.T) {
	for _, fixture := range []struct {
		name string
		new  func(*testing.T) kv.Store
	}{
		{name: "memory", new: func(*testing.T) kv.Store { return kv.NewMemory(nil) }},
		{name: "badger", new: func(t *testing.T) kv.Store { return newTestStore(t, nil) }},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			store := fixture.new(t)
			ctx := context.Background()
			guardKey := kv.Key{"pending", "resource"}
			if err := store.Set(ctx, guardKey, []byte("existing")); err != nil {
				t.Fatalf("seed guard: %v", err)
			}
			expired := time.Now().Add(-time.Second)
			extraKey := kv.Key{"records", "new"}
			existing, created, err := kv.CreateIfAbsent(
				ctx,
				store,
				kv.Entry{Key: guardKey, Value: []byte("replacement"), Deadline: expired},
				[]kv.Entry{{Key: extraKey, Value: []byte("new"), Deadline: expired}},
			)
			if err != nil {
				t.Fatalf("CreateIfAbsent() error = %v", err)
			}
			if created {
				t.Fatal("CreateIfAbsent() created = true, want false")
			}
			if string(existing) != "existing" {
				t.Fatalf("CreateIfAbsent() existing = %q, want existing", existing)
			}
			if value, err := store.Get(ctx, guardKey); err != nil || string(value) != "existing" {
				t.Fatalf("Get(guard) = %q, %v", value, err)
			}
			if _, err := store.Get(ctx, extraKey); !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("Get(extra) error = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestCreateIfAbsentGuardWinsEntryCollision(t *testing.T) {
	for _, fixture := range []struct {
		name string
		new  func(*testing.T) kv.Store
	}{
		{name: "memory", new: func(*testing.T) kv.Store { return kv.NewMemory(nil) }},
		{name: "badger", new: func(t *testing.T) kv.Store { return newTestStore(t, nil) }},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			store := fixture.new(t)
			ctx := context.Background()
			guard := kv.Entry{Key: kv.Key{"pending", "resource"}, Value: []byte("guard")}
			existing, created, err := kv.CreateIfAbsent(
				ctx,
				store,
				guard,
				[]kv.Entry{{Key: guard.Key, Value: []byte("entry")}},
			)
			if err != nil {
				t.Fatalf("CreateIfAbsent() error = %v", err)
			}
			if !created {
				t.Fatalf("CreateIfAbsent() = (%q, false), want created", existing)
			}
			if value, err := store.Get(ctx, guard.Key); err != nil || string(value) != "guard" {
				t.Fatalf("Get(guard) = %q, %v", value, err)
			}
		})
	}
}

func TestCreateIfAllAbsentClaimsEveryGuardAtomically(t *testing.T) {
	for _, fixture := range []struct {
		name string
		new  func(*testing.T) kv.Store
	}{
		{name: "memory", new: func(*testing.T) kv.Store { return kv.NewMemory(nil) }},
		{name: "badger", new: func(t *testing.T) kv.Store { return newTestStore(t, nil) }},
		{
			name: "prefixed memory",
			new: func(*testing.T) kv.Store {
				return kv.Prefixed(kv.NewMemory(nil), kv.Key{"scope"})
			},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			store := fixture.new(t)
			ctx := t.Context()
			shared := kv.Entry{Key: kv.Key{"by-invoke-name", "shared"}, Value: []byte("winner")}
			_, _, created, err := kv.CreateIfAllAbsent(ctx, store, []kv.Entry{
				{Key: kv.Key{"by-id", "winner"}, Value: []byte("record")},
				shared,
			}, nil)
			if err != nil || !created {
				t.Fatalf("CreateIfAllAbsent(first) = (_, _, %v, %v), want created", created, err)
			}

			loserID := kv.Key{"by-id", "loser"}
			conflict, existing, created, err := kv.CreateIfAllAbsent(ctx, store, []kv.Entry{
				{Key: loserID, Value: []byte("loser-record")},
				{Key: shared.Key, Value: []byte("loser")},
			}, []kv.Entry{{Key: kv.Key{"side-effect"}, Value: []byte("loser")}})
			if err != nil || created {
				t.Fatalf("CreateIfAllAbsent(second) = (%v, %q, %v, %v), want conflict", conflict, existing, created, err)
			}
			if !slices.Equal(conflict, shared.Key) || string(existing) != "winner" {
				t.Fatalf("CreateIfAllAbsent conflict = (%v, %q), want (%v, winner)", conflict, existing, shared.Key)
			}
			if _, err := store.Get(ctx, loserID); !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("loser ID error = %v, want not found", err)
			}
			if _, err := store.Get(ctx, kv.Key{"side-effect"}); !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("side effect error = %v, want not found", err)
			}
		})
	}
}

func TestCompareAndMutateRequiresExactGuardValue(t *testing.T) {
	for _, fixture := range []struct {
		name string
		new  func(*testing.T) kv.Store
	}{
		{name: "memory", new: func(*testing.T) kv.Store { return kv.NewMemory(nil) }},
		{name: "badger", new: func(t *testing.T) kv.Store { return newTestStore(t, nil) }},
		{
			name: "prefixed memory",
			new: func(*testing.T) kv.Store {
				return kv.Prefixed(kv.NewMemory(nil), kv.Key{"scope"})
			},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			store := fixture.new(t)
			ctx := context.Background()
			guard := kv.Key{"pending", "resource"}
			record := kv.Key{"records", "winner"}
			if err := store.Set(ctx, guard, []byte("incarnation-b")); err != nil {
				t.Fatalf("seed guard: %v", err)
			}
			matched, err := kv.CompareAndMutate(
				ctx,
				store,
				guard,
				[]byte("incarnation-a"),
				[]kv.Entry{{
					Key:      record,
					Value:    []byte("wrong"),
					Deadline: time.Now().Add(-time.Second),
				}},
				[]kv.Key{guard},
			)
			if err != nil || matched {
				t.Fatalf("CompareAndMutate(stale) = %v, %v; want false, nil", matched, err)
			}
			if value, err := store.Get(ctx, guard); err != nil ||
				string(value) != "incarnation-b" {
				t.Fatalf("Get(guard) after stale compare = %q, %v", value, err)
			}
			matched, err = kv.CompareAndMutate(
				ctx,
				store,
				guard,
				[]byte("incarnation-b"),
				[]kv.Entry{{Key: record, Value: []byte("winner")}},
				[]kv.Key{guard},
			)
			if err != nil || !matched {
				t.Fatalf("CompareAndMutate(current) = %v, %v; want true, nil", matched, err)
			}
			if _, err := store.Get(ctx, guard); !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("Get(guard) error = %v, want ErrNotFound", err)
			}
			if value, err := store.Get(ctx, record); err != nil ||
				string(value) != "winner" {
				t.Fatalf("Get(record) = %q, %v", value, err)
			}
		})
	}
}

func TestBatchMutateSetAndDeleteAtomically(t *testing.T) {
	for _, fixture := range []struct {
		name string
		new  func(*testing.T) kv.Store
	}{
		{name: "memory", new: func(*testing.T) kv.Store { return kv.NewMemory(nil) }},
		{name: "badger", new: func(t *testing.T) kv.Store { return newTestStore(t, nil) }},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			store := fixture.new(t)
			ctx := context.Background()
			active := kv.Key{"active", "resource"}
			pending := kv.Key{"pending", "deletion"}
			if err := store.Set(ctx, active, []byte("active")); err != nil {
				t.Fatalf("seed active: %v", err)
			}
			if err := store.BatchMutate(ctx, []kv.Entry{{Key: pending, Value: []byte("pending")}}, []kv.Key{active}); err != nil {
				t.Fatalf("BatchMutate: %v", err)
			}
			if _, err := store.Get(ctx, active); !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("active Get error = %v, want ErrNotFound", err)
			}
			if value, err := store.Get(ctx, pending); err != nil || string(value) != "pending" {
				t.Fatalf("pending Get = %q, error = %v", value, err)
			}
		})
	}
}

func TestBatchMutateValidationFailureLeavesStoreUnchanged(t *testing.T) {
	for _, fixture := range []struct {
		name string
		new  func(*testing.T) kv.Store
	}{
		{name: "memory", new: func(*testing.T) kv.Store { return kv.NewMemory(nil) }},
		{name: "badger", new: func(t *testing.T) kv.Store { return newTestStore(t, nil) }},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			store := fixture.new(t)
			ctx := context.Background()
			active := kv.Key{"active", "resource"}
			pending := kv.Key{"pending", "deletion"}
			if err := store.Set(ctx, active, []byte("active")); err != nil {
				t.Fatalf("seed active: %v", err)
			}
			err := store.BatchMutate(ctx, []kv.Entry{{Key: pending, Value: []byte("pending"), Deadline: time.Now().Add(-time.Second)}}, []kv.Key{active})
			if !errors.Is(err, kv.ErrInvalidDeadline) {
				t.Fatalf("BatchMutate error = %v, want ErrInvalidDeadline", err)
			}
			if value, err := store.Get(ctx, active); err != nil || string(value) != "active" {
				t.Fatalf("active Get = %q, error = %v", value, err)
			}
			if _, err := store.Get(ctx, pending); !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("pending Get error = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestBatchMutateCanceledContextLeavesStoreUnchanged(t *testing.T) {
	for _, fixture := range []struct {
		name string
		new  func(*testing.T) kv.Store
	}{
		{name: "memory", new: func(*testing.T) kv.Store { return kv.NewMemory(nil) }},
		{name: "badger", new: func(t *testing.T) kv.Store { return newTestStore(t, nil) }},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			store := fixture.new(t)
			active := kv.Key{"active", "resource"}
			pending := kv.Key{"pending", "deletion"}
			if err := store.Set(context.Background(), active, []byte("active")); err != nil {
				t.Fatalf("seed active: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := store.BatchMutate(ctx, []kv.Entry{{Key: pending, Value: []byte("pending")}}, []kv.Key{active}); !errors.Is(err, context.Canceled) {
				t.Fatalf("BatchMutate error = %v, want context.Canceled", err)
			}
			if value, err := store.Get(context.Background(), active); err != nil || string(value) != "active" {
				t.Fatalf("active Get = %q, error = %v", value, err)
			}
			if _, err := store.Get(context.Background(), pending); !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("pending Get error = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestPrefixedBatchMutateStaysInsidePrefix(t *testing.T) {
	ctx := context.Background()
	base := kv.NewMemory(nil)
	store := kv.Prefixed(base, kv.Key{"domain"})
	if err := store.Set(ctx, kv.Key{"active"}, []byte("active")); err != nil {
		t.Fatalf("seed active: %v", err)
	}
	if err := store.BatchMutate(ctx, []kv.Entry{{Key: kv.Key{"pending"}, Value: []byte("pending")}}, []kv.Key{{"active"}}); err != nil {
		t.Fatalf("BatchMutate: %v", err)
	}
	if _, err := base.Get(ctx, kv.Key{"domain", "active"}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("prefixed active Get error = %v", err)
	}
	if value, err := base.Get(ctx, kv.Key{"domain", "pending"}); err != nil || string(value) != "pending" {
		t.Fatalf("prefixed pending Get = %q, error = %v", value, err)
	}
}
