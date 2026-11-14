package pendingdeletion

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type storeWithoutCreateIfAbsent struct {
	kv.Store
}

type createIfAbsentWinnerStore struct {
	kv.Store
	existingID string
}

func (s createIfAbsentWinnerStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	return false, s.Store.Set(ctx, mutation.Conditions[0].Key, []byte(s.existingID))
}

type recordGuardRaceStore struct {
	kv.Store
	beforeMutation func()
}

func (s *recordGuardRaceStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	if s.beforeMutation != nil {
		before := s.beforeMutation
		s.beforeMutation = nil
		before()
	}
	return s.Store.ApplyMutation(ctx, mutation)
}

func TestMutateRecordGuardedPreservesTaskAndRejectsChangedEvidence(t *testing.T) {
	for _, scenario := range []string{"success", "marker_race", "locator_race", "domain_race", "completed", "invalid_domain"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := t.Context()
			authority := kv.NewMemory(nil)
			store := &recordGuardRaceStore{Store: authority}
			now := time.Now().UTC()
			record, err := New(KindPeer, "peer-a", new("peer-a"), ReasonPeerDelete, struct{}{}, now)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := CreateOrGet(ctx, store, record); err != nil {
				t.Fatal(err)
			}
			source := KVSource{Store: store, SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
			fingerprint, err := Fingerprint(record)
			if err != nil {
				t.Fatal(err)
			}
			if _, claimed, err := source.Claim(ctx, Reference{Source: source.Name(), DeletionID: record.DeletionID, MarkerFingerprint: fingerprint}, now, time.Minute); err != nil || !claimed {
				t.Fatalf("claim = %v, %v", claimed, err)
			}
			taskBefore, err := authority.Get(ctx, kvTaskKey(record.DeletionID))
			if err != nil {
				t.Fatal(err)
			}
			domain, snapshot := kv.Key{"peers", "peer-a"}, kv.Key{"snapshots", record.DeletionID}
			if err := authority.Set(ctx, domain, []byte("active")); err != nil {
				t.Fatal(err)
			}
			entries := []kv.Entry{{Key: domain, Value: []byte("deleted")}, {Key: snapshot, Value: []byte("original-bytes")}}
			switch scenario {
			case "marker_race":
				store.beforeMutation = func() {
					changed := record
					changed.DeletedAt = now.Add(time.Second)
					data, err := json.Marshal(changed)
					if err != nil {
						t.Fatal(err)
					}
					if err := authority.Set(ctx, byIDKey(record.DeletionID), data); err != nil {
						t.Fatal(err)
					}
				}
			case "locator_race":
				store.beforeMutation = func() {
					if err := authority.Set(ctx, byLocatorKey(record.Kind, record.ResourceID), []byte("replacement")); err != nil {
						t.Fatal(err)
					}
				}
			case "domain_race":
				store.beforeMutation = func() {
					if err := authority.Set(ctx, domain, []byte("replacement")); err != nil {
						t.Fatal(err)
					}
				}
			case "completed":
				if err := authority.Delete(ctx, byIDKey(record.DeletionID)); err != nil {
					t.Fatal(err)
				}
			case "invalid_domain":
				entries = append(entries, kv.Entry{Key: kvTaskKey(record.DeletionID), Value: []byte("overwrite")})
			}
			matched, err := MutateRecordGuarded(ctx, store, record, []kv.Condition{{Key: domain, Expected: []byte("active")}, {Key: snapshot}}, entries...)
			if scenario == "invalid_domain" {
				if err == nil || matched {
					t.Fatalf("unsafe domain key accepted: %v, %v", matched, err)
				}
			} else if err != nil || matched != (scenario == "success") {
				t.Fatalf("guarded mutation = %v, %v", matched, err)
			}
			taskAfter, err := authority.Get(ctx, kvTaskKey(record.DeletionID))
			if err != nil || !bytes.Equal(taskBefore, taskAfter) {
				t.Fatalf("mutable task or lease changed: %q, %v", taskAfter, err)
			}
			data, err := authority.Get(ctx, snapshot)
			if scenario == "success" {
				if err != nil || string(data) != "original-bytes" {
					t.Fatalf("snapshot missing: %q, %v", data, err)
				}
			} else if !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("rejected mutation published snapshot: %q, %v", data, err)
			}
		})
	}
}

func TestCreateOrGetUsesAtomicMutation(t *testing.T) {
	store := storeWithoutCreateIfAbsent{Store: kv.NewMemory(nil)}
	record, err := New(KindWorkspace, "workspace-a", nil, ReasonResourceDelete, struct{}{}, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, created, err := CreateOrGet(t.Context(), store, record); err != nil || !created {
		t.Fatalf("CreateOrGet = %v, %v", created, err)
	}
	source := KVSource{Store: store, SourceName: "workspace", OwnedKinds: []Kind{KindWorkspace}}
	task, err := source.GetTask(t.Context(), record.DeletionID)
	if err != nil || task.Status != StatusQueued {
		t.Fatalf("initial persisted task = %#v, %v", task, err)
	}
}

func TestCreateOrGetGuardedDomainEntries(t *testing.T) {
	for _, backend := range []string{"memory", "badger"} {
		for _, outcome := range []string{"commit", "condition conflict", "marker conflict", "task conflict"} {
			t.Run(backend+"/"+outcome, func(t *testing.T) {
				var base kv.Store
				if backend == "memory" {
					base = kv.NewMemory(&kv.Options{Separator: '/'})
				} else {
					var err error
					base, err = kv.NewBadgerInMemory(&kv.Options{Separator: '/'})
					if err != nil {
						t.Fatal(err)
					}
				}
				t.Cleanup(func() { _ = base.Close() })
				store := kv.Prefixed(base, kv.Key{"peer"})
				record, err := New(KindPeer, "peer-a", nil, ReasonPeerDelete, map[string]string{"public_key": "peer-a"}, time.Unix(1, 0))
				if err != nil {
					t.Fatal(err)
				}
				domainKey := kv.Key{"by-pubkey", "peer-a"}
				original := []byte("active peer")
				if err := store.Set(t.Context(), domainKey, original); err != nil {
					t.Fatal(err)
				}
				condition := kv.Condition{Key: domainKey, Expected: original}
				switch outcome {
				case "condition conflict":
					condition.Expected = []byte("stale peer")
				case "marker conflict":
					if err := store.Set(t.Context(), byIDKey(record.DeletionID), []byte("occupied")); err != nil {
						t.Fatal(err)
					}
				case "task conflict":
					if err := store.Set(t.Context(), kvTaskKey(record.DeletionID), []byte("occupied")); err != nil {
						t.Fatal(err)
					}
				}
				domainEntries := []kv.Entry{
					{Key: domainKey, Value: []byte("permanent tombstone")},
					{Key: kv.Key{"retired", "peer-a"}, Value: []byte("retired")},
				}
				keys := []kv.Key{domainKey, domainEntries[1].Key, byIDKey(record.DeletionID), byLocatorKey(record.Kind, record.ResourceID), kvTaskKey(record.DeletionID)}
				before := store.BatchGet(t.Context(), keys)
				got, created, err := CreateOrGetGuarded(t.Context(), store, record, []kv.Condition{condition}, domainEntries...)
				if outcome == "commit" {
					if err != nil || !created || got.DeletionID != record.DeletionID {
						t.Fatalf("CreateOrGetGuarded = %#v, %v, %v", got, created, err)
					}
					for _, entry := range domainEntries {
						value, err := store.Get(t.Context(), entry.Key)
						if err != nil || !bytes.Equal(value, entry.Value) {
							t.Fatalf("domain entry %v = %q, %v", entry.Key, value, err)
						}
					}
					stored, err := GetByLocator(t.Context(), store, record.Kind, record.ResourceID)
					if err != nil || stored.DeletionID != record.DeletionID {
						t.Fatalf("GetByLocator = %#v, %v", stored, err)
					}
					task, err := (KVSource{Store: store, SourceName: "peer", OwnedKinds: []Kind{KindPeer}}).GetTask(t.Context(), record.DeletionID)
					if err != nil || task.Status != StatusQueued || task.Phase != PhaseValidate {
						t.Fatalf("GetTask = %#v, %v", task, err)
					}
				} else {
					if !errors.Is(err, ErrConflict) || created {
						t.Fatalf("CreateOrGetGuarded = %#v, %v, %v, want conflict", got, created, err)
					}
					after := store.BatchGet(t.Context(), keys)
					for i, previous := range before {
						if !errors.Is(after[i].Err, previous.Err) || !bytes.Equal(after[i].Value, previous.Value) {
							t.Fatalf("key %v changed on conflict: before=%#v, after=%#v", keys[i], previous, after[i])
						}
					}
				}
				fingerprint, err := Fingerprint(record)
				if err != nil {
					t.Fatal(err)
				}
				task := Task{Record: record, MarkerFingerprint: fingerprint, Status: StatusQueued, NextAttemptAt: record.DeletedAt}
				indexes, _ := kvTaskIndexChanges(nil, &task)
				for _, index := range indexes {
					members, err := store.RangeOrderedMembers(t.Context(), index.Key, kv.OrderedRange{Limit: 2})
					if err != nil {
						t.Fatal(err)
					}
					if outcome == "commit" {
						if len(members) != 1 || members[0] != index.Members[0] {
							t.Fatalf("index %v = %v, want %v", index.Key, members, index.Members)
						}
					} else if len(members) != 0 {
						t.Fatalf("index %v leaked on conflict: %v", index.Key, members)
					}
				}
			})
		}
	}
}

func TestCreateOrGetGuardedReusesTaskWithoutDomainWrites(t *testing.T) {
	for _, matchingCondition := range []bool{false, true} {
		t.Run(map[bool]string{false: "rejected condition", true: "matching condition"}[matchingCondition], func(t *testing.T) {
			store := kv.NewMemory(nil)
			first, err := New(KindPeer, "peer-a", nil, ReasonPeerDelete, map[string]string{"name": "original"}, time.Unix(1, 0))
			if err != nil {
				t.Fatal(err)
			}
			domainKey := kv.Key{"by-pubkey", "peer-a"}
			tombstone := []byte("original tombstone")
			if _, created, err := CreateOrGetGuarded(t.Context(), store, first, nil, kv.Entry{Key: domainKey, Value: tombstone}); err != nil || !created {
				t.Fatalf("first create = %v, %v", created, err)
			}
			taskBefore, err := store.Get(t.Context(), kvTaskKey(first.DeletionID))
			if err != nil {
				t.Fatal(err)
			}
			second, err := New(KindPeer, "peer-a", nil, ReasonPeerDelete, map[string]string{"name": "replacement"}, time.Unix(2, 0))
			if err != nil {
				t.Fatal(err)
			}
			expected := []byte("active peer")
			if matchingCondition {
				expected = tombstone
			}
			newKey := kv.Key{"retired", "peer-a"}
			got, created, err := CreateOrGetGuarded(t.Context(), store, second, []kv.Condition{{Key: domainKey, Expected: expected}},
				kv.Entry{Key: domainKey, Value: []byte("replacement tombstone")}, kv.Entry{Key: newKey, Value: []byte("new entry")})
			if err != nil || created || got.DeletionID != first.DeletionID || !bytes.Equal(got.Descriptor, first.Descriptor) || !got.DeletedAt.Equal(first.DeletedAt) {
				t.Fatalf("reentrant create = %#v, %v, %v, want original record", got, created, err)
			}
			value, err := store.Get(t.Context(), domainKey)
			if err != nil || !bytes.Equal(value, tombstone) {
				t.Fatalf("tombstone = %q, %v, want original", value, err)
			}
			if _, err := store.Get(t.Context(), newKey); !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("new domain entry error = %v, want not found", err)
			}
			taskAfter, err := store.Get(t.Context(), kvTaskKey(first.DeletionID))
			if err != nil || !bytes.Equal(taskAfter, taskBefore) {
				t.Fatalf("task changed on reentry: %q, %v", taskAfter, err)
			}
		})
	}
}

func TestCreateOrGetGuardedRejectsInvalidDomainEntries(t *testing.T) {
	record, err := New(KindPeer, "peer-a", nil, ReasonPeerDelete, struct{}{}, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		key  kv.Key
	}{
		{name: "empty key"},
		{name: "empty segment", key: kv.Key{"by-pubkey", ""}},
		{name: "reserved root", key: kv.Key{"pending-deletion"}},
		{name: "reserved marker", key: byIDKey(record.DeletionID)},
		{name: "reserved locator", key: byLocatorKey(record.Kind, record.ResourceID)},
		{name: "reserved task", key: kvTaskKey(record.DeletionID)},
		{name: "reserved other key", key: kv.Key{"pending-deletion", "other"}},
		{name: "duplicate key", key: kv.Key{"by-pubkey", "peer-a"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := kv.NewMemory(nil)
			domainKey := kv.Key{"by-pubkey", "peer-a"}
			if err := store.Set(t.Context(), domainKey, []byte("active")); err != nil {
				t.Fatal(err)
			}
			_, created, err := CreateOrGetGuarded(t.Context(), store, record, nil,
				kv.Entry{Key: domainKey, Value: []byte("tombstone")}, kv.Entry{Key: test.key, Value: []byte("invalid")})
			if err == nil || created {
				t.Fatalf("CreateOrGetGuarded = %v, %v, want validation error", created, err)
			}
			value, err := store.Get(t.Context(), domainKey)
			if err != nil || string(value) != "active" {
				t.Fatalf("domain value = %q, %v, want unchanged", value, err)
			}
			for _, key := range []kv.Key{byIDKey(record.DeletionID), byLocatorKey(record.Kind, record.ResourceID), kvTaskKey(record.DeletionID)} {
				if _, err := store.Get(t.Context(), key); !errors.Is(err, kv.ErrNotFound) {
					t.Fatalf("reserved key %v error = %v, want not found", key, err)
				}
			}
		})
	}
}

func TestCreateOrGetRejectsMismatchedFixedLocator(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	other, err := New(KindWorkspace, "workspace-b", nil, ReasonResourceDelete, struct{}{}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("New(other): %v", err)
	}
	entries, err := KVEntries(other)
	if err != nil {
		t.Fatalf("KVEntries(other): %v", err)
	}
	entries = append(entries, kv.Entry{
		Key:   byLocatorKey(KindWorkspace, "workspace-a"),
		Value: []byte(other.DeletionID),
	})
	if err := store.BatchSet(ctx, entries); err != nil {
		t.Fatalf("BatchSet: %v", err)
	}
	record, err := New(KindWorkspace, "workspace-a", nil, ReasonResourceDelete, struct{}{}, time.Unix(2, 0))
	if err != nil {
		t.Fatalf("New(record): %v", err)
	}
	if _, _, err := CreateOrGet(ctx, store, record); err == nil {
		t.Fatal("CreateOrGet error = nil")
	}
	if exists, err := HasLocator(ctx, store, KindWorkspace, record.ResourceID); err == nil || exists {
		t.Fatalf("HasLocator = %v, %v, want integrity error", exists, err)
	}
}

func TestCreateOrGetRejectsMismatchedConcurrentWinner(t *testing.T) {
	ctx := context.Background()
	base := kv.NewMemory(nil)
	other, err := New(KindWorkspace, "workspace-b", nil, ReasonResourceDelete, struct{}{}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("New(other): %v", err)
	}
	entries, err := KVEntries(other)
	if err != nil {
		t.Fatalf("KVEntries(other): %v", err)
	}
	if err := base.BatchSet(ctx, entries); err != nil {
		t.Fatalf("BatchSet(other): %v", err)
	}
	store := createIfAbsentWinnerStore{Store: base, existingID: other.DeletionID}
	record, err := New(KindWorkspace, "workspace-a", nil, ReasonResourceDelete, struct{}{}, time.Unix(2, 0))
	if err != nil {
		t.Fatalf("New(record): %v", err)
	}
	if _, _, err := CreateOrGet(ctx, store, record); err == nil {
		t.Fatal("CreateOrGet error = nil")
	}
}

func TestCreateOrGetReusesOneDeletionEvent(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	owner := "peer-a"
	first, err := New(KindPeer, owner, &owner, ReasonPeerDelete, map[string]string{"public_key": owner}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("New(first): %v", err)
	}
	second, err := New(KindPeer, owner, &owner, ReasonPeerDelete, map[string]string{"public_key": owner}, time.Unix(2, 0))
	if err != nil {
		t.Fatalf("New(second): %v", err)
	}
	got, created, err := CreateOrGet(ctx, store, first)
	if err != nil || !created || got.DeletionID != first.DeletionID {
		t.Fatalf("CreateOrGet(first) = %#v, %v, %v", got, created, err)
	}
	got, created, err = CreateOrGet(ctx, store, second)
	if err != nil || created || got.DeletionID != first.DeletionID {
		t.Fatalf("CreateOrGet(second) = %#v, %v, %v", got, created, err)
	}
}

func TestCreateOrGetSupportsSlashSeparator(t *testing.T) {
	for _, fixture := range []struct {
		name string
		new  func(*testing.T) kv.Store
	}{
		{name: "memory", new: func(*testing.T) kv.Store {
			return kv.NewMemory(&kv.Options{Separator: '/'})
		}},
		{name: "badger", new: func(t *testing.T) kv.Store {
			store, err := kv.NewBadgerInMemory(&kv.Options{Separator: '/'})
			if err != nil {
				t.Fatalf("NewBadgerInMemory: %v", err)
			}
			t.Cleanup(func() { _ = store.Close() })
			return store
		}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			ctx := context.Background()
			store := fixture.new(t)
			record, err := New(KindWorkspace, "workspace-a", nil, ReasonResourceDelete, struct{}{}, time.Unix(1, 0))
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			got, created, err := CreateOrGet(ctx, store, record)
			if err != nil || !created || got.DeletionID != record.DeletionID {
				t.Fatalf("CreateOrGet = %#v, %v, %v", got, created, err)
			}
			stored, err := Get(ctx, store, record.DeletionID)
			if err != nil || stored.DeletionID != record.DeletionID {
				t.Fatalf("Get = %#v, %v", stored, err)
			}
		})
	}
}

func TestKVSourceLookup(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	source := KVSource{Store: store}
	record, err := New(KindWorkspace, "workspace-a", nil, ReasonResourceDelete, map[string]string{"name": "workspace-a"}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, _, err := CreateOrGet(ctx, store, record); err != nil {
		t.Fatalf("CreateOrGet: %v", err)
	}
	got, err := source.Get(ctx, record.DeletionID)
	if err != nil || got.DeletionID != record.DeletionID {
		t.Fatalf("Get = %#v, error = %v", got, err)
	}
	exists, err := source.HasLocator(ctx, Locator{Kind: KindWorkspace, ResourceID: record.ResourceID})
	if err != nil || !exists {
		t.Fatalf("HasLocator(existing) = %v, error = %v", exists, err)
	}
	exists, err = source.HasLocator(ctx, Locator{Kind: KindWorkspace, ResourceID: "missing"})
	if err != nil || exists {
		t.Fatalf("HasLocator(missing) = %v, error = %v", exists, err)
	}
	owner := "peer-a"
	if _, err := source.HasLocator(ctx, Locator{Kind: KindWorkspace, ResourceID: record.ResourceID, OwnerPublicKey: &owner}); err == nil {
		t.Fatal("HasLocator(owner filter) error = nil")
	}
}

func TestHasLocatorRejectsEmptyFixedLocator(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	if err := store.Set(ctx, byLocatorKey(KindPeer, "peer-a"), nil); err != nil {
		t.Fatalf("Set(empty locator): %v", err)
	}
	exists, err := HasLocator(ctx, store, KindPeer, "peer-a")
	if err == nil {
		t.Fatalf("HasLocator(empty locator) = %v, nil, want error", exists)
	}
}

func TestHasLocatorRejectsMissingFixedRecord(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	if err := store.Set(ctx, byLocatorKey(KindPeer, "peer-a"), []byte("10000000-0000-4000-8000-000000000001")); err != nil {
		t.Fatalf("Set(locator): %v", err)
	}
	exists, err := HasLocator(ctx, store, KindPeer, "peer-a")
	if err == nil {
		t.Fatalf("HasLocator(missing record) = %v, nil, want error", exists)
	}
}

func TestKVSourceRejectsMissingStore(t *testing.T) {
	source := KVSource{}
	if _, err := source.Get(context.Background(), "missing"); err == nil {
		t.Fatal("Get error = nil")
	}
	if _, err := source.HasLocator(context.Background(), Locator{Kind: KindPeer, ResourceID: "peer-a"}); err == nil {
		t.Fatal("HasLocator error = nil")
	}
}

func TestGetRejectsInvalidStoredEnvelope(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	record, err := New(KindPeer, "peer-a", nil, ReasonPeerDelete, map[string]string{"public_key": "peer-a"}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	record.DescriptorVersion++
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := store.Set(ctx, byIDKey(record.DeletionID), data); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if _, err := Get(ctx, store, record.DeletionID); err == nil {
		t.Fatal("Get error = nil")
	}
}

func TestGetRejectsMismatchedStoredDeletionID(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	record, err := New(KindPeer, "peer-a", nil, ReasonPeerDelete, map[string]string{"public_key": "peer-a"}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	otherID := "10000000-0000-4000-8000-000000000001"
	if err := store.Set(ctx, byIDKey(otherID), data); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if _, err := Get(ctx, store, otherID); err == nil {
		t.Fatal("Get error = nil")
	}
}
