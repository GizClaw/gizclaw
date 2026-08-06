//go:build gizclaw_e2e

package rpc_test

import (
	"bytes"
	"errors"
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
	if group.Name == "" || group.WorkspaceName == nil || *group.WorkspaceName == "" {
		t.Fatalf("friend_group.create = %#v", group)
	}
	secondGroup, err := env.a.CreateFriendGroup(env.ctx, "friend_group.create.backup", rpcapi.FriendGroupCreateRequest{Name: "backup"})
	if err != nil {
		t.Fatalf("friend_group.create backup: %v", err)
	}
	got, err := env.a.GetFriendGroup(env.ctx, "friend_group.get", rpcapi.FriendGroupGetRequest{Name: group.Name})
	if err != nil {
		t.Fatalf("friend_group.get: %v", err)
	}
	if got.Name != "family" {
		t.Fatalf("friend_group.get name = %#v", got.Name)
	}
	renamed := "family chat"
	updated, err := env.a.PutFriendGroup(env.ctx, "friend_group.put", rpcapi.FriendGroupPutRequest{Name: group.Name, DisplayName: &renamed})
	if err != nil {
		t.Fatalf("friend_group.put: %v", err)
	}
	if updated.Name != group.Name || updated.DisplayName == nil || *updated.DisplayName != renamed {
		t.Fatalf("friend_group.put = %#v", updated)
	}
	if updated.MyRole == nil || *updated.MyRole != rpcapi.FriendGroupMemberRoleOwner {
		t.Fatalf("friend_group.put my_role = %#v, want admin", updated.MyRole)
	}

	emptyToken, err := env.a.GetFriendGroupInviteToken(env.ctx, "friend_group.invite_token.get.empty", rpcapi.FriendGroupInviteTokenGetRequest{FriendGroupName: group.Name})
	if err != nil {
		t.Fatalf("friend_group.invite_token.get empty: %v", err)
	}
	if emptyToken.InviteToken != nil || emptyToken.ExpiresAt != nil {
		t.Fatalf("friend_group.invite_token.get empty = %#v, want no token", emptyToken)
	}
	token, err := env.a.CreateFriendGroupInviteToken(env.ctx, "friend_group.invite_token.create", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupName: group.Name})
	if err != nil {
		t.Fatalf("friend_group.invite_token.create: %v", err)
	}
	if token.InviteToken == "" || token.ExpiresAt.IsZero() {
		t.Fatalf("friend_group.invite_token.create = %#v", token)
	}
	joined, err := env.b.JoinFriendGroup(env.ctx, "friend_group.join.b", rpcapi.FriendGroupJoinRequest{Name: group.Name, InviteToken: token.InviteToken})
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
		FriendGroupName: group.Name,
		Name:            env.peer["peer-b"],
		Role:            rpcapi.FriendGroupMemberMutableRoleAdmin,
	})
	if err != nil {
		t.Fatalf("friend_group.members.put b: %v", err)
	}
	if memberB.Role == nil || *memberB.Role != rpcapi.FriendGroupMemberRoleAdmin {
		t.Fatalf("member b role = %#v", memberB.Role)
	}
	memberC, err := env.b.AddFriendGroupMember(env.ctx, "friend_group.members.add.c", rpcapi.FriendGroupMemberAddRequest{
		FriendGroupName: group.Name,
		PeerPublicKey:   env.peer["peer-c"],
		Role:            rpcapi.FriendGroupMemberMutableRoleMember,
		MemberName:      "peer-c",
	})
	if err != nil {
		t.Fatalf("friend_group.members.add c: %v", err)
	}
	if memberC.PeerPublicKey == nil || *memberC.PeerPublicKey != env.peer["peer-c"] {
		t.Fatalf("member c peer_public_key = %#v", memberC.PeerPublicKey)
	}
	if memberC.FriendGroupName == nil || *memberC.FriendGroupName != "peer-c" {
		t.Fatalf("member c friend_group_name = %#v, want peer-specific name", memberC.FriendGroupName)
	}
	peerCGroupName := *memberC.FriendGroupName
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
	members, err := env.a.ListFriendGroupMembers(env.ctx, "friend_group.members.list", rpcapi.FriendGroupMemberListRequest{FriendGroupName: &group.Name})
	if err != nil {
		t.Fatalf("friend_group.members.list: %v", err)
	}
	if len(members.Items) < 3 {
		t.Fatalf("friend_group.members.list = %#v, want admin plus two members", members.Items)
	}
	messages, err := env.c.ListFriendGroupMessages(env.ctx, "friend_group.messages.list", rpcapi.FriendGroupMessageListRequest{FriendGroupName: peerCGroupName})
	if err != nil {
		t.Fatalf("friend_group.messages.list: %v", err)
	}
	for _, message := range messages.Items {
		if message.FriendGroupName != peerCGroupName || message.Name == "" {
			t.Fatalf("friend_group.messages.list item = %#v", message)
		}
	}
	missingHistoryID := "issue-686-missing-history"
	var audio bytes.Buffer
	audioResult, err := env.c.GetFriendGroupMessageAudio(env.ctx, "friend_group.messages.audio.get.missing", rpcapi.FriendGroupMessageAudioGetRequest{
		FriendGroupName: peerCGroupName,
		HistoryName:     missingHistoryID,
	}, &audio)
	if err == nil {
		t.Fatal("friend_group.messages.audio.get missing unexpectedly succeeded")
	}
	var rpcErr rpcapi.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != rpcapi.RPCErrorCodeNotFound || rpcErr.Message != "not found" || err.Error() != "rpc: not found" {
		t.Fatalf("friend_group.messages.audio.get missing error = %v (%+v), want generic typed not found", err, rpcErr)
	}
	if audioResult.Metadata != (rpcapi.FriendGroupMessageAudioGetResponse{}) || audioResult.Bytes != 0 || audio.Len() != 0 {
		t.Fatalf("friend_group.messages.audio.get missing result = %#v payload=%q, want zero result", audioResult, audio.String())
	}
	gotAfterAudioFailure, err := env.c.GetFriendGroup(env.ctx, "friend_group.get.after_audio_failure", rpcapi.FriendGroupGetRequest{Name: peerCGroupName})
	if err != nil {
		t.Fatalf("friend_group.get after audio failure: %v", err)
	}
	if gotAfterAudioFailure.Name != peerCGroupName {
		t.Fatalf("friend_group.get after audio failure = %#v", gotAfterAudioFailure)
	}
	if _, err := env.d.GetFriendGroup(env.ctx, "friend_group.get.denied", rpcapi.FriendGroupGetRequest{Name: group.Name}); err == nil {
		t.Fatal("non-member unexpectedly read group")
	}
	if _, err := env.b.DeleteFriendGroupMember(env.ctx, "friend_group.members.delete.c", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupName: group.Name, Name: env.peer["peer-c"]}); err != nil {
		t.Fatalf("friend_group.members.delete c: %v", err)
	}
	deleted, err := env.a.DeleteFriendGroup(env.ctx, "friend_group.delete", rpcapi.FriendGroupDeleteRequest{Name: secondGroup.Name})
	if err != nil {
		t.Fatalf("friend_group.delete: %v", err)
	}
	if deleted.Name != secondGroup.Name {
		t.Fatalf("friend_group.delete name = %#v, want %q", deleted.Name, secondGroup.Name)
	}
}
