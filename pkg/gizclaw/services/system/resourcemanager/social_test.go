package resourcemanager

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
	"github.com/GizClaw/gizclaw-go/pkg/store/objectstore"
)

func TestApplyFriendResourceCreatesAndDeletes(t *testing.T) {
	manager := newSocialResourceManager(t)

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Friend",
		"metadata": {"name": "peer-a:peer-b"},
		"spec": {"owner_public_key": "peer-a", "peer_public_key": "peer-b"}
	}`))
	if err != nil {
		t.Fatalf("Apply(create) returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("create action = %q, want created", result.Action)
	}

	result, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Friend",
		"metadata": {"name": "peer-a:peer-b"},
		"spec": {"owner_public_key": "peer-a", "peer_public_key": "peer-b"}
	}`))
	if err != nil {
		t.Fatalf("Apply(unchanged) returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("unchanged action = %q, want unchanged", result.Action)
	}

	resource, err := manager.Get(context.Background(), apitypes.ResourceKindFriend, "peer-a:peer-b")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	got, err := resource.AsFriendResource()
	if err != nil {
		t.Fatalf("AsFriendResource returned error: %v", err)
	}
	if got.Spec.OwnerPublicKey != "peer-a" || got.Spec.PeerPublicKey != "peer-b" {
		t.Fatalf("friend spec = %+v", got.Spec)
	}

	deleted, err := manager.Delete(context.Background(), apitypes.ResourceKindFriend, "peer-a:peer-b")
	if err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	deletedFriend, err := deleted.AsFriendResource()
	if err != nil {
		t.Fatalf("deleted AsFriendResource returned error: %v", err)
	}
	if deletedFriend.Metadata.Name != "peer-a:peer-b" {
		t.Fatalf("deleted metadata.name = %q, want peer-a:peer-b", deletedFriend.Metadata.Name)
	}
	_, err = manager.Get(context.Background(), apitypes.ResourceKindFriend, "peer-a:peer-b")
	assertResourceError(t, err, 404, "RESOURCE_NOT_FOUND")
}

func TestApplyFriendGroupResourceCreatesUpdatesAndDeletes(t *testing.T) {
	manager := newSocialResourceManager(t)

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroup",
		"metadata": {"name": "family"},
		"spec": {"name": "Family", "description": "voice room"}
	}`))
	if err != nil {
		t.Fatalf("Apply(create) returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("create action = %q, want created", result.Action)
	}

	result, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroup",
		"metadata": {"name": "family"},
		"spec": {"name": "Family", "description": "voice room"}
	}`))
	if err != nil {
		t.Fatalf("Apply(unchanged) returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("unchanged action = %q, want unchanged", result.Action)
	}

	resource, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroup",
		"metadata": {"name": "family"},
		"spec": {"name": "Family+", "description": "updated"}
	}`))
	if err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
	group, err := resource.AsFriendGroupResource()
	if err != nil {
		t.Fatalf("AsFriendGroupResource returned error: %v", err)
	}
	if group.Spec.Name != "Family+" || group.Spec.Description == nil || *group.Spec.Description != "updated" {
		t.Fatalf("friend group spec = %+v", group.Spec)
	}

	deleted, err := manager.Delete(context.Background(), apitypes.ResourceKindFriendGroup, "family")
	if err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	deletedGroup, err := deleted.AsFriendGroupResource()
	if err != nil {
		t.Fatalf("deleted AsFriendGroupResource returned error: %v", err)
	}
	if deletedGroup.Metadata.Name != "family" {
		t.Fatalf("deleted metadata.name = %q, want family", deletedGroup.Metadata.Name)
	}
	_, err = manager.Get(context.Background(), apitypes.ResourceKindFriendGroup, "family")
	assertResourceError(t, err, 404, "RESOURCE_NOT_FOUND")
}

func TestApplyFriendGroupMemberResourceCreatesUpdatesAndDeletes(t *testing.T) {
	manager := newSocialResourceManager(t)
	createFriendGroup(t, manager, "family")

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupMember",
		"metadata": {"name": "family:peer-a"},
		"spec": {"friend_group_id": "family", "peer_public_key": "peer-a", "role": "member"}
	}`))
	if err != nil {
		t.Fatalf("Apply(create) returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("create action = %q, want created", result.Action)
	}

	result, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupMember",
		"metadata": {"name": "family:peer-a"},
		"spec": {"friend_group_id": "family", "peer_public_key": "peer-a", "role": "admin"}
	}`))
	if err != nil {
		t.Fatalf("Apply(update) returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("update action = %q, want updated", result.Action)
	}

	result, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupMember",
		"metadata": {"name": "family:peer-a"},
		"spec": {"friend_group_id": "family", "peer_public_key": "peer-a", "role": "admin"}
	}`))
	if err != nil {
		t.Fatalf("Apply(unchanged) returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("unchanged action = %q, want unchanged", result.Action)
	}

	resource, err := manager.Get(context.Background(), apitypes.ResourceKindFriendGroupMember, "family:peer-a")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	member, err := resource.AsFriendGroupMemberResource()
	if err != nil {
		t.Fatalf("AsFriendGroupMemberResource returned error: %v", err)
	}
	if member.Spec.Role != apitypes.FriendGroupMemberRoleAdmin {
		t.Fatalf("member role = %q, want admin", member.Spec.Role)
	}

	deleted, err := manager.Delete(context.Background(), apitypes.ResourceKindFriendGroupMember, "family:peer-a")
	if err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	deletedMember, err := deleted.AsFriendGroupMemberResource()
	if err != nil {
		t.Fatalf("deleted AsFriendGroupMemberResource returned error: %v", err)
	}
	if deletedMember.Metadata.Name != "family:peer-a" {
		t.Fatalf("deleted metadata.name = %q, want family:peer-a", deletedMember.Metadata.Name)
	}
	_, err = manager.Get(context.Background(), apitypes.ResourceKindFriendGroupMember, "family:peer-a")
	assertResourceError(t, err, 404, "RESOURCE_NOT_FOUND")
}

func TestApplyFriendGroupInviteTokenResourceCreatesUpdatesAndDeletes(t *testing.T) {
	manager := newSocialResourceManager(t)
	createFriendGroup(t, manager, "family")
	expiresAt := time.Now().UTC().Add(time.Hour)

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupInviteToken",
		"metadata": {"name": "family"},
		"spec": {"friend_group_id": "family", "invite_token": "token-a", "expires_at": "`+expiresAt.Format(time.RFC3339Nano)+`"}
	}`))
	if err != nil {
		t.Fatalf("Apply(create) returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("create action = %q, want created", result.Action)
	}

	result, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupInviteToken",
		"metadata": {"name": "family"},
		"spec": {"friend_group_id": "family", "invite_token": "token-b", "expires_at": "`+expiresAt.Add(time.Hour).Format(time.RFC3339Nano)+`"}
	}`))
	if err != nil {
		t.Fatalf("Apply(update) returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("update action = %q, want updated", result.Action)
	}

	result, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupInviteToken",
		"metadata": {"name": "family"},
		"spec": {"friend_group_id": "family", "invite_token": "token-b", "expires_at": "`+expiresAt.Add(time.Hour).Format(time.RFC3339Nano)+`"}
	}`))
	if err != nil {
		t.Fatalf("Apply(unchanged) returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("unchanged action = %q, want unchanged", result.Action)
	}

	resource, err := manager.Get(context.Background(), apitypes.ResourceKindFriendGroupInviteToken, "family")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	token, err := resource.AsFriendGroupInviteTokenResource()
	if err != nil {
		t.Fatalf("AsFriendGroupInviteTokenResource returned error: %v", err)
	}
	if token.Spec.InviteToken != "token-b" {
		t.Fatalf("invite token = %q, want token-b", token.Spec.InviteToken)
	}

	deleted, err := manager.Delete(context.Background(), apitypes.ResourceKindFriendGroupInviteToken, "family")
	if err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	deletedToken, err := deleted.AsFriendGroupInviteTokenResource()
	if err != nil {
		t.Fatalf("deleted AsFriendGroupInviteTokenResource returned error: %v", err)
	}
	if deletedToken.Spec.InviteToken != "token-b" {
		t.Fatalf("deleted invite token = %q, want token-b", deletedToken.Spec.InviteToken)
	}
	_, err = manager.Get(context.Background(), apitypes.ResourceKindFriendGroupInviteToken, "family")
	assertResourceError(t, err, 404, "RESOURCE_NOT_FOUND")
}

func TestSocialResourceValidationRejectsMismatchedNames(t *testing.T) {
	manager := newSocialResourceManager(t)

	_, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Friend",
		"metadata": {"name": "peer-b:peer-a"},
		"spec": {"owner_public_key": "peer-b", "peer_public_key": "peer-a"}
	}`))
	assertResourceError(t, err, 400, "INVALID_FRIEND_RESOURCE")

	_, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Friend",
		"metadata": {"name": "peer-a:peer-b"},
		"spec": {"owner_public_key": "peer-a", "peer_public_key": "peer-c"}
	}`))
	assertResourceError(t, err, 400, "INVALID_FRIEND_RESOURCE")

	_, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroup",
		"metadata": {"name": "family"},
		"spec": {"name": " "}
	}`))
	assertResourceError(t, err, 400, "INVALID_FRIEND_GROUP_RESOURCE")

	_, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupInviteToken",
		"metadata": {"name": "family"},
		"spec": {"friend_group_id": "other", "invite_token": "token", "expires_at": "2099-01-01T00:00:00Z"}
	}`))
	assertResourceError(t, err, 400, "INVALID_FRIEND_GROUP_INVITE_TOKEN_RESOURCE")

	_, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupInviteToken",
		"metadata": {"name": "family"},
		"spec": {"friend_group_id": "family", "invite_token": " ", "expires_at": "0001-01-01T00:00:00Z"}
	}`))
	assertResourceError(t, err, 400, "INVALID_FRIEND_GROUP_INVITE_TOKEN_RESOURCE")

	_, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupMember",
		"metadata": {"name": "family:peer-b"},
		"spec": {"friend_group_id": "family", "peer_public_key": "peer-a", "role": "member"}
	}`))
	assertResourceError(t, err, 400, "INVALID_FRIEND_GROUP_MEMBER_RESOURCE")

	_, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupMember",
		"metadata": {"name": "family:peer-a"},
		"spec": {"friend_group_id": "family", "peer_public_key": "peer-a", "role": "observer"}
	}`))
	assertResourceError(t, err, 400, "INVALID_FRIEND_GROUP_MEMBER_RESOURCE")
}

func TestSocialResourcesRequireConfiguredServices(t *testing.T) {
	manager := New(Services{})

	_, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Friend",
		"metadata": {"name": "peer-a:peer-b"},
		"spec": {"owner_public_key": "peer-a", "peer_public_key": "peer-b"}
	}`))
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroup",
		"metadata": {"name": "family"},
		"spec": {"name": "Family"}
	}`))
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Get(context.Background(), apitypes.ResourceKindFriendGroupMember, "family:peer-a")
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupInviteToken",
		"metadata": {"name": "family"},
		"spec": {"friend_group_id": "family", "invite_token": "token", "expires_at": "2099-01-01T00:00:00Z"}
	}`))
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Friend",
		"metadata": {"name": "peer-a:peer-b"},
		"spec": {"owner_public_key": "peer-a", "peer_public_key": "peer-b"}
	}`))
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroup",
		"metadata": {"name": "family"},
		"spec": {"name": "Family"}
	}`))
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupMember",
		"metadata": {"name": "family:peer-a"},
		"spec": {"friend_group_id": "family", "peer_public_key": "peer-a", "role": "member"}
	}`))
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Get(context.Background(), apitypes.ResourceKindFriend, "peer-a:peer-b")
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Get(context.Background(), apitypes.ResourceKindFriendGroup, "family")
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Get(context.Background(), apitypes.ResourceKindFriendGroupInviteToken, "family")
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Delete(context.Background(), apitypes.ResourceKindFriend, "peer-a:peer-b")
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Delete(context.Background(), apitypes.ResourceKindFriendGroup, "family")
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Delete(context.Background(), apitypes.ResourceKindFriendGroupInviteToken, "family")
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Delete(context.Background(), apitypes.ResourceKindFriendGroupMember, "family:peer-a")
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")
}

func TestSocialResourcePutAndDeleteBranches(t *testing.T) {
	manager := newSocialResourceManager(t)

	friend, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Friend",
		"metadata": {"name": "peer-a:peer-b"},
		"spec": {"owner_public_key": "peer-a", "peer_public_key": "peer-b"}
	}`))
	if err != nil {
		t.Fatalf("Put friend returned error: %v", err)
	}
	if got, err := friend.AsFriendResource(); err != nil || got.Metadata.Name != "peer-a:peer-b" {
		t.Fatalf("Put friend resource = %#v, err = %v", got, err)
	}

	group, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroup",
		"metadata": {"name": "family"},
		"spec": {"name": "Family", "description": "room"}
	}`))
	if err != nil {
		t.Fatalf("Put group returned error: %v", err)
	}
	if got, err := group.AsFriendGroupResource(); err != nil || got.Spec.Name != "Family" {
		t.Fatalf("Put group resource = %#v, err = %v", got, err)
	}

	member, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupMember",
		"metadata": {"name": "family:peer-a"},
		"spec": {"friend_group_id": "family", "peer_public_key": "peer-a", "role": "owner"}
	}`))
	if err != nil {
		t.Fatalf("Put member returned error: %v", err)
	}
	if got, err := member.AsFriendGroupMemberResource(); err != nil || got.Spec.Role != apitypes.FriendGroupMemberRoleOwner {
		t.Fatalf("Put member resource = %#v, err = %v", got, err)
	}

	expiresAt := time.Now().UTC().Add(time.Hour)
	token, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupInviteToken",
		"metadata": {"name": "family"},
		"spec": {"friend_group_id": "family", "invite_token": "token-a", "expires_at": "`+expiresAt.Format(time.RFC3339Nano)+`"}
	}`))
	if err != nil {
		t.Fatalf("Put invite token returned error: %v", err)
	}
	if got, err := token.AsFriendGroupInviteTokenResource(); err != nil || got.Spec.InviteToken != "token-a" {
		t.Fatalf("Put invite token resource = %#v, err = %v", got, err)
	}

	if _, err := manager.Delete(context.Background(), apitypes.ResourceKindFriend, "invalid"); err == nil {
		t.Fatal("Delete invalid friend id error = nil")
	}
	if _, err := manager.Delete(context.Background(), apitypes.ResourceKindFriendGroupInviteToken, "missing"); err == nil {
		t.Fatal("Delete missing invite token error = nil")
	}
	if _, err := manager.Delete(context.Background(), apitypes.ResourceKindFriendGroupMember, "family"); err == nil {
		t.Fatal("Delete invalid member id error = nil")
	}
	if _, err := manager.Get(context.Background(), apitypes.ResourceKindFriendGroupInviteToken, "missing"); err == nil {
		t.Fatal("Get missing invite token error = nil")
	}
}

func createFriendGroup(t *testing.T, manager *Manager, name string) {
	t.Helper()

	if _, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroup",
		"metadata": {"name": "`+name+`"},
		"spec": {"name": "`+name+`"}
	}`)); err != nil {
		t.Fatalf("create friend group %s: %v", name, err)
	}
}

func newSocialResourceManager(t *testing.T) *Manager {
	t.Helper()

	return New(Services{
		Friends: &friend.Server{
			InviteTokens: kv.NewMemory(nil),
			Friends:      kv.NewMemory(nil),
		},
		FriendGroups: &friendgroup.Server{
			Groups:        kv.NewMemory(nil),
			InviteTokens:  kv.NewMemory(nil),
			Members:       kv.NewMemory(nil),
			Belongs:       kv.NewMemory(nil),
			Messages:      kv.NewMemory(nil),
			MessageAssets: objectstore.Dir(t.TempDir()),
		},
	})
}
