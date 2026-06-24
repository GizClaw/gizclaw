package resourcemanager

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func (m *Manager) applyWorkspace(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.Workspaces == nil {
		return apitypes.ApplyResult{}, missingService("workspaces")
	}
	item, err := resource.AsWorkspaceResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_WORKSPACE_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	name := string(pathParam(item.Metadata.Name))
	existing, exists, err := m.getWorkspace(ctx, name)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(workspaceSpec(existing), item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindWorkspace, item.Metadata.Name), nil
		}
	}
	if err := m.putWorkspace(ctx, name, workspaceUpsert(item)); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindWorkspace, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindWorkspace, item.Metadata.Name), nil
}

func (m *Manager) getWorkspace(ctx context.Context, name string) (apitypes.Workspace, bool, error) {
	response, err := m.services.Workspaces.GetWorkspace(ctx, adminservice.GetWorkspaceRequestObject{Name: name})
	if err != nil {
		return apitypes.Workspace{}, false, err
	}
	switch response := response.(type) {
	case adminservice.GetWorkspace200JSONResponse:
		return apitypes.Workspace(response), true, nil
	case adminservice.GetWorkspace404JSONResponse:
		return apitypes.Workspace{}, false, nil
	case adminservice.GetWorkspace500JSONResponse:
		return apitypes.Workspace{}, false, responseError(500, "GET_WORKSPACE_FAILED", "failed to get workspace", response)
	default:
		return apitypes.Workspace{}, false, unexpectedResponse("GetWorkspace", response)
	}
}

func (m *Manager) putWorkspace(ctx context.Context, name string, body adminservice.WorkspaceUpsert) error {
	response, err := m.services.Workspaces.PutWorkspace(ctx, adminservice.PutWorkspaceRequestObject{Name: name, Body: &body})
	if err != nil {
		return err
	}
	switch response := response.(type) {
	case adminservice.PutWorkspace200JSONResponse:
		return nil
	case adminservice.PutWorkspace400JSONResponse:
		return responseError(400, "PUT_WORKSPACE_FAILED", "failed to put workspace", response)
	case adminservice.PutWorkspace500JSONResponse:
		return responseError(500, "PUT_WORKSPACE_FAILED", "failed to put workspace", response)
	default:
		return unexpectedResponse("PutWorkspace", response)
	}
}

func (m *Manager) deleteWorkspace(ctx context.Context, name string) (apitypes.Workspace, bool, error) {
	response, err := m.services.Workspaces.DeleteWorkspace(ctx, adminservice.DeleteWorkspaceRequestObject{Name: name})
	if err != nil {
		return apitypes.Workspace{}, false, err
	}
	switch response := response.(type) {
	case adminservice.DeleteWorkspace200JSONResponse:
		return apitypes.Workspace(response), true, nil
	case adminservice.DeleteWorkspace404JSONResponse:
		return apitypes.Workspace{}, false, nil
	case adminservice.DeleteWorkspace500JSONResponse:
		return apitypes.Workspace{}, false, responseError(500, "DELETE_WORKSPACE_FAILED", "failed to delete workspace", response)
	default:
		return apitypes.Workspace{}, false, unexpectedResponse("DeleteWorkspace", response)
	}
}

func workspaceSpec(workspace apitypes.Workspace) apitypes.WorkspaceSpec {
	return apitypes.WorkspaceSpec{
		Parameters:   workspace.Parameters,
		WorkflowName: workspace.WorkflowName,
	}
}

func workspaceUpsert(resource apitypes.WorkspaceResource) adminservice.WorkspaceUpsert {
	return adminservice.WorkspaceUpsert{
		Name:         string(resource.Metadata.Name),
		Parameters:   resource.Spec.Parameters,
		WorkflowName: resource.Spec.WorkflowName,
	}
}

func resourceFromWorkspace(item apitypes.Workspace) (apitypes.Resource, error) {
	return marshalResource(apitypes.WorkspaceResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.WorkspaceResourceKind(apitypes.ResourceKindWorkspace),
		Metadata:   apitypes.ResourceMetadata{Name: string(item.Name)},
		Spec:       workspaceSpec(item),
	})
}
