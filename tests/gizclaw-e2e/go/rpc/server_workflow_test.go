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
	var localized *rpcapi.Workflow
	for i := range workflowList.Items {
		if workflowList.Items[i].Name == sharedWorkflow {
			localized = &workflowList.Items[i]
			break
		}
	}
	if localized == nil || localized.I18n == nil || localized.I18n.Name == nil || *localized.I18n.Name != "支持助手" {
		t.Fatalf("zh-CN workflow catalog = %#v", localized)
	}
	sharedFlow, err := env.peer.GetWorkflow(env.ctx, "workflow.get.shared", rpcapi.WorkflowGetRequest{Name: sharedWorkflow, Lang: rpcapi.WorkflowLocaleEn})
	if err != nil {
		t.Fatalf("workflow.get shared: %v", err)
	}
	if sharedFlow.Name != sharedWorkflow {
		t.Fatalf("workflow.get shared name = %q", sharedFlow.Name)
	}
	if sharedFlow.I18n == nil || sharedFlow.I18n.Name == nil || *sharedFlow.I18n.Name != "Support Assistant" {
		t.Fatalf("en workflow catalog = %#v", sharedFlow.I18n)
	}
	assertWorkflowPagination(t, env.ctx, env.peer, sharedWorkflow)
}
