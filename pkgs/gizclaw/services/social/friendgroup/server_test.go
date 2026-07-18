package friendgroup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	_ "modernc.org/sqlite"
)

func TestRolesAudioMessagesAndTTL(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	s.MessageDefaultTTL = time.Second
	s.MessageMaxAudioBytes = 16

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := socialutil.StringValue(group.Id)
	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: friendGroupID, PeerPublicKey: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("member")}); err != nil {
		t.Fatalf("AddFriendGroupMember member: %v", err)
	}
	if _, err := s.PutFriendGroupMember(ctx, "peer-b", rpcapi.FriendGroupMemberPutRequest{FriendGroupId: friendGroupID, Id: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("admin")}); err == nil {
		t.Fatal("PutFriendGroupMember by member error = nil")
	}
	if _, err := s.PutFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberPutRequest{FriendGroupId: friendGroupID, Id: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("admin")}); err != nil {
		t.Fatalf("PutFriendGroupMember by owner: %v", err)
	}
	if _, err := s.AddFriendGroupMember(ctx, "peer-b", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: friendGroupID, PeerPublicKey: "peer-c", Role: rpcapi.FriendGroupMemberMutableRole("member")}); err != nil {
		t.Fatalf("AddFriendGroupMember by admin: %v", err)
	}
	if _, err := s.AddFriendGroupMember(ctx, "peer-b", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: friendGroupID, PeerPublicKey: "peer-d", Role: rpcapi.FriendGroupMemberMutableRole("admin")}); err == nil {
		t.Fatal("admin adding admin error = nil")
	}
	if _, err := s.GetFriendGroup(ctx, "peer-d", rpcapi.FriendGroupGetRequest{Id: friendGroupID}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("GetFriendGroup by non-member error = %v, want kv.ErrNotFound", err)
	}

	msg, err := s.SendFriendGroupMessage(ctx, "peer-b", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    " " + friendGroupID + " ",
		AudioBase64:      []byte("opus"),
		AudioContentType: "audio/opus",
	})
	if err != nil {
		t.Fatalf("SendFriendGroupMessage: %v", err)
	}
	if msg.AudioPath == nil || strings.Contains(*msg.AudioPath, "..") || filepath.IsAbs(*msg.AudioPath) {
		t.Fatalf("audio_path = %v", msg.AudioPath)
	}
	rc, err := s.MessageAssets.Get(socialutil.StringValue(msg.AudioPath))
	if err != nil {
		t.Fatalf("Get audio object: %v", err)
	}
	data, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(data) != "opus" {
		t.Fatalf("audio bytes = %q", data)
	}
	if _, err := s.SendFriendGroupMessage(ctx, "peer-b", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    friendGroupID,
		AudioBase64:      []byte("0123456789abcdefg"),
		AudioContentType: "audio/opus",
	}); err == nil {
		t.Fatal("oversized SendFriendGroupMessage error = nil")
	}
	if _, err := s.GetFriendGroupMessage(ctx, "peer-c", rpcapi.FriendGroupMessageGetRequest{FriendGroupId: friendGroupID, Id: socialutil.StringValue(msg.Id)}); err != nil {
		t.Fatalf("GetFriendGroupMessage by member: %v", err)
	}
	if _, err := s.SendFriendGroupMessage(ctx, "peer-d", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    friendGroupID,
		AudioBase64:      []byte("opus"),
		AudioContentType: "audio/opus",
	}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("SendFriendGroupMessage by non-member error = %v, want kv.ErrNotFound", err)
	}

	s.Now = func() time.Time { return time.Date(2026, 6, 13, 0, 0, 2, 0, time.UTC) }
	if _, err := s.GetFriendGroupMessage(ctx, "peer-c", rpcapi.FriendGroupMessageGetRequest{FriendGroupId: friendGroupID, Id: socialutil.StringValue(msg.Id)}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("GetFriendGroupMessage expired error = %v, want kv.ErrNotFound", err)
	}
	if err := s.CleanupExpiredFriendGroupMessages(ctx); err != nil {
		t.Fatalf("CleanupExpiredFriendGroupMessages: %v", err)
	}
	if _, err := s.MessageAssets.Get(socialutil.StringValue(msg.AudioPath)); err == nil {
		t.Fatal("expired audio object still exists")
	}
}

func TestAdminApplyFriendGroupRollsBackWorkspaceOnGroupWriteFailure(t *testing.T) {
	ctx := context.Background()
	workspaces := &recordingWorkspaceService{}
	s := newTestServer(t)
	s.Workspaces = workspaces
	s.Groups = failingSetStore{Store: kv.NewMemory(nil)}

	if _, err := s.AdminApplyFriendGroup(ctx, "family01", "Family", nil); err == nil {
		t.Fatal("AdminApplyFriendGroup with failing group store error = nil")
	}
	if len(workspaces.deleted) != 1 || workspaces.deleted[0] != socialutil.GroupWorkspaceName("family01") {
		t.Fatalf("deleted workspaces = %#v, want family01 workspace rollback", workspaces.deleted)
	}
}

func TestAdminDeleteFriendGroupMemberRollsBackWhenBelongsDeleteFails(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	group, err := s.AdminApplyFriendGroup(ctx, "family01", "Family", nil)
	if err != nil {
		t.Fatalf("AdminApplyFriendGroup: %v", err)
	}
	friendGroupID := socialutil.StringValue(group.Id)
	if _, err := s.AdminPutFriendGroupMember(ctx, friendGroupID, "peer-b", rpcapi.FriendGroupMemberRoleMember); err != nil {
		t.Fatalf("AdminPutFriendGroupMember: %v", err)
	}
	s.Belongs = failingDeleteStore{Store: s.Belongs}
	if _, err := s.AdminDeleteFriendGroupMember(ctx, friendGroupID, "peer-b"); err == nil {
		t.Fatal("AdminDeleteFriendGroupMember with failing belongs delete error = nil")
	}
	if _, err := s.groupMember(ctx, friendGroupID, "peer-b"); err != nil {
		t.Fatalf("groupMember after failed admin delete = %v, want restored", err)
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
	friendGroupID := socialutil.StringValue(group.Id)
	assertBelongs(t, ctx, s, "peer-a", friendGroupID, rpcapi.FriendGroupMemberRoleOwner)

	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: friendGroupID, PeerPublicKey: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("member")}); err != nil {
		t.Fatalf("AddFriendGroupMember: %v", err)
	}
	groups, err := s.ListFriendGroups(ctx, "peer-b", rpcapi.FriendGroupListRequest{})
	if err != nil {
		t.Fatalf("ListFriendGroups peer-b: %v", err)
	}
	if len(groups.Items) != 1 || socialutil.StringValue(groups.Items[0].Id) != friendGroupID || groups.Items[0].MyRole == nil || *groups.Items[0].MyRole != rpcapi.FriendGroupMemberRoleMember {
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
	friendGroupID := socialutil.StringValue(group.Id)
	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: friendGroupID, PeerPublicKey: "peer-a", Role: rpcapi.FriendGroupMemberMutableRole("member")}); err == nil {
		t.Fatal("AddFriendGroupMember owner role change error = nil")
	}
	ownerMember, err := s.groupMember(ctx, friendGroupID, "peer-a")
	if err != nil {
		t.Fatalf("owner groupMember after failed add: %v", err)
	}
	if got := socialutil.GroupRole(ownerMember); got != rpcapi.FriendGroupMemberRoleOwner {
		t.Fatalf("owner role after failed add = %q, want owner", got)
	}
	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: friendGroupID, PeerPublicKey: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("member")}); err != nil {
		t.Fatalf("AddFriendGroupMember peer-b: %v", err)
	}
	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: friendGroupID, PeerPublicKey: "peer-c", Role: rpcapi.FriendGroupMemberMutableRole("admin")}); err != nil {
		t.Fatalf("AddFriendGroupMember peer-c admin: %v", err)
	}
	if _, err := s.DeleteFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupId: friendGroupID, Id: "peer-a"}); err == nil {
		t.Fatal("DeleteFriendGroupMember owner error = nil")
	}
	if _, err := s.DeleteFriendGroupMember(ctx, "peer-b", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupId: friendGroupID, Id: "peer-c"}); err == nil {
		t.Fatal("DeleteFriendGroupMember admin by member error = nil")
	}
	deletedAdmin, err := s.DeleteFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupId: friendGroupID, Id: "peer-c"})
	if err != nil {
		t.Fatalf("DeleteFriendGroupMember admin by owner: %v", err)
	}
	if socialutil.StringValue(deletedAdmin.PeerPublicKey) != "peer-c" {
		t.Fatalf("deleted admin peer_public_key = %q, want peer-c", socialutil.StringValue(deletedAdmin.PeerPublicKey))
	}
	selfDeleted, err := s.DeleteFriendGroupMember(ctx, "peer-b", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupId: friendGroupID, Id: "peer-b"})
	if err != nil {
		t.Fatalf("DeleteFriendGroupMember self member: %v", err)
	}
	if socialutil.StringValue(selfDeleted.PeerPublicKey) != "peer-b" {
		t.Fatalf("self deleted peer_public_key = %q, want peer-b", socialutil.StringValue(selfDeleted.PeerPublicKey))
	}
}

func TestConfigurationErrorsAndHelpers(t *testing.T) {
	ctx := context.Background()
	empty := &Server{}
	if _, err := empty.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"}); err == nil {
		t.Fatal("CreateFriendGroup without store error = nil")
	}
	if _, err := empty.ListFriendGroupMembers(ctx, "peer-a", rpcapi.FriendGroupMemberListRequest{FriendGroupId: strPtr("group-a")}); err == nil {
		t.Fatal("ListFriendGroupMembers without store error = nil")
	}
	if _, err := empty.SendFriendGroupMessage(ctx, "peer-a", rpcapi.FriendGroupMessageSendRequest{FriendGroupId: "group-a", AudioContentType: "audio/opus"}); err == nil {
		t.Fatal("SendFriendGroupMessage without store error = nil")
	}
	if _, err := empty.AdminApplyFriendGroup(ctx, "group-a", "Group A", nil); err == nil {
		t.Fatal("AdminApplyFriendGroup without store error = nil")
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
	friendGroupID := socialutil.StringValue(group.Id)
	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: friendGroupID, Role: rpcapi.FriendGroupMemberMutableRole("member")}); err == nil {
		t.Fatal("AddFriendGroupMember empty peer public key error = nil")
	}
	if _, err := s.AdminPutFriendGroup(ctx, "", strPtr("renamed"), nil); err == nil {
		t.Fatal("AdminPutFriendGroup empty id error = nil")
	}
	if _, err := s.AdminDeleteFriendGroup(ctx, ""); err == nil {
		t.Fatal("AdminDeleteFriendGroup empty id error = nil")
	}
	if _, err := s.AdminListFriendGroupMembers(ctx, "missing", rpcapi.FriendGroupMemberListRequest{}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("AdminListFriendGroupMembers missing group error = %v, want kv.ErrNotFound", err)
	}
	if _, err := s.AdminPutFriendGroupMember(ctx, "missing", "peer-b", rpcapi.FriendGroupMemberRoleMember); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("AdminPutFriendGroupMember missing group error = %v, want kv.ErrNotFound", err)
	}
	if _, err := s.groupMember(ctx, "missing", "peer-b"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("groupMember after rejected admin put error = %v, want kv.ErrNotFound", err)
	}
	if _, err := s.AdminPutFriendGroupMember(ctx, friendGroupID, "peer-b", rpcapi.FriendGroupMemberRole("observer")); err == nil {
		t.Fatal("AdminPutFriendGroupMember invalid role error = nil")
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
	if _, err := s.SendFriendGroupMessage(ctx, "peer-a", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    friendGroupID,
		AudioBase64:      []byte("opus"),
		AudioContentType: "audio/wav",
	}); err == nil {
		t.Fatal("SendFriendGroupMessage unsupported content type error = nil")
	}
	noAssets := *s
	noAssets.MessageAssets = nil
	if _, err := noAssets.SendFriendGroupMessage(ctx, "peer-a", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    friendGroupID,
		AudioBase64:      []byte("opus"),
		AudioContentType: "audio/opus",
	}); err == nil {
		t.Fatal("SendFriendGroupMessage without assets error = nil")
	}
	s.MessageMaxTTL = time.Second
	if _, err := s.SendFriendGroupMessage(ctx, "peer-a", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    friendGroupID,
		AudioBase64:      []byte("opus"),
		AudioContentType: "audio/opus",
		TtlSeconds:       socialutil.IntPtr(2),
	}); err == nil {
		t.Fatal("SendFriendGroupMessage exceeding max ttl error = nil")
	}
	defaultClock := &Server{Groups: kv.NewMemory(nil), Members: kv.NewMemory(nil), Messages: kv.NewMemory(nil), MessageAssets: objectstore.Dir(t.TempDir())}
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
	if socialutil.StringValue(group.Id) != "" {
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

func TestSendMessageDeletesObjectWhenMetadataWriteFails(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	baseAssets := s.MessageAssets
	s.Messages = failingSetStore{Store: kv.NewMemory(nil)}

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	if _, err := s.SendFriendGroupMessage(ctx, "peer-a", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    socialutil.StringValue(group.Id),
		AudioBase64:      []byte("opus"),
		AudioContentType: "audio/opus",
	}); err == nil {
		t.Fatal("SendFriendGroupMessage with failing metadata store error = nil")
	}
	objects, err := baseAssets.List("")
	if err != nil {
		t.Fatalf("List message assets: %v", err)
	}
	if len(objects) != 0 {
		t.Fatalf("message assets after failed send = %#v, want empty", objects)
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
	friendGroups, err := s.ListFriendGroups(ctx, "peer-a", rpcapi.FriendGroupListRequest{Limit: socialutil.IntPtr(1)})
	if err != nil {
		t.Fatalf("ListFriendGroups: %v", err)
	}
	if len(friendGroups.Items) != 1 || socialutil.StringValue(friendGroups.Items[0].Id) != socialutil.StringValue(group.Id) || friendGroups.HasNext {
		t.Fatalf("ListFriendGroups page = %#v, want only visible group without next page", friendGroups)
	}

	olderMessage, err := s.SendFriendGroupMessage(ctx, "peer-a", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    socialutil.StringValue(group.Id),
		AudioBase64:      []byte("old"),
		AudioContentType: "audio/opus",
	})
	if err != nil {
		t.Fatalf("SendFriendGroupMessage older: %v", err)
	}
	newerMessage, err := s.SendFriendGroupMessage(ctx, "peer-a", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    socialutil.StringValue(group.Id),
		AudioBase64:      []byte("new"),
		AudioContentType: "audio/opus",
	})
	if err != nil {
		t.Fatalf("SendFriendGroupMessage newer: %v", err)
	}
	messages, err := s.ListFriendGroupMessages(ctx, "peer-a", rpcapi.FriendGroupMessageListRequest{FriendGroupId: group.Id, Limit: socialutil.IntPtr(1)})
	if err != nil {
		t.Fatalf("ListFriendGroupMessages first page: %v", err)
	}
	if len(messages.Items) != 1 || socialutil.StringValue(messages.Items[0].Id) != socialutil.StringValue(newerMessage.Id) || !messages.HasNext || messages.NextCursor == nil {
		t.Fatalf("ListFriendGroupMessages first page = %#v, want newest message and next cursor", messages)
	}
	messages, err = s.ListFriendGroupMessages(ctx, "peer-a", rpcapi.FriendGroupMessageListRequest{FriendGroupId: group.Id, Limit: socialutil.IntPtr(1), Cursor: messages.NextCursor})
	if err != nil {
		t.Fatalf("ListFriendGroupMessages second page: %v", err)
	}
	if len(messages.Items) != 1 || socialutil.StringValue(messages.Items[0].Id) != socialutil.StringValue(olderMessage.Id) || messages.HasNext {
		t.Fatalf("ListFriendGroupMessages second page = %#v, want older message without next page", messages)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	store := kv.NewMemory(nil)
	now := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	nextID := 0
	return &Server{
		Groups:        store,
		InviteTokens:  store,
		Members:       store,
		Belongs:       store,
		Messages:      store,
		MessageAssets: objectstore.Dir(t.TempDir()),
		Now:           func() time.Time { return now },
		NewID: func() string {
			nextID++
			return "id-" + string(rune('a'+nextID-1))
		},
	}
}

func assertBelongs(t *testing.T, ctx context.Context, s *Server, peerID, friendGroupID string, wantRole rpcapi.FriendGroupMemberRole) {
	t.Helper()
	belongs, err := s.belongsStore()
	if err != nil {
		t.Fatalf("belongsStore: %v", err)
	}
	item, err := socialutil.ReadJSONValue[rpcapi.FriendGroupMemberObject](ctx, belongs, socialutil.GroupBelongKey(peerID, friendGroupID))
	if err != nil {
		t.Fatalf("group belong %s/%s: %v", peerID, friendGroupID, err)
	}
	if got := socialutil.StringValue(item.FriendGroupId); got != friendGroupID {
		t.Fatalf("belong friend_group_id = %q, want %q", got, friendGroupID)
	}
	if got := socialutil.StringValue(item.PeerPublicKey); got != peerID {
		t.Fatalf("belong peer_public_key = %q, want %q", got, peerID)
	}
	if got := socialutil.GroupRole(item); got != wantRole {
		t.Fatalf("belong role = %q, want %q", got, wantRole)
	}
}

func assertNoBelongs(t *testing.T, ctx context.Context, s *Server, peerID, friendGroupID string) {
	t.Helper()
	belongs, err := s.belongsStore()
	if err != nil {
		t.Fatalf("belongsStore: %v", err)
	}
	if _, err := socialutil.ReadJSONValue[rpcapi.FriendGroupMemberObject](ctx, belongs, socialutil.GroupBelongKey(peerID, friendGroupID)); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("group belong %s/%s error = %v, want not found", peerID, friendGroupID, err)
	}
}

type failingSetStore struct {
	kv.Store
}

func (s failingSetStore) Set(context.Context, kv.Key, []byte) error {
	return errors.New("forced set failure")
}

type failingDeleteStore struct {
	kv.Store
}

func (s failingDeleteStore) Delete(context.Context, kv.Key) error {
	return errors.New("forced delete failure")
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

type failingDeletePrefixStore struct {
	objectstore.ObjectStore
}

func (s failingDeletePrefixStore) DeletePrefix(string) error {
	return errors.New("forced delete prefix failure")
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

type failingWorkspaceService struct {
	createResp adminhttp.CreateWorkspaceResponseObject
	createErr  error
}

func (s failingWorkspaceService) CreateSystemWorkspace(context.Context, adminhttp.WorkspaceUpsert) (apitypes.Workspace, bool, error) {
	if s.createErr != nil {
		return apitypes.Workspace{}, false, s.createErr
	}
	if s.createResp != nil {
		return apitypes.Workspace{}, false, fmt.Errorf("create system workspace failed: %T", s.createResp)
	}
	system := true
	return apitypes.Workspace{System: &system}, true, nil
}

func (s failingWorkspaceService) DeleteSystemWorkspace(context.Context, string) (apitypes.Workspace, error) {
	return apitypes.Workspace{}, kv.ErrNotFound
}

func (s failingWorkspaceService) CreateWorkspace(context.Context, adminhttp.CreateWorkspaceRequestObject) (adminhttp.CreateWorkspaceResponseObject, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	return s.createResp, nil
}

func (s failingWorkspaceService) DeleteWorkspace(context.Context, adminhttp.DeleteWorkspaceRequestObject) (adminhttp.DeleteWorkspaceResponseObject, error) {
	return adminhttp.DeleteWorkspace404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", "missing")), nil
}

func strPtr(v string) *string {
	return &v
}
