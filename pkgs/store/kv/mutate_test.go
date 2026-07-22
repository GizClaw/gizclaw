package kv_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

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
