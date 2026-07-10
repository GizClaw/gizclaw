package resourcemanager

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestToolResourceApplyGetPutDelete(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	tools := &toolkit.Server{Store: kv.NewMemory(nil), Now: func() time.Time { return now }}
	manager := New(Services{Tools: tools})
	resource := toolResource(t, "system.music.play")

	result, err := manager.Apply(ctx, resource)
	if err != nil || result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("Apply(create) = %#v, %v", result, err)
	}
	result, err = manager.Apply(ctx, resource)
	if err != nil || result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("Apply(unchanged) = %#v, %v", result, err)
	}

	gotResource, err := manager.Get(ctx, apitypes.ResourceKindTool, "system.music.play")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	got, err := gotResource.AsToolResource()
	if err != nil {
		t.Fatalf("AsToolResource() error = %v", err)
	}
	if got.Spec.Enabled == nil || !*got.Spec.Enabled || got.Spec.InputSchema.Properties["query"].Type != "string" {
		t.Fatalf("stored Tool spec = %#v", got.Spec)
	}

	now = now.Add(time.Minute)
	disabled := false
	got.Spec.Enabled = &disabled
	description := "updated"
	got.Spec.Description = &description
	var putResource apitypes.Resource
	if err := putResource.FromToolResource(got); err != nil {
		t.Fatalf("FromToolResource() error = %v", err)
	}
	storedResource, err := manager.Put(ctx, putResource)
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	stored, err := storedResource.AsToolResource()
	if err != nil || stored.Spec.Enabled == nil || *stored.Spec.Enabled || stored.Spec.Description == nil || *stored.Spec.Description != description {
		t.Fatalf("Put() stored = %#v, %v", stored, err)
	}
	tool, err := tools.GetTool(ctx, "system.music.play")
	if err != nil || tool.CreatedAt.Equal(tool.UpdatedAt) {
		t.Fatalf("stored timestamps = %s/%s, %v", tool.CreatedAt, tool.UpdatedAt, err)
	}

	deleted, err := manager.Delete(ctx, apitypes.ResourceKindTool, "system.music.play")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if item, err := deleted.AsToolResource(); err != nil || item.Metadata.Name != "system.music.play" {
		t.Fatalf("Delete() resource = %#v, %v", item, err)
	}
	if _, err := manager.Get(ctx, apitypes.ResourceKindTool, "system.music.play"); err == nil {
		t.Fatal("Get(deleted) error = nil")
	}
}

func toolResource(t *testing.T, id string) apitypes.Resource {
	t.Helper()
	name := "play_music"
	executor := "music.play"
	resource := apitypes.ToolResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.ToolResourceKindTool,
		Metadata:   apitypes.ResourceMetadata{Name: id},
		Spec: apitypes.ToolSpec{
			Name:        &name,
			Source:      apitypes.ToolSourceBuiltin,
			InputSchema: jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{"query": {Type: "string"}}},
			Executor:    apitypes.ToolExecutor{Kind: apitypes.ToolExecutorKindBuiltin, Name: &executor},
		},
	}
	var out apitypes.Resource
	if err := out.FromToolResource(resource); err != nil {
		t.Fatalf("FromToolResource() error = %v", err)
	}
	return out
}
