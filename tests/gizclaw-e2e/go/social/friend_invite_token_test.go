//go:build gizclaw_e2e

package social_test

import "testing"

func TestSocialFriendInviteTokenRPC(t *testing.T) {
	h := newSocialSimulatorHarness(t)
	peerB := h.ContextPublicKey("peer-b")
	peerC := h.ContextPublicKey("peer-c")

	assertFriendInviteTokenFailureCases(t, h)
	friendAB := createFriendByInviteToken(t, h, "peer-a", "peer-b", peerB)
	friendAC := createFriendByInviteToken(t, h, "peer-a", "peer-c", peerC)
	if stringValue(friendAB.WorkspaceName) == "" || stringValue(friendAC.WorkspaceName) == "" {
		t.Fatalf("friend workspaces are empty: ab=%#v ac=%#v", friendAB, friendAC)
	}
	assertFriendPagination(t, h, friendAB, friendAC)

	deletedFriend := mustDeleteFriend(t, h, "peer-a", stringValue(friendAC.Id))
	if stringValue(deletedFriend.Id) != stringValue(friendAC.Id) {
		t.Fatalf("friend.delete id = %q, want %q", stringValue(deletedFriend.Id), stringValue(friendAC.Id))
	}
	assertWorkspaceHistoryDenied(t, h, "peer-c", stringValue(friendAC.WorkspaceName))
}
