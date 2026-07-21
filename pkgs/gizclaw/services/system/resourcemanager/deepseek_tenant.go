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
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	name := string(pathParam(item.Metadata.Name))
	existing, exists, err := m.getDeepSeekTenant(ctx, name)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(deepSeekTenantSpec(existing), item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindDeepSeekTenant, item.Metadata.Name), nil
		}
	}
	if err := m.putDeepSeekTenant(ctx, name, deepSeekTenantUpsert(item)); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindDeepSeekTenant, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindDeepSeekTenant, item.Metadata.Name), nil
}

func (m *Manager) getDeepSeekTenant(ctx context.Context, name string) (apitypes.DeepSeekTenant, bool, error) {
	response, err := m.services.ProviderTenants.GetDeepSeekTenant(ctx, adminhttp.GetDeepSeekTenantRequestObject{Name: name})
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

func (m *Manager) putDeepSeekTenant(ctx context.Context, name string, body adminhttp.DeepSeekTenantUpsert) error {
	response, err := m.services.ProviderTenants.PutDeepSeekTenant(ctx, adminhttp.PutDeepSeekTenantRequestObject{Name: name, Body: &body})
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

func (m *Manager) deleteDeepSeekTenant(ctx context.Context, name string) (apitypes.DeepSeekTenant, bool, error) {
	response, err := m.services.ProviderTenants.DeleteDeepSeekTenant(ctx, adminhttp.DeleteDeepSeekTenantRequestObject{Name: name})
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
		BaseUrl:        item.BaseUrl,
		CredentialName: item.CredentialName,
		Description:    item.Description,
	}
}

func deepSeekTenantUpsert(resource apitypes.DeepSeekTenantResource) adminhttp.DeepSeekTenantUpsert {
	return adminhttp.DeepSeekTenantUpsert{
		BaseUrl:        resource.Spec.BaseUrl,
		CredentialName: resource.Spec.CredentialName,
		Description:    resource.Spec.Description,
		Name:           string(resource.Metadata.Name),
	}
}

func resourceFromDeepSeekTenant(item apitypes.DeepSeekTenant) (apitypes.Resource, error) {
	return marshalResource(apitypes.DeepSeekTenantResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.DeepSeekTenantResourceKind(apitypes.ResourceKindDeepSeekTenant),
		Metadata:   apitypes.ResourceMetadata{Name: string(item.Name)},
		Spec:       deepSeekTenantSpec(item),
	})
}
