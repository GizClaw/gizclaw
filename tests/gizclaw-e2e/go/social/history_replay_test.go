//go:build gizclaw_e2e

package social_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestSocialHistoryReplayRPC(t *testing.T) {
	h := newSocialSimulatorHarness(t)
	peerB := h.ContextPublicKey("peer-b")
	peerC := h.ContextPublicKey("peer-c")

	requestAB := createFriendByInviteToken(t, h, "peer-a", "peer-b", peerB)
	t.Run("friend direct chat", func(t *testing.T) {
		assertChatWorkspaceHistory(t, h, "peer-a", "peer-b", stringValue(requestAB.WorkspaceName), []string{
			"hello direct chat round one",
			"hello direct chat round two",
			"hello direct chat round three",
		})
	})

	group := mustCreateFriendGroup(t, h, "peer-a", "family", "voice room")
	mustAddFriendGroupMember(t, h, "peer-a", stringValue(group.Id), peerB, rpcapi.FriendGroupMemberMutableRoleMember)
	mustPutFriendGroupMember(t, h, "peer-a", stringValue(group.Id), peerB, rpcapi.FriendGroupMemberMutableRoleAdmin)
	mustAddFriendGroupMember(t, h, "peer-b", stringValue(group.Id), peerC, rpcapi.FriendGroupMemberMutableRoleMember)
	t.Run("group chat", func(t *testing.T) {
		assertChatWorkspaceHistory(t, h, "peer-b", "peer-c", stringValue(group.WorkspaceName), []string{
			"hello group chat round one",
			"hello group chat round two",
			"hello group chat round three",
		})
	})
	assertWorkspaceHistoryDenied(t, h, "peer-d", stringValue(group.WorkspaceName))
}
