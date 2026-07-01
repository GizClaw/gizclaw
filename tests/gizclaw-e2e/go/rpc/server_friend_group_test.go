//go:build gizclaw_e2e

package rpc_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestServerFriendGroupRPC(t *testing.T) {
	env := newSocialRPCHarness(t)

	description := "voice room"
	group, err := env.a.CreateFriendGroup(env.ctx, "friend_group.create", rpcapi.FriendGroupCreateRequest{Name: "family", Description: &description})
	if err != nil {
		t.Fatalf("friend_group.create: %v", err)
	}
	if group.Id == nil || *group.Id == "" || group.WorkspaceName == nil || *group.WorkspaceName == "" {
		t.Fatalf("friend_group.create = %#v", group)
	}
	secondGroup, err := env.a.CreateFriendGroup(env.ctx, "friend_group.create.backup", rpcapi.FriendGroupCreateRequest{Name: "backup"})
	if err != nil {
		t.Fatalf("friend_group.create backup: %v", err)
	}
	got, err := env.a.GetFriendGroup(env.ctx, "friend_group.get", rpcapi.FriendGroupGetRequest{Id: *group.Id})
	if err != nil {
		t.Fatalf("friend_group.get: %v", err)
	}
	if got.Name == nil || *got.Name != "family" {
		t.Fatalf("friend_group.get name = %#v", got.Name)
	}
	renamed := "family chat"
	updated, err := env.a.PutFriendGroup(env.ctx, "friend_group.put", rpcapi.FriendGroupPutRequest{Id: *group.Id, Name: &renamed})
	if err != nil {
		t.Fatalf("friend_group.put: %v", err)
	}
	if updated.Name == nil || *updated.Name != renamed {
		t.Fatalf("friend_group.put name = %#v", updated.Name)
	}
	if updated.MyRole == nil || *updated.MyRole != rpcapi.FriendGroupMemberRoleOwner {
		t.Fatalf("friend_group.put my_role = %#v, want owner", updated.MyRole)
	}

	emptyToken, err := env.a.GetFriendGroupInviteToken(env.ctx, "friend_group.invite_token.get.empty", rpcapi.FriendGroupInviteTokenGetRequest{FriendGroupId: *group.Id})
	if err != nil {
		t.Fatalf("friend_group.invite_token.get empty: %v", err)
	}
	if emptyToken.InviteToken != nil || emptyToken.ExpiresAt != nil {
		t.Fatalf("friend_group.invite_token.get empty = %#v, want no token", emptyToken)
	}
	token, err := env.a.CreateFriendGroupInviteToken(env.ctx, "friend_group.invite_token.create", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupId: *group.Id})
	if err != nil {
		t.Fatalf("friend_group.invite_token.create: %v", err)
	}
	if token.InviteToken == "" || token.ExpiresAt.IsZero() {
		t.Fatalf("friend_group.invite_token.create = %#v", token)
	}
	joined, err := env.b.JoinFriendGroup(env.ctx, "friend_group.join.b", rpcapi.FriendGroupJoinRequest{InviteToken: token.InviteToken})
	if err != nil {
		t.Fatalf("friend_group.join b: %v", err)
	}
	if joined.Member.PeerPublicKey == nil || *joined.Member.PeerPublicKey != env.peer["peer-b"] || joined.Member.Role == nil || *joined.Member.Role != rpcapi.FriendGroupMemberRoleMember {
		t.Fatalf("friend_group.join member = %#v, want peer-b member", joined.Member)
	}
	if joined.Group.MyRole == nil || *joined.Group.MyRole != rpcapi.FriendGroupMemberRoleMember {
		t.Fatalf("friend_group.join my_role = %#v, want member", joined.Group.MyRole)
	}
	memberB, err := env.a.PutFriendGroupMember(env.ctx, "friend_group.members.put.b", rpcapi.FriendGroupMemberPutRequest{
		FriendGroupId: *group.Id,
		Id:            env.peer["peer-b"],
		Role:          rpcapi.FriendGroupMemberMutableRoleAdmin,
	})
	if err != nil {
		t.Fatalf("friend_group.members.put b: %v", err)
	}
	if memberB.Role == nil || *memberB.Role != rpcapi.FriendGroupMemberRoleAdmin {
		t.Fatalf("member b role = %#v", memberB.Role)
	}
	memberC, err := env.b.AddFriendGroupMember(env.ctx, "friend_group.members.add.c", rpcapi.FriendGroupMemberAddRequest{
		FriendGroupId: *group.Id,
		PeerPublicKey: env.peer["peer-c"],
		Role:          rpcapi.FriendGroupMemberMutableRoleMember,
	})
	if err != nil {
		t.Fatalf("friend_group.members.add c: %v", err)
	}
	if memberC.PeerPublicKey == nil || *memberC.PeerPublicKey != env.peer["peer-c"] {
		t.Fatalf("member c peer_public_key = %#v", memberC.PeerPublicKey)
	}
	limit := 1
	groups, err := env.a.ListFriendGroups(env.ctx, "friend_group.list.page1", rpcapi.FriendGroupListRequest{Limit: &limit})
	if err != nil {
		t.Fatalf("friend_group.list page1: %v", err)
	}
	if len(groups.Items) != 1 || !groups.HasNext || groups.NextCursor == nil {
		t.Fatalf("friend_group.list page1 = %#v", groups)
	}
	groups, err = env.a.ListFriendGroups(env.ctx, "friend_group.list.page2", rpcapi.FriendGroupListRequest{Limit: &limit, Cursor: groups.NextCursor})
	if err != nil {
		t.Fatalf("friend_group.list page2: %v", err)
	}
	if len(groups.Items) != 1 || groups.HasNext {
		t.Fatalf("friend_group.list page2 = %#v", groups)
	}
	members, err := env.a.ListFriendGroupMembers(env.ctx, "friend_group.members.list", rpcapi.FriendGroupMemberListRequest{FriendGroupId: group.Id})
	if err != nil {
		t.Fatalf("friend_group.members.list: %v", err)
	}
	if len(members.Items) < 3 {
		t.Fatalf("friend_group.members.list = %#v, want owner plus two members", members.Items)
	}
	msg, err := env.b.SendFriendGroupMessage(env.ctx, "friend_group.messages.send", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    *group.Id,
		AudioContentType: "audio/opus",
		AudioBase64:      []byte("not-real-opus-but-rpc-payload"),
	})
	if err != nil {
		t.Fatalf("friend_group.messages.send: %v", err)
	}
	if msg.Id == nil || *msg.Id == "" {
		t.Fatalf("friend_group.messages.send id is empty: %#v", msg)
	}
	gotMsg, err := env.c.GetFriendGroupMessage(env.ctx, "friend_group.messages.get", rpcapi.FriendGroupMessageGetRequest{FriendGroupId: *group.Id, Id: *msg.Id})
	if err != nil {
		t.Fatalf("friend_group.messages.get: %v", err)
	}
	if gotMsg.Id == nil || *gotMsg.Id != *msg.Id {
		t.Fatalf("friend_group.messages.get id = %#v, want %q", gotMsg.Id, *msg.Id)
	}
	messages, err := env.c.ListFriendGroupMessages(env.ctx, "friend_group.messages.list", rpcapi.FriendGroupMessageListRequest{FriendGroupId: group.Id})
	if err != nil {
		t.Fatalf("friend_group.messages.list: %v", err)
	}
	if len(messages.Items) != 1 || messages.Items[0].Id == nil || *messages.Items[0].Id != *msg.Id {
		t.Fatalf("friend_group.messages.list = %#v, want %q", messages.Items, *msg.Id)
	}
	if _, err := env.d.GetFriendGroup(env.ctx, "friend_group.get.denied", rpcapi.FriendGroupGetRequest{Id: *group.Id}); err == nil {
		t.Fatal("non-member unexpectedly read group")
	}
	if _, err := env.b.DeleteFriendGroupMember(env.ctx, "friend_group.members.delete.c", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupId: *group.Id, Id: env.peer["peer-c"]}); err != nil {
		t.Fatalf("friend_group.members.delete c: %v", err)
	}
	deleted, err := env.a.DeleteFriendGroup(env.ctx, "friend_group.delete", rpcapi.FriendGroupDeleteRequest{Id: *secondGroup.Id})
	if err != nil {
		t.Fatalf("friend_group.delete: %v", err)
	}
	if deleted.Id == nil || *deleted.Id != *secondGroup.Id {
		t.Fatalf("friend_group.delete id = %#v, want %q", deleted.Id, *secondGroup.Id)
	}
}
