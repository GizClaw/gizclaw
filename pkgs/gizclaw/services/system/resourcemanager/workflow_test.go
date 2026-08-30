package resourcemanager

import (
	"context"
	"encoding/json"
	"strings"
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
		"metadata": {"id": "workflow"},
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {"graph": {"name": "assistant", "entry": "answer", "nodes": [{"id": "answer", "type":"inference", "publish": true, "config":{"model":{"id":{"provider":"gizclaw","name":"pet-care.model"}},"messages_channel":"answer","stream":true}}], "edges": [{"from": "answer", "to": "__end__"}]}, "voice_adapter": {"default_voice": "pet-care.pet"}}		}
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
	raw, err := json.Marshal(stored.Spec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"name":"pet-care.model"`) || !strings.Contains(string(raw), `"default_voice":"pet-care.pet"`) {
		t.Fatalf("stored Workflow aliases = %s", raw)
	}
	unchanged, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Workflow",
		"metadata": {"id": "workflow"},
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {"graph": {"name": "assistant", "entry": "answer", "nodes": [{"id": "answer", "type":"inference", "publish": true, "config":{"model":{"id":{"provider":"gizclaw","name":"pet-care.model"}},"messages_channel":"answer","stream":true}}], "edges": [{"from": "answer", "to": "__end__"}]}, "voice_adapter": {"default_voice": "pet-care.pet"}}		}
	}`))
	if err != nil {
		t.Fatalf("Apply unchanged Workflow returned error: %v", err)
	}
	if unchanged.Action != apitypes.ApplyActionUnchanged || workflows.putCount != 1 {
		t.Fatalf("Apply unchanged Workflow = %#v, putCount = %d", unchanged, workflows.putCount)
	}
}

func TestGetWorkflowReturnsResource(t *testing.T) {
	workflows := newFakeWorkflows()
	workflows.items["workflow"] = mustWorkflow(t, `{
		"id": "workflow",
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {"graph": {"name": "assistant", "entry": "answer", "nodes": [{"id": "answer", "type":"inference", "publish": true, "config":{"model":{"id":{"provider":"gizclaw","name":"pet-care.model"}},"messages_channel":"answer","stream":true}}], "edges": [{"from": "answer", "to": "__end__"}]}, "voice_adapter": {"default_voice": "pet-care.pet"}}		}
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
	if metadataID(t, workflow.Metadata) != "workflow" {
		t.Fatalf("metadata.id = %q, want workflow", metadataID(t, workflow.Metadata))
	}
	if workflow.Spec.Driver != apitypes.WorkflowDriverFlowcraft {
		t.Fatalf("spec = %#v", workflow.Spec)
	}
}

func TestPutWorkflowWritesResource(t *testing.T) {
	workflows := newFakeWorkflows()
	workflows.items["workflow"] = mustWorkflow(t, `{"id":"workflow","spec":{"driver":"flowcraft","flowcraft":{"graph":{"name":"old","entry":"answer","nodes":[{"id":"answer","type":"inference","publish":true,"config":{"model":{"id":{"provider":"gizclaw","name":"llm"}},"messages_channel":"answer","stream":true}}],"edges":[{"from":"answer","to":"__end__"}]}}}}`)
	manager := New(Services{Workflows: workflows})

	_, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Workflow",
		"metadata": {"id": "workflow"},
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {"graph": {"name": "assistant", "entry": "answer", "nodes": [{"id": "answer", "type":"inference", "publish": true, "config":{"model":{"id":{"provider":"gizclaw","name":"pet-care.model"}},"messages_channel":"answer","stream":true}}], "edges": [{"from": "answer", "to": "__end__"}]}, "voice_adapter": {"default_voice": "pet-care.pet"}}		}
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
	workflows.items["workflow"] = mustWorkflow(t, `{
		"name": "workflow",
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {"graph": {"name": "assistant", "entry": "answer", "nodes": [{"id": "answer", "type":"inference", "publish": true, "config":{"model":{"id":{"provider":"gizclaw","name":"llm"}},"messages_channel":"answer","stream":true}}], "edges": [{"from": "answer", "to": "__end__"}]}}		}
	}`)
	manager := New(Services{Workflows: workflows})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Workflow",
		"metadata": {"id": "workflow"},
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {"graph": {"name": "assistant", "entry": "answer", "nodes": [{"id": "answer", "type":"inference", "publish": true, "config":{"model":{"id":{"provider":"gizclaw","name":"llm"}},"messages_channel":"answer","stream":true}}], "edges": [{"from": "answer", "to": "__end__"}]}}		}
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

func TestApplyWorkflowCanonicalizesToolkitPolicyBeforeCompare(t *testing.T) {
	workflows := newFakeWorkflows()
	workflows.items["workflow"] = mustWorkflow(t, `{
		"name": "workflow",
		"spec": {
			"driver": "flowcraft",
			"toolkit": {"tool_ids": ["system.mode.switch", "system.music.play"]},
			"flowcraft": {"graph": {"name": "assistant", "entry": "answer", "nodes": [{"id": "answer", "type":"inference", "publish": true, "config":{"model":{"id":{"provider":"gizclaw","name":"llm"}},"messages_channel":"answer","stream":true}}], "edges": [{"from": "answer", "to": "__end__"}]}}		}
	}`)
	manager := New(Services{Workflows: workflows})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Workflow",
		"metadata": {"id": "workflow"},
		"spec": {
			"driver": "flowcraft",
			"toolkit": {"tool_ids": ["system.music.play", "system.mode.switch", "system.music.play"]},
			"flowcraft": {"graph": {"name": "assistant", "entry": "answer", "nodes": [{"id": "answer", "type":"inference", "publish": true, "config":{"model":{"id":{"provider":"gizclaw","name":"llm"}},"messages_channel":"answer","stream":true}}], "edges": [{"from": "answer", "to": "__end__"}]}}		}
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

	_, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Workflow",
		"metadata": {"id": "workflow"},
		"spec": {
			"driver": "flowcraft",
			"toolkit": {"tool_ids": [" system.music.play "]},
			"flowcraft": {"graph": {"name": "assistant", "entry": "answer", "nodes": [{"id": "answer", "type":"inference", "publish": true, "config":{"model":{"id":{"provider":"gizclaw","name":"llm"}},"messages_channel":"answer","stream":true}}], "edges": [{"from": "answer", "to": "__end__"}]}}		}
	}`))
	if err == nil || !strings.Contains(err.Error(), "surrounding whitespace") {
		t.Fatalf("Apply(whitespace tool ID) error = %v", err)
	}
}

func TestApplyWorkflowUpdatesResource(t *testing.T) {
	workflows := newFakeWorkflows()
	workflows.items["workflow"] = mustWorkflow(t, `{
		"name": "workflow",
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {"graph": {"name": "old-assistant", "entry": "answer", "nodes": [{"id": "answer", "type":"inference", "publish": true, "config":{"model":{"id":{"provider":"gizclaw","name":"llm"}},"messages_channel":"answer","stream":true}}], "edges": [{"from": "answer", "to": "__end__"}]}}		}
	}`)
	manager := New(Services{Workflows: workflows})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Workflow",
		"metadata": {"id": "workflow"},
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {"graph": {"name": "new-assistant", "entry": "answer", "nodes": [{"id": "answer", "type":"inference", "publish": true, "config":{"model":{"id":{"provider":"gizclaw","name":"llm"}},"messages_channel":"answer","stream":true}}], "edges": [{"from": "answer", "to": "__end__"}]}}		}
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

func TestWorkflowServiceErrorResponses(t *testing.T) {
	workflows := newFakeWorkflows()
	manager := New(Services{Workflows: workflows})

	workflows.getStatus = 500
	_, _, err := manager.getWorkflow(context.Background(), "workflow")
	assertResourceError(t, err, 500, "INTERNAL_ERROR")

	workflows.getStatus = 0
	workflows.putStatus = 400
	err = manager.putWorkflow(context.Background(), "workflow", adminhttp.WorkflowUpsert{})
	assertResourceError(t, err, 400, "INVALID_WORKFLOW")

	workflows.putStatus = 500
	err = manager.putWorkflow(context.Background(), "workflow", adminhttp.WorkflowUpsert{})
	assertResourceError(t, err, 500, "INTERNAL_ERROR")
}

type fakeWorkflows struct {
	items     map[string]apitypes.Workflow
	putCount  int
	getStatus int
	putStatus int
}

func newFakeWorkflows() *fakeWorkflows {
	return &fakeWorkflows{items: map[string]apitypes.Workflow{}}
}

func (f *fakeWorkflows) ListWorkflows(context.Context, adminhttp.ListWorkflowsRequestObject) (adminhttp.ListWorkflowsResponseObject, error) {
	return nil, nil
}

func (f *fakeWorkflows) CreateWorkflow(_ context.Context, request adminhttp.CreateWorkflowRequestObject) (adminhttp.CreateWorkflowResponseObject, error) {
	f.putCount++
	body := *request.Body
	item := apitypes.Workflow{Id: body.Id, Spec: body.Spec}
	f.items[item.Id] = item
	return adminhttp.CreateWorkflow200JSONResponse(item), nil
}

func (f *fakeWorkflows) DeleteWorkflow(_ context.Context, request adminhttp.DeleteWorkflowRequestObject) (adminhttp.DeleteWorkflowResponseObject, error) {
	item, ok := f.items[string(request.Id)]
	if !ok {
		return adminhttp.DeleteWorkflow404JSONResponse(apitypes.NewErrorResponse("WORKFLOW_NOT_FOUND", "not found")), nil
	}
	delete(f.items, string(request.Id))
	return adminhttp.DeleteWorkflow200JSONResponse(item), nil
}

func (f *fakeWorkflows) GetWorkflow(_ context.Context, request adminhttp.GetWorkflowRequestObject) (adminhttp.GetWorkflowResponseObject, error) {
	if f.getStatus == 500 {
		return adminhttp.GetWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
	}
	item, ok := f.items[string(request.Id)]
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
	body := *request.Body
	item := apitypes.Workflow{Id: string(request.Id), Spec: body.Spec}
	f.items[string(request.Id)] = item
	return adminhttp.PutWorkflow200JSONResponse(item), nil
}
