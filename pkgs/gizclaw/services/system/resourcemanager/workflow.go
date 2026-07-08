package resourcemanager

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
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
	name := string(pathParam(item.Metadata.Name))
	existing, exists, err := m.getWorkflow(ctx, name)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(existing.Spec, item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindWorkflow, item.Metadata.Name), nil
		}
	}
	if err := m.putWorkflow(ctx, name, workflowDocumentFromResource(item)); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindWorkflow, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindWorkflow, item.Metadata.Name), nil
}

func (m *Manager) getWorkflow(ctx context.Context, name string) (apitypes.WorkflowDocument, bool, error) {
	response, err := m.services.Workflows.GetWorkflow(ctx, adminhttp.GetWorkflowRequestObject{Name: name})
	if err != nil {
		return apitypes.WorkflowDocument{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.GetWorkflow200JSONResponse:
		return apitypes.WorkflowDocument(response), true, nil
	case adminhttp.GetWorkflow404JSONResponse:
		return apitypes.WorkflowDocument{}, false, nil
	case adminhttp.GetWorkflow500JSONResponse:
		return apitypes.WorkflowDocument{}, false, responseError(500, "GET_WORKFLOW_FAILED", "failed to get workflow", response)
	default:
		return apitypes.WorkflowDocument{}, false, unexpectedResponse("GetWorkflow", response)
	}
}

func (m *Manager) putWorkflow(ctx context.Context, name string, body apitypes.WorkflowDocument) error {
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

func (m *Manager) deleteWorkflow(ctx context.Context, name string) (apitypes.WorkflowDocument, bool, error) {
	response, err := m.services.Workflows.DeleteWorkflow(ctx, adminhttp.DeleteWorkflowRequestObject{Name: name})
	if err != nil {
		return apitypes.WorkflowDocument{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.DeleteWorkflow200JSONResponse:
		return apitypes.WorkflowDocument(response), true, nil
	case adminhttp.DeleteWorkflow404JSONResponse:
		return apitypes.WorkflowDocument{}, false, nil
	case adminhttp.DeleteWorkflow500JSONResponse:
		return apitypes.WorkflowDocument{}, false, responseError(500, "DELETE_WORKFLOW_FAILED", "failed to delete workflow", response)
	default:
		return apitypes.WorkflowDocument{}, false, unexpectedResponse("DeleteWorkflow", response)
	}
}

func resourceFromWorkflow(name string, item apitypes.WorkflowDocument) (apitypes.Resource, error) {
	return marshalResource(apitypes.WorkflowResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.WorkflowResourceKind(apitypes.ResourceKindWorkflow),
		Metadata:   apitypes.ResourceMetadata{Name: name},
		Spec:       item.Spec,
	})
}

func workflowDocumentFromResource(item apitypes.WorkflowResource) apitypes.WorkflowDocument {
	return apitypes.WorkflowDocument{
		Metadata: apitypes.WorkflowMetadata{Name: item.Metadata.Name},
		Spec:     item.Spec,
	}
}
