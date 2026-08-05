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
	if err := validateResourceHeader(item.ApiVersion, item.Metadata); err != nil {
		return apitypes.ApplyResult{}, err
	}
	spec, err := normalizeWorkflowResourceSpec(item.Spec)
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_WORKFLOW_RESOURCE", err.Error())
	}
	item.Spec = spec
	body := workflowFromResource(item)
	return applyConcreteResource(ctx, item.Metadata, apitypes.ResourceKindWorkflow, item.Spec,
		m.getWorkflow,
		func(ctx context.Context) (string, error) { return m.createWorkflow(ctx, body) },
		func(ctx context.Context, id string) error { return m.putWorkflow(ctx, id, body) },
		func(value apitypes.Workflow) apitypes.WorkflowSpec { return value.Spec })
}

func (m *Manager) createWorkflow(ctx context.Context, body adminhttp.WorkflowUpsert) (string, error) {
	response, err := m.services.Workflows.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &body})
	if err != nil {
		return "", err
	}
	switch response := response.(type) {
	case adminhttp.CreateWorkflow200JSONResponse:
		return response.Id, nil
	case adminhttp.CreateWorkflow400JSONResponse:
		return "", responseError(400, "CREATE_WORKFLOW_FAILED", "failed to create workflow", response)
	case adminhttp.CreateWorkflow409JSONResponse:
		return "", responseError(409, "CREATE_WORKFLOW_FAILED", "failed to create workflow", response)
	case adminhttp.CreateWorkflow500JSONResponse:
		return "", responseError(500, "CREATE_WORKFLOW_FAILED", "failed to create workflow", response)
	default:
		return "", unexpectedResponse("CreateWorkflow", response)
	}
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
	response, err := m.services.Workflows.GetWorkflow(ctx, adminhttp.GetWorkflowRequestObject{Id: name})
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

func (m *Manager) putWorkflow(ctx context.Context, name string, body adminhttp.WorkflowUpsert) error {
	response, err := m.services.Workflows.PutWorkflow(ctx, adminhttp.PutWorkflowRequestObject{Id: name, Body: &body})
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
	response, err := m.services.Workflows.DeleteWorkflow(ctx, adminhttp.DeleteWorkflowRequestObject{Id: name})
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
		Metadata:   apitypes.ResourceMetadata{Id: item.Id},
		Spec:       item.Spec,
	})
}

func workflowFromResource(item apitypes.WorkflowResource) adminhttp.WorkflowUpsert {
	return adminhttp.WorkflowUpsert{
		Id:   item.Metadata.Id,
		Spec: item.Spec,
	}
}
