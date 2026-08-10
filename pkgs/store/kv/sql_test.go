package kv

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
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
	var listed []Entry
	for entry, err := range store.List(ctx, Key{"a"}) {
		if err != nil {
			t.Fatal(err)
		}
		listed = append(listed, entry)
	}
	if len(listed) != 2 || !slices.Equal(listed[0].Key, Key{"a", "1"}) || !slices.Equal(listed[1].Key, Key{"a", "2"}) {
		t.Fatalf("List() = %+v", listed)
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

func TestSQLStoresZeroLengthValuesAsNonNull(t *testing.T) {
	values := []struct {
		name  string
		value []byte
	}{
		{name: "nil"},
		{name: "empty", value: []byte{}},
	}
	operations := []struct {
		name string
		run  func(*testing.T, context.Context, *SQL, []byte) Key
	}{
		{
			name: "set",
			run: func(t *testing.T, ctx context.Context, store *SQL, value []byte) Key {
				key := Key{"empty"}
				if err := store.Set(ctx, key, value); err != nil {
					t.Fatal(err)
				}
				return key
			},
		},
		{
			name: "batch-set",
			run: func(t *testing.T, ctx context.Context, store *SQL, value []byte) Key {
				key := Key{"empty"}
				if err := store.BatchSet(ctx, []Entry{{Key: key, Value: value}}); err != nil {
					t.Fatal(err)
				}
				return key
			},
		},
		{
			name: "batch-mutate",
			run: func(t *testing.T, ctx context.Context, store *SQL, value []byte) Key {
				key := Key{"empty"}
				if err := store.BatchMutate(ctx, []Entry{{Key: key, Value: value}}, nil); err != nil {
					t.Fatal(err)
				}
				return key
			},
		},
		{
			name: "create-if-absent-guard",
			run: func(t *testing.T, ctx context.Context, store *SQL, value []byte) Key {
				guard := Entry{Key: Key{"empty"}, Value: value}
				if _, created, err := store.CreateIfAbsent(ctx, guard, nil); err != nil || !created {
					t.Fatalf("CreateIfAbsent() = _, %v, %v", created, err)
				}
				return guard.Key
			},
		},
		{
			name: "create-if-all-absent-related-entry",
			run: func(t *testing.T, ctx context.Context, store *SQL, value []byte) Key {
				key := Key{"empty"}
				_, _, created, err := store.CreateIfAllAbsent(ctx,
					[]Entry{{Key: Key{"guard"}, Value: []byte("guard")}},
					[]Entry{{Key: key, Value: value}},
				)
				if err != nil || !created {
					t.Fatalf("CreateIfAllAbsent() = _, _, %v, %v", created, err)
				}
				return key
			},
		},
		{
			name: "compare-and-mutate",
			run: func(t *testing.T, ctx context.Context, store *SQL, value []byte) Key {
				guard := Key{"guard"}
				if err := store.Set(ctx, guard, []byte("guard")); err != nil {
					t.Fatal(err)
				}
				key := Key{"empty"}
				matched, err := store.CompareAndMutate(ctx, guard, []byte("guard"), []Entry{{Key: key, Value: value}}, nil)
				if err != nil || !matched {
					t.Fatalf("CompareAndMutate() = %v, %v", matched, err)
				}
				return key
			},
		},
	}

	for _, operation := range operations {
		for _, input := range values {
			t.Run(operation.name+"/"+input.name, func(t *testing.T) {
				store := newSQLiteStore(t)
				key := operation.run(t, context.Background(), store, input.value)
				assertSQLZeroLengthValue(t, store, key)
			})
		}
	}
}

func assertSQLZeroLengthValue(t *testing.T, store *SQL, key Key) {
	t.Helper()
	value, err := store.Get(context.Background(), key)
	if err != nil {
		t.Fatalf("Get(%v) error = %v", key, err)
	}
	if len(value) != 0 {
		t.Fatalf("Get(%v) value length = %d, want 0", key, len(value))
	}
	var isNull bool
	var length int
	query := "SELECT value IS NULL, length(value) FROM " + store.quoted + " WHERE encoded_key = ?"
	if err := store.db.QueryRow(store.db.Rebind(query), store.opts.encode(key)).Scan(&isNull, &length); err != nil {
		t.Fatalf("inspect %v: %v", key, err)
	}
	if isNull || length != 0 {
		t.Fatalf("stored %v value: is_null=%v length=%d, want false and 0", key, isNull, length)
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
	index, err := storage.SQLIndexName(store.table, "expires_idx")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP INDEX "` + index + `"`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE INDEX "` + index + `" ON "kv_schema" (encoded_key)`); err != nil {
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
