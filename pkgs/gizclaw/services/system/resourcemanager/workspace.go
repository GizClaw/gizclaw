package resourcemanager

import (
	"context"
	"maps"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
)

func (m *Manager) getWorkspace(ctx context.Context, id string) (apitypes.Workspace, bool, error) {
	response, err := m.services.Workspaces.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Id: id})
	if err != nil {
		return apitypes.Workspace{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.GetWorkspace200JSONResponse:
		return apitypes.Workspace(response), true, nil
	case adminhttp.GetWorkspace404JSONResponse:
		return apitypes.Workspace{}, false, nil
	case adminhttp.GetWorkspace500JSONResponse:
		return apitypes.Workspace{}, false, responseError(500, "GET_WORKSPACE_FAILED", "failed to get workspace", response)
	default:
		return apitypes.Workspace{}, false, unexpectedResponse("GetWorkspace", response)
	}
}

func (m *Manager) putWorkspace(ctx context.Context, id string, body adminhttp.WorkspaceUpsert) error {
	response, err := m.services.Workspaces.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Id: id, Body: &body})
	if err != nil {
		return err
	}
	switch response := response.(type) {
	case adminhttp.PutWorkspace200JSONResponse:
		return nil
	case adminhttp.PutWorkspace400JSONResponse:
		return responseError(400, "PUT_WORKSPACE_FAILED", "failed to put workspace", response)
	case adminhttp.PutWorkspace500JSONResponse:
		return responseError(500, "PUT_WORKSPACE_FAILED", "failed to put workspace", response)
	default:
		return unexpectedResponse("PutWorkspace", response)
	}
}

func (m *Manager) deleteWorkspace(ctx context.Context, id string) (apitypes.Workspace, bool, error) {
	response, err := m.services.Workspaces.DeleteWorkspace(ctx, adminhttp.DeleteWorkspaceRequestObject{Id: id})
	if err != nil {
		return apitypes.Workspace{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.DeleteWorkspace200JSONResponse:
		return apitypes.Workspace(response), true, nil
	case adminhttp.DeleteWorkspace404JSONResponse:
		return apitypes.Workspace{}, false, nil
	case adminhttp.DeleteWorkspace409JSONResponse:
		return apitypes.Workspace{}, false, responseError(409, workspace.SystemWorkspaceDeleteForbiddenCode, "system Workspace deletion is forbidden", response)
	case adminhttp.DeleteWorkspace500JSONResponse:
		return apitypes.Workspace{}, false, responseError(500, "DELETE_WORKSPACE_FAILED", "failed to delete workspace", response)
	default:
		return apitypes.Workspace{}, false, unexpectedResponse("DeleteWorkspace", response)
	}
}

func workspaceSpec(workspace apitypes.Workspace) apitypes.WorkspaceSpec {
	return apitypes.WorkspaceSpec{
		Name:       workspace.Name,
		Parameters: workspace.Parameters,
		Toolkit:    workspace.Toolkit,
		WorkflowId: workspace.WorkflowId,
	}
}

func workspaceUpsert(resource apitypes.WorkspaceResource) adminhttp.WorkspaceUpsert {
	return adminhttp.WorkspaceUpsert{
		Id:         resource.Metadata.Id,
		Labels:     cloneWorkspaceLabels(resource.Metadata.Labels),
		Name:       resource.Spec.Name,
		Parameters: resource.Spec.Parameters,
		Toolkit:    resource.Spec.Toolkit,
		WorkflowId: resource.Spec.WorkflowId,
	}
}

func resourceFromWorkspace(item apitypes.Workspace) (apitypes.Resource, error) {
	return marshalResource(apitypes.WorkspaceResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.WorkspaceResourceKind(apitypes.ResourceKindWorkspace),
		Metadata:   apitypes.ResourceMetadata{Id: item.Id, Labels: cloneWorkspaceLabels(item.Labels)},
		Icon:       item.Icon,
		Spec:       workspaceSpec(item),
	})
}

func cloneWorkspaceLabels(labels *map[string]string) *map[string]string {
	if labels == nil {
		return nil
	}
	cloned := make(map[string]string, len(*labels))
	maps.Copy(cloned, *labels)
	return &cloned
}
