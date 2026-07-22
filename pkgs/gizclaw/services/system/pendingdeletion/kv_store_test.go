package pendingdeletion

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestKVEntriesPreserveMultipleDeletionEvents(t *testing.T) {
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
	for _, record := range []Record{first, second} {
		entries, err := KVEntries(record)
		if err != nil {
			t.Fatalf("KVEntries(%s): %v", record.DeletionID, err)
		}
		if err := store.BatchSet(ctx, entries); err != nil {
			t.Fatalf("BatchSet(%s): %v", record.DeletionID, err)
		}
	}
	if first.DeletionID == second.DeletionID {
		t.Fatal("deletion IDs collided")
	}
	for _, want := range []Record{first, second} {
		got, err := Get(ctx, store, want.DeletionID)
		if err != nil {
			t.Fatalf("Get(%s): %v", want.DeletionID, err)
		}
		if got.ResourceID != owner || got.Kind != KindPeer || got.DescriptorVersion != DescriptorVersion {
			t.Fatalf("Get(%s) = %#v", want.DeletionID, got)
		}
	}
	count := 0
	for _, err := range store.List(ctx, byLocatorPrefix(KindPeer, owner)) {
		if err != nil {
			t.Fatalf("List locator: %v", err)
		}
		count++
	}
	if count != 2 {
		t.Fatalf("locator entries = %d, want 2", count)
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
	entries, err := KVEntries(record)
	if err != nil {
		t.Fatalf("KVEntries: %v", err)
	}
	if err := store.BatchSet(ctx, entries); err != nil {
		t.Fatalf("BatchSet: %v", err)
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
