package resourcemanager

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
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
	spec, err := normalizeWorkspaceResourceSpec(item.Spec)
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_WORKSPACE_RESOURCE", err.Error())
	}
	item.Spec = spec
	name := string(pathParam(item.Metadata.Name))
	existing, exists, err := m.getWorkspace(ctx, name)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if err := m.validateOwnedResourceOwner(apitypes.ACLResourceKindWorkspace, item.Metadata.Name, item.Metadata, exists); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(
			struct {
				Spec apitypes.WorkspaceSpec `json:"spec"`
				Icon *apitypes.Icon         `json:"icon,omitempty"`
			}{Spec: workspaceSpec(existing), Icon: existing.Icon},
			struct {
				Spec apitypes.WorkspaceSpec `json:"spec"`
				Icon *apitypes.Icon         `json:"icon,omitempty"`
			}{Spec: item.Spec, Icon: item.Icon},
		)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			ownerChanged, err := m.ensureOwnedResourceOwnerFromMetadata(ctx, apitypes.ACLResourceKindWorkspace, item.Metadata.Name, item.Metadata)
			if err != nil {
				return apitypes.ApplyResult{}, err
			}
			if ownerChanged {
				return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindWorkspace, item.Metadata.Name), nil
			}
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindWorkspace, item.Metadata.Name), nil
		}
	}
	ownerRollback, err := m.ensureOwnedResourceOwnerBeforeWrite(ctx, apitypes.ACLResourceKindWorkspace, item.Metadata.Name, item.Metadata)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if err := m.putWorkspace(ctx, name, workspaceUpsert(item)); err != nil {
		return apitypes.ApplyResult{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, err)
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindWorkspace, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindWorkspace, item.Metadata.Name), nil
}

func normalizeWorkspaceResourceSpec(spec apitypes.WorkspaceSpec) (apitypes.WorkspaceSpec, error) {
	policy, err := toolkit.NormalizePolicy(spec.Toolkit)
	if err != nil {
		return spec, err
	}
	spec.Toolkit = policy
	return spec, nil
}

func (m *Manager) getWorkspace(ctx context.Context, name string) (apitypes.Workspace, bool, error) {
	response, err := m.services.Workspaces.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Name: name})
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

func (m *Manager) putWorkspace(ctx context.Context, name string, body adminhttp.WorkspaceUpsert) error {
	response, err := m.services.Workspaces.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Name: name, Body: &body})
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

func (m *Manager) deleteWorkspace(ctx context.Context, name string) (apitypes.Workspace, bool, error) {
	response, err := m.services.Workspaces.DeleteWorkspace(ctx, adminhttp.DeleteWorkspaceRequestObject{Name: name})
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
		Parameters:   workspace.Parameters,
		Toolkit:      workspace.Toolkit,
		WorkflowName: workspace.WorkflowName,
	}
}

func workspaceUpsert(resource apitypes.WorkspaceResource) adminhttp.WorkspaceUpsert {
	return adminhttp.WorkspaceUpsert{
		Icon:         resource.Icon,
		Name:         string(resource.Metadata.Name),
		Parameters:   resource.Spec.Parameters,
		Toolkit:      resource.Spec.Toolkit,
		WorkflowName: resource.Spec.WorkflowName,
	}
}

func resourceFromWorkspace(item apitypes.Workspace) (apitypes.Resource, error) {
	return marshalResource(apitypes.WorkspaceResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.WorkspaceResourceKind(apitypes.ResourceKindWorkspace),
		Metadata:   apitypes.ResourceMetadata{Name: string(item.Name)},
		Icon:       item.Icon,
		Spec:       workspaceSpec(item),
	})
}
