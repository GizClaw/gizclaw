package resourcemanager

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestApplyOpenAITenantCreatesUpdatesAndSkipsUnchanged(t *testing.T) {
	manager := newModelManager()
	resource := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "OpenAITenant",
		"metadata": {"name": "minimax"},
		"spec": {
			"kind": "compatible",
			"credential_name": "minimax",
			"base_url": "https://api.minimax.chat/v1",
			"api_mode": "chat_completions"
		}
	}`)

	result, err := manager.Apply(context.Background(), resource)
	if err != nil {
		t.Fatalf("Apply(create OpenAITenant) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("Apply(create OpenAITenant) action = %s", result.Action)
	}
	result, err = manager.Apply(context.Background(), resource)
	if err != nil {
		t.Fatalf("Apply(unchanged OpenAITenant) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("Apply(unchanged OpenAITenant) action = %s", result.Action)
	}

	updated := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "OpenAITenant",
		"metadata": {"name": "minimax"},
		"spec": {
			"kind": "compatible",
			"credential_name": "minimax",
			"base_url": "https://api.minimax.chat/v1",
			"api_mode": "chat_completions",
			"description": "MiniMax compatible endpoint"
		}
	}`)
	result, err = manager.Apply(context.Background(), updated)
	if err != nil {
		t.Fatalf("Apply(update OpenAITenant) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("Apply(update OpenAITenant) action = %s", result.Action)
	}
}

func TestPutGetDeleteOpenAITenantResource(t *testing.T) {
	manager := newModelManager()
	resource := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "OpenAITenant",
		"metadata": {"name": "minimax"},
		"spec": {
			"credential_name": "minimax",
			"base_url": "https://api.minimax.chat/v1"
		}
	}`)

	stored, err := manager.Put(context.Background(), resource)
	if err != nil {
		t.Fatalf("Put(OpenAITenant) error = %v", err)
	}
	tenant, err := stored.AsOpenAITenantResource()
	if err != nil {
		t.Fatalf("AsOpenAITenantResource(Put) error = %v", err)
	}
	if tenant.Spec.CredentialName != "minimax" {
		t.Fatalf("Put(OpenAITenant) credential_name = %s", tenant.Spec.CredentialName)
	}

	got, err := manager.Get(context.Background(), apitypes.ResourceKindOpenAITenant, "minimax")
	if err != nil {
		t.Fatalf("Get(OpenAITenant) error = %v", err)
	}
	gotTenant, err := got.AsOpenAITenantResource()
	if err != nil {
		t.Fatalf("AsOpenAITenantResource(Get) error = %v", err)
	}
	if gotTenant.Metadata.Name != "minimax" {
		t.Fatalf("Get(OpenAITenant) metadata.name = %s", gotTenant.Metadata.Name)
	}

	deleted, err := manager.Delete(context.Background(), apitypes.ResourceKindOpenAITenant, "minimax")
	if err != nil {
		t.Fatalf("Delete(OpenAITenant) error = %v", err)
	}
	deletedTenant, err := deleted.AsOpenAITenantResource()
	if err != nil {
		t.Fatalf("AsOpenAITenantResource(Delete) error = %v", err)
	}
	if deletedTenant.Metadata.Name != "minimax" {
		t.Fatalf("Delete(OpenAITenant) metadata.name = %s", deletedTenant.Metadata.Name)
	}
	_, err = manager.Get(context.Background(), apitypes.ResourceKindOpenAITenant, "minimax")
	assertResourceError(t, err, 404, "RESOURCE_NOT_FOUND")
	_, err = manager.Delete(context.Background(), apitypes.ResourceKindOpenAITenant, "minimax")
	assertResourceError(t, err, 404, "RESOURCE_NOT_FOUND")
}

func TestOpenAITenantServiceResponseErrors(t *testing.T) {
	manager := New(Services{ProviderTenants: errorModelService{}})
	_, _, err := manager.getOpenAITenant(context.Background(), "tenant")
	assertResourceError(t, err, 500, "INTERNAL_ERROR")

	err = manager.putOpenAITenant(context.Background(), "tenant", adminhttp.OpenAITenantUpsert{})
	assertResourceError(t, err, 500, "INTERNAL_ERROR")
	manager = New(Services{ProviderTenants: errorModelService{openAIPutStatus: 400}})
	err = manager.putOpenAITenant(context.Background(), "tenant", adminhttp.OpenAITenantUpsert{})
	assertResourceError(t, err, 400, "INVALID_OPENAI_TENANT")

	manager = New(Services{ProviderTenants: errorModelService{}})
	_, _, err = manager.deleteOpenAITenant(context.Background(), "tenant")
	assertResourceError(t, err, 500, "INTERNAL_ERROR")
}

func TestOpenAITenantMissingServiceErrors(t *testing.T) {
	manager := New(Services{})
	resource := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "OpenAITenant",
		"metadata": {"name": "minimax"},
		"spec": {"credential_name": "minimax"}
	}`)

	if _, err := manager.Get(context.Background(), apitypes.ResourceKindOpenAITenant, "minimax"); err == nil {
		t.Fatal("Get(OpenAITenant) error = nil")
	}
	if _, err := manager.Put(context.Background(), resource); err == nil {
		t.Fatal("Put(OpenAITenant) error = nil")
	}
	if _, err := manager.Delete(context.Background(), apitypes.ResourceKindOpenAITenant, "minimax"); err == nil {
		t.Fatal("Delete(OpenAITenant) error = nil")
	}
	if _, err := manager.Apply(context.Background(), resource); err == nil {
		t.Fatal("Apply(OpenAITenant) error = nil")
	}
}

func TestApplyOpenAITenantRejectsInvalidHeader(t *testing.T) {
	manager := newModelManager()
	resource := mustResource(t, `{
		"apiVersion": "unsupported",
		"kind": "OpenAITenant",
		"metadata": {"name": "minimax"},
		"spec": {"credential_name": "minimax"}
	}`)
	_, err := manager.Apply(context.Background(), resource)
	assertResourceError(t, err, 400, "UNSUPPORTED_RESOURCE_VERSION")
}
