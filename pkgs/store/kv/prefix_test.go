package kv_test

import (
	"context"
	"errors"
	"iter"
	"slices"
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

func TestPrefixedStoreListReturnsLocalKeys(t *testing.T) {
	ctx := context.Background()
	base := kv.NewMemory(nil)
	store := kv.Prefixed(base, kv.Key{"service", "workspace"})

	if err := base.BatchSet(ctx, []kv.Entry{
		{Key: kv.Key{"service", "workspace", "items", "a"}, Value: []byte("a")},
		{Key: kv.Key{"service", "workspace", "items", "b"}, Value: []byte("b")},
		{Key: kv.Key{"service", "workspace", "settings"}, Value: []byte("settings")},
		{Key: kv.Key{"service", "credentials", "items", "x"}, Value: []byte("x")},
	}); err != nil {
		t.Fatalf("BatchSet: %v", err)
	}

	var got []string
	for entry, err := range store.List(ctx, nil) {
		if err != nil {
			t.Fatalf("List all: %v", err)
		}
		got = append(got, entry.Key.String()+"="+string(entry.Value))
	}
	want := []string{
		"items:a=a",
		"items:b=b",
		"settings=settings",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("List all = %v, want %v", got, want)
	}

	got = nil
	for entry, err := range store.List(ctx, kv.Key{"items"}) {
		if err != nil {
			t.Fatalf("List items: %v", err)
		}
		got = append(got, entry.Key.String())
	}
	want = []string{"items:a", "items:b"}
	if !slices.Equal(got, want) {
		t.Fatalf("List items = %v, want %v", got, want)
	}
}

func TestPrefixedStoreListAfterUsesLocalCursor(t *testing.T) {
	ctx := context.Background()
	base := kv.NewMemory(nil)
	store := kv.Prefixed(base, kv.Key{"service", "workspace"})

	if err := base.BatchSet(ctx, []kv.Entry{
		{Key: kv.Key{"service", "workspace", "items", "a"}, Value: []byte("a")},
		{Key: kv.Key{"service", "workspace", "items", "b"}, Value: []byte("b")},
		{Key: kv.Key{"service", "workspace", "items", "c"}, Value: []byte("c")},
		{Key: kv.Key{"service", "workspace-other", "items", "z"}, Value: []byte("z")},
	}); err != nil {
		t.Fatalf("BatchSet: %v", err)
	}

	got, err := kv.ListAfter(ctx, store, kv.Key{"items"}, nil, 2)
	if err != nil {
		t.Fatalf("ListAfter first page: %v", err)
	}
	if len(got) != 2 || got[0].Key.String() != "items:a" || got[1].Key.String() != "items:b" {
		t.Fatalf("ListAfter first page = %+v", got)
	}

	got, err = kv.ListAfter(ctx, store, kv.Key{"items"}, got[len(got)-1].Key, 2)
	if err != nil {
		t.Fatalf("ListAfter second page: %v", err)
	}
	if len(got) != 1 || got[0].Key.String() != "items:c" {
		t.Fatalf("ListAfter second page = %+v", got)
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

	existing, created, err := store.CreateIfAbsent(ctx, guard, []kv.Entry{record})
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

	existing, created, err = store.CreateIfAbsent(
		ctx,
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

	var keys []string
	for entry, err := range store.List(ctx, nil) {
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		keys = append(keys, entry.Key.String())
	}
	if !slices.Equal(keys, []string{"service:workspace"}) {
		t.Fatalf("List = %v, want [service:workspace]", keys)
	}
}

func TestPrefixedStoreListPropagatesBaseError(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("list failed")
	store := kv.Prefixed(listScriptStore{
		list: func(context.Context, kv.Key) iter.Seq2[kv.Entry, error] {
			return func(yield func(kv.Entry, error) bool) {
				yield(kv.Entry{}, wantErr)
			}
		},
	}, kv.Key{"service"})

	for _, err := range collectListErrors(ctx, store, nil) {
		if errors.Is(err, wantErr) {
			return
		}
	}
	t.Fatal("List did not propagate base error")
}

func TestPrefixedStoreListRejectsOutOfScopeKeys(t *testing.T) {
	ctx := context.Background()
	store := kv.Prefixed(listScriptStore{
		list: func(context.Context, kv.Key) iter.Seq2[kv.Entry, error] {
			return func(yield func(kv.Entry, error) bool) {
				yield(kv.Entry{Key: kv.Key{"other", "key"}}, nil)
			}
		},
	}, kv.Key{"service"})

	errs := collectListErrors(ctx, store, nil)
	if len(errs) != 1 {
		t.Fatalf("List errors = %v, want one error", errs)
	}
	if got := errs[0].Error(); got != "kv: prefixed store got key other:key outside prefix service" {
		t.Fatalf("List error = %q", got)
	}
}

func TestPrefixedStoreListAfterPropagatesBaseError(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("page failed")
	store := kv.Prefixed(listScriptStore{
		listAfter: func(context.Context, kv.Key, kv.Key, int) ([]kv.Entry, error) {
			return nil, wantErr
		},
	}, kv.Key{"service"})

	if _, err := kv.ListAfter(ctx, store, nil, nil, 10); !errors.Is(err, wantErr) {
		t.Fatalf("ListAfter error = %v, want %v", err, wantErr)
	}
}

func TestPrefixedStoreListAfterRejectsOutOfScopeKeys(t *testing.T) {
	ctx := context.Background()
	store := kv.Prefixed(listScriptStore{
		listAfter: func(context.Context, kv.Key, kv.Key, int) ([]kv.Entry, error) {
			return []kv.Entry{{Key: kv.Key{"other", "key"}}}, nil
		},
	}, kv.Key{"service"})

	if _, err := kv.ListAfter(ctx, store, nil, nil, 10); err == nil {
		t.Fatal("ListAfter succeeded with out-of-scope key")
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

type listScriptStore struct {
	list      func(context.Context, kv.Key) iter.Seq2[kv.Entry, error]
	listAfter func(context.Context, kv.Key, kv.Key, int) ([]kv.Entry, error)
}

func (s listScriptStore) Get(context.Context, kv.Key) ([]byte, error) {
	return nil, kv.ErrNotFound
}

func (s listScriptStore) Set(context.Context, kv.Key, []byte) error {
	return nil
}

func (s listScriptStore) Delete(context.Context, kv.Key) error {
	return nil
}

func (s listScriptStore) List(ctx context.Context, prefix kv.Key) iter.Seq2[kv.Entry, error] {
	return s.list(ctx, prefix)
}

func (s listScriptStore) ListAfter(ctx context.Context, prefix, after kv.Key, limit int) ([]kv.Entry, error) {
	return s.listAfter(ctx, prefix, after, limit)
}

func (s listScriptStore) BatchSet(context.Context, []kv.Entry) error {
	return nil
}

func (s listScriptStore) BatchDelete(context.Context, []kv.Key) error {
	return nil
}

func (s listScriptStore) BatchMutate(context.Context, []kv.Entry, []kv.Key) error {
	return nil
}

func (s listScriptStore) CreateIfAbsent(context.Context, kv.Entry, []kv.Entry) ([]byte, bool, error) {
	return nil, false, nil
}

func (s listScriptStore) Close() error {
	return nil
}

func collectListErrors(ctx context.Context, store kv.Store, prefix kv.Key) []error {
	var errs []error
	for _, err := range store.List(ctx, prefix) {
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}
