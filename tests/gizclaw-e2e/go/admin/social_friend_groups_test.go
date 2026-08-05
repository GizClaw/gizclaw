//go:build gizclaw_e2e

package admin_test

import (
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
)

func TestAdminAPIFriendGroupsMembersAndInviteToken(t *testing.T) {
	env := newAdminAPIHarness(t)
	registerAdminHistoryPeers(t, env, env.admin)

	groupID := mutationName("friend-group-id")
	created, err := env.api.CreateFriendGroupWithResponse(env.ctx, adminhttp.AdminFriendGroupCreateRequest{
		Id:             groupID,
		Name:           mutationName("friend-group"),
		Description:    ptr("Admin API friend group"),
		OwnerPublicKey: env.adminKey,
	})
	if err != nil {
		t.Fatalf("create friend group: %v", err)
	}
	requireStatusOK(t, created, created.Body)
	if created.JSON200 == nil || created.JSON200.Id == "" ||
		created.JSON200.CreatedByPeerPublicKey != env.adminKey {
		t.Fatalf("created friend group = %#v", created.JSON200)
	}
	groupID = created.JSON200.Id
	t.Cleanup(func() { _, _ = env.api.DeleteFriendGroupWithResponse(env.ctx, groupID) })

	get, err := env.api.GetFriendGroupWithResponse(env.ctx, groupID)
	if err != nil {
		t.Fatalf("get friend group: %v", err)
	}
	requireStatusOK(t, get, get.Body)
	if get.JSON200 == nil || get.JSON200.WorkspaceId == nil || *get.JSON200.WorkspaceId == "" {
		t.Fatalf("get friend group = %#v", get.JSON200)
	}

	updated, err := env.api.PutFriendGroupWithResponse(env.ctx, groupID, adminhttp.AdminFriendGroupPutRequest{
		Id:          groupID,
		DisplayName: ptr("Renamed Group"),
		Description: ptr("renamed"),
	})
	if err != nil {
		t.Fatalf("put friend group: %v", err)
	}
	requireStatusOK(t, updated, updated.Body)
	if updated.JSON200 == nil || updated.JSON200.Name != created.JSON200.Name || updated.JSON200.DisplayName == nil || *updated.JSON200.DisplayName != "Renamed Group" {
		t.Fatalf("updated friend group = %#v", updated.JSON200)
	}

	member, err := env.api.CreateFriendGroupMemberWithResponse(env.ctx, groupID, adminhttp.AdminFriendGroupMemberCreateRequest{
		Id:            customid.MembershipName(groupID, env.peerKey),
		Name:          created.JSON200.Name,
		PeerPublicKey: env.peerKey,
		Role:          rpcapi.FriendGroupMemberRoleMember,
	})
	if err != nil {
		t.Fatalf("create member: %v", err)
	}
	requireStatusOK(t, member, member.Body)
	if member.JSON200 == nil || member.JSON200.Role != rpcapi.FriendGroupMemberRoleMember || member.JSON200.FriendGroupId != groupID {
		t.Fatalf("member = %#v", member.JSON200)
	}

	updatedMember, err := env.api.PutFriendGroupMemberWithResponse(env.ctx, groupID, env.peerKey, adminhttp.AdminFriendGroupMemberPutRequest{
		Id:   customid.MembershipName(groupID, env.peerKey),
		Role: rpcapi.FriendGroupMemberRoleAdmin,
	})
	if err != nil {
		t.Fatalf("put member: %v", err)
	}
	requireStatusOK(t, updatedMember, updatedMember.Body)
	if updatedMember.JSON200 == nil || updatedMember.JSON200.Role != rpcapi.FriendGroupMemberRoleAdmin || updatedMember.JSON200.FriendGroupId != groupID {
		t.Fatalf("updated member = %#v", updatedMember.JSON200)
	}

	members := collectAdminPagesInt(t, 1, func(cursor *string, limit int) ([]adminhttp.AdminFriendGroupMemberObject, bool, *string) {
		resp, err := env.api.ListFriendGroupMembersWithResponse(env.ctx, groupID, &adminhttp.ListFriendGroupMembersParams{Cursor: cursor, Limit: &limit})
		if err != nil {
			t.Fatalf("list friend group members: %v", err)
		}
		requireStatusOK(t, resp, resp.Body)
		if resp.JSON200 == nil {
			t.Fatalf("list friend group members missing JSON200")
		}
		return resp.JSON200.Items, resp.JSON200.HasNext, resp.JSON200.NextCursor
	})
	requireName(t, members, env.peerKey, func(item adminhttp.AdminFriendGroupMemberObject) string {
		return item.PeerPublicKey
	})
	requireName(t, members, env.adminKey, func(item adminhttp.AdminFriendGroupMemberObject) string {
		return item.PeerPublicKey
	})

	expiresAt := time.Now().UTC().Add(10 * time.Minute)
	token, err := env.api.PutFriendGroupInviteTokenWithResponse(env.ctx, groupID, adminhttp.AdminFriendGroupInviteTokenPutRequest{
		Id:          groupID,
		InviteToken: mutationName("group-token"),
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		t.Fatalf("put friend group invite token: %v", err)
	}
	requireStatusOK(t, token, token.Body)
	if token.JSON200 == nil || token.JSON200.InviteToken == nil || *token.JSON200.InviteToken != mutationName("group-token") {
		t.Fatalf("put invite token = %#v", token.JSON200)
	}
	gotToken, err := env.api.GetFriendGroupInviteTokenWithResponse(env.ctx, groupID)
	if err != nil {
		t.Fatalf("get friend group invite token: %v", err)
	}
	requireStatusOK(t, gotToken, gotToken.Body)
	if gotToken.JSON200 == nil || gotToken.JSON200.InviteToken == nil || *gotToken.JSON200.InviteToken != mutationName("group-token") {
		t.Fatalf("get invite token = %#v", gotToken.JSON200)
	}
	deletedToken, err := env.api.DeleteFriendGroupInviteTokenWithResponse(env.ctx, groupID)
	if err != nil {
		t.Fatalf("delete friend group invite token: %v", err)
	}
	requireStatusOK(t, deletedToken, deletedToken.Body)

	deletedOwner, err := env.api.DeleteFriendGroupMemberWithResponse(env.ctx, groupID, env.adminKey)
	if err != nil {
		t.Fatalf("delete owner member: %v", err)
	}
	requireStatusOK(t, deletedOwner, deletedOwner.Body)
	deletedPeer, err := env.api.DeleteFriendGroupMemberWithResponse(env.ctx, groupID, env.peerKey)
	if err != nil {
		t.Fatalf("delete peer member: %v", err)
	}
	requireStatusOK(t, deletedPeer, deletedPeer.Body)

	deletedGroup, err := env.api.DeleteFriendGroupWithResponse(env.ctx, groupID)
	if err != nil {
		t.Fatalf("delete friend group: %v", err)
	}
	requireStatusOK(t, deletedGroup, deletedGroup.Body)
	missing, err := env.api.GetFriendGroupWithResponse(env.ctx, groupID)
	if err != nil {
		t.Fatalf("get deleted friend group: %v", err)
	}
	if missing.StatusCode() != 404 {
		t.Fatalf("get deleted friend group status = %d body=%s", missing.StatusCode(), string(missing.Body))
	}
}
