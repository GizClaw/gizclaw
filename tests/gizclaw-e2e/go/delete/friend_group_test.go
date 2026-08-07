//go:build gizclaw_e2e

package delete_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestFriendGroupDeletionQuiescesEveryMemberRuntime(t *testing.T) {
	env := newDeletionHarness(t)
	owner := env.newPeer(t, "delete-group-owner")
	member := env.newPeer(t, "delete-group-member")
	foreign := env.newPeer(t, "delete-group-foreign")

	groupName := "delete-friend-group-active"
	group, err := owner.client.CreateFriendGroup(env.ctx, "delete.group.create", rpcapi.FriendGroupCreateRequest{Name: groupName})
	if err != nil {
		t.Fatalf("create Friend Group: %v", err)
	}
	if group.WorkspaceName == nil || *group.WorkspaceName == "" {
		t.Fatalf("Friend Group has no chat Workspace: %#v", group)
	}
	invite, err := owner.client.CreateFriendGroupInviteToken(env.ctx, "delete.group.invite", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupName: group.Name})
	if err != nil {
		t.Fatalf("create Friend Group invite: %v", err)
	}
	joined, err := member.client.JoinFriendGroup(env.ctx, "delete.group.join", rpcapi.FriendGroupJoinRequest{Name: group.Name, InviteToken: invite.InviteToken})
	if err != nil {
		t.Fatalf("join Friend Group: %v", err)
	}
	if joined.Group.WorkspaceName == nil || *joined.Group.WorkspaceName != *group.WorkspaceName {
		t.Fatalf("joined Friend Group Workspace = %#v, want %q", joined.Group.WorkspaceName, *group.WorkspaceName)
	}
	foreignGroup, err := foreign.client.CreateFriendGroup(env.ctx, "delete.group.foreign.create", rpcapi.FriendGroupCreateRequest{Name: "delete-friend-group-foreign-kept"})
	if err != nil {
		t.Fatalf("create foreign Friend Group: %v", err)
	}

	storedGroup := env.findFriendGroup(t, owner.publicKey, group.Name)
	if storedGroup.WorkspaceId == nil {
		t.Fatalf("stored Friend Group has no Workspace: %#v", storedGroup)
	}
	storedForeign := env.findFriendGroup(t, foreign.publicKey, foreignGroup.Name)
	env.startWorkspace(t, owner, *group.WorkspaceName)
	env.startWorkspace(t, member, *group.WorkspaceName)
	requireRunningWorkspace(t, env, owner, *group.WorkspaceName)
	requireRunningWorkspace(t, env, member, *group.WorkspaceName)
	ownerActive := env.startActiveTransform(t, owner)
	memberActive := env.startActiveTransform(t, member)

	deleted, err := owner.client.DeleteFriendGroup(env.ctx, "delete.group.submit", rpcapi.FriendGroupDeleteRequest{Name: group.Name})
	if err != nil {
		t.Fatalf("delete active Friend Group: %v", err)
	}
	if deleted.Name != group.Name {
		t.Fatalf("Friend Group delete response = %#v", deleted)
	}
	if _, err := owner.client.GetFriendGroup(env.ctx, "delete.group.fenced", rpcapi.FriendGroupGetRequest{Name: group.Name}); err == nil {
		t.Fatal("Friend Group accepted business access after delete response")
	}
	if _, err := member.client.CreateFriendGroupInviteToken(env.ctx, "delete.group.member.fenced", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupName: group.Name}); err == nil {
		t.Fatal("Friend Group member mutated the group after delete response")
	}
	repeated, err := owner.client.DeleteFriendGroup(env.ctx, "delete.group.repeat", rpcapi.FriendGroupDeleteRequest{Name: group.Name})
	if err != nil {
		t.Fatalf("repeat Friend Group delete was not idempotent: %v", err)
	}
	if repeated.Name != deleted.Name {
		t.Fatalf("repeat Friend Group delete = %#v, want the original target %#v", repeated, deleted)
	}

	waitUntil(t, env.ctx, "Friend Group deletion", func() (bool, string) {
		response, err := env.api.GetFriendGroupWithResponse(env.ctx, storedGroup.Id)
		if err != nil {
			return false, err.Error()
		}
		return response.StatusCode() == http.StatusNotFound, fmt.Sprintf("status=%d body=%s", response.StatusCode(), response.Body)
	})
	env.waitWorkspaceAbsent(t, *storedGroup.WorkspaceId)
	ownerActive.requireTerminated(t, env.ctx, "Friend Group owner")
	memberActive.requireTerminated(t, env.ctx, "Friend Group member")
	env.waitRunStopped(t, owner, "delete.group.owner.status.after")
	env.waitRunStopped(t, member, "delete.group.member.status.after")
	if _, err := owner.client.GetServerInfo(env.ctx, "delete.group.owner-still-active"); err != nil {
		t.Fatalf("Friend Group deletion stopped the owner Peer connection: %v", err)
	}
	if _, err := member.client.GetServerInfo(env.ctx, "delete.group.member-still-active"); err != nil {
		t.Fatalf("Friend Group deletion stopped the member Peer connection: %v", err)
	}
	if response, err := env.api.GetFriendGroupWithResponse(env.ctx, storedForeign.Id); err != nil || response.StatusCode() != http.StatusOK {
		t.Fatalf("foreign Friend Group was affected: status=%d body=%s error=%v", response.StatusCode(), response.Body, err)
	}
	assertNoPendingDeletion(t, env, "friend_group", "friend_group", storedGroup.Id)
	assertNoPendingDeletion(t, env, "workspace", "workspace", *storedGroup.WorkspaceId)
}

func requireRunningWorkspace(t *testing.T, env *deletionHarness, peer deletionPeer, workspaceName string) {
	t.Helper()
	status, err := env.runStatus(peer, "delete.workspace.status.running."+peer.contextName)
	if err != nil || status.State != rpcapi.PeerRunStatusStateRunning || status.WorkspaceName == nil || *status.WorkspaceName != workspaceName {
		t.Fatalf("%s Workspace was not running: status=%#v error=%v", peer.contextName, status, err)
	}
}
