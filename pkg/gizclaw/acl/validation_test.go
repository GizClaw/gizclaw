package acl

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func TestValidationErrors(t *testing.T) {
	ctx := context.Background()
	if _, err := (*Server)(nil).CreateRole(ctx, "role", nil); !errors.Is(err, errNilServer) {
		t.Fatalf("nil CreateRole() error = %v, want %v", err, errNilServer)
	}
	if _, err := (&Server{}).CreateRole(ctx, "role", nil); !errors.Is(err, errNilDB) {
		t.Fatalf("missing db CreateRole() error = %v, want %v", err, errNilDB)
	}
	server := migratedTestServer(t)
	if _, err := server.CreateRole(ctx, "", nil); err == nil {
		t.Fatal("CreateRole(empty name) error = nil")
	}
	if _, err := server.CreateRole(ctx, "bad", apitypes.ACLPermissionList{""}); err == nil {
		t.Fatal("CreateRole(empty permission) error = nil")
	}
	if _, err := server.CreatePolicyBinding(ctx, "binding", 0, apitypes.ACLPolicy{
		Subject:  PublicKeySubject("subject-a"),
		Resource: WorkspaceResource("workspace-a"),
		Role:     "missing",
	}); !errors.Is(err, ErrRoleNotFound) {
		t.Fatalf("CreatePolicyBinding(missing role) error = %v, want %v", err, ErrRoleNotFound)
	}
	if _, err := server.PutPolicyBinding(ctx, "binding", 0, apitypes.ACLPolicy{
		Resource: WorkspaceResource("workspace-a"),
		Role:     "missing",
	}); err == nil {
		t.Fatal("PutPolicyBinding(invalid subject) error = nil")
	}
	if _, _, _, err := server.ListPolicyBindings(ctx, ListPolicyBindingsRequest{
		ResourceKind: apitypes.ACLResourceKind("bad"),
	}); err == nil {
		t.Fatal("ListPolicyBindings(invalid kind) error = nil")
	}
	if _, _, _, err := server.ListPolicyBindings(ctx, ListPolicyBindingsRequest{
		OrderBy: "bad",
	}); err == nil {
		t.Fatal("ListPolicyBindings(invalid order) error = nil")
	}
}

func TestNormalizeListParams(t *testing.T) {
	cursor, limit := normalizeListParams(" next ", -1)
	if cursor != "next" || limit != defaultListLimit {
		t.Fatalf("normalizeListParams(default) = %q %d", cursor, limit)
	}
	_, limit = normalizeListParams("", maxListLimit+1)
	if limit != maxListLimit {
		t.Fatalf("normalizeListParams(clamp) = %d, want %d", limit, maxListLimit)
	}
}

func TestPlannedResourceKindsAndPermissionsAreValid(t *testing.T) {
	for _, kind := range []apitypes.ACLResourceKind{
		ResourceKindPet,
		ResourceKindWallet,
		ResourceKindContact,
		ResourceKindFriend,
		ResourceKindFriendReq,
		ResourceKindGroup,
		ResourceKindCall,
		ResourceKindGameResult,
		ResourceKindReward,
	} {
		if !kind.Valid() {
			t.Fatalf("resource kind %q is not valid", kind)
		}
		if _, err := CanonicalResource(apitypes.ACLResource{Kind: kind, Id: "demo"}); err != nil {
			t.Fatalf("CanonicalResource(%q) error = %v", kind, err)
		}
		for _, suffix := range []string{"read", "use", "admin"} {
			permission := apitypes.ACLPermission(string(kind) + "." + suffix)
			if !permission.Valid() {
				t.Fatalf("permission %q is not valid", permission)
			}
		}
	}
}
