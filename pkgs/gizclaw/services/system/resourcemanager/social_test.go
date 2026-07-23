package resourcemanager

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/contact"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
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

func TestApplyContactResourceCreatesUpdatesAndDeletes(t *testing.T) {
	manager := newSocialResourceManager(t)

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Contact",
		"metadata": {"name": "peer-a:alice001"},
		"spec": {"owner_public_key": "peer-a", "id": "alice001", "display_name": "Alice", "phone_number": "+1 555 0100"}
	}`))
	if err != nil {
		t.Fatalf("Apply(create) returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("create action = %q, want created", result.Action)
	}

	result, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Contact",
		"metadata": {"name": "peer-a:alice001"},
		"spec": {"owner_public_key": "peer-a", "id": "alice001", "display_name": "Alice", "phone_number": "+1 555 0100"}
	}`))
	if err != nil {
		t.Fatalf("Apply(unchanged) returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("unchanged action = %q, want unchanged", result.Action)
	}

	result, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Contact",
		"metadata": {"name": "peer-a:alice001"},
		"spec": {"owner_public_key": "peer-a", "id": "alice001", "display_name": "Alice Zhang", "phone_number": "+1 555 0101"}
	}`))
	if err != nil {
		t.Fatalf("Apply(update) returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("update action = %q, want updated", result.Action)
	}

	resource, err := manager.Get(context.Background(), apitypes.ResourceKindContact, "peer-a:alice001")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	got, err := resource.AsContactResource()
	if err != nil {
		t.Fatalf("AsContactResource returned error: %v", err)
	}
	if got.Spec.OwnerPublicKey != "peer-a" || got.Spec.Id != "alice001" || socialTestString(got.Spec.DisplayName) != "Alice Zhang" {
		t.Fatalf("contact spec = %+v", got.Spec)
	}

	deleted, err := manager.Delete(context.Background(), apitypes.ResourceKindContact, "peer-a:alice001")
	if err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	deletedContact, err := deleted.AsContactResource()
	if err != nil {
		t.Fatalf("deleted AsContactResource returned error: %v", err)
	}
	if deletedContact.Metadata.Name != "peer-a:alice001" {
		t.Fatalf("deleted metadata.name = %q, want peer-a:alice001", deletedContact.Metadata.Name)
	}
	_, err = manager.Get(context.Background(), apitypes.ResourceKindContact, "peer-a:alice001")
	assertResourceError(t, err, 404, "RESOURCE_NOT_FOUND")
}

func TestApplyFriendGroupResourceCreatesUpdatesAndDeletes(t *testing.T) {
	manager := newSocialResourceManager(t)

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroup",
		"metadata": {"name": "family01"},
		"spec": {"owner_public_key": "peer-a", "name": "Family", "description": "voice room"}
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
		"metadata": {"name": "family01"},
		"spec": {"owner_public_key": "peer-a", "name": "Family", "description": "voice room"}
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
		"metadata": {"name": "family01"},
		"spec": {"owner_public_key": "peer-a", "name": "Family+", "description": "updated"}
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

	deleted, err := manager.Delete(context.Background(), apitypes.ResourceKindFriendGroup, "family01")
	if err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	deletedGroup, err := deleted.AsFriendGroupResource()
	if err != nil {
		t.Fatalf("deleted AsFriendGroupResource returned error: %v", err)
	}
	if deletedGroup.Metadata.Name != "family01" {
		t.Fatalf("deleted metadata.name = %q, want family01", deletedGroup.Metadata.Name)
	}
	_, err = manager.Get(context.Background(), apitypes.ResourceKindFriendGroup, "family01")
	assertResourceError(t, err, 404, "RESOURCE_NOT_FOUND")
}

func TestApplyFriendGroupMemberResourceCreatesUpdatesAndDeletes(t *testing.T) {
	manager := newSocialResourceManager(t)
	createFriendGroup(t, manager, "family01")

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupMember",
		"metadata": {"name": "family01:peer-a"},
		"spec": {"friend_group_id": "family01", "peer_public_key": "peer-a", "role": "member"}
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
		"metadata": {"name": "family01:peer-a"},
		"spec": {"friend_group_id": "family01", "peer_public_key": "peer-a", "role": "admin"}
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
		"metadata": {"name": "family01:peer-a"},
		"spec": {"friend_group_id": "family01", "peer_public_key": "peer-a", "role": "admin"}
	}`))
	if err != nil {
		t.Fatalf("Apply(unchanged) returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("unchanged action = %q, want unchanged", result.Action)
	}

	resource, err := manager.Get(context.Background(), apitypes.ResourceKindFriendGroupMember, "family01:peer-a")
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

	deleted, err := manager.Delete(context.Background(), apitypes.ResourceKindFriendGroupMember, "family01:peer-a")
	if err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	deletedMember, err := deleted.AsFriendGroupMemberResource()
	if err != nil {
		t.Fatalf("deleted AsFriendGroupMemberResource returned error: %v", err)
	}
	if deletedMember.Metadata.Name != "family01:peer-a" {
		t.Fatalf("deleted metadata.name = %q, want family01:peer-a", deletedMember.Metadata.Name)
	}
	_, err = manager.Get(context.Background(), apitypes.ResourceKindFriendGroupMember, "family01:peer-a")
	assertResourceError(t, err, 404, "RESOURCE_NOT_FOUND")
}

func TestApplyFriendGroupInviteTokenResourceCreatesUpdatesAndDeletes(t *testing.T) {
	manager := newSocialResourceManager(t)
	createFriendGroup(t, manager, "family01")
	expiresAt := time.Now().UTC().Add(time.Hour)

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupInviteToken",
		"metadata": {"name": "family01"},
		"spec": {"friend_group_id": "family01", "invite_token": "token-a", "expires_at": "`+expiresAt.Format(time.RFC3339Nano)+`"}
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
		"metadata": {"name": "family01"},
		"spec": {"friend_group_id": "family01", "invite_token": "token-b", "expires_at": "`+expiresAt.Add(time.Hour).Format(time.RFC3339Nano)+`"}
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
		"metadata": {"name": "family01"},
		"spec": {"friend_group_id": "family01", "invite_token": "token-b", "expires_at": "`+expiresAt.Add(time.Hour).Format(time.RFC3339Nano)+`"}
	}`))
	if err != nil {
		t.Fatalf("Apply(unchanged) returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("unchanged action = %q, want unchanged", result.Action)
	}

	resource, err := manager.Get(context.Background(), apitypes.ResourceKindFriendGroupInviteToken, "family01")
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

	deleted, err := manager.Delete(context.Background(), apitypes.ResourceKindFriendGroupInviteToken, "family01")
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
	_, err = manager.Get(context.Background(), apitypes.ResourceKindFriendGroupInviteToken, "family01")
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
		"kind": "Contact",
		"metadata": {"name": "peer-a:alice001"},
		"spec": {"owner_public_key": "peer-b", "id": "alice001", "display_name": "Alice"}
	}`))
	assertResourceError(t, err, 400, "INVALID_CONTACT_RESOURCE")

	_, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Contact",
		"metadata": {"name": "peer-a:alice001"},
		"spec": {"owner_public_key": "peer-a", "id": "bob00001", "display_name": "Bob"}
	}`))
	assertResourceError(t, err, 400, "INVALID_CONTACT_RESOURCE")

	_, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Contact",
		"metadata": {"name": "peer-a:alice001"},
		"spec": {"owner_public_key": "peer-a", "id": "alice001"}
	}`))
	assertResourceError(t, err, 400, "INVALID_CONTACT_RESOURCE")

	_, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroup",
		"metadata": {"name": "family01"},
		"spec": {"owner_public_key": "peer-a", "name": " "}
	}`))
	assertResourceError(t, err, 400, "INVALID_FRIEND_GROUP_RESOURCE")

	_, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupInviteToken",
		"metadata": {"name": "family01"},
		"spec": {"friend_group_id": "other001", "invite_token": "token", "expires_at": "2099-01-01T00:00:00Z"}
	}`))
	assertResourceError(t, err, 400, "INVALID_FRIEND_GROUP_INVITE_TOKEN_RESOURCE")

	_, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupInviteToken",
		"metadata": {"name": "family01"},
		"spec": {"friend_group_id": "family01", "invite_token": " ", "expires_at": "0001-01-01T00:00:00Z"}
	}`))
	assertResourceError(t, err, 400, "INVALID_FRIEND_GROUP_INVITE_TOKEN_RESOURCE")

	_, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupMember",
		"metadata": {"name": "family01:peer-b"},
		"spec": {"friend_group_id": "family01", "peer_public_key": "peer-a", "role": "member"}
	}`))
	assertResourceError(t, err, 400, "INVALID_FRIEND_GROUP_MEMBER_RESOURCE")

	_, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupMember",
		"metadata": {"name": "family01:peer-a"},
		"spec": {"friend_group_id": "family01", "peer_public_key": "peer-a", "role": "observer"}
	}`))
	assertResourceError(t, err, 400, "INVALID_FRIEND_GROUP_MEMBER_RESOURCE")
}

func TestSocialResourceValidationRejectsInvalidCustomIDs(t *testing.T) {
	manager := newSocialResourceManager(t)

	_, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Contact",
		"metadata": {"name": "peer-a:alice"},
		"spec": {"owner_public_key": "peer-a", "id": "alice", "display_name": "Alice"}
	}`))
	assertResourceError(t, err, 400, "INVALID_CONTACT_RESOURCE")

	_, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroup",
		"metadata": {"name": "family"},
		"spec": {"owner_public_key": "peer-a", "name": "Family"}
	}`))
	assertResourceError(t, err, 400, "INVALID_FRIEND_GROUP_RESOURCE")

	_, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupMember",
		"metadata": {"name": "family:peer-a"},
		"spec": {"friend_group_id": "family", "peer_public_key": "peer-a", "role": "member"}
	}`))
	assertResourceError(t, err, 400, "INVALID_FRIEND_GROUP_MEMBER_RESOURCE")

	_, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupInviteToken",
		"metadata": {"name": "family"},
		"spec": {"friend_group_id": "family", "invite_token": "token", "expires_at": "2099-01-01T00:00:00Z"}
	}`))
	assertResourceError(t, err, 400, "INVALID_FRIEND_GROUP_INVITE_TOKEN_RESOURCE")
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
		"kind": "Contact",
		"metadata": {"name": "peer-a:alice001"},
		"spec": {"owner_public_key": "peer-a", "id": "alice001", "display_name": "Alice"}
	}`))
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroup",
		"metadata": {"name": "family01"},
		"spec": {"owner_public_key": "peer-a", "name": "Family"}
	}`))
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Get(context.Background(), apitypes.ResourceKindFriendGroupMember, "family01:peer-a")
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupInviteToken",
		"metadata": {"name": "family01"},
		"spec": {"friend_group_id": "family01", "invite_token": "token", "expires_at": "2099-01-01T00:00:00Z"}
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
		"kind": "Contact",
		"metadata": {"name": "peer-a:alice001"},
		"spec": {"owner_public_key": "peer-a", "id": "alice001", "display_name": "Alice"}
	}`))
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroup",
		"metadata": {"name": "family01"},
		"spec": {"owner_public_key": "peer-a", "name": "Family"}
	}`))
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupMember",
		"metadata": {"name": "family01:peer-a"},
		"spec": {"friend_group_id": "family01", "peer_public_key": "peer-a", "role": "member"}
	}`))
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Get(context.Background(), apitypes.ResourceKindFriend, "peer-a:peer-b")
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Get(context.Background(), apitypes.ResourceKindContact, "peer-a:alice001")
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Get(context.Background(), apitypes.ResourceKindFriendGroup, "family01")
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Get(context.Background(), apitypes.ResourceKindFriendGroupInviteToken, "family01")
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Delete(context.Background(), apitypes.ResourceKindFriend, "peer-a:peer-b")
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Delete(context.Background(), apitypes.ResourceKindContact, "peer-a:alice001")
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Delete(context.Background(), apitypes.ResourceKindFriendGroup, "family01")
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Delete(context.Background(), apitypes.ResourceKindFriendGroupInviteToken, "family01")
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Delete(context.Background(), apitypes.ResourceKindFriendGroupMember, "family01:peer-a")
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

	contactResource, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Contact",
		"metadata": {"name": "peer-a:alice001"},
		"spec": {"owner_public_key": "peer-a", "id": "alice001", "display_name": "Alice"}
	}`))
	if err != nil {
		t.Fatalf("Put contact returned error: %v", err)
	}
	if got, err := contactResource.AsContactResource(); err != nil || got.Spec.Id != "alice001" {
		t.Fatalf("Put contact resource = %#v, err = %v", got, err)
	}

	group, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroup",
		"metadata": {"name": "family01"},
		"spec": {"owner_public_key": "peer-a", "name": "Family", "description": "room"}
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
		"metadata": {"name": "family01:peer-a"},
		"spec": {"friend_group_id": "family01", "peer_public_key": "peer-a", "role": "admin"}
	}`))
	if err != nil {
		t.Fatalf("Put member returned error: %v", err)
	}
	if got, err := member.AsFriendGroupMemberResource(); err != nil || got.Spec.Role != apitypes.FriendGroupMemberRoleAdmin {
		t.Fatalf("Put member resource = %#v, err = %v", got, err)
	}

	expiresAt := time.Now().UTC().Add(time.Hour)
	token, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "FriendGroupInviteToken",
		"metadata": {"name": "family01"},
		"spec": {"friend_group_id": "family01", "invite_token": "token-a", "expires_at": "`+expiresAt.Format(time.RFC3339Nano)+`"}
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
	if _, err := manager.Delete(context.Background(), apitypes.ResourceKindContact, "invalid"); err == nil {
		t.Fatal("Delete invalid contact id error = nil")
	}
	if _, err := manager.Delete(context.Background(), apitypes.ResourceKindFriendGroupInviteToken, "missing"); err == nil {
		t.Fatal("Delete missing invite token error = nil")
	}
	if _, err := manager.Delete(context.Background(), apitypes.ResourceKindFriendGroupMember, "family01"); err == nil {
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
		"spec": {"owner_public_key": "peer-a", "name": "`+name+`"}
	}`)); err != nil {
		t.Fatalf("create friend group %s: %v", name, err)
	}
}

func newSocialResourceManager(t *testing.T) *Manager {
	t.Helper()

	friendGroupRelationships := kv.NewMemory(nil)
	workspaces := socialWorkspaceService{}
	return New(Services{
		Contacts: &contact.Server{
			Store: kv.NewMemory(nil),
		},
		Friends: &friend.Server{
			InviteTokens:           kv.NewMemory(nil),
			Friends:                kv.NewMemory(nil),
			Workspaces:             workspaces,
			RuntimeProfileForOwner: socialRuntimeProfile,
		},
		FriendGroups: &friendgroup.Server{
			Groups:                 friendGroupRelationships,
			InviteTokens:           friendGroupRelationships,
			Members:                friendGroupRelationships,
			Belongs:                friendGroupRelationships,
			RelationshipStore:      friendGroupRelationships,
			Messages:               kv.NewMemory(nil),
			MessageAssets:          objectstore.Dir(t.TempDir()),
			Workspaces:             workspaces,
			RuntimeProfileForOwner: socialRuntimeProfile,
		},
	})
}

func socialRuntimeProfile(context.Context, string) (apitypes.RuntimeProfile, error) {
	return apitypes.RuntimeProfile{Spec: apitypes.RuntimeProfileSpec{
		Workflows: apitypes.RuntimeProfileWorkflows{
			System: apitypes.RuntimeProfileSystemWorkflows{
				FriendChatroom: "friend-chatroom",
				GroupChatroom:  "group-chatroom",
			},
		},
	}}, nil
}

type socialWorkspaceService struct{}

func (socialWorkspaceService) CreateSystemWorkspace(
	_ context.Context,
	body adminhttp.WorkspaceUpsert,
) (apitypes.Workspace, bool, error) {
	return apitypes.Workspace{
		Name:         body.Name,
		WorkflowName: body.WorkflowName,
		Parameters:   body.Parameters,
	}, true, nil
}

func (socialWorkspaceService) DeleteSystemWorkspace(
	_ context.Context,
	name string,
) (apitypes.Workspace, error) {
	return apitypes.Workspace{Name: name}, nil
}

func (socialWorkspaceService) RetireSystemWorkspace(
	_ context.Context,
	name string,
	_ apitypes.ChatRoomMode,
	_ string,
) (apitypes.Workspace, error) {
	return apitypes.Workspace{Name: name}, nil
}

func (socialWorkspaceService) GetRetiredSystemWorkspace(
	_ context.Context,
	_ string,
	_ apitypes.ChatRoomMode,
	_ string,
) (apitypes.Workspace, error) {
	return apitypes.Workspace{}, kv.ErrNotFound
}

func socialTestString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
