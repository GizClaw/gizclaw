package kv

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func newSQLiteStore(t *testing.T) *SQL {
	t.Helper()
	db := sqlx.MustOpen("sqlite", ":memory:")
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewSQLWithDB(db, "kv_items", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestSQLStoreContract(t *testing.T) {
	store := newSQLiteStore(t)
	ctx := context.Background()
	entries := []Entry{
		{Key: Key{"a", "1"}, Value: []byte("one")},
		{Key: Key{"a", "2"}, Value: []byte("two")},
		{Key: Key{"ab", "1"}, Value: []byte("other")},
	}
	if err := store.BatchSet(ctx, entries); err != nil {
		t.Fatal(err)
	}
	got, err := store.ListAfter(ctx, Key{"a"}, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !slices.Equal(got[0].Key, Key{"a", "1"}) || !slices.Equal(got[1].Key, Key{"a", "2"}) {
		t.Fatalf("ListAfter() = %+v", got)
	}
	got[0].Value[0] = 'X'
	value, err := store.Get(ctx, Key{"a", "1"})
	if err != nil || string(value) != "one" {
		t.Fatalf("Get() = %q, %v", value, err)
	}
	if err := store.BatchMutate(ctx, []Entry{{Key: Key{"a", "3"}, Value: []byte("three")}}, []Key{{"a", "2"}, {"a", "3"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, Key{"a", "3"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete-wins Get() error = %v", err)
	}
}

func TestSQLConditionalAndExpiration(t *testing.T) {
	store := newSQLiteStore(t)
	ctx := context.Background()
	guard := Entry{Key: Key{"guard"}, Value: []byte("winner")}
	_, created, err := store.CreateIfAbsent(ctx, guard, []Entry{{Key: guard.Key, Value: []byte("loser")}})
	if err != nil || !created {
		t.Fatalf("CreateIfAbsent() = _, %v, %v", created, err)
	}
	value, err := store.Get(ctx, guard.Key)
	if err != nil || string(value) != "winner" {
		t.Fatalf("Get(guard) = %q, %v", value, err)
	}
	existing, created, err := store.CreateIfAbsent(ctx, Entry{Key: guard.Key, Value: []byte("replacement"), Deadline: time.Now().Add(-time.Second)}, nil)
	if err != nil || created || string(existing) != "winner" {
		t.Fatalf("CreateIfAbsent(existing) = %q, %v, %v", existing, created, err)
	}
	matched, err := store.CompareAndMutate(ctx, guard.Key, []byte("winner"), []Entry{{Key: Key{"next"}, Value: []byte("ok")}}, nil)
	if err != nil || !matched {
		t.Fatalf("CompareAndMutate() = %v, %v", matched, err)
	}
	if err := store.BatchSet(ctx, []Entry{{Key: Key{"expired"}, Value: []byte("gone"), Deadline: time.Now().Add(20 * time.Millisecond)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec("UPDATE "+store.quoted+" SET expires_at_unix_nano = 0 WHERE encoded_key = ?", store.opts.encode(Key{"expired"})); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, Key{"expired"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired Get() error = %v", err)
	}
	duplicate := Key{"duplicate-guard"}
	conflict, existing, created, err := store.CreateIfAllAbsent(ctx, []Entry{
		{Key: duplicate, Value: []byte("first")},
		{Key: duplicate, Value: []byte("last")},
	}, nil)
	if err != nil || !created || conflict != nil || existing != nil {
		t.Fatalf("CreateIfAllAbsent(duplicate) = %v, %q, %v, %v", conflict, existing, created, err)
	}
	value, err = store.Get(ctx, duplicate)
	if err != nil || string(value) != "last" {
		t.Fatalf("Get(duplicate guard) = %q, %v", value, err)
	}
}

func TestSQLCloseLeavesPoolOpen(t *testing.T) {
	db := sqlx.MustOpen("sqlite", ":memory:")
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewSQLWithDB(db, "kv_close", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(context.Background(), Key{"a"}); err == nil {
		t.Fatal("closed Store accepted Get")
	}
	if _, err := store.ListAfter(context.Background(), nil, nil, 0); err == nil {
		t.Fatal("closed Store accepted zero-limit ListAfter")
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("Close closed borrowed pool: %v", err)
	}
}

func TestSQLRejectsIncompatibleSchemaAndRollsBackCanceledMutation(t *testing.T) {
	db := sqlx.MustOpen("sqlite", ":memory:")
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewSQLWithDB(db, "kv_schema", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set(context.Background(), Key{"stable"}, []byte("value")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.BatchMutate(ctx, []Entry{{Key: Key{"new"}, Value: []byte("new")}}, []Key{{"stable"}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("BatchMutate() error = %v, want context.Canceled", err)
	}
	if value, err := store.Get(context.Background(), Key{"stable"}); err != nil || string(value) != "value" {
		t.Fatalf("Get(stable) = %q, %v", value, err)
	}
	index := strings.TrimSuffix(store.namespace.VersionTable, "_schema_versions") + "_expires_idx"
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP INDEX "` + index + `"`); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSQLWithDB(db, "kv_schema", nil); err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("NewSQLWithDB() error = %v", err)
	}
}

func TestSQLRejectsOutOfRangeDeadline(t *testing.T) {
	store := newSQLiteStore(t)
	err := store.BatchSet(context.Background(), []Entry{{
		Key: Key{"future"}, Value: []byte("value"), Deadline: time.Date(2500, 1, 1, 0, 0, 0, 0, time.UTC),
	}})
	if !errors.Is(err, ErrInvalidDeadline) {
		t.Fatalf("BatchSet() error = %v, want ErrInvalidDeadline", err)
	}
}
