//go:build gizclaw_e2e

package admin_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestAdminAPIApplyResource(t *testing.T) {
	env := newAdminAPIHarness(t)

	name := mutationName("apply-workflow")
	_, _ = env.api.DeleteWorkflowWithResponse(env.ctx, name)
	var resource apitypes.Resource
	if err := resource.FromWorkflowResource(apitypes.WorkflowResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.WorkflowResourceKindWorkflow,
		Metadata:   apitypes.ResourceMetadata{Name: name},
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
	if resp.JSON200 == nil || resp.JSON200.Name != name || resp.JSON200.Kind != apitypes.ResourceKindWorkflow {
		t.Fatalf("apply resource = %#v", resp.JSON200)
	}
	deleted, err := env.api.DeleteWorkflowWithResponse(env.ctx, name)
	if err != nil {
		t.Fatalf("delete applied workflow: %v", err)
	}
	requireStatusOK(t, deleted, deleted.Body)
}

func TestAdminAPIApplySocialResources(t *testing.T) {
	env := newAdminAPIHarness(t)

	owner, peer := sortedPublicKeys(env.adminKey, env.peerKey)
	relationID := owner + ":" + peer
	groupID := mutationName("social-group")
	memberName := groupID + ":" + env.peerKey
	expiresAt := time.Now().UTC().Add(30 * time.Minute)

	t.Cleanup(func() {
		_, _ = env.api.DeleteResourceWithResponse(env.ctx, apitypes.ResourceKindFriendGroupInviteToken, groupID)
		_, _ = env.api.DeleteResourceWithResponse(env.ctx, apitypes.ResourceKindFriendGroupMember, memberName)
		_, _ = env.api.DeleteResourceWithResponse(env.ctx, apitypes.ResourceKindFriendGroup, groupID)
		_, _ = env.api.DeleteResourceWithResponse(env.ctx, apitypes.ResourceKindFriend, relationID)
	})

	applyAndRequire(t, env, apitypes.ResourceKindFriend, relationID, friendResource(t, relationID, owner, peer))
	applyAndRequire(t, env, apitypes.ResourceKindFriendGroup, groupID, friendGroupResource(t, groupID, env.adminKey, "E2E Mut Social Group", "created by e2e apply"))
	applyAndRequire(t, env, apitypes.ResourceKindFriendGroupMember, memberName, friendGroupMemberResource(t, memberName, groupID, env.peerKey, apitypes.FriendGroupMemberRoleMember))
	applyAndRequire(t, env, apitypes.ResourceKindFriendGroupInviteToken, groupID, friendGroupInviteTokenResource(t, groupID, "e2e-mut-social-token", expiresAt))
}

func TestAdminAPIApplyRejectsInvalidCustomIDResources(t *testing.T) {
	env := newAdminAPIHarness(t)

	for _, tc := range []struct {
		name     string
		resource apitypes.Resource
	}{
		{
			name:     "workflow metadata.name",
			resource: workflowResource(t, "short"),
		},
		{
			name:     "friend group metadata.name",
			resource: friendGroupResource(t, "family", env.adminKey, "Family", "invalid short group id"),
		},
		{
			name:     "contact id segment",
			resource: contactResource(t, env.peerKey+":alice", env.peerKey, "alice"),
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

func applyAndRequire(t *testing.T, env *adminAPIHarness, kind apitypes.ResourceKind, name string, resource apitypes.Resource) {
	t.Helper()

	resp, err := env.api.ApplyResourceWithResponse(env.ctx, resource)
	if err != nil {
		t.Fatalf("apply %s %s: %v", kind, name, err)
	}
	requireStatusOK(t, resp, resp.Body)
	if resp.JSON200 == nil || resp.JSON200.Kind != kind || resp.JSON200.Name != name {
		t.Fatalf("apply %s %s = %#v", kind, name, resp.JSON200)
	}
}

func workflowResource(t *testing.T, name string) apitypes.Resource {
	t.Helper()

	var resource apitypes.Resource
	if err := resource.FromWorkflowResource(apitypes.WorkflowResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.WorkflowResourceKindWorkflow,
		Metadata:   apitypes.ResourceMetadata{Name: name},
		Spec: apitypes.WorkflowSpec{
			Driver:    apitypes.WorkflowDriverFlowcraft,
			Flowcraft: testFlowcraftWorkflowSpec(),
		},
	}); err != nil {
		t.Fatalf("build workflow resource: %v", err)
	}
	return resource
}

func contactResource(t *testing.T, name, owner, id string) apitypes.Resource {
	t.Helper()

	var resource apitypes.Resource
	if err := resource.FromContactResource(apitypes.ContactResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.ContactResourceKindContact,
		Metadata:   apitypes.ResourceMetadata{Name: name},
		Spec: apitypes.ContactSpec{
			OwnerPublicKey: owner,
			Id:             id,
			DisplayName:    ptr("Invalid Contact"),
		},
	}); err != nil {
		t.Fatalf("build contact resource: %v", err)
	}
	return resource
}

func friendResource(t *testing.T, name, owner, peer string) apitypes.Resource {
	t.Helper()

	var resource apitypes.Resource
	if err := resource.FromFriendResource(apitypes.FriendResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FriendResourceKindFriend,
		Metadata:   apitypes.ResourceMetadata{Name: name},
		Spec: apitypes.FriendSpec{
			OwnerPublicKey: owner,
			PeerPublicKey:  peer,
		},
	}); err != nil {
		t.Fatalf("build friend resource: %v", err)
	}
	return resource
}

func friendGroupResource(t *testing.T, id, owner, name, description string) apitypes.Resource {
	t.Helper()

	var resource apitypes.Resource
	if err := resource.FromFriendGroupResource(apitypes.FriendGroupResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FriendGroupResourceKindFriendGroup,
		Metadata:   apitypes.ResourceMetadata{Name: id},
		Spec: apitypes.FriendGroupSpec{
			OwnerPublicKey: owner,
			Name:           name,
			Description:    ptr(description),
		},
	}); err != nil {
		t.Fatalf("build friend group resource: %v", err)
	}
	return resource
}

func friendGroupMemberResource(t *testing.T, name, groupID, peer string, role apitypes.FriendGroupMemberRole) apitypes.Resource {
	t.Helper()

	var resource apitypes.Resource
	if err := resource.FromFriendGroupMemberResource(apitypes.FriendGroupMemberResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FriendGroupMemberResourceKindFriendGroupMember,
		Metadata:   apitypes.ResourceMetadata{Name: name},
		Spec: apitypes.FriendGroupMemberSpec{
			FriendGroupId: groupID,
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
		Metadata:   apitypes.ResourceMetadata{Name: groupID},
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
