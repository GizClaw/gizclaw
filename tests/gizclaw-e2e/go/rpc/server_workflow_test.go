//go:build gizclaw_e2e

package rpc_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestServerWorkflowRPC(t *testing.T) {
	env := newServerResourceHarness(t)

	workflowList, err := env.peer.ListWorkflows(env.ctx, "workflow.list.shared", rpcapi.WorkflowListRequest{})
	if err != nil {
		t.Fatalf("workflow.list shared: %v", err)
	}
	if len(workflowList.Items) == 0 {
		t.Fatalf("workflow.list returned no items")
	}
	sharedFlow, err := env.peer.GetWorkflow(env.ctx, "workflow.get.shared", rpcapi.WorkflowGetRequest{Name: sharedWorkflow})
	if err != nil {
		t.Fatalf("workflow.get shared: %v", err)
	}
	if sharedFlow.Metadata.Name != sharedWorkflow {
		t.Fatalf("workflow.get shared name = %q", sharedFlow.Metadata.Name)
	}

	_, _ = env.peer.DeleteWorkflow(env.ctx, "workflow.delete.preclean", rpcapi.WorkflowDeleteRequest{Name: mutationWorkflow})
	createdFlow, err := env.peer.CreateWorkflow(env.ctx, "workflow.create", rpcWorkflow(mutationWorkflow, "created by peer rpc"))
	if err != nil {
		t.Fatalf("workflow.create: %v", err)
	}
	if createdFlow.Metadata.Name != mutationWorkflow {
		t.Fatalf("workflow.create name = %q", createdFlow.Metadata.Name)
	}
	updatedFlowDoc := rpcWorkflow(mutationWorkflow, "updated by peer rpc")
	updatedFlow, err := env.peer.PutWorkflow(env.ctx, "workflow.put", rpcapi.WorkflowPutRequest{Name: mutationWorkflow, Body: updatedFlowDoc})
	if err != nil {
		t.Fatalf("workflow.put: %v", err)
	}
	catalog := updatedFlow.I18n.Value["en"]
	if catalog.Description == nil || *catalog.Description != "updated by peer rpc" {
		t.Fatalf("workflow.put i18n = %#v", updatedFlow.I18n)
	}
	assertWorkflowPagination(t, env.ctx, env.peer, sharedWorkflow, mutationWorkflow)
	if _, err := env.peer.DeleteWorkflow(env.ctx, "workflow.delete", rpcapi.WorkflowDeleteRequest{Name: mutationWorkflow}); err != nil {
		t.Fatalf("workflow.delete: %v", err)
	}
}
