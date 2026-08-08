//go:build gizclaw_e2e

package social_test

import "testing"

func TestSocialFriendInviteTokenRPC(t *testing.T) {
	h := newSocialSimulatorHarness(t)
	peerB := h.ContextPublicKey("peer-b")
	peerC := h.ContextPublicKey("peer-c")

	friendAB := assertFriendInviteTokenFailureCases(t, h, peerB)
	friendAC := createFriendByInviteToken(t, h, "peer-a", "peer-c", peerC)
	if stringValue(friendAB.WorkspaceName) == "" || stringValue(friendAC.WorkspaceName) == "" {
		t.Fatalf("friend workspaces are empty: ab=%#v ac=%#v", friendAB, friendAC)
	}
	assertFriendPagination(t, h, friendAB, friendAC)

	deletedFriend := mustDeleteFriend(t, h, "peer-a", friendAC.Name)
	if deletedFriend.Name != friendAC.Name {
		t.Fatalf("friend.delete name = %q, want %q", deletedFriend.Name, friendAC.Name)
	}
	assertWorkspaceHistoryDenied(t, h, "peer-c", stringValue(friendAC.WorkspaceName))
}
