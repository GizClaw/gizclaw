package acl

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func TestPolicyBindingCRUDAndAuthorize(t *testing.T) {
	server := migratedTestServer(t)
	ctx := context.Background()
	if _, err := server.CreateRole(ctx, "workspace-reader", apitypes.ACLPermissionList{"workspace.read"}); err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}
	policy := apitypes.ACLPolicy{
		Subject:  PublicKeySubject("subject-a"),
		Resource: WorkspaceResource("workspace-a"),
		Role:     "workspace-reader",
	}
	binding, err := server.CreatePolicyBinding(ctx, "binding-a", 0, policy)
	if err != nil {
		t.Fatalf("CreatePolicyBinding() error = %v", err)
	}
	if binding.Id != "binding-a" {
		t.Fatalf("CreatePolicyBinding() id = %q", binding.Id)
	}
	if _, err := server.CreatePolicyBinding(ctx, "binding-a", 0, policy); !errors.Is(err, ErrPolicyBindingAlreadyExists) {
		t.Fatalf("CreatePolicyBinding(duplicate) error = %v, want %v", err, ErrPolicyBindingAlreadyExists)
	}
	if _, err := server.CreatePolicyBinding(ctx, "binding-b", 0, policy); !errors.Is(err, ErrPolicyBindingAlreadyExists) {
		t.Fatalf("CreatePolicyBinding(conflict) error = %v, want %v", err, ErrPolicyBindingAlreadyExists)
	}
	if _, err := server.PutPolicyBinding(ctx, "binding-c", 0, policy); !errors.Is(err, ErrPolicyBindingAlreadyExists) {
		t.Fatalf("PutPolicyBinding(conflict) error = %v, want %v", err, ErrPolicyBindingAlreadyExists)
	}
	if err := server.Authorize(ctx, AuthorizeRequest{
		Subject:    PublicKeySubject("subject-a"),
		Resource:   WorkspaceResource("workspace-a"),
		Permission: "workspace.read",
	}); err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	if err := server.Authorize(ctx, AuthorizeRequest{
		Subject:    PublicKeySubject("subject-a"),
		Resource:   WorkspaceResource("workspace-a"),
		Permission: "workspace.use",
	}); !errors.Is(err, ErrDenied) {
		t.Fatalf("Authorize(ungranted) error = %v, want %v", err, ErrDenied)
	}
	if _, err := server.PutRole(ctx, "workspace-reader", apitypes.ACLPermissionList{"workspace.use"}); err != nil {
		t.Fatalf("PutRole() error = %v", err)
	}
	if err := server.Authorize(ctx, AuthorizeRequest{
		Subject:    PublicKeySubject("subject-a"),
		Resource:   WorkspaceResource("workspace-a"),
		Permission: "workspace.use",
	}); err != nil {
		t.Fatalf("Authorize(updated role) error = %v", err)
	}
	if _, err := server.DeletePolicyBinding(ctx, "binding-a"); err != nil {
		t.Fatalf("DeletePolicyBinding() error = %v", err)
	}
	if _, err := server.GetPolicyBinding(ctx, "binding-a"); !errors.Is(err, ErrPolicyBindingNotFound) {
		t.Fatalf("GetPolicyBinding(deleted) error = %v, want %v", err, ErrPolicyBindingNotFound)
	}
}

func TestCreatePolicyBindingGeneratesID(t *testing.T) {
	server := migratedTestServer(t)
	ctx := context.Background()
	if _, err := server.CreateRole(ctx, "workspace-reader", apitypes.ACLPermissionList{"workspace.read"}); err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}
	longSubjectID := strings.Repeat("a", 64)
	binding, err := server.CreatePolicyBinding(ctx, "", 0, apitypes.ACLPolicy{
		Subject:  PublicKeySubject(longSubjectID),
		Resource: WorkspaceResource("demo-workspace"),
		Role:     "workspace-reader",
	})
	if err != nil {
		t.Fatalf("CreatePolicyBinding(empty id) error = %v", err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{12}-`).MatchString(binding.Id) {
		t.Fatalf("generated id = %q, want 12 hex prefix", binding.Id)
	}
	if strings.Contains(binding.Id, longSubjectID) {
		t.Fatalf("generated id contains full subject id: %q", binding.Id)
	}
	if _, err := server.GetPolicyBinding(ctx, binding.Id); err != nil {
		t.Fatalf("GetPolicyBinding(generated id) error = %v", err)
	}
}

func TestPolicyBindingAllowsProviderScopedResourceIDs(t *testing.T) {
	server := migratedTestServer(t)
	ctx := context.Background()
	if _, err := server.CreateRole(ctx, "voice-reader", apitypes.ACLPermissionList{"voice.read"}); err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}
	resource := VoiceResource("minimax-tenant:minimax-cn:Arabic_CalmWoman")
	if _, err := server.CreatePolicyBinding(ctx, "binding-provider-voice", 0, apitypes.ACLPolicy{
		Subject:  ViewSubject("play-openai"),
		Resource: resource,
		Role:     "voice-reader",
	}); err != nil {
		t.Fatalf("CreatePolicyBinding(provider voice) error = %v", err)
	}
	if err := server.Authorize(ctx, AuthorizeRequest{
		Subject:    ViewSubject("play-openai"),
		Resource:   resource,
		Permission: apitypes.ACLPermissionVoiceRead,
	}); err != nil {
		t.Fatalf("Authorize(provider voice) error = %v", err)
	}
}

func TestPolicyBindingListPutAndCleanupExpired(t *testing.T) {
	now := time.Date(2026, 5, 20, 1, 2, 3, 0, time.UTC)
	server := migratedTestServer(t)
	server.Now = func() time.Time { return now }
	ctx := context.Background()
	if _, err := server.CreateRole(ctx, "workspace-reader", apitypes.ACLPermissionList{"workspace.read"}); err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}
	expiredAt := now.Add(-time.Second)
	if _, err := server.CreatePolicyBinding(ctx, "binding-a", 0, apitypes.ACLPolicy{
		Subject:  PublicKeySubject("subject-a"),
		Resource: WorkspaceResource("workspace-a"),
		Role:     "workspace-reader",
	}); err != nil {
		t.Fatalf("CreatePolicyBinding(active) error = %v", err)
	}
	if _, err := server.CreatePolicyBinding(ctx, "binding-expired", 0, apitypes.ACLPolicy{
		Subject:   PublicKeySubject("subject-a"),
		Resource:  WorkspaceResource("workspace-old"),
		Role:      "workspace-reader",
		ExpiresAt: &expiredAt,
	}); err != nil {
		t.Fatalf("CreatePolicyBinding(expired) error = %v", err)
	}
	bindings, hasNext, nextCursor, err := server.ListPolicyBindings(ctx, ListPolicyBindingsRequest{Limit: 1})
	if err != nil {
		t.Fatalf("ListPolicyBindings() error = %v", err)
	}
	if len(bindings) != 1 || !hasNext || nextCursor == nil {
		t.Fatalf("ListPolicyBindings() = bindings:%#v hasNext:%v next:%v", bindings, hasNext, nextCursor)
	}
	updated, err := server.PutPolicyBinding(ctx, "binding-a", 0, apitypes.ACLPolicy{
		Subject:  PublicKeySubject("subject-a"),
		Resource: WorkspaceResource("workspace-b"),
		Role:     "workspace-reader",
	})
	if err != nil {
		t.Fatalf("PutPolicyBinding() error = %v", err)
	}
	if updated.Policy.Resource.Id != "workspace-b" {
		t.Fatalf("PutPolicyBinding() resource = %q, want workspace-b", updated.Policy.Resource.Id)
	}
	n, err := server.CleanupExpired(ctx, 10)
	if err != nil {
		t.Fatalf("CleanupExpired() error = %v", err)
	}
	if n != 1 {
		t.Fatalf("CleanupExpired() = %d, want 1", n)
	}
	if _, err := server.GetPolicyBinding(ctx, "binding-expired"); !errors.Is(err, ErrPolicyBindingNotFound) {
		t.Fatalf("GetPolicyBinding(expired) error = %v, want %v", err, ErrPolicyBindingNotFound)
	}
}

func TestListPolicyBindingsFilters(t *testing.T) {
	server := migratedTestServer(t)
	ctx := context.Background()
	if _, err := server.CreateRole(ctx, "workspace-reader", apitypes.ACLPermissionList{"workspace.read"}); err != nil {
		t.Fatalf("CreateRole(reader) error = %v", err)
	}
	if _, err := server.CreateRole(ctx, "workspace-user", apitypes.ACLPermissionList{"workspace.use"}); err != nil {
		t.Fatalf("CreateRole(user) error = %v", err)
	}
	bindings := []struct {
		id     string
		policy apitypes.ACLPolicy
	}{
		{"read-demo", apitypes.ACLPolicy{Subject: PublicKeySubject("subject-a"), Resource: WorkspaceResource("demo"), Role: "workspace-reader"}},
		{"use-demo", apitypes.ACLPolicy{Subject: PublicKeySubject("subject-a"), Resource: WorkspaceResource("other"), Role: "workspace-user"}},
		{"read-other-subject", apitypes.ACLPolicy{Subject: PublicKeySubject("subject-b"), Resource: WorkspaceResource("demo"), Role: "workspace-reader"}},
	}
	for _, binding := range bindings {
		if _, err := server.CreatePolicyBinding(ctx, binding.id, 0, binding.policy); err != nil {
			t.Fatalf("CreatePolicyBinding(%s) error = %v", binding.id, err)
		}
	}
	items, hasNext, nextCursor, err := server.ListPolicyBindings(ctx, ListPolicyBindingsRequest{
		SubjectKind:  SubjectKindPublicKey,
		SubjectID:    "subject-a",
		ResourceKind: ResourceKindWorkspace,
		Permission:   "workspace.read",
	})
	if err != nil {
		t.Fatalf("ListPolicyBindings(filters) error = %v", err)
	}
	if len(items) != 1 || items[0].Id != "read-demo" || hasNext || nextCursor != nil {
		t.Fatalf("ListPolicyBindings(filters) = items:%#v hasNext:%v next:%v", items, hasNext, nextCursor)
	}
	items, _, _, err = server.ListPolicyBindings(ctx, ListPolicyBindingsRequest{ResourceID: "other", Role: "workspace-user"})
	if err != nil {
		t.Fatalf("ListPolicyBindings(resource/role) error = %v", err)
	}
	if len(items) != 1 || items[0].Id != "use-demo" {
		t.Fatalf("ListPolicyBindings(resource/role) = %#v", items)
	}
}

func TestListPolicyBindingsResourceIDPrefixAndPagination(t *testing.T) {
	server := migratedTestServer(t)
	ctx := context.Background()
	if _, err := server.CreateRole(ctx, "workspace-reader", apitypes.ACLPermissionList{"workspace.read"}); err != nil {
		t.Fatalf("CreateRole(reader) error = %v", err)
	}
	if _, err := server.CreateRole(ctx, "workspace-user", apitypes.ACLPermissionList{"workspace.use"}); err != nil {
		t.Fatalf("CreateRole(user) error = %v", err)
	}
	bindings := []struct {
		id       string
		subject  string
		resource string
		role     string
	}{
		{id: "binding-social-a", subject: "subject-a", resource: "social-direct-a", role: "workspace-reader"},
		{id: "binding-social-b", subject: "subject-a", resource: "social-direct-b", role: "workspace-reader"},
		{id: "binding-social-c-use", subject: "subject-a", resource: "social-direct-c", role: "workspace-user"},
		{id: "binding-group-a", subject: "subject-a", resource: "social-group-a", role: "workspace-reader"},
		{id: "binding-other-subject", subject: "subject-b", resource: "social-direct-c", role: "workspace-reader"},
	}
	for _, binding := range bindings {
		if _, err := server.CreatePolicyBinding(ctx, binding.id, 0, apitypes.ACLPolicy{
			Subject:  PublicKeySubject(binding.subject),
			Resource: WorkspaceResource(binding.resource),
			Role:     binding.role,
		}); err != nil {
			t.Fatalf("CreatePolicyBinding(%s) error = %v", binding.id, err)
		}
	}

	first, hasNext, nextCursor, err := server.ListPolicyBindings(ctx, ListPolicyBindingsRequest{
		Limit:            1,
		SubjectKind:      SubjectKindPublicKey,
		SubjectID:        "subject-a",
		ResourceKind:     ResourceKindWorkspace,
		ResourceIDPrefix: "social-direct-",
		Permission:       "workspace.read",
	})
	if err != nil {
		t.Fatalf("ListPolicyBindings(prefix page 1) error = %v", err)
	}
	if len(first) != 1 || first[0].Id != "binding-social-a" || !hasNext || nextCursor == nil || *nextCursor != "binding-social-a" {
		t.Fatalf("ListPolicyBindings(prefix page 1) = items:%#v hasNext:%v next:%v", first, hasNext, nextCursor)
	}
	second, hasNext, nextCursor, err := server.ListPolicyBindings(ctx, ListPolicyBindingsRequest{
		Cursor:           *nextCursor,
		Limit:            1,
		SubjectKind:      SubjectKindPublicKey,
		SubjectID:        "subject-a",
		ResourceKind:     ResourceKindWorkspace,
		ResourceIDPrefix: "social-direct-",
		Permission:       "workspace.read",
	})
	if err != nil {
		t.Fatalf("ListPolicyBindings(prefix page 2) error = %v", err)
	}
	if len(second) != 1 || second[0].Id != "binding-social-b" || hasNext || nextCursor != nil {
		t.Fatalf("ListPolicyBindings(prefix page 2) = items:%#v hasNext:%v next:%v", second, hasNext, nextCursor)
	}

	exact, _, _, err := server.ListPolicyBindings(ctx, ListPolicyBindingsRequest{
		ResourceID:       "social-group-a",
		ResourceIDPrefix: "social-direct-",
	})
	if err != nil {
		t.Fatalf("ListPolicyBindings(exact and prefix) error = %v", err)
	}
	if len(exact) != 0 {
		t.Fatalf("ListPolicyBindings(exact and prefix) = %#v, want none", exact)
	}
}

func TestListPolicyBindingsDisplayOrder(t *testing.T) {
	server := migratedTestServer(t)
	ctx := context.Background()
	if _, err := server.CreateRole(ctx, "workspace-reader", apitypes.ACLPermissionList{"workspace.read"}); err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}
	bindings := []struct {
		id           string
		displayOrder float64
		resourceID   string
	}{
		{id: "binding-c", displayOrder: 20, resourceID: "workspace-c"},
		{id: "binding-a", displayOrder: 10, resourceID: "workspace-a"},
		{id: "binding-b", displayOrder: 10, resourceID: "workspace-b"},
	}
	for _, binding := range bindings {
		if _, err := server.CreatePolicyBinding(ctx, binding.id, binding.displayOrder, apitypes.ACLPolicy{
			Subject:  PublicKeySubject("subject-a"),
			Resource: WorkspaceResource(binding.resourceID),
			Role:     "workspace-reader",
		}); err != nil {
			t.Fatalf("CreatePolicyBinding(%s) error = %v", binding.id, err)
		}
	}
	items, hasNext, nextCursor, err := server.ListPolicyBindings(ctx, ListPolicyBindingsRequest{
		Limit:     2,
		OrderBy:   PolicyBindingOrderByDisplayOrder,
		SubjectID: "subject-a",
	})
	if err != nil {
		t.Fatalf("ListPolicyBindings(display_order) error = %v", err)
	}
	if len(items) != 2 || items[0].Id != "binding-a" || items[1].Id != "binding-b" || !hasNext || nextCursor == nil {
		t.Fatalf("ListPolicyBindings(display_order) = items:%#v hasNext:%v next:%v", items, hasNext, nextCursor)
	}
	if *nextCursor != "binding-b" {
		t.Fatalf("ListPolicyBindings(display_order) cursor = %q, want binding-b", *nextCursor)
	}
	items, hasNext, nextCursor, err = server.ListPolicyBindings(ctx, ListPolicyBindingsRequest{
		Cursor:    *nextCursor,
		Limit:     2,
		OrderBy:   PolicyBindingOrderByDisplayOrder,
		SubjectID: "subject-a",
	})
	if err != nil {
		t.Fatalf("ListPolicyBindings(display_order page 2) error = %v", err)
	}
	if len(items) != 1 || items[0].Id != "binding-c" || hasNext || nextCursor != nil {
		t.Fatalf("ListPolicyBindings(display_order page 2) = items:%#v hasNext:%v next:%v", items, hasNext, nextCursor)
	}
	updated, err := server.PutPolicyBinding(ctx, "binding-c", 5, apitypes.ACLPolicy{
		Subject:  PublicKeySubject("subject-a"),
		Resource: WorkspaceResource("workspace-c"),
		Role:     "workspace-reader",
	})
	if err != nil {
		t.Fatalf("PutPolicyBinding(display_order) error = %v", err)
	}
	if updated.DisplayOrder != 5 {
		t.Fatalf("PutPolicyBinding(display_order) = %v, want 5", updated.DisplayOrder)
	}
}

func TestPutPolicyBindingCreatesNewBinding(t *testing.T) {
	server := migratedTestServer(t)
	ctx := context.Background()
	if _, err := server.CreateRole(ctx, "workspace-reader", apitypes.ACLPermissionList{"workspace.read"}); err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}
	binding, err := server.PutPolicyBinding(ctx, "binding-a", 0, apitypes.ACLPolicy{
		Subject:  PublicKeySubject("subject-a"),
		Resource: WorkspaceResource("workspace-a"),
		Role:     "workspace-reader",
	})
	if err != nil {
		t.Fatalf("PutPolicyBinding(new) error = %v", err)
	}
	if binding.Id != "binding-a" {
		t.Fatalf("PutPolicyBinding(new) id = %q", binding.Id)
	}
	bindings, hasNext, nextCursor, err := server.ListPolicyBindings(ctx, ListPolicyBindingsRequest{Cursor: "binding-a"})
	if err != nil {
		t.Fatalf("ListPolicyBindings(empty page) error = %v", err)
	}
	if len(bindings) != 0 || hasNext || nextCursor != nil {
		t.Fatalf("ListPolicyBindings(empty page) = bindings:%#v hasNext:%v next:%v", bindings, hasNext, nextCursor)
	}
	if _, err := server.DeletePolicyBinding(ctx, "missing"); !errors.Is(err, ErrPolicyBindingNotFound) {
		t.Fatalf("DeletePolicyBinding(missing) error = %v, want %v", err, ErrPolicyBindingNotFound)
	}
}
