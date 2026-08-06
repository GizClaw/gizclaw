package resourcemanager

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func (m *Manager) applyDeepSeekTenant(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.ProviderTenants == nil {
		return apitypes.ApplyResult{}, missingService("provider tenants")
	}
	item, err := resource.AsDeepSeekTenantResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_DEEPSEEK_TENANT_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata); err != nil {
		return apitypes.ApplyResult{}, err
	}
	body := deepSeekTenantUpsert(item)
	return applyConcreteResource(ctx, item.Metadata, apitypes.ResourceKindDeepSeekTenant, item.Spec,
		m.getDeepSeekTenant,
		func(ctx context.Context) (string, error) { return m.createDeepSeekTenant(ctx, body) },
		func(ctx context.Context, id string) error { return m.putDeepSeekTenant(ctx, id, body) }, deepSeekTenantSpec)
}

func (m *Manager) createDeepSeekTenant(ctx context.Context, body adminhttp.DeepSeekTenantUpsert) (string, error) {
	response, err := m.services.ProviderTenants.CreateDeepSeekTenant(ctx, adminhttp.CreateDeepSeekTenantRequestObject{Body: &body})
	if err != nil {
		return "", err
	}
	switch response := response.(type) {
	case adminhttp.CreateDeepSeekTenant200JSONResponse:
		return response.Id, nil
	case adminhttp.CreateDeepSeekTenant400JSONResponse:
		return "", responseError(400, "CREATE_DEEPSEEK_TENANT_FAILED", "failed to create DeepSeek tenant", response)
	case adminhttp.CreateDeepSeekTenant409JSONResponse:
		return "", responseError(409, "CREATE_DEEPSEEK_TENANT_FAILED", "failed to create DeepSeek tenant", response)
	case adminhttp.CreateDeepSeekTenant500JSONResponse:
		return "", responseError(500, "CREATE_DEEPSEEK_TENANT_FAILED", "failed to create DeepSeek tenant", response)
	default:
		return "", unexpectedResponse("CreateDeepSeekTenant", response)
	}
}

func (m *Manager) getDeepSeekTenant(ctx context.Context, id string) (apitypes.DeepSeekTenant, bool, error) {
	response, err := m.services.ProviderTenants.GetDeepSeekTenant(ctx, adminhttp.GetDeepSeekTenantRequestObject{Id: id})
	if err != nil {
		return apitypes.DeepSeekTenant{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.GetDeepSeekTenant200JSONResponse:
		return apitypes.DeepSeekTenant(response), true, nil
	case adminhttp.GetDeepSeekTenant404JSONResponse:
		return apitypes.DeepSeekTenant{}, false, nil
	case adminhttp.GetDeepSeekTenant500JSONResponse:
		return apitypes.DeepSeekTenant{}, false, responseError(500, "GET_DEEPSEEK_TENANT_FAILED", "failed to get DeepSeek tenant", response)
	default:
		return apitypes.DeepSeekTenant{}, false, unexpectedResponse("GetDeepSeekTenant", response)
	}
}

func (m *Manager) putDeepSeekTenant(ctx context.Context, id string, body adminhttp.DeepSeekTenantUpsert) error {
	response, err := m.services.ProviderTenants.PutDeepSeekTenant(ctx, adminhttp.PutDeepSeekTenantRequestObject{Id: id, Body: &body})
	if err != nil {
		return err
	}
	switch response := response.(type) {
	case adminhttp.PutDeepSeekTenant200JSONResponse:
		return nil
	case adminhttp.PutDeepSeekTenant400JSONResponse:
		return responseError(400, "PUT_DEEPSEEK_TENANT_FAILED", "failed to put DeepSeek tenant", response)
	case adminhttp.PutDeepSeekTenant500JSONResponse:
		return responseError(500, "PUT_DEEPSEEK_TENANT_FAILED", "failed to put DeepSeek tenant", response)
	default:
		return unexpectedResponse("PutDeepSeekTenant", response)
	}
}

func (m *Manager) deleteDeepSeekTenant(ctx context.Context, id string) (apitypes.DeepSeekTenant, bool, error) {
	response, err := m.services.ProviderTenants.DeleteDeepSeekTenant(ctx, adminhttp.DeleteDeepSeekTenantRequestObject{Id: id})
	if err != nil {
		return apitypes.DeepSeekTenant{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.DeleteDeepSeekTenant200JSONResponse:
		return apitypes.DeepSeekTenant(response), true, nil
	case adminhttp.DeleteDeepSeekTenant404JSONResponse:
		return apitypes.DeepSeekTenant{}, false, nil
	case adminhttp.DeleteDeepSeekTenant500JSONResponse:
		return apitypes.DeepSeekTenant{}, false, responseError(500, "DELETE_DEEPSEEK_TENANT_FAILED", "failed to delete DeepSeek tenant", response)
	default:
		return apitypes.DeepSeekTenant{}, false, unexpectedResponse("DeleteDeepSeekTenant", response)
	}
}

func deepSeekTenantSpec(item apitypes.DeepSeekTenant) apitypes.DeepSeekTenantSpec {
	return apitypes.DeepSeekTenantSpec{
		BaseUrl:      item.BaseUrl,
		CredentialId: item.CredentialId,
		Description:  item.Description,
	}
}

func deepSeekTenantUpsert(resource apitypes.DeepSeekTenantResource) adminhttp.DeepSeekTenantUpsert {
	return adminhttp.DeepSeekTenantUpsert{
		BaseUrl:      resource.Spec.BaseUrl,
		CredentialId: resource.Spec.CredentialId,
		Description:  resource.Spec.Description,
		Id:           resource.Metadata.Id,
	}
}

func resourceFromDeepSeekTenant(item apitypes.DeepSeekTenant) (apitypes.Resource, error) {
	return marshalResource(apitypes.DeepSeekTenantResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.DeepSeekTenantResourceKind(apitypes.ResourceKindDeepSeekTenant),
		Metadata:   apitypes.ResourceMetadata{Id: item.Id},
		Spec:       deepSeekTenantSpec(item),
	})
}
