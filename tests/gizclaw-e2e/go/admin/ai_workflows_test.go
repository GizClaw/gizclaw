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
	requireName(t, all, "flowcraft-chat-assistant", func(item apitypes.Workflow) string { return item.Name })
	requirePrefixCount(t, all, "flowcraft-scenario-", 100, func(item apitypes.Workflow) string { return item.Name })

	get, err := env.api.GetWorkflowWithResponse(env.ctx, "flowcraft-chat-assistant")
	if err != nil {
		t.Fatalf("get workflow: %v", err)
	}
	requireStatusOK(t, get, get.Body)
	if get.JSON200 == nil || get.JSON200.Name != "flowcraft-chat-assistant" || get.JSON200.Spec.Driver != apitypes.WorkflowDriverFlowcraft {
		t.Fatalf("get workflow = %#v", get.JSON200)
	}

	name := mutationName("workflow")
	_, _ = env.api.DeleteWorkflowWithResponse(env.ctx, name)
	created, err := env.api.CreateWorkflowWithResponse(env.ctx, apitypes.Workflow{
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
	deleted, err := env.api.DeleteWorkflowWithResponse(env.ctx, name)
	if err != nil {
		t.Fatalf("delete workflow: %v", err)
	}
	requireStatusOK(t, deleted, deleted.Body)
}

func TestAdminAPIWorkflowHasExecutionDefinitionOnly(t *testing.T) {
	env := newAdminAPIHarness(t)
	const name = "flowcraft-chat-assistant"
	workflow, err := env.api.GetWorkflowWithResponse(env.ctx, name)
	if err != nil {
		t.Fatalf("get workflow: %v", err)
	}
	requireStatusOK(t, workflow, workflow.Body)
	if workflow.JSON200 == nil || workflow.JSON200.Name != name || workflow.JSON200.Spec.Driver != apitypes.WorkflowDriverFlowcraft {
		t.Fatalf("workflow = %#v", workflow.JSON200)
	}
}
