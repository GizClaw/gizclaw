package kv_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestPrefixedStoreScopesOperations(t *testing.T) {
	ctx := context.Background()
	base := kv.NewMemory(nil)
	store := kv.Prefixed(base, kv.Key{"service", "credentials"})
	peer := kv.Prefixed(base, kv.Key{"service", "workspace"})

	if err := store.Set(ctx, kv.Key{"tenants", "mini-max"}, []byte("secret")); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := store.Get(ctx, kv.Key{"tenants", "mini-max"})
	if err != nil {
		t.Fatalf("Get through prefixed store: %v", err)
	}
	if string(got) != "secret" {
		t.Fatalf("Get through prefixed store = %q, want %q", got, "secret")
	}

	got, err = base.Get(ctx, kv.Key{"service", "credentials", "tenants", "mini-max"})
	if err != nil {
		t.Fatalf("Get through base store: %v", err)
	}
	if string(got) != "secret" {
		t.Fatalf("Get through base store = %q, want %q", got, "secret")
	}

	_, err = base.Get(ctx, kv.Key{"tenants", "mini-max"})
	if !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("unprefixed base key should not exist, got %v", err)
	}

	_, err = peer.Get(ctx, kv.Key{"tenants", "mini-max"})
	if !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("peer prefixed store should not see key, got %v", err)
	}

	if err := base.Set(ctx, kv.Key{"service", "credentials", "unrelated"}, []byte("kept")); err != nil {
		t.Fatalf("Set unrelated through base: %v", err)
	}
	if err := store.Delete(ctx, kv.Key{"tenants", "mini-max"}); err != nil {
		t.Fatalf("Delete through prefixed store: %v", err)
	}
	_, err = base.Get(ctx, kv.Key{"service", "credentials", "tenants", "mini-max"})
	if !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("prefixed global key should be deleted, got %v", err)
	}
	if _, err := base.Get(ctx, kv.Key{"service", "credentials", "unrelated"}); err != nil {
		t.Fatalf("unrelated key should remain: %v", err)
	}
}

func TestPrefixedStoreBatchOperations(t *testing.T) {
	ctx := context.Background()
	base := kv.NewMemory(nil)
	store := kv.Prefixed(base, kv.Key{"service", "mmx"})

	if err := store.BatchSet(ctx, []kv.Entry{
		{Key: kv.Key{"tenants", "a"}, Value: []byte("a")},
		{Key: kv.Key{"tenants", "b"}, Value: []byte("b")},
	}); err != nil {
		t.Fatalf("BatchSet: %v", err)
	}
	if _, err := base.Get(ctx, kv.Key{"service", "mmx", "tenants", "a"}); err != nil {
		t.Fatalf("Get tenants/a through base: %v", err)
	}
	if _, err := base.Get(ctx, kv.Key{"service", "mmx", "tenants", "b"}); err != nil {
		t.Fatalf("Get tenants/b through base: %v", err)
	}

	if err := store.BatchDelete(ctx, []kv.Key{{"tenants", "a"}, {"tenants", "b"}}); err != nil {
		t.Fatalf("BatchDelete: %v", err)
	}
	for _, key := range []kv.Key{
		{"service", "mmx", "tenants", "a"},
		{"service", "mmx", "tenants", "b"},
	} {
		if _, err := base.Get(ctx, key); !errors.Is(err, kv.ErrNotFound) {
			t.Fatalf("%v should be deleted, got %v", key, err)
		}
	}
}

func TestPrefixedStoreBatchSetPreservesDeadline(t *testing.T) {
	ctx := context.Background()
	base := kv.NewMemory(nil)
	store := kv.Prefixed(base, kv.Key{"service", "sessions"})

	if err := store.BatchSet(ctx, []kv.Entry{
		{Key: kv.Key{"session", "expired"}, Value: []byte("gone"), Deadline: time.Now().Add(20 * time.Millisecond)},
		{Key: kv.Key{"session", "kept"}, Value: []byte("kept")},
	}); err != nil {
		t.Fatalf("BatchSet deadline entry: %v", err)
	}
	time.Sleep(30 * time.Millisecond)

	_, err := base.Get(ctx, kv.Key{"service", "sessions", "session", "expired"})
	if !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("base Get expired prefixed key err = %v, want ErrNotFound", err)
	}
	got, err := base.Get(ctx, kv.Key{"service", "sessions", "session", "kept"})
	if err != nil {
		t.Fatalf("base Get kept prefixed key: %v", err)
	}
	if string(got) != "kept" {
		t.Fatalf("base Get kept prefixed key = %q, want kept", got)
	}
}

func TestPrefixedStoreCreateIfAbsentScopesKeysAndForwardsResult(t *testing.T) {
	ctx := context.Background()
	base := kv.NewMemory(nil)
	store := kv.Prefixed(base, kv.Key{"service", "pending"})
	guard := kv.Entry{Key: kv.Key{"locators", "peer-a"}, Value: []byte("deletion-a")}
	record := kv.Entry{Key: kv.Key{"records", "deletion-a"}, Value: []byte("record-a")}

	existing, created, err := kv.CreateIfAbsent(ctx, store, guard, []kv.Entry{record})
	if err != nil {
		t.Fatalf("CreateIfAbsent(first): %v", err)
	}
	if !created || existing != nil {
		t.Fatalf("CreateIfAbsent(first) = (%q, %v), want (nil, true)", existing, created)
	}
	if value, err := base.Get(ctx, kv.Key{"service", "pending", "locators", "peer-a"}); err != nil || string(value) != "deletion-a" {
		t.Fatalf("base Get(guard) = %q, %v", value, err)
	}
	if value, err := base.Get(ctx, kv.Key{"service", "pending", "records", "deletion-a"}); err != nil || string(value) != "record-a" {
		t.Fatalf("base Get(record) = %q, %v", value, err)
	}
	if _, err := base.Get(ctx, guard.Key); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("base Get(unprefixed guard) error = %v, want ErrNotFound", err)
	}

	existing, created, err = kv.CreateIfAbsent(
		ctx,
		store,
		kv.Entry{Key: guard.Key, Value: []byte("deletion-b")},
		[]kv.Entry{{Key: kv.Key{"records", "deletion-b"}, Value: []byte("record-b")}},
	)
	if err != nil {
		t.Fatalf("CreateIfAbsent(second): %v", err)
	}
	if created || string(existing) != "deletion-a" {
		t.Fatalf("CreateIfAbsent(second) = (%q, %v), want (deletion-a, false)", existing, created)
	}
	if _, err := base.Get(ctx, kv.Key{"service", "pending", "records", "deletion-b"}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("base Get(second record) error = %v, want ErrNotFound", err)
	}
}

func TestPrefixedStoreClonesPrefix(t *testing.T) {
	ctx := context.Background()
	base := kv.NewMemory(nil)
	prefix := kv.Key{"service", "credentials"}
	store := kv.Prefixed(base, prefix)

	prefix[1] = "workspace"
	if err := store.Set(ctx, kv.Key{"tenants", "mini-max"}, []byte("secret")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if _, err := base.Get(ctx, kv.Key{"service", "credentials", "tenants", "mini-max"}); err != nil {
		t.Fatalf("prefixed store should keep original prefix: %v", err)
	}
	if _, err := base.Get(ctx, kv.Key{"service", "workspace", "tenants", "mini-max"}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("mutated caller prefix should not be used, got %v", err)
	}
}

func TestPrefixedStoreEmptyPrefixActsAsTransparentView(t *testing.T) {
	ctx := context.Background()
	base := kv.NewMemory(nil)
	store := kv.Prefixed(base, nil)

	if err := store.Set(ctx, kv.Key{"service", "workspace"}, []byte("settings")); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := base.Get(ctx, kv.Key{"service", "workspace"})
	if err != nil {
		t.Fatalf("Get through base: %v", err)
	}
	if string(got) != "settings" {
		t.Fatalf("Get through base = %q, want %q", got, "settings")
	}

}

func TestSharedAtomicStoreResolvesNestedPrefixes(t *testing.T) {
	base := kv.NewMemory(nil)
	first := kv.Prefixed(
		kv.Prefixed(base, kv.Key{"social"}),
		kv.Key{"friend-groups"},
	)
	second := kv.Prefixed(base, kv.Key{"social", "friend-group-members"})

	root, prefixes, ok := kv.SharedAtomicStore(first, second)
	if !ok {
		t.Fatal("SharedAtomicStore() ok = false, want true")
	}
	if root != base {
		t.Fatalf("SharedAtomicStore() root = %T %p, want base %p", root, root, base)
	}
	want := []kv.Key{
		{"social", "friend-groups"},
		{"social", "friend-group-members"},
	}
	if !reflect.DeepEqual(prefixes, want) {
		t.Fatalf("SharedAtomicStore() prefixes = %#v, want %#v", prefixes, want)
	}
}

func TestSharedAtomicStoreRejectsDifferentRoots(t *testing.T) {
	first := kv.Prefixed(kv.NewMemory(nil), kv.Key{"friend-groups"})
	second := kv.Prefixed(kv.NewMemory(nil), kv.Key{"friend-group-members"})

	if root, prefixes, ok := kv.SharedAtomicStore(first, second); ok || root != nil || prefixes != nil {
		t.Fatalf(
			"SharedAtomicStore() = (%T, %#v, %t), want (nil, nil, false)",
			root,
			prefixes,
			ok,
		)
	}
}

func TestPrefixedStoreCloseDoesNotCloseBase(t *testing.T) {
	base := &closeTrackingStore{Store: kv.NewMemory(nil)}
	store := kv.Prefixed(base, kv.Key{"service"})

	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if base.closed {
		t.Fatal("prefixed store closed the underlying store")
	}
}

type closeTrackingStore struct {
	kv.Store
	closed bool
}

func (s *closeTrackingStore) Close() error {
	s.closed = true
	return nil
}
