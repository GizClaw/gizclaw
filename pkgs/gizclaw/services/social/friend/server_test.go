package friend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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

type assignmentStub map[giznet.PublicKey]apitypes.PeerAssignment

func (s assignmentStub) Lookup(_ context.Context, key giznet.PublicKey) (apitypes.PeerAssignment, error) {
	assignment, ok := s[key]
	if !ok {
		return apitypes.PeerAssignment{}, kv.ErrNotFound
	}
	return assignment, nil
}

func TestCrossServerFriendCreationStopsBeforeMutation(t *testing.T) {
	localKey := giznet.PublicKey{1}
	foreignKey := giznet.PublicKey{2}
	serverKey := giznet.PublicKey{8}
	foreignServerKey := giznet.PublicKey{9}
	s := newTestServer()
	s.ServerPublicKey = serverKey
	s.PeerAssignments = assignmentStub{
		localKey:   {PeerPublicKey: localKey.String(), ServerPublicKey: serverKey.String()},
		foreignKey: {PeerPublicKey: foreignKey.String(), ServerPublicKey: foreignServerKey.String()},
	}
	workspaces := s.Workspaces.(*recordingWorkspaceService)
	profileCalls := 0
	s.RuntimeProfileForOwner = func(context.Context, string) (apitypes.RuntimeProfile, error) {
		profileCalls++
		return apitypes.RuntimeProfile{}, errors.New("profile must not be resolved")
	}
	token, err := s.CreateFriendInviteToken(t.Context(), foreignKey.String(), rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFriend(t.Context(), localKey.String(), rpcapi.FriendAddRequest{InviteToken: token.InviteToken}); !errors.Is(err, ErrCrossServerFriendCreation) {
		t.Fatalf("AddFriend() error = %v, want cross-server conflict", err)
	}
	if profileCalls != 0 || len(workspaces.created) != 0 {
		t.Fatalf("rejected creation resolved %d profiles and created %d Workspaces", profileCalls, len(workspaces.created))
	}
	assertNoFriendCreationState(t, s.Friends)
	active, ok, err := s.activeInviteToken(t.Context(), s.InviteTokens, foreignKey.String())
	if err != nil || !ok || active.InviteToken != token.InviteToken {
		t.Fatalf("invite after rejection = %+v, %v, %v", active, ok, err)
	}
	if _, err := s.AdminCreateFriend(t.Context(), localKey.String(), foreignKey.String()); !errors.Is(err, ErrCrossServerFriendCreation) {
		t.Fatalf("AdminCreateFriend() error = %v, want cross-server conflict", err)
	}
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
	now := s.now()
	if err := socialutil.WriteJSON(ctx, s.Friends, socialutil.FriendKey(owner, relationID), friendRecord{
		RelationID: relationID, PeerPublicKey: target, WorkspaceName: "friend-chat", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetFriendInfo(ctx, owner, rpcapi.FriendInfoGetRequest{Name: target})
	if err != nil {
		t.Fatalf("GetFriendInfo() error = %v", err)
	}
	if got.Name != target || got.Value.DisplayName == nil || *got.Value.DisplayName != name || got.Value.Emoji == nil || *got.Value.Emoji != emoji {
		t.Fatalf("GetFriendInfo() = %+v", got)
	}
	if _, err := s.GetFriendInfo(ctx, giznet.PublicKey{3}.String(), rpcapi.FriendInfoGetRequest{Name: target}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("GetFriendInfo() unauthorized error = %v, want not found", err)
	}
}

func TestLegacyWireFriendRecordIsRejected(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	relationID := socialutil.RelationID("peer-a", "peer-b")
	legacy := []byte(`{"id":"peer-b","name":"peer-b","peer_public_key":"peer-b","workspace_name":"friend-chat"}`)
	if err := s.Friends.Set(ctx, socialutil.FriendKey("peer-a", relationID), legacy); err != nil {
		t.Fatalf("seed legacy Friend wire record: %v", err)
	}
	if _, err := s.ListFriends(ctx, "peer-a", rpcapi.FriendListRequest{}); err == nil || err.Error() != "social: persisted Friend relationship is invalid" {
		t.Fatalf("ListFriends legacy wire record error = %v", err)
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

	if _, err := s.AddFriend(ctx, "peer-a", rpcapi.FriendAddRequest{InviteToken: "missing"}); !errors.Is(err, ErrInviteTokenUnavailable) {
		t.Fatalf("AddFriend missing token error = %v, want unavailable", err)
	}
	if _, err := s.AddFriend(ctx, "peer-b", rpcapi.FriendAddRequest{InviteToken: created.InviteToken}); !errors.Is(err, ErrInviteTokenSelfOwned) {
		t.Fatalf("AddFriend self token error = %v, want self-owned", err)
	}

	friend, err := s.AddFriend(ctx, "peer-a", rpcapi.FriendAddRequest{InviteToken: created.InviteToken})
	if err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	if socialutil.StringValue(friend.PeerPublicKey) != "peer-b" {
		t.Fatalf("AddFriend peer_public_key = %q, want peer-b", socialutil.StringValue(friend.PeerPublicKey))
	}
	if friend.Name != "peer-b" {
		t.Fatalf("AddFriend name = %q, want peer-b", friend.Name)
	}
	friendStore, err := s.friendsStore()
	if err != nil {
		t.Fatalf("friendsStore: %v", err)
	}
	relationID := socialutil.RelationID("peer-a", "peer-b")
	storedFriend, err := friendStore.Get(ctx, socialutil.FriendKey("peer-a", relationID))
	if err != nil {
		t.Fatalf("read persisted Friend: %v", err)
	}
	var storedFields map[string]json.RawMessage
	if err := json.Unmarshal(storedFriend, &storedFields); err != nil {
		t.Fatalf("decode persisted Friend: %v", err)
	}
	if _, ok := storedFields["relation_id"]; !ok {
		t.Fatalf("persisted Friend = %s, want canonical relation_id", storedFriend)
	}
	if _, ok := storedFields["id"]; ok {
		t.Fatalf("persisted Friend = %s, unexpectedly stores Peer wire id", storedFriend)
	}
	if _, ok := storedFields["name"]; ok {
		t.Fatalf("persisted Friend = %s, unexpectedly stores Peer wire name", storedFriend)
	}
	workspaceName := socialutil.StringValue(friend.WorkspaceName)
	if workspaceName == "" {
		t.Fatal("AddFriend workspace_name is empty")
	}
	duplicate, err := s.AddFriend(ctx, "peer-a", rpcapi.FriendAddRequest{InviteToken: created.InviteToken})
	if err != nil {
		t.Fatalf("AddFriend duplicate: %v", err)
	}
	if duplicate.Name != friend.Name {
		t.Fatalf("duplicate friend name = %q, want %q", duplicate.Name, friend.Name)
	}

	for _, tc := range []struct{ owner, wantID string }{{"peer-a", "peer-b"}, {"peer-b", "peer-a"}} {
		friends, err := s.ListFriends(ctx, tc.owner, rpcapi.FriendListRequest{})
		if err != nil {
			t.Fatalf("ListFriends(%s): %v", tc.owner, err)
		}
		if len(friends.Items) != 1 {
			t.Fatalf("ListFriends(%s) len = %d, want 1", tc.owner, len(friends.Items))
		}
		if friends.Items[0].Name != tc.wantID {
			t.Fatalf("ListFriends(%s) name = %#v, want %q", tc.owner, friends.Items[0].Name, tc.wantID)
		}
		if socialutil.StringValue(friends.Items[0].WorkspaceName) != workspaceName {
			t.Fatalf("ListFriends(%s) workspace_name = %#v, want %q", tc.owner, friends.Items[0].WorkspaceName, workspaceName)
		}
	}
}

func TestAddFriendInviteTokenRejectionsDoNotCreateRelationships(t *testing.T) {
	s := newTestServer()
	created, err := s.CreateFriendInviteToken(t.Context(), "peer-b", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatalf("CreateFriendInviteToken: %v", err)
	}

	tests := []struct {
		name        string
		owner       string
		inviteToken string
		want        error
	}{
		{name: "empty", owner: "peer-a", want: ErrInviteTokenRequired},
		{name: "whitespace only", owner: "peer-a", inviteToken: " \t\n", want: ErrInviteTokenRequired},
		{name: "unknown", owner: "peer-a", inviteToken: "missing", want: ErrInviteTokenUnavailable},
		{name: "malformed", owner: "peer-a", inviteToken: "not/a/token?!", want: ErrInviteTokenUnavailable},
		{name: "whitespace wrapped active", owner: "peer-a", inviteToken: " " + created.InviteToken + " ", want: ErrInviteTokenUnavailable},
		{name: "self owned", owner: "peer-b", inviteToken: created.InviteToken, want: ErrInviteTokenSelfOwned},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := s.AddFriend(t.Context(), test.owner, rpcapi.FriendAddRequest{InviteToken: test.inviteToken}); !errors.Is(err, test.want) {
				t.Fatalf("AddFriend() error = %v, want %v", err, test.want)
			}
			friends, err := s.ListFriends(t.Context(), test.owner, rpcapi.FriendListRequest{})
			if err != nil {
				t.Fatalf("ListFriends(): %v", err)
			}
			if len(friends.Items) != 0 {
				t.Fatalf("ListFriends() = %#v, want no relationship", friends.Items)
			}
			assertNoFriendCreationState(t, s.Friends)
			if created := s.Workspaces.(*recordingWorkspaceService).created; len(created) != 0 {
				t.Fatalf("created Workspaces = %#v, want none", created)
			}
		})
	}
}

func TestFriendRelationshipEventsReachBothRecipientViews(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	var notifications []friendNotification
	s.NotifyPeer = func(_ context.Context, recipient string, event *eventpb.PeerEvent) {
		notifications = append(notifications, friendNotification{recipient: recipient, event: event})
	}

	friend, err := s.AdminCreateFriend(ctx, "peer-a", "peer-b")
	if err != nil {
		t.Fatalf("AdminCreateFriendResource: %v", err)
	}
	s.Workspaces = &recordingWorkspaceService{}
	assertFriendRelationshipNotifications(
		t,
		notifications,
		eventpb.FriendRelationshipChange_FRIEND_RELATIONSHIP_CHANGE_CREATED,
		socialutil.StringValue(friend.WorkspaceName),
	)

	notifications = nil
	if _, err := s.DeleteFriend(ctx, "peer-a", rpcapi.FriendDeleteRequest{Name: "peer-b"}); err != nil {
		t.Fatalf("DeleteFriend: %v", err)
	}
	assertFriendRelationshipNotifications(
		t,
		notifications,
		eventpb.FriendRelationshipChange_FRIEND_RELATIONSHIP_CHANGE_DELETED,
		socialutil.StringValue(friend.WorkspaceName),
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
	if len(workspaces.created) != 1 || workspaces.created[0].WorkflowId != "owner-direct-chat" {
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
	friend, err := s.AdminCreateFriend(ctx, "peer-a", "peer-b")
	if err != nil {
		t.Fatalf("AdminCreateFriendResource: %v", err)
	}
	var notifications []friendNotification
	s.NotifyPeer = func(_ context.Context, recipient string, event *eventpb.PeerEvent) {
		notifications = append(notifications, friendNotification{recipient: recipient, event: event})
	}
	s.Workspaces = workspaces
	workspaces.retireErr = errors.New("forced retirement failure")

	if _, err := s.DeleteFriend(ctx, "peer-a", rpcapi.FriendDeleteRequest{Name: "peer-b"}); !errors.Is(err, workspaces.retireErr) {
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
		socialutil.StringValue(friend.WorkspaceName),
	)
	notificationCount := len(notifications)
	retried, err := restarted.DeleteFriend(ctx, "peer-a", rpcapi.FriendDeleteRequest{Name: "peer-b"})
	if err != nil {
		t.Fatalf("DeleteFriend retry after completed retirement: %v", err)
	}
	if retried.Name != "peer-b" ||
		socialutil.StringValue(retried.WorkspaceName) != socialutil.StringValue(friend.WorkspaceName) {
		t.Fatalf("DeleteFriend completed retry = %#v", retried)
	}
	if len(notifications) != notificationCount {
		t.Fatalf("completed retry notifications = %d, want %d", len(notifications), notificationCount)
	}
	if len(workspaces.retired) != 2 {
		t.Fatalf("completed retry retired Workspace again: %v", workspaces.retired)
	}
}

func TestStaleRetirementCompletionLeavesNewerIntentUntouched(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	workspaces := &recordingWorkspaceService{}
	s.Workspaces = workspaces
	relationID := socialutil.RelationID("peer-a", "peer-b")
	stale := retirementIntent{
		RelationID:    relationID,
		FirstPeer:     "peer-a",
		SecondPeer:    "peer-b",
		WorkspaceID:   "id-stale-workspace",
		WorkspaceName: "stale-workspace",
		DeletedAt:     s.now(),
	}
	newer := retirementIntent{
		RelationID:    relationID,
		FirstPeer:     "peer-a",
		SecondPeer:    "peer-b",
		WorkspaceID:   "id-newer-workspace",
		WorkspaceName: "newer-workspace",
		DeletedAt:     s.now().Add(time.Second),
	}
	if err := socialutil.WriteJSON(
		ctx,
		s.Friends,
		retirementIntentKey(relationID),
		newer,
	); err != nil {
		t.Fatalf("seed newer retirement intent: %v", err)
	}

	if err := s.completeFriendRetirement(ctx, s.Friends, stale); err != nil {
		t.Fatalf("complete stale retirement: %v", err)
	}
	current, err := readRetirementIntent(ctx, s.Friends, relationID)
	if err != nil {
		t.Fatalf("read newer retirement intent: %v", err)
	}
	if current.WorkspaceID != newer.WorkspaceID || current.WorkspaceName != newer.WorkspaceName ||
		!current.DeletedAt.Equal(newer.DeletedAt) {
		t.Fatalf("stale completion changed newer intent: got %#v, want %#v", current, newer)
	}
	if _, err := readRetirementReceipt(
		ctx,
		s.Friends,
		relationID,
	); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("stale completion receipt error = %v, want not found", err)
	}
	if len(workspaces.retired) != 1 || workspaces.retired[0] != stale.WorkspaceID {
		t.Fatalf("retired Workspaces = %v, want %q", workspaces.retired, stale.WorkspaceID)
	}
}

func TestFriendRecreationUsesNewWorkspaceIncarnation(t *testing.T) {
	ctx := t.Context()
	workspaces := &recordingWorkspaceService{}
	s := newTestServer()
	s.Workspaces = workspaces

	var workspaceNames []string
	for lifecycle := range 3 {
		item, err := s.AdminCreateFriend(ctx, "peer-a", "peer-b")
		if err != nil {
			t.Fatalf("AdminCreateFriend lifecycle %d: %v", lifecycle, err)
		}
		workspaceNames = append(
			workspaceNames,
			socialutil.StringValue(item.WorkspaceName),
		)
		if lifecycle == 2 {
			break
		}
		if _, err := s.DeleteFriend(
			ctx,
			"peer-a",
			rpcapi.FriendDeleteRequest{Name: "peer-b"},
		); err != nil {
			t.Fatalf("DeleteFriend lifecycle %d: %v", lifecycle, err)
		}
	}
	if len(workspaces.created) != 3 {
		t.Fatalf("created Workspaces = %#v, want three incarnations", workspaces.created)
	}
	if workspaceNames[0] == workspaceNames[1] ||
		workspaceNames[0] == workspaceNames[2] ||
		workspaceNames[1] == workspaceNames[2] {
		t.Fatalf("Workspace incarnations are not unique: %v", workspaceNames)
	}
	if len(workspaces.retired) != 2 ||
		workspaces.retired[0] != "id-"+workspaceNames[0] ||
		workspaces.retired[1] != "id-"+workspaceNames[1] {
		t.Fatalf(
			"retired Workspaces = %v, want first two incarnations %v",
			workspaces.retired,
			workspaceNames[:2],
		)
	}
}

func TestAddFriendAfterDeletionKeepsRetiredWorkspaceIsolated(t *testing.T) {
	ctx := t.Context()
	workspaces := &recordingWorkspaceService{}
	s := newTestServer()
	s.Workspaces = workspaces
	token, err := s.CreateFriendInviteToken(
		ctx,
		"peer-b",
		rpcapi.FriendInviteTokenCreateRequest{},
	)
	if err != nil {
		t.Fatalf("CreateFriendInviteToken: %v", err)
	}
	first, err := s.AddFriend(
		ctx,
		"peer-a",
		rpcapi.FriendAddRequest{InviteToken: token.InviteToken},
	)
	if err != nil {
		t.Fatalf("AddFriend first lifecycle: %v", err)
	}
	firstWorkspace := socialutil.StringValue(first.WorkspaceName)
	if _, err := s.DeleteFriend(
		ctx,
		"peer-a",
		rpcapi.FriendDeleteRequest{Name: "peer-b"},
	); err != nil {
		t.Fatalf("DeleteFriend first lifecycle: %v", err)
	}
	second, err := s.AddFriend(
		ctx,
		"peer-a",
		rpcapi.FriendAddRequest{InviteToken: token.InviteToken},
	)
	if err != nil {
		t.Fatalf("AddFriend second lifecycle: %v", err)
	}
	secondWorkspace := socialutil.StringValue(second.WorkspaceName)
	if firstWorkspace == secondWorkspace {
		t.Fatalf("re-added Friend reused Workspace %q", firstWorkspace)
	}
	if len(workspaces.retired) != 1 || workspaces.retired[0] != "id-"+firstWorkspace {
		t.Fatalf(
			"retired Workspaces = %v, want unchanged old Workspace %q",
			workspaces.retired,
			firstWorkspace,
		)
	}
	relationID := socialutil.RelationID("peer-a", "peer-b")
	receipt, err := readRetirementReceipt(ctx, s.Friends, relationID)
	if err != nil {
		t.Fatalf("readRetirementReceipt: %v", err)
	}
	if receipt.WorkspaceName != firstWorkspace || receipt.WorkspaceID != "id-"+firstWorkspace {
		t.Fatalf("retirement receipt Workspace = %#v, want ID %q and name %q", receipt, "id-"+firstWorkspace, firstWorkspace)
	}
	receiptData, err := s.Friends.Get(ctx, retirementReceiptKey(relationID))
	if err != nil {
		t.Fatalf("read retirement receipt data: %v", err)
	}
	var receiptFields map[string]json.RawMessage
	if err := json.Unmarshal(receiptData, &receiptFields); err != nil {
		t.Fatalf("decode retirement receipt data: %v", err)
	}
	if _, exists := receiptFields["relationship"]; exists {
		t.Fatalf("retirement receipt retained full relationship: %s", receiptData)
	}
	duplicate, err := s.AddFriend(
		ctx,
		"peer-a",
		rpcapi.FriendAddRequest{InviteToken: token.InviteToken},
	)
	if err != nil {
		t.Fatalf("AddFriend duplicate second lifecycle: %v", err)
	}
	if socialutil.StringValue(duplicate.WorkspaceName) != secondWorkspace ||
		len(workspaces.created) != 2 {
		t.Fatalf(
			"duplicate AddFriend = %#v, created Workspaces = %#v",
			duplicate,
			workspaces.created,
		)
	}
}

func TestReconcileCreationIntentReusesWorkspaceIdentity(t *testing.T) {
	ctx := t.Context()
	baseStore := kv.NewMemory(nil)
	friendStore := &toggleBatchMutateStore{Store: baseStore, fail: true}
	workspaces := &recordingWorkspaceService{}
	s := newTestServer()
	s.Friends = friendStore
	s.Workspaces = workspaces
	resolverCalls := 0
	s.RuntimeProfileForOwner = func(
		context.Context,
		string,
	) (apitypes.RuntimeProfile, error) {
		resolverCalls++
		return apitypes.RuntimeProfile{Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{
				System: apitypes.RuntimeProfileSystemWorkflows{
					FriendChatroom: "durable-friend-chatroom",
				},
			},
		}}, nil
	}

	if _, err := s.AdminCreateFriend(ctx, "peer-a", "peer-b"); err == nil {
		t.Fatal("AdminCreateFriend commit failure error = nil")
	}
	relationID := socialutil.RelationID("peer-a", "peer-b")
	intent, err := readCreationIntent(ctx, friendStore, relationID)
	if err != nil {
		t.Fatalf("readCreationIntent after commit failure: %v", err)
	}
	if len(workspaces.created) != 1 || workspaces.created[0].Name != intent.Workspace {
		t.Fatalf("created Workspaces = %#v, intent = %#v", workspaces.created, intent)
	}
	if _, err := s.GetFriendRelation(ctx, "peer-a", "peer-b"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("GetFriendRelation before reconciliation error = %v, want not found", err)
	}
	if _, err := s.AdminCreateFriend(ctx, "peer-b", "peer-a"); err == nil ||
		!strings.Contains(err.Error(), "invalid Friend creation intent") {
		t.Fatalf("AdminCreateFriend conflicting owner error = %v", err)
	}

	friendStore.fail = false
	restarted := &Server{
		Friends:    friendStore,
		Workspaces: workspaces,
		Now:        s.Now,
		NotifyPeer: s.NotifyPeer,
		NewID:      func() string { return "must-not-be-used" },
		RuntimeProfileForOwner: func(
			context.Context,
			string,
		) (apitypes.RuntimeProfile, error) {
			return apitypes.RuntimeProfile{}, errors.New(
				"reconciliation must reuse the persisted Workflow",
			)
		},
	}
	if err := restarted.ReconcileCreationIntents(ctx); err != nil {
		t.Fatalf("ReconcileCreationIntents: %v", err)
	}
	if resolverCalls != 1 {
		t.Fatalf("runtime profile resolver calls = %d, want 1", resolverCalls)
	}
	for _, pair := range [][2]string{{"peer-a", "peer-b"}, {"peer-b", "peer-a"}} {
		item, err := restarted.GetFriendRelation(ctx, pair[0], pair[1])
		if err != nil {
			t.Fatalf("GetFriendRelation(%s,%s): %v", pair[0], pair[1], err)
		}
		if socialutil.StringValue(item.WorkspaceName) != intent.Workspace {
			t.Fatalf("reconciled Friend = %#v, want Workspace %q", item, intent.Workspace)
		}
	}
	if _, err := readCreationIntent(ctx, friendStore, relationID); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("creation intent after reconciliation error = %v, want not found", err)
	}
	if len(workspaces.created) != 1 {
		t.Fatalf("reconciliation created a second Workspace: %#v", workspaces.created)
	}
}

func TestReconcileCommittedDecisionDoesNotRestoreDeletedRelationship(t *testing.T) {
	ctx := t.Context()
	workspaces := &recordingWorkspaceService{}
	s := newTestServer()
	s.Workspaces = workspaces
	intent, err := s.getOrCreateCreationIntent(
		ctx,
		s.Friends,
		"peer-a",
		"peer-b",
		"peer-a",
	)
	if err != nil {
		t.Fatalf("getOrCreateCreationIntent: %v", err)
	}
	workspace, err := s.ensureCreationWorkspace(ctx, intent)
	if err != nil {
		t.Fatalf("ensureCreationWorkspace: %v", err)
	}
	if _, err := s.commitFriendCreation(
		ctx,
		s.Friends,
		"peer-a",
		"peer-b",
		intent,
		workspace,
	); err != nil {
		t.Fatalf("commitFriendCreation: %v", err)
	}
	if err := socialutil.WriteJSON(
		ctx,
		s.Friends,
		creationIntentKey(intent.RelationID),
		intent,
	); err != nil {
		t.Fatalf("restore creation intent to simulate post-commit crash: %v", err)
	}
	if _, err := s.DeleteFriend(
		ctx,
		"peer-a",
		rpcapi.FriendDeleteRequest{Name: "peer-b"},
	); err != nil {
		t.Fatalf("DeleteFriend: %v", err)
	}

	if err := s.ReconcileCreationIntents(ctx); err != nil {
		t.Fatalf("ReconcileCreationIntents: %v", err)
	}
	if len(workspaces.created) != 1 {
		t.Fatalf("reconciliation restored deleted Workspace: %#v", workspaces.created)
	}
	if _, err := readCreationIntent(
		ctx,
		s.Friends,
		intent.RelationID,
	); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("creation intent error = %v, want not found", err)
	}
	for _, pair := range [][2]string{{"peer-a", "peer-b"}, {"peer-b", "peer-a"}} {
		if _, err := s.GetFriendRelation(
			ctx,
			pair[0],
			pair[1],
		); !errors.Is(err, kv.ErrNotFound) {
			t.Fatalf(
				"GetFriendRelation(%s,%s) error = %v, want not found",
				pair[0],
				pair[1],
				err,
			)
		}
	}
}

func TestDeleteFriendCancelsFailedCreationBeforeReconciliation(t *testing.T) {
	ctx := t.Context()
	friendStore := &toggleBatchMutateStore{
		Store: kv.NewMemory(nil),
		fail:  true,
	}
	workspaces := &recordingWorkspaceService{}
	s := newTestServer()
	s.Friends = friendStore
	s.Workspaces = workspaces
	var notifications []friendNotification
	s.NotifyPeer = func(
		_ context.Context,
		recipient string,
		event *eventpb.PeerEvent,
	) {
		notifications = append(
			notifications,
			friendNotification{recipient: recipient, event: event},
		)
	}

	if _, err := s.AdminCreateFriend(ctx, "peer-a", "peer-b"); err == nil {
		t.Fatal("AdminCreateFriend commit failure error = nil")
	}
	relationID := socialutil.RelationID("peer-a", "peer-b")
	creation, err := readCreationIntent(ctx, friendStore, relationID)
	if err != nil {
		t.Fatalf("readCreationIntent: %v", err)
	}
	if len(workspaces.created) != 1 ||
		workspaces.created[0].Name != creation.Workspace {
		t.Fatalf("created Workspaces = %#v, intent = %#v", workspaces.created, creation)
	}

	friendStore.fail = false
	deleted, err := s.DeleteFriend(
		ctx,
		"peer-a",
		rpcapi.FriendDeleteRequest{Name: "peer-b"},
	)
	if err != nil {
		t.Fatalf("DeleteFriend pending creation: %v", err)
	}
	if socialutil.StringValue(deleted.WorkspaceName) != creation.Workspace {
		t.Fatalf("DeleteFriend pending creation = %#v, want %q", deleted, creation.Workspace)
	}
	if len(workspaces.deleted) != 1 ||
		workspaces.deleted[0] != creation.Workspace ||
		len(workspaces.retired) != 0 {
		t.Fatalf(
			"Workspace cancellation calls: deleted=%v retired=%v",
			workspaces.deleted,
			workspaces.retired,
		)
	}
	if _, err := readCreationIntent(ctx, friendStore, relationID); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("creation intent after delete error = %v, want not found", err)
	}
	receipt, err := readRetirementReceipt(ctx, friendStore, relationID)
	if err != nil {
		t.Fatalf("readRetirementReceipt: %v", err)
	}
	if receipt.WorkspaceName != creation.Workspace {
		t.Fatalf("retirement receipt = %#v, want Workspace %q", receipt, creation.Workspace)
	}
	newerRetirement := retirementIntent{
		RelationID:    relationID,
		FirstPeer:     "peer-a",
		SecondPeer:    "peer-b",
		WorkspaceID:   "id-newer-direct-workspace",
		WorkspaceName: "newer-direct-workspace",
		DeletedAt:     s.now().Add(time.Second),
	}
	if err := socialutil.WriteJSON(
		ctx,
		friendStore,
		retirementIntentKey(relationID),
		newerRetirement,
	); err != nil {
		t.Fatalf("seed newer retirement intent: %v", err)
	}
	duplicate, err := s.cancelFriendCreation(
		ctx,
		friendStore,
		"peer-a",
		relationID,
		creation,
	)
	if err != nil {
		t.Fatalf("cancelFriendCreation completed retry: %v", err)
	}
	if socialutil.StringValue(duplicate.WorkspaceName) != creation.Workspace ||
		len(workspaces.deleted) != 1 {
		t.Fatalf(
			"completed cancellation retry = %#v, deleted Workspaces = %v",
			duplicate,
			workspaces.deleted,
		)
	}
	stillPending, err := readRetirementIntent(ctx, friendStore, relationID)
	if err != nil {
		t.Fatalf("read newer retirement intent: %v", err)
	}
	if stillPending.WorkspaceID != newerRetirement.WorkspaceID ||
		stillPending.WorkspaceName != newerRetirement.WorkspaceName ||
		stillPending.DeletedAt != newerRetirement.DeletedAt {
		t.Fatalf(
			"newer retirement intent changed: got %#v, want %#v",
			stillPending,
			newerRetirement,
		)
	}
	if err := s.ReconcileCreationIntents(ctx); err != nil {
		t.Fatalf("ReconcileCreationIntents after cancellation: %v", err)
	}
	for _, pair := range [][2]string{{"peer-a", "peer-b"}, {"peer-b", "peer-a"}} {
		if _, err := s.GetFriendRelation(ctx, pair[0], pair[1]); !errors.Is(err, kv.ErrNotFound) {
			t.Fatalf(
				"GetFriendRelation(%s,%s) after cancellation error = %v, want not found",
				pair[0],
				pair[1],
				err,
			)
		}
	}
	if len(notifications) != 0 {
		t.Fatalf("pending creation cancellation notifications = %#v, want none", notifications)
	}
}

func TestCreationCommitCannotWinAfterConcurrentCancellation(t *testing.T) {
	ctx := t.Context()
	baseStore := kv.NewMemory(nil)
	commitStarted := make(chan struct{})
	releaseCommit := make(chan struct{})
	creatorStore := &blockingCreationDecisionStore{
		Store:   baseStore,
		state:   creationDecisionCommitted,
		started: commitStarted,
		release: releaseCommit,
	}
	workspaces := &recordingWorkspaceService{}
	creator := newTestServer()
	creator.Friends = creatorStore
	creator.Workspaces = workspaces
	deleter := newTestServer()
	deleter.Friends = baseStore
	deleter.Workspaces = workspaces
	deleter.NewID = func() string { return "incarnation-b" }

	intent, err := creator.getOrCreateCreationIntent(
		ctx,
		creatorStore,
		"peer-a",
		"peer-b",
		"peer-a",
	)
	if err != nil {
		t.Fatalf("getOrCreateCreationIntent: %v", err)
	}
	workspace, err := creator.ensureCreationWorkspace(ctx, intent)
	if err != nil {
		t.Fatalf("ensureCreationWorkspace: %v", err)
	}

	commitResult := make(chan error, 1)
	go func() {
		_, err := creator.commitFriendCreation(
			ctx,
			creatorStore,
			"peer-a",
			"peer-b",
			intent,
			workspace,
		)
		commitResult <- err
	}()
	<-commitStarted

	deleted, err := deleter.DeleteFriend(
		ctx,
		"peer-a",
		rpcapi.FriendDeleteRequest{Name: "peer-b"},
	)
	if err != nil {
		t.Fatalf("DeleteFriend during blocked creation commit: %v", err)
	}
	if socialutil.StringValue(deleted.WorkspaceName) != intent.Workspace {
		t.Fatalf("DeleteFriend = %#v, want Workspace %q", deleted, intent.Workspace)
	}
	nextIntent, err := deleter.getOrCreateCreationIntent(
		ctx,
		baseStore,
		"peer-a",
		"peer-b",
		"peer-a",
	)
	if err != nil {
		t.Fatalf("getOrCreateCreationIntent for re-add: %v", err)
	}
	if nextIntent.IncarnationID == intent.IncarnationID {
		t.Fatalf("re-add reused incarnation: old=%#v new=%#v", intent, nextIntent)
	}
	close(releaseCommit)
	if err := <-commitResult; !errors.Is(err, errFriendCreationCancelled) {
		t.Fatalf("commitFriendCreation error = %v, want cancellation", err)
	}

	decision, err := readCreationDecision(ctx, baseStore, intent)
	if err != nil {
		t.Fatalf("readCreationDecision: %v", err)
	}
	if decision.State != creationDecisionCancelled {
		t.Fatalf("creation decision = %#v, want cancelled", decision)
	}
	currentIntent, err := readCreationIntent(
		ctx,
		baseStore,
		intent.RelationID,
	)
	if err != nil {
		t.Fatalf("read re-add creation intent: %v", err)
	}
	if currentIntent.IncarnationID != nextIntent.IncarnationID ||
		currentIntent.Workspace != nextIntent.Workspace {
		t.Fatalf(
			"losing creator changed re-add intent: got %#v, want %#v",
			currentIntent,
			nextIntent,
		)
	}
	for _, pair := range [][2]string{{"peer-a", "peer-b"}, {"peer-b", "peer-a"}} {
		if _, err := deleter.GetFriendRelation(
			ctx,
			pair[0],
			pair[1],
		); !errors.Is(err, kv.ErrNotFound) {
			t.Fatalf(
				"GetFriendRelation(%s,%s) error = %v, want not found",
				pair[0],
				pair[1],
				err,
			)
		}
	}
	if len(workspaces.deleted) < 1 ||
		workspaces.deleted[len(workspaces.deleted)-1] != intent.Workspace {
		t.Fatalf("deleted Workspaces = %v, want %q", workspaces.deleted, intent.Workspace)
	}
}

func TestConcurrentCancellationRetiresCreationThatAlreadyCommitted(t *testing.T) {
	ctx := t.Context()
	baseStore := kv.NewMemory(nil)
	cancellationStarted := make(chan struct{})
	releaseCancellation := make(chan struct{})
	deleterStore := &blockingCreationDecisionStore{
		Store:   baseStore,
		state:   creationDecisionCancelled,
		started: cancellationStarted,
		release: releaseCancellation,
	}
	workspaces := &recordingWorkspaceService{}
	creator := newTestServer()
	creator.Friends = baseStore
	creator.Workspaces = workspaces
	deleter := newTestServer()
	deleter.Friends = deleterStore
	deleter.Workspaces = workspaces

	intent, err := creator.getOrCreateCreationIntent(
		ctx,
		baseStore,
		"peer-a",
		"peer-b",
		"peer-a",
	)
	if err != nil {
		t.Fatalf("getOrCreateCreationIntent: %v", err)
	}
	workspace, err := creator.ensureCreationWorkspace(ctx, intent)
	if err != nil {
		t.Fatalf("ensureCreationWorkspace: %v", err)
	}

	deleteResult := make(chan error, 1)
	go func() {
		_, err := deleter.cancelFriendCreation(
			ctx,
			deleterStore,
			"peer-a",
			intent.RelationID,
			intent,
		)
		deleteResult <- err
	}()
	<-cancellationStarted

	if _, err := creator.commitFriendCreation(
		ctx,
		baseStore,
		"peer-a",
		"peer-b",
		intent,
		workspace,
	); err != nil {
		t.Fatalf("commitFriendCreation before cancellation: %v", err)
	}
	close(releaseCancellation)
	if err := <-deleteResult; err != nil {
		t.Fatalf("cancelFriendCreation after committed decision: %v", err)
	}

	decision, err := readCreationDecision(ctx, baseStore, intent)
	if err != nil {
		t.Fatalf("readCreationDecision: %v", err)
	}
	if decision.State != creationDecisionCommitted {
		t.Fatalf("creation decision = %#v, want committed", decision)
	}
	for _, pair := range [][2]string{{"peer-a", "peer-b"}, {"peer-b", "peer-a"}} {
		if _, err := deleter.GetFriendRelation(
			ctx,
			pair[0],
			pair[1],
		); !errors.Is(err, kv.ErrNotFound) {
			t.Fatalf(
				"GetFriendRelation(%s,%s) error = %v, want not found",
				pair[0],
				pair[1],
				err,
			)
		}
	}
	receipt, err := readRetirementReceipt(ctx, baseStore, intent.RelationID)
	if err != nil {
		t.Fatalf("readRetirementReceipt: %v", err)
	}
	if receipt.WorkspaceName != intent.Workspace ||
		receipt.WorkspaceID != "id-"+intent.Workspace ||
		len(workspaces.retired) != 1 ||
		workspaces.retired[0] != "id-"+intent.Workspace ||
		len(workspaces.deleted) != 0 {
		t.Fatalf(
			"committed-then-deleted lifecycle: receipt=%#v retired=%v deleted=%v",
			receipt,
			workspaces.retired,
			workspaces.deleted,
		)
	}
}

func TestFriendCreationRejectsIncompleteReciprocalRows(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	relationID := socialutil.RelationID("peer-a", "peer-b")
	peer := "peer-b"
	if err := socialutil.WriteJSON(
		ctx,
		s.Friends,
		socialutil.FriendKey("peer-a", relationID),
		rpcapi.FriendObject{PeerPublicKey: &peer},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdminCreateFriend(ctx, "peer-a", "peer-b"); err == nil ||
		!strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("AdminCreateFriend incomplete relationship error = %v", err)
	}
}

func TestDeleteFriendWithoutWorkspaceRetirementKeepsRelationship(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	if _, err := s.AdminCreateFriendResource(ctx, socialutil.RelationID("peer-a", "peer-b"), "peer-a", "peer-b"); err != nil {
		t.Fatalf("AdminCreateFriendResource: %v", err)
	}
	s.Workspaces = nil

	if _, err := s.DeleteFriend(ctx, "peer-a", rpcapi.FriendDeleteRequest{Name: "peer-b"}); err == nil ||
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
	if _, err := s.AdminCreateFriendResource(ctx, socialutil.RelationID("peer-a", "peer-b"), "peer-a", "peer-b"); err != nil {
		t.Fatalf("AdminCreateFriendResource: %v", err)
	}
	s.Workspaces = workspaces
	s.Friends = failingBatchMutateStore{Store: s.Friends}

	if _, err := s.DeleteFriend(ctx, "peer-a", rpcapi.FriendDeleteRequest{Name: "peer-b"}); err == nil {
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
	if _, err := s.AddFriend(ctx, "peer-a", rpcapi.FriendAddRequest{InviteToken: created.InviteToken}); !errors.Is(err, ErrInviteTokenUnavailable) {
		t.Fatalf("AddFriend expired token error = %v, want unavailable", err)
	}
	got, err := s.GetFriendInviteToken(ctx, "peer-b", rpcapi.FriendInviteTokenGetRequest{})
	if err != nil {
		t.Fatalf("GetFriendInviteToken expired: %v", err)
	}
	if got.InviteToken != nil || got.ExpiresAt != nil {
		t.Fatalf("expired token response = %#v, want no token fields", got)
	}
	if _, err := s.InviteTokens.Get(ctx, socialutil.FriendInviteTokenKey("peer-b")); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("expired invite token cleanup error = %v, want not found", err)
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

	created, err := s.AdminCreateFriendResource(ctx, socialutil.RelationID("peer-c", "peer-d"), " peer-c ", "peer-d")
	if err != nil {
		t.Fatalf("AdminCreateFriendResource: %v", err)
	}
	if created.OwnerPublicKey != "peer-c" || created.PeerPublicKey != "peer-d" || created.Id != "peer-c:peer-d" {
		t.Fatalf("AdminCreateFriendResource row = %#v", created)
	}
	if created.WorkspaceId == "" || !strings.HasPrefix(created.WorkspaceId, "id-social-direct-") {
		t.Fatalf("AdminCreateFriendResource workspace ID = %q, want canonical ID", created.WorkspaceId)
	}
	if _, err := s.AdminCreateFriendResource(ctx, created.Id, "peer-c", "peer-d"); !errors.Is(err, socialutil.ErrResourceAlreadyExists) {
		t.Fatalf("AdminCreateFriendResource duplicate error = %v, want conflict", err)
	}
	page, err := s.AdminListFriends(ctx, new("malformed/cursor/value"), new(10))
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
	got, err := s.AdminGetFriend(ctx, "peer-c", created.Id)
	if err != nil {
		t.Fatalf("AdminGetFriend by canonical ID: %v", err)
	}
	if got.Id != created.Id {
		t.Fatalf("AdminGetFriend ID = %q, want %q", got.Id, created.Id)
	}
	if _, err := s.GetFriendRelation(ctx, "peer-c", created.Id); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("GetFriendRelation with canonical ID error = %v, want not found", err)
	}
	if _, err := s.DeleteFriend(ctx, "peer-c", rpcapi.FriendDeleteRequest{Name: created.Id}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("DeleteFriend with canonical ID error = %v, want not found", err)
	}
	if _, err := s.AdminGetFriend(ctx, "peer-c", created.Id); err != nil {
		t.Fatalf("Peer canonical-ID delete changed Admin relationship: %v", err)
	}
	deleted, err := s.AdminDeleteFriend(ctx, "peer-c", created.Id)
	if err != nil {
		t.Fatalf("AdminDeleteFriend by canonical ID: %v", err)
	}
	if deleted.Id != created.Id {
		t.Fatalf("AdminDeleteFriend ID = %q, want %q", deleted.Id, created.Id)
	}
}

func TestPeerRetirementSnapshotsAndRetiresReciprocalFriend(t *testing.T) {
	s := newTestServer()
	created, err := s.AdminCreateFriendResource(t.Context(), socialutil.RelationID("peer-a", "peer-b"), "peer-a", "peer-b")
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := s.AdminCreateFriendResource(t.Context(), socialutil.RelationID("peer-c", "peer-d"), "peer-c", "peer-d")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.SnapshotPeerFriends(t.Context(), "peer-a")
	if err != nil || len(snapshot) != 1 || snapshot[0].RelationID != created.Id || snapshot[0].WorkspaceID != created.WorkspaceId {
		t.Fatalf("SnapshotPeerFriends() = %#v, %v", snapshot, err)
	}
	if err := s.RetirePeerFriend(t.Context(), "peer-a", snapshot[0]); err != nil {
		t.Fatal(err)
	}
	if err := s.RetirePeerFriend(t.Context(), "peer-a", snapshot[0]); err != nil {
		t.Fatalf("replayed RetirePeerFriend() error = %v", err)
	}
	for _, owner := range []string{"peer-a", "peer-b"} {
		if _, err := s.AdminGetFriend(t.Context(), owner, created.Id); !errors.Is(err, kv.ErrNotFound) {
			t.Fatalf("retired Friend owner %q error = %v", owner, err)
		}
	}
	if _, err := s.AdminGetFriend(t.Context(), "peer-c", foreign.Id); err != nil {
		t.Fatalf("foreign Friend removed: %v", err)
	}
}

func TestPeerRetirementSnapshotOnlyBlocksRelationsForTargetPeer(t *testing.T) {
	s := newTestServer()
	var nextID atomic.Int32
	s.NewID = func() string { return fmt.Sprintf("id-concurrent-%d", nextID.Add(1)) }
	if _, err := s.AdminCreateFriend(t.Context(), "peer-a", "peer-b"); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	s.Friends = &blockingFriendListStore{
		Store: s.Friends, prefix: socialutil.OwnerPrefix(socialutil.FriendsRoot, "peer-a"),
		entered: entered, release: release,
	}
	secondary := *s
	snapshotDone := make(chan error, 1)
	go func() {
		_, err := s.SnapshotPeerFriends(t.Context(), "peer-a")
		snapshotDone <- err
	}()
	<-entered

	unrelatedDone := make(chan error, 1)
	go func() {
		_, err := secondary.AdminCreateFriend(t.Context(), "peer-c", "peer-d")
		unrelatedDone <- err
	}()
	targetDone := make(chan error, 1)
	go func() {
		_, err := secondary.AdminCreateFriend(t.Context(), "peer-a", "peer-e")
		targetDone <- err
	}()

	select {
	case err := <-unrelatedDone:
		if err != nil {
			t.Fatalf("unrelated Friend mutation: %v", err)
		}
	case <-time.After(time.Second):
		close(release)
		<-snapshotDone
		<-unrelatedDone
		<-targetDone
		t.Fatal("unrelated Friend mutation was blocked by peer-a retirement snapshot")
	}
	select {
	case err := <-targetDone:
		close(release)
		<-snapshotDone
		t.Fatalf("peer-a Friend mutation crossed accepted snapshot: %v", err)
	default:
	}
	close(release)
	if err := <-snapshotDone; err != nil {
		t.Fatalf("SnapshotPeerFriends(): %v", err)
	}
	if err := <-targetDone; err != nil {
		t.Fatalf("target Friend mutation after snapshot: %v", err)
	}
}

func TestFriendCreationFailsClosedWhenTargetUnavailable(t *testing.T) {
	s := newTestServer()
	wantErr := errors.New("PEER_PENDING_DELETION")
	s.PeerAvailability = func(_ context.Context, publicKey string) error {
		if publicKey == "peer-b" {
			return wantErr
		}
		return nil
	}
	_, err := s.AdminCreateFriendResource(t.Context(), socialutil.RelationID("peer-a", "peer-b"), "peer-a", "peer-b")
	if !errors.Is(err, wantErr) {
		t.Fatalf("AdminCreateFriendResource() error = %v, want target fence", err)
	}
	if len(s.Workspaces.(*recordingWorkspaceService).created) != 0 {
		t.Fatalf("Workspaces created before target fence = %#v", s.Workspaces.(*recordingWorkspaceService).created)
	}
}

func TestFriendDeletionRejectsUnavailablePeerButRetirementBypassesFence(t *testing.T) {
	s := newTestServer()
	created, err := s.AdminCreateFriendResource(t.Context(), socialutil.RelationID("peer-a", "peer-b"), "peer-a", "peer-b")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.SnapshotPeerFriends(t.Context(), "peer-a")
	if err != nil || len(snapshot) != 1 {
		t.Fatalf("SnapshotPeerFriends() = %#v, %v", snapshot, err)
	}
	wantErr := errors.New("PEER_PENDING_DELETION")
	s.PeerAvailability = func(_ context.Context, publicKey string) error {
		if publicKey == "peer-a" {
			return wantErr
		}
		return nil
	}
	if _, err := s.AdminDeleteFriend(t.Context(), "peer-a", created.Id); !errors.Is(err, wantErr) {
		t.Fatalf("AdminDeleteFriend() error = %v, want Peer fence", err)
	}
	if _, err := s.AdminGetFriend(t.Context(), "peer-a", created.Id); err != nil {
		t.Fatalf("fenced deletion changed retained Friend: %v", err)
	}
	if err := s.RetirePeerFriend(t.Context(), "peer-a", snapshot[0]); err != nil {
		t.Fatalf("RetirePeerFriend() error = %v", err)
	}
}

func TestConfigurationAndValidationErrors(t *testing.T) {
	ctx := context.Background()
	empty := &Server{}
	if _, err := empty.CreateFriendInviteToken(ctx, "peer-a", rpcapi.FriendInviteTokenCreateRequest{}); err == nil {
		t.Fatal("CreateFriendInviteToken without store error = nil")
	}
	if _, err := empty.AddFriend(ctx, "peer-a", rpcapi.FriendAddRequest{InviteToken: "token"}); !errors.Is(err, ErrInviteTokenLookupFailed) {
		t.Fatalf("AddFriend without store error = %v, want lookup failed", err)
	}
	if _, err := empty.ListFriends(ctx, "peer-a", rpcapi.FriendListRequest{}); err == nil {
		t.Fatal("ListFriends without store error = nil")
	}
	if _, err := empty.AdminListFriends(ctx, nil, nil); err == nil {
		t.Fatal("AdminListFriends without store error = nil")
	}
	if _, err := empty.AdminCreateFriendResource(ctx, socialutil.RelationID("peer-a", "peer-b"), "peer-a", "peer-b"); err == nil {
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
	if _, err := s.AddFriend(ctx, "peer-a", rpcapi.FriendAddRequest{}); !errors.Is(err, ErrInviteTokenRequired) {
		t.Fatalf("AddFriend empty token error = %v, want required", err)
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
	wantErr := errors.New("forced list failure")
	s.InviteTokens = failingGetStore{Store: s.InviteTokens, err: wantErr}

	_, err := s.AddFriend(ctx, "peer-a", rpcapi.FriendAddRequest{InviteToken: "token"})
	if !errors.Is(err, ErrInviteTokenLookupFailed) || !errors.Is(err, wantErr) {
		t.Fatalf("AddFriend error = %v, want lookup failure wrapping store cause", err)
	}
	assertNoFriendCreationState(t, s.Friends)
	if created := s.Workspaces.(*recordingWorkspaceService).created; len(created) != 0 {
		t.Fatalf("created Workspaces = %#v, want none", created)
	}
}

func TestAddFriendRejectsCorruptInviteTokenRecords(t *testing.T) {
	tests := []struct {
		name  string
		value []byte
		token string
	}{
		{name: "invalid JSON", value: []byte("{"), token: "token"},
		{
			name: "invalid active timestamp",
			value: mustJSON(t, inviteTokenRecord{
				PeerPublicKey: "peer-b",
				InviteToken:   "different-token",
				ExpiresAt:     time.Date(2026, 6, 13, 0, 5, 0, 0, time.UTC),
			}),
			token: "token",
		},
		{
			name: "invalid nonmatching owner",
			value: mustJSON(t, inviteTokenRecord{
				PeerPublicKey: " peer-b ",
				InviteToken:   "different-token",
				CreatedAt:     time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC),
				ExpiresAt:     time.Date(2026, 6, 13, 0, 5, 0, 0, time.UTC),
			}),
			token: "token",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := newTestServer()
			if err := s.InviteTokens.Set(t.Context(), socialutil.FriendInviteTokenKey("peer-b"), test.value); err != nil {
				t.Fatalf("seed invite token: %v", err)
			}
			if _, err := s.AddFriend(t.Context(), "peer-a", rpcapi.FriendAddRequest{InviteToken: test.token}); !errors.Is(err, ErrInviteTokenLookupFailed) {
				t.Fatalf("AddFriend() error = %v, want lookup failed", err)
			}
			assertNoFriendCreationState(t, s.Friends)
			if created := s.Workspaces.(*recordingWorkspaceService).created; len(created) != 0 {
				t.Fatalf("created Workspaces = %#v, want none", created)
			}
		})
	}
}

func TestAddFriendReportsExpiredInviteTokenCleanupFailure(t *testing.T) {
	s := newTestServer()
	wantErr := errors.New("forced delete failure")
	s.InviteTokens = failingDeleteStore{Store: s.InviteTokens, err: wantErr}
	record := inviteTokenRecord{
		PeerPublicKey: "peer-b",
		InviteToken:   "expired",
		CreatedAt:     time.Date(2026, 6, 12, 23, 0, 0, 0, time.UTC),
		ExpiresAt:     time.Date(2026, 6, 12, 23, 5, 0, 0, time.UTC),
	}
	if err := socialutil.WriteJSON(t.Context(), s.InviteTokens, socialutil.FriendInviteTokenKey("peer-b"), record); err != nil {
		t.Fatalf("seed expired invite token: %v", err)
	}

	_, err := s.AddFriend(t.Context(), "peer-a", rpcapi.FriendAddRequest{InviteToken: record.InviteToken})
	if !errors.Is(err, ErrInviteTokenLookupFailed) || !errors.Is(err, wantErr) {
		t.Fatalf("AddFriend() error = %v, want lookup failure wrapping delete cause", err)
	}
	assertNoFriendCreationState(t, s.Friends)
	if created := s.Workspaces.(*recordingWorkspaceService).created; len(created) != 0 {
		t.Fatalf("created Workspaces = %#v, want none", created)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal(): %v", err)
	}
	return data
}

func assertNoFriendCreationState(t *testing.T, store kv.Store) {
	t.Helper()
	for _, root := range []kv.Key{
		socialutil.FriendsRoot,
		creationIntentsRoot,
		creationDecisionsRoot,
		workspaceBindingsRoot,
	} {
		for entry, err := range store.List(t.Context(), root) {
			if err != nil {
				t.Fatalf("list %s: %v", root, err)
			}
			t.Fatalf("unexpected Friend creation state under %s: %s", root, entry.Key)
		}
	}
}

func newTestServer() *Server {
	now := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	nextID := 0
	return &Server{
		InviteTokens: kv.NewMemory(nil),
		Friends:      kv.NewMemory(nil),
		Workspaces:   &recordingWorkspaceService{},
		RuntimeProfileForOwner: func(context.Context, string) (apitypes.RuntimeProfile, error) {
			return apitypes.RuntimeProfile{Spec: apitypes.RuntimeProfileSpec{
				Workflows: apitypes.RuntimeProfileWorkflows{
					System: apitypes.RuntimeProfileSystemWorkflows{
						FriendChatroom: "friend-chatroom",
					},
				},
			}}, nil
		},
		Now: func() time.Time { return now },
		NewID: func() string {
			nextID++
			return "id-" + string(rune('a'+nextID-1))
		},
	}
}

type failingBatchSetStore struct {
	kv.Store
}

type blockingFriendListStore struct {
	kv.Store
	prefix           kv.Key
	entered, release chan struct{}
	once             sync.Once
}

func (s *blockingFriendListStore) List(ctx context.Context, prefix kv.Key) iter.Seq2[kv.Entry, error] {
	entries := s.Store.List(ctx, prefix)
	return func(yield func(kv.Entry, error) bool) {
		if slices.Equal(prefix, s.prefix) {
			s.once.Do(func() {
				close(s.entered)
				<-s.release
			})
		}
		entries(yield)
	}
}

func (s *blockingFriendListStore) CreateIfAbsent(ctx context.Context, guard kv.Entry, entries []kv.Entry) ([]byte, bool, error) {
	return kv.CreateIfAbsent(ctx, s.Store, guard, entries)
}

func (s *blockingFriendListStore) CreateIfAllAbsent(ctx context.Context, guards []kv.Entry, entries []kv.Entry) (kv.Key, []byte, bool, error) {
	return kv.CreateIfAllAbsent(ctx, s.Store, guards, entries)
}

func (s *blockingFriendListStore) CompareAndMutate(ctx context.Context, guard kv.Key, expected []byte, entries []kv.Entry, keys []kv.Key) (bool, error) {
	return kv.CompareAndMutate(ctx, s.Store, guard, expected, entries, keys)
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

type toggleBatchMutateStore struct {
	kv.Store
	fail bool
}

func (s *toggleBatchMutateStore) BatchMutate(
	ctx context.Context,
	entries []kv.Entry,
	keys []kv.Key,
) error {
	if s.fail {
		return errors.New("forced batch mutate failure")
	}
	return s.Store.BatchMutate(ctx, entries, keys)
}

func (s *toggleBatchMutateStore) CreateIfAbsent(
	ctx context.Context,
	guard kv.Entry,
	entries []kv.Entry,
) ([]byte, bool, error) {
	if s.fail &&
		len(guard.Key) > 0 &&
		guard.Key[0] == creationDecisionsRoot[0] {
		return nil, false, errors.New("forced creation commit failure")
	}
	return kv.CreateIfAbsent(ctx, s.Store, guard, entries)
}

func (s *toggleBatchMutateStore) CompareAndMutate(
	ctx context.Context,
	guard kv.Key,
	expected []byte,
	entries []kv.Entry,
	keys []kv.Key,
) (bool, error) {
	return kv.CompareAndMutate(ctx, s.Store, guard, expected, entries, keys)
}

type blockingCreationDecisionStore struct {
	kv.Store
	state   string
	started chan struct{}
	release chan struct{}
}

func (s *blockingCreationDecisionStore) CreateIfAbsent(
	ctx context.Context,
	guard kv.Entry,
	entries []kv.Entry,
) ([]byte, bool, error) {
	if len(guard.Key) > 0 && guard.Key[0] == creationDecisionsRoot[0] {
		var decision creationDecision
		if err := json.Unmarshal(guard.Value, &decision); err != nil {
			return nil, false, err
		}
		if decision.State == s.state {
			close(s.started)
			select {
			case <-ctx.Done():
				return nil, false, ctx.Err()
			case <-s.release:
			}
		}
	}
	return kv.CreateIfAbsent(ctx, s.Store, guard, entries)
}

func (s *blockingCreationDecisionStore) CompareAndMutate(
	ctx context.Context,
	guard kv.Key,
	expected []byte,
	entries []kv.Entry,
	keys []kv.Key,
) (bool, error) {
	return kv.CompareAndMutate(ctx, s.Store, guard, expected, entries, keys)
}

type failingGetStore struct {
	kv.Store
	err error
}

func (s failingGetStore) List(context.Context, kv.Key) iter.Seq2[kv.Entry, error] {
	return func(yield func(kv.Entry, error) bool) {
		yield(kv.Entry{}, s.err)
	}
}

type failingDeleteStore struct {
	kv.Store
	err error
}

func (s failingDeleteStore) Delete(context.Context, kv.Key) error {
	return s.err
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
			return apitypes.Workspace{Id: "id-" + body.Name, Name: body.Name, WorkflowId: body.WorkflowId, Parameters: body.Parameters, OwnerPublicKey: &owner, System: &system}, false, nil
		}
	}
	s.created = append(s.created, body)
	system := true
	return apitypes.Workspace{Id: "id-" + body.Name, Name: body.Name, WorkflowId: body.WorkflowId, Parameters: body.Parameters, OwnerPublicKey: &owner, System: &system}, true, nil
}

func (s *recordingWorkspaceService) GetWorkspaceByName(_ context.Context, name string) (apitypes.Workspace, error) {
	for _, existing := range s.created {
		if existing.Name == name {
			return apitypes.Workspace{Id: "id-" + name, Name: name}, nil
		}
	}
	return apitypes.Workspace{}, kv.ErrNotFound
}

func (s *recordingWorkspaceService) DeleteSystemWorkspace(_ context.Context, name string) (apitypes.Workspace, error) {
	s.deleted = append(s.deleted, name)
	return apitypes.Workspace{Name: name}, nil
}

func (s *recordingWorkspaceService) RetireSystemWorkspace(_ context.Context, name string, _ apitypes.ChatRoomMode, _ string) (apitypes.Workspace, error) {
	s.retired = append(s.retired, name)
	return apitypes.Workspace{Name: name}, s.retireErr
}

func (s *recordingWorkspaceService) RetireSystemWorkspaceByID(_ context.Context, id string, _ apitypes.ChatRoomMode, _ string) (apitypes.Workspace, error) {
	s.retired = append(s.retired, id)
	return apitypes.Workspace{Id: id}, s.retireErr
}

func (s *recordingWorkspaceService) DeleteWorkspace(_ context.Context, req adminhttp.DeleteWorkspaceRequestObject) (adminhttp.DeleteWorkspaceResponseObject, error) {
	s.deleted = append(s.deleted, req.Id)
	return adminhttp.DeleteWorkspace200JSONResponse(apitypes.Workspace{Name: req.Id}), nil
}
