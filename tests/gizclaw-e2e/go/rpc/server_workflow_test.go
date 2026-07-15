//go:build gizclaw_e2e

package rpc_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestServerWorkflowRPC(t *testing.T) {
	env := newServerResourceHarness(t)

	workflowList, err := env.peer.ListWorkflows(env.ctx, "workflow.list.shared", rpcapi.WorkflowListRequest{Lang: rpcapi.WorkflowLocaleZhCN})
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
	if sharedFlow.Name != sharedWorkflow {
		t.Fatalf("workflow.get shared name = %q", sharedFlow.Name)
	}
	assertWorkflowPagination(t, env.ctx, env.peer, sharedWorkflow)
}
