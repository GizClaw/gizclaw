package friendgroup

import (
	"context"
	"database/sql"
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
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
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

func TestMembersMaintainBelongsAndWorkspaceACLBindings(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	baseACL := newTestACL(t)
	s.ACL = baseACL
	workspaces := &recordingWorkspaceService{}
	s.Workspaces = workspaces

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := socialutil.StringValue(group.Id)
	workspaceName := socialutil.StringValue(group.WorkspaceName)
	if workspaceName == "" {
		t.Fatal("CreateFriendGroup workspace_name is empty")
	}
	if len(workspaces.created) != 1 || workspaces.created[0].Name != workspaceName || workspaces.created[0].WorkflowName != socialutil.ChatRoomWorkflowName {
		t.Fatalf("created workspaces = %#v, want %q chatroom", workspaces.created, workspaceName)
	}
	assertBelongs(t, ctx, s, "peer-a", friendGroupID, rpcapi.FriendGroupMemberRoleOwner)
	if err := baseACL.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject("peer-a"),
		Resource:   acl.WorkspaceResource(workspaceName),
		Permission: apitypes.ACLPermissionUse,
	}); err != nil {
		t.Fatalf("owner workspace use authorize: %v", err)
	}

	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: friendGroupID, PeerPublicKey: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("member")}); err != nil {
		t.Fatalf("AddFriendGroupMember: %v", err)
	}
	assertBelongs(t, ctx, s, "peer-b", friendGroupID, rpcapi.FriendGroupMemberRoleMember)
	if err := baseACL.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject("peer-b"),
		Resource:   acl.WorkspaceResource(workspaceName),
		Permission: apitypes.ACLPermissionUse,
	}); err != nil {
		t.Fatalf("member workspace use authorize: %v", err)
	}
	peerBGroups, err := s.ListFriendGroups(ctx, " peer-b ", rpcapi.FriendGroupListRequest{})
	if err != nil {
		t.Fatalf("ListFriendGroups peer-b: %v", err)
	}
	if len(peerBGroups.Items) != 1 || socialutil.StringValue(peerBGroups.Items[0].Id) != friendGroupID || peerBGroups.Items[0].MyRole == nil || *peerBGroups.Items[0].MyRole != rpcapi.FriendGroupMemberRoleMember {
		t.Fatalf("ListFriendGroups peer-b = %#v, want member group", peerBGroups)
	}

	if _, err := s.PutFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberPutRequest{FriendGroupId: friendGroupID, Id: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("admin")}); err != nil {
		t.Fatalf("PutFriendGroupMember: %v", err)
	}
	assertBelongs(t, ctx, s, "peer-b", friendGroupID, rpcapi.FriendGroupMemberRoleAdmin)

	if _, err := s.DeleteFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupId: friendGroupID, Id: "peer-b"}); err != nil {
		t.Fatalf("DeleteFriendGroupMember: %v", err)
	}
	assertNoBelongs(t, ctx, s, "peer-b", friendGroupID)
	if err := baseACL.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject("peer-b"),
		Resource:   acl.WorkspaceResource(workspaceName),
		Permission: apitypes.ACLPermissionUse,
	}); !errors.Is(err, acl.ErrDenied) {
		t.Fatalf("deleted member workspace use authorize error = %v, want denied", err)
	}
	peerBGroups, err = s.ListFriendGroups(ctx, "peer-b", rpcapi.FriendGroupListRequest{})
	if err != nil {
		t.Fatalf("ListFriendGroups peer-b after delete: %v", err)
	}
	if len(peerBGroups.Items) != 0 {
		t.Fatalf("ListFriendGroups peer-b after delete = %#v, want empty", peerBGroups)
	}
}

func TestAdminFriendGroupLifecycleMaintainsMembersBelongsAndWorkspaceACL(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	baseACL := newTestACL(t)
	s.ACL = baseACL
	workspaces := &recordingWorkspaceService{}
	s.Workspaces = workspaces

	group, err := s.AdminCreateFriendGroup(ctx, "room", strPtr("admin room"))
	if err != nil {
		t.Fatalf("AdminCreateFriendGroup: %v", err)
	}
	friendGroupID := socialutil.StringValue(group.Id)
	workspaceName := socialutil.StringValue(group.WorkspaceName)
	if friendGroupID == "" || workspaceName == "" {
		t.Fatalf("AdminCreateFriendGroup returned %#v", group)
	}
	if group.CreatedByPeerPublicKey != nil || group.MyRole != nil {
		t.Fatalf("admin-created group has peer fields: %#v", group)
	}
	if len(workspaces.created) != 1 || workspaces.created[0].Name != workspaceName {
		t.Fatalf("created workspaces = %#v, want %q", workspaces.created, workspaceName)
	}
	if _, err := s.groupMember(ctx, friendGroupID, "peer-owner"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("implicit member error = %v, want not found", err)
	}

	groups, err := s.AdminListFriendGroups(ctx, rpcapi.FriendGroupListRequest{})
	if err != nil {
		t.Fatalf("AdminListFriendGroups: %v", err)
	}
	if len(groups.Items) != 1 || socialutil.StringValue(groups.Items[0].Id) != friendGroupID {
		t.Fatalf("AdminListFriendGroups = %#v", groups)
	}
	renamed, err := s.AdminPutFriendGroup(ctx, friendGroupID, strPtr("renamed"), strPtr(""))
	if err != nil {
		t.Fatalf("AdminPutFriendGroup: %v", err)
	}
	if socialutil.StringValue(renamed.Name) != "renamed" || renamed.Description != nil {
		t.Fatalf("renamed group = %#v", renamed)
	}

	owner, err := s.AdminPutFriendGroupMember(ctx, friendGroupID, "peer-owner", rpcapi.FriendGroupMemberRoleOwner)
	if err != nil {
		t.Fatalf("AdminPutFriendGroupMember owner: %v", err)
	}
	if socialutil.GroupRole(owner) != rpcapi.FriendGroupMemberRoleOwner {
		t.Fatalf("owner role = %q", socialutil.GroupRole(owner))
	}
	assertBelongs(t, ctx, s, "peer-owner", friendGroupID, rpcapi.FriendGroupMemberRoleOwner)
	if err := baseACL.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject("peer-owner"),
		Resource:   acl.WorkspaceResource(workspaceName),
		Permission: apitypes.ACLPermissionUse,
	}); err != nil {
		t.Fatalf("owner workspace authorize: %v", err)
	}
	member, err := s.AdminPutFriendGroupMember(ctx, friendGroupID, "peer-member", rpcapi.FriendGroupMemberRoleMember)
	if err != nil {
		t.Fatalf("AdminPutFriendGroupMember member: %v", err)
	}
	if socialutil.GroupRole(member) != rpcapi.FriendGroupMemberRoleMember {
		t.Fatalf("member role = %q", socialutil.GroupRole(member))
	}
	member, err = s.AdminPutFriendGroupMember(ctx, friendGroupID, "peer-member", rpcapi.FriendGroupMemberRoleAdmin)
	if err != nil {
		t.Fatalf("AdminPutFriendGroupMember update: %v", err)
	}
	if socialutil.GroupRole(member) != rpcapi.FriendGroupMemberRoleAdmin {
		t.Fatalf("updated member role = %q", socialutil.GroupRole(member))
	}
	assertBelongs(t, ctx, s, "peer-member", friendGroupID, rpcapi.FriendGroupMemberRoleAdmin)

	members, err := s.AdminListFriendGroupMembers(ctx, friendGroupID, rpcapi.FriendGroupMemberListRequest{})
	if err != nil {
		t.Fatalf("AdminListFriendGroupMembers: %v", err)
	}
	if len(members.Items) != 2 {
		t.Fatalf("AdminListFriendGroupMembers len = %d, want 2", len(members.Items))
	}
	if _, err := s.AdminDeleteFriendGroupMember(ctx, friendGroupID, "peer-owner"); err != nil {
		t.Fatalf("AdminDeleteFriendGroupMember owner: %v", err)
	}
	assertNoBelongs(t, ctx, s, "peer-owner", friendGroupID)
	if err := baseACL.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject("peer-owner"),
		Resource:   acl.WorkspaceResource(workspaceName),
		Permission: apitypes.ACLPermissionUse,
	}); !errors.Is(err, acl.ErrDenied) {
		t.Fatalf("owner workspace authorize after delete = %v, want denied", err)
	}

	expiresAt := s.now().Add(time.Hour)
	putToken, err := s.AdminPutFriendGroupInviteToken(ctx, friendGroupID, "admin-token", expiresAt)
	if err != nil {
		t.Fatalf("AdminPutFriendGroupInviteToken: %v", err)
	}
	if putToken.InviteToken != "admin-token" || !putToken.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("put token = %#v", putToken)
	}
	gotToken, err := s.AdminGetFriendGroupInviteToken(ctx, friendGroupID)
	if err != nil {
		t.Fatalf("AdminGetFriendGroupInviteToken: %v", err)
	}
	if gotToken.InviteToken == nil || *gotToken.InviteToken != "admin-token" {
		t.Fatalf("got token = %#v", gotToken)
	}
	if _, err := s.AdminDeleteFriendGroupInviteToken(ctx, friendGroupID); err != nil {
		t.Fatalf("AdminDeleteFriendGroupInviteToken: %v", err)
	}
	gotToken, err = s.AdminGetFriendGroupInviteToken(ctx, friendGroupID)
	if err != nil {
		t.Fatalf("AdminGetFriendGroupInviteToken cleared: %v", err)
	}
	if gotToken.InviteToken != nil || gotToken.ExpiresAt != nil {
		t.Fatalf("cleared token = %#v, want empty", gotToken)
	}

	deleted, err := s.AdminDeleteFriendGroup(ctx, friendGroupID)
	if err != nil {
		t.Fatalf("AdminDeleteFriendGroup: %v", err)
	}
	if socialutil.StringValue(deleted.Id) != friendGroupID {
		t.Fatalf("deleted group = %#v", deleted)
	}
	if _, err := s.AdminGetFriendGroup(ctx, friendGroupID); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("AdminGetFriendGroup after delete error = %v, want not found", err)
	}
	assertNoBelongs(t, ctx, s, "peer-member", friendGroupID)
}

func TestAdminApplyFriendGroupAndGetMember(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	s.ACL = newTestACL(t)
	workspaces := &recordingWorkspaceService{}
	s.Workspaces = workspaces

	group, err := s.AdminApplyFriendGroup(ctx, "family01", " Family ", strPtr("first"))
	if err != nil {
		t.Fatalf("AdminApplyFriendGroup create: %v", err)
	}
	if got := socialutil.StringValue(group.Id); got != "family01" {
		t.Fatalf("created group id = %q, want family01", got)
	}
	if got := socialutil.StringValue(group.Name); got != "Family" {
		t.Fatalf("created group name = %q, want Family", got)
	}
	workspaceName := socialutil.StringValue(group.WorkspaceName)
	if workspaceName != socialutil.GroupWorkspaceName("family01") {
		t.Fatalf("created workspace_name = %q", workspaceName)
	}
	if len(workspaces.created) != 1 || workspaces.created[0].Name != workspaceName {
		t.Fatalf("created workspaces = %#v, want %q", workspaces.created, workspaceName)
	}

	updated, err := s.AdminApplyFriendGroup(ctx, "family01", "Family+", strPtr(""))
	if err != nil {
		t.Fatalf("AdminApplyFriendGroup update: %v", err)
	}
	if got := socialutil.StringValue(updated.Name); got != "Family+" {
		t.Fatalf("updated group name = %q, want Family+", got)
	}
	if updated.Description != nil {
		t.Fatalf("updated description = %q, want nil", socialutil.StringValue(updated.Description))
	}
	if len(workspaces.created) != 1 {
		t.Fatalf("updated created workspaces = %#v, want unchanged", workspaces.created)
	}

	if _, err := s.AdminApplyFriendGroup(ctx, "", "Family", nil); err == nil {
		t.Fatal("AdminApplyFriendGroup empty id error = nil")
	}
	if _, err := s.AdminApplyFriendGroup(ctx, " family01 ", "Family", nil); err == nil {
		t.Fatal("AdminApplyFriendGroup padded id error = nil")
	}
	if _, err := s.AdminApplyFriendGroup(ctx, "family01", "", nil); err == nil {
		t.Fatal("AdminApplyFriendGroup empty name error = nil")
	}

	member, err := s.AdminPutFriendGroupMember(ctx, "family01", "peer-member", rpcapi.FriendGroupMemberRoleMember)
	if err != nil {
		t.Fatalf("AdminPutFriendGroupMember: %v", err)
	}
	gotMember, err := s.AdminGetFriendGroupMember(ctx, " family01 ", " peer-member ")
	if err != nil {
		t.Fatalf("AdminGetFriendGroupMember: %v", err)
	}
	if socialutil.StringValue(gotMember.Id) != socialutil.StringValue(member.Id) || socialutil.GroupRole(gotMember) != rpcapi.FriendGroupMemberRoleMember {
		t.Fatalf("got member = %#v, want %#v", gotMember, member)
	}
	if _, err := s.AdminGetFriendGroupMember(ctx, "missing", "peer-member"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("AdminGetFriendGroupMember missing group error = %v, want kv.ErrNotFound", err)
	}
	if _, err := s.AdminGetFriendGroupMember(ctx, "family01", "missing"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("AdminGetFriendGroupMember missing member error = %v, want kv.ErrNotFound", err)
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

func TestMemberRollsBackWhenWorkspaceACLWriteFails(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	baseACL := newTestACL(t)
	s.ACL = baseACL

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := socialutil.StringValue(group.Id)

	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: friendGroupID, PeerPublicKey: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("member")}); err != nil {
		t.Fatalf("AddFriendGroupMember: %v", err)
	}
	assertBelongs(t, ctx, s, "peer-b", friendGroupID, rpcapi.FriendGroupMemberRoleMember)

	s.ACL = failingWorkspaceACL{ACL: baseACL}
	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: friendGroupID, PeerPublicKey: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("admin")}); err == nil {
		t.Fatal("AddFriendGroupMember with failing workspace ACL error = nil")
	}
	member, err := s.groupMember(ctx, friendGroupID, "peer-b")
	if err != nil {
		t.Fatalf("groupMember after failed workspace ACL: %v", err)
	}
	if socialutil.GroupRole(member) != rpcapi.FriendGroupMemberRoleMember {
		t.Fatalf("member role after failed workspace ACL = %s, want member", socialutil.GroupRole(member))
	}
	assertBelongs(t, ctx, s, "peer-b", friendGroupID, rpcapi.FriendGroupMemberRoleMember)

	s.ACL = failingACL{ACL: baseACL, failDelete: true}
	if _, err := s.DeleteFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupId: friendGroupID, Id: "peer-b"}); err == nil {
		t.Fatal("DeleteFriendGroupMember with failing ACL error = nil")
	}
	if _, err := s.groupMember(ctx, friendGroupID, "peer-b"); err != nil {
		t.Fatalf("groupMember after failed delete = %v, want preserved", err)
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

func TestJoinFriendGroupRollsBackWhenFinalReadFails(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	baseACL := newTestACL(t)
	s.ACL = baseACL
	s.Workspaces = &recordingWorkspaceService{}

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := socialutil.StringValue(group.Id)
	workspaceName := socialutil.StringValue(group.WorkspaceName)
	token, err := s.CreateFriendGroupInviteToken(ctx, "peer-a", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupId: friendGroupID})
	if err != nil {
		t.Fatalf("CreateFriendGroupInviteToken: %v", err)
	}

	groupStore := s.Groups
	s.Groups = &failAfterGetStore{Store: groupStore, failAfter: 1}
	if _, err := s.JoinFriendGroup(ctx, "peer-b", rpcapi.FriendGroupJoinRequest{InviteToken: token.InviteToken}); err == nil {
		t.Fatal("JoinFriendGroup with final group read failure error = nil")
	}
	s.Groups = groupStore
	if _, err := s.groupMember(ctx, friendGroupID, "peer-b"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("member after failed join error = %v, want not found", err)
	}
	assertNoBelongs(t, ctx, s, "peer-b", friendGroupID)
	if err := baseACL.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject("peer-b"),
		Resource:   acl.WorkspaceResource(workspaceName),
		Permission: apitypes.ACLPermissionUse,
	}); !errors.Is(err, acl.ErrDenied) {
		t.Fatalf("workspace ACL after failed join error = %v, want denied", err)
	}
}

func TestInviteTokensAndJoinLifecycle(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	baseACL := newTestACL(t)
	s.ACL = baseACL
	s.Workspaces = &recordingWorkspaceService{}

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := socialutil.StringValue(group.Id)
	workspaceName := socialutil.StringValue(group.WorkspaceName)

	empty, err := s.GetFriendGroupInviteToken(ctx, "peer-a", rpcapi.FriendGroupInviteTokenGetRequest{FriendGroupId: friendGroupID})
	if err != nil {
		t.Fatalf("GetFriendGroupInviteToken empty: %v", err)
	}
	if empty.InviteToken != nil || empty.ExpiresAt != nil {
		t.Fatalf("empty invite token = %#v, want nil fields", empty)
	}

	token, err := s.CreateFriendGroupInviteToken(ctx, "peer-a", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupId: friendGroupID})
	if err != nil {
		t.Fatalf("CreateFriendGroupInviteToken: %v", err)
	}
	if token.InviteToken == "" || token.ExpiresAt.IsZero() {
		t.Fatalf("created invite token = %#v", token)
	}
	got, err := s.GetFriendGroupInviteToken(ctx, "peer-a", rpcapi.FriendGroupInviteTokenGetRequest{FriendGroupId: friendGroupID})
	if err != nil {
		t.Fatalf("GetFriendGroupInviteToken: %v", err)
	}
	if got.InviteToken == nil || *got.InviteToken != token.InviteToken {
		t.Fatalf("GetFriendGroupInviteToken = %#v, want %q", got, token.InviteToken)
	}
	activeAgain, err := s.CreateFriendGroupInviteToken(ctx, "peer-a", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupId: friendGroupID})
	if err != nil {
		t.Fatalf("CreateFriendGroupInviteToken active: %v", err)
	}
	if activeAgain.InviteToken != token.InviteToken {
		t.Fatalf("active invite token = %q, want existing %q", activeAgain.InviteToken, token.InviteToken)
	}

	joined, err := s.JoinFriendGroup(ctx, "peer-b", rpcapi.FriendGroupJoinRequest{InviteToken: token.InviteToken})
	if err != nil {
		t.Fatalf("JoinFriendGroup: %v", err)
	}
	if socialutil.GroupRole(joined.Member) != rpcapi.FriendGroupMemberRoleMember {
		t.Fatalf("joined role = %q, want member", socialutil.GroupRole(joined.Member))
	}
	if joined.Group.MyRole == nil || *joined.Group.MyRole != rpcapi.FriendGroupMemberRoleMember {
		t.Fatalf("joined group my_role = %#v, want member", joined.Group.MyRole)
	}
	assertBelongs(t, ctx, s, "peer-b", friendGroupID, rpcapi.FriendGroupMemberRoleMember)
	if err := baseACL.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject("peer-b"),
		Resource:   acl.WorkspaceResource(workspaceName),
		Permission: apitypes.ACLPermissionUse,
	}); err != nil {
		t.Fatalf("joined peer workspace authorize: %v", err)
	}

	joinedAgain, err := s.JoinFriendGroup(ctx, "peer-b", rpcapi.FriendGroupJoinRequest{InviteToken: token.InviteToken})
	if err != nil {
		t.Fatalf("JoinFriendGroup existing: %v", err)
	}
	if socialutil.StringValue(joinedAgain.Member.PeerPublicKey) != "peer-b" {
		t.Fatalf("existing join member = %#v, want peer-b", joinedAgain.Member)
	}

	if _, err := s.ClearFriendGroupInviteToken(ctx, "peer-a", rpcapi.FriendGroupInviteTokenClearRequest{FriendGroupId: friendGroupID}); err != nil {
		t.Fatalf("ClearFriendGroupInviteToken: %v", err)
	}
	empty, err = s.GetFriendGroupInviteToken(ctx, "peer-a", rpcapi.FriendGroupInviteTokenGetRequest{FriendGroupId: friendGroupID})
	if err != nil {
		t.Fatalf("GetFriendGroupInviteToken after clear: %v", err)
	}
	if empty.InviteToken != nil || empty.ExpiresAt != nil {
		t.Fatalf("cleared invite token = %#v, want nil fields", empty)
	}

	expiring, err := s.CreateFriendGroupInviteToken(ctx, "peer-a", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupId: friendGroupID})
	if err != nil {
		t.Fatalf("CreateFriendGroupInviteToken expiring: %v", err)
	}
	s.Now = func() time.Time { return time.Date(2026, 6, 13, 0, 6, 0, 0, time.UTC) }
	expired, err := s.GetFriendGroupInviteToken(ctx, "peer-a", rpcapi.FriendGroupInviteTokenGetRequest{FriendGroupId: friendGroupID})
	if err != nil {
		t.Fatalf("GetFriendGroupInviteToken expired: %v", err)
	}
	if expired.InviteToken != nil || expired.ExpiresAt != nil {
		t.Fatalf("expired invite token = %#v, want nil fields", expired)
	}
	if _, err := s.JoinFriendGroup(ctx, "peer-c", rpcapi.FriendGroupJoinRequest{InviteToken: expiring.InviteToken}); err == nil {
		t.Fatal("JoinFriendGroup expired token error = nil")
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

func TestLifecycleDeletePathsAndPagination(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	baseACL := newTestACL(t)
	s.ACL = baseACL
	workspaces := &recordingWorkspaceService{}
	s.Workspaces = workspaces

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := socialutil.StringValue(group.Id)
	workspaceName := socialutil.StringValue(group.WorkspaceName)
	group, err = s.PutFriendGroup(ctx, "peer-a", rpcapi.FriendGroupPutRequest{Id: friendGroupID, Name: strPtr("renamed")})
	if err != nil {
		t.Fatalf("PutFriendGroup: %v", err)
	}
	if socialutil.StringValue(group.Name) != "renamed" {
		t.Fatalf("PutFriendGroup name = %q, want renamed", socialutil.StringValue(group.Name))
	}
	if group.MyRole == nil || *group.MyRole != rpcapi.FriendGroupMemberRoleOwner {
		t.Fatalf("PutFriendGroup my_role = %#v, want owner", group.MyRole)
	}
	if _, err := s.PutFriendGroup(ctx, "peer-a", rpcapi.FriendGroupPutRequest{Id: friendGroupID, Name: strPtr(" ")}); err == nil {
		t.Fatal("PutFriendGroup empty name error = nil")
	}
	if _, err := s.DeleteFriendGroup(ctx, "peer-a", rpcapi.FriendGroupDeleteRequest{}); err == nil {
		t.Fatal("DeleteFriendGroup empty id error = nil")
	}
	if _, err := s.DeleteFriendGroup(ctx, "peer-b", rpcapi.FriendGroupDeleteRequest{Id: friendGroupID}); err == nil {
		t.Fatal("DeleteFriendGroup by non-owner error = nil")
	}
	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: friendGroupID, PeerPublicKey: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("member")}); err != nil {
		t.Fatalf("AddFriendGroupMember: %v", err)
	}
	members, err := s.ListFriendGroupMembers(ctx, "peer-a", rpcapi.FriendGroupMemberListRequest{FriendGroupId: &friendGroupID, Limit: socialutil.IntPtr(1)})
	if err != nil {
		t.Fatalf("ListFriendGroupMembers: %v", err)
	}
	if len(members.Items) != 1 || !members.HasNext {
		t.Fatalf("ListFriendGroupMembers = %#v, want first page with next", members)
	}
	msg, err := s.SendFriendGroupMessage(ctx, "peer-b", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    friendGroupID,
		AudioBase64:      []byte("opus"),
		AudioContentType: "audio/opus",
	})
	if err != nil {
		t.Fatalf("SendFriendGroupMessage before delete: %v", err)
	}

	deleted, err := s.DeleteFriendGroup(ctx, "peer-a", rpcapi.FriendGroupDeleteRequest{Id: friendGroupID})
	if err != nil {
		t.Fatalf("DeleteFriendGroup: %v", err)
	}
	if socialutil.StringValue(deleted.Id) != friendGroupID {
		t.Fatalf("DeleteFriendGroup id = %q, want %q", socialutil.StringValue(deleted.Id), friendGroupID)
	}
	if len(workspaces.deleted) != 1 || workspaces.deleted[0] != workspaceName {
		t.Fatalf("deleted workspaces = %#v, want %q", workspaces.deleted, workspaceName)
	}
	if _, err := s.MessageAssets.Get(socialutil.StringValue(msg.AudioPath)); err == nil {
		t.Fatal("group audio object still exists after group delete")
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

func TestDeleteClearsBelongsAndWorkspaceACLBeyondFirstPage(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	baseACL := newTestACL(t)
	s.ACL = baseACL
	nextID := 0
	s.NewID = func() string {
		nextID++
		return fmt.Sprintf("id-%03d", nextID)
	}

	group, err := s.CreateFriendGroup(ctx, "peer-owner", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := socialutil.StringValue(group.Id)
	workspaceName := socialutil.StringValue(group.WorkspaceName)
	var lastPeer string
	for i := range socialutil.MaxListLimit + 1 {
		lastPeer = fmt.Sprintf("peer-%03d", i)
		if _, err := s.AddFriendGroupMember(ctx, "peer-owner", rpcapi.FriendGroupMemberAddRequest{
			FriendGroupId: friendGroupID,
			PeerPublicKey: lastPeer,
			Role:          rpcapi.FriendGroupMemberMutableRole("member"),
		}); err != nil {
			t.Fatalf("AddFriendGroupMember %d: %v", i, err)
		}
	}
	assertBelongs(t, ctx, s, lastPeer, friendGroupID, rpcapi.FriendGroupMemberRoleMember)
	if err := baseACL.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject(lastPeer),
		Resource:   acl.WorkspaceResource(workspaceName),
		Permission: apitypes.ACLPermissionUse,
	}); err != nil {
		t.Fatalf("last member workspace use authorize before delete: %v", err)
	}
	if _, err := s.DeleteFriendGroup(ctx, "peer-owner", rpcapi.FriendGroupDeleteRequest{Id: friendGroupID}); err != nil {
		t.Fatalf("DeleteFriendGroup: %v", err)
	}
	assertNoBelongs(t, ctx, s, lastPeer, friendGroupID)
	if err := baseACL.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject(lastPeer),
		Resource:   acl.WorkspaceResource(workspaceName),
		Permission: apitypes.ACLPermissionUse,
	}); !errors.Is(err, acl.ErrDenied) {
		t.Fatalf("last member workspace use authorize after delete error = %v, want denied", err)
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

func TestCreateHandlesWorkspaceFailures(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	s.Workspaces = failingWorkspaceService{createErr: errors.New("workspace store down")}
	if _, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"}); err == nil {
		t.Fatal("CreateFriendGroup with workspace error = nil")
	}

	s = newTestServer(t)
	s.Workspaces = failingWorkspaceService{
		createResp: adminhttp.CreateWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")),
	}
	if _, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"}); err == nil {
		t.Fatal("CreateFriendGroup with workspace failure response = nil")
	}

	s = newTestServer(t)
	baseACL := newTestACL(t)
	s.ACL = baseACL
	workspaces := &recordingWorkspaceService{}
	s.Workspaces = workspaces
	if created, err := s.ensureGroupWorkspace(ctx, "workspace-a", "peer-a"); err != nil || !created {
		t.Fatalf("ensureGroupWorkspace create = %v, %v; want created", created, err)
	}
	if created, err := s.ensureGroupWorkspace(ctx, "workspace-a", "peer-b"); err != nil || created {
		t.Fatalf("ensureGroupWorkspace existing = %v, %v; want existing", created, err)
	}
	if err := baseACL.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject("peer-b"),
		Resource:   acl.WorkspaceResource("workspace-a"),
		Permission: apitypes.ACLPermissionUse,
	}); err != nil {
		t.Fatalf("peer-b workspace use after existing workspace: %v", err)
	}
}

func TestWorkspaceHelperFallbacks(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	id := "legacy-group"
	if err := socialutil.WriteJSON(ctx, s.Groups, socialutil.GroupKey(id), rpcapi.FriendGroupObject{Id: &id}); err != nil {
		t.Fatalf("write legacy group: %v", err)
	}
	workspaceName, err := s.workspaceName(ctx, id)
	if err != nil {
		t.Fatalf("workspaceName legacy group: %v", err)
	}
	if want := socialutil.GroupWorkspaceName(id); workspaceName != want {
		t.Fatalf("workspaceName legacy group = %q, want %q", workspaceName, want)
	}
	if err := s.revokeWorkspace(ctx, workspaceName, "peer-a"); err != nil {
		t.Fatalf("revokeWorkspace without ACL: %v", err)
	}
	s.Workspaces = failingWorkspaceService{}
	if err := s.deleteWorkspace(ctx, workspaceName); err != nil {
		t.Fatalf("deleteWorkspace missing workspace: %v", err)
	}
}

func TestDeletePropagatesCleanupErrors(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	s.ACL = newTestACL(t)
	baseAssets := s.MessageAssets
	s.MessageAssets = failingDeletePrefixStore{ObjectStore: baseAssets}

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := socialutil.StringValue(group.Id)
	if _, err := s.SendFriendGroupMessage(ctx, "peer-a", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    friendGroupID,
		AudioBase64:      []byte("opus"),
		AudioContentType: "audio/opus",
	}); err != nil {
		t.Fatalf("SendFriendGroupMessage: %v", err)
	}
	if _, err := s.DeleteFriendGroup(ctx, "peer-a", rpcapi.FriendGroupDeleteRequest{Id: friendGroupID}); err == nil {
		t.Fatal("DeleteFriendGroup with failing asset cleanup error = nil")
	}
	if _, err := s.GetFriendGroup(ctx, "peer-a", rpcapi.FriendGroupGetRequest{Id: friendGroupID}); err != nil {
		t.Fatalf("GetFriendGroup after failed delete = %v, want group preserved", err)
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

func (s failingWorkspaceService) CreateWorkspace(context.Context, adminhttp.CreateWorkspaceRequestObject) (adminhttp.CreateWorkspaceResponseObject, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	return s.createResp, nil
}

func (s failingWorkspaceService) DeleteWorkspace(context.Context, adminhttp.DeleteWorkspaceRequestObject) (adminhttp.DeleteWorkspaceResponseObject, error) {
	return adminhttp.DeleteWorkspace404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", "missing")), nil
}

type failingACL struct {
	ACL
	failPut    bool
	failDelete bool
}

func (a failingACL) PutPolicyBinding(ctx context.Context, id string, priority float64, policy apitypes.ACLPolicy) (apitypes.ACLPolicyBinding, error) {
	if a.failPut {
		return apitypes.ACLPolicyBinding{}, errors.New("forced put policy binding failure")
	}
	return a.ACL.PutPolicyBinding(ctx, id, priority, policy)
}

func (a failingACL) DeletePolicyBinding(ctx context.Context, id string) (apitypes.ACLPolicyBinding, error) {
	if a.failDelete {
		return apitypes.ACLPolicyBinding{}, errors.New("forced delete policy binding failure")
	}
	return a.ACL.DeletePolicyBinding(ctx, id)
}

type failingWorkspaceACL struct {
	ACL
}

func (a failingWorkspaceACL) PutPolicyBinding(ctx context.Context, id string, priority float64, policy apitypes.ACLPolicy) (apitypes.ACLPolicyBinding, error) {
	if policy.Resource.Kind == apitypes.ACLResourceKindWorkspace {
		return apitypes.ACLPolicyBinding{}, errors.New("forced workspace policy binding failure")
	}
	return a.ACL.PutPolicyBinding(ctx, id, priority, policy)
}

func newTestACL(t *testing.T) *acl.Server {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	server := &acl.Server{DB: db}
	if err := server.Migration(context.Background()); err != nil {
		t.Fatalf("acl migration: %v", err)
	}
	return server
}

func strPtr(v string) *string {
	return &v
}
