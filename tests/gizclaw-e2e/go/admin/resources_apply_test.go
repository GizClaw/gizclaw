//go:build gizclaw_e2e

package admin_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
)

func TestAdminAPIApplyResource(t *testing.T) {
	env := newAdminAPIHarness(t)

	name := mutationName("apply-workflow")
	var resource apitypes.Resource
	if err := resource.FromWorkflowResource(apitypes.WorkflowResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.WorkflowResourceKindWorkflow,
		Metadata:   apitypes.ResourceMetadata{Id: name},
		Spec: apitypes.WorkflowSpec{
			Driver:    apitypes.WorkflowDriverFlowcraft,
			Flowcraft: testFlowcraftWorkflowSpec(),
		},
	}); err != nil {
		t.Fatalf("build workflow resource: %v", err)
	}
	resp, err := env.api.ApplyResourceWithResponse(env.ctx, resource)
	if err != nil {
		t.Fatalf("apply resource: %v", err)
	}
	requireStatusOK(t, resp, resp.Body)
	if resp.JSON200 == nil || resp.JSON200.Id == nil || *resp.JSON200.Id != name || resp.JSON200.Kind != apitypes.ResourceKindWorkflow {
		t.Fatalf("apply resource = %#v", resp.JSON200)
	}
	deleted, err := env.api.DeleteWorkflowWithResponse(env.ctx, *resp.JSON200.Id)
	if err != nil {
		t.Fatalf("delete applied workflow: %v", err)
	}
	requireStatusOK(t, deleted, deleted.Body)
}

func TestAdminAPIApplySocialResources(t *testing.T) {
	env := newAdminAPIHarness(t)
	peerClient := env.h.ConnectClientFromContext("admin-api-peer")
	defer peerClient.Close()
	registerAdminHistoryPeers(t, env, env.admin, peerClient)

	owner, peer := sortedPublicKeys(env.adminKey, env.peerKey)
	relationID := owner + ":" + peer
	groupResourceID := mutationName("social-group-id")
	groupName := mutationName("social-group")
	expiresAt := time.Now().UTC().Add(30 * time.Minute)

	friendID := applyAndRequire(t, env, apitypes.ResourceKindFriend, relationID, friendResource(t, relationID, owner, peer))
	groupID := applyAndRequire(t, env, apitypes.ResourceKindFriendGroup, groupResourceID, friendGroupResource(t, groupResourceID, groupName, env.adminKey, "E2E Mut Social Group", "created by e2e apply"))
	membershipID := customid.MembershipName(groupID, env.peerKey)
	memberID := applyAndRequire(t, env, apitypes.ResourceKindFriendGroupMember, membershipID, friendGroupMemberResource(t, membershipID, groupName, groupID, env.peerKey, apitypes.FriendGroupMemberRoleMember))
	tokenID := applyAndRequire(t, env, apitypes.ResourceKindFriendGroupInviteToken, groupID, friendGroupInviteTokenResource(t, groupID, "e2e-mut-social-token", expiresAt))
	t.Cleanup(func() {
		_, _ = env.api.DeleteResourceWithResponse(env.ctx, apitypes.ResourceKindFriendGroupInviteToken, tokenID)
		_, _ = env.api.DeleteResourceWithResponse(env.ctx, apitypes.ResourceKindFriendGroupMember, memberID)
		_, _ = env.api.DeleteResourceWithResponse(env.ctx, apitypes.ResourceKindFriendGroup, groupID)
		_, _ = env.api.DeleteResourceWithResponse(env.ctx, apitypes.ResourceKindFriend, friendID)
	})
}

func TestAdminAPIApplyRejectsInvalidResourceIdentities(t *testing.T) {
	env := newAdminAPIHarness(t)

	for _, tc := range []struct {
		name     string
		resource apitypes.Resource
	}{
		{
			name:     "workflow metadata.id",
			resource: workflowResource(t, " leading"),
		},
		{
			name:     "friend group spec.name",
			resource: friendGroupResource(t, mutationName("family-id"), "family", env.adminKey, "Family", "invalid short group name"),
		},
		{
			name:     "contact spec.name",
			resource: contactResource(t, mutationName("alice-id"), "alice", env.peerKey),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := env.api.ApplyResourceWithResponse(env.ctx, tc.resource)
			if err != nil {
				t.Fatalf("apply invalid resource: %v", err)
			}
			if resp.StatusCode() != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", resp.StatusCode(), resp.Body)
			}
		})
	}
}

func applyAndRequire(t *testing.T, env *adminAPIHarness, kind apitypes.ResourceKind, id string, resource apitypes.Resource) string {
	t.Helper()

	resp, err := env.api.ApplyResourceWithResponse(env.ctx, resource)
	if err != nil {
		t.Fatalf("apply %s %s: %v", kind, id, err)
	}
	requireStatusOK(t, resp, resp.Body)
	if resp.JSON200 == nil || resp.JSON200.Id == nil || *resp.JSON200.Id != id || resp.JSON200.Kind != kind {
		t.Fatalf("apply %s %s = %#v", kind, id, resp.JSON200)
	}
	return *resp.JSON200.Id
}

func workflowResource(t *testing.T, id string) apitypes.Resource {
	t.Helper()

	var resource apitypes.Resource
	if err := resource.FromWorkflowResource(apitypes.WorkflowResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.WorkflowResourceKindWorkflow,
		Metadata:   apitypes.ResourceMetadata{Id: id},
		Spec: apitypes.WorkflowSpec{
			Driver:    apitypes.WorkflowDriverFlowcraft,
			Flowcraft: testFlowcraftWorkflowSpec(),
		},
	}); err != nil {
		t.Fatalf("build workflow resource: %v", err)
	}
	return resource
}

func contactResource(t *testing.T, id, name, owner string) apitypes.Resource {
	t.Helper()

	var resource apitypes.Resource
	if err := resource.FromContactResource(apitypes.ContactResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.ContactResourceKindContact,
		Metadata:   apitypes.ResourceMetadata{Id: id},
		Spec: apitypes.ContactSpec{
			OwnerPublicKey: owner,
			Name:           name,
			DisplayName:    ptr("Invalid Contact"),
		},
	}); err != nil {
		t.Fatalf("build contact resource: %v", err)
	}
	return resource
}

func friendResource(t *testing.T, id, owner, peer string) apitypes.Resource {
	t.Helper()

	var resource apitypes.Resource
	if err := resource.FromFriendResource(apitypes.FriendResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FriendResourceKindFriend,
		Metadata:   apitypes.ResourceMetadata{Id: id},
		Spec: apitypes.FriendSpec{
			OwnerPublicKey: owner,
			PeerPublicKey:  peer,
		},
	}); err != nil {
		t.Fatalf("build friend resource: %v", err)
	}
	return resource
}

func friendGroupResource(t *testing.T, id, name, owner, displayName, description string) apitypes.Resource {
	t.Helper()

	var resource apitypes.Resource
	if err := resource.FromFriendGroupResource(apitypes.FriendGroupResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FriendGroupResourceKindFriendGroup,
		Metadata:   apitypes.ResourceMetadata{Id: id},
		Spec: apitypes.FriendGroupSpec{
			OwnerPublicKey: owner,
			Name:           name,
			DisplayName:    ptr(displayName),
			Description:    ptr(description),
		},
	}); err != nil {
		t.Fatalf("build friend group resource: %v", err)
	}
	return resource
}

func friendGroupMemberResource(t *testing.T, id, name, groupID, peer string, role apitypes.FriendGroupMemberRole) apitypes.Resource {
	t.Helper()

	var resource apitypes.Resource
	if err := resource.FromFriendGroupMemberResource(apitypes.FriendGroupMemberResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FriendGroupMemberResourceKindFriendGroupMember,
		Metadata:   apitypes.ResourceMetadata{Id: id},
		Spec: apitypes.FriendGroupMemberSpec{
			FriendGroupId: groupID,
			Name:          name,
			PeerPublicKey: peer,
			Role:          role,
		},
	}); err != nil {
		t.Fatalf("build friend group member resource: %v", err)
	}
	return resource
}

func friendGroupInviteTokenResource(t *testing.T, groupID, token string, expiresAt time.Time) apitypes.Resource {
	t.Helper()

	var resource apitypes.Resource
	if err := resource.FromFriendGroupInviteTokenResource(apitypes.FriendGroupInviteTokenResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FriendGroupInviteTokenResourceKindFriendGroupInviteToken,
		Metadata:   apitypes.ResourceMetadata{Id: groupID},
		Spec: apitypes.FriendGroupInviteTokenSpec{
			FriendGroupId: groupID,
			InviteToken:   token,
			ExpiresAt:     expiresAt,
		},
	}); err != nil {
		t.Fatalf("build friend group invite token resource: %v", err)
	}
	return resource
}

func sortedPublicKeys(a, b string) (string, string) {
	if b < a {
		return b, a
	}
	return a, b
}
