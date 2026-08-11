package resourcemanager

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/memorylayout"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestMemoryLayoutResourceLifecycle(t *testing.T) {
	store, err := kv.NewBadgerInMemory(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	manager := New(Services{MemoryLayouts: &memorylayout.Server{Store: store}})
	resource := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "MemoryLayout",
		"metadata": {"id": "pet-memory"},
		"spec": {
			"flowcraft": {
				"extraction": {"model": "pet-care.extract", "mode": "two_pass"},
				"embedding": {"model": "pet-care.embedding"},
				"bbh": {"search_overfetch": 2},
				"lanes": [{"name": "owner_profile", "kind": "note"}],
				"write": {"mode": "sync", "tier": "general"}
			},
			"mem0": {"custom_instructions": "Keep stable preferences."},
			"volc_mem0": {"strategies": [{"name": "pet-facts", "type": "user_preference", "custom_instructions": "Keep pet facts."}]}
		}
	}`)

	created, err := manager.Apply(context.Background(), resource)
	if err != nil {
		t.Fatalf("Apply(create MemoryLayout) error = %v", err)
	}
	if created.Action != apitypes.ApplyActionCreated {
		t.Fatalf("Apply(create MemoryLayout) action = %s", created.Action)
	}
	resource = withResourceID(t, resource, *created.Id)
	unchanged, err := manager.Apply(context.Background(), resource)
	if err != nil {
		t.Fatalf("Apply(unchanged MemoryLayout) error = %v", err)
	}
	if unchanged.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("Apply(unchanged MemoryLayout) action = %s", unchanged.Action)
	}

	id := *created.Id
	got, err := manager.Get(context.Background(), apitypes.ResourceKindMemoryLayout, id)
	if err != nil {
		t.Fatalf("Get(MemoryLayout) error = %v", err)
	}
	layout, err := got.AsMemoryLayoutResource()
	if err != nil {
		t.Fatalf("AsMemoryLayoutResource(Get) error = %v", err)
	}
	if layout.Spec.Flowcraft.Extraction.Model != "pet-care.extract" {
		t.Fatalf("Get(MemoryLayout) extraction model = %q", layout.Spec.Flowcraft.Extraction.Model)
	}

	deleted, err := manager.Delete(context.Background(), apitypes.ResourceKindMemoryLayout, id)
	if err != nil {
		t.Fatalf("Delete(MemoryLayout) error = %v", err)
	}
	deletedLayout, err := deleted.AsMemoryLayoutResource()
	if err != nil {
		t.Fatalf("AsMemoryLayoutResource(Delete) error = %v", err)
	}
	if metadataID(t, deletedLayout.Metadata) != "pet-memory" {
		t.Fatalf("Delete(MemoryLayout) name = %q", metadataID(t, deletedLayout.Metadata))
	}
	if _, err := manager.Get(context.Background(), apitypes.ResourceKindMemoryLayout, id); err == nil {
		t.Fatal("Get(deleted MemoryLayout) succeeded")
	}
}

func TestMemoryLayoutResourceApplyComparesNormalizedSpec(t *testing.T) {
	store, err := kv.NewBadgerInMemory(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	manager := New(Services{MemoryLayouts: &memorylayout.Server{Store: store}})
	resource := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "MemoryLayout",
		"metadata": {"id": "adventure"},
		"spec": {
			"flowcraft": {
				"extraction": {"model": " extraction ", "mode": "two_pass"},
				"bbh": {"search_overfetch": 2},
				"lanes": [{"name": " adventure_state ", "kind": "state"}],
				"write": {"mode": "sync", "tier": "general"}
			},
			"mem0": {"custom_instructions": " Keep confirmed state.\n"},
			"volc_mem0": {"strategies": [{"name": " adventure_state ", "type": "summary", "custom_instructions": " Keep confirmed state.\n"}]}
		}
	}`)

	created, err := manager.Apply(t.Context(), resource)
	if err != nil || created.Action != apitypes.ApplyActionCreated {
		t.Fatalf("Apply(create normalized MemoryLayout) = %#v, %v", created, err)
	}
	unchanged, err := manager.Apply(t.Context(), resource)
	if err != nil || unchanged.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("Apply(reapply normalized MemoryLayout) = %#v, %v", unchanged, err)
	}
}
