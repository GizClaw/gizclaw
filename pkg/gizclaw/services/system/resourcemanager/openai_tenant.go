package resourcemanager

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func (m *Manager) applyOpenAITenant(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.ProviderTenants == nil {
		return apitypes.ApplyResult{}, missingService("provider tenants")
	}
	item, err := resource.AsOpenAITenantResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_OPENAI_TENANT_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	name := string(pathParam(item.Metadata.Name))
	existing, exists, err := m.getOpenAITenant(ctx, name)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(openAITenantSpec(existing), item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindOpenAITenant, item.Metadata.Name), nil
		}
	}
	if err := m.putOpenAITenant(ctx, name, openAITenantUpsert(item)); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindOpenAITenant, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindOpenAITenant, item.Metadata.Name), nil
}

func (m *Manager) getOpenAITenant(ctx context.Context, name string) (apitypes.OpenAITenant, bool, error) {
	response, err := m.services.ProviderTenants.GetOpenAITenant(ctx, adminservice.GetOpenAITenantRequestObject{Name: name})
	if err != nil {
		return apitypes.OpenAITenant{}, false, err
	}
	switch response := response.(type) {
	case adminservice.GetOpenAITenant200JSONResponse:
		return apitypes.OpenAITenant(response), true, nil
	case adminservice.GetOpenAITenant404JSONResponse:
		return apitypes.OpenAITenant{}, false, nil
	case adminservice.GetOpenAITenant500JSONResponse:
		return apitypes.OpenAITenant{}, false, responseError(500, "GET_OPENAI_TENANT_FAILED", "failed to get OpenAI tenant", response)
	default:
		return apitypes.OpenAITenant{}, false, unexpectedResponse("GetOpenAITenant", response)
	}
}

func (m *Manager) putOpenAITenant(ctx context.Context, name string, body adminservice.OpenAITenantUpsert) error {
	response, err := m.services.ProviderTenants.PutOpenAITenant(ctx, adminservice.PutOpenAITenantRequestObject{Name: name, Body: &body})
	if err != nil {
		return err
	}
	switch response := response.(type) {
	case adminservice.PutOpenAITenant200JSONResponse:
		return nil
	case adminservice.PutOpenAITenant400JSONResponse:
		return responseError(400, "PUT_OPENAI_TENANT_FAILED", "failed to put OpenAI tenant", response)
	case adminservice.PutOpenAITenant500JSONResponse:
		return responseError(500, "PUT_OPENAI_TENANT_FAILED", "failed to put OpenAI tenant", response)
	default:
		return unexpectedResponse("PutOpenAITenant", response)
	}
}

func (m *Manager) deleteOpenAITenant(ctx context.Context, name string) (apitypes.OpenAITenant, bool, error) {
	response, err := m.services.ProviderTenants.DeleteOpenAITenant(ctx, adminservice.DeleteOpenAITenantRequestObject{Name: name})
	if err != nil {
		return apitypes.OpenAITenant{}, false, err
	}
	switch response := response.(type) {
	case adminservice.DeleteOpenAITenant200JSONResponse:
		return apitypes.OpenAITenant(response), true, nil
	case adminservice.DeleteOpenAITenant404JSONResponse:
		return apitypes.OpenAITenant{}, false, nil
	case adminservice.DeleteOpenAITenant500JSONResponse:
		return apitypes.OpenAITenant{}, false, responseError(500, "DELETE_OPENAI_TENANT_FAILED", "failed to delete OpenAI tenant", response)
	default:
		return apitypes.OpenAITenant{}, false, unexpectedResponse("DeleteOpenAITenant", response)
	}
}

func openAITenantSpec(item apitypes.OpenAITenant) apitypes.OpenAITenantSpec {
	return apitypes.OpenAITenantSpec{
		ApiMode:        &item.ApiMode,
		BaseUrl:        item.BaseUrl,
		CredentialName: item.CredentialName,
		Description:    item.Description,
		Kind:           &item.Kind,
	}
}

func openAITenantUpsert(resource apitypes.OpenAITenantResource) adminservice.OpenAITenantUpsert {
	return adminservice.OpenAITenantUpsert{
		ApiMode:        resource.Spec.ApiMode,
		BaseUrl:        resource.Spec.BaseUrl,
		CredentialName: resource.Spec.CredentialName,
		Description:    resource.Spec.Description,
		Kind:           resource.Spec.Kind,
		Name:           string(resource.Metadata.Name),
	}
}

func resourceFromOpenAITenant(item apitypes.OpenAITenant) (apitypes.Resource, error) {
	return marshalResource(apitypes.OpenAITenantResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.OpenAITenantResourceKind(apitypes.ResourceKindOpenAITenant),
		Metadata:   apitypes.ResourceMetadata{Name: string(item.Name)},
		Spec:       openAITenantSpec(item),
	})
}
