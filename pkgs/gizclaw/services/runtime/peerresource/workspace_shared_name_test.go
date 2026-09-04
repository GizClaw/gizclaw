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
	workflowID := socialutil.SFUWorkflowID
	workspaces := &sharedWorkspaceNameService{owner: workspaceOwner.String()}

	friendStore := kv.NewMemory(nil)
	t.Cleanup(func() { _ = friendStore.Close() })
	friends := &friend.Server{
		Friends:    friendStore,
		Workspaces: workspaces,
		SFUURL:     "wss://sfu.test",
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

	profile := apitypes.RuntimeProfile{}
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

func TestResolveRunWorkspaceSelectionMaterializesSharedSocialWorkspace(t *testing.T) {
	ctx := t.Context()
	caller := giznet.PublicKey{1}
	workspaceOwner := giznet.PublicKey{2}
	stranger := giznet.PublicKey{3}
	friendStore := kv.NewMemory(nil)
	t.Cleanup(func() { _ = friendStore.Close() })
	homeWorkspaces := &sharedWorkspaceNameService{owner: workspaceOwner.String()}
	friends := &friend.Server{Friends: friendStore, Workspaces: homeWorkspaces, SFUURL: "wss://sfu.test"}
	relation, err := friends.AdminCreateFriend(ctx, workspaceOwner.String(), caller.String())
	if err != nil {
		t.Fatalf("create Friend relationship: %v", err)
	}
	workspaceName := socialutil.StringValue(relation.WorkspaceName)

	// The caller's Server shares the Social KV but has an empty local catalog.
	localWorkspaces := &sharedWorkspaceNameService{owner: workspaceOwner.String()}
	profile := apitypes.RuntimeProfile{}
	server := &Server{
		Caller:         caller,
		Workspaces:     localWorkspaces,
		Friends:        &friend.Server{Friends: friendStore, Workspaces: localWorkspaces, SFUURL: "wss://sfu.test"},
		RuntimeProfile: func() *apitypes.RuntimeProfile { return &profile },
	}
	resolved, rpcErr := server.ResolveRunWorkspaceSelection(ctx, workspaceName)
	if rpcErr != nil {
		t.Fatalf("ResolveRunWorkspaceSelection() error = %#v", rpcErr)
	}
	if resolved.Id != homeWorkspaces.item.Id || resolved.Name != workspaceName || resolved.WorkflowId != socialutil.SFUWorkflowID ||
		resolved.OwnerPublicKey == nil || *resolved.OwnerPublicKey != workspaceOwner.String() || localWorkspaces.created != 1 {
		t.Fatalf("materialized Workspace = %#v (created %d), want copy of %#v", resolved, localWorkspaces.created, homeWorkspaces.item)
	}
	if again, rpcErr := server.ResolveRunWorkspaceSelection(ctx, workspaceName); rpcErr != nil || again.Id != resolved.Id || localWorkspaces.created != 1 {
		t.Fatalf("second ResolveRunWorkspaceSelection() = %#v, %#v (created %d), want reuse", again, rpcErr, localWorkspaces.created)
	}
	items, err := server.effectiveWorkspaces(ctx)
	if err != nil || len(items) != 1 || items[0].Name != workspaceName {
		t.Fatalf("effectiveWorkspaces() = %#v, %v, want the shared Social Workspace", items, err)
	}

	server.Caller = stranger
	strangerWorkspaces := &sharedWorkspaceNameService{owner: workspaceOwner.String()}
	server.Workspaces = strangerWorkspaces
	if _, rpcErr := server.ResolveRunWorkspaceSelection(ctx, workspaceName); rpcErr == nil || rpcErr.Code != rpcapi.StatusCodeNotFound {
		t.Fatalf("stranger ResolveRunWorkspaceSelection() error = %#v, want NOT_FOUND", rpcErr)
	}
	if strangerWorkspaces.created != 0 {
		t.Fatal("stranger materialized a Social Workspace")
	}
}

type sharedWorkspaceNameService struct {
	owner   string
	item    apitypes.Workspace
	decoy   apitypes.Workspace
	created int
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
	if s.item.Name == body.Name {
		return s.item, false, nil
	}
	s.created++
	id := body.Id
	if id == "" {
		id = "workspace-id"
	}
	s.item = apitypes.Workspace{
		Id: id, Name: body.Name, OwnerPublicKey: &owner,
		System: &system, WorkflowId: body.WorkflowId, Parameters: body.Parameters,
	}
	return s.item, true, nil
}

func (s *sharedWorkspaceNameService) DeleteSystemWorkspace(context.Context, string) (apitypes.Workspace, error) {
	return s.item, nil
}

func (s *sharedWorkspaceNameService) RetireSystemWorkspaceByID(context.Context, string, socialutil.SFUWorkspaceKind, string) (apitypes.Workspace, error) {
	return s.item, nil
}

func (s *sharedWorkspaceNameService) GetWorkspaceByName(ctx context.Context, name string) (apitypes.Workspace, error) {
	owner, ok := ownership.FromContext(ctx)
	if !ok || owner != s.owner || name != s.item.Name {
		return apitypes.Workspace{}, kv.ErrNotFound
	}
	return s.item, nil
}
