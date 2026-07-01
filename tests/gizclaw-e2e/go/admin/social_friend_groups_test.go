//go:build gizclaw_e2e

package admin_test

import (
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestAdminAPIFriendGroupsMembersAndInviteToken(t *testing.T) {
	env := newAdminAPIHarness(t)

	created, err := env.api.CreateFriendGroupWithResponse(env.ctx, adminservice.AdminFriendGroupCreateRequest{
		Name:        mutationName("friend-group"),
		Description: ptr("Admin API friend group"),
	})
	if err != nil {
		t.Fatalf("create friend group: %v", err)
	}
	requireStatusOK(t, created, created.Body)
	if created.JSON200 == nil || created.JSON200.Id == nil || *created.JSON200.Id == "" || created.JSON200.CreatedByPeerPublicKey != nil || created.JSON200.MyRole != nil {
		t.Fatalf("created friend group = %#v", created.JSON200)
	}
	groupID := *created.JSON200.Id
	t.Cleanup(func() { _, _ = env.api.DeleteFriendGroupWithResponse(env.ctx, groupID) })

	get, err := env.api.GetFriendGroupWithResponse(env.ctx, groupID)
	if err != nil {
		t.Fatalf("get friend group: %v", err)
	}
	requireStatusOK(t, get, get.Body)
	if get.JSON200 == nil || get.JSON200.WorkspaceName == nil || *get.JSON200.WorkspaceName == "" {
		t.Fatalf("get friend group = %#v", get.JSON200)
	}

	renamed, err := env.api.PutFriendGroupWithResponse(env.ctx, groupID, adminservice.AdminFriendGroupPutRequest{
		Name:        ptr(mutationName("renamed-group")),
		Description: ptr("renamed"),
	})
	if err != nil {
		t.Fatalf("put friend group: %v", err)
	}
	requireStatusOK(t, renamed, renamed.Body)
	if renamed.JSON200 == nil || renamed.JSON200.Name == nil || *renamed.JSON200.Name != mutationName("renamed-group") {
		t.Fatalf("renamed friend group = %#v", renamed.JSON200)
	}

	owner, err := env.api.CreateFriendGroupMemberWithResponse(env.ctx, groupID, adminservice.AdminFriendGroupMemberCreateRequest{
		PeerPublicKey: env.adminKey,
		Role:          rpcapi.FriendGroupMemberRoleOwner,
	})
	if err != nil {
		t.Fatalf("create owner member: %v", err)
	}
	requireStatusOK(t, owner, owner.Body)
	if owner.JSON200 == nil || owner.JSON200.Role == nil || *owner.JSON200.Role != rpcapi.FriendGroupMemberRoleOwner {
		t.Fatalf("owner member = %#v", owner.JSON200)
	}

	member, err := env.api.CreateFriendGroupMemberWithResponse(env.ctx, groupID, adminservice.AdminFriendGroupMemberCreateRequest{
		PeerPublicKey: env.peerKey,
		Role:          rpcapi.FriendGroupMemberRoleMember,
	})
	if err != nil {
		t.Fatalf("create member: %v", err)
	}
	requireStatusOK(t, member, member.Body)
	if member.JSON200 == nil || member.JSON200.Role == nil || *member.JSON200.Role != rpcapi.FriendGroupMemberRoleMember {
		t.Fatalf("member = %#v", member.JSON200)
	}

	updatedMember, err := env.api.PutFriendGroupMemberWithResponse(env.ctx, groupID, env.peerKey, adminservice.AdminFriendGroupMemberPutRequest{
		Role: rpcapi.FriendGroupMemberRoleAdmin,
	})
	if err != nil {
		t.Fatalf("put member: %v", err)
	}
	requireStatusOK(t, updatedMember, updatedMember.Body)
	if updatedMember.JSON200 == nil || updatedMember.JSON200.Role == nil || *updatedMember.JSON200.Role != rpcapi.FriendGroupMemberRoleAdmin {
		t.Fatalf("updated member = %#v", updatedMember.JSON200)
	}

	members := collectAdminPagesInt(t, 1, func(cursor *string, limit int) ([]rpcapi.FriendGroupMemberObject, bool, *string) {
		resp, err := env.api.ListFriendGroupMembersWithResponse(env.ctx, groupID, &adminservice.ListFriendGroupMembersParams{Cursor: cursor, Limit: &limit})
		if err != nil {
			t.Fatalf("list friend group members: %v", err)
		}
		requireStatusOK(t, resp, resp.Body)
		if resp.JSON200 == nil {
			t.Fatalf("list friend group members missing JSON200")
		}
		return resp.JSON200.Items, resp.JSON200.HasNext, resp.JSON200.NextCursor
	})
	requireName(t, members, env.peerKey, func(item rpcapi.FriendGroupMemberObject) string {
		if item.PeerPublicKey == nil {
			return ""
		}
		return *item.PeerPublicKey
	})

	expiresAt := time.Now().UTC().Add(10 * time.Minute)
	token, err := env.api.PutFriendGroupInviteTokenWithResponse(env.ctx, groupID, adminservice.AdminFriendGroupInviteTokenPutRequest{
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
