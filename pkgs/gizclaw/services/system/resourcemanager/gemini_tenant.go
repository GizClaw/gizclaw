package resourcemanager

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func (m *Manager) applyGeminiTenant(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.ProviderTenants == nil {
		return apitypes.ApplyResult{}, missingService("provider tenants")
	}
	item, err := resource.AsGeminiTenantResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_GEMINI_TENANT_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata); err != nil {
		return apitypes.ApplyResult{}, err
	}
	body := geminiTenantUpsert(item)
	return applyConcreteResource(ctx, item.Metadata, apitypes.ResourceKindGeminiTenant, item.Spec,
		m.getGeminiTenant,
		func(ctx context.Context) (string, error) { return m.createGeminiTenant(ctx, body) },
		func(ctx context.Context, id string) error { return m.putGeminiTenant(ctx, id, body) }, geminiTenantSpec)
}

func (m *Manager) createGeminiTenant(ctx context.Context, body adminhttp.GeminiTenantUpsert) (string, error) {
	response, err := m.services.ProviderTenants.CreateGeminiTenant(ctx, adminhttp.CreateGeminiTenantRequestObject{Body: &body})
	if err != nil {
		return "", err
	}
	switch response := response.(type) {
	case adminhttp.CreateGeminiTenant200JSONResponse:
		return response.Id, nil
	case adminhttp.CreateGeminiTenant400JSONResponse:
		return "", responseError(400, "CREATE_GEMINI_TENANT_FAILED", "failed to create Gemini tenant", response)
	case adminhttp.CreateGeminiTenant409JSONResponse:
		return "", responseError(409, "CREATE_GEMINI_TENANT_FAILED", "failed to create Gemini tenant", response)
	case adminhttp.CreateGeminiTenant500JSONResponse:
		return "", responseError(500, "CREATE_GEMINI_TENANT_FAILED", "failed to create Gemini tenant", response)
	default:
		return "", unexpectedResponse("CreateGeminiTenant", response)
	}
}

func (m *Manager) getGeminiTenant(ctx context.Context, id string) (apitypes.GeminiTenant, bool, error) {
	response, err := m.services.ProviderTenants.GetGeminiTenant(ctx, adminhttp.GetGeminiTenantRequestObject{Id: id})
	if err != nil {
		return apitypes.GeminiTenant{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.GetGeminiTenant200JSONResponse:
		return apitypes.GeminiTenant(response), true, nil
	case adminhttp.GetGeminiTenant404JSONResponse:
		return apitypes.GeminiTenant{}, false, nil
	case adminhttp.GetGeminiTenant500JSONResponse:
		return apitypes.GeminiTenant{}, false, responseError(500, "GET_GEMINI_TENANT_FAILED", "failed to get Gemini tenant", response)
	default:
		return apitypes.GeminiTenant{}, false, unexpectedResponse("GetGeminiTenant", response)
	}
}

func (m *Manager) putGeminiTenant(ctx context.Context, id string, body adminhttp.GeminiTenantUpsert) error {
	response, err := m.services.ProviderTenants.PutGeminiTenant(ctx, adminhttp.PutGeminiTenantRequestObject{Id: id, Body: &body})
	if err != nil {
		return err
	}
	switch response := response.(type) {
	case adminhttp.PutGeminiTenant200JSONResponse:
		return nil
	case adminhttp.PutGeminiTenant400JSONResponse:
		return responseError(400, "PUT_GEMINI_TENANT_FAILED", "failed to put Gemini tenant", response)
	case adminhttp.PutGeminiTenant500JSONResponse:
		return responseError(500, "PUT_GEMINI_TENANT_FAILED", "failed to put Gemini tenant", response)
	default:
		return unexpectedResponse("PutGeminiTenant", response)
	}
}

func (m *Manager) deleteGeminiTenant(ctx context.Context, id string) (apitypes.GeminiTenant, bool, error) {
	response, err := m.services.ProviderTenants.DeleteGeminiTenant(ctx, adminhttp.DeleteGeminiTenantRequestObject{Id: id})
	if err != nil {
		return apitypes.GeminiTenant{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.DeleteGeminiTenant200JSONResponse:
		return apitypes.GeminiTenant(response), true, nil
	case adminhttp.DeleteGeminiTenant404JSONResponse:
		return apitypes.GeminiTenant{}, false, nil
	case adminhttp.DeleteGeminiTenant500JSONResponse:
		return apitypes.GeminiTenant{}, false, responseError(500, "DELETE_GEMINI_TENANT_FAILED", "failed to delete Gemini tenant", response)
	default:
		return apitypes.GeminiTenant{}, false, unexpectedResponse("DeleteGeminiTenant", response)
	}
}

func geminiTenantSpec(item apitypes.GeminiTenant) apitypes.GeminiTenantSpec {
	return apitypes.GeminiTenantSpec{
		BaseUrl:      item.BaseUrl,
		CredentialId: item.CredentialId,
		Description:  item.Description,
		Location:     item.Location,
		ProjectId:    item.ProjectId,
	}
}

func geminiTenantUpsert(resource apitypes.GeminiTenantResource) adminhttp.GeminiTenantUpsert {
	return adminhttp.GeminiTenantUpsert{
		BaseUrl:      resource.Spec.BaseUrl,
		CredentialId: resource.Spec.CredentialId,
		Description:  resource.Spec.Description,
		Location:     resource.Spec.Location,
		Id:           resource.Metadata.Id,
		ProjectId:    resource.Spec.ProjectId,
	}
}

func resourceFromGeminiTenant(item apitypes.GeminiTenant) (apitypes.Resource, error) {
	return marshalResource(apitypes.GeminiTenantResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.GeminiTenantResourceKind(apitypes.ResourceKindGeminiTenant),
		Metadata:   apitypes.ResourceMetadata{Id: item.Id},
		Spec:       geminiTenantSpec(item),
	})
}
