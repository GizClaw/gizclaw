//go:build gizclaw_e2e

package multiserver_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

// TestFriendGroupMembersReportInRoom covers in_room on
// server.friend_group.members.list against real Servers and LiveKit: a member
// is in the Room only while its device runs the Group's Workspace. Shared
// presence converges after the configured historical KV read delay; stopping
// the runtime takes the member out of the Room while it stays online.
//
// The file name sorts this test after multi_server_test.go on purpose:
// deleting the Group queues asynchronous cleanup in shared TiKV, and
// TestSharedAssignmentRoutesAcrossBothEdges requires that store to be
// quiescent while it compares snapshots.
func TestFriendGroupMembersReportInRoom(t *testing.T) {
	serverA := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_A"))
	owner, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	member, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	ownerClient := connectAndServe(t, owner, serverA, serverA.PublicKey, "in-room-owner")
	defer ownerClient.Close()
	memberClient := connectAndServe(t, member, serverA, serverA.PublicKey, "in-room-member")
	defer memberClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	registerSocialPeer(t, ctx, ownerClient, serverA, "GIZCLAW_TEST_REGISTRATION_TOKEN_A")
	registerSocialPeer(t, ctx, memberClient, serverA, "GIZCLAW_TEST_REGISTRATION_TOKEN_A")

	group, err := ownerClient.CreateFriendGroup(ctx, "in-room-group-create", rpcapi.FriendGroupCreateRequest{Name: "in-room"})
	if err != nil {
		t.Fatalf("create FriendGroup: %v", err)
	}
	defer func() {
		_, _ = ownerClient.StopServerRun(context.Background(), "in-room-stop-owner")
		_, _ = memberClient.StopServerRun(context.Background(), "in-room-stop-member")
		_, _ = ownerClient.DeleteFriendGroup(context.Background(), "in-room-group-delete", rpcapi.FriendGroupDeleteRequest{Name: group.Name})
	}()
	if group.WorkspaceName == nil || *group.WorkspaceName == "" {
		t.Fatalf("FriendGroup has no Workspace: %+v", group)
	}
	workspace := *group.WorkspaceName
	invite, err := ownerClient.CreateFriendGroupInviteToken(ctx, "in-room-invite", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupName: group.Name})
	if err != nil {
		t.Fatalf("create FriendGroup invite: %v", err)
	}
	if _, err := memberClient.JoinFriendGroup(ctx, "in-room-join", rpcapi.FriendGroupJoinRequest{InviteToken: invite.InviteToken, Name: "in-room"}); err != nil {
		t.Fatalf("join FriendGroup: %v", err)
	}
	ownerKey, memberKey := owner.Public.String(), member.Public.String()

	// Both devices are connected but neither runs the Group's Workspace.
	assertInRoom(t, ctx, memberClient, "in-room-list-idle", map[string]bool{ownerKey: false, memberKey: false})

	if _, err := ownerClient.SetServerRunWorkspace(ctx, "in-room-select-owner", rpcapi.ServerSetRunWorkspaceRequest{WorkspaceName: workspace}); err != nil {
		t.Fatalf("owner select Workspace: %v", err)
	}
	waitRuntimeRunning(t, ctx, ownerClient, "in-room-status-owner")
	assertInRoom(t, ctx, memberClient, "in-room-list-owner", map[string]bool{ownerKey: true, memberKey: false})
	assertInRoom(t, ctx, ownerClient, "in-room-list-owner-self", map[string]bool{ownerKey: true, memberKey: false})

	if _, err := memberClient.SetServerRunWorkspace(ctx, "in-room-select-member", rpcapi.ServerSetRunWorkspaceRequest{WorkspaceName: workspace}); err != nil {
		t.Fatalf("member select Workspace: %v", err)
	}
	waitRuntimeRunning(t, ctx, memberClient, "in-room-status-member")
	assertInRoom(t, ctx, memberClient, "in-room-list-both", map[string]bool{ownerKey: true, memberKey: true})

	// Leaving the Room keeps the device online.
	if _, err := ownerClient.StopServerRun(ctx, "in-room-leave-owner"); err != nil {
		t.Fatalf("owner stop run: %v", err)
	}
	assertInRoom(t, ctx, memberClient, "in-room-list-left", map[string]bool{ownerKey: false, memberKey: true})
}

// assertInRoom lists the "in-room" Group through client and requires every
// member to be online with exactly the wanted in_room value.
func assertInRoom(t *testing.T, ctx context.Context, client *gizcli.Client, id string, want map[string]bool) {
	t.Helper()
	read := func() error {
		members, err := client.ListFriendGroupMembers(ctx, id, rpcapi.FriendGroupMemberListRequest{FriendGroupName: new("in-room")})
		if err != nil {
			return err
		}
		if len(members.Items) != len(want) {
			return fmt.Errorf("members = %+v, want %d", members.Items, len(want))
		}
		for _, item := range members.Items {
			if item.PeerPublicKey == nil || item.Online == nil || !*item.Online || item.InRoom == nil {
				return fmt.Errorf("member without presence = %+v", item)
			}
			inRoom, ok := want[*item.PeerPublicKey]
			if !ok || *item.InRoom != inRoom {
				return fmt.Errorf("member %s in_room = %v, want %v", *item.PeerPublicKey, *item.InRoom, inRoom)
			}
		}
		return nil
	}
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := read()
		if err == nil {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("%s: %v: %v", id, ctx.Err(), err)
		case <-deadline.C:
			t.Fatalf("%s: shared presence did not converge: %v", id, err)
		case <-ticker.C:
		}
	}
}
