package friend

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type profileStub struct {
	want giznet.PublicKey
	info apitypes.DeviceInfo
}

func (s profileStub) GetSelfInfo(_ context.Context, key giznet.PublicKey) (apitypes.DeviceInfo, error) {
	if key != s.want {
		return apitypes.DeviceInfo{}, errors.New("unexpected profile key")
	}
	return s.info, nil
}

func TestGetFriendInfoRequiresCallerRelation(t *testing.T) {
	ctx := context.Background()
	owner := giznet.PublicKey{1}.String()
	targetKey := giznet.PublicKey{2}
	target := targetKey.String()
	name, emoji := "Astronaut", "🧑‍🚀"
	s := newTestServer()
	s.Profiles = profileStub{want: targetKey, info: apitypes.DeviceInfo{Name: &name, Emoji: &emoji}}
	relationID := socialutil.RelationID(owner, target)
	if err := socialutil.WriteJSON(ctx, s.Friends, socialutil.FriendKey(owner, relationID), rpcapi.FriendObject{Id: &target, PeerPublicKey: &target}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetFriendInfo(ctx, owner, rpcapi.FriendInfoGetRequest{Id: target})
	if err != nil {
		t.Fatalf("GetFriendInfo() error = %v", err)
	}
	if got.Id != target || got.Value.Name == nil || *got.Value.Name != name || got.Value.Emoji == nil || *got.Value.Emoji != emoji {
		t.Fatalf("GetFriendInfo() = %+v", got)
	}
	if _, err := s.GetFriendInfo(ctx, giznet.PublicKey{3}.String(), rpcapi.FriendInfoGetRequest{Id: target}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("GetFriendInfo() unauthorized error = %v, want not found", err)
	}
}

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
	created []adminhttp.WorkspaceUpsert
	deleted []string
}

func (s *recordingWorkspaceService) CreateSystemWorkspace(_ context.Context, body adminhttp.WorkspaceUpsert) (apitypes.Workspace, bool, error) {
	for _, existing := range s.created {
		if existing.Name == body.Name {
			system := true
			return apitypes.Workspace{Name: body.Name, WorkflowName: body.WorkflowName, Parameters: body.Parameters, System: &system}, false, nil
		}
	}
	s.created = append(s.created, body)
	system := true
	return apitypes.Workspace{Name: body.Name, WorkflowName: body.WorkflowName, Parameters: body.Parameters, System: &system}, true, nil
}

func (s *recordingWorkspaceService) DeleteSystemWorkspace(_ context.Context, name string) (apitypes.Workspace, error) {
	s.deleted = append(s.deleted, name)
	return apitypes.Workspace{Name: name}, nil
}

func (s *recordingWorkspaceService) CreateWorkspace(_ context.Context, req adminhttp.CreateWorkspaceRequestObject) (adminhttp.CreateWorkspaceResponseObject, error) {
	if req.Body == nil {
		return adminhttp.CreateWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", "request body required")), nil
	}
	for _, workspace := range s.created {
		if workspace.Name == req.Body.Name {
			return adminhttp.CreateWorkspace409JSONResponse(apitypes.NewErrorResponse("WORKSPACE_ALREADY_EXISTS", "exists")), nil
		}
	}
	s.created = append(s.created, *req.Body)
	return adminhttp.CreateWorkspace200JSONResponse(apitypes.Workspace{Name: req.Body.Name, WorkflowName: req.Body.WorkflowName, Parameters: req.Body.Parameters}), nil
}

func (s *recordingWorkspaceService) DeleteWorkspace(_ context.Context, req adminhttp.DeleteWorkspaceRequestObject) (adminhttp.DeleteWorkspaceResponseObject, error) {
	s.deleted = append(s.deleted, req.Name)
	return adminhttp.DeleteWorkspace200JSONResponse(apitypes.Workspace{Name: req.Name}), nil
}
