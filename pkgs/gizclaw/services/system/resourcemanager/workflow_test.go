package resourcemanager

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestApplyWorkflowCreatesResource(t *testing.T) {
	workflows := newFakeWorkflows()
	manager := New(Services{Workflows: workflows})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Workflow",
		"metadata": {"name": "workflow"},
		"i18n": {
			"default_locale": "en",
			"en": {"name": "Workflow", "description": "A workflow"},
			"zh-CN": {}
		},
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {"prompt": "hello"}
		}
	}`))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("action = %q, want created", result.Action)
	}
	if workflows.putCount != 1 {
		t.Fatalf("putCount = %d, want 1", workflows.putCount)
	}
	stored, ok := workflows.items["workflow"]
	if !ok {
		t.Fatal("stored workflow missing")
	}
	if stored.I18n == nil || len(stored.I18n.AdditionalProperties) != 2 {
		t.Fatalf("stored i18n = %#v", stored.I18n)
	}
}

func TestGetWorkflowReturnsResource(t *testing.T) {
	workflows := newFakeWorkflows()
	workflows.items["workflow"] = mustWorkflowDocument(t, `{
		"metadata": {"name": "workflow"},
		"i18n": {
			"default_locale": "en",
			"en": {"name": "Workflow"},
			"zh-CN": {"description": "工作流"}
		},
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {"prompt": "hello"}
		}
	}`)
	manager := New(Services{Workflows: workflows})

	resource, err := manager.Get(context.Background(), apitypes.ResourceKindWorkflow, "workflow")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	workflow, err := resource.AsWorkflowResource()
	if err != nil {
		t.Fatalf("AsWorkflowResource returned error: %v", err)
	}
	if workflow.Metadata.Name != "workflow" {
		t.Fatalf("metadata.name = %q, want workflow", workflow.Metadata.Name)
	}
	if workflow.I18n == nil || len(workflow.I18n.AdditionalProperties) != 2 {
		t.Fatalf("i18n = %#v", workflow.I18n)
	}
}

func TestPutWorkflowWritesResource(t *testing.T) {
	workflows := newFakeWorkflows()
	manager := New(Services{Workflows: workflows})

	_, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Workflow",
		"metadata": {"name": "workflow"},
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {"prompt": "hello"}
		}
	}`))
	if err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
	if workflows.putCount != 1 {
		t.Fatalf("putCount = %d, want 1", workflows.putCount)
	}
}

func TestApplyWorkflowUnchangedSkipsPut(t *testing.T) {
	workflows := newFakeWorkflows()
	workflows.items["workflow"] = mustWorkflowDocument(t, `{
		"metadata": {"name": "workflow"},
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {"prompt": "hello"}
		}
	}`)
	manager := New(Services{Workflows: workflows})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Workflow",
		"metadata": {"name": "workflow"},
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {"prompt": "hello"}
		}
	}`))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("action = %q, want unchanged", result.Action)
	}
	if workflows.putCount != 0 {
		t.Fatalf("putCount = %d, want 0", workflows.putCount)
	}
}

func TestApplyWorkflowNormalizesToolkitPolicyBeforeCompare(t *testing.T) {
	workflows := newFakeWorkflows()
	workflows.items["workflow"] = mustWorkflowDocument(t, `{
		"metadata": {"name": "workflow"},
		"spec": {
			"driver": "flowcraft",
			"toolkit": {"tool_ids": ["system.mode.switch", "system.music.play"]},
			"flowcraft": {"prompt": "hello"}
		}
	}`)
	manager := New(Services{Workflows: workflows})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Workflow",
		"metadata": {"name": "workflow"},
		"spec": {
			"driver": "flowcraft",
			"toolkit": {"tool_ids": [" system.music.play ", "system.mode.switch", "system.music.play"]},
			"flowcraft": {"prompt": "hello"}
		}
	}`))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("action = %q, want unchanged", result.Action)
	}
	if workflows.putCount != 0 {
		t.Fatalf("putCount = %d, want 0", workflows.putCount)
	}
}

func TestApplyWorkflowUpdatesResource(t *testing.T) {
	workflows := newFakeWorkflows()
	workflows.items["workflow"] = mustWorkflowDocument(t, `{
		"metadata": {"name": "workflow"},
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {"prompt": "old"}
		}
	}`)
	manager := New(Services{Workflows: workflows})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Workflow",
		"metadata": {"name": "workflow"},
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {"prompt": "new"}
		}
	}`))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("action = %q, want updated", result.Action)
	}
	if workflows.putCount != 1 {
		t.Fatalf("putCount = %d, want 1", workflows.putCount)
	}
}

func TestApplyWorkflowUpdatesI18nOnly(t *testing.T) {
	workflows := newFakeWorkflows()
	workflows.items["workflow"] = mustWorkflowDocument(t, `{
		"metadata": {"name": "workflow"},
		"i18n": {"default_locale": "en", "en": {"description": "old"}},
		"spec": {"driver": "flowcraft", "flowcraft": {"prompt": "same"}}
	}`)
	manager := New(Services{Workflows: workflows})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Workflow",
		"metadata": {"name": "workflow"},
		"i18n": {"default_locale": "en", "en": {"description": "new"}},
		"spec": {"driver": "flowcraft", "flowcraft": {"prompt": "same"}}
	}`))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUpdated || workflows.putCount != 1 {
		t.Fatalf("Apply result = %#v, putCount = %d", result, workflows.putCount)
	}
	catalog := workflows.items["workflow"].I18n.AdditionalProperties["en"]
	if catalog.Description == nil || *catalog.Description != "new" {
		t.Fatalf("stored catalog = %#v", catalog)
	}
}

func TestWorkflowServiceErrorResponses(t *testing.T) {
	workflows := newFakeWorkflows()
	manager := New(Services{Workflows: workflows})

	workflows.getStatus = 500
	_, _, err := manager.getWorkflow(context.Background(), "workflow")
	assertResourceError(t, err, 500, "INTERNAL_ERROR")

	workflows.getStatus = 0
	workflows.putStatus = 400
	err = manager.putWorkflow(context.Background(), "workflow", apitypes.WorkflowDocument{})
	assertResourceError(t, err, 400, "INVALID_WORKFLOW")

	workflows.putStatus = 500
	err = manager.putWorkflow(context.Background(), "workflow", apitypes.WorkflowDocument{})
	assertResourceError(t, err, 500, "INTERNAL_ERROR")
}

type fakeWorkflows struct {
	items     map[string]apitypes.WorkflowDocument
	putCount  int
	getStatus int
	putStatus int
}

func newFakeWorkflows() *fakeWorkflows {
	return &fakeWorkflows{items: map[string]apitypes.WorkflowDocument{}}
}

func (f *fakeWorkflows) ListWorkflows(context.Context, adminhttp.ListWorkflowsRequestObject) (adminhttp.ListWorkflowsResponseObject, error) {
	return nil, nil
}

func (f *fakeWorkflows) CreateWorkflow(context.Context, adminhttp.CreateWorkflowRequestObject) (adminhttp.CreateWorkflowResponseObject, error) {
	return nil, nil
}

func (f *fakeWorkflows) DeleteWorkflow(_ context.Context, request adminhttp.DeleteWorkflowRequestObject) (adminhttp.DeleteWorkflowResponseObject, error) {
	item, ok := f.items[string(request.Name)]
	if !ok {
		return adminhttp.DeleteWorkflow404JSONResponse(apitypes.NewErrorResponse("WORKFLOW_NOT_FOUND", "not found")), nil
	}
	delete(f.items, string(request.Name))
	return adminhttp.DeleteWorkflow200JSONResponse(item), nil
}

func (f *fakeWorkflows) GetWorkflow(_ context.Context, request adminhttp.GetWorkflowRequestObject) (adminhttp.GetWorkflowResponseObject, error) {
	if f.getStatus == 500 {
		return adminhttp.GetWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
	}
	item, ok := f.items[string(request.Name)]
	if !ok {
		return adminhttp.GetWorkflow404JSONResponse(apitypes.NewErrorResponse("WORKFLOW_NOT_FOUND", "not found")), nil
	}
	return adminhttp.GetWorkflow200JSONResponse(item), nil
}

func (f *fakeWorkflows) PutWorkflow(_ context.Context, request adminhttp.PutWorkflowRequestObject) (adminhttp.PutWorkflowResponseObject, error) {
	switch f.putStatus {
	case 400:
		return adminhttp.PutWorkflow400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKFLOW", "invalid")), nil
	case 500:
		return adminhttp.PutWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
	}
	f.putCount++
	f.items[string(request.Name)] = *request.Body
	return adminhttp.PutWorkflow200JSONResponse(*request.Body), nil
}
