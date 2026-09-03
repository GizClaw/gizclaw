package peerresource

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
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

// A shared Chatroom Workspace reaches the listing through the Friend
// relationship instead of the owner index, and its retirement marks the record
// pending deletion. That Workspace must drop out of the listing rather than
// fail the caller's own Collection.
func TestWorkspaceListSkipsSharedWorkspacePendingDeletion(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	workspaceStore := kv.Prefixed(store, kv.Key{"workspaces"})
	workflows := &workflow.Server{Store: kv.Prefixed(store, kv.Key{"workflows"})}
	createWorkflowForCollectionTest(t, ctx, workflows, "canonical-workflow")
	if _, err := workflows.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{
		Body: &adminhttp.WorkflowUpsert{Id: "chatroom-workflow", Spec: apitypes.WorkflowSpec{
			Driver:   apitypes.WorkflowDriverChatroom,
			Chatroom: &apitypes.ChatRoomWorkflowSpec{History: apitypes.ChatRoomWorkflowHistorySpec{}},
		}},
	}); err != nil {
		t.Fatalf("create Chatroom Workflow: %v", err)
	}
	workspaces := &workspace.Server{Store: workspaceStore, Workflows: workflows}
	friendStore := kv.NewMemory(nil)
	t.Cleanup(func() { _ = friendStore.Close() })
	friends := &friend.Server{
		Friends:    friendStore,
		Workspaces: workspaces,
		RuntimeProfileForOwner: func(context.Context, string) (apitypes.RuntimeProfile, error) {
			return apitypes.RuntimeProfile{Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{
				System: apitypes.RuntimeProfileSystemWorkflows{FriendChatroom: "chatroom-workflow"},
			}}}, nil
		},
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
	profile.Spec.Workflows.System.FriendChatroom = "chatroom-workflow"
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
		ctx, shared.Id, apitypes.ChatRoomModeDirect, socialutil.RelationID(caller.String(), peer.String()),
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

// pendingDeletionWorkspaceService answers every name lookup with the
// pending-deletion sentinel, so the single-Workspace fences can be exercised
// without standing up a store and a background deletion.
type pendingDeletionWorkspaceService struct{}

func (pendingDeletionWorkspaceService) GetWorkspaceByName(context.Context, string) (apitypes.Workspace, error) {
	return apitypes.Workspace{}, workspace.ErrWorkspacePendingDeletion
}

func (pendingDeletionWorkspaceService) ListWorkspaces(context.Context, adminhttp.ListWorkspacesRequestObject) (adminhttp.ListWorkspacesResponseObject, error) {
	return nil, errors.New("unexpected ListWorkspaces")
}

func (pendingDeletionWorkspaceService) DeleteWorkspace(context.Context, adminhttp.DeleteWorkspaceRequestObject) (adminhttp.DeleteWorkspaceResponseObject, error) {
	return nil, errors.New("unexpected DeleteWorkspace")
}

func (pendingDeletionWorkspaceService) GetWorkspace(context.Context, adminhttp.GetWorkspaceRequestObject) (adminhttp.GetWorkspaceResponseObject, error) {
	return nil, errors.New("unexpected GetWorkspace")
}

func (pendingDeletionWorkspaceService) PutWorkspace(context.Context, adminhttp.PutWorkspaceRequestObject) (adminhttp.PutWorkspaceResponseObject, error) {
	return nil, errors.New("unexpected PutWorkspace")
}

// Addressing one Workspace by name is a fence, not an enumeration: the caller
// asked for that Workspace specifically, so a pending deletion is reported
// rather than skipped. It used to fall through to INTERNAL, which left the
// caller unable to tell "being deleted" from "the server broke" and made
// polling for the deletion to finish impossible.
func TestResolveAccessibleWorkspaceReportsPendingDeletion(t *testing.T) {
	server := &Server{
		Workspaces: pendingDeletionWorkspaceService{},
		Caller:     giznet.PublicKey{1},
	}
	_, status := server.ResolveAccessibleWorkspace(t.Context(), "going-away")
	if status == nil {
		t.Fatal("ResolveAccessibleWorkspace accepted a Workspace pending deletion")
	}
	if status.Code != rpcapi.StatusCodeFailedPrecondition {
		t.Fatalf("code = %s, want %s", status.Code, rpcapi.StatusCodeFailedPrecondition)
	}
	if status.Reason != "WORKSPACE_PENDING_DELETION" {
		t.Fatalf("reason = %q, want WORKSPACE_PENDING_DELETION", status.Reason)
	}
}

// server.workspace.get resolves the Workspace on its own path, so it needs the
// same classification. It reported an internal error until this was shared.
func TestWorkspaceGetReportsPendingDeletion(t *testing.T) {
	server := &Server{
		Workspaces: pendingDeletionWorkspaceService{},
		Caller:     giznet.PublicKey{1},
	}
	request := &rpcapi.RPCRequest{V: rpcapi.RPCVersionV1, Id: "get-1", Method: rpcapi.RPCMethodServerWorkspaceGet}
	params := &rpcapi.RPCPayload{}
	if err := params.FromWorkspaceGetRequest(rpcapi.WorkspaceGetRequest{Name: "going-away"}); err != nil {
		t.Fatalf("encode params: %v", err)
	}
	request.Params = params
	response := server.handleWorkspaceGet(t.Context(), request)
	if response.Error == nil {
		t.Fatalf("workspace get response = %#v, want a failure", response)
	}
	if response.Error.Code != rpcapi.StatusCodeFailedPrecondition || response.Error.Reason != "WORKSPACE_PENDING_DELETION" {
		t.Fatalf("workspace get error = %#v", response.Error)
	}
}
