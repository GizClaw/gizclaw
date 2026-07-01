//go:build gizclaw_e2e

package admin_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestAdminAPIACLRolesListGetAndMutation(t *testing.T) {
	env := newAdminAPIHarness(t)

	list, err := env.api.ListACLRolesWithResponse(env.ctx, &adminservice.ListACLRolesParams{Limit: ptr[int32](100)})
	if err != nil {
		t.Fatalf("list ACL roles: %v", err)
	}
	requireStatusOK(t, list, list.Body)
	if list.JSON200 == nil {
		t.Fatalf("list ACL roles missing JSON200")
	}
	requireName(t, list.JSON200.Items, "default-client", func(item apitypes.ACLRole) string { return item.Name })

	get, err := env.api.GetACLRoleWithResponse(env.ctx, "default-client")
	if err != nil {
		t.Fatalf("get ACL role: %v", err)
	}
	requireStatusOK(t, get, get.Body)
	if get.JSON200 == nil || get.JSON200.Name != "default-client" || len(get.JSON200.Permissions) == 0 {
		t.Fatalf("get ACL role = %#v", get.JSON200)
	}

	name := mutationName("acl-role")
	_, _ = env.api.DeleteACLRoleWithResponse(env.ctx, name)
	created, err := env.api.CreateACLRoleWithResponse(env.ctx, adminservice.ACLRoleUpsert{
		Name:        name,
		Permissions: apitypes.ACLPermissionList{apitypes.ACLPermissionWorkflowRead},
	})
	if err != nil {
		t.Fatalf("create ACL role: %v", err)
	}
	requireStatusOK(t, created, created.Body)
	if created.JSON200 == nil || created.JSON200.Name != name {
		t.Fatalf("created ACL role = %#v", created.JSON200)
	}
	deleted, err := env.api.DeleteACLRoleWithResponse(env.ctx, name)
	if err != nil {
		t.Fatalf("delete ACL role: %v", err)
	}
	requireStatusOK(t, deleted, deleted.Body)
}
