//go:build gizclaw_e2e

package social_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestSocialFriendGroupMemberRPC(t *testing.T) {
	h := newSocialSimulatorHarness(t)
	peerB := h.ContextPublicKey("peer-b")
	peerC := h.ContextPublicKey("peer-c")
	peerCClient := h.Client("peer-c")

	group := mustCreateFriendGroup(t, h, "peer-a", "family", "voice room")
	if group.MyRole == nil || *group.MyRole != rpcapi.FriendGroupMemberRoleOwner {
		t.Fatalf("friend_group.create my_role = %v, want owner", group.MyRole)
	}
	empty := mustGetFriendGroupInviteToken(t, h, "peer-a", group.Name)
	if empty.InviteToken != nil || empty.ExpiresAt != nil {
		t.Fatalf("friend group invite token empty get = %#v, want no token", empty)
	}
	token := mustCreateFriendGroupInviteToken(t, h, "peer-a", group.Name)
	if token.InviteToken == "" || token.ExpiresAt.IsZero() {
		t.Fatalf("friend group invite token create = %#v", token)
	}
	if err := friendGroupInviteTokenError(t, h, "peer-b", group.Name); err == nil {
		t.Fatal("non-owner unexpectedly created group invite token")
	}
	join := mustJoinFriendGroup(t, h, "peer-b", group.Name, token.InviteToken)
	if join.Member.PeerPublicKey == nil || *join.Member.PeerPublicKey != peerB || join.Member.Role == nil || *join.Member.Role != rpcapi.FriendGroupMemberRoleMember {
		t.Fatalf("friend_group.join member = %#v, want peer-b member", join.Member)
	}
	if join.Group.MyRole == nil || *join.Group.MyRole != rpcapi.FriendGroupMemberRoleMember {
		t.Fatalf("friend_group.join group my_role = %v, want member", join.Group.MyRole)
	}
	memberB := mustPutFriendGroupMember(t, h, "peer-a", group.Name, peerB, rpcapi.FriendGroupMemberMutableRoleAdmin)
	if memberB.Role == nil || *memberB.Role != rpcapi.FriendGroupMemberRoleAdmin {
		t.Fatalf("member b role = %v, want admin", memberB.Role)
	}
	mustClearFriendGroupInviteToken(t, h, "peer-a", group.Name)
	if err := joinFriendGroupError(t, peerCClient, group.Name, token.InviteToken); err == nil {
		t.Fatal("join with cleared group invite token unexpectedly succeeded")
	}
	assertFriendGroupClientConnection(t, peerCClient, "friend-group-cleared-token")
	memberC := mustAddFriendGroupMember(t, h, "peer-b", group.Name, peerC, rpcapi.FriendGroupMemberMutableRoleMember)
	if stringValue(memberC.PeerPublicKey) != peerC {
		t.Fatalf("member c peer_public_key = %q, want %q", stringValue(memberC.PeerPublicKey), peerC)
	}
	assertFriendGroupMemberPagination(t, h, group.Name)

	deletedMember := mustDeleteFriendGroupMember(t, h, "peer-b", group.Name, peerC)
	if stringValue(deletedMember.PeerPublicKey) != peerC {
		t.Fatalf("friend_group.members.delete peer_public_key = %q, want %q", stringValue(deletedMember.PeerPublicKey), peerC)
	}
	assertWorkspaceHistoryDenied(t, h, "peer-c", stringValue(group.WorkspaceName))
}
