package peerresource

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// Workspace deletion only marks the record pending until the asynchronous
// cleanup finalizes it. The Workspace listing must skip that record instead of
// failing, so the caller keeps seeing the collection's remaining Workspaces
// while the deletion is still in flight.
func TestWorkspaceListSkipsWorkspacePendingDeletion(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	workflows := &workflow.Server{Store: store}
	createWorkflowForCollectionTest(t, ctx, workflows, "canonical-workflow")
	profile := runtimeProfileWithWorkspaceAlias("r1")
	server := &Server{
		Caller:     giznet.PublicKey{1},
		Workspaces: &workspace.Server{Store: store, Workflows: workflows},
		Workflows:  workflows,
		RuntimeProfile: func() *apitypes.RuntimeProfile {
			return &profile
		},
	}
	for _, name := range []string{"journey-1", "journey-2"} {
		callWorkspaceCreate(t, ctx, server, rpcapi.WorkspaceCreateBody{
			Name: name, Collection: "story-teller", WorkflowName: "journey",
		})
	}

	var payload rpcapi.RPCPayload
	if err := payload.FromWorkspaceDeleteRequest(rpcapi.WorkspaceDeleteRequest{Name: "journey-1"}); err != nil {
		t.Fatal(err)
	}
	response, handled, err := server.Dispatch(ctx, &rpcapi.RPCRequest{
		Id: "delete", Method: rpcapi.RPCMethodServerWorkspaceDelete, Params: &payload,
	})
	if err != nil || !handled || response.Error != nil || response.Result == nil {
		t.Fatalf("workspace delete response = %#v, handled=%v error=%v", response, handled, err)
	}

	listed := callWorkspaceList(t, ctx, server, "story-teller")
	if len(listed.Items) != 1 || listed.Items[0].Name != "journey-2" {
		t.Fatalf("workspace list while deletion is pending = %#v, want only journey-2", listed)
	}
}

// A shared SFU Workspace reaches the listing through the Friend relationship
// instead of the owner index, and its retirement marks the record pending
// deletion. That Workspace must drop out of the listing rather than fail the
// caller's own Collection.
func TestWorkspaceListSkipsSharedWorkspacePendingDeletion(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	workspaceStore := kv.Prefixed(store, kv.Key{"workspaces"})
	workflows := &workflow.Server{Store: kv.Prefixed(store, kv.Key{"workflows"})}
	createWorkflowForCollectionTest(t, ctx, workflows, "canonical-workflow")
	// The shared Workspace binds the built-in system-sfu Workflow, which every
	// Server materializes at startup.
	if err := workflows.EnsureBuiltinWorkflows(ctx); err != nil {
		t.Fatalf("materialize built-in Workflows: %v", err)
	}
	workspaces := &workspace.Server{Store: workspaceStore, Workflows: workflows}
	friendStore := kv.NewMemory(nil)
	t.Cleanup(func() { _ = friendStore.Close() })
	friends := &friend.Server{
		Friends:    friendStore,
		Workspaces: workspaces,
		SFUURL:     "wss://sfu.test",
	}
	caller := giznet.PublicKey{1}
	peer := giznet.PublicKey{2}
	relation, err := friends.AdminCreateFriend(ctx, caller.String(), peer.String())
	if err != nil {
		t.Fatalf("create Friend relationship: %v", err)
	}
	sharedName := socialutil.StringValue(relation.WorkspaceName)
	shared, err := workspaces.GetWorkspaceByName(ownership.WithOwner(ctx, caller.String()), sharedName)
	if err != nil {
		t.Fatalf("resolve shared Workspace %q: %v", sharedName, err)
	}

	profile := runtimeProfileWithWorkspaceAlias("r1")
	server := &Server{
		Caller:     caller,
		Workspaces: workspaces,
		Workflows:  workflows,
		Friends:    friends,
		RuntimeProfile: func() *apitypes.RuntimeProfile {
			return &profile
		},
	}
	callWorkspaceCreate(t, ctx, server, rpcapi.WorkspaceCreateBody{
		Name: "journey-1", Collection: "story-teller", WorkflowName: "journey",
	})
	if listed := callWorkspaceList(t, ctx, server, "story-teller"); len(listed.Items) != 1 || listed.Items[0].Name != "journey-1" {
		t.Fatalf("workspace list before retirement = %#v", listed)
	}

	if _, err := workspaces.RetireSystemWorkspaceByID(
		ctx, shared.Id, socialutil.SFUWorkspaceKindFriend, socialutil.RelationID(caller.String(), peer.String()),
	); err != nil {
		t.Fatalf("retire shared Workspace: %v", err)
	}
	if pending, err := pendingdeletion.HasLocator(ctx, workspaceStore, pendingdeletion.KindWorkspace, shared.Id); err != nil || !pending {
		t.Fatalf("shared Workspace pending deletion = %v, error = %v", pending, err)
	}

	listed := callWorkspaceList(t, ctx, server, "story-teller")
	if len(listed.Items) != 1 || listed.Items[0].Name != "journey-1" {
		t.Fatalf("workspace list while shared deletion is pending = %#v, want only journey-1", listed)
	}
}
