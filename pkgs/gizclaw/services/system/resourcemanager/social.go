package resourcemanager

import (
	"context"
	"errors"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func (m *Manager) applyFriend(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.Friends == nil {
		return apitypes.ApplyResult{}, missingService("friends")
	}
	item, err := resource.AsFriendResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_FRIEND_RESOURCE", err.Error())
	}
	if err := validateFriendResource(item); err != nil {
		return apitypes.ApplyResult{}, err
	}
	id := item.Metadata.Id
	existing, exists, err := m.getFriend(ctx, id)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if !exists {
		created, err := m.services.Friends.AdminCreateFriendResource(ctx, id, item.Spec.OwnerPublicKey, item.Spec.PeerPublicKey)
		if err != nil {
			if errors.Is(err, socialutil.ErrResourceAlreadyExists) {
				return apitypes.ApplyResult{}, applyError(409, "FRIEND_ALREADY_EXISTS", err.Error())
			}
			return apitypes.ApplyResult{}, err
		}
		return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindFriend, created.Id), nil
	}
	if !equalFriendSpec(friendSpec(existing), item.Spec) {
		return apitypes.ApplyResult{}, applyError(409, "IMMUTABLE_FRIEND", "Friend relationships cannot be updated")
	}
	return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindFriend, id), nil
}

func (m *Manager) applyContact(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.Contacts == nil {
		return apitypes.ApplyResult{}, missingService("contacts")
	}
	item, err := resource.AsContactResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_CONTACT_RESOURCE", err.Error())
	}
	if err := validateContactResource(item); err != nil {
		return apitypes.ApplyResult{}, err
	}
	id := item.Metadata.Id
	existing, exists, err := m.getContact(ctx, id)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if !exists {
		created, err := m.services.Contacts.AdminCreateContact(ctx, adminhttp.AdminContactCreateRequest{Id: id, OwnerPublicKey: item.Spec.OwnerPublicKey, Name: item.Spec.Name, DisplayName: item.Spec.DisplayName, PhoneNumber: item.Spec.PhoneNumber})
		if err != nil {
			if errors.Is(err, socialutil.ErrResourceAlreadyExists) {
				return apitypes.ApplyResult{}, applyError(409, "CONTACT_ALREADY_EXISTS", err.Error())
			}
			return apitypes.ApplyResult{}, err
		}
		return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindContact, created.Id), nil
	}
	if existing.Name != item.Spec.Name || existing.OwnerPublicKey != item.Spec.OwnerPublicKey {
		return apitypes.ApplyResult{}, applyError(409, "IMMUTABLE_CONTACT_IDENTITY", "Contact name and owner are immutable")
	}
	desired := item.Spec
	same, err := semanticEqual(contactSpec(existing), desired)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if same {
		return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindContact, id), nil
	}
	if _, err := m.services.Contacts.AdminPutContactByID(ctx, id, adminhttp.AdminContactPutRequest{Id: id, DisplayName: item.Spec.DisplayName, PhoneNumber: item.Spec.PhoneNumber}); err != nil {
		return apitypes.ApplyResult{}, err
	}
	return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindContact, id), nil
}

func (m *Manager) applyFriendGroup(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.FriendGroups == nil {
		return apitypes.ApplyResult{}, missingService("friend groups")
	}
	item, err := resource.AsFriendGroupResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_FRIEND_GROUP_RESOURCE", err.Error())
	}
	if err := validateFriendGroupResource(item); err != nil {
		return apitypes.ApplyResult{}, err
	}
	id := item.Metadata.Id
	existing, exists, err := m.getFriendGroup(ctx, id)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if !exists {
		created, err := m.services.FriendGroups.AdminCreateFriendGroup(ctx, id, item.Spec.OwnerPublicKey, item.Spec.Name, item.Spec.DisplayName, item.Spec.Description)
		if err != nil {
			if errors.Is(err, socialutil.ErrResourceAlreadyExists) {
				return apitypes.ApplyResult{}, applyError(409, "FRIEND_GROUP_ALREADY_EXISTS", err.Error())
			}
			return apitypes.ApplyResult{}, err
		}
		return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindFriendGroup, created.Id), nil
	}
	if existing.Name != item.Spec.Name || socialutil.StringValue(existing.CreatedByPeerPublicKey) != item.Spec.OwnerPublicKey {
		return apitypes.ApplyResult{}, applyError(409, "IMMUTABLE_FRIEND_GROUP_IDENTITY", "FriendGroup name and owner are immutable")
	}
	if exists {
		same, err := semanticEqual(friendGroupSpec(existing), item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindFriendGroup, id), nil
		}
	}
	if _, err := m.services.FriendGroups.AdminPutFriendGroup(ctx, id, item.Spec.DisplayName, item.Spec.Description); err != nil {
		return apitypes.ApplyResult{}, err
	}
	return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindFriendGroup, id), nil
}

func (m *Manager) applyFriendGroupInviteToken(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.FriendGroups == nil {
		return apitypes.ApplyResult{}, missingService("friend groups")
	}
	item, err := resource.AsFriendGroupInviteTokenResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_FRIEND_GROUP_INVITE_TOKEN_RESOURCE", err.Error())
	}
	if err := validateFriendGroupInviteTokenResource(item); err != nil {
		return apitypes.ApplyResult{}, err
	}
	id := item.Metadata.Id
	existing, exists, err := m.getFriendGroupInviteToken(ctx, id)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if !exists {
		if _, err := m.services.FriendGroups.AdminPutFriendGroupInviteToken(ctx, item.Spec.FriendGroupId, item.Spec.InviteToken, item.Spec.ExpiresAt); err != nil {
			return apitypes.ApplyResult{}, err
		}
		return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindFriendGroupInviteToken, id), nil
	}
	if id != item.Spec.FriendGroupId {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_FRIEND_GROUP_INVITE_TOKEN_RESOURCE", "metadata.id must match spec.friend_group_id")
	}
	if exists {
		same, err := semanticEqual(friendGroupInviteTokenSpec(id, existing), item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindFriendGroupInviteToken, id), nil
		}
	}
	if _, err := m.services.FriendGroups.AdminPutFriendGroupInviteToken(ctx, id, item.Spec.InviteToken, item.Spec.ExpiresAt); err != nil {
		return apitypes.ApplyResult{}, err
	}
	return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindFriendGroupInviteToken, id), nil
}

func (m *Manager) applyFriendGroupMember(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.FriendGroups == nil {
		return apitypes.ApplyResult{}, missingService("friend groups")
	}
	item, err := resource.AsFriendGroupMemberResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_FRIEND_GROUP_MEMBER_RESOURCE", err.Error())
	}
	if err := validateFriendGroupMemberResource(item); err != nil {
		return apitypes.ApplyResult{}, err
	}
	id := item.Metadata.Id
	canonicalID := friendGroupMemberResourceName(item.Spec.FriendGroupId, item.Spec.PeerPublicKey)
	if id != canonicalID {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_FRIEND_GROUP_MEMBER_RESOURCE", "metadata.id must match spec friend group and peer")
	}
	existing, exists, err := m.getFriendGroupMember(ctx, id)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if !exists {
		if _, err := m.services.FriendGroups.AdminCreateFriendGroupMember(ctx, item.Spec.FriendGroupId, item.Spec.PeerPublicKey, item.Spec.Name, rpcapi.FriendGroupMemberRole(item.Spec.Role)); err != nil {
			if errors.Is(err, friendgroup.ErrFriendGroupMemberAlreadyExists) {
				return apitypes.ApplyResult{}, applyError(409, "FRIEND_GROUP_MEMBER_ALREADY_EXISTS", err.Error())
			}
			return apitypes.ApplyResult{}, err
		}
		return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindFriendGroupMember, id), nil
	}
	if socialutil.StringValue(existing.FriendGroupName) != item.Spec.Name {
		return apitypes.ApplyResult{}, applyError(409, "IMMUTABLE_FRIEND_GROUP_MEMBER_NAME", "FriendGroupMember name is immutable")
	}
	if exists {
		same, err := semanticEqual(friendGroupMemberSpec(item.Spec.FriendGroupId, existing), item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindFriendGroupMember, id), nil
		}
	}
	if _, err := m.services.FriendGroups.AdminPutFriendGroupMember(ctx, item.Spec.FriendGroupId, item.Spec.PeerPublicKey, item.Spec.Name, rpcapi.FriendGroupMemberRole(item.Spec.Role)); err != nil {
		return apitypes.ApplyResult{}, err
	}
	return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindFriendGroupMember, id), nil
}

func (m *Manager) getFriend(ctx context.Context, name string) (adminhttp.AdminFriendObject, bool, error) {
	owner, _, err := friendResourcePeers(name)
	if err != nil {
		return adminhttp.AdminFriendObject{}, false, err
	}
	item, err := m.services.Friends.AdminGetFriend(ctx, owner, name)
	if errors.Is(err, kv.ErrNotFound) {
		return adminhttp.AdminFriendObject{}, false, nil
	}
	if err != nil {
		return adminhttp.AdminFriendObject{}, false, err
	}
	return item, true, nil
}

func (m *Manager) getContact(ctx context.Context, id string) (adminhttp.AdminContactObject, bool, error) {
	item, err := m.services.Contacts.AdminGetContactByID(ctx, id)
	if errors.Is(err, kv.ErrNotFound) {
		return adminhttp.AdminContactObject{}, false, nil
	}
	if err != nil {
		return adminhttp.AdminContactObject{}, false, err
	}
	return item, true, nil
}

func (m *Manager) getFriendGroup(ctx context.Context, name string) (rpcapi.FriendGroupObject, bool, error) {
	item, err := m.services.FriendGroups.AdminGetFriendGroup(ctx, name)
	if errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendGroupObject{}, false, nil
	}
	if err != nil {
		return rpcapi.FriendGroupObject{}, false, err
	}
	return item, true, nil
}

func (m *Manager) getFriendGroupInviteToken(ctx context.Context, name string) (rpcapi.FriendGroupInviteTokenGetResponse, bool, error) {
	item, err := m.services.FriendGroups.AdminGetFriendGroupInviteToken(ctx, name)
	if errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendGroupInviteTokenGetResponse{}, false, nil
	}
	if err != nil {
		return rpcapi.FriendGroupInviteTokenGetResponse{}, false, err
	}
	if item.InviteToken == nil || item.ExpiresAt == nil {
		return rpcapi.FriendGroupInviteTokenGetResponse{}, false, nil
	}
	return item, true, nil
}

func (m *Manager) getFriendGroupMember(ctx context.Context, name string) (rpcapi.FriendGroupMemberObject, bool, error) {
	friendGroupID, peerID, err := friendGroupMemberResourceParts(name)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, false, err
	}
	item, err := m.services.FriendGroups.AdminGetFriendGroupMember(ctx, friendGroupID, peerID)
	if errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendGroupMemberObject{}, false, nil
	}
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, false, err
	}
	return item, true, nil
}

func resourceFromFriend(item adminhttp.AdminFriendObject) (apitypes.Resource, error) {
	return marshalResource(apitypes.FriendResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FriendResourceKindFriend,
		Metadata:   apitypes.ResourceMetadata{Id: item.Id},
		Spec:       friendSpec(item),
	})
}

func resourceFromContact(item adminhttp.AdminContactObject) (apitypes.Resource, error) {
	return marshalResource(apitypes.ContactResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.ContactResourceKindContact,
		Metadata:   apitypes.ResourceMetadata{Id: item.Id},
		Spec:       contactSpec(item),
	})
}

func resourceFromFriendGroup(friendGroupID string, item rpcapi.FriendGroupObject) (apitypes.Resource, error) {
	return marshalResource(apitypes.FriendGroupResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FriendGroupResourceKindFriendGroup,
		Metadata:   apitypes.ResourceMetadata{Id: friendGroupID},
		Spec:       friendGroupSpec(item),
	})
}

func resourceFromFriendGroupInviteToken(friendGroupID string, item rpcapi.FriendGroupInviteTokenGetResponse) (apitypes.Resource, error) {
	return marshalResource(apitypes.FriendGroupInviteTokenResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FriendGroupInviteTokenResourceKindFriendGroupInviteToken,
		Metadata:   apitypes.ResourceMetadata{Id: friendGroupID},
		Spec:       friendGroupInviteTokenSpec(friendGroupID, item),
	})
}

func resourceFromFriendGroupMember(friendGroupID string, item rpcapi.FriendGroupMemberObject) (apitypes.Resource, error) {
	spec := friendGroupMemberSpec(friendGroupID, item)
	id := friendGroupMemberResourceName(spec.FriendGroupId, spec.PeerPublicKey)
	return marshalResource(apitypes.FriendGroupMemberResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FriendGroupMemberResourceKindFriendGroupMember,
		Metadata:   apitypes.ResourceMetadata{Id: id},
		Spec:       spec,
	})
}

func friendSpec(item adminhttp.AdminFriendObject) apitypes.FriendSpec {
	return apitypes.FriendSpec{
		OwnerPublicKey: item.OwnerPublicKey,
		PeerPublicKey:  item.PeerPublicKey,
	}
}

func equalFriendSpec(left, right apitypes.FriendSpec) bool {
	return left.OwnerPublicKey == right.OwnerPublicKey && left.PeerPublicKey == right.PeerPublicKey
}

func contactSpec(item adminhttp.AdminContactObject) apitypes.ContactSpec {
	return apitypes.ContactSpec{
		Name:           item.Name,
		OwnerPublicKey: item.OwnerPublicKey,
		DisplayName:    item.DisplayName,
		PhoneNumber:    item.PhoneNumber,
	}
}

func friendGroupSpec(item rpcapi.FriendGroupObject) apitypes.FriendGroupSpec {
	return apitypes.FriendGroupSpec{
		Name:           item.Name,
		OwnerPublicKey: socialutil.StringValue(item.CreatedByPeerPublicKey),
		DisplayName:    item.DisplayName,
		Description:    socialutil.OptionalString(strings.TrimSpace(socialutil.StringValue(item.Description))),
	}
}

func friendGroupInviteTokenSpec(friendGroupID string, item rpcapi.FriendGroupInviteTokenGetResponse) apitypes.FriendGroupInviteTokenSpec {
	spec := apitypes.FriendGroupInviteTokenSpec{FriendGroupId: friendGroupID}
	if item.InviteToken != nil {
		spec.InviteToken = *item.InviteToken
	}
	if item.ExpiresAt != nil {
		spec.ExpiresAt = item.ExpiresAt.UTC()
	}
	return spec
}

func friendGroupMemberSpec(friendGroupID string, item rpcapi.FriendGroupMemberObject) apitypes.FriendGroupMemberSpec {
	return apitypes.FriendGroupMemberSpec{
		FriendGroupId: friendGroupID,
		Name:          socialutil.StringValue(item.FriendGroupName),
		PeerPublicKey: socialutil.StringValue(item.PeerPublicKey),
		Role:          apitypes.FriendGroupMemberRole(socialutil.GroupRole(item)),
	}
}

func validateFriendResource(item apitypes.FriendResource) error {
	if err := validateResourceHeader(item.ApiVersion, item.Metadata); err != nil {
		return err
	}
	owner, peer, err := friendResourcePeers(item.Metadata.Id)
	if err != nil {
		return err
	}
	if item.Spec.OwnerPublicKey != owner || item.Spec.PeerPublicKey != peer {
		return applyError(400, "INVALID_FRIEND_RESOURCE", "metadata.id must match the deterministic relation ID")
	}
	return nil
}

func validateContactResource(item apitypes.ContactResource) error {
	if err := validateResourceHeader(item.ApiVersion, item.Metadata); err != nil {
		return err
	}
	if err := customid.ValidateField("spec.name", item.Spec.Name); err != nil {
		return applyError(400, "INVALID_CONTACT_RESOURCE", err.Error())
	}
	if strings.TrimSpace(item.Spec.OwnerPublicKey) == "" {
		return applyError(400, "INVALID_CONTACT_RESOURCE", "spec.owner_public_key is required")
	}
	if strings.TrimSpace(socialutil.StringValue(item.Spec.DisplayName)) == "" && strings.TrimSpace(socialutil.StringValue(item.Spec.PhoneNumber)) == "" {
		return applyError(400, "INVALID_CONTACT_RESOURCE", "spec.display_name or spec.phone_number is required")
	}
	return nil
}

func validateFriendGroupResource(item apitypes.FriendGroupResource) error {
	if err := validateResourceHeader(item.ApiVersion, item.Metadata); err != nil {
		return err
	}
	if err := customid.ValidateFriendGroupID(item.Metadata.Id); err != nil {
		return applyError(400, "INVALID_FRIEND_GROUP_RESOURCE", err.Error())
	}
	if err := customid.ValidateMembershipName(item.Metadata.Id, item.Spec.OwnerPublicKey); err != nil {
		return applyError(400, "INVALID_FRIEND_GROUP_RESOURCE", err.Error())
	}
	if err := customid.ValidateField("spec.name", item.Spec.Name); err != nil {
		return applyError(400, "INVALID_FRIEND_GROUP_RESOURCE", err.Error())
	}
	if strings.TrimSpace(item.Spec.OwnerPublicKey) == "" {
		return applyError(400, "INVALID_FRIEND_GROUP_RESOURCE", "spec.owner_public_key is required")
	}
	return nil
}

func validateFriendGroupInviteTokenResource(item apitypes.FriendGroupInviteTokenResource) error {
	if err := validateResourceHeader(item.ApiVersion, item.Metadata); err != nil {
		return err
	}
	if strings.TrimSpace(item.Spec.FriendGroupId) == "" {
		return applyError(400, "INVALID_FRIEND_GROUP_INVITE_TOKEN_RESOURCE", "spec.friend_group_id is required")
	}
	if item.Spec.FriendGroupId != item.Metadata.Id {
		return applyError(400, "INVALID_FRIEND_GROUP_INVITE_TOKEN_RESOURCE", "metadata.id must match spec.friend_group_id")
	}
	if strings.TrimSpace(item.Spec.InviteToken) == "" || item.Spec.ExpiresAt.IsZero() {
		return applyError(400, "INVALID_FRIEND_GROUP_INVITE_TOKEN_RESOURCE", "active invite_token and expires_at are required")
	}
	return nil
}

func validateFriendGroupMemberResource(item apitypes.FriendGroupMemberResource) error {
	if err := validateResourceHeader(item.ApiVersion, item.Metadata); err != nil {
		return err
	}
	if err := customid.ValidateField("spec.name", item.Spec.Name); err != nil {
		return applyError(400, "INVALID_FRIEND_GROUP_MEMBER_RESOURCE", err.Error())
	}
	if strings.TrimSpace(item.Spec.FriendGroupId) == "" {
		return applyError(400, "INVALID_FRIEND_GROUP_MEMBER_RESOURCE", "spec.friend_group_id is required")
	}
	if strings.TrimSpace(item.Spec.PeerPublicKey) == "" {
		return applyError(400, "INVALID_FRIEND_GROUP_MEMBER_RESOURCE", "spec.peer_public_key is required")
	}
	if err := customid.ValidateMembershipName(item.Spec.FriendGroupId, item.Spec.PeerPublicKey); err != nil {
		return applyError(400, "INVALID_FRIEND_GROUP_MEMBER_RESOURCE", err.Error())
	}
	if !item.Spec.Role.Valid() {
		return applyError(400, "INVALID_FRIEND_GROUP_MEMBER_RESOURCE", "spec.role is invalid")
	}
	return nil
}

func friendResourcePeers(name string) (string, string, error) {
	left, right, ok := strings.Cut(strings.TrimSpace(name), ":")
	if !ok || strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return "", "", applyError(400, "INVALID_FRIEND_RESOURCE", "metadata.id must be owner_public_key:peer_public_key")
	}
	if socialutil.RelationID(left, right) != name {
		return "", "", applyError(400, "INVALID_FRIEND_RESOURCE", "metadata.id must use sorted relation id order")
	}
	return left, right, nil
}

func friendGroupMemberResourceName(friendGroupID, peerID string) string {
	return customid.MembershipName(friendGroupID, peerID)
}

func friendGroupMemberResourceParts(name string) (string, string, error) {
	friendGroupID, peerID, err := customid.SplitMembershipName(name)
	if err != nil {
		return "", "", applyError(400, "INVALID_FRIEND_GROUP_MEMBER_RESOURCE", err.Error())
	}
	return friendGroupID, peerID, nil
}
