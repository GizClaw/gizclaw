package kv_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// storeFactory creates a new Store for testing.
func newTestStore(t *testing.T, opts *kv.Options) kv.Store {
	t.Helper()
	s, err := kv.NewBadgerInMemory(opts)
	if err != nil {
		t.Fatalf("NewBadgerInMemory: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestGetSetDelete(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, nil)

	key := kv.Key{"user", "profile", "123"}
	val := []byte("hello")

	// Get non-existent key.
	_, err := s.Get(ctx, key)
	if !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// Set and Get.
	if err := s.Set(ctx, key, val); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != string(val) {
		t.Fatalf("Get = %q, want %q", got, val)
	}

	// Overwrite.
	val2 := []byte("world")
	if err := s.Set(ctx, key, val2); err != nil {
		t.Fatalf("Set overwrite: %v", err)
	}
	got, err = s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get after overwrite: %v", err)
	}
	if string(got) != string(val2) {
		t.Fatalf("Get = %q, want %q", got, val2)
	}

	// Delete.
	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = s.Get(ctx, key)
	if !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}

	// Delete non-existent key should not error.
	if err := s.Delete(ctx, kv.Key{"no", "such", "key"}); err != nil {
		t.Fatalf("Delete non-existent: %v", err)
	}
}

func TestBatchSetBatchDelete(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, nil)

	entries := []kv.Entry{
		{Key: kv.Key{"a", "1"}, Value: []byte("v1")},
		{Key: kv.Key{"a", "2"}, Value: []byte("v2")},
		{Key: kv.Key{"a", "3"}, Value: []byte("v3")},
	}
	if err := s.BatchSet(ctx, entries); err != nil {
		t.Fatalf("BatchSet: %v", err)
	}

	// Verify all set.
	for _, e := range entries {
		got, err := s.Get(ctx, e.Key)
		if err != nil {
			t.Fatalf("Get %v: %v", e.Key, err)
		}
		if string(got) != string(e.Value) {
			t.Fatalf("Get %v = %q, want %q", e.Key, got, e.Value)
		}
	}

	// BatchDelete first two.
	if err := s.BatchDelete(ctx, []kv.Key{{"a", "1"}, {"a", "2"}}); err != nil {
		t.Fatalf("BatchDelete: %v", err)
	}

	// First two gone, third remains.
	_, err := s.Get(ctx, kv.Key{"a", "1"})
	if !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a:1, got %v", err)
	}
	_, err = s.Get(ctx, kv.Key{"a", "2"})
	if !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a:2, got %v", err)
	}
	got, err := s.Get(ctx, kv.Key{"a", "3"})
	if err != nil {
		t.Fatalf("Get a:3: %v", err)
	}
	if string(got) != "v3" {
		t.Fatalf("Get a:3 = %q, want %q", got, "v3")
	}
}

func TestCustomSeparator(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, &kv.Options{Separator: '/'})

	key := kv.Key{"path", "to", "value"}
	val := []byte("data")

	if err := s.Set(ctx, key, val); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != string(val) {
		t.Fatalf("Get = %q, want %q", got, val)
	}

}

func TestValueIsolation(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, nil)

	key := kv.Key{"iso", "test"}
	original := []byte("original")

	if err := s.Set(ctx, key, original); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Mutate the original slice — store should not be affected.
	original[0] = 'X'

	got, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got[0] != 'o' {
		t.Fatal("store value was mutated via original slice")
	}

	// Mutate the returned slice — store should not be affected.
	got[0] = 'Y'
	got2, _ := s.Get(ctx, key)
	if got2[0] != 'o' {
		t.Fatal("store value was mutated via returned slice")
	}
}

func TestKeySegmentValidation(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, nil)

	// A key segment containing the separator should panic.
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for key segment containing separator")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "contains separator") {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()

	_ = s.Set(ctx, kv.Key{"bad:seg", "x"}, []byte("v"))
}
