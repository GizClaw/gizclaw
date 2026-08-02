//go:build gizclaw_e2e

package rpc_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestServerWorkflowRuntimeAliases(t *testing.T) {
	env := newServerResourceHarness(t)
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
			if list.Items[i].Name == "shared" {
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
		Name: "shared",
	})
	if err != nil {
		t.Fatalf("workflow.get runtime alias: %v", err)
	}
	if got.Value.Name != "shared" || got.Value.Driver != rpcapi.WorkflowDriverFlowcraft {
		t.Fatalf("workflow.get runtime alias = %#v", got)
	}
	if _, err := env.peer.GetWorkflow(env.ctx, "workflow.get.runtime.concrete", rpcapi.WorkflowGetRequest{
		Name: sharedWorkflow,
	}); err == nil {
		t.Fatal("runtime Workflow get accepted a concrete resource name")
	}
	if _, err := env.peer.GetWorkflow(env.ctx, "workflow.get.runtime.missing", rpcapi.WorkflowGetRequest{
		Name: "mutation",
	}); err == nil {
		t.Fatal("runtime Workflow get resolved an alias that is not in the profile")
	}
	// The mutation alias is added only by the Workspace mutation test after its
	// concrete Workflow exists.
	assertWorkflowPagination(t, env.ctx, env.peer, "shared", "chatroom")
}
