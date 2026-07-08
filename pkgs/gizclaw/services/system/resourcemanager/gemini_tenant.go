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
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	name := string(pathParam(item.Metadata.Name))
	existing, exists, err := m.getGeminiTenant(ctx, name)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(geminiTenantSpec(existing), item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindGeminiTenant, item.Metadata.Name), nil
		}
	}
	if err := m.putGeminiTenant(ctx, name, geminiTenantUpsert(item)); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindGeminiTenant, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindGeminiTenant, item.Metadata.Name), nil
}

func (m *Manager) getGeminiTenant(ctx context.Context, name string) (apitypes.GeminiTenant, bool, error) {
	response, err := m.services.ProviderTenants.GetGeminiTenant(ctx, adminhttp.GetGeminiTenantRequestObject{Name: name})
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

func (m *Manager) putGeminiTenant(ctx context.Context, name string, body adminhttp.GeminiTenantUpsert) error {
	response, err := m.services.ProviderTenants.PutGeminiTenant(ctx, adminhttp.PutGeminiTenantRequestObject{Name: name, Body: &body})
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

func (m *Manager) deleteGeminiTenant(ctx context.Context, name string) (apitypes.GeminiTenant, bool, error) {
	response, err := m.services.ProviderTenants.DeleteGeminiTenant(ctx, adminhttp.DeleteGeminiTenantRequestObject{Name: name})
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
		BaseUrl:        item.BaseUrl,
		CredentialName: item.CredentialName,
		Description:    item.Description,
		Location:       item.Location,
		ProjectId:      item.ProjectId,
	}
}

func geminiTenantUpsert(resource apitypes.GeminiTenantResource) adminhttp.GeminiTenantUpsert {
	return adminhttp.GeminiTenantUpsert{
		BaseUrl:        resource.Spec.BaseUrl,
		CredentialName: resource.Spec.CredentialName,
		Description:    resource.Spec.Description,
		Location:       resource.Spec.Location,
		Name:           string(resource.Metadata.Name),
		ProjectId:      resource.Spec.ProjectId,
	}
}

func resourceFromGeminiTenant(item apitypes.GeminiTenant) (apitypes.Resource, error) {
	return marshalResource(apitypes.GeminiTenantResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.GeminiTenantResourceKind(apitypes.ResourceKindGeminiTenant),
		Metadata:   apitypes.ResourceMetadata{Name: string(item.Name)},
		Spec:       geminiTenantSpec(item),
	})
}
