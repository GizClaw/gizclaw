package peerresource

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestValidateRunWorkspaceSelectionResolvesSharedFriendWorkspaceByPeerName(t *testing.T) {
	ctx := t.Context()
	caller := giznet.PublicKey{1}
	workspaceOwner := giznet.PublicKey{2}
	unrelated := giznet.PublicKey{3}
	workflowID := "workflow-direct"
	workspaces := &sharedWorkspaceNameService{owner: workspaceOwner.String()}

	friendStore := kv.NewMemory(nil)
	t.Cleanup(func() { _ = friendStore.Close() })
	friends := &friend.Server{
		Friends:    friendStore,
		Workspaces: workspaces,
		RuntimeProfileForOwner: func(context.Context, string) (apitypes.RuntimeProfile, error) {
			return apitypes.RuntimeProfile{Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{
				System: apitypes.RuntimeProfileSystemWorkflows{FriendChatroom: workflowID},
			}}}, nil
		},
	}
	relation, err := friends.AdminCreateFriend(ctx, workspaceOwner.String(), caller.String())
	if err != nil {
		t.Fatalf("create Friend relationship: %v", err)
	}
	workspaceName := socialutil.StringValue(relation.WorkspaceName)
	decoyOwner := unrelated.String()
	workspaces.decoy = apitypes.Workspace{
		Id: "workspace-decoy", Name: workspaceName, OwnerPublicKey: &decoyOwner,
		System: workspaces.item.System, WorkflowId: workflowID, Parameters: workspaces.item.Parameters,
	}

	profile := apitypes.RuntimeProfile{Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{
		System: apitypes.RuntimeProfileSystemWorkflows{FriendChatroom: workflowID},
	}}}
	server := &Server{
		Caller:         caller,
		Workspaces:     workspaces,
		Friends:        friends,
		RuntimeProfile: func() *apitypes.RuntimeProfile { return &profile },
	}
	if got, rpcErr := server.ValidateRunWorkspaceSelection(ctx, workspaceName); rpcErr != nil || got != workspaceName {
		t.Fatalf("ValidateRunWorkspaceSelection() = %q, %#v", got, rpcErr)
	}
	resolved, rpcErr := server.ResolveRunWorkspaceSelection(ctx, workspaceName)
	if rpcErr != nil || resolved.Id != workspaces.item.Id {
		t.Fatalf("ResolveRunWorkspaceSelection() = %#v, %#v; want %q", resolved, rpcErr, workspaces.item.Id)
	}

	server.Caller = unrelated
	if _, rpcErr := server.ValidateRunWorkspaceSelection(ctx, workspaceName); rpcErr == nil || rpcErr.Code != rpcapi.StatusCodeNotFound {
		t.Fatalf("unrelated ValidateRunWorkspaceSelection() error = %#v, want NOT_FOUND", rpcErr)
	}
}

type sharedWorkspaceNameService struct {
	owner string
	item  apitypes.Workspace
	decoy apitypes.Workspace
}

func (s *sharedWorkspaceNameService) ListWorkspaces(context.Context, adminhttp.ListWorkspacesRequestObject) (adminhttp.ListWorkspacesResponseObject, error) {
	return adminhttp.ListWorkspaces200JSONResponse(adminhttp.WorkspaceList{Items: []apitypes.Workspace{s.decoy, s.item}}), nil
}

func (*sharedWorkspaceNameService) DeleteWorkspace(context.Context, adminhttp.DeleteWorkspaceRequestObject) (adminhttp.DeleteWorkspaceResponseObject, error) {
	return nil, errors.New("unexpected DeleteWorkspace")
}

func (*sharedWorkspaceNameService) GetWorkspace(context.Context, adminhttp.GetWorkspaceRequestObject) (adminhttp.GetWorkspaceResponseObject, error) {
	return nil, errors.New("unexpected GetWorkspace")
}

func (*sharedWorkspaceNameService) PutWorkspace(context.Context, adminhttp.PutWorkspaceRequestObject) (adminhttp.PutWorkspaceResponseObject, error) {
	return nil, errors.New("unexpected PutWorkspace")
}

func (s *sharedWorkspaceNameService) CreateSystemWorkspace(ctx context.Context, body adminhttp.WorkspaceUpsert) (apitypes.Workspace, bool, error) {
	system := true
	owner, _ := ownership.FromContext(ctx)
	s.item = apitypes.Workspace{
		Id: "workspace-id", Name: body.Name, OwnerPublicKey: &owner,
		System: &system, WorkflowId: body.WorkflowId, Parameters: body.Parameters,
	}
	return s.item, true, nil
}

func (s *sharedWorkspaceNameService) DeleteSystemWorkspace(context.Context, string) (apitypes.Workspace, error) {
	return s.item, nil
}

func (s *sharedWorkspaceNameService) RetireSystemWorkspaceByID(context.Context, string, apitypes.ChatRoomMode, string) (apitypes.Workspace, error) {
	return s.item, nil
}

func (s *sharedWorkspaceNameService) GetWorkspaceByName(ctx context.Context, name string) (apitypes.Workspace, error) {
	owner, ok := ownership.FromContext(ctx)
	if !ok || owner != s.owner || name != s.item.Name {
		return apitypes.Workspace{}, kv.ErrNotFound
	}
	return s.item, nil
}
