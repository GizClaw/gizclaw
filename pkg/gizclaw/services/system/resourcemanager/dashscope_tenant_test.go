package resourcemanager

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func TestApplyDashScopeTenantCreatesUpdatesAndSkipsUnchanged(t *testing.T) {
	manager := newModelManager()
	resource := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "DashScopeTenant",
		"metadata": {"name": "default"},
		"spec": {
			"credential_name": "dashscope",
			"base_url": "https://dashscope.example.com"
		}
	}`)

	result, err := manager.Apply(context.Background(), resource)
	if err != nil {
		t.Fatalf("Apply(create DashScopeTenant) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("Apply(create DashScopeTenant) action = %s", result.Action)
	}
	result, err = manager.Apply(context.Background(), resource)
	if err != nil {
		t.Fatalf("Apply(unchanged DashScopeTenant) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("Apply(unchanged DashScopeTenant) action = %s", result.Action)
	}

	updated := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "DashScopeTenant",
		"metadata": {"name": "default"},
		"spec": {
			"credential_name": "dashscope",
			"base_url": "https://dashscope.example.com",
			"description": "DashScope project"
		}
	}`)
	result, err = manager.Apply(context.Background(), updated)
	if err != nil {
		t.Fatalf("Apply(update DashScopeTenant) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("Apply(update DashScopeTenant) action = %s", result.Action)
	}
}

func TestPutGetDeleteDashScopeTenantResource(t *testing.T) {
	manager := newModelManager()
	resource := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "DashScopeTenant",
		"metadata": {"name": "default"},
		"spec": {
			"credential_name": "dashscope",
			"base_url": "https://dashscope.example.com"
		}
	}`)

	stored, err := manager.Put(context.Background(), resource)
	if err != nil {
		t.Fatalf("Put(DashScopeTenant) error = %v", err)
	}
	tenant, err := stored.AsDashScopeTenantResource()
	if err != nil {
		t.Fatalf("AsDashScopeTenantResource(Put) error = %v", err)
	}
	if tenant.Spec.CredentialName != "dashscope" {
		t.Fatalf("Put(DashScopeTenant) credential_name = %s", tenant.Spec.CredentialName)
	}

	got, err := manager.Get(context.Background(), apitypes.ResourceKindDashScopeTenant, "default")
	if err != nil {
		t.Fatalf("Get(DashScopeTenant) error = %v", err)
	}
	gotTenant, err := got.AsDashScopeTenantResource()
	if err != nil {
		t.Fatalf("AsDashScopeTenantResource(Get) error = %v", err)
	}
	if gotTenant.Metadata.Name != "default" {
		t.Fatalf("Get(DashScopeTenant) metadata.name = %s", gotTenant.Metadata.Name)
	}

	deleted, err := manager.Delete(context.Background(), apitypes.ResourceKindDashScopeTenant, "default")
	if err != nil {
		t.Fatalf("Delete(DashScopeTenant) error = %v", err)
	}
	deletedTenant, err := deleted.AsDashScopeTenantResource()
	if err != nil {
		t.Fatalf("AsDashScopeTenantResource(Delete) error = %v", err)
	}
	if deletedTenant.Metadata.Name != "default" {
		t.Fatalf("Delete(DashScopeTenant) metadata.name = %s", deletedTenant.Metadata.Name)
	}
	_, err = manager.Get(context.Background(), apitypes.ResourceKindDashScopeTenant, "default")
	assertResourceError(t, err, 404, "RESOURCE_NOT_FOUND")
	_, err = manager.Delete(context.Background(), apitypes.ResourceKindDashScopeTenant, "default")
	assertResourceError(t, err, 404, "RESOURCE_NOT_FOUND")
}

func TestDashScopeTenantServiceResponseErrors(t *testing.T) {
	manager := New(Services{ProviderTenants: errorModelService{}})
	_, _, err := manager.getDashScopeTenant(context.Background(), "tenant")
	assertResourceError(t, err, 500, "INTERNAL_ERROR")

	err = manager.putDashScopeTenant(context.Background(), "tenant", adminservice.DashScopeTenantUpsert{})
	assertResourceError(t, err, 500, "INTERNAL_ERROR")
	manager = New(Services{ProviderTenants: errorModelService{dashScopePutStatus: 400}})
	err = manager.putDashScopeTenant(context.Background(), "tenant", adminservice.DashScopeTenantUpsert{})
	assertResourceError(t, err, 400, "INVALID_DASHSCOPE_TENANT")

	manager = New(Services{ProviderTenants: errorModelService{}})
	_, _, err = manager.deleteDashScopeTenant(context.Background(), "tenant")
	assertResourceError(t, err, 500, "INTERNAL_ERROR")
}

func TestDashScopeTenantMissingServiceErrors(t *testing.T) {
	manager := New(Services{})
	resource := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "DashScopeTenant",
		"metadata": {"name": "default"},
		"spec": {"credential_name": "dashscope"}
	}`)

	if _, err := manager.Get(context.Background(), apitypes.ResourceKindDashScopeTenant, "default"); err == nil {
		t.Fatal("Get(DashScopeTenant) error = nil")
	}
	if _, err := manager.Put(context.Background(), resource); err == nil {
		t.Fatal("Put(DashScopeTenant) error = nil")
	}
	if _, err := manager.Delete(context.Background(), apitypes.ResourceKindDashScopeTenant, "default"); err == nil {
		t.Fatal("Delete(DashScopeTenant) error = nil")
	}
	if _, err := manager.Apply(context.Background(), resource); err == nil {
		t.Fatal("Apply(DashScopeTenant) error = nil")
	}
}

func TestApplyDashScopeTenantRejectsInvalidHeader(t *testing.T) {
	manager := newModelManager()
	resource := mustResource(t, `{
		"apiVersion": "unsupported",
		"kind": "DashScopeTenant",
		"metadata": {"name": "default"},
		"spec": {"credential_name": "dashscope"}
	}`)
	_, err := manager.Apply(context.Background(), resource)
	assertResourceError(t, err, 400, "UNSUPPORTED_RESOURCE_VERSION")
}
