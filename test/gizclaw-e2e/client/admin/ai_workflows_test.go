//go:build gizclaw_e2e

package admin_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func TestAdminAPIWorkflowsListGetPaginationAndMutation(t *testing.T) {
	env := newAdminAPIHarness(t)

	all := collectAdminPages(t, 25, func(cursor *string, limit int32) ([]apitypes.WorkflowDocument, bool, *string) {
		resp, err := env.api.ListWorkflowsWithResponse(env.ctx, &adminservice.ListWorkflowsParams{Cursor: cursor, Limit: &limit})
		if err != nil {
			t.Fatalf("list workflows: %v", err)
		}
		requireStatusOK(t, resp, resp.Body)
		if resp.JSON200 == nil {
			t.Fatalf("list workflows missing JSON200")
		}
		return resp.JSON200.Items, resp.JSON200.HasNext, resp.JSON200.NextCursor
	})
	requireName(t, all, "flowcraft-support", func(item apitypes.WorkflowDocument) string { return item.Metadata.Name })
	requirePrefixCount(t, all, "flowcraft-scenario-", 100, func(item apitypes.WorkflowDocument) string { return item.Metadata.Name })

	get, err := env.api.GetWorkflowWithResponse(env.ctx, "flowcraft-support")
	if err != nil {
		t.Fatalf("get workflow: %v", err)
	}
	requireStatusOK(t, get, get.Body)
	if get.JSON200 == nil || get.JSON200.Metadata.Name != "flowcraft-support" || get.JSON200.Spec.Driver != apitypes.WorkflowDriverFlowcraft {
		t.Fatalf("get workflow = %#v", get.JSON200)
	}

	name := mutationName("workflow")
	_, _ = env.api.DeleteWorkflowWithResponse(env.ctx, name)
	created, err := env.api.CreateWorkflowWithResponse(env.ctx, apitypes.WorkflowDocument{
		Metadata: apitypes.WorkflowMetadata{Name: name, Description: ptr("admin API mutation workflow")},
		Spec:     apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverFlowcraft, Flowcraft: &apitypes.FlowcraftWorkflowSpec{}},
	})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	requireStatusOK(t, created, created.Body)
	if created.JSON200 == nil || created.JSON200.Metadata.Name != name {
		t.Fatalf("created workflow = %#v", created.JSON200)
	}
	deleted, err := env.api.DeleteWorkflowWithResponse(env.ctx, name)
	if err != nil {
		t.Fatalf("delete workflow: %v", err)
	}
	requireStatusOK(t, deleted, deleted.Body)
}
