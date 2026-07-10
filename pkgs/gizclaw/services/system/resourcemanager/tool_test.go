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
	ownerPermissions := apitypes.ACLPermissionList{apitypes.ACLPermissionRead, apitypes.ACLPermissionUse, apitypes.ACLPermissionAdmin}
	if _, err := manager.services.ACL.CreateRole(ctx, toolkit.ToolOwnerRole, ownerPermissions); err != nil {
		t.Fatalf("CreateRole(resource owner) error = %v", err)
	}
	if _, err := manager.services.ACL.CreateRole(ctx, "tool-owner", ownerPermissions); err != nil {
		t.Fatalf("CreateRole(legacy tool owner) error = %v", err)
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
			genericBindingID := toolkit.ToolOwnerPolicyBindingID(tc.id, owner)
			if _, err := manager.services.ACL.CreatePolicyBinding(ctx, genericBindingID, 0, apitypes.ACLPolicy{Subject: acl.PublicKeySubject(owner), Resource: acl.ToolResource(tc.id), Role: toolkit.ToolOwnerRole}); err != nil {
				t.Fatalf("CreatePolicyBinding(generic) error = %v", err)
			}
			legacyBindingID := toolkit.LegacyToolOwnerPolicyBindingID(tc.id, owner)
			if _, err := manager.services.ACL.CreatePolicyBinding(ctx, legacyBindingID, 0, apitypes.ACLPolicy{Subject: acl.PublicKeySubject(owner), Resource: acl.ToolResource(tc.id), Role: "tool-owner"}); err != nil {
				t.Fatalf("CreatePolicyBinding(legacy) error = %v", err)
			}

			if err := tc.run(toolResource(t, tc.id)); err != nil {
				t.Fatalf("admin %s error = %v", tc.name, err)
			}
			if _, err := manager.services.ACL.GetPolicyBinding(ctx, genericBindingID); !errors.Is(err, acl.ErrPolicyBindingNotFound) {
				t.Fatalf("GetPolicyBinding(generic stale) error = %v, want %v", err, acl.ErrPolicyBindingNotFound)
			}
			if _, err := manager.services.ACL.GetPolicyBinding(ctx, legacyBindingID); !errors.Is(err, acl.ErrPolicyBindingNotFound) {
				t.Fatalf("GetPolicyBinding(legacy stale) error = %v, want %v", err, acl.ErrPolicyBindingNotFound)
			}
		})
	}
}

func TestToolResourceWritesWithoutMetadataOwnerPreserveLegacyOwnerBinding(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		run  func(*Manager, apitypes.Resource) error
	}{
		{name: "apply", run: func(manager *Manager, resource apitypes.Resource) error {
			_, err := manager.Apply(ctx, resource)
			return err
		}},
		{name: "put", run: func(manager *Manager, resource apitypes.Resource) error {
			_, err := manager.Put(ctx, resource)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager := newACLResourceManager(t)
			manager.services.Tools = &toolkit.Server{Store: kv.NewMemory(nil)}
			ownerPermissions := apitypes.ACLPermissionList{apitypes.ACLPermissionRead, apitypes.ACLPermissionUse, apitypes.ACLPermissionAdmin}
			if _, err := manager.services.ACL.CreateRole(ctx, "tool-owner", ownerPermissions); err != nil {
				t.Fatalf("CreateRole(legacy tool owner) error = %v", err)
			}
			owner := "owner-peer"
			method := "device.invoke"
			existing := toolkit.Tool{ID: "peer.legacy-owner." + tc.name, Source: toolkit.ToolSourceDevice, Enabled: true, OwnerPeer: &owner, InputSchema: jsonschema.Schema{Type: "object"}, Executor: toolkit.ToolExecutor{Kind: toolkit.ToolExecutorKindDeviceRPC, Method: &method, PeerID: &owner}}
			if _, err := manager.services.Tools.PutTool(ctx, existing); err != nil {
				t.Fatalf("PutTool(device) error = %v", err)
			}
			legacyBindingID := toolkit.LegacyToolOwnerPolicyBindingID(existing.ID, owner)
			if _, err := manager.services.ACL.CreatePolicyBinding(ctx, legacyBindingID, 0, apitypes.ACLPolicy{Subject: acl.PublicKeySubject(owner), Resource: acl.ToolResource(existing.ID), Role: "tool-owner"}); err != nil {
				t.Fatalf("CreatePolicyBinding(legacy) error = %v", err)
			}
			resource, err := resourceFromTool(existing)
			if err != nil {
				t.Fatalf("resourceFromTool() error = %v", err)
			}

			if err := tc.run(manager, resource); err != nil {
				t.Fatalf("%s without metadata owner error = %v", tc.name, err)
			}
			if _, err := manager.services.ACL.GetPolicyBinding(ctx, legacyBindingID); err != nil {
				t.Fatalf("GetPolicyBinding(legacy owner) error = %v", err)
			}
			if _, err := manager.services.ACL.GetPolicyBinding(ctx, toolkit.ToolOwnerPolicyBindingID(existing.ID, owner)); !errors.Is(err, acl.ErrPolicyBindingNotFound) {
				t.Fatalf("GetPolicyBinding(generic owner) error = %v, want %v", err, acl.ErrPolicyBindingNotFound)
			}
		})
	}
}

func TestToolResourcePutReturnsOwnerMetadata(t *testing.T) {
	ctx := context.Background()
	manager := newACLResourceManager(t)
	manager.services.Tools = &toolkit.Server{Store: kv.NewMemory(nil)}
	owner := "owner-peer"
	resource := toolResource(t, "system/music.play")
	item, err := resource.AsToolResource()
	if err != nil {
		t.Fatalf("AsToolResource() error = %v", err)
	}
	item.Metadata.OwnerPublicKey = &owner
	if err := resource.FromToolResource(item); err != nil {
		t.Fatalf("FromToolResource() error = %v", err)
	}

	storedResource, err := manager.Put(ctx, resource)
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	stored, err := storedResource.AsToolResource()
	if err != nil {
		t.Fatalf("AsToolResource(stored) error = %v", err)
	}
	if stored.Metadata.OwnerPublicKey == nil || *stored.Metadata.OwnerPublicKey != owner {
		t.Fatalf("owner_public_key = %#v, want %q", stored.Metadata.OwnerPublicKey, owner)
	}
}

func TestToolResourceOwnerOnlyApplyRemovesLegacyOwnerBinding(t *testing.T) {
	ctx := context.Background()
	manager := newACLResourceManager(t)
	manager.services.Tools = &toolkit.Server{Store: kv.NewMemory(nil)}
	owner := "old-owner"
	newOwner := "new-owner"
	method := "device.invoke"
	existing := toolkit.Tool{
		ID:          "peer.old-owner.music.play",
		Source:      toolkit.ToolSourceDevice,
		Enabled:     true,
		OwnerPeer:   &owner,
		InputSchema: jsonschema.Schema{Type: "object"},
		Executor:    toolkit.ToolExecutor{Kind: toolkit.ToolExecutorKindDeviceRPC, Method: &method, PeerID: &owner},
	}
	if _, err := manager.services.Tools.PutTool(ctx, existing); err != nil {
		t.Fatalf("PutTool(existing) error = %v", err)
	}
	ownerPermissions := apitypes.ACLPermissionList{apitypes.ACLPermissionRead, apitypes.ACLPermissionUse, apitypes.ACLPermissionAdmin}
	if _, err := manager.services.ACL.CreateRole(ctx, "tool-owner", ownerPermissions); err != nil {
		t.Fatalf("CreateRole(legacy tool owner) error = %v", err)
	}
	legacyBindingID := toolkit.LegacyToolOwnerPolicyBindingID(existing.ID, owner)
	if _, err := manager.services.ACL.CreatePolicyBinding(ctx, legacyBindingID, 0, apitypes.ACLPolicy{Subject: acl.PublicKeySubject(owner), Resource: acl.ToolResource(existing.ID), Role: "tool-owner"}); err != nil {
		t.Fatalf("CreatePolicyBinding(legacy) error = %v", err)
	}
	resource, err := resourceFromTool(existing)
	if err != nil {
		t.Fatalf("resourceFromTool() error = %v", err)
	}
	item, err := resource.AsToolResource()
	if err != nil {
		t.Fatalf("AsToolResource() error = %v", err)
	}
	item.Metadata.OwnerPublicKey = &newOwner
	if err := resource.FromToolResource(item); err != nil {
		t.Fatalf("FromToolResource() error = %v", err)
	}

	result, err := manager.Apply(ctx, resource)
	if err != nil {
		t.Fatalf("Apply(owner only) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("Apply(owner only) action = %q, want %q", result.Action, apitypes.ApplyActionUpdated)
	}
	if _, err := manager.services.ACL.GetPolicyBinding(ctx, legacyBindingID); !errors.Is(err, acl.ErrPolicyBindingNotFound) {
		t.Fatalf("GetPolicyBinding(legacy stale) error = %v, want %v", err, acl.ErrPolicyBindingNotFound)
	}
	binding, err := manager.services.ACL.GetPolicyBinding(ctx, toolkit.ToolOwnerPolicyBindingID(existing.ID, newOwner))
	if err != nil {
		t.Fatalf("GetPolicyBinding(generic owner) error = %v", err)
	}
	if binding.Policy.Subject != acl.PublicKeySubject(newOwner) {
		t.Fatalf("owner subject = %#v, want %q", binding.Policy.Subject, newOwner)
	}
}

func TestToolResourceSameSpecApplyRemovesLegacyWhenMetadataOwnerAlreadyCurrent(t *testing.T) {
	ctx := context.Background()
	manager := newACLResourceManager(t)
	manager.services.Tools = &toolkit.Server{Store: kv.NewMemory(nil)}
	owner := "old-owner"
	newOwner := "new-owner"
	method := "device.invoke"
	existing := toolkit.Tool{
		ID:          "peer.old-owner.music.status",
		Source:      toolkit.ToolSourceDevice,
		Enabled:     true,
		OwnerPeer:   &owner,
		InputSchema: jsonschema.Schema{Type: "object"},
		Executor:    toolkit.ToolExecutor{Kind: toolkit.ToolExecutorKindDeviceRPC, Method: &method, PeerID: &owner},
	}
	if _, err := manager.services.Tools.PutTool(ctx, existing); err != nil {
		t.Fatalf("PutTool(existing) error = %v", err)
	}
	ownerPermissions := apitypes.ACLPermissionList{apitypes.ACLPermissionRead, apitypes.ACLPermissionUse, apitypes.ACLPermissionAdmin}
	if _, err := manager.services.ACL.CreateRole(ctx, resourceOwnerRole, ownerPermissions); err != nil {
		t.Fatalf("CreateRole(resource owner) error = %v", err)
	}
	if _, err := manager.services.ACL.CreateRole(ctx, "tool-owner", ownerPermissions); err != nil {
		t.Fatalf("CreateRole(legacy tool owner) error = %v", err)
	}
	if _, err := manager.services.ACL.CreatePolicyBinding(ctx, toolkit.ToolOwnerPolicyBindingID(existing.ID, newOwner), 0, apitypes.ACLPolicy{Subject: acl.PublicKeySubject(newOwner), Resource: acl.ToolResource(existing.ID), Role: resourceOwnerRole}); err != nil {
		t.Fatalf("CreatePolicyBinding(generic owner) error = %v", err)
	}
	legacyBindingID := toolkit.LegacyToolOwnerPolicyBindingID(existing.ID, owner)
	if _, err := manager.services.ACL.CreatePolicyBinding(ctx, legacyBindingID, 0, apitypes.ACLPolicy{Subject: acl.PublicKeySubject(owner), Resource: acl.ToolResource(existing.ID), Role: "tool-owner"}); err != nil {
		t.Fatalf("CreatePolicyBinding(legacy) error = %v", err)
	}
	resource, err := resourceFromTool(existing)
	if err != nil {
		t.Fatalf("resourceFromTool() error = %v", err)
	}
	item, err := resource.AsToolResource()
	if err != nil {
		t.Fatalf("AsToolResource() error = %v", err)
	}
	item.Metadata.OwnerPublicKey = &newOwner
	if err := resource.FromToolResource(item); err != nil {
		t.Fatalf("FromToolResource() error = %v", err)
	}

	result, err := manager.Apply(ctx, resource)
	if err != nil {
		t.Fatalf("Apply(same spec metadata owner) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("Apply(same spec metadata owner) action = %q, want %q", result.Action, apitypes.ApplyActionUpdated)
	}
	if _, err := manager.services.ACL.GetPolicyBinding(ctx, legacyBindingID); !errors.Is(err, acl.ErrPolicyBindingNotFound) {
		t.Fatalf("GetPolicyBinding(legacy stale) error = %v, want %v", err, acl.ErrPolicyBindingNotFound)
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
