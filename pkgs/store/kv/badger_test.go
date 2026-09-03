package kv_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/dgraph-io/badger/v4"
)

func newBadgerStore(t *testing.T, opts *kv.Options) kv.Store {
	t.Helper()
	s, err := kv.NewBadger(t.TempDir(), opts)
	if err != nil {
		t.Fatalf("NewBadger: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newBadgerInMemoryStore(t *testing.T, opts *kv.Options) kv.Store {
	t.Helper()
	s, err := kv.NewBadgerInMemory(opts)
	if err != nil {
		t.Fatalf("NewBadgerInMemory: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestBadgerGetSetDelete(t *testing.T) {
	ctx := context.Background()
	s := newBadgerStore(t, nil)

	key := kv.Key{"user", "profile", "123"}
	val := []byte("hello")

	_, err := s.Get(ctx, key)
	if !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

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

	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = s.Get(ctx, key)
	if !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}

	if err := s.Delete(ctx, kv.Key{"no", "such", "key"}); err != nil {
		t.Fatalf("Delete non-existent: %v", err)
	}
}

func TestBadgerList(t *testing.T) {
	ctx := context.Background()
	s := newBadgerStore(t, nil)

	entries := []kv.Entry{
		{Key: kv.Key{"m1", "g", "e", "Alice"}, Value: []byte("a")},
		{Key: kv.Key{"m1", "g", "e", "Bob"}, Value: []byte("b")},
		{Key: kv.Key{"m1", "g", "r", "Alice", "knows", "Bob"}, Value: []byte("r1")},
		{Key: kv.Key{"m1", "seg", "20260101", "1"}, Value: []byte("s1")},
		{Key: kv.Key{"m2", "g", "e", "Charlie"}, Value: []byte("c")},
	}
	if err := s.BatchSet(ctx, entries); err != nil {
		t.Fatalf("BatchSet: %v", err)
	}

	var got []string
	for entry, err := range s.List(ctx, kv.Key{"m1", "g", "e"}) {
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		got = append(got, entry.Key.String()+"="+string(entry.Value))
	}
	want := []string{
		"m1:g:e:Alice=a",
		"m1:g:e:Bob=b",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("List m1:g:e = %v, want %v", got, want)
	}

	got = nil
	for entry, err := range s.List(ctx, kv.Key{"m1"}) {
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		got = append(got, entry.Key.String())
	}
	if len(got) != 4 {
		t.Fatalf("List m1: got %d entries, want 4: %v", len(got), got)
	}

	got = nil
	for entry, err := range s.List(ctx, nil) {
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		got = append(got, entry.Key.String())
	}
	if len(got) != 5 {
		t.Fatalf("List all: got %d entries, want 5: %v", len(got), got)
	}
}

func TestBadgerListPrefixBoundary(t *testing.T) {
	ctx := context.Background()
	s := newBadgerStore(t, nil)

	entries := []kv.Entry{
		{Key: kv.Key{"ab", "1"}, Value: []byte("yes")},
		{Key: kv.Key{"abc", "2"}, Value: []byte("no")},
		{Key: kv.Key{"ab", "3"}, Value: []byte("yes")},
	}
	if err := s.BatchSet(ctx, entries); err != nil {
		t.Fatalf("BatchSet: %v", err)
	}

	var got []string
	for entry, err := range s.List(ctx, kv.Key{"ab"}) {
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		got = append(got, entry.Key.String())
	}
	want := []string{"ab:1", "ab:3"}
	if !slices.Equal(got, want) {
		t.Fatalf("List ab = %v, want %v", got, want)
	}
}

func TestBadgerBatchSetBatchDelete(t *testing.T) {
	ctx := context.Background()
	s := newBadgerStore(t, nil)

	entries := []kv.Entry{
		{Key: kv.Key{"a", "1"}, Value: []byte("v1")},
		{Key: kv.Key{"a", "2"}, Value: []byte("v2")},
		{Key: kv.Key{"a", "3"}, Value: []byte("v3")},
	}
	if err := s.BatchSet(ctx, entries); err != nil {
		t.Fatalf("BatchSet: %v", err)
	}

	for _, e := range entries {
		got, err := s.Get(ctx, e.Key)
		if err != nil {
			t.Fatalf("Get %v: %v", e.Key, err)
		}
		if string(got) != string(e.Value) {
			t.Fatalf("Get %v = %q, want %q", e.Key, got, e.Value)
		}
	}

	if err := s.BatchDelete(ctx, []kv.Key{{"a", "1"}, {"a", "2"}}); err != nil {
		t.Fatalf("BatchDelete: %v", err)
	}

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

func TestBadgerBatchSetDeadlineExpires(t *testing.T) {
	ctx := context.Background()
	s := newBadgerStore(t, nil)

	if err := s.BatchSet(ctx, []kv.Entry{
		{Key: kv.Key{"sessions", "expired"}, Value: []byte("gone"), Deadline: time.Now().Add(2 * time.Second)},
		{Key: kv.Key{"sessions", "kept"}, Value: []byte("kept")},
	}); err != nil {
		t.Fatalf("BatchSet deadline entry: %v", err)
	}
	got, err := s.Get(ctx, kv.Key{"sessions", "expired"})
	if err != nil {
		t.Fatalf("Get before expiration: %v", err)
	}
	if string(got) != "gone" {
		t.Fatalf("Get before expiration = %q, want gone", got)
	}

	waitForBadgerNotFound(t, func() error {
		_, err := s.Get(ctx, kv.Key{"sessions", "expired"})
		return err
	})

	var gotKeys []string
	for entry, err := range s.List(ctx, kv.Key{"sessions"}) {
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		gotKeys = append(gotKeys, entry.Key.String())
	}
	if !slices.Equal(gotKeys, []string{"sessions:kept"}) {
		t.Fatalf("List after expiration = %v, want [sessions:kept]", gotKeys)
	}
}

func TestBadgerBatchSetRejectsExpiredDeadline(t *testing.T) {
	ctx := context.Background()
	s := newBadgerStore(t, nil)

	err := s.BatchSet(ctx, []kv.Entry{
		{Key: kv.Key{"session"}, Value: []byte("value"), Deadline: time.Now().Add(-time.Second)},
	})
	if !errors.Is(err, kv.ErrInvalidDeadline) {
		t.Fatalf("BatchSet expired deadline err = %v, want ErrInvalidDeadline", err)
	}
}

func TestBadgerCustomSeparator(t *testing.T) {
	ctx := context.Background()
	s := newBadgerStore(t, &kv.Options{Separator: '/'})

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

	var keys []string
	for entry, err := range s.List(ctx, kv.Key{"path", "to"}) {
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		keys = append(keys, entry.Key.String())
	}
	if len(keys) != 1 || keys[0] != "path:to:value" {
		t.Fatalf("List = %v, want [path:to:value]", keys)
	}
}

func waitForBadgerNotFound(t *testing.T, getErr func() error) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		err := getErr()
		if errors.Is(err, kv.ErrNotFound) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for ErrNotFound, last err = %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestBadgerValueIsolation(t *testing.T) {
	ctx := context.Background()
	s := newBadgerStore(t, nil)

	key := kv.Key{"iso", "test"}
	original := []byte("original")

	if err := s.Set(ctx, key, original); err != nil {
		t.Fatalf("Set: %v", err)
	}

	original[0] = 'X'
	got, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got[0] != 'o' {
		t.Fatal("store value was mutated via original slice")
	}

	got[0] = 'Y'
	got2, _ := s.Get(ctx, key)
	if got2[0] != 'o' {
		t.Fatal("store value was mutated via returned slice")
	}
}

func TestBadgerKeySegmentValidation(t *testing.T) {
	ctx := context.Background()
	s := newBadgerStore(t, nil)

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

func TestBadgerPersistence(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	s1, err := kv.NewBadger(dir, nil)
	if err != nil {
		t.Fatalf("NewBadger: %v", err)
	}
	if err := s1.Set(ctx, kv.Key{"persist", "key"}, []byte("value")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := kv.NewBadger(dir, nil)
	if err != nil {
		t.Fatalf("NewBadger reopen: %v", err)
	}
	defer s2.Close()

	got, err := s2.Get(ctx, kv.Key{"persist", "key"})
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if string(got) != "value" {
		t.Fatalf("Get = %q, want %q", got, "value")
	}
}

func TestBadgerWithDBBorrowsDatabase(t *testing.T) {
	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true).WithLogger(nil))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := kv.NewBadgerWithDB(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.View(func(*badger.Txn) error { return nil }); err != nil {
		t.Fatalf("borrowed database was closed: %v", err)
	}
}

func TestBadgerInMemory(t *testing.T) {
	ctx := context.Background()
	s := newBadgerInMemoryStore(t, nil)

	if err := s.Set(ctx, kv.Key{"a", "b"}, []byte("c")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get(ctx, kv.Key{"a", "b"})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "c" {
		t.Fatalf("Get = %q, want %q", got, "c")
	}
}

func TestBadgerGCAndSize(t *testing.T) {
	s, err := kv.NewBadger(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("NewBadger: %v", err)
	}
	defer s.Close()

	_ = s.RunGC(0.5)

	lsm, vlog, err := s.Size()
	if err != nil {
		t.Fatalf("Size: %v", err)
	}
	if lsm < 0 || vlog < 0 {
		t.Fatalf("unexpected negative sizes: lsm=%d, vlog=%d", lsm, vlog)
	}
}

func TestBadgerClosedStoreAnswersErrorInsteadOfPanic(t *testing.T) {
	ctx := context.Background()
	store, err := kv.NewBadgerInMemory(nil)
	if err != nil {
		t.Fatalf("NewBadgerInMemory: %v", err)
	}
	key := kv.Key{"pending", "task"}
	if err := store.Set(ctx, key, []byte("value")); err != nil {
		t.Fatalf("Set before Close: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	entries := []kv.Entry{{Key: key, Value: []byte("value")}}
	keys := []kv.Key{key}
	calls := []struct {
		name string
		call func() error
	}{
		{"Get", func() error { _, err := store.Get(ctx, key); return err }},
		{"Set", func() error { return store.Set(ctx, key, []byte("value")) }},
		{"Delete", func() error { return store.Delete(ctx, key) }},
		{"List", func() error {
			for _, err := range store.List(ctx, kv.Key{"pending"}) {
				return err
			}
			return nil
		}},
		{"ListAfter", func() error {
			_, err := store.ListAfter(ctx, kv.Key{"pending"}, nil, 10)
			return err
		}},
		{"BatchSet", func() error { return store.BatchSet(ctx, entries) }},
		{"BatchDelete", func() error { return store.BatchDelete(ctx, keys) }},
		{"BatchMutate", func() error { return store.BatchMutate(ctx, entries, keys) }},
		{"CreateIfAbsent", func() error {
			_, _, err := store.CreateIfAbsent(ctx, entries[0], nil)
			return err
		}},
		{"CreateIfAllAbsent", func() error {
			_, _, _, err := store.CreateIfAllAbsent(ctx, entries, nil)
			return err
		}},
		{"CompareAndMutate", func() error {
			_, err := store.CompareAndMutate(ctx, key, []byte("value"), entries, nil)
			return err
		}},
		{"RunGC", func() error { return store.RunGC(0.5) }},
		{"Size", func() error {
			lsm, vlog, err := store.Size()
			if lsm != 0 || vlog != 0 {
				t.Errorf("Size() = (%d, %d), want (0, 0) on a closed store", lsm, vlog)
			}
			return err
		}},
	}
	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); !errors.Is(err, kv.ErrStoreClosed) {
				t.Fatalf("%s() error = %v, want %v", tc.name, err, kv.ErrStoreClosed)
			}
		})
	}
}

func TestBadgerCloseIsIdempotent(t *testing.T) {
	store, err := kv.NewBadger(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("NewBadger: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	for range 2 {
		if err := store.Close(); err != nil {
			t.Fatalf("repeated Close: %v", err)
		}
	}
	if _, err := store.Get(context.Background(), kv.Key{"a"}); !errors.Is(err, kv.ErrStoreClosed) {
		t.Fatalf("Get after repeated Close error = %v, want %v", err, kv.ErrStoreClosed)
	}
}

func TestBadgerCloseWaitsForInFlightReads(t *testing.T) {
	ctx := context.Background()
	store, err := kv.NewBadgerInMemory(nil)
	if err != nil {
		t.Fatalf("NewBadgerInMemory: %v", err)
	}
	for i := range 32 {
		key := kv.Key{"pending", "by-id", strings.Repeat("0", 4) + string(rune('a'+i%26))}
		if err := store.Set(ctx, key, []byte("value")); err != nil {
			t.Fatalf("Set: %v", err)
		}
	}

	readerDone := make(chan struct{})
	closedSeen := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			_, err := store.ListAfter(ctx, kv.Key{"pending", "by-id"}, nil, 16)
			if errors.Is(err, kv.ErrStoreClosed) {
				close(closedSeen)
				return
			}
			if err != nil {
				t.Errorf("ListAfter before close error = %v", err)
				return
			}
		}
	}()

	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-readerDone:
	case <-time.After(10 * time.Second):
		t.Fatal("reader did not exit after Close")
	}
	select {
	case <-closedSeen:
	default:
		t.Fatal("reader never observed ErrStoreClosed")
	}
}

func TestBadgerNestedCallInsideListIsNotBlockedByClose(t *testing.T) {
	ctx := context.Background()
	store, err := kv.NewBadgerInMemory(nil)
	if err != nil {
		t.Fatalf("NewBadgerInMemory: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, name := range []string{"a", "b", "c"} {
		if err := store.Set(ctx, kv.Key{"pending", name}, []byte(name)); err != nil {
			t.Fatalf("Set: %v", err)
		}
	}

	// A Get inside a List iteration is the shape KVSource.ListTasks uses. It
	// must not be able to wedge against a concurrent Close.
	seen := 0
	for entry, err := range store.List(ctx, kv.Key{"pending"}) {
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		value, err := store.Get(ctx, entry.Key)
		if err != nil {
			t.Fatalf("nested Get: %v", err)
		}
		if string(value) != string(entry.Value) {
			t.Fatalf("nested Get = %q, want %q", value, entry.Value)
		}
		seen++
	}
	if seen != 3 {
		t.Fatalf("listed %d entries, want 3", seen)
	}
}

func TestBadgerNestedCallRacingCloseTerminates(t *testing.T) {
	ctx := context.Background()
	store, err := kv.NewBadgerInMemory(nil)
	if err != nil {
		t.Fatalf("NewBadgerInMemory: %v", err)
	}
	if err := store.Set(ctx, kv.Key{"pending", "a"}, []byte("a")); err != nil {
		t.Fatalf("Set: %v", err)
	}

	started := make(chan struct{})
	proceed := make(chan struct{})
	iterated := make(chan error, 1)
	go func() {
		var nested error
		for entry, err := range store.List(ctx, kv.Key{"pending"}) {
			if err != nil {
				iterated <- err
				return
			}
			close(started)
			<-proceed
			_, nested = store.Get(ctx, entry.Key)
			break
		}
		iterated <- nested
	}()

	<-started
	closed := make(chan error, 1)
	go func() { closed <- store.Close() }()
	close(proceed)

	select {
	case nested := <-iterated:
		if nested != nil && !errors.Is(nested, kv.ErrStoreClosed) {
			t.Fatalf("nested Get error = %v, want nil or %v", nested, kv.ErrStoreClosed)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("nested call did not finish while Close was draining")
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not return")
	}
}
