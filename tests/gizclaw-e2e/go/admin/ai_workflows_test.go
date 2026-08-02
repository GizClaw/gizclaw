//go:build gizclaw_e2e

package admin_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestAdminAPIWorkflowsListGetPaginationAndMutation(t *testing.T) {
	env := newAdminAPIHarness(t)

	all := collectAdminPages(t, 25, func(cursor *string, limit int32) ([]apitypes.Workflow, bool, *string) {
		resp, err := env.api.ListWorkflowsWithResponse(env.ctx, &adminhttp.ListWorkflowsParams{Cursor: cursor, Limit: &limit})
		if err != nil {
			t.Fatalf("list workflows: %v", err)
		}
		requireStatusOK(t, resp, resp.Body)
		if resp.JSON200 == nil {
			t.Fatalf("list workflows missing JSON200")
		}
		return resp.JSON200.Items, resp.JSON200.HasNext, resp.JSON200.NextCursor
	})
	seed := requireName(t, all, "flowcraft-chat-assistant", func(item apitypes.Workflow) string { return item.Name })
	requirePrefixCount(t, all, "flowcraft-scenario-", 100, func(item apitypes.Workflow) string { return item.Name })

	get, err := env.api.GetWorkflowWithResponse(env.ctx, seed.Id)
	if err != nil {
		t.Fatalf("get workflow: %v", err)
	}
	requireStatusOK(t, get, get.Body)
	if get.JSON200 == nil || get.JSON200.Id != seed.Id || get.JSON200.Name != seed.Name || get.JSON200.Spec.Driver != apitypes.WorkflowDriverFlowcraft {
		t.Fatalf("get workflow = %#v", get.JSON200)
	}

	name := mutationName("workflow")
	created, err := env.api.CreateWorkflowWithResponse(env.ctx, adminhttp.WorkflowUpsert{
		Name: name,
		Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverFlowcraft, Flowcraft: testFlowcraftWorkflowSpec()},
	})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	requireStatusOK(t, created, created.Body)
	if created.JSON200 == nil || created.JSON200.Name != name {
		t.Fatalf("created workflow = %#v", created.JSON200)
	}
	deleted, err := env.api.DeleteWorkflowWithResponse(env.ctx, created.JSON200.Id)
	if err != nil {
		t.Fatalf("delete workflow: %v", err)
	}
	requireStatusOK(t, deleted, deleted.Body)
}

func TestAdminAPIWorkflowHasExecutionDefinitionOnly(t *testing.T) {
	env := newAdminAPIHarness(t)
	const name = "flowcraft-chat-assistant"
	all := collectAdminPages(t, 200, func(cursor *string, limit int32) ([]apitypes.Workflow, bool, *string) {
		response, listErr := env.api.ListWorkflowsWithResponse(env.ctx, &adminhttp.ListWorkflowsParams{Cursor: cursor, Limit: &limit})
		if listErr != nil || response.JSON200 == nil {
			t.Fatalf("list workflows: response=%#v err=%v", response, listErr)
		}
		return response.JSON200.Items, response.JSON200.HasNext, response.JSON200.NextCursor
	})
	seed := requireName(t, all, name, func(item apitypes.Workflow) string { return item.Name })
	workflow, err := env.api.GetWorkflowWithResponse(env.ctx, seed.Id)
	if err != nil {
		t.Fatalf("get workflow: %v", err)
	}
	requireStatusOK(t, workflow, workflow.Body)
	if workflow.JSON200 == nil || workflow.JSON200.Name != name || workflow.JSON200.Spec.Driver != apitypes.WorkflowDriverFlowcraft {
		t.Fatalf("workflow = %#v", workflow.JSON200)
	}
}
