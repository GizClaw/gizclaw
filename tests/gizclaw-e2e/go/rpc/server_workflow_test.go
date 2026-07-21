//go:build gizclaw_e2e

package rpc_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestServerWorkflowRuntimeAliases(t *testing.T) {
	env := newServerResourceHarness(t)
	admin := serverResourceAdminClient(t, env)
	_, _ = admin.DeleteWorkflowWithResponse(env.ctx, mutationWorkflow)

	limit := 1
	var cursor *string
	var found *rpcapi.Workflow
	for page := 0; page < 100 && found == nil; page++ {
		list, err := env.peer.ListWorkflows(env.ctx, "workflow.list.runtime", rpcapi.WorkflowListRequest{
			Collection: "assistants",
			Cursor:     cursor,
			Limit:      &limit,
		})
		if err != nil {
			t.Fatalf("workflow.list runtime page %d: %v", page, err)
		}
		for i := range list.Items {
			if list.Items[i].Alias == "shared" {
				found = &list.Items[i]
				break
			}
		}
		if found != nil || !list.HasNext {
			break
		}
		if list.NextCursor == nil || *list.NextCursor == "" {
			t.Fatalf("workflow.list runtime page %d has_next without cursor", page)
		}
		cursor = list.NextCursor
	}
	if found == nil || found.Driver != rpcapi.WorkflowDriverFlowcraft || found.Collection != "assistants" {
		t.Fatalf("runtime Workflow alias = %#v", found)
	}
	got, err := env.peer.GetWorkflow(env.ctx, "workflow.get.runtime", rpcapi.WorkflowGetRequest{
		Alias: "shared",
	})
	if err != nil {
		t.Fatalf("workflow.get runtime alias: %v", err)
	}
	if got.Value.Alias != "shared" || got.Value.Driver != rpcapi.WorkflowDriverFlowcraft {
		t.Fatalf("workflow.get runtime alias = %#v", got)
	}
	if _, err := env.peer.GetWorkflow(env.ctx, "workflow.get.runtime.concrete", rpcapi.WorkflowGetRequest{
		Alias: sharedWorkflow,
	}); err == nil {
		t.Fatal("runtime Workflow get accepted a concrete resource name")
	}
	if _, err := env.peer.GetWorkflow(env.ctx, "workflow.get.runtime.missing", rpcapi.WorkflowGetRequest{
		Alias: "mutation",
	}); err == nil {
		t.Fatal("runtime Workflow get resolved an alias whose target is missing")
	}
	// The mutation target is intentionally absent until the Workspace mutation
	// test creates it. RuntimeProfile references that resolve to 404 are ignored.
	assertWorkflowPagination(t, env.ctx, env.peer, "shared", "chatroom")
}
