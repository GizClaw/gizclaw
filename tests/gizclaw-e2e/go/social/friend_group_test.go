//go:build gizclaw_e2e

package social_test

import "testing"

func TestSocialFriendGroupRPC(t *testing.T) {
	h := newSocialSimulatorHarness(t)

	group := mustCreateFriendGroup(t, h, "peer-a", "family", "voice room")
	if stringValue(group.WorkspaceName) == "" {
		t.Fatalf("friend_group.create workspace_name is empty: %#v", group)
	}
	secondFriendGroup := mustCreateFriendGroup(t, h, "peer-a", "backup", "")
	gotFriendGroup := mustGetFriendGroup(t, h, "peer-a", stringValue(group.Id))
	if stringValue(gotFriendGroup.Name) != "family" {
		t.Fatalf("friend_group.get name = %q, want family", stringValue(gotFriendGroup.Name))
	}
	if stringValue(gotFriendGroup.WorkspaceName) != stringValue(group.WorkspaceName) {
		t.Fatalf("friend_group.get workspace_name = %q, want %q", stringValue(gotFriendGroup.WorkspaceName), stringValue(group.WorkspaceName))
	}
	if err := getFriendGroupError(t, h, "peer-d", stringValue(group.Id)); err == nil {
		t.Fatal("non-member unexpectedly read group")
	}
	renamedFriendGroup := mustPutFriendGroup(t, h, "peer-a", stringValue(group.Id), "family chat")
	if stringValue(renamedFriendGroup.Name) != "family chat" {
		t.Fatalf("friend_group.put name = %q, want family chat", stringValue(renamedFriendGroup.Name))
	}
	assertFriendGroupPagination(t, h, []string{stringValue(group.Id), stringValue(secondFriendGroup.Id)})
	deletedFriendGroup := mustDeleteFriendGroup(t, h, "peer-a", stringValue(secondFriendGroup.Id))
	if stringValue(deletedFriendGroup.Id) != stringValue(secondFriendGroup.Id) {
		t.Fatalf("friend_group.delete id = %q, want %q", stringValue(deletedFriendGroup.Id), stringValue(secondFriendGroup.Id))
	}
}
