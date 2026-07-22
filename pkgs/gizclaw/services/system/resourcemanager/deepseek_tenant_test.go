package resourcemanager

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestApplyDeepSeekTenantCreatesUpdatesAndSkipsUnchanged(t *testing.T) {
	manager := newModelManager()
	resource := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "DeepSeekTenant",
		"metadata": {"name": "default"},
		"spec": {
			"credential_name": "deepseek",
			"base_url": "https://deepseek.example.com"
		}
	}`)

	result, err := manager.Apply(context.Background(), resource)
	if err != nil {
		t.Fatalf("Apply(create DeepSeekTenant) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("Apply(create DeepSeekTenant) action = %s", result.Action)
	}
	result, err = manager.Apply(context.Background(), resource)
	if err != nil {
		t.Fatalf("Apply(unchanged DeepSeekTenant) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("Apply(unchanged DeepSeekTenant) action = %s", result.Action)
	}

	updated := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "DeepSeekTenant",
		"metadata": {"name": "default"},
		"spec": {
			"credential_name": "deepseek",
			"base_url": "https://deepseek.example.com",
			"description": "DeepSeek project"
		}
	}`)
	result, err = manager.Apply(context.Background(), updated)
	if err != nil {
		t.Fatalf("Apply(update DeepSeekTenant) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("Apply(update DeepSeekTenant) action = %s", result.Action)
	}
}

func TestPutGetDeleteDeepSeekTenantResource(t *testing.T) {
	manager := newModelManager()
	resource := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "DeepSeekTenant",
		"metadata": {"name": "default"},
		"spec": {
			"credential_name": "deepseek",
			"base_url": "https://deepseek.example.com"
		}
	}`)

	stored, err := manager.Put(context.Background(), resource)
	if err != nil {
		t.Fatalf("Put(DeepSeekTenant) error = %v", err)
	}
	tenant, err := stored.AsDeepSeekTenantResource()
	if err != nil {
		t.Fatalf("AsDeepSeekTenantResource(Put) error = %v", err)
	}
	if tenant.Spec.CredentialName != "deepseek" {
		t.Fatalf("Put(DeepSeekTenant) credential_name = %s", tenant.Spec.CredentialName)
	}

	got, err := manager.Get(context.Background(), apitypes.ResourceKindDeepSeekTenant, "default")
	if err != nil {
		t.Fatalf("Get(DeepSeekTenant) error = %v", err)
	}
	gotTenant, err := got.AsDeepSeekTenantResource()
	if err != nil {
		t.Fatalf("AsDeepSeekTenantResource(Get) error = %v", err)
	}
	if gotTenant.Metadata.Name != "default" {
		t.Fatalf("Get(DeepSeekTenant) metadata.name = %s", gotTenant.Metadata.Name)
	}

	deleted, err := manager.Delete(context.Background(), apitypes.ResourceKindDeepSeekTenant, "default")
	if err != nil {
		t.Fatalf("Delete(DeepSeekTenant) error = %v", err)
	}
	deletedTenant, err := deleted.AsDeepSeekTenantResource()
	if err != nil {
		t.Fatalf("AsDeepSeekTenantResource(Delete) error = %v", err)
	}
	if deletedTenant.Metadata.Name != "default" {
		t.Fatalf("Delete(DeepSeekTenant) metadata.name = %s", deletedTenant.Metadata.Name)
	}
	_, err = manager.Get(context.Background(), apitypes.ResourceKindDeepSeekTenant, "default")
	assertResourceError(t, err, 404, "RESOURCE_NOT_FOUND")
	_, err = manager.Delete(context.Background(), apitypes.ResourceKindDeepSeekTenant, "default")
	assertResourceError(t, err, 404, "RESOURCE_NOT_FOUND")
}

func TestDeepSeekTenantServiceResponseErrors(t *testing.T) {
	manager := New(Services{ProviderTenants: errorModelService{}})
	_, _, err := manager.getDeepSeekTenant(context.Background(), "tenant")
	assertResourceError(t, err, 500, "INTERNAL_ERROR")

	err = manager.putDeepSeekTenant(context.Background(), "tenant", adminhttp.DeepSeekTenantUpsert{})
	assertResourceError(t, err, 500, "INTERNAL_ERROR")
	manager = New(Services{ProviderTenants: errorModelService{deepSeekPutStatus: 400}})
	err = manager.putDeepSeekTenant(context.Background(), "tenant", adminhttp.DeepSeekTenantUpsert{})
	assertResourceError(t, err, 400, "INVALID_DEEPSEEK_TENANT")

	manager = New(Services{ProviderTenants: errorModelService{}})
	_, _, err = manager.deleteDeepSeekTenant(context.Background(), "tenant")
	assertResourceError(t, err, 500, "INTERNAL_ERROR")
}

func TestDeepSeekTenantMissingServiceErrors(t *testing.T) {
	manager := New(Services{})
	resource := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "DeepSeekTenant",
		"metadata": {"name": "default"},
		"spec": {"credential_name": "deepseek"}
	}`)

	if _, err := manager.Get(context.Background(), apitypes.ResourceKindDeepSeekTenant, "default"); err == nil {
		t.Fatal("Get(DeepSeekTenant) error = nil")
	}
	if _, err := manager.Put(context.Background(), resource); err == nil {
		t.Fatal("Put(DeepSeekTenant) error = nil")
	}
	if _, err := manager.Delete(context.Background(), apitypes.ResourceKindDeepSeekTenant, "default"); err == nil {
		t.Fatal("Delete(DeepSeekTenant) error = nil")
	}
	if _, err := manager.Apply(context.Background(), resource); err == nil {
		t.Fatal("Apply(DeepSeekTenant) error = nil")
	}
}

func TestApplyDeepSeekTenantRejectsInvalidHeader(t *testing.T) {
	manager := newModelManager()
	resource := mustResource(t, `{
		"apiVersion": "unsupported",
		"kind": "DeepSeekTenant",
		"metadata": {"name": "default"},
		"spec": {"credential_name": "deepseek"}
	}`)
	_, err := manager.Apply(context.Background(), resource)
	assertResourceError(t, err, 400, "UNSUPPORTED_RESOURCE_VERSION")
}
