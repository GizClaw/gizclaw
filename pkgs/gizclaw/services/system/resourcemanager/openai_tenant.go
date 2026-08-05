package resourcemanager

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func (m *Manager) applyOpenAITenant(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.ProviderTenants == nil {
		return apitypes.ApplyResult{}, missingService("provider tenants")
	}
	item, err := resource.AsOpenAITenantResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_OPENAI_TENANT_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata); err != nil {
		return apitypes.ApplyResult{}, err
	}
	body := openAITenantUpsert(item)
	return applyConcreteResource(ctx, item.Metadata, apitypes.ResourceKindOpenAITenant, item.Spec,
		m.getOpenAITenant,
		func(ctx context.Context) (string, error) { return m.createOpenAITenant(ctx, body) },
		func(ctx context.Context, id string) error { return m.putOpenAITenant(ctx, id, body) }, openAITenantSpec)
}

func (m *Manager) createOpenAITenant(ctx context.Context, body adminhttp.OpenAITenantUpsert) (string, error) {
	response, err := m.services.ProviderTenants.CreateOpenAITenant(ctx, adminhttp.CreateOpenAITenantRequestObject{Body: &body})
	if err != nil {
		return "", err
	}
	switch response := response.(type) {
	case adminhttp.CreateOpenAITenant200JSONResponse:
		return response.Id, nil
	case adminhttp.CreateOpenAITenant400JSONResponse:
		return "", responseError(400, "CREATE_OPENAI_TENANT_FAILED", "failed to create OpenAI tenant", response)
	case adminhttp.CreateOpenAITenant409JSONResponse:
		return "", responseError(409, "CREATE_OPENAI_TENANT_FAILED", "failed to create OpenAI tenant", response)
	case adminhttp.CreateOpenAITenant500JSONResponse:
		return "", responseError(500, "CREATE_OPENAI_TENANT_FAILED", "failed to create OpenAI tenant", response)
	default:
		return "", unexpectedResponse("CreateOpenAITenant", response)
	}
}

func (m *Manager) getOpenAITenant(ctx context.Context, name string) (apitypes.OpenAITenant, bool, error) {
	response, err := m.services.ProviderTenants.GetOpenAITenant(ctx, adminhttp.GetOpenAITenantRequestObject{Id: name})
	if err != nil {
		return apitypes.OpenAITenant{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.GetOpenAITenant200JSONResponse:
		return apitypes.OpenAITenant(response), true, nil
	case adminhttp.GetOpenAITenant404JSONResponse:
		return apitypes.OpenAITenant{}, false, nil
	case adminhttp.GetOpenAITenant500JSONResponse:
		return apitypes.OpenAITenant{}, false, responseError(500, "GET_OPENAI_TENANT_FAILED", "failed to get OpenAI tenant", response)
	default:
		return apitypes.OpenAITenant{}, false, unexpectedResponse("GetOpenAITenant", response)
	}
}

func (m *Manager) putOpenAITenant(ctx context.Context, name string, body adminhttp.OpenAITenantUpsert) error {
	response, err := m.services.ProviderTenants.PutOpenAITenant(ctx, adminhttp.PutOpenAITenantRequestObject{Id: name, Body: &body})
	if err != nil {
		return err
	}
	switch response := response.(type) {
	case adminhttp.PutOpenAITenant200JSONResponse:
		return nil
	case adminhttp.PutOpenAITenant400JSONResponse:
		return responseError(400, "PUT_OPENAI_TENANT_FAILED", "failed to put OpenAI tenant", response)
	case adminhttp.PutOpenAITenant500JSONResponse:
		return responseError(500, "PUT_OPENAI_TENANT_FAILED", "failed to put OpenAI tenant", response)
	default:
		return unexpectedResponse("PutOpenAITenant", response)
	}
}

func (m *Manager) deleteOpenAITenant(ctx context.Context, name string) (apitypes.OpenAITenant, bool, error) {
	response, err := m.services.ProviderTenants.DeleteOpenAITenant(ctx, adminhttp.DeleteOpenAITenantRequestObject{Id: name})
	if err != nil {
		return apitypes.OpenAITenant{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.DeleteOpenAITenant200JSONResponse:
		return apitypes.OpenAITenant(response), true, nil
	case adminhttp.DeleteOpenAITenant404JSONResponse:
		return apitypes.OpenAITenant{}, false, nil
	case adminhttp.DeleteOpenAITenant500JSONResponse:
		return apitypes.OpenAITenant{}, false, responseError(500, "DELETE_OPENAI_TENANT_FAILED", "failed to delete OpenAI tenant", response)
	default:
		return apitypes.OpenAITenant{}, false, unexpectedResponse("DeleteOpenAITenant", response)
	}
}

func openAITenantSpec(item apitypes.OpenAITenant) apitypes.OpenAITenantSpec {
	return apitypes.OpenAITenantSpec{
		ApiMode:      &item.ApiMode,
		BaseUrl:      item.BaseUrl,
		CredentialId: item.CredentialId,
		Description:  item.Description,
		Kind:         &item.Kind,
	}
}

func openAITenantUpsert(resource apitypes.OpenAITenantResource) adminhttp.OpenAITenantUpsert {
	return adminhttp.OpenAITenantUpsert{
		ApiMode:      resource.Spec.ApiMode,
		BaseUrl:      resource.Spec.BaseUrl,
		CredentialId: resource.Spec.CredentialId,
		Description:  resource.Spec.Description,
		Kind:         resource.Spec.Kind,
		Id:           resource.Metadata.Id,
	}
}

func resourceFromOpenAITenant(item apitypes.OpenAITenant) (apitypes.Resource, error) {
	return marshalResource(apitypes.OpenAITenantResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.OpenAITenantResourceKind(apitypes.ResourceKindOpenAITenant),
		Metadata:   apitypes.ResourceMetadata{Id: item.Id},
		Spec:       openAITenantSpec(item),
	})
}
