package pendingdeletion

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestCreateOrGetMigratesLegacyLocator(t *testing.T) {
	for _, fixture := range []struct {
		name string
		new  func(*testing.T) kv.Store
	}{
		{name: "memory", new: func(*testing.T) kv.Store { return kv.NewMemory(nil) }},
		{name: "badger", new: func(t *testing.T) kv.Store {
			store, err := kv.NewBadgerInMemory(nil)
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
			legacy, err := New(KindWorkspace, "workspace-a", nil, ReasonResourceDelete, map[string]string{"name": "workspace-a"}, time.Unix(1, 0))
			if err != nil {
				t.Fatalf("New(legacy): %v", err)
			}
			entries, err := KVEntries(legacy)
			if err != nil {
				t.Fatalf("KVEntries(legacy): %v", err)
			}
			legacyLocator := append(legacyByLocatorPrefix(legacy.Kind, legacy.ResourceID), legacy.DeletionID)
			entries = append(entries, kv.Entry{Key: legacyLocator})
			if err := store.BatchSet(ctx, entries); err != nil {
				t.Fatalf("BatchSet(legacy): %v", err)
			}
			retry, err := New(KindWorkspace, "workspace-a", nil, ReasonResourceDelete, map[string]string{"name": "workspace-a"}, time.Unix(2, 0))
			if err != nil {
				t.Fatalf("New(retry): %v", err)
			}

			got, created, err := CreateOrGet(ctx, store, retry)
			if err != nil || created || got.DeletionID != legacy.DeletionID {
				t.Fatalf("CreateOrGet(retry) = %#v, %v, %v", got, created, err)
			}
			fixedID, err := store.Get(ctx, byLocatorKey(legacy.Kind, legacy.ResourceID))
			if err != nil || string(fixedID) != legacy.DeletionID {
				t.Fatalf("Get(fixed locator) = %q, %v", fixedID, err)
			}
			if _, err := store.Get(ctx, byIDKey(retry.DeletionID)); !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("Get(retry record) error = %v, want ErrNotFound", err)
			}
		})
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
