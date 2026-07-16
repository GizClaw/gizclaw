package resourcemanager

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
)

func (m *Manager) applyWorkflow(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.Workflows == nil {
		return apitypes.ApplyResult{}, missingService("workflows")
	}
	item, err := resource.AsWorkflowResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_WORKFLOW_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	spec, err := normalizeWorkflowResourceSpec(item.Spec)
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_WORKFLOW_RESOURCE", err.Error())
	}
	item.Spec = spec
	name := string(pathParam(item.Metadata.Name))
	existing, exists, err := m.getWorkflow(ctx, name)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if err := m.validateOwnedResourceOwner(apitypes.ACLResourceKindWorkflow, item.Metadata.Name, item.Metadata, exists); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(
			struct {
				Spec apitypes.WorkflowSpec  `json:"spec"`
				I18n *apitypes.WorkflowI18n `json:"i18n,omitempty"`
				Icon *apitypes.Icon         `json:"icon,omitempty"`
			}{Spec: existing.Spec, I18n: existing.I18n, Icon: existing.Icon},
			struct {
				Spec apitypes.WorkflowSpec  `json:"spec"`
				I18n *apitypes.WorkflowI18n `json:"i18n,omitempty"`
				Icon *apitypes.Icon         `json:"icon,omitempty"`
			}{Spec: item.Spec, I18n: item.I18n, Icon: item.Icon},
		)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			ownerChanged, err := m.ensureOwnedResourceOwnerFromMetadata(ctx, apitypes.ACLResourceKindWorkflow, item.Metadata.Name, item.Metadata)
			if err != nil {
				return apitypes.ApplyResult{}, err
			}
			if ownerChanged {
				return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindWorkflow, item.Metadata.Name), nil
			}
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindWorkflow, item.Metadata.Name), nil
		}
	}
	ownerRollback, err := m.ensureOwnedResourceOwnerBeforeWrite(ctx, apitypes.ACLResourceKindWorkflow, item.Metadata.Name, item.Metadata)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if err := m.putWorkflow(ctx, name, workflowFromResource(item)); err != nil {
		return apitypes.ApplyResult{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, err)
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindWorkflow, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindWorkflow, item.Metadata.Name), nil
}

func normalizeWorkflowResourceSpec(spec apitypes.WorkflowSpec) (apitypes.WorkflowSpec, error) {
	policy, err := toolkit.NormalizePolicy(spec.Toolkit)
	if err != nil {
		return spec, err
	}
	spec.Toolkit = policy
	return spec, nil
}

func (m *Manager) getWorkflow(ctx context.Context, name string) (apitypes.Workflow, bool, error) {
	response, err := m.services.Workflows.GetWorkflow(ctx, adminhttp.GetWorkflowRequestObject{Name: name})
	if err != nil {
		return apitypes.Workflow{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.GetWorkflow200JSONResponse:
		return apitypes.Workflow(response), true, nil
	case adminhttp.GetWorkflow404JSONResponse:
		return apitypes.Workflow{}, false, nil
	case adminhttp.GetWorkflow500JSONResponse:
		return apitypes.Workflow{}, false, responseError(500, "GET_WORKFLOW_FAILED", "failed to get workflow", response)
	default:
		return apitypes.Workflow{}, false, unexpectedResponse("GetWorkflow", response)
	}
}

func (m *Manager) putWorkflow(ctx context.Context, name string, body apitypes.Workflow) error {
	response, err := m.services.Workflows.PutWorkflow(ctx, adminhttp.PutWorkflowRequestObject{Name: name, Body: &body})
	if err != nil {
		return err
	}
	switch response := response.(type) {
	case adminhttp.PutWorkflow200JSONResponse:
		return nil
	case adminhttp.PutWorkflow400JSONResponse:
		return responseError(400, "PUT_WORKFLOW_FAILED", "failed to put workflow", response)
	case adminhttp.PutWorkflow500JSONResponse:
		return responseError(500, "PUT_WORKFLOW_FAILED", "failed to put workflow", response)
	default:
		return unexpectedResponse("PutWorkflow", response)
	}
}

func (m *Manager) deleteWorkflow(ctx context.Context, name string) (apitypes.Workflow, bool, error) {
	response, err := m.services.Workflows.DeleteWorkflow(ctx, adminhttp.DeleteWorkflowRequestObject{Name: name})
	if err != nil {
		return apitypes.Workflow{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.DeleteWorkflow200JSONResponse:
		return apitypes.Workflow(response), true, nil
	case adminhttp.DeleteWorkflow404JSONResponse:
		return apitypes.Workflow{}, false, nil
	case adminhttp.DeleteWorkflow500JSONResponse:
		return apitypes.Workflow{}, false, responseError(500, "DELETE_WORKFLOW_FAILED", "failed to delete workflow", response)
	default:
		return apitypes.Workflow{}, false, unexpectedResponse("DeleteWorkflow", response)
	}
}

func resourceFromWorkflow(_ string, item apitypes.Workflow) (apitypes.Resource, error) {
	return marshalResource(apitypes.WorkflowResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.WorkflowResourceKind(apitypes.ResourceKindWorkflow),
		Metadata:   apitypes.ResourceMetadata{Name: item.Name},
		I18n:       item.I18n,
		Icon:       item.Icon,
		Spec:       item.Spec,
	})
}

func workflowFromResource(item apitypes.WorkflowResource) apitypes.Workflow {
	return apitypes.Workflow{
		I18n: item.I18n,
		Icon: item.Icon,
		Name: item.Metadata.Name,
		Spec: item.Spec,
	}
}
