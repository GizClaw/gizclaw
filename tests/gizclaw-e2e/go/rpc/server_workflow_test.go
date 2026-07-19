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
			Source: rpcapi.ResourceSourceRuntime,
			Cursor: cursor,
			Limit:  &limit,
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
	if found == nil || found.Spec.Driver != rpcapi.WorkflowDriverFlowcraft || found.OwnerPublicKey != nil {
		t.Fatalf("runtime Workflow alias = %#v", found)
	}
	got, err := env.peer.GetWorkflow(env.ctx, "workflow.get.runtime", rpcapi.WorkflowGetRequest{
		Name:   "shared",
		Source: rpcapi.ResourceSourceRuntime,
	})
	if err != nil {
		t.Fatalf("workflow.get runtime alias: %v", err)
	}
	if got.Name != "shared" || got.Spec.Driver != rpcapi.WorkflowDriverFlowcraft {
		t.Fatalf("workflow.get runtime alias = %#v", got)
	}
	if _, err := env.peer.GetWorkflow(env.ctx, "workflow.get.runtime.concrete", rpcapi.WorkflowGetRequest{
		Name:   sharedWorkflow,
		Source: rpcapi.ResourceSourceRuntime,
	}); err == nil {
		t.Fatal("runtime Workflow get accepted a concrete resource name")
	}
	assertWorkflowPagination(t, env.ctx, env.peer, "shared", "chatroom", "mutation")
}

func TestServerWorkflowOwnedCRUD(t *testing.T) {
	env := newServerResourceHarness(t)
	const name = "owned-rpc-workflow"
	_, _ = env.peer.DeleteWorkflow(env.ctx, "workflow.delete.preclean", rpcapi.WorkflowDeleteRequest{
		Name: name, Source: rpcapi.ResourceSourceOwned,
	})

	created, err := env.peer.CreateWorkflow(env.ctx, "workflow.create.owned", rpcapi.WorkflowCreateRequest{
		Source: rpcapi.ResourceSourceOwned,
		Body: rpcapi.WorkflowUpsert{
			Name: name,
			Spec: rpcapi.WorkflowSpec{Driver: rpcapi.WorkflowDriverFlowcraft, Flowcraft: &rpcapi.FlowcraftWorkflowSpec{}},
		},
	})
	if err != nil {
		t.Fatalf("workflow.create owned: %v", err)
	}
	if created.Name != name || created.OwnerPublicKey == nil || *created.OwnerPublicKey != env.h.ContextPublicKey("peer-a") {
		t.Fatalf("workflow.create owned = %#v", created)
	}
	t.Cleanup(func() {
		_, _ = env.peer.DeleteWorkflow(env.ctx, "workflow.delete.cleanup", rpcapi.WorkflowDeleteRequest{Name: name, Source: rpcapi.ResourceSourceOwned})
	})

	owned, err := env.peer.GetWorkflow(env.ctx, "workflow.get.owned", rpcapi.WorkflowGetRequest{Name: name, Source: rpcapi.ResourceSourceOwned})
	if err != nil || owned.Name != name {
		t.Fatalf("workflow.get owned = %#v, %v", owned, err)
	}
	list, err := env.peer.ListWorkflows(env.ctx, "workflow.list.owned", rpcapi.WorkflowListRequest{Source: rpcapi.ResourceSourceOwned})
	if err != nil {
		t.Fatalf("workflow.list owned: %v", err)
	}
	found := false
	for _, item := range list.Items {
		found = found || item.Name == name
	}
	if !found {
		t.Fatalf("workflow.list owned = %#v", list.Items)
	}

	denied := env.h.ConnectClientFromContext("peer-denied")
	defer denied.Close()
	if _, err := denied.GetWorkflow(env.ctx, "workflow.get.owned.denied", rpcapi.WorkflowGetRequest{Name: name, Source: rpcapi.ResourceSourceOwned}); err == nil {
		t.Fatal("non-owner accessed owned Workflow")
	}
	if _, err := env.peer.CreateWorkflow(env.ctx, "workflow.create.runtime.denied", rpcapi.WorkflowCreateRequest{
		Source: rpcapi.ResourceSourceRuntime,
		Body:   rpcapi.WorkflowUpsert{Name: "invalid-runtime", Spec: rpcapi.WorkflowSpec{Driver: rpcapi.WorkflowDriverFlowcraft, Flowcraft: &rpcapi.FlowcraftWorkflowSpec{}}},
	}); err == nil {
		t.Fatal("workflow.create accepted runtime source")
	}

	updated, err := env.peer.PutWorkflow(env.ctx, "workflow.put.owned", rpcapi.WorkflowPutRequest{
		Name:   name,
		Source: rpcapi.ResourceSourceOwned,
		Body:   rpcapi.WorkflowUpsert{Name: name, Spec: rpcapi.WorkflowSpec{Driver: rpcapi.WorkflowDriverChatroom, Chatroom: &rpcapi.ChatRoomWorkflowSpec{}}},
	})
	if err != nil || updated.OwnerPublicKey == nil || *updated.OwnerPublicKey != env.h.ContextPublicKey("peer-a") {
		t.Fatalf("workflow.put owned = %#v, %v", updated, err)
	}
	if _, err := env.peer.DeleteWorkflow(env.ctx, "workflow.delete.owned", rpcapi.WorkflowDeleteRequest{Name: name, Source: rpcapi.ResourceSourceOwned}); err != nil {
		t.Fatalf("workflow.delete owned: %v", err)
	}
}
