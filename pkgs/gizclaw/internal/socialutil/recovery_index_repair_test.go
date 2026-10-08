package socialutil

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"slices"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/jmoiron/sqlx"
	redis "github.com/redis/go-redis/v9"
	_ "modernc.org/sqlite"
)

func TestRecoveryIndexAutomaticallyRepairsMetadata(t *testing.T) {
	factories := map[string]func(*testing.T) kv.Store{
		"memory": func(t *testing.T) kv.Store {
			store := kv.NewMemory(nil)
			t.Cleanup(func() { _ = store.Close() })
			return store
		},
		"badger": func(t *testing.T) kv.Store {
			store, err := kv.NewBadgerInMemory(nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			return store
		},
		"sqlite": func(t *testing.T) kv.Store {
			db, err := sqlx.Open("sqlite", ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			db.SetMaxOpenConns(1)
			t.Cleanup(func() { _ = db.Close() })
			store, err := kv.NewSQLWithDB(db, "recovery_repair", nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			return store
		},
	}
	if dsn := os.Getenv("GIZCLAW_TEST_REDIS_DSN"); dsn != "" {
		factories["redis"] = func(t *testing.T) kv.Store {
			options, err := redis.ParseURL(dsn)
			if err != nil {
				t.Fatal(err)
			}
			client := redis.NewClient(options)
			t.Cleanup(func() { _ = client.Close() })
			store, err := kv.NewRedisWithClient(client, nil)
			if err != nil {
				t.Fatal(err)
			}
			return store
		}
	}
	for backend, factory := range factories {
		t.Run(backend, func(t *testing.T) {
			for _, scenario := range []string{
				"missing_directory", "scalar_directory", "empty_scalar_directory",
				"wrong_bucket", "invalid_bucket", "noncanonical_member", "empty_members",
				"stale_member", "empty_record", "oversized_directory",
			} {
				t.Run(scenario, func(t *testing.T) {
					base := factory(t)
					// Even an explicitly configured Redis test server only receives
					// mutations under this test's unique, nested namespace.
					store := kv.Prefixed(kv.Prefixed(base, kv.Key{"repair-test", rand.Text()}), kv.Key{"tenant"})
					index := RecoveryIndex{Root: kv.Key{"social-retirement-intents", "friend-groups"}}
					const id = "alpha"
					value := []byte(`{"identity":"alpha","generation":1}`)
					if scenario == "empty_record" {
						value = []byte{}
					}
					// Delete exactly this fixture's keys; never flush a shared DB.
					keys := []kv.Key{index.directory(), index.recordKey(id), index.bucket("invalid"), {"unrelated", "record"}}
					for number := range 256 {
						keys = append(keys, index.bucket(bucketName(number)))
					}
					defer func() { _ = store.BatchDelete(context.Background(), keys) }()
					if err := store.Set(t.Context(), kv.Key{"unrelated", "record"}, []byte("business payload")); err != nil {
						t.Fatal(err)
					}
					if _, err := store.ApplyMutation(t.Context(), kv.Mutation{
						Entries: []kv.Entry{{Key: index.recordKey(id), Value: value}}, AddMembers: index.Add(id),
					}); err != nil {
						t.Fatal(err)
					}
					correct := recoveryBucket(id)
					wrong := "ff"
					if wrong == correct {
						wrong = "00"
					}
					must := func(err error) {
						t.Helper()
						if err != nil {
							t.Fatal(err)
						}
					}
					switch scenario {
					case "missing_directory":
						must(store.Delete(t.Context(), index.directory()))
					case "scalar_directory", "empty_scalar_directory":
						metadata := []byte("corrupt directory")
						if scenario == "empty_scalar_directory" {
							metadata = []byte{}
						}
						must(store.Set(t.Context(), index.directory(), metadata))
					case "wrong_bucket", "empty_record":
						must(store.RemoveMembers(t.Context(), index.bucket(correct), id))
						must(store.AddMembers(t.Context(), index.bucket(wrong), id))
					case "invalid_bucket":
						must(store.RemoveMembers(t.Context(), index.bucket(correct), id))
						must(store.AddMembers(t.Context(), index.directory(), "invalid"))
						must(store.AddMembers(t.Context(), index.bucket("invalid"), id))
					case "noncanonical_member":
						must(store.RemoveMembers(t.Context(), index.bucket(correct), id))
						must(store.AddMembers(t.Context(), index.bucket(correct), " alpha "))
					case "empty_members":
						must(store.AddMembers(t.Context(), index.bucket(correct), "", " \t "))
					case "stale_member":
						must(store.Delete(t.Context(), index.recordKey(id)))
					case "oversized_directory":
						var names []string
						for number := range 256 {
							names = append(names, bucketName(number))
						}
						must(store.AddMembers(t.Context(), index.directory(), append(names, "invalid")...))
					}
					var attempted []string
					for range 2 {
						must(index.Reconcile(t.Context(), store, func(_ context.Context, id string) error {
							attempted = append(attempted, id)
							return nil
						}))
					}
					var indexed []string
					for id, err := range index.IDs(t.Context(), store) {
						must(err)
						indexed = append(indexed, id)
					}
					if scenario == "stale_member" {
						if len(attempted) != 0 || len(indexed) != 0 {
							t.Fatalf("stale task still discovered: attempts=%v index=%v", attempted, indexed)
						}
					} else {
						if !slices.Equal(indexed, []string{id}) || len(attempted) == 0 {
							t.Fatalf("live task not recovered: attempts=%v index=%v", attempted, indexed)
						}
						got, err := store.Get(t.Context(), index.recordKey(id))
						must(err)
						if !bytes.Equal(got, value) {
							t.Fatal("index repair changed the authoritative record")
						}
					}
					foreign, err := store.Get(t.Context(), kv.Key{"unrelated", "record"})
					must(err)
					if string(foreign) != "business payload" {
						t.Fatal("index repair changed unrelated business data")
					}
				})
			}
		})
	}
}

func bucketName(number int) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[number/16], digits[number%16]})
}

type interveningRecoveryStore struct {
	kv.Store
	before func(kv.Mutation)
}

func (s *interveningRecoveryStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	if s.before != nil {
		s.before(mutation)
	}
	return s.Store.ApplyMutation(ctx, mutation)
}

func TestRecoveryIndexDoesNotPruneConcurrentPublication(t *testing.T) {
	base := kv.NewMemory(nil)
	defer base.Close()
	index := RecoveryIndex{Root: kv.Key{"retirement"}}
	const id = "recreated"
	if _, err := base.ApplyMutation(t.Context(), kv.Mutation{AddMembers: index.Add(id)}); err != nil {
		t.Fatal(err)
	}
	store := &interveningRecoveryStore{Store: base}
	store.before = func(mutation kv.Mutation) {
		if len(mutation.Conditions) == 1 && slices.Equal(mutation.Conditions[0].Key, index.recordKey(id)) {
			store.before = nil
			if _, err := base.ApplyMutation(t.Context(), kv.Mutation{
				Entries: []kv.Entry{{Key: index.recordKey(id), Value: []byte("new generation")}}, AddMembers: index.Add(id),
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := index.Reconcile(t.Context(), store, func(context.Context, string) error {
		t.Fatal("stale snapshot was dispatched")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	present, err := base.HasMember(t.Context(), index.bucket(recoveryBucket(id)), id)
	if err != nil || !present {
		t.Fatalf("concurrently published task was unindexed: %v, %v", present, err)
	}
	var attempted []string
	if err := index.Reconcile(t.Context(), store, func(_ context.Context, id string) error {
		attempted = append(attempted, id)
		return nil
	}); err != nil || !slices.Equal(attempted, []string{id}) {
		t.Fatalf("new generation was not recovered: %v, %v", attempted, err)
	}
}

func TestRecoveryIndexKeepsMalformedRecordAndContinues(t *testing.T) {
	store := kv.NewMemory(nil)
	defer store.Close()
	index := RecoveryIndex{Root: kv.Key{"retirement"}}
	for _, id := range []string{"broken", "healthy"} {
		if _, err := store.ApplyMutation(t.Context(), kv.Mutation{
			Entries: []kv.Entry{{Key: index.recordKey(id), Value: []byte("{")}}, AddMembers: index.Add(id),
		}); err != nil {
			t.Fatal(err)
		}
	}
	invalidRecord := errors.New("invalid domain record")
	var healthy bool
	err := index.Reconcile(t.Context(), store, func(_ context.Context, id string) error {
		if id == "broken" {
			return invalidRecord
		}
		healthy = true
		return nil
	})
	if !errors.Is(err, invalidRecord) || !healthy {
		t.Fatalf("malformed record blocked healthy recovery: %v", err)
	}
	if value, err := store.Get(t.Context(), index.recordKey("broken")); err != nil || string(value) != "{" {
		t.Fatalf("malformed work record was altered: %q, %v", value, err)
	}
}

func TestRecoveryIndexMoveDoesNotOverwriteConcurrentGeneration(t *testing.T) {
	base := kv.NewMemory(nil)
	defer base.Close()
	index := RecoveryIndex{Root: kv.Key{"retirement"}}
	const id = "alpha"
	wrong := "ff"
	if wrong == recoveryBucket(id) {
		wrong = "00"
	}
	if _, err := base.ApplyMutation(t.Context(), kv.Mutation{
		Entries:    []kv.Entry{{Key: index.recordKey(id), Value: []byte("old generation")}},
		AddMembers: []kv.SetMembers{{Key: index.directory(), Members: []string{wrong}}, {Key: index.bucket(wrong), Members: []string{id}}},
	}); err != nil {
		t.Fatal(err)
	}
	store := &interveningRecoveryStore{Store: base}
	store.before = func(mutation kv.Mutation) {
		if len(mutation.Conditions) == 1 && slices.Equal(mutation.Conditions[0].Key, index.recordKey(id)) {
			store.before = nil
			if _, err := base.ApplyMutation(t.Context(), kv.Mutation{
				Entries: []kv.Entry{{Key: index.recordKey(id), Value: []byte("new generation")}}, AddMembers: index.Add(id),
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	for range 2 {
		if err := index.Reconcile(t.Context(), store, func(context.Context, string) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	value, err := base.Get(t.Context(), index.recordKey(id))
	if err != nil || string(value) != "new generation" {
		t.Fatalf("concurrent generation changed: %q, %v", value, err)
	}
	if present, err := base.HasMember(t.Context(), index.bucket(recoveryBucket(id)), id); err != nil || !present {
		t.Fatalf("new generation lost its membership: %v, %v", present, err)
	}
}

func TestRecoveryIndexDirectoryRepairPreservesConcurrentReplacement(t *testing.T) {
	base := kv.NewMemory(nil)
	defer base.Close()
	index := RecoveryIndex{Root: kv.Key{"retirement"}}
	if _, err := base.ApplyMutation(t.Context(), kv.Mutation{
		Entries: []kv.Entry{{Key: index.recordKey("alpha"), Value: []byte("record")}}, AddMembers: index.Add("alpha"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := base.Set(t.Context(), index.directory(), []byte("bad metadata")); err != nil {
		t.Fatal(err)
	}
	store := &interveningRecoveryStore{Store: base}
	store.before = func(mutation kv.Mutation) {
		if len(mutation.DeleteKeys) == 1 && slices.Equal(mutation.DeleteKeys[0], index.directory()) {
			store.before = nil
			if err := base.Delete(t.Context(), index.directory()); err != nil {
				t.Fatal(err)
			}
			if err := base.AddMembers(t.Context(), index.directory(), recoveryBucket("alpha"), recoveryBucket("beta")); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := index.Reconcile(t.Context(), store, func(context.Context, string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if present, err := base.HasMember(t.Context(), index.directory(), recoveryBucket("beta")); err != nil || !present {
		t.Fatalf("concurrent valid directory entry removed: %v, %v", present, err)
	}
}

func TestRecoveryIndexMaintenanceUsesOnlyFixedDiscoveryKeys(t *testing.T) {
	store := kv.NewMemory(nil)
	defer store.Close()
	index := RecoveryIndex{Root: kv.Key{"retirement"}}
	for i := range 10000 {
		if err := store.Set(t.Context(), kv.Key{"business", fmt.Sprintf("row-%05d", i)}, []byte("unrelated data")); err != nil {
			t.Fatal(err)
		}
	}
	// A record with no remaining membership cannot be discovered by bounded
	// index reads. Keep it intact; never pretend to reconstruct its identity.
	if err := store.Set(t.Context(), index.recordKey("unindexed"), []byte("orphaned record")); err != nil {
		t.Fatal(err)
	}
	reader := &recoveryReadStore{Store: store}
	if err := index.Reconcile(t.Context(), reader, func(context.Context, string) error {
		t.Fatal("unindexed record was guessed")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if reader.reads != 257 {
		t.Fatalf("discovery collection reads = %d, want directory plus 256 fixed buckets", reader.reads)
	}
	if value, err := store.Get(t.Context(), index.recordKey("unindexed")); err != nil || string(value) != "orphaned record" {
		t.Fatalf("undiscoverable record changed: %q, %v", value, err)
	}
}

func TestRecoveryIndexRejectsUnaddressableDirectoryEntriesWithoutPanicking(t *testing.T) {
	for _, separator := range []byte{':', '/', '|'} {
		t.Run(string(separator), func(t *testing.T) {
			store := kv.NewMemory(&kv.Options{Separator: separator})
			defer store.Close()
			index := RecoveryIndex{Root: kv.Key{"retirement"}}
			invalid := "invalid" + string(separator) + "key"
			if _, err := store.ApplyMutation(t.Context(), kv.Mutation{
				Entries:    []kv.Entry{{Key: index.recordKey("alpha"), Value: []byte("record")}},
				AddMembers: append(index.Add("alpha"), kv.SetMembers{Key: index.directory(), Members: []string{invalid}}),
			}); err != nil {
				t.Fatal(err)
			}
			var attempted bool
			err := index.Reconcile(t.Context(), store, func(context.Context, string) error {
				attempted = true
				return nil
			})
			if err == nil || !attempted {
				t.Fatalf("invalid metadata hid valid work: attempted=%v error=%v", attempted, err)
			}
			if present, err := store.HasMember(t.Context(), index.directory(), invalid); err != nil || !present {
				t.Fatalf("unreadable reference was dropped: %v, %v", present, err)
			}
		})
	}
}

func TestRecoveryIndexIsolatesUnaddressableMemberKey(t *testing.T) {
	store := kv.NewMemory(&kv.Options{Separator: '%'})
	defer store.Close()
	index := RecoveryIndex{Root: kv.Key{"retirement"}}
	const invalid = "key:1"
	if _, err := store.ApplyMutation(t.Context(), kv.Mutation{
		Entries:    []kv.Entry{{Key: index.recordKey("alpha"), Value: []byte("record")}},
		AddMembers: append(index.Add("alpha"), index.Add(invalid)...),
	}); err != nil {
		t.Fatal(err)
	}
	var attempted []string
	err := index.Reconcile(t.Context(), store, func(_ context.Context, id string) error {
		attempted = append(attempted, id)
		return nil
	})
	if err == nil || !slices.Equal(attempted, []string{"alpha"}) {
		t.Fatalf("unaddressable member blocked valid work: %v, %v", attempted, err)
	}
	if present, err := store.HasMember(t.Context(), index.bucket(recoveryBucket(invalid)), invalid); err != nil || !present {
		t.Fatalf("unaddressable member was removed: %v, %v", present, err)
	}
}
