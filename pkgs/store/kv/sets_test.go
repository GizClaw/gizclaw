package kv

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/jmoiron/sqlx"
	redis "github.com/redis/go-redis/v9"
)

func TestCollectionMutation(t *testing.T) {
	type collectionStore interface {
		Get(context.Context, Key) ([]byte, error)
		Set(context.Context, Key, []byte) error
		HasMember(context.Context, Key, string) (bool, error)
		ListMembers(context.Context, Key) ([]string, error)
		RangeOrderedMembers(context.Context, Key, OrderedRange) ([]string, error)
		ApplyMutation(context.Context, Mutation) (bool, error)
	}
	factories := map[string]func(*testing.T) collectionStore{
		"memory":   func(t *testing.T) collectionStore { return NewMemory(nil) },
		"sqlite":   func(t *testing.T) collectionStore { return newSQLiteStore(t) },
		"prefixed": func(t *testing.T) collectionStore { return Prefixed(NewMemory(nil), Key{"tenant", "nested"}) },
		"badger": func(t *testing.T) collectionStore {
			store, err := NewBadger(t.TempDir(), nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			return store
		},
	}
	if dsn := os.Getenv("GIZCLAW_TEST_REDIS_DSN"); dsn != "" {
		factories["redis"] = func(t *testing.T) collectionStore {
			options, err := redis.ParseURL(dsn)
			if err != nil {
				t.Fatal(err)
			}
			client := redis.NewClient(options)
			t.Cleanup(func() { _ = client.Close() })
			store, err := NewRedisWithClient(client, nil)
			if err != nil {
				t.Fatal(err)
			}
			return store
		}
	}
	if dsn := os.Getenv("GIZCLAW_TEST_POSTGRES_DSN"); dsn != "" {
		factories["postgres"] = func(t *testing.T) collectionStore {
			db, err := sqlx.Open("postgres", dsn)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			store, err := NewSQLWithDB(db, "kv_collection_contract", nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			return store
		}
	}
	for name, factory := range factories {
		t.Run(name, func(t *testing.T) {
			store := factory(t)
			t.Run("ordered", func(t *testing.T) { testOrderedCollection(t, store) })
			ctx := t.Context()
			namespace := "set-contract-" + rand.Text()
			record := Key{namespace, name, "record"}
			index := Key{namespace, name, "index"}
			wrong := Key{namespace, name, "wrong"}
			// Unique test keys are removed without flushing the shared Redis database.
			cleanup := Mutation{DeleteKeys: []Key{record, index, wrong}}
			if _, err := store.ApplyMutation(ctx, cleanup); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _, _ = store.ApplyMutation(context.Background(), cleanup) })
			update := Mutation{
				Conditions: []Condition{{Key: record}},
				Entries:    []Entry{{Key: record, Value: []byte("v1")}},
				AddMembers: []SetMembers{{Key: index, Members: []string{"peer:a", "peer:b", "peer:a"}}},
			}
			applied, err := store.ApplyMutation(ctx, update)
			if err != nil || !applied {
				t.Fatalf("create = %v, %v", applied, err)
			}
			members, err := store.ListMembers(ctx, index)
			slices.Sort(members)
			if err != nil || !slices.Equal(members, []string{"peer:a", "peer:b"}) {
				t.Fatalf("members = %v, %v", members, err)
			}
			if _, err := store.Get(ctx, index); !errors.Is(err, ErrWrongType) {
				t.Fatalf("collection read as record: %v", err)
			}
			update.Entries[0].Value = []byte("v2")
			update.AddMembers[0].Members = []string{"peer:c"}
			if applied, err := store.ApplyMutation(ctx, update); err != nil || applied {
				t.Fatalf("conflict = %v, %v", applied, err)
			}
			if found, err := store.HasMember(ctx, index, "peer:c"); found || err != nil {
				t.Fatalf("conflict changed index: %v, %v", found, err)
			}
			if err := store.Set(ctx, wrong, []byte("record")); err != nil {
				t.Fatal(err)
			}
			update.Conditions = []Condition{{Key: record, Expected: []byte("v1")}}
			update.AddMembers = append(update.AddMembers, SetMembers{Key: wrong, Members: []string{"invalid"}})
			if _, err := store.ApplyMutation(ctx, update); !errors.Is(err, ErrWrongType) {
				t.Fatalf("wrong type = %v", err)
			}
			if value, err := store.Get(ctx, record); err != nil || string(value) != "v1" {
				t.Fatalf("partial record write: %q, %v", value, err)
			}
			if found, err := store.HasMember(ctx, index, "peer:c"); found || err != nil {
				t.Fatalf("partial set write: %v, %v", found, err)
			}
			update.AddMembers = update.AddMembers[:1]
			update.RemoveMembers = []SetMembers{{Key: index, Members: []string{"peer:a", "peer:b", "peer:c"}}}
			if applied, err := store.ApplyMutation(ctx, update); err != nil || !applied {
				t.Fatalf("update = %v, %v", applied, err)
			}
			if members, err := store.ListMembers(ctx, index); err != nil || len(members) != 0 {
				t.Fatalf("remove = %v, %v", members, err)
			}
			if err := store.Set(ctx, index, []byte("now a record")); err != nil {
				t.Fatal(err)
			}
			if _, err := store.ListMembers(ctx, index); !errors.Is(err, ErrWrongType) {
				t.Fatalf("record listed as set: %v", err)
			}
		})
	}
}
