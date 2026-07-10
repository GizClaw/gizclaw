package resourcemanager

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestToolResourceApplyGetPutDelete(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	tools := &toolkit.Server{Store: kv.NewMemory(nil), Now: func() time.Time { return now }}
	manager := New(Services{Tools: tools})
	const id = "system/music.play"
	resource := toolResource(t, id)

	result, err := manager.Apply(ctx, resource)
	if err != nil || result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("Apply(create) = %#v, %v", result, err)
	}
	result, err = manager.Apply(ctx, resource)
	if err != nil || result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("Apply(unchanged) = %#v, %v", result, err)
	}

	gotResource, err := manager.Get(ctx, apitypes.ResourceKindTool, id)
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
	tool, err := tools.GetTool(ctx, id)
	if err != nil || tool.CreatedAt.Equal(tool.UpdatedAt) {
		t.Fatalf("stored timestamps = %s/%s, %v", tool.CreatedAt, tool.UpdatedAt, err)
	}

	deleted, err := manager.Delete(ctx, apitypes.ResourceKindTool, id)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if item, err := deleted.AsToolResource(); err != nil || item.Metadata.Name != id {
		t.Fatalf("Delete() resource = %#v, %v", item, err)
	}
	if _, err := manager.Get(ctx, apitypes.ResourceKindTool, id); err == nil {
		t.Fatal("Get(deleted) error = nil")
	}
}

func TestToolResourceAdminWritesRemoveStaleOwnerBindings(t *testing.T) {
	ctx := context.Background()
	manager := newACLResourceManager(t)
	manager.services.Tools = &toolkit.Server{Store: kv.NewMemory(nil)}
	if _, err := manager.services.ACL.CreateRole(ctx, toolkit.ToolOwnerRole, apitypes.ACLPermissionList{apitypes.ACLPermissionRead, apitypes.ACLPermissionUse, apitypes.ACLPermissionAdmin}); err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}

	for _, tc := range []struct {
		name string
		id   string
		run  func(apitypes.Resource) error
	}{
		{name: "apply", id: "peer.owner.apply", run: func(resource apitypes.Resource) error { _, err := manager.Apply(ctx, resource); return err }},
		{name: "put", id: "peer.owner.put", run: func(resource apitypes.Resource) error { _, err := manager.Put(ctx, resource); return err }},
		{name: "delete", id: "peer.owner.delete", run: func(_ apitypes.Resource) error {
			_, err := manager.Delete(ctx, apitypes.ResourceKindTool, "peer.owner.delete")
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner := "owner-peer"
			method := "device.invoke"
			device := toolkit.Tool{ID: tc.id, Source: toolkit.ToolSourceDevice, Enabled: true, OwnerPeer: &owner, InputSchema: jsonschema.Schema{Type: "object"}, Executor: toolkit.ToolExecutor{Kind: toolkit.ToolExecutorKindDeviceRPC, Method: &method, PeerID: &owner}}
			if _, err := manager.services.Tools.PutTool(ctx, device); err != nil {
				t.Fatalf("PutTool(device) error = %v", err)
			}
			bindingID := toolkit.ToolOwnerPolicyBindingID(tc.id, owner)
			if _, err := manager.services.ACL.CreatePolicyBinding(ctx, bindingID, 0, apitypes.ACLPolicy{Subject: acl.PublicKeySubject(owner), Resource: acl.ToolResource(tc.id), Role: toolkit.ToolOwnerRole}); err != nil {
				t.Fatalf("CreatePolicyBinding() error = %v", err)
			}

			if err := tc.run(toolResource(t, tc.id)); err != nil {
				t.Fatalf("admin %s error = %v", tc.name, err)
			}
			if _, err := manager.services.ACL.GetPolicyBinding(ctx, bindingID); !errors.Is(err, acl.ErrPolicyBindingNotFound) {
				t.Fatalf("GetPolicyBinding(stale) error = %v, want %v", err, acl.ErrPolicyBindingNotFound)
			}
		})
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
