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
	gotFriendGroup := mustGetFriendGroup(t, h, "peer-a", group.Name)
	if gotFriendGroup.Name != "family" {
		t.Fatalf("friend_group.get name = %q, want family", gotFriendGroup.Name)
	}
	if stringValue(gotFriendGroup.WorkspaceName) != stringValue(group.WorkspaceName) {
		t.Fatalf("friend_group.get workspace_name = %q, want %q", stringValue(gotFriendGroup.WorkspaceName), stringValue(group.WorkspaceName))
	}
	if err := getFriendGroupError(t, h, "peer-d", group.Name); err == nil {
		t.Fatal("non-member unexpectedly read group")
	}
	updatedFriendGroup := mustPutFriendGroup(t, h, "peer-a", group.Name, "family chat")
	if updatedFriendGroup.Name != group.Name || stringValue(updatedFriendGroup.DisplayName) != "family chat" {
		t.Fatalf("friend_group.put = %#v, want immutable name and updated display_name", updatedFriendGroup)
	}
	assertFriendGroupPagination(t, h, []string{group.Name, secondFriendGroup.Name})
	deletedFriendGroup := mustDeleteFriendGroup(t, h, "peer-a", secondFriendGroup.Name)
	if deletedFriendGroup.Name != secondFriendGroup.Name {
		t.Fatalf("friend_group.delete id = %q, want %q", deletedFriendGroup.Name, secondFriendGroup.Name)
	}
}
