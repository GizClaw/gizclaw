package resourcemanager

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/model"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/providertenants"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
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
			err := manager.putModel(context.Background(), "model", adminhttp.ModelUpsert{})
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

func (e errorModelService) CreateModel(context.Context, adminhttp.CreateModelRequestObject) (adminhttp.CreateModelResponseObject, error) {
	return adminhttp.CreateModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) ListModels(context.Context, adminhttp.ListModelsRequestObject) (adminhttp.ListModelsResponseObject, error) {
	return adminhttp.ListModels500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) DeleteModel(context.Context, adminhttp.DeleteModelRequestObject) (adminhttp.DeleteModelResponseObject, error) {
	return adminhttp.DeleteModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) GetModel(context.Context, adminhttp.GetModelRequestObject) (adminhttp.GetModelResponseObject, error) {
	return adminhttp.GetModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) PutModel(context.Context, adminhttp.PutModelRequestObject) (adminhttp.PutModelResponseObject, error) {
	switch e.putStatus {
	case 400:
		return adminhttp.PutModel400JSONResponse(apitypes.NewErrorResponse("INVALID_MODEL", "invalid")), nil
	case 409:
		return adminhttp.PutModel409JSONResponse(apitypes.NewErrorResponse("MODEL_CONFLICT", "conflict")), nil
	default:
		return adminhttp.PutModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
	}
}

func (e errorModelService) CreateDashScopeTenant(context.Context, adminhttp.CreateDashScopeTenantRequestObject) (adminhttp.CreateDashScopeTenantResponseObject, error) {
	return adminhttp.CreateDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) ListDashScopeTenants(context.Context, adminhttp.ListDashScopeTenantsRequestObject) (adminhttp.ListDashScopeTenantsResponseObject, error) {
	return adminhttp.ListDashScopeTenants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) DeleteDashScopeTenant(context.Context, adminhttp.DeleteDashScopeTenantRequestObject) (adminhttp.DeleteDashScopeTenantResponseObject, error) {
	return adminhttp.DeleteDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) GetDashScopeTenant(context.Context, adminhttp.GetDashScopeTenantRequestObject) (adminhttp.GetDashScopeTenantResponseObject, error) {
	return adminhttp.GetDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) PutDashScopeTenant(context.Context, adminhttp.PutDashScopeTenantRequestObject) (adminhttp.PutDashScopeTenantResponseObject, error) {
	switch e.dashScopePutStatus {
	case 400:
		return adminhttp.PutDashScopeTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_DASHSCOPE_TENANT", "invalid")), nil
	}
	return adminhttp.PutDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) CreateGeminiTenant(context.Context, adminhttp.CreateGeminiTenantRequestObject) (adminhttp.CreateGeminiTenantResponseObject, error) {
	return adminhttp.CreateGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) ListGeminiTenants(context.Context, adminhttp.ListGeminiTenantsRequestObject) (adminhttp.ListGeminiTenantsResponseObject, error) {
	return adminhttp.ListGeminiTenants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) DeleteGeminiTenant(context.Context, adminhttp.DeleteGeminiTenantRequestObject) (adminhttp.DeleteGeminiTenantResponseObject, error) {
	return adminhttp.DeleteGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) GetGeminiTenant(context.Context, adminhttp.GetGeminiTenantRequestObject) (adminhttp.GetGeminiTenantResponseObject, error) {
	return adminhttp.GetGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) PutGeminiTenant(context.Context, adminhttp.PutGeminiTenantRequestObject) (adminhttp.PutGeminiTenantResponseObject, error) {
	switch e.geminiPutStatus {
	case 400:
		return adminhttp.PutGeminiTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_GEMINI_TENANT", "invalid")), nil
	}
	return adminhttp.PutGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) CreateOpenAITenant(context.Context, adminhttp.CreateOpenAITenantRequestObject) (adminhttp.CreateOpenAITenantResponseObject, error) {
	return adminhttp.CreateOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) ListOpenAITenants(context.Context, adminhttp.ListOpenAITenantsRequestObject) (adminhttp.ListOpenAITenantsResponseObject, error) {
	return adminhttp.ListOpenAITenants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) DeleteOpenAITenant(context.Context, adminhttp.DeleteOpenAITenantRequestObject) (adminhttp.DeleteOpenAITenantResponseObject, error) {
	return adminhttp.DeleteOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) GetOpenAITenant(context.Context, adminhttp.GetOpenAITenantRequestObject) (adminhttp.GetOpenAITenantResponseObject, error) {
	return adminhttp.GetOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) PutOpenAITenant(context.Context, adminhttp.PutOpenAITenantRequestObject) (adminhttp.PutOpenAITenantResponseObject, error) {
	switch e.openAIPutStatus {
	case 400:
		return adminhttp.PutOpenAITenant400JSONResponse(apitypes.NewErrorResponse("INVALID_OPENAI_TENANT", "invalid")), nil
	}
	return adminhttp.PutOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) ListMiniMaxTenants(context.Context, adminhttp.ListMiniMaxTenantsRequestObject) (adminhttp.ListMiniMaxTenantsResponseObject, error) {
	return adminhttp.ListMiniMaxTenants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) CreateMiniMaxTenant(context.Context, adminhttp.CreateMiniMaxTenantRequestObject) (adminhttp.CreateMiniMaxTenantResponseObject, error) {
	return adminhttp.CreateMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) DeleteMiniMaxTenant(context.Context, adminhttp.DeleteMiniMaxTenantRequestObject) (adminhttp.DeleteMiniMaxTenantResponseObject, error) {
	return adminhttp.DeleteMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) GetMiniMaxTenant(context.Context, adminhttp.GetMiniMaxTenantRequestObject) (adminhttp.GetMiniMaxTenantResponseObject, error) {
	return adminhttp.GetMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) PutMiniMaxTenant(context.Context, adminhttp.PutMiniMaxTenantRequestObject) (adminhttp.PutMiniMaxTenantResponseObject, error) {
	return adminhttp.PutMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) SyncMiniMaxTenantVoices(context.Context, adminhttp.SyncMiniMaxTenantVoicesRequestObject) (adminhttp.SyncMiniMaxTenantVoicesResponseObject, error) {
	return adminhttp.SyncMiniMaxTenantVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) ListVolcTenants(context.Context, adminhttp.ListVolcTenantsRequestObject) (adminhttp.ListVolcTenantsResponseObject, error) {
	return adminhttp.ListVolcTenants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) CreateVolcTenant(context.Context, adminhttp.CreateVolcTenantRequestObject) (adminhttp.CreateVolcTenantResponseObject, error) {
	return adminhttp.CreateVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) DeleteVolcTenant(context.Context, adminhttp.DeleteVolcTenantRequestObject) (adminhttp.DeleteVolcTenantResponseObject, error) {
	return adminhttp.DeleteVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) GetVolcTenant(context.Context, adminhttp.GetVolcTenantRequestObject) (adminhttp.GetVolcTenantResponseObject, error) {
	return adminhttp.GetVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) PutVolcTenant(context.Context, adminhttp.PutVolcTenantRequestObject) (adminhttp.PutVolcTenantResponseObject, error) {
	return adminhttp.PutVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}

func (e errorModelService) SyncVolcTenantVoices(context.Context, adminhttp.SyncVolcTenantVoicesRequestObject) (adminhttp.SyncVolcTenantVoicesResponseObject, error) {
	return adminhttp.SyncVolcTenantVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
}
