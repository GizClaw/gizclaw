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
	name := string(pathParam(item.Metadata.Name))
	existing, exists, err := m.getDashScopeTenant(ctx, name)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(dashScopeTenantSpec(existing), item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindDashScopeTenant, item.Metadata.Name), nil
		}
	}
	if err := m.putDashScopeTenant(ctx, name, dashScopeTenantUpsert(item)); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindDashScopeTenant, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindDashScopeTenant, item.Metadata.Name), nil
}

func (m *Manager) getDashScopeTenant(ctx context.Context, name string) (apitypes.DashScopeTenant, bool, error) {
	response, err := m.services.ProviderTenants.GetDashScopeTenant(ctx, adminhttp.GetDashScopeTenantRequestObject{Name: name})
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
	response, err := m.services.ProviderTenants.PutDashScopeTenant(ctx, adminhttp.PutDashScopeTenantRequestObject{Name: name, Body: &body})
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
	response, err := m.services.ProviderTenants.DeleteDashScopeTenant(ctx, adminhttp.DeleteDashScopeTenantRequestObject{Name: name})
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
		BaseUrl:        item.BaseUrl,
		CredentialName: item.CredentialName,
		Description:    item.Description,
	}
}

func dashScopeTenantUpsert(resource apitypes.DashScopeTenantResource) adminhttp.DashScopeTenantUpsert {
	return adminhttp.DashScopeTenantUpsert{
		BaseUrl:        resource.Spec.BaseUrl,
		CredentialName: resource.Spec.CredentialName,
		Description:    resource.Spec.Description,
		Name:           string(resource.Metadata.Name),
	}
}

func resourceFromDashScopeTenant(item apitypes.DashScopeTenant) (apitypes.Resource, error) {
	return marshalResource(apitypes.DashScopeTenantResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.DashScopeTenantResourceKind(apitypes.ResourceKindDashScopeTenant),
		Metadata:   apitypes.ResourceMetadata{Name: string(item.Name)},
		Spec:       dashScopeTenantSpec(item),
	})
}
