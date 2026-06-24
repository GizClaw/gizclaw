package resourcemanager

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/system/acl"
	_ "modernc.org/sqlite"
)

func TestApplyACLViewResource(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	server := &acl.Server{DB: db}
	if err := server.Migration(context.Background()); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	manager := New(Services{ACL: server})
	resource := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "ACLView",
		"metadata": {"name": "under-12"},
		"spec": {
			"description": "Content for children under 12."
		}
	}`)

	result, err := manager.Apply(context.Background(), resource)
	if err != nil {
		t.Fatalf("Apply(create) error = %v", err)
	}
	if result.Kind != apitypes.ResourceKindACLView || result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("Apply(create) = %+v", result)
	}
	result, err = manager.Apply(context.Background(), resource)
	if err != nil {
		t.Fatalf("Apply(unchanged) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("Apply(unchanged) = %+v", result)
	}
	stored, err := manager.Get(context.Background(), apitypes.ResourceKindACLView, "under-12")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	data := string(mustMarshal(t, stored))
	if !strings.Contains(data, `"kind":"ACLView"`) || !strings.Contains(data, `"description":"Content for children under 12."`) {
		t.Fatalf("Get() resource = %s", data)
	}
	deleted, err := manager.Delete(context.Background(), apitypes.ResourceKindACLView, "under-12")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	data = string(mustMarshal(t, deleted))
	if !strings.Contains(data, `"kind":"ACLView"`) || !strings.Contains(data, `"name":"under-12"`) {
		t.Fatalf("Delete() resource = %s", data)
	}
}

func TestApplyACLRoleResource(t *testing.T) {
	manager := newACLResourceManager(t)
	resource := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "ACLRole",
		"metadata": {"name": "e2e-client"},
		"spec": {
			"permissions": ["workspace.admin", "workflow.admin", "model.read"]
		}
	}`)

	result, err := manager.Apply(context.Background(), resource)
	if err != nil {
		t.Fatalf("Apply(create) error = %v", err)
	}
	if result.Kind != apitypes.ResourceKindACLRole || result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("Apply(create) = %+v", result)
	}
	result, err = manager.Apply(context.Background(), resource)
	if err != nil {
		t.Fatalf("Apply(unchanged) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("Apply(unchanged) = %+v", result)
	}
	stored, err := manager.Get(context.Background(), apitypes.ResourceKindACLRole, "e2e-client")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	data := string(mustMarshal(t, stored))
	if !strings.Contains(data, `"kind":"ACLRole"`) || !strings.Contains(data, `"workspace.admin"`) {
		t.Fatalf("Get() resource = %s", data)
	}
	deleted, err := manager.Delete(context.Background(), apitypes.ResourceKindACLRole, "e2e-client")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	data = string(mustMarshal(t, deleted))
	if !strings.Contains(data, `"kind":"ACLRole"`) || !strings.Contains(data, `"name":"e2e-client"`) {
		t.Fatalf("Delete() resource = %s", data)
	}
}

func TestApplyACLPolicyBindingResource(t *testing.T) {
	manager := newACLResourceManager(t)
	if _, err := manager.services.ACL.PutRole(context.Background(), "workspace-reader", apitypes.ACLPermissionList{"workspace.read"}); err != nil {
		t.Fatalf("PutRole() error = %v", err)
	}
	resource := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "ACLPolicyBinding",
		"metadata": {"name": "e2e-workspace-read"},
		"spec": {
			"subject": {"kind": "pk", "id": "client-public-key"},
			"resource": {"kind": "workspace", "id": "e2e-workspace"},
			"role": "workspace-reader"
		}
	}`)

	result, err := manager.Apply(context.Background(), resource)
	if err != nil {
		t.Fatalf("Apply(create) error = %v", err)
	}
	if result.Kind != apitypes.ResourceKindACLPolicyBinding || result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("Apply(create) = %+v", result)
	}
	result, err = manager.Apply(context.Background(), resource)
	if err != nil {
		t.Fatalf("Apply(unchanged) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("Apply(unchanged) = %+v", result)
	}
	if err := manager.services.ACL.Authorize(context.Background(), acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject("client-public-key"),
		Resource:   acl.WorkspaceResource("e2e-workspace"),
		Permission: apitypes.ACLPermissionWorkspaceRead,
	}); err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	stored, err := manager.Get(context.Background(), apitypes.ResourceKindACLPolicyBinding, "e2e-workspace-read")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	data := string(mustMarshal(t, stored))
	if !strings.Contains(data, `"kind":"ACLPolicyBinding"`) || !strings.Contains(data, `"role":"workspace-reader"`) {
		t.Fatalf("Get() resource = %s", data)
	}
	deleted, err := manager.Delete(context.Background(), apitypes.ResourceKindACLPolicyBinding, "e2e-workspace-read")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	data = string(mustMarshal(t, deleted))
	if !strings.Contains(data, `"kind":"ACLPolicyBinding"`) || !strings.Contains(data, `"name":"e2e-workspace-read"`) {
		t.Fatalf("Delete() resource = %s", data)
	}
	if err := manager.services.ACL.Authorize(context.Background(), acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject("client-public-key"),
		Resource:   acl.WorkspaceResource("e2e-workspace"),
		Permission: apitypes.ACLPermissionWorkspaceRead,
	}); !errors.Is(err, acl.ErrDenied) {
		t.Fatalf("Authorize(deleted) error = %v, want %v", err, acl.ErrDenied)
	}
}

func newACLResourceManager(t *testing.T) *Manager {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	server := &acl.Server{DB: db}
	if err := server.Migration(context.Background()); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	return New(Services{ACL: server})
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal value: %v", err)
	}
	return data
}
