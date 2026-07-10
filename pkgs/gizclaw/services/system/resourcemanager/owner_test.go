package resourcemanager

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
)

func TestApplyOwnedResourceAllowsMissingOwnerWithACL(t *testing.T) {
	manager := newACLResourceManager(t)
	manager.services.Workspaces = newFakeWorkspaces()

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Workspace",
		"metadata": {"name": "demo"},
		"spec": {
			"workflow_name": "workflow"
		}
	}`))
	if err != nil {
		t.Fatalf("Apply(without owner) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("Apply(without owner) action = %q, want %q", result.Action, apitypes.ApplyActionCreated)
	}
}

func TestApplyOwnedResourceManagesOwnerBinding(t *testing.T) {
	ctx := context.Background()
	manager := newACLResourceManager(t)
	workspaces := newFakeWorkspaces()
	manager.services.Workspaces = workspaces

	resource := workspaceResourceWithOwner(t, "demo", "owner-a")
	result, err := manager.Apply(ctx, resource)
	if err != nil {
		t.Fatalf("Apply(create) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("Apply(create) action = %q, want %q", result.Action, apitypes.ApplyActionCreated)
	}
	if workspaces.putCount != 1 {
		t.Fatalf("putCount after create = %d, want 1", workspaces.putCount)
	}

	role, err := manager.services.ACL.GetRole(ctx, resourceOwnerRole)
	if err != nil {
		t.Fatalf("GetRole(%q) error = %v", resourceOwnerRole, err)
	}
	if !permissionsEqual(role.Permissions, resourceOwnerPermissions) {
		t.Fatalf("owner role permissions = %#v, want %#v", role.Permissions, resourceOwnerPermissions)
	}
	assertWorkspaceOwnerBinding(t, manager, "demo", "owner-a")

	gotResource, err := manager.Get(ctx, apitypes.ResourceKindWorkspace, "demo")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	got, err := gotResource.AsWorkspaceResource()
	if err != nil {
		t.Fatalf("AsWorkspaceResource() error = %v", err)
	}
	if got.Metadata.OwnerPublicKey == nil || *got.Metadata.OwnerPublicKey != "owner-a" {
		t.Fatalf("owner_public_key = %#v, want owner-a", got.Metadata.OwnerPublicKey)
	}

	result, err = manager.Apply(ctx, workspaceResourceWithOwner(t, "demo", "owner-b"))
	if err != nil {
		t.Fatalf("Apply(owner update) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("Apply(owner update) action = %q, want %q", result.Action, apitypes.ApplyActionUpdated)
	}
	if workspaces.putCount != 1 {
		t.Fatalf("putCount after owner-only update = %d, want 1", workspaces.putCount)
	}
	assertWorkspaceOwnerBinding(t, manager, "demo", "owner-b")

	if _, err := manager.Delete(ctx, apitypes.ResourceKindWorkspace, "demo"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	bindingID := resourceOwnerPolicyBindingID(apitypes.ACLResourceKindWorkspace, "demo")
	if _, err := manager.services.ACL.GetPolicyBinding(ctx, bindingID); !errors.Is(err, acl.ErrPolicyBindingNotFound) {
		t.Fatalf("GetPolicyBinding(deleted owner) error = %v, want %v", err, acl.ErrPolicyBindingNotFound)
	}
}

func TestOwnedResourceOwnerReadsDeterministicBinding(t *testing.T) {
	ctx := context.Background()
	manager := newACLResourceManager(t)
	manager.services.Workspaces = newFakeWorkspaces()

	if _, err := manager.Apply(ctx, workspaceResourceWithOwner(t, "demo", "owner-a")); err != nil {
		t.Fatalf("Apply(create) error = %v", err)
	}
	if _, err := manager.services.ACL.CreatePolicyBinding(ctx, "extra-owner", 0, apitypes.ACLPolicy{
		Subject:  acl.PublicKeySubject("stale-owner"),
		Resource: acl.WorkspaceResource("demo"),
		Role:     resourceOwnerRole,
	}); err != nil {
		t.Fatalf("CreatePolicyBinding(extra owner) error = %v", err)
	}

	gotResource, err := manager.Get(ctx, apitypes.ResourceKindWorkspace, "demo")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	got, err := gotResource.AsWorkspaceResource()
	if err != nil {
		t.Fatalf("AsWorkspaceResource() error = %v", err)
	}
	if got.Metadata.OwnerPublicKey == nil || *got.Metadata.OwnerPublicKey != "owner-a" {
		t.Fatalf("owner_public_key = %#v, want deterministic owner-a", got.Metadata.OwnerPublicKey)
	}
}

func TestOwnedResourceOwnerIgnoresDriftedDeterministicBinding(t *testing.T) {
	ctx := context.Background()
	manager := newACLResourceManager(t)
	workspaces := newFakeWorkspaces()
	now := time.Now().UTC()
	workspaces.items["demo"] = apitypes.Workspace{
		CreatedAt:    now,
		Name:         "demo",
		UpdatedAt:    now,
		WorkflowName: "workflow",
	}
	manager.services.Workspaces = workspaces
	if _, err := manager.services.ACL.CreateRole(ctx, resourceOwnerRole, resourceOwnerPermissions); err != nil {
		t.Fatalf("CreateRole(owner) error = %v", err)
	}
	if _, err := manager.services.ACL.CreatePolicyBinding(ctx, resourceOwnerPolicyBindingID(apitypes.ACLResourceKindWorkspace, "demo"), 0, apitypes.ACLPolicy{
		Subject:  acl.PublicKeySubject("owner-a"),
		Resource: apitypes.ACLResource{Kind: apitypes.ACLResourceKindModel, Id: "wrong-resource"},
		Role:     resourceOwnerRole,
	}); err != nil {
		t.Fatalf("CreatePolicyBinding(drifted owner) error = %v", err)
	}

	gotResource, err := manager.Get(ctx, apitypes.ResourceKindWorkspace, "demo")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	got, err := gotResource.AsWorkspaceResource()
	if err != nil {
		t.Fatalf("AsWorkspaceResource() error = %v", err)
	}
	if got.Metadata.OwnerPublicKey != nil {
		t.Fatalf("owner_public_key = %#v, want nil for drifted owner binding", got.Metadata.OwnerPublicKey)
	}
}

func TestApplyOwnedResourceRepairsOwnerBindingAndRoleDrift(t *testing.T) {
	ctx := context.Background()
	manager := newACLResourceManager(t)
	workspaces := newFakeWorkspaces()
	now := time.Now().UTC()
	workspaces.items["demo"] = apitypes.Workspace{
		CreatedAt:    now,
		Name:         "demo",
		UpdatedAt:    now,
		WorkflowName: "workflow",
	}
	manager.services.Workspaces = workspaces
	if _, err := manager.services.ACL.CreateRole(ctx, resourceOwnerRole, apitypes.ACLPermissionList{apitypes.ACLPermissionRead}); err != nil {
		t.Fatalf("CreateRole(drifted owner role) error = %v", err)
	}
	expiresAt := now.Add(time.Hour)
	if _, err := manager.services.ACL.CreatePolicyBinding(ctx, resourceOwnerPolicyBindingID(apitypes.ACLResourceKindWorkspace, "demo"), 0, apitypes.ACLPolicy{
		ExpiresAt: &expiresAt,
		Subject:   acl.PublicKeySubject("owner-a"),
		Resource:  acl.WorkspaceResource("demo"),
		Role:      resourceOwnerRole,
	}); err != nil {
		t.Fatalf("CreatePolicyBinding(drifted owner) error = %v", err)
	}

	result, err := manager.Apply(ctx, workspaceResourceWithOwner(t, "demo", "owner-a"))
	if err != nil {
		t.Fatalf("Apply(repair owner) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("Apply(repair owner) action = %q, want %q", result.Action, apitypes.ApplyActionUpdated)
	}
	if workspaces.putCount != 0 {
		t.Fatalf("putCount after owner repair = %d, want 0", workspaces.putCount)
	}
	role, err := manager.services.ACL.GetRole(ctx, resourceOwnerRole)
	if err != nil {
		t.Fatalf("GetRole(%q) error = %v", resourceOwnerRole, err)
	}
	if !permissionsEqual(role.Permissions, resourceOwnerPermissions) {
		t.Fatalf("owner role permissions = %#v, want %#v", role.Permissions, resourceOwnerPermissions)
	}
	assertWorkspaceOwnerBinding(t, manager, "demo", "owner-a")
}

func TestPutOwnedResourceManagesOwnerBinding(t *testing.T) {
	ctx := context.Background()
	manager := newACLResourceManager(t)
	manager.services.Workspaces = newFakeWorkspaces()

	if _, err := manager.Put(ctx, workspaceResourceWithOwner(t, "demo", "owner-a")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	assertWorkspaceOwnerBinding(t, manager, "demo", "owner-a")
}

func TestPutOwnedResourceRollsBackOwnerWhenWriteFails(t *testing.T) {
	ctx := context.Background()
	manager := newACLResourceManager(t)
	workspaces := newFakeWorkspaces()
	workspaces.putStatus = 500
	manager.services.Workspaces = workspaces

	_, err := manager.Put(ctx, workspaceResourceWithOwner(t, "demo", "owner-a"))
	assertResourceError(t, err, 500, "INTERNAL_ERROR")
	bindingID := resourceOwnerPolicyBindingID(apitypes.ACLResourceKindWorkspace, "demo")
	if _, err := manager.services.ACL.GetPolicyBinding(ctx, bindingID); !errors.Is(err, acl.ErrPolicyBindingNotFound) {
		t.Fatalf("GetPolicyBinding(rolled back owner) error = %v, want %v", err, acl.ErrPolicyBindingNotFound)
	}
}

func TestPutExistingOwnedResourceAllowsMissingOwner(t *testing.T) {
	ctx := context.Background()
	manager := newACLResourceManager(t)
	workspaces := newFakeWorkspaces()
	now := time.Now().UTC()
	workspaces.items["demo"] = apitypes.Workspace{
		CreatedAt:    now,
		Name:         "demo",
		UpdatedAt:    now,
		WorkflowName: "old-workflow",
	}
	manager.services.Workspaces = workspaces

	_, err := manager.Put(ctx, mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Workspace",
		"metadata": {"name": "demo"},
		"spec": {
			"workflow_name": "new-workflow"
		}
	}`))
	if err != nil {
		t.Fatalf("Put(existing without owner) error = %v", err)
	}
	if workspaces.items["demo"].WorkflowName != "new-workflow" {
		t.Fatalf("workflow = %q, want new-workflow", workspaces.items["demo"].WorkflowName)
	}
}

func TestDeleteOwnedResourceRestoresOwnerWhenDeleteFails(t *testing.T) {
	ctx := context.Background()
	manager := newACLResourceManager(t)
	workspaces := newFakeWorkspaces()
	now := time.Now().UTC()
	workspaces.items["demo"] = apitypes.Workspace{
		CreatedAt:    now,
		Name:         "demo",
		UpdatedAt:    now,
		WorkflowName: "workflow",
	}
	workspaces.deleteStatus = 500
	manager.services.Workspaces = workspaces
	if _, err := manager.ensureOwnedResourceOwnerFromMetadata(ctx, apitypes.ACLResourceKindWorkspace, "demo", apitypes.ResourceMetadata{Name: "demo", OwnerPublicKey: ptr("owner-a")}); err != nil {
		t.Fatalf("ensureOwnedResourceOwnerFromMetadata() error = %v", err)
	}

	_, err := manager.Delete(ctx, apitypes.ResourceKindWorkspace, "demo")
	assertResourceError(t, err, 500, "INTERNAL_ERROR")
	assertWorkspaceOwnerBinding(t, manager, "demo", "owner-a")
}

func workspaceResourceWithOwner(t *testing.T, name, owner string) apitypes.Resource {
	t.Helper()
	resource := apitypes.WorkspaceResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.WorkspaceResourceKindWorkspace,
		Metadata: apitypes.ResourceMetadata{
			Name:           name,
			OwnerPublicKey: &owner,
		},
		Spec: apitypes.WorkspaceSpec{
			WorkflowName: "workflow",
		},
	}
	var out apitypes.Resource
	if err := out.FromWorkspaceResource(resource); err != nil {
		t.Fatalf("FromWorkspaceResource() error = %v", err)
	}
	return out
}

func assertWorkspaceOwnerBinding(t *testing.T, manager *Manager, name, owner string) {
	t.Helper()
	bindingID := resourceOwnerPolicyBindingID(apitypes.ACLResourceKindWorkspace, name)
	binding, err := manager.services.ACL.GetPolicyBinding(context.Background(), bindingID)
	if err != nil {
		t.Fatalf("GetPolicyBinding(%q) error = %v", bindingID, err)
	}
	if binding.Policy.Subject != acl.PublicKeySubject(owner) || binding.Policy.Resource != acl.WorkspaceResource(name) || binding.Policy.Role != resourceOwnerRole {
		t.Fatalf("owner binding = %#v, want owner %q workspace %q role %q", binding.Policy, owner, name, resourceOwnerRole)
	}
}

func permissionsEqual(left, right apitypes.ACLPermissionList) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[apitypes.ACLPermission]int, len(left))
	for _, permission := range left {
		seen[permission]++
	}
	for _, permission := range right {
		if seen[permission] == 0 {
			return false
		}
		seen[permission]--
	}
	return true
}
