package friend

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

func TestInviteTokenLifecycleAndAddFriend(t *testing.T) {
	ctx := context.Background()
	s := newTestServer()

	empty, err := s.GetFriendInviteToken(ctx, "peer-b", rpcapi.FriendInviteTokenGetRequest{})
	if err != nil {
		t.Fatalf("GetFriendInviteToken empty: %v", err)
	}
	if empty.InviteToken != nil || empty.ExpiresAt != nil {
		t.Fatalf("empty token response = %#v, want no token fields", empty)
	}

	created, err := s.CreateFriendInviteToken(ctx, "peer-b", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatalf("CreateFriendInviteToken: %v", err)
	}
	if created.InviteToken != "id-a" || !created.ExpiresAt.Equal(s.now().Add(socialutil.DefaultInviteTokenTTL)) {
		t.Fatalf("created token = %#v", created)
	}
	createdAgain, err := s.CreateFriendInviteToken(ctx, "peer-b", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatalf("CreateFriendInviteToken existing: %v", err)
	}
	if createdAgain.InviteToken != created.InviteToken || !createdAgain.ExpiresAt.Equal(created.ExpiresAt) {
		t.Fatalf("existing token = %#v, want %#v", createdAgain, created)
	}
	got, err := s.GetFriendInviteToken(ctx, "peer-b", rpcapi.FriendInviteTokenGetRequest{})
	if err != nil {
		t.Fatalf("GetFriendInviteToken: %v", err)
	}
	if got.InviteToken == nil || *got.InviteToken != created.InviteToken {
		t.Fatalf("got token = %#v, want %q", got, created.InviteToken)
	}

	if _, err := s.AddFriend(ctx, "peer-a", rpcapi.FriendAddRequest{InviteToken: "missing"}); err == nil {
		t.Fatal("AddFriend missing token error = nil")
	}
	if _, err := s.AddFriend(ctx, "peer-b", rpcapi.FriendAddRequest{InviteToken: created.InviteToken}); err == nil {
		t.Fatal("AddFriend self token error = nil")
	}

	friend, err := s.AddFriend(ctx, "peer-a", rpcapi.FriendAddRequest{InviteToken: created.InviteToken})
	if err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	if socialutil.StringValue(friend.PeerPublicKey) != "peer-b" {
		t.Fatalf("AddFriend peer_public_key = %q, want peer-b", socialutil.StringValue(friend.PeerPublicKey))
	}
	workspaceName := socialutil.StringValue(friend.WorkspaceName)
	if workspaceName == "" {
		t.Fatal("AddFriend workspace_name is empty")
	}
	duplicate, err := s.AddFriend(ctx, "peer-a", rpcapi.FriendAddRequest{InviteToken: created.InviteToken})
	if err != nil {
		t.Fatalf("AddFriend duplicate: %v", err)
	}
	if socialutil.StringValue(duplicate.Id) != socialutil.StringValue(friend.Id) {
		t.Fatalf("duplicate relation id = %q, want %q", socialutil.StringValue(duplicate.Id), socialutil.StringValue(friend.Id))
	}

	for _, peer := range []string{"peer-a", "peer-b"} {
		friends, err := s.ListFriends(ctx, peer, rpcapi.FriendListRequest{})
		if err != nil {
			t.Fatalf("ListFriends(%s): %v", peer, err)
		}
		if len(friends.Items) != 1 {
			t.Fatalf("ListFriends(%s) len = %d, want 1", peer, len(friends.Items))
		}
		if socialutil.StringValue(friends.Items[0].WorkspaceName) != workspaceName {
			t.Fatalf("ListFriends(%s) workspace_name = %#v, want %q", peer, friends.Items[0].WorkspaceName, workspaceName)
		}
	}
}

func TestInviteTokenExpiryAndClear(t *testing.T) {
	ctx := context.Background()
	s := newTestServer()
	created, err := s.CreateFriendInviteToken(ctx, "peer-b", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatalf("CreateFriendInviteToken: %v", err)
	}
	s.Now = func() time.Time { return time.Date(2026, 6, 13, 0, 6, 0, 0, time.UTC) }
	got, err := s.GetFriendInviteToken(ctx, "peer-b", rpcapi.FriendInviteTokenGetRequest{})
	if err != nil {
		t.Fatalf("GetFriendInviteToken expired: %v", err)
	}
	if got.InviteToken != nil || got.ExpiresAt != nil {
		t.Fatalf("expired token response = %#v, want no token fields", got)
	}
	if _, err := s.AddFriend(ctx, "peer-a", rpcapi.FriendAddRequest{InviteToken: created.InviteToken}); err == nil {
		t.Fatal("AddFriend expired token error = nil")
	}

	refreshed, err := s.CreateFriendInviteToken(ctx, "peer-b", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatalf("CreateFriendInviteToken refreshed: %v", err)
	}
	if refreshed.InviteToken == created.InviteToken {
		t.Fatalf("refreshed token reused expired token %q", refreshed.InviteToken)
	}
	if _, err := s.ClearFriendInviteToken(ctx, "peer-b", rpcapi.FriendInviteTokenClearRequest{}); err != nil {
		t.Fatalf("ClearFriendInviteToken: %v", err)
	}
	cleared, err := s.GetFriendInviteToken(ctx, "peer-b", rpcapi.FriendInviteTokenGetRequest{})
	if err != nil {
		t.Fatalf("GetFriendInviteToken cleared: %v", err)
	}
	if cleared.InviteToken != nil || cleared.ExpiresAt != nil {
		t.Fatalf("cleared token response = %#v, want no token fields", cleared)
	}
}

func TestAddAndDeleteMaintainChatWorkspace(t *testing.T) {
	ctx := context.Background()
	s := newTestServer()
	workspaces := &recordingWorkspaceService{}
	aclSvc := &recordingACL{}
	s.Workspaces = workspaces
	s.ACL = aclSvc

	token, err := s.CreateFriendInviteToken(ctx, "peer-b", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatalf("CreateFriendInviteToken: %v", err)
	}
	friend, err := s.AddFriend(ctx, "peer-a", rpcapi.FriendAddRequest{InviteToken: token.InviteToken})
	if err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	workspaceName := socialutil.StringValue(friend.WorkspaceName)
	if workspaceName == "" {
		t.Fatal("friend workspace_name is empty")
	}
	if len(workspaces.created) != 1 || workspaces.created[0].Name != workspaceName || workspaces.created[0].WorkflowName != socialutil.ChatRoomWorkflowName {
		t.Fatalf("created workspaces = %#v, want %q chatroom", workspaces.created, workspaceName)
	}
	if err := aclSvc.authorizeWorkspace(workspaceName, "peer-a"); err != nil {
		t.Fatalf("peer-a workspace authorize: %v", err)
	}
	if err := aclSvc.authorizeWorkspace(workspaceName, "peer-b"); err != nil {
		t.Fatalf("peer-b workspace authorize: %v", err)
	}

	if _, err := s.DeleteFriend(ctx, "peer-a", rpcapi.FriendDeleteRequest{Id: socialutil.RelationID("peer-a", "peer-b")}); err != nil {
		t.Fatalf("DeleteFriend: %v", err)
	}
	if len(workspaces.deleted) != 1 || workspaces.deleted[0] != workspaceName {
		t.Fatalf("deleted workspaces = %#v, want %q", workspaces.deleted, workspaceName)
	}
	if err := aclSvc.authorizeWorkspace(workspaceName, "peer-a"); !errors.Is(err, acl.ErrDenied) {
		t.Fatalf("peer-a workspace authorize after delete = %v, want denied", err)
	}
	if err := aclSvc.authorizeWorkspace(workspaceName, "peer-b"); !errors.Is(err, acl.ErrDenied) {
		t.Fatalf("peer-b workspace authorize after delete = %v, want denied", err)
	}
	peerBFriends, err := s.ListFriends(ctx, "peer-b", rpcapi.FriendListRequest{})
	if err != nil {
		t.Fatalf("ListFriends peer-b: %v", err)
	}
	if len(peerBFriends.Items) != 0 {
		t.Fatalf("peer-b friends after delete = %#v, want none", peerBFriends.Items)
	}
}

func TestAddFriendRollsBackWorkspaceWhenFriendRowsFail(t *testing.T) {
	ctx := context.Background()
	s := newTestServer()
	workspaces := &recordingWorkspaceService{}
	aclSvc := &recordingACL{}
	s.Workspaces = workspaces
	s.ACL = aclSvc
	token, err := s.CreateFriendInviteToken(ctx, "peer-b", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatalf("CreateFriendInviteToken: %v", err)
	}
	s.Friends = failingBatchSetStore{Store: kv.NewMemory(nil)}
	if _, err := s.AddFriend(ctx, "peer-a", rpcapi.FriendAddRequest{InviteToken: token.InviteToken}); err == nil {
		t.Fatal("AddFriend with failing friend store error = nil")
	}
	workspaceName := socialutil.DirectWorkspaceName(socialutil.RelationID("peer-a", "peer-b"))
	if len(workspaces.deleted) != 1 || workspaces.deleted[0] != workspaceName {
		t.Fatalf("deleted workspaces after rollback = %#v, want %q", workspaces.deleted, workspaceName)
	}
	if err := aclSvc.authorizeWorkspace(workspaceName, "peer-a"); !errors.Is(err, acl.ErrDenied) {
		t.Fatalf("peer-a workspace authorize after rollback = %v, want denied", err)
	}
}

func TestConfigurationAndValidationErrors(t *testing.T) {
	ctx := context.Background()
	empty := &Server{}
	if _, err := empty.CreateFriendInviteToken(ctx, "peer-a", rpcapi.FriendInviteTokenCreateRequest{}); err == nil {
		t.Fatal("CreateFriendInviteToken without store error = nil")
	}
	if _, err := empty.AddFriend(ctx, "peer-a", rpcapi.FriendAddRequest{InviteToken: "token"}); err == nil {
		t.Fatal("AddFriend without store error = nil")
	}
	if _, err := empty.ListFriends(ctx, "peer-a", rpcapi.FriendListRequest{}); err == nil {
		t.Fatal("ListFriends without store error = nil")
	}

	s := newTestServer()
	if _, err := s.CreateFriendInviteToken(ctx, "", rpcapi.FriendInviteTokenCreateRequest{}); err == nil {
		t.Fatal("CreateFriendInviteToken empty owner error = nil")
	}
	if _, err := s.ClearFriendInviteToken(ctx, "", rpcapi.FriendInviteTokenClearRequest{}); err == nil {
		t.Fatal("ClearFriendInviteToken empty owner error = nil")
	}
	if _, err := s.AddFriend(ctx, "", rpcapi.FriendAddRequest{InviteToken: "token"}); err == nil {
		t.Fatal("AddFriend empty owner error = nil")
	}
	if _, err := s.AddFriend(ctx, "peer-a", rpcapi.FriendAddRequest{}); err == nil {
		t.Fatal("AddFriend empty token error = nil")
	}
	defaultClock := &Server{InviteTokens: kv.NewMemory(nil), Friends: kv.NewMemory(nil)}
	if created, err := defaultClock.CreateFriendInviteToken(ctx, "peer-z", rpcapi.FriendInviteTokenCreateRequest{}); err != nil || created.InviteToken == "" || created.ExpiresAt.IsZero() {
		t.Fatalf("CreateFriendInviteToken with defaults = %#v, %v", created, err)
	}
	if id := (&Server{}).newID(); id == "" {
		t.Fatal("newID without override returned empty string")
	}
}

func TestAddFriendPropagatesInviteTokenStoreErrors(t *testing.T) {
	ctx := context.Background()
	s := newTestServer()
	s.InviteTokens = failingGetStore{Store: s.InviteTokens}

	_, err := s.AddFriend(ctx, "peer-a", rpcapi.FriendAddRequest{InviteToken: "token"})
	if err == nil {
		t.Fatal("AddFriend with failing invite token store error = nil")
	}
	if err.Error() != "forced list failure" {
		t.Fatalf("AddFriend error = %v, want forced list failure", err)
	}
}

func newTestServer() *Server {
	now := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	nextID := 0
	return &Server{
		InviteTokens: kv.NewMemory(nil),
		Friends:      kv.NewMemory(nil),
		Now:          func() time.Time { return now },
		NewID: func() string {
			nextID++
			return "id-" + string(rune('a'+nextID-1))
		},
	}
}

type failingBatchSetStore struct {
	kv.Store
}

func (s failingBatchSetStore) BatchSet(context.Context, []kv.Entry) error {
	return errors.New("forced batch set failure")
}

type failingGetStore struct {
	kv.Store
}

func (s failingGetStore) List(context.Context, kv.Key) iter.Seq2[kv.Entry, error] {
	return func(yield func(kv.Entry, error) bool) {
		yield(kv.Entry{}, errors.New("forced list failure"))
	}
}

type recordingWorkspaceService struct {
	created []adminservice.WorkspaceUpsert
	deleted []string
}

func (s *recordingWorkspaceService) CreateWorkspace(_ context.Context, req adminservice.CreateWorkspaceRequestObject) (adminservice.CreateWorkspaceResponseObject, error) {
	if req.Body == nil {
		return adminservice.CreateWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", "request body required")), nil
	}
	for _, workspace := range s.created {
		if workspace.Name == req.Body.Name {
			return adminservice.CreateWorkspace409JSONResponse(apitypes.NewErrorResponse("WORKSPACE_ALREADY_EXISTS", "exists")), nil
		}
	}
	s.created = append(s.created, *req.Body)
	return adminservice.CreateWorkspace200JSONResponse(apitypes.Workspace{Name: req.Body.Name, WorkflowName: req.Body.WorkflowName, Parameters: req.Body.Parameters}), nil
}

func (s *recordingWorkspaceService) DeleteWorkspace(_ context.Context, req adminservice.DeleteWorkspaceRequestObject) (adminservice.DeleteWorkspaceResponseObject, error) {
	s.deleted = append(s.deleted, req.Name)
	return adminservice.DeleteWorkspace200JSONResponse(apitypes.Workspace{Name: req.Name}), nil
}

type recordingACL struct {
	roles    map[string]apitypes.ACLPermissionList
	bindings map[string]apitypes.ACLPolicy
}

func (a *recordingACL) PutRole(_ context.Context, name string, permissions apitypes.ACLPermissionList) (apitypes.ACLRole, error) {
	if a.roles == nil {
		a.roles = make(map[string]apitypes.ACLPermissionList)
	}
	a.roles[name] = permissions
	return apitypes.ACLRole{Name: name, Permissions: permissions}, nil
}

func (a *recordingACL) PutPolicyBinding(_ context.Context, id string, _ float64, policy apitypes.ACLPolicy) (apitypes.ACLPolicyBinding, error) {
	if a.bindings == nil {
		a.bindings = make(map[string]apitypes.ACLPolicy)
	}
	a.bindings[id] = policy
	return apitypes.ACLPolicyBinding{Id: id, Policy: policy}, nil
}

func (a *recordingACL) DeletePolicyBinding(_ context.Context, id string) (apitypes.ACLPolicyBinding, error) {
	if a.bindings == nil {
		return apitypes.ACLPolicyBinding{}, acl.ErrPolicyBindingNotFound
	}
	policy, ok := a.bindings[id]
	if !ok {
		return apitypes.ACLPolicyBinding{}, acl.ErrPolicyBindingNotFound
	}
	delete(a.bindings, id)
	return apitypes.ACLPolicyBinding{Id: id, Policy: policy}, nil
}

func (a *recordingACL) authorizeWorkspace(workspaceName string, peerID string) error {
	id := socialutil.WorkspaceACLBindingID(workspaceName, peerID)
	policy, ok := a.bindings[id]
	if !ok {
		return acl.ErrDenied
	}
	if policy.Resource.Kind != apitypes.ACLResourceKindWorkspace || policy.Resource.Id != workspaceName || policy.Subject.Kind != apitypes.ACLSubjectKindPk || policy.Subject.Id != peerID {
		return errors.New("unexpected workspace ACL policy")
	}
	return nil
}
