package resourcemanager

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestApplyGameDefIgnoresOwnerManagedIcon(t *testing.T) {
	ctx := context.Background()
	iconName := "game-defs/demo/icon.png"
	item := apitypes.GameDef{
		CreatedAt: time.Now().UTC(),
		Icon:      &apitypes.Icon{Png: &iconName},
		Id:        "demo",
		Spec:      apitypes.GameDefSpec{DisplayName: "Demo"},
		UpdatedAt: time.Now().UTC(),
	}
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("Marshal game def error = %v", err)
	}
	store := kv.NewMemory(nil)
	if err := store.Set(ctx, kv.Key{"by-id", "demo"}, data); err != nil {
		t.Fatalf("seed game def error = %v", err)
	}
	catalog := &gameplay.Catalog{GameDefs: store}
	manager := New(Services{GameplayCatalog: catalog})

	unchanged, err := manager.Apply(ctx, mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "GameDef",
		"metadata": {"name": "demo"},
		"spec": {"display_name": "Demo"}
	}`))
	if err != nil {
		t.Fatalf("Apply without icon returned error: %v", err)
	}
	if unchanged.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("Apply without icon = %#v", unchanged)
	}

	updated, err := manager.Apply(ctx, mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "GameDef",
		"metadata": {"name": "demo"},
		"icon": {"png": "caller-controlled/icon.png"},
		"spec": {"display_name": "Updated demo"}
	}`))
	if err != nil {
		t.Fatalf("Apply with projected icon returned error: %v", err)
	}
	if updated.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("Apply with spec update = %#v", updated)
	}
	stored, err := catalog.GetGameDefByID(ctx, "demo")
	if err != nil {
		t.Fatalf("GetGameDefByID error = %v", err)
	}
	if stored.Icon == nil || stored.Icon.Png == nil || *stored.Icon.Png != iconName {
		t.Fatalf("stored icon = %#v, want owner-managed projection", stored.Icon)
	}
}
