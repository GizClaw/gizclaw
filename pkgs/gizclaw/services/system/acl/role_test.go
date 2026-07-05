package acl

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestRoleCRUD(t *testing.T) {
	server := migratedTestServer(t)
	ctx := context.Background()

	role, err := server.CreateRole(ctx, "workspace-reader", apitypes.ACLPermissionList{"read", "read"})
	if err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}
	if len(role.Permissions) != 1 || role.Permissions[0] != "read" {
		t.Fatalf("CreateRole() permissions = %#v", role.Permissions)
	}
	if _, err := server.CreateRole(ctx, "workspace-reader", apitypes.ACLPermissionList{"read"}); !errors.Is(err, ErrRoleAlreadyExists) {
		t.Fatalf("CreateRole(duplicate) error = %v, want %v", err, ErrRoleAlreadyExists)
	}
	if _, err := server.CreateRole(ctx, "bad-permission", apitypes.ACLPermissionList{"workspace.destroy"}); err == nil {
		t.Fatal("CreateRole(unsupported permission) error = nil")
	}
	stored, err := server.GetRole(ctx, "workspace-reader")
	if err != nil {
		t.Fatalf("GetRole() error = %v", err)
	}
	if stored.Name != role.Name {
		t.Fatalf("GetRole() = %q, want %q", stored.Name, role.Name)
	}
	stored, err = server.PutRole(ctx, "workspace-reader", apitypes.ACLPermissionList{"use"})
	if err != nil {
		t.Fatalf("PutRole() error = %v", err)
	}
	if len(stored.Permissions) != 1 || stored.Permissions[0] != "use" {
		t.Fatalf("PutRole() permissions = %#v", stored.Permissions)
	}
	if _, err := server.PutRole(ctx, "workspace-admin", apitypes.ACLPermissionList{"admin"}); err != nil {
		t.Fatalf("PutRole(new) error = %v", err)
	}
	roles, hasNext, nextCursor, err := server.ListRoles(ctx, ListRolesRequest{Limit: 1})
	if err != nil {
		t.Fatalf("ListRoles() error = %v", err)
	}
	if len(roles) != 1 || !hasNext || nextCursor == nil {
		t.Fatalf("ListRoles() = roles:%#v hasNext:%v next:%v", roles, hasNext, nextCursor)
	}
	roles, hasNext, nextCursor, err = server.ListRoles(ctx, ListRolesRequest{Cursor: *nextCursor, Limit: 1})
	if err != nil {
		t.Fatalf("ListRoles(next) error = %v", err)
	}
	if len(roles) != 1 || hasNext || nextCursor != nil {
		t.Fatalf("ListRoles(next) = roles:%#v hasNext:%v next:%v", roles, hasNext, nextCursor)
	}
	if _, err := server.DeleteRole(ctx, "workspace-reader"); err != nil {
		t.Fatalf("DeleteRole() error = %v", err)
	}
	if _, err := server.GetRole(ctx, "workspace-reader"); !errors.Is(err, ErrRoleNotFound) {
		t.Fatalf("GetRole(deleted) error = %v, want %v", err, ErrRoleNotFound)
	}
}

func TestListRolesEmptyPage(t *testing.T) {
	server := migratedTestServer(t)
	roles, hasNext, nextCursor, err := server.ListRoles(context.Background(), ListRolesRequest{})
	if err != nil {
		t.Fatalf("ListRoles(empty) error = %v", err)
	}
	if len(roles) != 0 || hasNext || nextCursor != nil {
		t.Fatalf("ListRoles(empty) = roles:%#v hasNext:%v next:%v", roles, hasNext, nextCursor)
	}
	if _, err := server.DeleteRole(context.Background(), "missing"); !errors.Is(err, ErrRoleNotFound) {
		t.Fatalf("DeleteRole(missing) error = %v, want %v", err, ErrRoleNotFound)
	}
}
