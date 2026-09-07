package pendingdeletion

import (
	"context"
	"encoding/json"
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
