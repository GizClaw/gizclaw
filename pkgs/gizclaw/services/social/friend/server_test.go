package friend

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type profileStub struct {
	want giznet.PublicKey
	info apitypes.DeviceInfo
}

type friendNotification struct {
	recipient string
	event     *eventpb.PeerEvent
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

func TestFriendRelationshipEventsReachBothRecipientViews(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	var notifications []friendNotification
	s.NotifyPeer = func(_ context.Context, recipient string, event *eventpb.PeerEvent) {
		notifications = append(notifications, friendNotification{recipient: recipient, event: event})
	}

	friend, err := s.AdminCreateFriendResource(ctx, "peer-a", "peer-b")
	if err != nil {
		t.Fatalf("AdminCreateFriendResource: %v", err)
	}
	s.Workspaces = &recordingWorkspaceService{}
	assertFriendRelationshipNotifications(
		t,
		notifications,
		eventpb.FriendRelationshipChange_FRIEND_RELATIONSHIP_CHANGE_CREATED,
		friend.WorkspaceName,
	)

	notifications = nil
	if _, err := s.DeleteFriend(ctx, "peer-a", rpcapi.FriendDeleteRequest{Id: "peer-b"}); err != nil {
		t.Fatalf("DeleteFriend: %v", err)
	}
	assertFriendRelationshipNotifications(
		t,
		notifications,
		eventpb.FriendRelationshipChange_FRIEND_RELATIONSHIP_CHANGE_DELETED,
		friend.WorkspaceName,
	)
}

func assertFriendRelationshipNotifications(
	t *testing.T,
	notifications []friendNotification,
	change eventpb.FriendRelationshipChange,
	workspaceName string,
) {
	t.Helper()
	if len(notifications) != 2 {
		t.Fatalf("notifications = %#v, want one event for each relationship view", notifications)
	}
	wantPeer := map[string]string{"peer-a": "peer-b", "peer-b": "peer-a"}
	for _, notification := range notifications {
		payload := notification.event.GetFriendRelationshipUpdated()
		if notification.event.GetType() != eventpb.PeerEventType_PEER_EVENT_TYPE_FRIEND_RELATIONSHIP_UPDATED ||
			payload == nil ||
			payload.GetPeerPublicKey() != wantPeer[notification.recipient] ||
			payload.GetWorkspaceName() != workspaceName ||
			payload.GetChange() != change {
			t.Fatalf("notification = recipient=%q event=%+v", notification.recipient, notification.event)
		}
		delete(wantPeer, notification.recipient)
	}
	if len(wantPeer) != 0 {
		t.Fatalf("missing recipients = %#v", wantPeer)
	}
}

func TestAddFriendWorkspaceBelongsToInviteTokenCreator(t *testing.T) {
	ctx := context.Background()
	workspaces := &recordingWorkspaceService{}
	s := newTestServer()
	s.Workspaces = workspaces
	s.RuntimeProfileForOwner = func(_ context.Context, owner string) (apitypes.RuntimeProfile, error) {
		if owner != "peer-b" {
			t.Fatalf("RuntimeProfileForOwner owner = %q, want peer-b", owner)
		}
		return apitypes.RuntimeProfile{Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{
				System: apitypes.RuntimeProfileSystemWorkflows{FriendChatroom: "owner-direct-chat"},
			},
		}}, nil
	}
	token, err := s.CreateFriendInviteToken(ctx, "peer-b", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFriend(ctx, "peer-a", rpcapi.FriendAddRequest{InviteToken: token.InviteToken}); err != nil {
		t.Fatal(err)
	}
	if len(workspaces.created) != 1 || workspaces.created[0].WorkflowName != "owner-direct-chat" {
		t.Fatalf("created Workspaces = %#v", workspaces.created)
	}
	if len(workspaces.owners) != 1 || workspaces.owners[0] != "peer-b" {
		t.Fatalf("Workspace owners = %#v, want peer-b", workspaces.owners)
	}
	reciprocalToken, err := s.CreateFriendInviteToken(ctx, "peer-a", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	reciprocal, err := s.AddFriend(ctx, "peer-b", rpcapi.FriendAddRequest{InviteToken: reciprocalToken.InviteToken})
	if err != nil {
		t.Fatalf("AddFriend existing relation through reciprocal invite: %v", err)
	}
	if socialutil.StringValue(reciprocal.PeerPublicKey) != "peer-a" {
		t.Fatalf("reciprocal friend = %#v, want peer-a", reciprocal)
	}
	if len(workspaces.created) != 1 || len(workspaces.owners) != 1 {
		t.Fatalf("reciprocal invite recreated Workspace: created=%#v owners=%#v", workspaces.created, workspaces.owners)
	}
}

func TestAdminCreateExistingFriendPreservesWorkspaceBinding(t *testing.T) {
	ctx := context.Background()
	workspaces := &recordingWorkspaceService{}
	s := newTestServer()
	s.Workspaces = workspaces
	s.RuntimeProfileForOwner = func(_ context.Context, owner string) (apitypes.RuntimeProfile, error) {
		return apitypes.RuntimeProfile{Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{
				System: apitypes.RuntimeProfileSystemWorkflows{FriendChatroom: owner + "-direct-chat"},
			},
		}}, nil
	}
	first, err := s.AdminCreateFriend(ctx, "peer-a", "peer-b")
	if err != nil {
		t.Fatal(err)
	}
	s.RuntimeProfileForOwner = func(context.Context, string) (apitypes.RuntimeProfile, error) {
		return apitypes.RuntimeProfile{}, errors.New("existing relation must not resolve a new system Workflow")
	}
	existing, err := s.AdminCreateFriend(ctx, "peer-b", "peer-a")
	if err != nil {
		t.Fatal(err)
	}
	if socialutil.StringValue(existing.WorkspaceName) != socialutil.StringValue(first.WorkspaceName) {
		t.Fatalf("existing Workspace = %q, want %q", socialutil.StringValue(existing.WorkspaceName), socialutil.StringValue(first.WorkspaceName))
	}
	if len(workspaces.created) != 1 || len(workspaces.owners) != 1 {
		t.Fatalf("existing Admin create recreated Workspace: created=%#v owners=%#v", workspaces.created, workspaces.owners)
	}
}

func TestDeleteFriendIsRelationshipFirstAndRetryable(t *testing.T) {
	ctx := t.Context()
	workspaces := &recordingWorkspaceService{}
	s := newTestServer()
	friend, err := s.AdminCreateFriendResource(ctx, "peer-a", "peer-b")
	if err != nil {
		t.Fatalf("AdminCreateFriendResource: %v", err)
	}
	var notifications []friendNotification
	s.NotifyPeer = func(_ context.Context, recipient string, event *eventpb.PeerEvent) {
		notifications = append(notifications, friendNotification{recipient: recipient, event: event})
	}
	s.Workspaces = workspaces
	workspaces.retireErr = errors.New("forced retirement failure")

	if _, err := s.DeleteFriend(ctx, "peer-a", rpcapi.FriendDeleteRequest{Id: "peer-b"}); !errors.Is(err, workspaces.retireErr) {
		t.Fatalf("DeleteFriend first error = %v, want retirement failure", err)
	}
	for _, owner := range []string{"peer-a", "peer-b"} {
		if _, err := s.GetFriendRelation(ctx, owner, map[string]string{"peer-a": "peer-b", "peer-b": "peer-a"}[owner]); !errors.Is(err, kv.ErrNotFound) {
			t.Fatalf("GetFriendRelation(%s) after committed delete = %v, want not found", owner, err)
		}
	}
	if len(workspaces.deleted) != 0 || len(workspaces.retired) != 1 {
		t.Fatalf("workspace calls after first delete: deleted=%v retired=%v", workspaces.deleted, workspaces.retired)
	}
	if len(notifications) != 0 {
		t.Fatalf("notifications before durable PendingDeletion = %#v, want none", notifications)
	}

	workspaces.retireErr = nil
	restarted := &Server{
		InviteTokens: s.InviteTokens,
		Friends:      s.Friends,
		Workspaces:   workspaces,
		Now:          s.Now,
		NotifyPeer:   s.NotifyPeer,
	}
	if err := restarted.ReconcileRetirementIntents(ctx); err != nil {
		t.Fatalf("ReconcileRetirementIntents after restart: %v", err)
	}
	if len(workspaces.retired) != 2 || workspaces.retired[0] != workspaces.retired[1] {
		t.Fatalf("retirement retry targets = %v, want same Workspace twice", workspaces.retired)
	}
	assertFriendRelationshipNotifications(
		t,
		notifications,
		eventpb.FriendRelationshipChange_FRIEND_RELATIONSHIP_CHANGE_DELETED,
		friend.WorkspaceName,
	)
	notificationCount := len(notifications)
	retried, err := restarted.DeleteFriend(ctx, "peer-a", rpcapi.FriendDeleteRequest{Id: "peer-b"})
	if err != nil {
		t.Fatalf("DeleteFriend retry after completed retirement: %v", err)
	}
	if socialutil.StringValue(retried.Id) != "peer-b" ||
		socialutil.StringValue(retried.WorkspaceName) != friend.WorkspaceName {
		t.Fatalf("DeleteFriend completed retry = %#v", retried)
	}
	if len(notifications) != notificationCount {
		t.Fatalf("completed retry notifications = %d, want %d", len(notifications), notificationCount)
	}
}

func TestDeleteFriendWithoutWorkspaceRetirementKeepsRelationship(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	if _, err := s.AdminCreateFriendResource(ctx, "peer-a", "peer-b"); err != nil {
		t.Fatalf("AdminCreateFriendResource: %v", err)
	}

	if _, err := s.DeleteFriend(ctx, "peer-a", rpcapi.FriendDeleteRequest{Id: "peer-b"}); err == nil ||
		!strings.Contains(err.Error(), "retirement service not configured") {
		t.Fatalf("DeleteFriend error = %v, want missing retirement service", err)
	}
	relationID := socialutil.RelationID("peer-a", "peer-b")
	if _, err := s.GetFriendRelation(ctx, "peer-a", "peer-b"); err != nil {
		t.Fatalf("GetFriendRelation after rejected delete: %v", err)
	}
	if _, err := readRetirementIntent(ctx, s.Friends, relationID); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("readRetirementIntent after rejected delete error = %v, want not found", err)
	}
}

func TestDeleteFriendBatchFailureKeepsRelationshipAndWorkspace(t *testing.T) {
	ctx := t.Context()
	workspaces := &recordingWorkspaceService{}
	s := newTestServer()
	if _, err := s.AdminCreateFriendResource(ctx, "peer-a", "peer-b"); err != nil {
		t.Fatalf("AdminCreateFriendResource: %v", err)
	}
	s.Workspaces = workspaces
	s.Friends = failingBatchMutateStore{Store: s.Friends}

	if _, err := s.DeleteFriend(ctx, "peer-a", rpcapi.FriendDeleteRequest{Id: "peer-b"}); err == nil {
		t.Fatal("DeleteFriend with failing BatchMutate error = nil")
	}
	for _, pair := range [][2]string{{"peer-a", "peer-b"}, {"peer-b", "peer-a"}} {
		if _, err := s.GetFriendRelation(ctx, pair[0], pair[1]); err != nil {
			t.Fatalf("GetFriendRelation(%s,%s) after batch failure: %v", pair[0], pair[1], err)
		}
	}
	if len(workspaces.retired) != 0 || len(workspaces.deleted) != 0 {
		t.Fatalf("workspace changed after relationship batch failure: retired=%v deleted=%v", workspaces.retired, workspaces.deleted)
	}
}

func TestConcurrentAdminCreateFriendSerializesWorkspaceLifecycle(t *testing.T) {
	ctx := context.Background()
	workspaces := &recordingWorkspaceService{}
	s := newTestServer()
	s.Workspaces = workspaces
	resolverCalls := make(chan string, 2)
	releaseResolver := make(chan struct{})
	s.RuntimeProfileForOwner = func(_ context.Context, owner string) (apitypes.RuntimeProfile, error) {
		resolverCalls <- owner
		<-releaseResolver
		return apitypes.RuntimeProfile{Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{
				System: apitypes.RuntimeProfileSystemWorkflows{FriendChatroom: "direct-chat"},
			},
		}}, nil
	}
	firstDone := make(chan error, 1)
	go func() {
		_, err := s.AdminCreateFriend(ctx, "peer-a", "peer-b")
		firstDone <- err
	}()
	if owner := <-resolverCalls; owner != "peer-a" {
		t.Fatalf("first resolver owner = %q, want peer-a", owner)
	}
	secondDone := make(chan error, 1)
	go func() {
		_, err := s.AdminCreateFriend(ctx, "peer-b", "peer-a")
		secondDone <- err
	}()
	select {
	case owner := <-resolverCalls:
		t.Fatalf("concurrent create resolved another Workspace binding for %q", owner)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseResolver)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	if len(workspaces.created) != 1 || len(workspaces.owners) != 1 {
		t.Fatalf("concurrent Admin create Workspaces: created=%#v owners=%#v", workspaces.created, workspaces.owners)
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

type failingBatchMutateStore struct {
	kv.Store
}

func (s failingBatchMutateStore) BatchMutate(context.Context, []kv.Entry, []kv.Key) error {
	return errors.New("forced batch mutate failure")
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
	created   []adminhttp.WorkspaceUpsert
	deleted   []string
	retired   []string
	owners    []string
	retireErr error
}

func (s *recordingWorkspaceService) CreateSystemWorkspace(ctx context.Context, body adminhttp.WorkspaceUpsert) (apitypes.Workspace, bool, error) {
	owner, _ := ownership.FromContext(ctx)
	s.owners = append(s.owners, owner)
	for _, existing := range s.created {
		if existing.Name == body.Name {
			system := true
			return apitypes.Workspace{Name: body.Name, WorkflowName: body.WorkflowName, Parameters: body.Parameters, OwnerPublicKey: &owner, System: &system}, false, nil
		}
	}
	s.created = append(s.created, body)
	system := true
	return apitypes.Workspace{Name: body.Name, WorkflowName: body.WorkflowName, Parameters: body.Parameters, OwnerPublicKey: &owner, System: &system}, true, nil
}

func (s *recordingWorkspaceService) DeleteSystemWorkspace(_ context.Context, name string) (apitypes.Workspace, error) {
	s.deleted = append(s.deleted, name)
	return apitypes.Workspace{Name: name}, nil
}

func (s *recordingWorkspaceService) RetireSystemWorkspace(_ context.Context, name string, _ apitypes.ChatRoomMode, _ string) (apitypes.Workspace, error) {
	s.retired = append(s.retired, name)
	return apitypes.Workspace{Name: name}, s.retireErr
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
