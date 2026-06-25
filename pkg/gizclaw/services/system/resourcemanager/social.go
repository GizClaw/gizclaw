package resourcemanager

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
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
	existing, exists, err := m.getFriend(ctx, item.Metadata.Name)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(friendSpec(existing), item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindFriend, item.Metadata.Name), nil
		}
	}
	if _, err := m.services.Friends.AdminCreateFriendResource(ctx, item.Spec.OwnerPublicKey, item.Spec.PeerPublicKey); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindFriend, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindFriend, item.Metadata.Name), nil
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
	existing, exists, err := m.getFriendGroup(ctx, item.Metadata.Name)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(friendGroupSpec(existing), item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindFriendGroup, item.Metadata.Name), nil
		}
	}
	if _, err := m.services.FriendGroups.AdminApplyFriendGroup(ctx, item.Metadata.Name, item.Spec.Name, item.Spec.Description); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindFriendGroup, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindFriendGroup, item.Metadata.Name), nil
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
	existing, exists, err := m.getFriendGroupInviteToken(ctx, item.Metadata.Name)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(friendGroupInviteTokenSpec(item.Metadata.Name, existing), item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindFriendGroupInviteToken, item.Metadata.Name), nil
		}
	}
	if _, err := m.services.FriendGroups.AdminPutFriendGroupInviteToken(ctx, item.Spec.FriendGroupId, item.Spec.InviteToken, item.Spec.ExpiresAt); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindFriendGroupInviteToken, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindFriendGroupInviteToken, item.Metadata.Name), nil
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
	existing, exists, err := m.getFriendGroupMember(ctx, item.Metadata.Name)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(friendGroupMemberSpec(existing), item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindFriendGroupMember, item.Metadata.Name), nil
		}
	}
	if _, err := m.services.FriendGroups.AdminPutFriendGroupMember(ctx, item.Spec.FriendGroupId, item.Spec.PeerPublicKey, rpcapi.FriendGroupMemberRole(item.Spec.Role)); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindFriendGroupMember, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindFriendGroupMember, item.Metadata.Name), nil
}

func (m *Manager) getFriend(ctx context.Context, name string) (adminservice.AdminFriendObject, bool, error) {
	owner, _, err := friendResourcePeers(name)
	if err != nil {
		return adminservice.AdminFriendObject{}, false, err
	}
	item, err := m.services.Friends.AdminGetFriend(ctx, owner, name)
	if errors.Is(err, kv.ErrNotFound) {
		return adminservice.AdminFriendObject{}, false, nil
	}
	if err != nil {
		return adminservice.AdminFriendObject{}, false, err
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

func resourceFromFriend(item adminservice.AdminFriendObject) (apitypes.Resource, error) {
	return marshalResource(apitypes.FriendResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FriendResourceKindFriend,
		Metadata:   apitypes.ResourceMetadata{Name: socialutil.RelationID(item.OwnerPublicKey, item.PeerPublicKey)},
		Spec:       friendSpec(item),
	})
}

func resourceFromFriendGroup(item rpcapi.FriendGroupObject) (apitypes.Resource, error) {
	return marshalResource(apitypes.FriendGroupResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FriendGroupResourceKindFriendGroup,
		Metadata:   apitypes.ResourceMetadata{Name: socialutil.StringValue(item.Id)},
		Spec:       friendGroupSpec(item),
	})
}

func resourceFromFriendGroupInviteToken(friendGroupID string, item rpcapi.FriendGroupInviteTokenGetResponse) (apitypes.Resource, error) {
	return marshalResource(apitypes.FriendGroupInviteTokenResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FriendGroupInviteTokenResourceKindFriendGroupInviteToken,
		Metadata:   apitypes.ResourceMetadata{Name: friendGroupID},
		Spec:       friendGroupInviteTokenSpec(friendGroupID, item),
	})
}

func resourceFromFriendGroupMember(item rpcapi.FriendGroupMemberObject) (apitypes.Resource, error) {
	spec := friendGroupMemberSpec(item)
	return marshalResource(apitypes.FriendGroupMemberResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FriendGroupMemberResourceKindFriendGroupMember,
		Metadata:   apitypes.ResourceMetadata{Name: friendGroupMemberResourceName(spec.FriendGroupId, spec.PeerPublicKey)},
		Spec:       spec,
	})
}

func friendSpec(item adminservice.AdminFriendObject) apitypes.FriendSpec {
	return apitypes.FriendSpec{
		OwnerPublicKey: item.OwnerPublicKey,
		PeerPublicKey:  item.PeerPublicKey,
	}
}

func friendGroupSpec(item rpcapi.FriendGroupObject) apitypes.FriendGroupSpec {
	return apitypes.FriendGroupSpec{
		Name:        socialutil.StringValue(item.Name),
		Description: socialutil.OptionalString(strings.TrimSpace(socialutil.StringValue(item.Description))),
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

func friendGroupMemberSpec(item rpcapi.FriendGroupMemberObject) apitypes.FriendGroupMemberSpec {
	return apitypes.FriendGroupMemberSpec{
		FriendGroupId: socialutil.StringValue(item.FriendGroupId),
		PeerPublicKey: socialutil.StringValue(item.PeerPublicKey),
		Role:          apitypes.FriendGroupMemberRole(socialutil.GroupRole(item)),
	}
}

func validateFriendResource(item apitypes.FriendResource) error {
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return err
	}
	owner, peer, err := friendResourcePeers(item.Metadata.Name)
	if err != nil {
		return err
	}
	if item.Spec.OwnerPublicKey != owner || item.Spec.PeerPublicKey != peer {
		return applyError(400, "INVALID_FRIEND_RESOURCE", "metadata.name must match canonical owner_public_key:peer_public_key order")
	}
	return nil
}

func validateFriendGroupResource(item apitypes.FriendGroupResource) error {
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return err
	}
	if strings.TrimSpace(item.Spec.Name) == "" {
		return applyError(400, "INVALID_FRIEND_GROUP_RESOURCE", "spec.name is required")
	}
	return nil
}

func validateFriendGroupInviteTokenResource(item apitypes.FriendGroupInviteTokenResource) error {
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return err
	}
	if item.Spec.FriendGroupId != item.Metadata.Name {
		return applyError(400, "INVALID_FRIEND_GROUP_INVITE_TOKEN_RESOURCE", "metadata.name must match spec.friend_group_id")
	}
	if strings.TrimSpace(item.Spec.InviteToken) == "" || item.Spec.ExpiresAt.IsZero() {
		return applyError(400, "INVALID_FRIEND_GROUP_INVITE_TOKEN_RESOURCE", "active invite_token and expires_at are required")
	}
	return nil
}

func validateFriendGroupMemberResource(item apitypes.FriendGroupMemberResource) error {
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return err
	}
	if strings.TrimSpace(item.Spec.FriendGroupId) == "" || strings.TrimSpace(item.Spec.PeerPublicKey) == "" {
		return applyError(400, "INVALID_FRIEND_GROUP_MEMBER_RESOURCE", "spec.friend_group_id and spec.peer_public_key are required")
	}
	if !item.Spec.Role.Valid() {
		return applyError(400, "INVALID_FRIEND_GROUP_MEMBER_RESOURCE", "spec.role is invalid")
	}
	expected := friendGroupMemberResourceName(item.Spec.FriendGroupId, item.Spec.PeerPublicKey)
	if item.Metadata.Name != expected {
		return applyError(400, "INVALID_FRIEND_GROUP_MEMBER_RESOURCE", fmt.Sprintf("metadata.name must be %q", expected))
	}
	return nil
}

func friendResourcePeers(name string) (string, string, error) {
	left, right, ok := strings.Cut(strings.TrimSpace(name), ":")
	if !ok || strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return "", "", applyError(400, "INVALID_FRIEND_RESOURCE", "metadata.name must be owner_public_key:peer_public_key")
	}
	if socialutil.RelationID(left, right) != name {
		return "", "", applyError(400, "INVALID_FRIEND_RESOURCE", "metadata.name must use sorted relation id order")
	}
	return left, right, nil
}

func friendGroupMemberResourceName(friendGroupID, peerID string) string {
	return strings.TrimSpace(friendGroupID) + ":" + strings.TrimSpace(peerID)
}

func friendGroupMemberResourceParts(name string) (string, string, error) {
	name = strings.TrimSpace(name)
	index := strings.LastIndex(name, ":")
	if index <= 0 || index == len(name)-1 {
		return "", "", applyError(400, "INVALID_FRIEND_GROUP_MEMBER_RESOURCE", "metadata.name must be friend_group_id:peer_public_key")
	}
	return name[:index], name[index+1:], nil
}
