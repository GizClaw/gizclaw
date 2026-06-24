package resourcemanager

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func TestApplyGeminiTenantCreatesUpdatesAndSkipsUnchanged(t *testing.T) {
	manager := newModelManager()
	resource := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "GeminiTenant",
		"metadata": {"name": "default"},
		"spec": {
			"credential_name": "gemini",
			"project_id": "project-a",
			"location": "global"
		}
	}`)

	result, err := manager.Apply(context.Background(), resource)
	if err != nil {
		t.Fatalf("Apply(create GeminiTenant) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("Apply(create GeminiTenant) action = %s", result.Action)
	}
	result, err = manager.Apply(context.Background(), resource)
	if err != nil {
		t.Fatalf("Apply(unchanged GeminiTenant) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("Apply(unchanged GeminiTenant) action = %s", result.Action)
	}

	updated := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "GeminiTenant",
		"metadata": {"name": "default"},
		"spec": {
			"credential_name": "gemini",
			"project_id": "project-a",
			"location": "global",
			"description": "Gemini project"
		}
	}`)
	result, err = manager.Apply(context.Background(), updated)
	if err != nil {
		t.Fatalf("Apply(update GeminiTenant) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("Apply(update GeminiTenant) action = %s", result.Action)
	}
}

func TestPutGetDeleteGeminiTenantResource(t *testing.T) {
	manager := newModelManager()
	resource := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "GeminiTenant",
		"metadata": {"name": "default"},
		"spec": {
			"credential_name": "gemini",
			"project_id": "project-a"
		}
	}`)

	stored, err := manager.Put(context.Background(), resource)
	if err != nil {
		t.Fatalf("Put(GeminiTenant) error = %v", err)
	}
	tenant, err := stored.AsGeminiTenantResource()
	if err != nil {
		t.Fatalf("AsGeminiTenantResource(Put) error = %v", err)
	}
	if tenant.Spec.CredentialName != "gemini" {
		t.Fatalf("Put(GeminiTenant) credential_name = %s", tenant.Spec.CredentialName)
	}

	got, err := manager.Get(context.Background(), apitypes.ResourceKindGeminiTenant, "default")
	if err != nil {
		t.Fatalf("Get(GeminiTenant) error = %v", err)
	}
	gotTenant, err := got.AsGeminiTenantResource()
	if err != nil {
		t.Fatalf("AsGeminiTenantResource(Get) error = %v", err)
	}
	if gotTenant.Metadata.Name != "default" {
		t.Fatalf("Get(GeminiTenant) metadata.name = %s", gotTenant.Metadata.Name)
	}

	deleted, err := manager.Delete(context.Background(), apitypes.ResourceKindGeminiTenant, "default")
	if err != nil {
		t.Fatalf("Delete(GeminiTenant) error = %v", err)
	}
	deletedTenant, err := deleted.AsGeminiTenantResource()
	if err != nil {
		t.Fatalf("AsGeminiTenantResource(Delete) error = %v", err)
	}
	if deletedTenant.Metadata.Name != "default" {
		t.Fatalf("Delete(GeminiTenant) metadata.name = %s", deletedTenant.Metadata.Name)
	}
	_, err = manager.Get(context.Background(), apitypes.ResourceKindGeminiTenant, "default")
	assertResourceError(t, err, 404, "RESOURCE_NOT_FOUND")
	_, err = manager.Delete(context.Background(), apitypes.ResourceKindGeminiTenant, "default")
	assertResourceError(t, err, 404, "RESOURCE_NOT_FOUND")
}

func TestGeminiTenantServiceResponseErrors(t *testing.T) {
	manager := New(Services{ProviderTenants: errorModelService{}})
	_, _, err := manager.getGeminiTenant(context.Background(), "tenant")
	assertResourceError(t, err, 500, "INTERNAL_ERROR")

	err = manager.putGeminiTenant(context.Background(), "tenant", adminservice.GeminiTenantUpsert{})
	assertResourceError(t, err, 500, "INTERNAL_ERROR")
	manager = New(Services{ProviderTenants: errorModelService{geminiPutStatus: 400}})
	err = manager.putGeminiTenant(context.Background(), "tenant", adminservice.GeminiTenantUpsert{})
	assertResourceError(t, err, 400, "INVALID_GEMINI_TENANT")

	manager = New(Services{ProviderTenants: errorModelService{}})
	_, _, err = manager.deleteGeminiTenant(context.Background(), "tenant")
	assertResourceError(t, err, 500, "INTERNAL_ERROR")
}

func TestGeminiTenantMissingServiceErrors(t *testing.T) {
	manager := New(Services{})
	resource := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "GeminiTenant",
		"metadata": {"name": "default"},
		"spec": {"credential_name": "gemini"}
	}`)

	if _, err := manager.Get(context.Background(), apitypes.ResourceKindGeminiTenant, "default"); err == nil {
		t.Fatal("Get(GeminiTenant) error = nil")
	}
	if _, err := manager.Put(context.Background(), resource); err == nil {
		t.Fatal("Put(GeminiTenant) error = nil")
	}
	if _, err := manager.Delete(context.Background(), apitypes.ResourceKindGeminiTenant, "default"); err == nil {
		t.Fatal("Delete(GeminiTenant) error = nil")
	}
	if _, err := manager.Apply(context.Background(), resource); err == nil {
		t.Fatal("Apply(GeminiTenant) error = nil")
	}
}

func TestApplyGeminiTenantRejectsInvalidHeader(t *testing.T) {
	manager := newModelManager()
	resource := mustResource(t, `{
		"apiVersion": "unsupported",
		"kind": "GeminiTenant",
		"metadata": {"name": "default"},
		"spec": {"credential_name": "gemini"}
	}`)
	_, err := manager.Apply(context.Background(), resource)
	assertResourceError(t, err, 400, "UNSUPPORTED_RESOURCE_VERSION")
}
