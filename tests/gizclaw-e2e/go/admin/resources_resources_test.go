//go:build gizclaw_e2e

package admin_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

const (
	e2eSocialAdminPublicKey  = "6Ww6ANsXDCf91Yp7Tvi65hqpywjMmXqAoZDiq33kfCee"
	e2eSocialClientPublicKey = "8rAUkTyxLHDa5o3VajtzWcQdNJq1thrjAGtpwQkEsaEu"
	e2eSocialRelationID      = e2eSocialAdminPublicKey + ":" + e2eSocialClientPublicKey
	e2eSocialGroupID         = "family-circle"
	e2eSocialClientMemberID  = e2eSocialGroupID + ":" + e2eSocialClientPublicKey
)

func TestAdminAPIResourcesGet(t *testing.T) {
	env := newAdminAPIHarness(t)

	get, err := env.api.GetResourceWithResponse(env.ctx, apitypes.ResourceKindWorkflow, "flowcraft-support")
	if err != nil {
		t.Fatalf("get workflow resource: %v", err)
	}
	requireStatusOK(t, get, get.Body)
	if get.JSON200 == nil {
		t.Fatalf("get resource = %#v", get.JSON200)
	}
	workflow, err := get.JSON200.AsWorkflowResource()
	if err != nil {
		t.Fatalf("decode workflow resource union: %v", err)
	}
	if workflow.Metadata.Name != "flowcraft-support" {
		t.Fatalf("workflow resource name = %q", workflow.Metadata.Name)
	}
}

func TestAdminAPIResourcesGetSocialFixtures(t *testing.T) {
	env := newAdminAPIHarness(t)

	friendResp, err := env.api.GetResourceWithResponse(env.ctx, apitypes.ResourceKindFriend, e2eSocialRelationID)
	if err != nil {
		t.Fatalf("get friend resource: %v", err)
	}
	requireStatusOK(t, friendResp, friendResp.Body)
	friend, err := friendResp.JSON200.AsFriendResource()
	if err != nil {
		t.Fatalf("decode friend resource union: %v", err)
	}
	if friend.Spec.OwnerPublicKey != e2eSocialAdminPublicKey || friend.Spec.PeerPublicKey != e2eSocialClientPublicKey {
		t.Fatalf("friend spec = %+v", friend.Spec)
	}

	groupResp, err := env.api.GetResourceWithResponse(env.ctx, apitypes.ResourceKindFriendGroup, e2eSocialGroupID)
	if err != nil {
		t.Fatalf("get friend group resource: %v", err)
	}
	requireStatusOK(t, groupResp, groupResp.Body)
	group, err := groupResp.JSON200.AsFriendGroupResource()
	if err != nil {
		t.Fatalf("decode friend group resource union: %v", err)
	}
	if group.Spec.Name != "Family Circle" {
		t.Fatalf("friend group spec = %+v", group.Spec)
	}

	memberResp, err := env.api.GetResourceWithResponse(env.ctx, apitypes.ResourceKindFriendGroupMember, e2eSocialClientMemberID)
	if err != nil {
		t.Fatalf("get friend group member resource: %v", err)
	}
	requireStatusOK(t, memberResp, memberResp.Body)
	member, err := memberResp.JSON200.AsFriendGroupMemberResource()
	if err != nil {
		t.Fatalf("decode friend group member resource union: %v", err)
	}
	if member.Spec.PeerPublicKey != e2eSocialClientPublicKey || member.Spec.Role != apitypes.FriendGroupMemberRoleMember {
		t.Fatalf("friend group member spec = %+v", member.Spec)
	}

	tokenResp, err := env.api.GetResourceWithResponse(env.ctx, apitypes.ResourceKindFriendGroupInviteToken, e2eSocialGroupID)
	if err != nil {
		t.Fatalf("get friend group invite token resource: %v", err)
	}
	requireStatusOK(t, tokenResp, tokenResp.Body)
	token, err := tokenResp.JSON200.AsFriendGroupInviteTokenResource()
	if err != nil {
		t.Fatalf("decode friend group invite token resource union: %v", err)
	}
	if token.Spec.InviteToken != "family-circle-token" || token.Spec.FriendGroupId != e2eSocialGroupID {
		t.Fatalf("friend group invite token spec = %+v", token.Spec)
	}
}
