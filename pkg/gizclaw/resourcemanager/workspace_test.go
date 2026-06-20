package resourcemanager

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func TestApplyWorkspaceCreatesResource(t *testing.T) {
	workspaces := newFakeWorkspaces()
	manager := New(Services{Workspaces: workspaces})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Workspace",
		"metadata": {"name": "demo"},
		"spec": {
			"workflow_name": "workflow",
			"parameters": {"topic": "demo"}
		}
	}`))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("action = %q, want created", result.Action)
	}
	if workspaces.putCount != 1 {
		t.Fatalf("putCount = %d, want 1", workspaces.putCount)
	}
	if workspaces.items["demo"].WorkflowName != "workflow" {
		t.Fatalf("workflow = %q, want workflow", workspaces.items["demo"].WorkflowName)
	}
}

func TestGetWorkspaceReturnsResource(t *testing.T) {
	workspaces := newFakeWorkspaces()
	workspaces.items["demo"] = apitypes.Workspace{
		CreatedAt:    time.Now().UTC(),
		Name:         "demo",
		Parameters:   testFlowcraftWorkspaceParameters(),
		UpdatedAt:    time.Now().UTC(),
		WorkflowName: "workflow",
	}
	manager := New(Services{Workspaces: workspaces})

	resource, err := manager.Get(context.Background(), apitypes.ResourceKindWorkspace, "demo")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	workspace, err := resource.AsWorkspaceResource()
	if err != nil {
		t.Fatalf("AsWorkspaceResource returned error: %v", err)
	}
	if workspace.Metadata.Name != "demo" {
		t.Fatalf("metadata.name = %q, want demo", workspace.Metadata.Name)
	}
	if workspace.Spec.WorkflowName != "workflow" {
		t.Fatalf("workflow_name = %q, want workflow", workspace.Spec.WorkflowName)
	}
}

func TestPutWorkspaceWritesResource(t *testing.T) {
	workspaces := newFakeWorkspaces()
	manager := New(Services{Workspaces: workspaces})

	_, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Workspace",
		"metadata": {"name": "demo"},
		"spec": {
			"workflow_name": "workflow"
		}
	}`))
	if err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
	if workspaces.putCount != 1 {
		t.Fatalf("putCount = %d, want 1", workspaces.putCount)
	}
}

func TestApplyWorkspaceUnchangedSkipsPut(t *testing.T) {
	workspaces := newFakeWorkspaces()
	workspaces.items["demo"] = apitypes.Workspace{
		CreatedAt:    time.Now().UTC(),
		Name:         "demo",
		UpdatedAt:    time.Now().UTC(),
		WorkflowName: "workflow",
	}
	manager := New(Services{Workspaces: workspaces})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Workspace",
		"metadata": {"name": "demo"},
		"spec": {
			"workflow_name": "workflow"
		}
	}`))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("action = %q, want unchanged", result.Action)
	}
	if workspaces.putCount != 0 {
		t.Fatalf("putCount = %d, want 0", workspaces.putCount)
	}
}

func TestApplyWorkspaceUpdatesResource(t *testing.T) {
	workspaces := newFakeWorkspaces()
	workspaces.items["demo"] = apitypes.Workspace{
		CreatedAt:    time.Now().UTC(),
		Name:         "demo",
		UpdatedAt:    time.Now().UTC(),
		WorkflowName: "old-workflow",
	}
	manager := New(Services{Workspaces: workspaces})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Workspace",
		"metadata": {"name": "demo"},
		"spec": {
			"workflow_name": "new-workflow"
		}
	}`))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("action = %q, want updated", result.Action)
	}
	if workspaces.putCount != 1 {
		t.Fatalf("putCount = %d, want 1", workspaces.putCount)
	}
}

func TestWorkspaceServiceErrorResponses(t *testing.T) {
	workspaces := newFakeWorkspaces()
	manager := New(Services{Workspaces: workspaces})

	workspaces.getStatus = 500
	_, _, err := manager.getWorkspace(context.Background(), "demo")
	assertResourceError(t, err, 500, "INTERNAL_ERROR")

	workspaces.getStatus = 0
	workspaces.putStatus = 400
	err = manager.putWorkspace(context.Background(), "demo", adminservice.WorkspaceUpsert{})
	assertResourceError(t, err, 400, "INVALID_WORKSPACE")

	workspaces.putStatus = 500
	err = manager.putWorkspace(context.Background(), "demo", adminservice.WorkspaceUpsert{})
	assertResourceError(t, err, 500, "INTERNAL_ERROR")
}

type fakeWorkspaces struct {
	items     map[string]apitypes.Workspace
	putCount  int
	getStatus int
	putStatus int
}

func newFakeWorkspaces() *fakeWorkspaces {
	return &fakeWorkspaces{items: map[string]apitypes.Workspace{}}
}

func (f *fakeWorkspaces) ListWorkspaces(context.Context, adminservice.ListWorkspacesRequestObject) (adminservice.ListWorkspacesResponseObject, error) {
	return nil, nil
}

func (f *fakeWorkspaces) CreateWorkspace(context.Context, adminservice.CreateWorkspaceRequestObject) (adminservice.CreateWorkspaceResponseObject, error) {
	return nil, nil
}

func (f *fakeWorkspaces) DeleteWorkspace(_ context.Context, request adminservice.DeleteWorkspaceRequestObject) (adminservice.DeleteWorkspaceResponseObject, error) {
	item, ok := f.items[string(request.Name)]
	if !ok {
		return adminservice.DeleteWorkspace404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", "not found")), nil
	}
	delete(f.items, string(request.Name))
	return adminservice.DeleteWorkspace200JSONResponse(item), nil
}

func (f *fakeWorkspaces) GetWorkspace(_ context.Context, request adminservice.GetWorkspaceRequestObject) (adminservice.GetWorkspaceResponseObject, error) {
	if f.getStatus == 500 {
		return adminservice.GetWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
	}
	item, ok := f.items[string(request.Name)]
	if !ok {
		return adminservice.GetWorkspace404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", "not found")), nil
	}
	return adminservice.GetWorkspace200JSONResponse(item), nil
}

func (f *fakeWorkspaces) PutWorkspace(_ context.Context, request adminservice.PutWorkspaceRequestObject) (adminservice.PutWorkspaceResponseObject, error) {
	switch f.putStatus {
	case 400:
		return adminservice.PutWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", "invalid")), nil
	case 500:
		return adminservice.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
	}
	f.putCount++
	body := *request.Body
	now := time.Now().UTC()
	item := apitypes.Workspace{
		CreatedAt:    now,
		Name:         body.Name,
		Parameters:   body.Parameters,
		UpdatedAt:    now,
		WorkflowName: body.WorkflowName,
	}
	f.items[string(request.Name)] = item
	return adminservice.PutWorkspace200JSONResponse(item), nil
}
