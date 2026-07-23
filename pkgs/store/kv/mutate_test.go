package kv_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

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
					existing, created, err := store.CreateIfAbsent(ctx, guard, entries)
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
