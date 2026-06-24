package resourcemanager

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/model"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/providertenants"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

func TestApplyModelCreatesUpdatesAndSkipsUnchanged(t *testing.T) {
	manager := newModelManager()
	resource := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Model",
		"metadata": {"name": "qwen-flash"},
		"spec": {
			"kind": "llm",
			"provider": {"kind": "openai-tenant", "name": "dashscope"},
			"source": "manual",
			"name": "Qwen Flash"
		}
	}`)

	result, err := manager.Apply(context.Background(), resource)
	if err != nil {
		t.Fatalf("Apply(create Model) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("Apply(create Model) action = %s", result.Action)
	}
	result, err = manager.Apply(context.Background(), resource)
	if err != nil {
		t.Fatalf("Apply(unchanged Model) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("Apply(unchanged Model) action = %s", result.Action)
	}

	updated := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Model",
		"metadata": {"name": "qwen-flash"},
		"spec": {
			"kind": "llm",
			"provider": {"kind": "openai-tenant", "name": "dashscope"},
			"source": "manual",
			"name": "Qwen Flash",
			"description": "fast model"
		}
	}`)
	result, err = manager.Apply(context.Background(), updated)
	if err != nil {
		t.Fatalf("Apply(update Model) error = %v", err)
	}
	if result.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("Apply(update Model) action = %s", result.Action)
	}
}

func TestPutGetDeleteModelResource(t *testing.T) {
	manager := newModelManager()
	resource := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Model",
		"metadata": {"name": "speech"},
		"spec": {
			"kind": "tts",
			"provider": {"kind": "openai-tenant", "name": "openai"},
			"source": "manual",
			"provider_data": {"openai-tenant":{"upstream_model":"gpt-4o-mini-tts"}}
		}
	}`)

	stored, err := manager.Put(context.Background(), resource)
	if err != nil {
		t.Fatalf("Put(Model) error = %v", err)
	}
	model, err := stored.AsModelResource()
	if err != nil {
		t.Fatalf("AsModelResource(Put) error = %v", err)
	}
	if model.Spec.Provider.Kind != "openai-tenant" {
		t.Fatalf("Put(Model) provider.kind = %s", model.Spec.Provider.Kind)
	}

	got, err := manager.Get(context.Background(), apitypes.ResourceKindModel, "speech")
	if err != nil {
		t.Fatalf("Get(Model) error = %v", err)
	}
	gotModel, err := got.AsModelResource()
	if err != nil {
		t.Fatalf("AsModelResource(Get) error = %v", err)
	}
	if gotModel.Metadata.Name != "speech" {
		t.Fatalf("Get(Model) metadata.name = %s", gotModel.Metadata.Name)
	}

	deleted, err := manager.Delete(context.Background(), apitypes.ResourceKindModel, "speech")
	if err != nil {
		t.Fatalf("Delete(Model) error = %v", err)
	}
	deletedModel, err := deleted.AsModelResource()
	if err != nil {
		t.Fatalf("AsModelResource(Delete) error = %v", err)
	}
	if deletedModel.Metadata.Name != "speech" {
		t.Fatalf("Delete(Model) metadata.name = %s", deletedModel.Metadata.Name)
	}
	_, err = manager.Get(context.Background(), apitypes.ResourceKindModel, "speech")
	assertResourceError(t, err, 404, "RESOURCE_NOT_FOUND")
}

func TestModelServiceResponseErrors(t *testing.T) {
	manager := New(Services{Models: errorModelService{}})
	_, _, err := manager.getModel(context.Background(), "model")
	assertResourceError(t, err, 500, "INTERNAL_ERROR")
	for _, tc := range []struct {
		status int
		code   string
	}{
		{status: 400, code: "INVALID_MODEL"},
		{status: 409, code: "MODEL_CONFLICT"},
		{status: 500, code: "INTERNAL_ERROR"},
	} {
		t.Run("put", func(t *testing.T) {
			manager := New(Services{Models: errorModelService{putStatus: tc.status}})
			err := manager.putModel(context.Background(), "model", adminservice.ModelUpsert{})
			assertResourceError(t, err, tc.status, tc.code)
		})
	}
	_, _, err = manager.deleteModel(context.Background(), "model")
	assertResourceError(t, err, 500, "INTERNAL_ERROR")
}

func newModelManager() *Manager {
	store := kv.NewMemory(nil)
	return New(Services{
		Models:          &model.Server{Store: store},
		ProviderTenants: &providertenants.Server{ModelStore: store},
	})
}

type errorModelService struct {
	putStatus          int
	dashScopePutStatus int
	geminiPutStatus    int
	openAIPutStatus    int
}

func (e errorModelService) CreateModel(context.Context, adminservice.CreateModelRequestObject) (adminservice.CreateModelResponseObject, error) {
	return adminservice.CreateModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) ListModels(context.Context, adminservice.ListModelsRequestObject) (adminservice.ListModelsResponseObject, error) {
	return adminservice.ListModels500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) DeleteModel(context.Context, adminservice.DeleteModelRequestObject) (adminservice.DeleteModelResponseObject, error) {
	return adminservice.DeleteModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) GetModel(context.Context, adminservice.GetModelRequestObject) (adminservice.GetModelResponseObject, error) {
	return adminservice.GetModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) PutModel(context.Context, adminservice.PutModelRequestObject) (adminservice.PutModelResponseObject, error) {
	switch e.putStatus {
	case 400:
		return adminservice.PutModel400JSONResponse(apitypes.NewErrorResponse("INVALID_MODEL", "invalid")), nil
	case 409:
		return adminservice.PutModel409JSONResponse(apitypes.NewErrorResponse("MODEL_CONFLICT", "conflict")), nil
	default:
		return adminservice.PutModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
	}
}

func (e errorModelService) CreateDashScopeTenant(context.Context, adminservice.CreateDashScopeTenantRequestObject) (adminservice.CreateDashScopeTenantResponseObject, error) {
	return adminservice.CreateDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) ListDashScopeTenants(context.Context, adminservice.ListDashScopeTenantsRequestObject) (adminservice.ListDashScopeTenantsResponseObject, error) {
	return adminservice.ListDashScopeTenants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) DeleteDashScopeTenant(context.Context, adminservice.DeleteDashScopeTenantRequestObject) (adminservice.DeleteDashScopeTenantResponseObject, error) {
	return adminservice.DeleteDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) GetDashScopeTenant(context.Context, adminservice.GetDashScopeTenantRequestObject) (adminservice.GetDashScopeTenantResponseObject, error) {
	return adminservice.GetDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) PutDashScopeTenant(context.Context, adminservice.PutDashScopeTenantRequestObject) (adminservice.PutDashScopeTenantResponseObject, error) {
	switch e.dashScopePutStatus {
	case 400:
		return adminservice.PutDashScopeTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_DASHSCOPE_TENANT", "invalid")), nil
	}
	return adminservice.PutDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) CreateGeminiTenant(context.Context, adminservice.CreateGeminiTenantRequestObject) (adminservice.CreateGeminiTenantResponseObject, error) {
	return adminservice.CreateGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) ListGeminiTenants(context.Context, adminservice.ListGeminiTenantsRequestObject) (adminservice.ListGeminiTenantsResponseObject, error) {
	return adminservice.ListGeminiTenants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) DeleteGeminiTenant(context.Context, adminservice.DeleteGeminiTenantRequestObject) (adminservice.DeleteGeminiTenantResponseObject, error) {
	return adminservice.DeleteGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) GetGeminiTenant(context.Context, adminservice.GetGeminiTenantRequestObject) (adminservice.GetGeminiTenantResponseObject, error) {
	return adminservice.GetGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) PutGeminiTenant(context.Context, adminservice.PutGeminiTenantRequestObject) (adminservice.PutGeminiTenantResponseObject, error) {
	switch e.geminiPutStatus {
	case 400:
		return adminservice.PutGeminiTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_GEMINI_TENANT", "invalid")), nil
	}
	return adminservice.PutGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) CreateOpenAITenant(context.Context, adminservice.CreateOpenAITenantRequestObject) (adminservice.CreateOpenAITenantResponseObject, error) {
	return adminservice.CreateOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) ListOpenAITenants(context.Context, adminservice.ListOpenAITenantsRequestObject) (adminservice.ListOpenAITenantsResponseObject, error) {
	return adminservice.ListOpenAITenants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) DeleteOpenAITenant(context.Context, adminservice.DeleteOpenAITenantRequestObject) (adminservice.DeleteOpenAITenantResponseObject, error) {
	return adminservice.DeleteOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) GetOpenAITenant(context.Context, adminservice.GetOpenAITenantRequestObject) (adminservice.GetOpenAITenantResponseObject, error) {
	return adminservice.GetOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) PutOpenAITenant(context.Context, adminservice.PutOpenAITenantRequestObject) (adminservice.PutOpenAITenantResponseObject, error) {
	switch e.openAIPutStatus {
	case 400:
		return adminservice.PutOpenAITenant400JSONResponse(apitypes.NewErrorResponse("INVALID_OPENAI_TENANT", "invalid")), nil
	}
	return adminservice.PutOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) ListMiniMaxTenants(context.Context, adminservice.ListMiniMaxTenantsRequestObject) (adminservice.ListMiniMaxTenantsResponseObject, error) {
	return adminservice.ListMiniMaxTenants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) CreateMiniMaxTenant(context.Context, adminservice.CreateMiniMaxTenantRequestObject) (adminservice.CreateMiniMaxTenantResponseObject, error) {
	return adminservice.CreateMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) DeleteMiniMaxTenant(context.Context, adminservice.DeleteMiniMaxTenantRequestObject) (adminservice.DeleteMiniMaxTenantResponseObject, error) {
	return adminservice.DeleteMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) GetMiniMaxTenant(context.Context, adminservice.GetMiniMaxTenantRequestObject) (adminservice.GetMiniMaxTenantResponseObject, error) {
	return adminservice.GetMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) PutMiniMaxTenant(context.Context, adminservice.PutMiniMaxTenantRequestObject) (adminservice.PutMiniMaxTenantResponseObject, error) {
	return adminservice.PutMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) SyncMiniMaxTenantVoices(context.Context, adminservice.SyncMiniMaxTenantVoicesRequestObject) (adminservice.SyncMiniMaxTenantVoicesResponseObject, error) {
	return adminservice.SyncMiniMaxTenantVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) ListVolcTenants(context.Context, adminservice.ListVolcTenantsRequestObject) (adminservice.ListVolcTenantsResponseObject, error) {
	return adminservice.ListVolcTenants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) CreateVolcTenant(context.Context, adminservice.CreateVolcTenantRequestObject) (adminservice.CreateVolcTenantResponseObject, error) {
	return adminservice.CreateVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) DeleteVolcTenant(context.Context, adminservice.DeleteVolcTenantRequestObject) (adminservice.DeleteVolcTenantResponseObject, error) {
	return adminservice.DeleteVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) GetVolcTenant(context.Context, adminservice.GetVolcTenantRequestObject) (adminservice.GetVolcTenantResponseObject, error) {
	return adminservice.GetVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) PutVolcTenant(context.Context, adminservice.PutVolcTenantRequestObject) (adminservice.PutVolcTenantResponseObject, error) {
	return adminservice.PutVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) SyncVolcTenantVoices(context.Context, adminservice.SyncVolcTenantVoicesRequestObject) (adminservice.SyncVolcTenantVoicesResponseObject, error) {
	return adminservice.SyncVolcTenantVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}
