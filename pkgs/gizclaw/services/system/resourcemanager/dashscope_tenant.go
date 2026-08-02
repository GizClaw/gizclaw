package resourcemanager

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func (m *Manager) applyDashScopeTenant(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.ProviderTenants == nil {
		return apitypes.ApplyResult{}, missingService("provider tenants")
	}
	item, err := resource.AsDashScopeTenantResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_DASHSCOPE_TENANT_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	body := dashScopeTenantUpsert(item)
	return applyNamedResource(ctx, item.Metadata, apitypes.ResourceKindDashScopeTenant, item.Spec,
		m.getDashScopeTenant,
		func(ctx context.Context) (string, error) { return m.createDashScopeTenant(ctx, body) },
		func(ctx context.Context, id string) error { return m.putDashScopeTenant(ctx, id, body) },
		func(value apitypes.DashScopeTenant) string { return value.Name }, dashScopeTenantSpec)
}

func (m *Manager) createDashScopeTenant(ctx context.Context, body adminhttp.DashScopeTenantUpsert) (string, error) {
	response, err := m.services.ProviderTenants.CreateDashScopeTenant(ctx, adminhttp.CreateDashScopeTenantRequestObject{Body: &body})
	if err != nil {
		return "", err
	}
	switch response := response.(type) {
	case adminhttp.CreateDashScopeTenant200JSONResponse:
		return response.Id, nil
	case adminhttp.CreateDashScopeTenant400JSONResponse:
		return "", responseError(400, "CREATE_DASHSCOPE_TENANT_FAILED", "failed to create DashScope tenant", response)
	case adminhttp.CreateDashScopeTenant409JSONResponse:
		return "", responseError(409, "CREATE_DASHSCOPE_TENANT_FAILED", "failed to create DashScope tenant", response)
	case adminhttp.CreateDashScopeTenant500JSONResponse:
		return "", responseError(500, "CREATE_DASHSCOPE_TENANT_FAILED", "failed to create DashScope tenant", response)
	default:
		return "", unexpectedResponse("CreateDashScopeTenant", response)
	}
}

func (m *Manager) getDashScopeTenant(ctx context.Context, name string) (apitypes.DashScopeTenant, bool, error) {
	response, err := m.services.ProviderTenants.GetDashScopeTenant(ctx, adminhttp.GetDashScopeTenantRequestObject{Id: name})
	if err != nil {
		return apitypes.DashScopeTenant{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.GetDashScopeTenant200JSONResponse:
		return apitypes.DashScopeTenant(response), true, nil
	case adminhttp.GetDashScopeTenant404JSONResponse:
		return apitypes.DashScopeTenant{}, false, nil
	case adminhttp.GetDashScopeTenant500JSONResponse:
		return apitypes.DashScopeTenant{}, false, responseError(500, "GET_DASHSCOPE_TENANT_FAILED", "failed to get DashScope tenant", response)
	default:
		return apitypes.DashScopeTenant{}, false, unexpectedResponse("GetDashScopeTenant", response)
	}
}

func (m *Manager) putDashScopeTenant(ctx context.Context, name string, body adminhttp.DashScopeTenantUpsert) error {
	response, err := m.services.ProviderTenants.PutDashScopeTenant(ctx, adminhttp.PutDashScopeTenantRequestObject{Id: name, Body: &body})
	if err != nil {
		return err
	}
	switch response := response.(type) {
	case adminhttp.PutDashScopeTenant200JSONResponse:
		return nil
	case adminhttp.PutDashScopeTenant400JSONResponse:
		return responseError(400, "PUT_DASHSCOPE_TENANT_FAILED", "failed to put DashScope tenant", response)
	case adminhttp.PutDashScopeTenant500JSONResponse:
		return responseError(500, "PUT_DASHSCOPE_TENANT_FAILED", "failed to put DashScope tenant", response)
	default:
		return unexpectedResponse("PutDashScopeTenant", response)
	}
}

func (m *Manager) deleteDashScopeTenant(ctx context.Context, name string) (apitypes.DashScopeTenant, bool, error) {
	response, err := m.services.ProviderTenants.DeleteDashScopeTenant(ctx, adminhttp.DeleteDashScopeTenantRequestObject{Id: name})
	if err != nil {
		return apitypes.DashScopeTenant{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.DeleteDashScopeTenant200JSONResponse:
		return apitypes.DashScopeTenant(response), true, nil
	case adminhttp.DeleteDashScopeTenant404JSONResponse:
		return apitypes.DashScopeTenant{}, false, nil
	case adminhttp.DeleteDashScopeTenant500JSONResponse:
		return apitypes.DashScopeTenant{}, false, responseError(500, "DELETE_DASHSCOPE_TENANT_FAILED", "failed to delete DashScope tenant", response)
	default:
		return apitypes.DashScopeTenant{}, false, unexpectedResponse("DeleteDashScopeTenant", response)
	}
}

func dashScopeTenantSpec(item apitypes.DashScopeTenant) apitypes.DashScopeTenantSpec {
	return apitypes.DashScopeTenantSpec{
		BaseUrl:      item.BaseUrl,
		CredentialId: item.CredentialId,
		Description:  item.Description,
	}
}

func dashScopeTenantUpsert(resource apitypes.DashScopeTenantResource) adminhttp.DashScopeTenantUpsert {
	return adminhttp.DashScopeTenantUpsert{
		BaseUrl:      resource.Spec.BaseUrl,
		CredentialId: resource.Spec.CredentialId,
		Description:  resource.Spec.Description,
		Name:         string(resource.Metadata.Name),
	}
}

func resourceFromDashScopeTenant(item apitypes.DashScopeTenant) (apitypes.Resource, error) {
	return marshalResource(apitypes.DashScopeTenantResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.DashScopeTenantResourceKind(apitypes.ResourceKindDashScopeTenant),
		Metadata:   apitypes.ResourceMetadata{Id: &item.Id, Name: item.Name},
		Spec:       dashScopeTenantSpec(item),
	})
}
