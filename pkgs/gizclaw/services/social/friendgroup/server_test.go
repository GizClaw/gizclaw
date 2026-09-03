package friendgroup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/sfu"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	_ "modernc.org/sqlite"
)

type groupNotification struct {
	recipient string
	event     *eventpb.PeerEvent
}

func TestCrossServerFriendGroupJoinSucceedsThroughSharedKV(t *testing.T) {
	ownerKey := giznet.PublicKey{1}
	foreignKey := giznet.PublicKey{2}
	home := newTestServer(t)
	other := &Server{
		Groups: home.Groups, InviteTokens: home.InviteTokens, Members: home.Members, Belongs: home.Belongs,
		RelationshipStore: home.RelationshipStore, Workspaces: &recordingWorkspaceService{},
		SFUURL: home.SFUURL, Now: home.Now, NewID: home.NewID,
	}
	group, err := home.CreateFriendGroup(t.Context(), ownerKey.String(), rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatal(err)
	}
	token, err := home.CreateFriendGroupInviteToken(t.Context(), ownerKey.String(), rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupName: group.Name})
	if err != nil {
		t.Fatal(err)
	}
	joined, err := other.JoinFriendGroup(t.Context(), foreignKey.String(), rpcapi.FriendGroupJoinRequest{Name: "foreign-room", InviteToken: token.InviteToken})
	if err != nil {
		t.Fatalf("JoinFriendGroup() across Servers error = %v", err)
	}
	groupID := mustGroupID(t, home, ownerKey.String(), group.Name)
	if _, err := home.groupMember(t.Context(), groupID, foreignKey.String()); err != nil {
		t.Fatalf("foreign member on home Server error = %v", err)
	}
	binding, err := home.ResolveSFUWorkspaceBindingByName(t.Context(), socialutil.StringValue(joined.Group.WorkspaceName), foreignKey.String())
	if err != nil {
		t.Fatalf("ResolveSFUWorkspaceBindingByName() error = %v", err)
	}
	if binding.Owner != ownerKey.String() || len(binding.Members) != 2 {
		t.Fatalf("shared binding = %#v", binding)
	}
}

func TestFriendGroupMemberLimitRejectsEleventhMember(t *testing.T) {
	ctx := t.Context()
	s := newTestServer(t)
	group, err := s.CreateFriendGroup(ctx, "owner", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatal(err)
	}
	groupID := mustGroupID(t, s, "owner", group.Name)
	for index := 1; index < socialutil.FriendGroupMemberLimit; index++ {
		peer := fmt.Sprintf("member-%02d", index)
		if _, err := s.AddFriendGroupMember(ctx, "owner", rpcapi.FriendGroupMemberAddRequest{
			FriendGroupName: group.Name, PeerPublicKey: peer, MemberName: "room", Role: rpcapi.FriendGroupMemberMutableRole("member"),
		}); err != nil {
			t.Fatalf("AddFriendGroupMember(%s): %v", peer, err)
		}
	}
	members, err := s.listAllMembers(ctx, groupID)
	if err != nil || len(members) != socialutil.FriendGroupMemberLimit {
		t.Fatalf("members = %d, %v, want %d", len(members), err, socialutil.FriendGroupMemberLimit)
	}
	if _, err := s.AddFriendGroupMember(ctx, "owner", rpcapi.FriendGroupMemberAddRequest{
		FriendGroupName: group.Name, PeerPublicKey: "member-11", MemberName: "room", Role: rpcapi.FriendGroupMemberMutableRole("member"),
	}); !errors.Is(err, ErrFriendGroupFull) {
		t.Fatalf("AddFriendGroupMember(11th) error = %v, want %v", err, ErrFriendGroupFull)
	}
	token, err := s.CreateFriendGroupInviteToken(ctx, "owner", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupName: group.Name})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.JoinFriendGroup(ctx, "member-11", rpcapi.FriendGroupJoinRequest{Name: "room", InviteToken: token.InviteToken}); !errors.Is(err, ErrFriendGroupFull) {
		t.Fatalf("JoinFriendGroup(11th) error = %v, want %v", err, ErrFriendGroupFull)
	}
	if active, ok, err := s.activeGroupInviteToken(ctx, s.InviteTokens, groupID); err != nil || !ok || active.InviteToken != token.InviteToken {
		t.Fatalf("invite token after rejected join = %+v, %v, %v, want unchanged", active, ok, err)
	}
	if _, err := s.AdminCreateFriendGroupMember(ctx, groupID, "member-11", "room", rpcapi.FriendGroupMemberRoleMember); !errors.Is(err, ErrFriendGroupFull) {
		t.Fatalf("AdminCreateFriendGroupMember(11th) error = %v, want %v", err, ErrFriendGroupFull)
	}
	if _, err := s.AdminPutFriendGroupMember(ctx, groupID, "member-11", "room", rpcapi.FriendGroupMemberRoleMember); !errors.Is(err, ErrFriendGroupFull) {
		t.Fatalf("AdminPutFriendGroupMember(11th) error = %v, want %v", err, ErrFriendGroupFull)
	}
	if _, err := s.AdminPutFriendGroupMember(ctx, groupID, "member-01", "room", rpcapi.FriendGroupMemberRoleAdmin); err != nil {
		t.Fatalf("AdminPutFriendGroupMember(existing at limit) error = %v", err)
	}
	if _, err := s.groupMember(ctx, groupID, "member-11"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("rejected member error = %v, want not found", err)
	}
}

func TestFriendGroupSFUBindingLifecycle(t *testing.T) {
	ctx := t.Context()
	s := newTestServer(t)
	workspaces := s.Workspaces.(*recordingWorkspaceService)
	group, err := s.CreateFriendGroup(ctx, "owner", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaces.created) != 1 || workspaces.created[0].WorkflowId != socialutil.SFUWorkflowID || workspaces.created[0].Parameters != nil {
		t.Fatalf("created Workspaces = %#v, want SFU Workflow without parameters", workspaces.created)
	}
	groupID := mustGroupID(t, s, "owner", group.Name)
	stored, err := s.readWorkspaceBinding(ctx, groupID)
	if err != nil {
		t.Fatalf("readWorkspaceBinding: %v", err)
	}
	if !strings.HasPrefix(stored.SFU.RoomToken, "room-") || strings.Contains(stored.SFU.RoomToken, groupID) ||
		stored.SFU.URL != s.SFUURL || stored.SFU.Generation != 1 || stored.Owner != "owner" {
		t.Fatalf("group binding = %#v", stored)
	}
	if _, err := s.AddFriendGroupMember(ctx, "owner", rpcapi.FriendGroupMemberAddRequest{
		FriendGroupName: group.Name, PeerPublicKey: "member", MemberName: "room", Role: rpcapi.FriendGroupMemberMutableRole("member"),
	}); err != nil {
		t.Fatal(err)
	}
	binding, err := s.ResolveSFUWorkspaceBinding(ctx, stored.WorkspaceID, "member")
	if err != nil {
		t.Fatalf("ResolveSFUWorkspaceBinding(member) error = %v", err)
	}
	if binding.Kind != socialutil.SFUWorkspaceKindFriendGroup || binding.SocialID != groupID || binding.Owner != "owner" ||
		binding.SFU != stored.SFU || !slices.Equal(binding.Members, []string{"member", "owner"}) {
		t.Fatalf("ResolveSFUWorkspaceBinding(member) = %#v", binding)
	}
	if _, err := s.ResolveSFUWorkspaceBinding(ctx, stored.WorkspaceID, "stranger"); !errors.Is(err, sfu.ErrNotMember) {
		t.Fatalf("ResolveSFUWorkspaceBinding(stranger) error = %v, want %v", err, sfu.ErrNotMember)
	}
	if _, err := s.ResolveSFUWorkspaceBinding(ctx, "unknown-workspace", "owner"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("ResolveSFUWorkspaceBinding(unbound) error = %v, want not found", err)
	}
	if listed, err := s.ListSFUWorkspaceBindingsForPeer(ctx, "member"); err != nil || len(listed) != 1 || listed[0].WorkspaceID != stored.WorkspaceID {
		t.Fatalf("ListSFUWorkspaceBindingsForPeer(member) = %#v, %v", listed, err)
	}
	if _, err := s.DeleteFriendGroupMember(ctx, "owner", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupName: group.Name, Name: "member"}); err != nil {
		t.Fatalf("DeleteFriendGroupMember: %v", err)
	}
	retained, err := s.readWorkspaceBinding(ctx, groupID)
	if err != nil || retained.SFU.Generation != stored.SFU.Generation || retained.SFU.RoomToken != stored.SFU.RoomToken {
		t.Fatalf("binding after member removal = %#v, %v, want the unchanged Room and generation", retained, err)
	}
	if remaining, err := s.ResolveSFUWorkspaceBinding(ctx, stored.WorkspaceID, "owner"); err != nil ||
		remaining.SFU.Generation != stored.SFU.Generation {
		t.Fatalf("remaining member binding after removal = %#v, %v, want an unchanged generation", remaining, err)
	}
	if _, err := s.ResolveSFUWorkspaceBinding(ctx, stored.WorkspaceID, "member"); !errors.Is(err, sfu.ErrNotMember) {
		t.Fatalf("ResolveSFUWorkspaceBinding(removed member) error = %v, want %v", err, sfu.ErrNotMember)
	}
	if _, err := s.groupMember(ctx, groupID, "member"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("removed member error = %v, want not found", err)
	}
	if _, err := s.DeleteFriendGroup(ctx, "owner", rpcapi.FriendGroupDeleteRequest{Name: group.Name}); err != nil {
		t.Fatalf("DeleteFriendGroup: %v", err)
	}
	if _, err := s.ResolveSFUWorkspaceBinding(ctx, stored.WorkspaceID, "owner"); !errors.Is(err, sfu.ErrRevoked) {
		t.Fatalf("ResolveSFUWorkspaceBinding(retired) error = %v, want %v", err, sfu.ErrRevoked)
	}
}

func TestCreateFriendGroupRequiresSFUConfiguration(t *testing.T) {
	s := newTestServer(t)
	s.SFUURL = ""
	if _, err := s.CreateFriendGroup(t.Context(), "owner", rpcapi.FriendGroupCreateRequest{Name: "room"}); !errors.Is(err, ErrSFUNotConfigured) {
		t.Fatalf("CreateFriendGroup() error = %v, want %v", err, ErrSFUNotConfigured)
	}
	if len(s.Workspaces.(*recordingWorkspaceService).created) != 0 {
		t.Fatal("CreateFriendGroup() without SFU created a Workspace")
	}
}

func TestFriendGroupEventsReachCurrentAndFormerMembers(t *testing.T) {
	ctx := t.Context()
	s := newTestServer(t)
	var notifications []groupNotification
	s.NotifyPeer = func(_ context.Context, recipient string, event *eventpb.PeerEvent) {
		notifications = append(notifications, groupNotification{recipient: recipient, event: event})
	}

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	assertGroupNotifications(
		t,
		notifications,
		map[string]string{"peer-a": "room"},
		eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_CREATED,
	)

	notifications = nil
	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{
		FriendGroupName: group.Name,
		PeerPublicKey:   "peer-b",
		Role:            rpcapi.FriendGroupMemberMutableRole("member"),
		MemberName:      "room-b",
	}); err != nil {
		t.Fatalf("AddFriendGroupMember: %v", err)
	}
	assertGroupNotifications(
		t,
		notifications,
		map[string]string{"peer-a": "room", "peer-b": "room-b"},
		eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_MEMBER_ADDED,
	)

	notifications = nil
	updatedName := "renamed room"
	if _, err := s.PutFriendGroup(ctx, "peer-a", rpcapi.FriendGroupPutRequest{
		Name:        group.Name,
		DisplayName: &updatedName,
	}); err != nil {
		t.Fatalf("PutFriendGroup: %v", err)
	}
	assertGroupNotifications(
		t,
		notifications,
		map[string]string{"peer-a": "room", "peer-b": "room-b"},
		eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_METADATA_UPDATED,
	)

	notifications = nil
	if _, err := s.DeleteFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberDeleteRequest{
		FriendGroupName: group.Name,
		Name:            "peer-b",
	}); err != nil {
		t.Fatalf("DeleteFriendGroupMember: %v", err)
	}
	assertGroupNotifications(
		t,
		notifications,
		map[string]string{"peer-a": "room", "peer-b": "room-b"},
		eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_MEMBER_REMOVED,
	)
	for _, notification := range notifications {
		if got := notification.event.GetFriendGroupUpdated().GetAffectedPeerPublicKey(); got != "peer-b" {
			t.Fatalf("removed member in notification = %q, want peer-b", got)
		}
	}
}

func assertGroupNotifications(
	t *testing.T,
	notifications []groupNotification,
	wantNames map[string]string,
	change eventpb.FriendGroupChange,
) {
	t.Helper()
	if len(notifications) != len(wantNames) {
		t.Fatalf("notifications = %#v, want recipients %#v", notifications, wantNames)
	}
	for _, notification := range notifications {
		wantName, ok := wantNames[notification.recipient]
		if !ok {
			t.Fatalf("unexpected recipient %q in %#v", notification.recipient, notifications)
		}
		payload := notification.event.GetFriendGroupUpdated()
		if notification.event.GetType() != eventpb.PeerEventType_PEER_EVENT_TYPE_FRIEND_GROUP_UPDATED ||
			payload == nil ||
			payload.GetFriendGroupName() != wantName ||
			payload.GetChange() != change {
			t.Fatalf("notification = recipient=%q event=%+v", notification.recipient, notification.event)
		}
		delete(wantNames, notification.recipient)
	}
	if len(wantNames) != 0 {
		t.Fatalf("missing recipients = %#v", wantNames)
	}
}

func TestGroupWorkspaceBelongsToCreator(t *testing.T) {
	workspaces := &recordingWorkspaceService{}
	s := newTestServer(t)
	s.Workspaces = workspaces
	if _, err := s.CreateFriendGroup(t.Context(), "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"}); err != nil {
		t.Fatal(err)
	}
	if len(workspaces.created) != 1 || workspaces.created[0].WorkflowId != socialutil.SFUWorkflowID {
		t.Fatalf("created Workspaces = %#v", workspaces.created)
	}
	if len(workspaces.owners) != 1 || workspaces.owners[0] != "peer-a" {
		t.Fatalf("Workspace owners = %#v, want peer-a", workspaces.owners)
	}
}

func TestAdminCreateFriendGroupStopsBeforeWorkspaceOnGroupReservationFailure(t *testing.T) {
	ctx := context.Background()
	workspaces := &recordingWorkspaceService{}
	s := newTestServer(t)
	s.Workspaces = workspaces
	s.Groups = failingSetStore{Store: kv.NewMemory(nil)}

	if _, err := s.AdminCreateFriendGroup(ctx, "id-a", "peer-a", "family", nil, nil); err == nil {
		t.Fatal("AdminCreateFriendGroup with failing group store error = nil")
	}
	if len(workspaces.created) != 0 || len(workspaces.deleted) != 0 {
		t.Fatalf("Workspace mutations = created:%#v deleted:%#v, want none", workspaces.created, workspaces.deleted)
	}
}

func TestAdminCreateFriendGroupCreatesOwnerMembership(t *testing.T) {
	ctx := t.Context()
	s := newTestServer(t)
	group, err := s.AdminCreateFriendGroup(ctx, "id-a", "peer-a", "family", nil, nil)
	if err != nil {
		t.Fatalf("AdminCreateFriendGroup: %v", err)
	}
	member, err := s.AdminGetFriendGroupMember(ctx, group.Id, "peer-a")
	if err != nil {
		t.Fatalf("AdminGetFriendGroupMember: %v", err)
	}
	if got := socialutil.StringValue(member.FriendGroupName); got != "family" {
		t.Fatalf("owner membership name = %q, want family", got)
	}
	if got := socialutil.GroupRole(member); got != rpcapi.FriendGroupMemberRoleOwner {
		t.Fatalf("owner membership role = %q, want owner", got)
	}
	members, err := s.membersStore()
	if err != nil {
		t.Fatalf("membersStore: %v", err)
	}
	storedMember, err := members.Get(ctx, socialutil.GroupMemberKey(group.Id, "peer-a"))
	if err != nil {
		t.Fatalf("read persisted FriendGroupMember: %v", err)
	}
	var storedFields map[string]json.RawMessage
	if err := json.Unmarshal(storedMember, &storedFields); err != nil {
		t.Fatalf("decode persisted FriendGroupMember: %v", err)
	}
	if _, ok := storedFields["friend_group_id"]; !ok {
		t.Fatalf("persisted FriendGroupMember = %s, want canonical friend_group_id", storedMember)
	}
	if _, ok := storedFields["id"]; ok {
		t.Fatalf("persisted FriendGroupMember = %s, unexpectedly stores Peer wire id", storedMember)
	}
	if _, ok := storedFields["name"]; ok {
		t.Fatalf("persisted FriendGroupMember = %s, unexpectedly stores Peer wire name", storedMember)
	}
}

func TestLegacyWireFriendGroupMemberRecordIsRejected(t *testing.T) {
	ctx := t.Context()
	s := newTestServer(t)
	legacy := []byte(`{"id":"peer-b","name":"peer-b","friend_group_name":"room-b","peer_public_key":"peer-b","role":"member"}`)
	if err := s.Members.Set(ctx, socialutil.GroupMemberKey("group-a", "peer-b"), legacy); err != nil {
		t.Fatalf("seed legacy FriendGroupMember wire record: %v", err)
	}
	if _, err := s.groupMember(ctx, "group-a", "peer-b"); err == nil || err.Error() != "social: persisted Friend Group member is invalid" {
		t.Fatalf("groupMember legacy wire record error = %v", err)
	}
}

func TestAdminCreateFriendGroupRejectsIDThatCannotProduceBoundedMembershipID(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.AdminCreateFriendGroup(t.Context(), strings.Repeat("😀", 81), "peer-a", "family", nil, nil); err == nil {
		t.Fatal("AdminCreateFriendGroup accepted oversized FriendGroup ID")
	}
}

func TestAdminCreateFriendGroupRejectsDuplicateCallerID(t *testing.T) {
	ctx := t.Context()
	workspaces := &recordingWorkspaceService{}
	s := newTestServer(t)
	s.Workspaces = workspaces
	created, err := s.AdminCreateFriendGroup(ctx, "id-a", "peer-a", "family", nil, nil)
	if err != nil {
		t.Fatalf("AdminCreateFriendGroup: %v", err)
	}
	if _, err := s.AdminCreateFriendGroup(ctx, "id-a", "peer-b", "replacement", nil, nil); !errors.Is(err, socialutil.ErrResourceAlreadyExists) {
		t.Fatalf("AdminCreateFriendGroup duplicate caller ID error = %v, want conflict", err)
	}
	got, err := s.AdminGetFriendGroupObject(ctx, created.Id)
	if err != nil {
		t.Fatalf("AdminGetFriendGroupObject after duplicate caller ID: %v", err)
	}
	if got.Name != "family" || got.CreatedByPeerPublicKey != "peer-a" {
		t.Fatalf("FriendGroup after duplicate caller ID = %+v, want original peer-a/family", got)
	}
	if len(workspaces.created) != 1 {
		t.Fatalf("created Workspaces = %d, want 1", len(workspaces.created))
	}
	if _, err := s.AdminGetFriendGroupMember(ctx, created.Id, "peer-b"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("duplicate owner membership error = %v, want not found", err)
	}
}

func TestAdminCreateFriendGroupRollsBackOnOwnerMembershipWriteFailure(t *testing.T) {
	ctx := t.Context()
	groups := kv.NewMemory(nil)
	workspaces := &recordingWorkspaceService{}
	s := newTestServer(t)
	s.Groups = groups
	s.Members = failingSetStore{Store: kv.NewMemory(nil)}
	s.Workspaces = workspaces

	if _, err := s.AdminCreateFriendGroup(ctx, "id-a", "peer-a", "family", nil, nil); err == nil {
		t.Fatal("AdminCreateFriendGroup with failing member store error = nil")
	}
	if _, err := groups.Get(ctx, socialutil.GroupKey("id-a")); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("group after rollback error = %v, want not found", err)
	}
	if len(workspaces.deleted) != 1 {
		t.Fatalf("deleted workspaces = %#v, want one workspace rollback", workspaces.deleted)
	}
}

func TestAdminDeleteFriendGroupMemberIsAtomicAndKeepsTheRoom(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	group, err := s.AdminCreateFriendGroup(ctx, "id-a", "peer-a", "family", nil, nil)
	if err != nil {
		t.Fatalf("AdminCreateFriendGroup: %v", err)
	}
	friendGroupID := group.Id
	if _, err := s.AdminPutFriendGroupMember(ctx, friendGroupID, "peer-b", "family-b", rpcapi.FriendGroupMemberRoleMember); err != nil {
		t.Fatalf("AdminPutFriendGroupMember: %v", err)
	}
	healthy := s.RelationshipStore
	s.RelationshipStore = failingBatchMutateStore{Store: healthy}
	if _, err := s.AdminDeleteFriendGroupMember(ctx, friendGroupID, "peer-b"); err == nil {
		t.Fatal("AdminDeleteFriendGroupMember with failing relationship store error = nil")
	}
	if _, err := s.groupMember(ctx, friendGroupID, "peer-b"); err != nil {
		t.Fatalf("groupMember after failed admin delete = %v, want retained", err)
	}
	s.RelationshipStore = healthy
	if binding, err := s.readWorkspaceBinding(ctx, friendGroupID); err != nil || binding.SFU.Generation != 1 {
		t.Fatalf("binding after failed delete = %#v, %v, want generation 1", binding, err)
	}
	if _, err := s.AdminDeleteFriendGroupMember(ctx, friendGroupID, "peer-b"); err != nil {
		t.Fatalf("AdminDeleteFriendGroupMember: %v", err)
	}
	if _, err := s.groupMember(ctx, friendGroupID, "peer-b"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("groupMember after delete error = %v, want not found", err)
	}
	if _, err := s.Belongs.Get(ctx, socialutil.GroupBelongKey("peer-b", friendGroupID)); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("belongs after delete error = %v, want not found", err)
	}
	if _, err := s.Belongs.Get(ctx, socialutil.GroupNameKey("peer-b", "family-b")); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("name projection after delete error = %v, want not found", err)
	}
	if binding, err := s.readWorkspaceBinding(ctx, friendGroupID); err != nil || binding.SFU.Generation != 1 {
		t.Fatalf("binding after delete = %#v, %v, want an unchanged generation", binding, err)
	}
}

func TestDeleteFriendGroupIsRelationshipFirstAndRetryable(t *testing.T) {
	ctx := t.Context()
	workspaces := &recordingWorkspaceService{
		retiredOwner: "peer-a",
		retireErr:    errors.New("forced retirement failure"),
	}
	s := newTestServer(t)
	s.NewID = func() string { return "group-0001" }
	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	groupID := mustGroupID(t, s, "peer-a", group.Name)
	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{
		FriendGroupName: group.Name,
		PeerPublicKey:   "peer-b",
		Role:            rpcapi.FriendGroupMemberMutableRole("member"),
		MemberName:      "room-b",
	}); err != nil {
		t.Fatalf("AddFriendGroupMember: %v", err)
	}
	legacyMessageRoot := kv.Key{"friend-group-messages"}
	legacyMessageKey := append(append(kv.Key{}, legacyMessageRoot...), socialutil.EscapeStoreSegment(groupID), "legacy-message")
	if err := s.Groups.Set(ctx, legacyMessageKey, []byte(`{"legacy":true}`)); err != nil {
		t.Fatalf("seed legacy message metadata: %v", err)
	}
	legacyDescriptor := retiredFriendGroupDataDescriptor{
		FriendGroupID:      groupID,
		MessageStorePrefix: []string{legacyMessageRoot[0], socialutil.EscapeStoreSegment(groupID)},
		MessageAssetPrefix: socialutil.EscapeStoreSegment(groupID) + "/",
	}
	legacyPending, err := pendingdeletion.New(pendingdeletion.KindFriendGroup, groupID, nil, pendingdeletion.ReasonFriendGroupDelete, legacyDescriptor, s.now())
	if err != nil {
		t.Fatalf("create legacy PendingDeletion: %v", err)
	}
	if _, _, err := pendingdeletion.CreateOrGet(ctx, s.RelationshipStore, legacyPending); err != nil {
		t.Fatalf("seed legacy PendingDeletion: %v", err)
	}
	var notifications []groupNotification
	s.NotifyPeer = func(_ context.Context, recipient string, event *eventpb.PeerEvent) {
		notifications = append(notifications, groupNotification{recipient: recipient, event: event})
	}
	s.Workspaces = workspaces

	if _, err := s.DeleteFriendGroup(ctx, "peer-a", rpcapi.FriendGroupDeleteRequest{Name: group.Name}); !errors.Is(err, workspaces.retireErr) {
		t.Fatalf("DeleteFriendGroup first error = %v, want retirement failure", err)
	}
	if _, err := s.AdminGetFriendGroup(ctx, groupID); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("AdminGetFriendGroup after committed relationship delete = %v, want not found", err)
	}
	if _, err := s.groupMember(ctx, groupID, "peer-b"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("groupMember after committed relationship delete = %v, want not found", err)
	}
	if len(workspaces.deleted) != 0 || len(workspaces.retired) != 1 {
		t.Fatalf("workspace calls after first delete: deleted=%v retired=%v", workspaces.deleted, workspaces.retired)
	}
	if len(notifications) != 0 {
		t.Fatalf("notifications before durable PendingDeletion = %#v, want none", notifications)
	}
	pending, err := pendingdeletion.GetByLocator(
		ctx,
		s.RelationshipStore,
		pendingdeletion.KindFriendGroup,
		groupID,
	)
	if err != nil {
		t.Fatalf("Friend Group data PendingDeletion after relationship commit: %v", err)
	}
	var descriptor retiredFriendGroupDataDescriptor
	if err := json.Unmarshal(pending.Descriptor, &descriptor); err != nil {
		t.Fatalf("decode Friend Group data PendingDeletion descriptor: %v", err)
	}
	if descriptor.FriendGroupID != groupID ||
		len(descriptor.MessageStorePrefix) != 2 ||
		descriptor.MessageStorePrefix[0] != legacyMessageRoot[0] ||
		descriptor.MessageStorePrefix[1] != socialutil.EscapeStoreSegment(groupID) ||
		descriptor.MessageAssetPrefix != socialutil.EscapeStoreSegment(groupID)+"/" {
		t.Fatalf("Friend Group data PendingDeletion descriptor = %#v", descriptor)
	}
	if _, err := s.Groups.Get(ctx, legacyMessageKey); err != nil {
		t.Fatalf("message metadata removed during retirement: %v", err)
	}

	workspaces.retireErr = nil
	restarted := &Server{
		Groups:                   s.Groups,
		InviteTokens:             s.InviteTokens,
		Members:                  s.Members,
		Belongs:                  s.Belongs,
		RelationshipStore:        s.RelationshipStore,
		GroupRelationshipPrefix:  s.GroupRelationshipPrefix,
		InviteRelationshipPrefix: s.InviteRelationshipPrefix,
		MemberRelationshipPrefix: s.MemberRelationshipPrefix,
		BelongRelationshipPrefix: s.BelongRelationshipPrefix,
		Workspaces:               workspaces,
		Now:                      s.Now,
		NotifyPeer:               s.NotifyPeer,
	}
	if err := restarted.ReconcileRetirementIntents(ctx); err != nil {
		t.Fatalf("ReconcileRetirementIntents after restart: %v", err)
	}
	if len(workspaces.retired) != 2 || workspaces.retired[0] != workspaces.retired[1] {
		t.Fatalf("retirement retry targets = %v, want same Workspace twice", workspaces.retired)
	}
	assertGroupNotifications(
		t,
		notifications,
		map[string]string{"peer-a": "room", "peer-b": "room-b"},
		eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_DELETED,
	)
	notificationCount := len(notifications)
	retried, err := restarted.DeleteFriendGroup(
		ctx,
		"peer-a",
		rpcapi.FriendGroupDeleteRequest{Name: group.Name},
	)
	if err != nil {
		t.Fatalf("DeleteFriendGroup retry after completed retirement: %v", err)
	}
	if retried.Name != group.Name ||
		socialutil.StringValue(retried.WorkspaceName) != socialutil.StringValue(group.WorkspaceName) {
		t.Fatalf("DeleteFriendGroup completed retry = %#v", retried)
	}
	if len(notifications) != notificationCount {
		t.Fatalf("completed retry notifications = %d, want %d", len(notifications), notificationCount)
	}
}

func TestDeleteFriendGroupRejectsUnauthorizedBeforePendingDeletion(t *testing.T) {
	ctx := t.Context()
	workspaces := &recordingWorkspaceService{}
	s := newTestServer(t)
	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	groupID := mustGroupID(t, s, "peer-a", group.Name)
	s.Workspaces = workspaces

	if _, err := s.DeleteFriendGroup(
		ctx,
		"peer-b",
		rpcapi.FriendGroupDeleteRequest{Name: group.Name},
	); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("DeleteFriendGroup unauthorized error = %v, want not found", err)
	}
	if len(workspaces.retired) != 0 || len(workspaces.deleted) != 0 {
		t.Fatalf(
			"workspace changed after unauthorized delete: retired=%v deleted=%v",
			workspaces.retired,
			workspaces.deleted,
		)
	}
	if exists, err := pendingdeletion.HasLocator(
		ctx,
		s.RelationshipStore,
		pendingdeletion.KindFriendGroup,
		groupID,
	); err != nil || exists {
		t.Fatalf("Friend Group data PendingDeletion after unauthorized delete = %v, error = %v", exists, err)
	}
}

func TestDeleteFriendGroupRejectsUnknownBeforePendingDeletion(t *testing.T) {
	ctx := t.Context()
	workspaces := &recordingWorkspaceService{}
	s := newTestServer(t)
	s.Workspaces = workspaces
	const groupID = "unknown-group"

	if _, err := s.DeleteFriendGroup(
		ctx,
		"peer-b",
		rpcapi.FriendGroupDeleteRequest{Name: "unknown-group"},
	); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("DeleteFriendGroup unknown error = %v, want not found", err)
	}
	if len(workspaces.retired) != 0 || len(workspaces.deleted) != 0 {
		t.Fatalf(
			"workspace changed after unknown delete: retired=%v deleted=%v",
			workspaces.retired,
			workspaces.deleted,
		)
	}
	if exists, err := pendingdeletion.HasLocator(
		ctx,
		s.RelationshipStore,
		pendingdeletion.KindFriendGroup,
		groupID,
	); err != nil || exists {
		t.Fatalf("Friend Group data PendingDeletion after unknown delete = %v, error = %v", exists, err)
	}
}

func TestDeleteFriendGroupWithoutWorkspaceRetirementKeepsRelationships(t *testing.T) {
	ctx := t.Context()
	s := newTestServer(t)
	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	groupID := mustGroupID(t, s, "peer-a", group.Name)
	s.Workspaces = nil

	if _, err := s.DeleteFriendGroup(ctx, "peer-a", rpcapi.FriendGroupDeleteRequest{Name: group.Name}); err == nil ||
		!strings.Contains(err.Error(), "retirement service not configured") {
		t.Fatalf("DeleteFriendGroup error = %v, want missing retirement service", err)
	}
	if _, err := s.AdminGetFriendGroup(ctx, groupID); err != nil {
		t.Fatalf("AdminGetFriendGroup after rejected delete: %v", err)
	}
	if _, err := s.readRetirementIntent(ctx, groupID); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("readRetirementIntent after rejected delete error = %v, want not found", err)
	}
}

func TestDeleteFriendGroupBatchFailureKeepsRelationshipsAndWorkspace(t *testing.T) {
	ctx := t.Context()
	workspaces := &recordingWorkspaceService{}
	s := newTestServer(t)
	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	groupID := mustGroupID(t, s, "peer-a", group.Name)
	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{
		FriendGroupName: group.Name,
		PeerPublicKey:   "peer-b",
		Role:            rpcapi.FriendGroupMemberMutableRole("member"),
		MemberName:      "room-b",
	}); err != nil {
		t.Fatalf("AddFriendGroupMember: %v", err)
	}
	s.Workspaces = workspaces
	s.RelationshipStore = failingBatchMutateStore{Store: s.RelationshipStore}

	if _, err := s.DeleteFriendGroup(ctx, "peer-a", rpcapi.FriendGroupDeleteRequest{Name: group.Name}); err == nil {
		t.Fatal("DeleteFriendGroup with failing BatchMutate error = nil")
	}
	if _, err := s.AdminGetFriendGroup(ctx, groupID); err != nil {
		t.Fatalf("AdminGetFriendGroup after batch failure: %v", err)
	}
	if _, err := s.groupMember(ctx, groupID, "peer-b"); err != nil {
		t.Fatalf("groupMember after batch failure: %v", err)
	}
	if len(workspaces.retired) != 0 || len(workspaces.deleted) != 0 {
		t.Fatalf("workspace changed after relationship batch failure: retired=%v deleted=%v", workspaces.retired, workspaces.deleted)
	}
	if exists, err := pendingdeletion.HasLocator(
		ctx,
		s.RelationshipStore,
		pendingdeletion.KindFriendGroup,
		groupID,
	); err != nil || exists {
		t.Fatalf("Friend Group data PendingDeletion after batch failure = %v, error = %v", exists, err)
	}
}

func TestDeleteFriendGroupRequiresConditionalCreateBeforeRelationshipMutation(t *testing.T) {
	ctx := t.Context()
	workspaces := &recordingWorkspaceService{}
	s := newTestServer(t)
	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	groupID := mustGroupID(t, s, "peer-a", group.Name)
	s.Workspaces = workspaces
	s.RelationshipStore = storeWithoutCreateIfAbsent{s.RelationshipStore}

	if _, err := s.DeleteFriendGroup(
		ctx,
		"peer-a",
		rpcapi.FriendGroupDeleteRequest{Name: group.Name},
	); !errors.Is(err, kv.ErrCreateIfAbsentUnsupported) {
		t.Fatalf("DeleteFriendGroup without conditional create error = %v", err)
	}
	if _, err := s.AdminGetFriendGroup(ctx, groupID); err != nil {
		t.Fatalf("AdminGetFriendGroup after capability rejection: %v", err)
	}
	if len(workspaces.retired) != 0 || len(workspaces.deleted) != 0 {
		t.Fatalf(
			"workspace changed after capability rejection: retired=%v deleted=%v",
			workspaces.retired,
			workspaces.deleted,
		)
	}
}

func TestBelongsStoreFallsBackToMembersStore(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	s.Belongs = nil

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := mustGroupID(t, s, "peer-a", group.Name)
	if _, err := s.AdminGetFriendGroup(ctx, " "+friendGroupID+" "); err == nil {
		t.Fatal("AdminGetFriendGroup padded id error = nil")
	}
	assertBelongs(t, ctx, s, "peer-a", friendGroupID, group.Name, rpcapi.FriendGroupMemberRoleOwner)

	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{FriendGroupName: group.Name, PeerPublicKey: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("member"), MemberName: "room-b"}); err != nil {
		t.Fatalf("AddFriendGroupMember: %v", err)
	}
	groups, err := s.ListFriendGroups(ctx, "peer-b", rpcapi.FriendGroupListRequest{})
	if err != nil {
		t.Fatalf("ListFriendGroups peer-b: %v", err)
	}
	if len(groups.Items) != 1 || groups.Items[0].Name != "room-b" || groups.Items[0].MyRole == nil || *groups.Items[0].MyRole != rpcapi.FriendGroupMemberRoleMember {
		t.Fatalf("ListFriendGroups peer-b = %#v, want member group", groups)
	}
}

func TestMemberDeleteRoleRules(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := mustGroupID(t, s, "peer-a", group.Name)
	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{FriendGroupName: group.Name, PeerPublicKey: "peer-a", Role: rpcapi.FriendGroupMemberMutableRole("member"), MemberName: group.Name}); err == nil {
		t.Fatal("AddFriendGroupMember owner role change error = nil")
	}
	ownerMember, err := s.groupMember(ctx, friendGroupID, "peer-a")
	if err != nil {
		t.Fatalf("owner groupMember after failed add: %v", err)
	}
	if got := socialutil.GroupRole(ownerMember); got != rpcapi.FriendGroupMemberRoleOwner {
		t.Fatalf("owner role after failed add = %q, want owner", got)
	}
	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{FriendGroupName: group.Name, PeerPublicKey: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("member"), MemberName: "room-b"}); err != nil {
		t.Fatalf("AddFriendGroupMember peer-b: %v", err)
	}
	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{FriendGroupName: group.Name, PeerPublicKey: "peer-c", Role: rpcapi.FriendGroupMemberMutableRole("admin"), MemberName: "room-c"}); err != nil {
		t.Fatalf("AddFriendGroupMember peer-c admin: %v", err)
	}
	if _, err := s.DeleteFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupName: group.Name, Name: "peer-a"}); err == nil {
		t.Fatal("DeleteFriendGroupMember owner error = nil")
	}
	if _, err := s.DeleteFriendGroupMember(ctx, "peer-b", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupName: "room-b", Name: "peer-c"}); err == nil {
		t.Fatal("DeleteFriendGroupMember admin by member error = nil")
	}
	deletedAdmin, err := s.DeleteFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupName: group.Name, Name: "peer-c"})
	if err != nil {
		t.Fatalf("DeleteFriendGroupMember admin by owner: %v", err)
	}
	if socialutil.StringValue(deletedAdmin.PeerPublicKey) != "peer-c" {
		t.Fatalf("deleted admin peer_public_key = %q, want peer-c", socialutil.StringValue(deletedAdmin.PeerPublicKey))
	}
	selfDeleted, err := s.DeleteFriendGroupMember(ctx, "peer-b", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupName: "room-b", Name: "peer-b"})
	if err != nil {
		t.Fatalf("DeleteFriendGroupMember self member: %v", err)
	}
	if socialutil.StringValue(selfDeleted.PeerPublicKey) != "peer-b" {
		t.Fatalf("self deleted peer_public_key = %q, want peer-b", socialutil.StringValue(selfDeleted.PeerPublicKey))
	}
}

func TestPeerRetirementDeletesOwnedGroupAndOnlyForeignMembership(t *testing.T) {
	s := newTestServer(t)
	s.NewID = func() string { return "group-owned" }
	owned, err := s.CreateFriendGroup(t.Context(), "peer-a", rpcapi.FriendGroupCreateRequest{Name: "owned"})
	if err != nil {
		t.Fatal(err)
	}
	ownedID := mustGroupID(t, s, "peer-a", owned.Name)
	if _, err := s.AddFriendGroupMember(t.Context(), "peer-a", rpcapi.FriendGroupMemberAddRequest{
		FriendGroupName: owned.Name, PeerPublicKey: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("member"), MemberName: "owned-b",
	}); err != nil {
		t.Fatal(err)
	}
	s.NewID = func() string { return "group-foreign" }
	foreign, err := s.CreateFriendGroup(t.Context(), "peer-b", rpcapi.FriendGroupCreateRequest{Name: "foreign"})
	if err != nil {
		t.Fatal(err)
	}
	foreignID := mustGroupID(t, s, "peer-b", foreign.Name)
	if _, err := s.AddFriendGroupMember(t.Context(), "peer-b", rpcapi.FriendGroupMemberAddRequest{
		FriendGroupName: foreign.Name, PeerPublicKey: "peer-a", Role: rpcapi.FriendGroupMemberMutableRole("member"), MemberName: "foreign-a",
	}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := s.SnapshotPeerGroups(t.Context(), "peer-a")
	if err != nil || len(snapshot) != 2 {
		t.Fatalf("SnapshotPeerGroups() = %#v, %v", snapshot, err)
	}
	for _, item := range snapshot {
		if err := s.RetirePeerGroup(t.Context(), item); err != nil {
			t.Fatalf("RetirePeerGroup(%s) error = %v", item.FriendGroupID, err)
		}
		if err := s.RetirePeerGroup(t.Context(), item); err != nil {
			t.Fatalf("replayed RetirePeerGroup(%s) error = %v", item.FriendGroupID, err)
		}
	}
	if _, err := s.AdminGetFriendGroup(t.Context(), ownedID); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("owned group error = %v", err)
	}
	if _, err := s.AdminGetFriendGroup(t.Context(), foreignID); err != nil {
		t.Fatalf("foreign group removed: %v", err)
	}
	if _, err := s.AdminGetFriendGroupMember(t.Context(), foreignID, "peer-a"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("retiring foreign membership error = %v", err)
	}
	if _, err := s.AdminGetFriendGroupMember(t.Context(), foreignID, "peer-b"); err != nil {
		t.Fatalf("foreign owner membership removed: %v", err)
	}
}

func TestPeerRetirementSnapshotOnlyBlocksGroupsForTargetPeer(t *testing.T) {
	s := newTestServer(t)
	s.NewID = func() string { return "group-a" }
	groupA, err := s.CreateFriendGroup(t.Context(), "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room-a"})
	if err != nil {
		t.Fatal(err)
	}
	s.NewID = func() string { return "group-b" }
	groupB, err := s.CreateFriendGroup(t.Context(), "peer-b", rpcapi.FriendGroupCreateRequest{Name: "room-b"})
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	s.Belongs = &blockingGroupListStore{
		Store:   s.Belongs,
		prefix:  append(append(kv.Key{}, socialutil.GroupBelongsRoot...), socialutil.EscapeStoreSegment("peer-a")),
		entered: entered, release: release,
	}
	secondary := *s
	snapshotDone := make(chan error, 1)
	go func() {
		_, err := s.SnapshotPeerGroups(t.Context(), "peer-a")
		snapshotDone <- err
	}()
	<-entered

	unrelatedDone := make(chan error, 1)
	go func() {
		display := "updated-b"
		_, err := secondary.PutFriendGroup(t.Context(), "peer-b", rpcapi.FriendGroupPutRequest{Name: groupB.Name, DisplayName: &display})
		unrelatedDone <- err
	}()
	targetDone := make(chan error, 1)
	go func() {
		display := "updated-a"
		_, err := secondary.PutFriendGroup(t.Context(), "peer-a", rpcapi.FriendGroupPutRequest{Name: groupA.Name, DisplayName: &display})
		targetDone <- err
	}()

	select {
	case err := <-unrelatedDone:
		if err != nil {
			t.Fatalf("unrelated Friend Group mutation: %v", err)
		}
	case <-time.After(time.Second):
		close(release)
		<-snapshotDone
		<-unrelatedDone
		<-targetDone
		t.Fatal("unrelated Friend Group mutation was blocked by peer-a retirement snapshot")
	}
	select {
	case err := <-targetDone:
		close(release)
		<-snapshotDone
		t.Fatalf("peer-a Friend Group mutation crossed accepted snapshot: %v", err)
	default:
	}
	close(release)
	if err := <-snapshotDone; err != nil {
		t.Fatalf("SnapshotPeerGroups(): %v", err)
	}
	if err := <-targetDone; err != nil {
		t.Fatalf("target Friend Group mutation after snapshot: %v", err)
	}
}

func TestFriendGroupMemberCreationFailsClosedWhenTargetUnavailable(t *testing.T) {
	s := newTestServer(t)
	group, err := s.CreateFriendGroup(t.Context(), "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatal(err)
	}
	groupID := mustGroupID(t, s, "peer-a", group.Name)
	wantErr := errors.New("PEER_DELETED")
	s.PeerAvailability = func(_ context.Context, publicKey string) error {
		if publicKey == "peer-b" {
			return wantErr
		}
		return nil
	}
	_, err = s.AdminPutFriendGroupMember(t.Context(), groupID, "peer-b", "room-b", rpcapi.FriendGroupMemberRoleMember)
	if !errors.Is(err, wantErr) {
		t.Fatalf("AdminPutFriendGroupMember() error = %v, want target fence", err)
	}
	if _, err := s.AdminGetFriendGroupMember(t.Context(), groupID, "peer-b"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("membership written before target fence: %v", err)
	}
}

func TestFriendGroupMutationsRejectUnavailableOwnerButRetirementBypassesFence(t *testing.T) {
	s := newTestServer(t)
	group, err := s.CreateFriendGroup(t.Context(), "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatal(err)
	}
	groupID := mustGroupID(t, s, "peer-a", group.Name)
	snapshot, err := s.SnapshotPeerGroups(t.Context(), "peer-a")
	if err != nil || len(snapshot) != 1 {
		t.Fatalf("SnapshotPeerGroups() = %#v, %v", snapshot, err)
	}
	wantErr := errors.New("PEER_PENDING_DELETION")
	s.PeerAvailability = func(_ context.Context, publicKey string) error {
		if publicKey == "peer-a" {
			return wantErr
		}
		return nil
	}
	if _, err := s.AdminPutFriendGroup(t.Context(), groupID, new("changed"), nil); !errors.Is(err, wantErr) {
		t.Fatalf("AdminPutFriendGroup() error = %v, want Peer fence", err)
	}
	if _, err := s.AdminPutFriendGroupInviteToken(t.Context(), groupID, "invite", time.Now().Add(time.Hour)); !errors.Is(err, wantErr) {
		t.Fatalf("AdminPutFriendGroupInviteToken() error = %v, want Peer fence", err)
	}
	if _, err := s.AdminPutFriendGroupMember(t.Context(), groupID, "peer-a", "room", rpcapi.FriendGroupMemberRoleOwner); !errors.Is(err, wantErr) {
		t.Fatalf("AdminPutFriendGroupMember() error = %v, want Peer fence", err)
	}
	if _, err := s.AdminDeleteFriendGroup(t.Context(), groupID); !errors.Is(err, wantErr) {
		t.Fatalf("AdminDeleteFriendGroup() error = %v, want Peer fence", err)
	}
	if _, err := s.AdminGetFriendGroup(t.Context(), groupID); err != nil {
		t.Fatalf("Admin get must remain available while Peer is fenced: %v", err)
	}
	if err := s.RetirePeerGroup(t.Context(), snapshot[0]); err != nil {
		t.Fatalf("RetirePeerGroup() error = %v", err)
	}
}

func TestConfigurationErrorsAndHelpers(t *testing.T) {
	ctx := context.Background()
	empty := &Server{}
	if _, err := empty.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"}); err == nil {
		t.Fatal("CreateFriendGroup without store error = nil")
	}
	if _, err := empty.ListFriendGroupMembers(ctx, "peer-a", rpcapi.FriendGroupMemberListRequest{FriendGroupName: new("group-a")}); err == nil {
		t.Fatal("ListFriendGroupMembers without store error = nil")
	}
	if _, err := empty.AdminCreateFriendGroup(ctx, "group-id", "peer-a", "group-a", nil, nil); err == nil {
		t.Fatal("AdminCreateFriendGroup without store error = nil")
	}
	if _, err := empty.AdminGetFriendGroupMember(ctx, "group-a", "peer-a"); err == nil {
		t.Fatal("AdminGetFriendGroupMember without store error = nil")
	}
	if _, err := empty.AdminPutFriendGroupInviteToken(ctx, "group-a", "token", time.Now().Add(time.Hour)); err == nil {
		t.Fatal("AdminPutFriendGroupInviteToken without store error = nil")
	}
	s := newTestServer(t)
	if _, err := s.CreateFriendGroup(ctx, "", rpcapi.FriendGroupCreateRequest{Name: "room"}); err == nil {
		t.Fatal("CreateFriendGroup empty owner error = nil")
	}
	if _, err := s.GetFriendGroup(ctx, "peer-a", rpcapi.FriendGroupGetRequest{}); err == nil {
		t.Fatal("GetFriendGroup empty id error = nil")
	}
	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := mustGroupID(t, s, "peer-a", group.Name)
	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{FriendGroupName: group.Name, Role: rpcapi.FriendGroupMemberMutableRole("member"), MemberName: "room-b"}); err == nil {
		t.Fatal("AddFriendGroupMember empty peer public key error = nil")
	}
	if _, err := s.AdminPutFriendGroup(ctx, "", new("renamed"), nil); err == nil {
		t.Fatal("AdminPutFriendGroup empty id error = nil")
	}
	if _, err := s.AdminDeleteFriendGroup(ctx, ""); err == nil {
		t.Fatal("AdminDeleteFriendGroup empty id error = nil")
	}
	if _, err := s.AdminListFriendGroupMembers(ctx, "missing", rpcapi.FriendGroupMemberListRequest{}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("AdminListFriendGroupMembers missing group error = %v, want kv.ErrNotFound", err)
	}
	if _, err := s.AdminPutFriendGroupMember(ctx, "missing", "peer-b", "missing-b", rpcapi.FriendGroupMemberRoleMember); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("AdminPutFriendGroupMember missing group error = %v, want kv.ErrNotFound", err)
	}
	if _, err := s.groupMember(ctx, "missing", "peer-b"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("groupMember after rejected admin put error = %v, want kv.ErrNotFound", err)
	}
	if _, err := s.AdminPutFriendGroupMember(ctx, friendGroupID, "peer-b", "room-b", rpcapi.FriendGroupMemberRole("observer")); err == nil {
		t.Fatal("AdminPutFriendGroupMember invalid role error = nil")
	}
	if _, err := s.AdminCreateFriendGroupMember(ctx, friendGroupID, "peer-c", "room-c", rpcapi.FriendGroupMemberRoleMember); err != nil {
		t.Fatalf("AdminCreateFriendGroupMember error = %v", err)
	}
	if _, err := s.AdminCreateFriendGroupMember(ctx, friendGroupID, "peer-c", "room-c", rpcapi.FriendGroupMemberRoleMember); !errors.Is(err, ErrFriendGroupMemberAlreadyExists) {
		t.Fatalf("AdminCreateFriendGroupMember duplicate error = %v, want %v", err, ErrFriendGroupMemberAlreadyExists)
	}
	if _, err := s.AdminPutFriendGroupInviteToken(ctx, friendGroupID, "", s.now().Add(time.Hour)); err == nil {
		t.Fatal("AdminPutFriendGroupInviteToken empty token error = nil")
	}
	if _, err := s.AdminPutFriendGroupInviteToken(ctx, friendGroupID, "token", s.now()); err == nil {
		t.Fatal("AdminPutFriendGroupInviteToken expired token error = nil")
	}
	if _, err := s.AdminDeleteFriendGroupInviteToken(ctx, "missing"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("AdminDeleteFriendGroupInviteToken missing group error = %v, want kv.ErrNotFound", err)
	}
	if _, err := s.AdminDeleteFriendGroupMember(ctx, friendGroupID, "missing"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("AdminDeleteFriendGroupMember missing member error = %v, want kv.ErrNotFound", err)
	}
	if _, err := s.JoinFriendGroup(ctx, "", rpcapi.FriendGroupJoinRequest{InviteToken: "token"}); err == nil {
		t.Fatal("JoinFriendGroup empty owner error = nil")
	}
	if _, err := s.JoinFriendGroup(ctx, "peer-b", rpcapi.FriendGroupJoinRequest{InviteToken: "missing"}); err == nil {
		t.Fatal("JoinFriendGroup missing token error = nil")
	}
	defaultStore := kv.NewMemory(nil)
	defaultClock := &Server{
		Groups:            defaultStore,
		Members:           defaultStore,
		RelationshipStore: defaultStore,
		Workspaces:        &recordingWorkspaceService{},
		SFUURL:            "wss://sfu.test",
	}
	if _, err := defaultClock.CreateFriendGroup(ctx, "peer-z", rpcapi.FriendGroupCreateRequest{Name: "room"}); err != nil {
		t.Fatalf("CreateFriendGroup with default clock: %v", err)
	}

	a := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	b := a.Add(time.Second)
	if !socialutil.CompareByCreatedAtAsc(a, "a", b, "b") || !socialutil.CompareByCreatedAtAsc(a, "a", a, "b") || socialutil.CompareByCreatedAtAsc(b, "b", a, "a") {
		t.Fatal("CompareByCreatedAtAsc returned unexpected ordering")
	}
	if !socialutil.CompareByCreatedAtDesc(b, "b", a, "a") || !socialutil.CompareByCreatedAtDesc(a, "b", a, "a") || socialutil.CompareByCreatedAtDesc(a, "a", b, "b") {
		t.Fatal("CompareByCreatedAtDesc returned unexpected ordering")
	}
	if role := socialutil.GroupRole(rpcapi.FriendGroupMemberObject{}); role != "" {
		t.Fatalf("GroupRole without role = %q, want empty", role)
	}
	if id := (&Server{}).newID(); id == "" {
		t.Fatal("newID without override returned empty string")
	}
}

func TestCreateRollsBackPartialWrites(t *testing.T) {
	ctx := context.Background()
	groupStore := kv.NewMemory(nil)
	s := newTestServer(t)
	s.Groups = groupStore
	s.Members = failingSetStore{Store: kv.NewMemory(nil)}

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err == nil {
		t.Fatal("CreateFriendGroup with failing member store error = nil")
	}
	if group.Name != "" {
		t.Fatalf("CreateFriendGroup returned partial group = %#v", group)
	}
	var groups []kv.Entry
	for entry, err := range groupStore.List(ctx, socialutil.GroupsRoot) {
		if err != nil {
			t.Fatalf("list groups after rollback: %v", err)
		}
		groups = append(groups, entry)
	}
	if len(groups) != 0 {
		t.Fatalf("groups after rollback = %#v, want empty", groups)
	}

	workspaces := &recordingWorkspaceService{}
	s = newTestServer(t)
	s.Groups = failingSetStore{Store: kv.NewMemory(nil)}
	s.Workspaces = workspaces
	if _, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"}); err == nil {
		t.Fatal("CreateFriendGroup with failing group store error = nil")
	}
	if len(workspaces.deleted) != 1 {
		t.Fatalf("deleted workspaces after group write rollback = %#v, want one", workspaces.deleted)
	}
}

func TestFilteredListsPaginateAfterFilteringAndSortNewestFirst(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)

	if _, err := s.CreateFriendGroup(ctx, "peer-x", rpcapi.FriendGroupCreateRequest{Name: "other"}); err != nil {
		t.Fatalf("CreateFriendGroup unrelated: %v", err)
	}
	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup visible: %v", err)
	}
	friendGroups, err := s.ListFriendGroups(ctx, "peer-a", rpcapi.FriendGroupListRequest{Limit: new(1)})
	if err != nil {
		t.Fatalf("ListFriendGroups: %v", err)
	}
	if len(friendGroups.Items) != 1 || friendGroups.Items[0].Name != group.Name || friendGroups.HasNext {
		t.Fatalf("ListFriendGroups page = %#v, want only visible group without next page", friendGroups)
	}

}

func TestCreateMemberAtomicallyClaimsIdentityAcrossServers(t *testing.T) {
	t.Parallel()

	primary := newTestServer(t)
	friendGroupID := "group-atomic"
	secondary := &Server{
		Groups:            primary.Groups,
		InviteTokens:      primary.InviteTokens,
		Members:           primary.Members,
		Belongs:           primary.Belongs,
		RelationshipStore: primary.RelationshipStore,
		Workspaces:        primary.Workspaces,
		SFUURL:            primary.SFUURL,
		Now:               primary.Now,
	}
	servers := []*Server{primary, secondary}
	roles := []rpcapi.FriendGroupMemberRole{
		rpcapi.FriendGroupMemberRoleMember,
		rpcapi.FriendGroupMemberRoleAdmin,
	}
	type result struct {
		member rpcapi.FriendGroupMemberObject
		err    error
	}
	start := make(chan struct{})
	results := make(chan result, len(servers))
	for i, server := range servers {
		go func(server *Server, role rpcapi.FriendGroupMemberRole) {
			<-start
			member, err := server.createMember(t.Context(), friendGroupID, "peer-b", role, "room-b")
			results <- result{member: member, err: err}
		}(server, roles[i])
	}
	close(start)

	var winner rpcapi.FriendGroupMemberObject
	created := 0
	conflicts := 0
	for range servers {
		result := <-results
		switch {
		case result.err == nil:
			winner = result.member
			created++
		case errors.Is(result.err, ErrFriendGroupMemberAlreadyExists):
			conflicts++
		default:
			t.Fatalf("createMember() error = %v", result.err)
		}
	}
	if created != 1 || conflicts != 1 {
		t.Fatalf("create results = %d created, %d conflicts; want 1 each", created, conflicts)
	}
	stored, err := primary.groupMember(t.Context(), friendGroupID, "peer-b")
	if err != nil {
		t.Fatalf("groupMember() error = %v", err)
	}
	if socialutil.StringValue(stored.FriendGroupName) != "room-b" || stored.Role == nil || winner.Role == nil || *stored.Role != *winner.Role {
		t.Fatalf("stored member = %#v, winner = %#v", stored, winner)
	}
	assertBelongs(t, t.Context(), primary, "peer-b", friendGroupID, "room-b", *winner.Role)
	groupID, err := primary.Belongs.Get(t.Context(), socialutil.GroupNameKey("peer-b", "room-b"))
	if err != nil {
		t.Fatalf("group name index error = %v", err)
	}
	if string(groupID) != friendGroupID {
		t.Fatalf("group name index = %q, want %q", groupID, friendGroupID)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	store := kv.NewMemory(nil)
	now := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	nextID := 0
	return &Server{
		Groups:            store,
		InviteTokens:      store,
		Members:           store,
		Belongs:           store,
		RelationshipStore: store,
		Workspaces:        &recordingWorkspaceService{},
		SFUURL:            "wss://sfu.test",
		Now:               func() time.Time { return now },
		NewID: func() string {
			nextID++
			return "id-" + string(rune('a'+nextID-1))
		},
	}
}

func mustGroupID(t *testing.T, s *Server, peerID, name string) string {
	t.Helper()
	id, err := s.resolveFriendGroupName(t.Context(), peerID, name)
	if err != nil {
		t.Fatalf("resolve friend group %s/%s: %v", peerID, name, err)
	}
	return id
}

func assertBelongs(t *testing.T, ctx context.Context, s *Server, peerID, friendGroupID, wantName string, wantRole rpcapi.FriendGroupMemberRole) {
	t.Helper()
	belongs, err := s.belongsStore()
	if err != nil {
		t.Fatalf("belongsStore: %v", err)
	}
	item, err := socialutil.ReadJSONValue[friendGroupMemberRecord](ctx, belongs, socialutil.GroupBelongKey(peerID, friendGroupID))
	if err != nil {
		t.Fatalf("group belong %s/%s: %v", peerID, friendGroupID, err)
	}
	if err := item.validate(); err != nil {
		t.Fatalf("group belong %s/%s is invalid: %v", peerID, friendGroupID, err)
	}
	if got := item.FriendGroupName; got != wantName {
		t.Fatalf("belong friend_group_name = %q, want %q", got, wantName)
	}
	if got := item.PeerPublicKey; got != peerID {
		t.Fatalf("belong peer_public_key = %q, want %q", got, peerID)
	}
	if got := item.Role; got != wantRole {
		t.Fatalf("belong role = %q, want %q", got, wantRole)
	}
}

func assertNoBelongs(t *testing.T, ctx context.Context, s *Server, peerID, friendGroupID string) {
	t.Helper()
	belongs, err := s.belongsStore()
	if err != nil {
		t.Fatalf("belongsStore: %v", err)
	}
	if _, err := socialutil.ReadJSONValue[friendGroupMemberRecord](ctx, belongs, socialutil.GroupBelongKey(peerID, friendGroupID)); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("group belong %s/%s error = %v, want not found", peerID, friendGroupID, err)
	}
}

type failingSetStore struct {
	kv.Store
}

type blockingGroupListStore struct {
	kv.Store
	prefix           kv.Key
	entered, release chan struct{}
	once             sync.Once
}

func (s *blockingGroupListStore) List(ctx context.Context, prefix kv.Key) iter.Seq2[kv.Entry, error] {
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

func (s failingSetStore) Set(context.Context, kv.Key, []byte) error {
	return errors.New("forced set failure")
}

func (s failingSetStore) CreateIfAbsent(context.Context, kv.Entry, []kv.Entry) ([]byte, bool, error) {
	return nil, false, errors.New("forced set failure")
}

type failAfterGetStore struct {
	kv.Store
	failAfter int
	count     int
}

func (s *failAfterGetStore) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	s.count++
	if s.count > s.failAfter {
		return nil, errors.New("forced get failure")
	}
	return s.Store.Get(ctx, key)
}

type failingBatchMutateStore struct {
	kv.Store
}

func (s failingBatchMutateStore) BatchMutate(context.Context, []kv.Entry, []kv.Key) error {
	return errors.New("forced batch mutate failure")
}

type storeWithoutCreateIfAbsent struct {
	store kv.Store
}

func (s storeWithoutCreateIfAbsent) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	return s.store.Get(ctx, key)
}

func (s storeWithoutCreateIfAbsent) Set(ctx context.Context, key kv.Key, value []byte) error {
	return s.store.Set(ctx, key, value)
}

func (s storeWithoutCreateIfAbsent) Delete(ctx context.Context, key kv.Key) error {
	return s.store.Delete(ctx, key)
}

func (s storeWithoutCreateIfAbsent) List(ctx context.Context, prefix kv.Key) iter.Seq2[kv.Entry, error] {
	return s.store.List(ctx, prefix)
}

func (s storeWithoutCreateIfAbsent) BatchSet(ctx context.Context, entries []kv.Entry) error {
	return s.store.BatchSet(ctx, entries)
}

func (s storeWithoutCreateIfAbsent) BatchDelete(ctx context.Context, keys []kv.Key) error {
	return s.store.BatchDelete(ctx, keys)
}

func (s storeWithoutCreateIfAbsent) BatchMutate(
	ctx context.Context,
	entries []kv.Entry,
	keys []kv.Key,
) error {
	return s.store.BatchMutate(ctx, entries, keys)
}

func (s storeWithoutCreateIfAbsent) Close() error {
	return s.store.Close()
}

type recordingWorkspaceService struct {
	created      []adminhttp.WorkspaceUpsert
	deleted      []string
	retired      []string
	owners       []string
	retiredOwner string
	retireErr    error
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

func (s *recordingWorkspaceService) RetireSystemWorkspace(_ context.Context, name string, _ socialutil.SFUWorkspaceKind, _ string) (apitypes.Workspace, error) {
	s.retired = append(s.retired, name)
	owner := s.retiredOwner
	var ownerPointer *string
	if owner != "" {
		ownerPointer = &owner
	}
	return apitypes.Workspace{Name: name, OwnerPublicKey: ownerPointer}, s.retireErr
}

func (s *recordingWorkspaceService) RetireSystemWorkspaceByID(_ context.Context, id string, _ socialutil.SFUWorkspaceKind, _ string) (apitypes.Workspace, error) {
	s.retired = append(s.retired, id)
	owner := s.retiredOwner
	var ownerPointer *string
	if owner != "" {
		ownerPointer = &owner
	}
	return apitypes.Workspace{Id: id, OwnerPublicKey: ownerPointer}, s.retireErr
}

func (s *recordingWorkspaceService) GetRetiredSystemWorkspace(_ context.Context, name string, _ socialutil.SFUWorkspaceKind, _ string) (apitypes.Workspace, error) {
	if len(s.retired) == 0 {
		return apitypes.Workspace{}, kv.ErrNotFound
	}
	owner := s.retiredOwner
	var ownerPointer *string
	if owner != "" {
		ownerPointer = &owner
	}
	return apitypes.Workspace{Name: name, OwnerPublicKey: ownerPointer}, nil
}

func (s *recordingWorkspaceService) GetRetiredSystemWorkspaceByID(_ context.Context, id string, _ socialutil.SFUWorkspaceKind, _ string) (apitypes.Workspace, error) {
	if len(s.retired) == 0 {
		return apitypes.Workspace{}, kv.ErrNotFound
	}
	owner := s.retiredOwner
	var ownerPointer *string
	if owner != "" {
		ownerPointer = &owner
	}
	return apitypes.Workspace{Id: id, OwnerPublicKey: ownerPointer}, nil
}

func (s *recordingWorkspaceService) DeleteWorkspace(_ context.Context, req adminhttp.DeleteWorkspaceRequestObject) (adminhttp.DeleteWorkspaceResponseObject, error) {
	s.deleted = append(s.deleted, req.Id)
	return adminhttp.DeleteWorkspace200JSONResponse(apitypes.Workspace{Name: req.Id}), nil
}

type failingWorkspaceService struct {
	createErr error
}

func (s failingWorkspaceService) CreateSystemWorkspace(context.Context, adminhttp.WorkspaceUpsert) (apitypes.Workspace, bool, error) {
	if s.createErr != nil {
		return apitypes.Workspace{}, false, s.createErr
	}
	system := true
	return apitypes.Workspace{System: &system}, true, nil
}

func (s failingWorkspaceService) DeleteSystemWorkspace(context.Context, string) (apitypes.Workspace, error) {
	return apitypes.Workspace{}, kv.ErrNotFound
}

func (s failingWorkspaceService) GetWorkspaceByName(context.Context, string) (apitypes.Workspace, error) {
	return apitypes.Workspace{}, kv.ErrNotFound
}

func (s failingWorkspaceService) RetireSystemWorkspaceByID(context.Context, string, socialutil.SFUWorkspaceKind, string) (apitypes.Workspace, error) {
	return apitypes.Workspace{}, kv.ErrNotFound
}

func (s failingWorkspaceService) GetRetiredSystemWorkspaceByID(context.Context, string, socialutil.SFUWorkspaceKind, string) (apitypes.Workspace, error) {
	return apitypes.Workspace{}, kv.ErrNotFound
}

func (s failingWorkspaceService) DeleteWorkspace(context.Context, adminhttp.DeleteWorkspaceRequestObject) (adminhttp.DeleteWorkspaceResponseObject, error) {
	return adminhttp.DeleteWorkspace404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", "missing")), nil
}

//go:fix inline
func strPtr(v string) *string {
	return new(v)
}
