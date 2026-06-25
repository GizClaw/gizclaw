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
	if socialutil.StringValue(friend.Id) != "peer-b" {
		t.Fatalf("AddFriend id = %q, want peer-b", socialutil.StringValue(friend.Id))
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
		t.Fatalf("duplicate friend id = %q, want %q", socialutil.StringValue(duplicate.Id), socialutil.StringValue(friend.Id))
	}

	for _, tc := range []struct{ owner, wantID string }{{"peer-a", "peer-b"}, {"peer-b", "peer-a"}} {
		friends, err := s.ListFriends(ctx, tc.owner, rpcapi.FriendListRequest{})
		if err != nil {
			t.Fatalf("ListFriends(%s): %v", tc.owner, err)
		}
		if len(friends.Items) != 1 {
			t.Fatalf("ListFriends(%s) len = %d, want 1", tc.owner, len(friends.Items))
		}
		if socialutil.StringValue(friends.Items[0].Id) != tc.wantID {
			t.Fatalf("ListFriends(%s) id = %#v, want %q", tc.owner, friends.Items[0].Id, tc.wantID)
		}
		if socialutil.StringValue(friends.Items[0].WorkspaceName) != workspaceName {
			t.Fatalf("ListFriends(%s) workspace_name = %#v, want %q", tc.owner, friends.Items[0].WorkspaceName, workspaceName)
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

	if _, err := s.DeleteFriend(ctx, "peer-a", rpcapi.FriendDeleteRequest{Id: socialutil.StringValue(friend.Id)}); err != nil {
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

func TestAdminCreateFriendMaintainsRowsAndChatWorkspace(t *testing.T) {
	ctx := context.Background()
	s := newTestServer()
	workspaces := &recordingWorkspaceService{}
	aclSvc := &recordingACL{}
	s.Workspaces = workspaces
	s.ACL = aclSvc

	friend, err := s.AdminCreateFriend(ctx, "peer-a", "peer-b")
	if err != nil {
		t.Fatalf("AdminCreateFriend: %v", err)
	}
	relationID := socialutil.RelationID("peer-a", "peer-b")
	if socialutil.StringValue(friend.Id) != "peer-b" || socialutil.StringValue(friend.PeerPublicKey) != "peer-b" {
		t.Fatalf("AdminCreateFriend item = %#v", friend)
	}
	workspaceName := socialutil.StringValue(friend.WorkspaceName)
	if workspaceName == "" {
		t.Fatal("AdminCreateFriend workspace_name is empty")
	}
	if len(workspaces.created) != 1 || workspaces.created[0].Name != workspaceName {
		t.Fatalf("created workspaces = %#v, want %q", workspaces.created, workspaceName)
	}
	if err := aclSvc.authorizeWorkspace(workspaceName, "peer-a"); err != nil {
		t.Fatalf("peer-a workspace authorize: %v", err)
	}
	if err := aclSvc.authorizeWorkspace(workspaceName, "peer-b"); err != nil {
		t.Fatalf("peer-b workspace authorize: %v", err)
	}

	otherRow, err := s.GetFriendRelation(ctx, "peer-b", relationID)
	if err != nil {
		t.Fatalf("GetFriendRelation peer-b: %v", err)
	}
	if socialutil.StringValue(otherRow.Id) != "peer-a" || socialutil.StringValue(otherRow.PeerPublicKey) != "peer-a" || socialutil.StringValue(otherRow.WorkspaceName) != workspaceName {
		t.Fatalf("peer-b row = %#v", otherRow)
	}
	duplicate, err := s.AdminCreateFriend(ctx, "peer-a", "peer-b")
	if err != nil {
		t.Fatalf("AdminCreateFriend duplicate: %v", err)
	}
	if socialutil.StringValue(duplicate.Id) != "peer-b" || len(workspaces.created) != 1 {
		t.Fatalf("duplicate = %#v created=%#v, want existing row without new workspace", duplicate, workspaces.created)
	}
	adminPage, err := s.AdminListFriends(ctx, nil, socialutil.IntPtr(1))
	if err != nil {
		t.Fatalf("AdminListFriends first page: %v", err)
	}
	if len(adminPage.Items) != 1 || !adminPage.HasNext || adminPage.NextCursor == nil {
		t.Fatalf("AdminListFriends first page = %#v, want one row with next cursor", adminPage)
	}
	if adminPage.Items[0].OwnerPublicKey == "" || adminPage.Items[0].PeerPublicKey == "" || adminPage.Items[0].Id != adminPage.Items[0].PeerPublicKey {
		t.Fatalf("AdminListFriends item = %#v", adminPage.Items[0])
	}
	nextPage, err := s.AdminListFriends(ctx, adminPage.NextCursor, socialutil.IntPtr(10))
	if err != nil {
		t.Fatalf("AdminListFriends next page: %v", err)
	}
	if len(nextPage.Items) != 1 || nextPage.HasNext {
		t.Fatalf("AdminListFriends next page = %#v, want final single row", nextPage)
	}
	adminRow, err := s.AdminGetFriend(ctx, "peer-a", relationID)
	if err != nil {
		t.Fatalf("AdminGetFriend: %v", err)
	}
	if adminRow.OwnerPublicKey != "peer-a" || adminRow.Id != "peer-b" || adminRow.PeerPublicKey != "peer-b" || adminRow.WorkspaceName != workspaceName {
		t.Fatalf("AdminGetFriend row = %#v", adminRow)
	}
	if _, err := s.AdminCreateFriend(ctx, "peer-a", "peer-a"); err == nil {
		t.Fatal("AdminCreateFriend self error = nil")
	}

	deleted, err := s.AdminDeleteFriend(ctx, "peer-a", relationID)
	if err != nil {
		t.Fatalf("AdminDeleteFriend: %v", err)
	}
	if deleted.OwnerPublicKey != "peer-a" || deleted.Id != "peer-b" || deleted.PeerPublicKey != "peer-b" {
		t.Fatalf("AdminDeleteFriend row = %#v", deleted)
	}
	if _, err := s.GetFriendRelation(ctx, "peer-a", relationID); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("peer-a row after delete error = %v, want not found", err)
	}
	if _, err := s.GetFriendRelation(ctx, "peer-b", relationID); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("peer-b row after delete error = %v, want not found", err)
	}
}

func TestAdminFriendResourceWrappersAndCursorHelpers(t *testing.T) {
	ctx := context.Background()
	s := newTestServer()

	created, err := s.AdminCreateFriendResource(ctx, " peer-c ", "peer-d")
	if err != nil {
		t.Fatalf("AdminCreateFriendResource: %v", err)
	}
	if created.OwnerPublicKey != "peer-c" || created.PeerPublicKey != "peer-d" || created.Id != "peer-d" {
		t.Fatalf("AdminCreateFriendResource row = %#v", created)
	}
	if created.WorkspaceName != socialutil.DirectWorkspaceName(socialutil.RelationID("peer-c", "peer-d")) {
		t.Fatalf("AdminCreateFriendResource workspace = %q, want direct workspace", created.WorkspaceName)
	}
	page, err := s.AdminListFriends(ctx, stringPtr("malformed/cursor/value"), socialutil.IntPtr(10))
	if err != nil {
		t.Fatalf("AdminListFriends malformed cursor: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("AdminListFriends malformed cursor items = %#v, want both owner-view rows", page.Items)
	}
	if owner, ok := adminFriendOwner(kv.Key{"friends"}); ok || owner != "" {
		t.Fatalf("adminFriendOwner short key = %q, %t; want empty false", owner, ok)
	}
	if cursor := adminFriendCursor(kv.Key{"friends"}); cursor != "" {
		t.Fatalf("adminFriendCursor short key = %q, want empty", cursor)
	}
	if after := adminFriendCursorAfter("/missing-owner"); after != nil {
		t.Fatalf("adminFriendCursorAfter malformed = %#v, want nil", after)
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
	if _, err := empty.AdminListFriends(ctx, nil, nil); err == nil {
		t.Fatal("AdminListFriends without store error = nil")
	}
	if _, err := empty.AdminCreateFriendResource(ctx, "peer-a", "peer-b"); err == nil {
		t.Fatal("AdminCreateFriendResource without store error = nil")
	}
	if _, err := empty.AdminGetFriend(ctx, "peer-a", "peer-a:peer-b"); err == nil {
		t.Fatal("AdminGetFriend without store error = nil")
	}
	if _, err := empty.AdminDeleteFriend(ctx, "peer-a", "peer-a:peer-b"); err == nil {
		t.Fatal("AdminDeleteFriend without store error = nil")
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

func stringPtr(value string) *string {
	return &value
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
