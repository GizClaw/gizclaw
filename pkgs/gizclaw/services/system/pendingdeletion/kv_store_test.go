package pendingdeletion

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

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
