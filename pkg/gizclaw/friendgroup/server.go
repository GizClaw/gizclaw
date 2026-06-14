package friendgroup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/acl"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
	"github.com/GizClaw/gizclaw-go/pkg/store/objectstore"
)

type ACL interface {
	PutRole(context.Context, string, apitypes.ACLPermissionList) (apitypes.ACLRole, error)
	PutPolicyBinding(context.Context, string, float64, apitypes.ACLPolicy) (apitypes.ACLPolicyBinding, error)
	DeletePolicyBinding(context.Context, string) (apitypes.ACLPolicyBinding, error)
	Authorize(context.Context, acl.AuthorizeRequest) error
}

type Server struct {
	Groups        kv.Store
	Members       kv.Store
	Messages      kv.Store
	MessageAssets objectstore.ObjectStore
	ACL           ACL

	MessageDefaultTTL    time.Duration
	MessageMaxTTL        time.Duration
	MessageMaxAudioBytes int64

	Now   func() time.Time
	NewID func() string
}

func (s *Server) CreateFriendGroup(ctx context.Context, owner string, req rpcapi.FriendGroupCreateRequest) (rpcapi.FriendGroupObject, error) {
	friendGroups, members, err := s.stores()
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	owner = strings.TrimSpace(owner)
	name := strings.TrimSpace(req.Name)
	if owner == "" || name == "" {
		return rpcapi.FriendGroupObject{}, errors.New("social: friend group owner and name are required")
	}
	now := s.now()
	id := s.newID()
	group := rpcapi.FriendGroupObject{
		Id:              &id,
		Name:            &name,
		Description:     socialutil.OptionalString(strings.TrimSpace(socialutil.StringValue(req.Description))),
		CreatedByPeerId: &owner,
		CreatedAt:       &now,
		UpdatedAt:       &now,
	}
	if err := socialutil.WriteJSON(ctx, friendGroups, socialutil.GroupKey(id), group); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	role := rpcapi.FriendGroupMemberRoleOwner
	member := rpcapi.FriendGroupMemberObject{Id: &owner, FriendGroupId: &id, PeerId: &owner, Role: &role, CreatedAt: &now, UpdatedAt: &now}
	if err := socialutil.WriteJSON(ctx, members, socialutil.GroupMemberKey(id, owner), member); err != nil {
		_ = friendGroups.Delete(ctx, socialutil.GroupKey(id))
		return rpcapi.FriendGroupObject{}, err
	}
	if err := s.upsertACLBinding(ctx, id, owner, role); err != nil {
		_ = members.Delete(ctx, socialutil.GroupMemberKey(id, owner))
		_ = friendGroups.Delete(ctx, socialutil.GroupKey(id))
		return rpcapi.FriendGroupObject{}, err
	}
	return group, nil
}

func (s *Server) GetFriendGroup(ctx context.Context, owner string, req rpcapi.FriendGroupGetRequest) (rpcapi.FriendGroupObject, error) {
	store, err := s.groupsStore()
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	friendGroupID := strings.TrimSpace(req.Id)
	if friendGroupID == "" {
		return rpcapi.FriendGroupObject{}, errors.New("social: group id is required")
	}
	if err := s.requireRead(ctx, owner, friendGroupID); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	return socialutil.ReadJSONValue[rpcapi.FriendGroupObject](ctx, store, socialutil.GroupKey(friendGroupID))
}

func (s *Server) ListFriendGroups(ctx context.Context, owner string, req rpcapi.FriendGroupListRequest) (rpcapi.FriendGroupListResponse, error) {
	store, err := s.groupsStore()
	if err != nil {
		return rpcapi.FriendGroupListResponse{}, err
	}
	items := make([]rpcapi.FriendGroupObject, 0)
	for entry, err := range store.List(ctx, socialutil.GroupsRoot) {
		if err != nil {
			return rpcapi.FriendGroupListResponse{}, err
		}
		var item rpcapi.FriendGroupObject
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return rpcapi.FriendGroupListResponse{}, err
		}
		if _, err := s.groupMember(ctx, socialutil.StringValue(item.Id), owner); err == nil {
			items = append(items, item)
		} else if !errors.Is(err, kv.ErrNotFound) {
			return rpcapi.FriendGroupListResponse{}, err
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return socialutil.CompareByCreatedAtAsc(socialutil.TimeValue(items[i].CreatedAt), socialutil.StringValue(items[i].Id), socialutil.TimeValue(items[j].CreatedAt), socialutil.StringValue(items[j].Id))
	})
	page := socialutil.PageItems(items, socialutil.StringValue(req.Cursor), socialutil.IntValue(req.Limit), func(item rpcapi.FriendGroupObject) string {
		return socialutil.StringValue(item.Id)
	})
	return rpcapi.FriendGroupListResponse{Items: page.Items, HasNext: page.HasNext, NextCursor: page.NextCursor}, nil
}

func (s *Server) PutFriendGroup(ctx context.Context, owner string, req rpcapi.FriendGroupPutRequest) (rpcapi.FriendGroupObject, error) {
	store, err := s.groupsStore()
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	friendGroupID := strings.TrimSpace(req.Id)
	if friendGroupID == "" {
		return rpcapi.FriendGroupObject{}, errors.New("social: group id is required")
	}
	if err := s.requireRole(ctx, owner, friendGroupID, rpcapi.FriendGroupMemberRoleOwner); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	group, err := socialutil.ReadJSONValue[rpcapi.FriendGroupObject](ctx, store, socialutil.GroupKey(friendGroupID))
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if req.Name != nil {
		v := strings.TrimSpace(*req.Name)
		if v == "" {
			return rpcapi.FriendGroupObject{}, errors.New("social: friend group name is required")
		}
		group.Name = &v
	}
	if req.Description != nil {
		group.Description = socialutil.OptionalString(strings.TrimSpace(*req.Description))
	}
	now := s.now()
	group.UpdatedAt = &now
	return group, socialutil.WriteJSON(ctx, store, socialutil.GroupKey(friendGroupID), group)
}

func (s *Server) DeleteFriendGroup(ctx context.Context, owner string, req rpcapi.FriendGroupDeleteRequest) (rpcapi.FriendGroupObject, error) {
	friendGroups, err := s.groupsStore()
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	friendGroupID := strings.TrimSpace(req.Id)
	if friendGroupID == "" {
		return rpcapi.FriendGroupObject{}, errors.New("social: group id is required")
	}
	if err := s.requireRole(ctx, owner, friendGroupID, rpcapi.FriendGroupMemberRoleOwner); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	group, err := socialutil.ReadJSONValue[rpcapi.FriendGroupObject](ctx, friendGroups, socialutil.GroupKey(friendGroupID))
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	var members []rpcapi.FriendGroupMemberObject
	if s.ACL != nil {
		members, err = s.listAllMembers(ctx, friendGroupID)
		if err != nil {
			return rpcapi.FriendGroupObject{}, err
		}
	}
	if s.MessageAssets != nil {
		if err := s.MessageAssets.DeletePrefix(socialutil.EscapeStoreSegment(friendGroupID)); err != nil {
			return rpcapi.FriendGroupObject{}, err
		}
	}
	if s.Members != nil {
		if err := socialutil.DeletePrefix(ctx, s.Members, append(socialutil.GroupMembersRoot, socialutil.EscapeStoreSegment(friendGroupID))); err != nil {
			return rpcapi.FriendGroupObject{}, err
		}
	}
	if s.Messages != nil {
		if err := socialutil.DeletePrefix(ctx, s.Messages, append(socialutil.GroupMessagesRoot, socialutil.EscapeStoreSegment(friendGroupID))); err != nil {
			return rpcapi.FriendGroupObject{}, err
		}
	}
	if err := s.deleteACLBindings(ctx, friendGroupID, members); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if err := friendGroups.Delete(ctx, socialutil.GroupKey(friendGroupID)); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	return group, nil
}

func (s *Server) AddFriendGroupMember(ctx context.Context, owner string, req rpcapi.FriendGroupMemberAddRequest) (rpcapi.FriendGroupMemberObject, error) {
	req.FriendGroupId = strings.TrimSpace(req.FriendGroupId)
	req.PeerId = strings.TrimSpace(req.PeerId)
	store, err := s.membersStore()
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if req.Role == rpcapi.FriendGroupMemberMutableRole("admin") {
		if err := s.requireRole(ctx, owner, req.FriendGroupId, rpcapi.FriendGroupMemberRoleOwner); err != nil {
			return rpcapi.FriendGroupMemberObject{}, err
		}
	} else if err := s.requireAdmin(ctx, owner, req.FriendGroupId); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	current, currentErr := s.groupMember(ctx, req.FriendGroupId, req.PeerId)
	if currentErr != nil && !errors.Is(currentErr, kv.ErrNotFound) {
		return rpcapi.FriendGroupMemberObject{}, currentErr
	}
	member, err := s.writeMember(ctx, req.FriendGroupId, req.PeerId, rpcapi.FriendGroupMemberRole(req.Role))
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := s.upsertACLBinding(ctx, req.FriendGroupId, req.PeerId, socialutil.GroupRole(member)); err != nil {
		if currentErr == nil {
			_ = socialutil.WriteJSON(ctx, store, socialutil.GroupMemberKey(req.FriendGroupId, req.PeerId), current)
		} else {
			_ = store.Delete(ctx, socialutil.GroupMemberKey(req.FriendGroupId, req.PeerId))
		}
		return rpcapi.FriendGroupMemberObject{}, err
	}
	return member, nil
}

func (s *Server) PutFriendGroupMember(ctx context.Context, owner string, req rpcapi.FriendGroupMemberPutRequest) (rpcapi.FriendGroupMemberObject, error) {
	req.FriendGroupId = strings.TrimSpace(req.FriendGroupId)
	req.Id = strings.TrimSpace(req.Id)
	store, err := s.membersStore()
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := s.requireRole(ctx, owner, req.FriendGroupId, rpcapi.FriendGroupMemberRoleOwner); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	current, err := s.groupMember(ctx, req.FriendGroupId, req.Id)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if current.Role != nil && *current.Role == rpcapi.FriendGroupMemberRoleOwner {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: cannot change owner role")
	}
	member, err := s.writeMember(ctx, req.FriendGroupId, req.Id, rpcapi.FriendGroupMemberRole(req.Role))
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := s.upsertACLBinding(ctx, req.FriendGroupId, req.Id, socialutil.GroupRole(member)); err != nil {
		_ = socialutil.WriteJSON(ctx, store, socialutil.GroupMemberKey(req.FriendGroupId, req.Id), current)
		return rpcapi.FriendGroupMemberObject{}, err
	}
	return member, nil
}

func (s *Server) DeleteFriendGroupMember(ctx context.Context, owner string, req rpcapi.FriendGroupMemberDeleteRequest) (rpcapi.FriendGroupMemberObject, error) {
	req.FriendGroupId = strings.TrimSpace(req.FriendGroupId)
	req.Id = strings.TrimSpace(req.Id)
	store, err := s.membersStore()
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	current, err := s.groupMember(ctx, req.FriendGroupId, req.Id)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	role := socialutil.GroupRole(current)
	switch role {
	case rpcapi.FriendGroupMemberRoleOwner:
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: cannot delete friend group owner")
	case rpcapi.FriendGroupMemberRoleAdmin:
		if err := s.requireRole(ctx, owner, req.FriendGroupId, rpcapi.FriendGroupMemberRoleOwner); err != nil {
			return rpcapi.FriendGroupMemberObject{}, err
		}
	default:
		if owner != req.Id {
			if err := s.requireAdmin(ctx, owner, req.FriendGroupId); err != nil {
				return rpcapi.FriendGroupMemberObject{}, err
			}
		}
	}
	if err := store.Delete(ctx, socialutil.GroupMemberKey(req.FriendGroupId, req.Id)); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := s.deleteACLBinding(ctx, req.FriendGroupId, req.Id); err != nil {
		_ = socialutil.WriteJSON(ctx, store, socialutil.GroupMemberKey(req.FriendGroupId, req.Id), current)
		return rpcapi.FriendGroupMemberObject{}, err
	}
	return current, nil
}

func (s *Server) ListFriendGroupMembers(ctx context.Context, owner string, req rpcapi.FriendGroupMemberListRequest) (rpcapi.FriendGroupMemberListResponse, error) {
	if err := s.requireRead(ctx, owner, socialutil.StringValue(req.FriendGroupId)); err != nil {
		return rpcapi.FriendGroupMemberListResponse{}, err
	}
	store, err := s.membersStore()
	if err != nil {
		return rpcapi.FriendGroupMemberListResponse{}, err
	}
	entries, err := socialutil.ListPage(ctx, store, append(socialutil.GroupMembersRoot, socialutil.EscapeStoreSegment(socialutil.StringValue(req.FriendGroupId))), socialutil.StringValue(req.Cursor), socialutil.IntValue(req.Limit))
	if err != nil {
		return rpcapi.FriendGroupMemberListResponse{}, err
	}
	items := make([]rpcapi.FriendGroupMemberObject, 0, len(entries.Items))
	for _, entry := range entries.Items {
		var item rpcapi.FriendGroupMemberObject
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return rpcapi.FriendGroupMemberListResponse{}, err
		}
		items = append(items, item)
	}
	return rpcapi.FriendGroupMemberListResponse{Items: items, HasNext: entries.HasNext, NextCursor: entries.NextCursor}, nil
}

func (s *Server) SendFriendGroupMessage(ctx context.Context, owner string, req rpcapi.FriendGroupMessageSendRequest) (rpcapi.FriendGroupMessageObject, error) {
	store, err := s.messagesStore()
	if err != nil {
		return rpcapi.FriendGroupMessageObject{}, err
	}
	if s.MessageAssets == nil {
		return rpcapi.FriendGroupMessageObject{}, errors.New("social: friend group message asset store not configured")
	}
	req.FriendGroupId = strings.TrimSpace(req.FriendGroupId)
	if err := s.requireUse(ctx, owner, req.FriendGroupId); err != nil {
		return rpcapi.FriendGroupMessageObject{}, err
	}
	if req.AudioContentType != socialutil.DefaultAudioContentType {
		return rpcapi.FriendGroupMessageObject{}, errors.New("social: unsupported audio content type")
	}
	if int64(len(req.AudioBase64)) > s.messageMaxAudioBytes() {
		return rpcapi.FriendGroupMessageObject{}, errors.New("social: friend group message audio exceeds max size")
	}
	now := s.now()
	ttl, err := s.messageTTL(req.TtlSeconds)
	if err != nil {
		return rpcapi.FriendGroupMessageObject{}, err
	}
	id := s.newID()
	path := socialutil.EscapeStoreSegment(req.FriendGroupId) + "/" + socialutil.EscapeStoreSegment(id) + ".opus"
	if err := s.MessageAssets.Put(path, bytes.NewReader(req.AudioBase64)); err != nil {
		return rpcapi.FriendGroupMessageObject{}, err
	}
	size := int64(len(req.AudioBase64))
	ttlSeconds := int(ttl.Seconds())
	expiresAt := now.Add(ttl)
	item := rpcapi.FriendGroupMessageObject{
		Id:               &id,
		FriendGroupId:    &req.FriendGroupId,
		SenderPeerId:     &owner,
		AudioPath:        &path,
		AudioContentType: &req.AudioContentType,
		AudioSizeBytes:   &size,
		TtlSeconds:       &ttlSeconds,
		ExpiresAt:        &expiresAt,
		CreatedAt:        &now,
	}
	if err := socialutil.WriteJSON(ctx, store, socialutil.GroupMessageKey(req.FriendGroupId, id), item); err != nil {
		_ = s.MessageAssets.Delete(path)
		return rpcapi.FriendGroupMessageObject{}, err
	}
	return item, nil
}

func (s *Server) GetFriendGroupMessage(ctx context.Context, owner string, req rpcapi.FriendGroupMessageGetRequest) (rpcapi.FriendGroupMessageObject, error) {
	req.FriendGroupId = strings.TrimSpace(req.FriendGroupId)
	req.Id = strings.TrimSpace(req.Id)
	if err := s.requireRead(ctx, owner, req.FriendGroupId); err != nil {
		return rpcapi.FriendGroupMessageObject{}, err
	}
	store, err := s.messagesStore()
	if err != nil {
		return rpcapi.FriendGroupMessageObject{}, err
	}
	item, err := socialutil.ReadJSONValue[rpcapi.FriendGroupMessageObject](ctx, store, socialutil.GroupMessageKey(req.FriendGroupId, req.Id))
	if err != nil {
		return rpcapi.FriendGroupMessageObject{}, err
	}
	if socialutil.MessageExpired(item, s.now()) {
		return rpcapi.FriendGroupMessageObject{}, kv.ErrNotFound
	}
	return item, nil
}

func (s *Server) ListFriendGroupMessages(ctx context.Context, owner string, req rpcapi.FriendGroupMessageListRequest) (rpcapi.FriendGroupMessageListResponse, error) {
	if req.FriendGroupId != nil {
		v := strings.TrimSpace(*req.FriendGroupId)
		req.FriendGroupId = &v
	}
	if err := s.requireRead(ctx, owner, socialutil.StringValue(req.FriendGroupId)); err != nil {
		return rpcapi.FriendGroupMessageListResponse{}, err
	}
	store, err := s.messagesStore()
	if err != nil {
		return rpcapi.FriendGroupMessageListResponse{}, err
	}
	items := make([]rpcapi.FriendGroupMessageObject, 0)
	for entry, err := range store.List(ctx, append(socialutil.GroupMessagesRoot, socialutil.EscapeStoreSegment(socialutil.StringValue(req.FriendGroupId)))) {
		if err != nil {
			return rpcapi.FriendGroupMessageListResponse{}, err
		}
		var item rpcapi.FriendGroupMessageObject
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return rpcapi.FriendGroupMessageListResponse{}, err
		}
		if !socialutil.MessageExpired(item, s.now()) {
			items = append(items, item)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return socialutil.CompareByCreatedAtDesc(socialutil.TimeValue(items[i].CreatedAt), socialutil.StringValue(items[i].Id), socialutil.TimeValue(items[j].CreatedAt), socialutil.StringValue(items[j].Id))
	})
	page := socialutil.PageItems(items, socialutil.StringValue(req.Cursor), socialutil.IntValue(req.Limit), func(item rpcapi.FriendGroupMessageObject) string {
		return socialutil.StringValue(item.Id)
	})
	return rpcapi.FriendGroupMessageListResponse{Items: page.Items, HasNext: page.HasNext, NextCursor: page.NextCursor}, nil
}

func (s *Server) CleanupExpiredFriendGroupMessages(ctx context.Context) error {
	if s.Messages == nil {
		return errors.New("social: friend group message store not configured")
	}
	now := s.now()
	var deleteKeys []kv.Key
	var deleteObjects []string
	for entry, err := range s.Messages.List(ctx, socialutil.GroupMessagesRoot) {
		if err != nil {
			return err
		}
		var item rpcapi.FriendGroupMessageObject
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return err
		}
		if socialutil.MessageExpired(item, now) {
			deleteKeys = append(deleteKeys, entry.Key)
			if item.AudioPath != nil {
				deleteObjects = append(deleteObjects, *item.AudioPath)
			}
		}
	}
	if len(deleteKeys) > 0 {
		if err := s.Messages.BatchDelete(ctx, deleteKeys); err != nil {
			return err
		}
	}
	for _, name := range deleteObjects {
		if s.MessageAssets != nil {
			_ = s.MessageAssets.Delete(name)
		}
	}
	return nil
}

func (s *Server) writeMember(ctx context.Context, friendGroupID, peerID string, role rpcapi.FriendGroupMemberRole) (rpcapi.FriendGroupMemberObject, error) {
	store, err := s.membersStore()
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if !role.Valid() || role == rpcapi.FriendGroupMemberRoleOwner {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: invalid group member role")
	}
	now := s.now()
	current, err := socialutil.ReadJSONValue[rpcapi.FriendGroupMemberObject](ctx, store, socialutil.GroupMemberKey(friendGroupID, peerID))
	if err == nil && current.CreatedAt != nil {
		nowCreated := *current.CreatedAt
		current.Role = &role
		current.UpdatedAt = &now
		current.CreatedAt = &nowCreated
		return current, socialutil.WriteJSON(ctx, store, socialutil.GroupMemberKey(friendGroupID, peerID), current)
	}
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	item := rpcapi.FriendGroupMemberObject{Id: &peerID, FriendGroupId: &friendGroupID, PeerId: &peerID, Role: &role, CreatedAt: &now, UpdatedAt: &now}
	return item, socialutil.WriteJSON(ctx, store, socialutil.GroupMemberKey(friendGroupID, peerID), item)
}

func (s *Server) requireRead(ctx context.Context, owner, friendGroupID string) error {
	if _, err := s.groupMember(ctx, friendGroupID, owner); err != nil {
		return err
	}
	return s.authorize(ctx, owner, friendGroupID, apitypes.ACLPermissionFriendGroupRead)
}

func (s *Server) requireUse(ctx context.Context, owner, friendGroupID string) error {
	if _, err := s.groupMember(ctx, friendGroupID, owner); err != nil {
		return err
	}
	return s.authorize(ctx, owner, friendGroupID, apitypes.ACLPermissionFriendGroupUse)
}

func (s *Server) requireAdmin(ctx context.Context, owner, friendGroupID string) error {
	member, err := s.groupMember(ctx, friendGroupID, owner)
	if err != nil {
		return err
	}
	role := socialutil.GroupRole(member)
	if role != rpcapi.FriendGroupMemberRoleOwner && role != rpcapi.FriendGroupMemberRoleAdmin {
		return errors.New("social: friend group admin required")
	}
	return s.authorize(ctx, owner, friendGroupID, apitypes.ACLPermissionFriendGroupAdmin)
}

func (s *Server) requireRole(ctx context.Context, owner, friendGroupID string, required rpcapi.FriendGroupMemberRole) error {
	member, err := s.groupMember(ctx, friendGroupID, owner)
	if err != nil {
		return err
	}
	if socialutil.GroupRole(member) != required {
		return fmt.Errorf("social: friend group role %s required", required)
	}
	if required == rpcapi.FriendGroupMemberRoleOwner {
		return s.authorize(ctx, owner, friendGroupID, apitypes.ACLPermissionFriendGroupAdmin)
	}
	return nil
}

func (s *Server) authorize(ctx context.Context, owner, friendGroupID string, permission apitypes.ACLPermission) error {
	if s == nil || s.ACL == nil {
		return nil
	}
	return s.ACL.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject(strings.TrimSpace(owner)),
		Resource:   acl.FriendGroupResource(strings.TrimSpace(friendGroupID)),
		Permission: permission,
	})
}

func (s *Server) upsertACLBinding(ctx context.Context, friendGroupID, peerID string, role rpcapi.FriendGroupMemberRole) error {
	if s == nil || s.ACL == nil {
		return nil
	}
	roleName, permissions, err := socialutil.GroupACLRole(role)
	if err != nil {
		return err
	}
	if _, err := s.ACL.PutRole(ctx, roleName, permissions); err != nil {
		return err
	}
	_, err = s.ACL.PutPolicyBinding(ctx, socialutil.GroupACLBindingID(friendGroupID, peerID), 0, apitypes.ACLPolicy{
		Subject:  acl.PublicKeySubject(strings.TrimSpace(peerID)),
		Resource: acl.FriendGroupResource(strings.TrimSpace(friendGroupID)),
		Role:     roleName,
	})
	return err
}

func (s *Server) deleteACLBinding(ctx context.Context, friendGroupID, peerID string) error {
	if s == nil || s.ACL == nil {
		return nil
	}
	if _, err := s.ACL.DeletePolicyBinding(ctx, socialutil.GroupACLBindingID(friendGroupID, peerID)); err != nil && !errors.Is(err, acl.ErrPolicyBindingNotFound) {
		return err
	}
	return nil
}

func (s *Server) deleteACLBindings(ctx context.Context, friendGroupID string, members []rpcapi.FriendGroupMemberObject) error {
	if s == nil || s.ACL == nil {
		return nil
	}
	for _, member := range members {
		if err := s.deleteACLBinding(ctx, friendGroupID, socialutil.StringValue(member.PeerId)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) groupMember(ctx context.Context, friendGroupID, peerID string) (rpcapi.FriendGroupMemberObject, error) {
	store, err := s.membersStore()
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	return socialutil.ReadJSONValue[rpcapi.FriendGroupMemberObject](ctx, store, socialutil.GroupMemberKey(friendGroupID, peerID))
}

func (s *Server) listAllMembers(ctx context.Context, friendGroupID string) ([]rpcapi.FriendGroupMemberObject, error) {
	store, err := s.membersStore()
	if err != nil {
		return nil, err
	}
	prefix := append(append(kv.Key{}, socialutil.GroupMembersRoot...), socialutil.EscapeStoreSegment(friendGroupID))
	out := make([]rpcapi.FriendGroupMemberObject, 0)
	for entry, err := range store.List(ctx, prefix) {
		if err != nil {
			return nil, err
		}
		var item rpcapi.FriendGroupMemberObject
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Server) groupsStore() (kv.Store, error) {
	if s == nil || s.Groups == nil {
		return nil, errors.New("social: friend group service not configured")
	}
	return s.Groups, nil
}

func (s *Server) membersStore() (kv.Store, error) {
	if s == nil || s.Members == nil {
		return nil, errors.New("social: group member service not configured")
	}
	return s.Members, nil
}

func (s *Server) messagesStore() (kv.Store, error) {
	if s == nil || s.Messages == nil {
		return nil, errors.New("social: friend group message service not configured")
	}
	return s.Messages, nil
}

func (s *Server) stores() (kv.Store, kv.Store, error) {
	friendGroups, err := s.groupsStore()
	if err != nil {
		return nil, nil, err
	}
	members, err := s.membersStore()
	if err != nil {
		return nil, nil, err
	}
	return friendGroups, members, nil
}

func (s *Server) messageTTL(value *int) (time.Duration, error) {
	ttl := s.messageDefaultTTL()
	if value != nil && *value > 0 {
		ttl = time.Duration(*value) * time.Second
	}
	maxTTL := s.messageMaxTTL()
	if maxTTL > 0 && ttl > maxTTL {
		return 0, errors.New("social: friend group message ttl exceeds max ttl")
	}
	return ttl, nil
}

func (s *Server) messageDefaultTTL() time.Duration {
	if s != nil && s.MessageDefaultTTL > 0 {
		return s.MessageDefaultTTL
	}
	return socialutil.DefaultMessageTTL
}

func (s *Server) messageMaxTTL() time.Duration {
	if s != nil && s.MessageMaxTTL > 0 {
		return s.MessageMaxTTL
	}
	return socialutil.DefaultMessageMaxTTL
}

func (s *Server) messageMaxAudioBytes() int64 {
	if s != nil && s.MessageMaxAudioBytes > 0 {
		return s.MessageMaxAudioBytes
	}
	return socialutil.DefaultMaxAudioBytes
}

func (s *Server) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Server) newID() string {
	if s != nil && s.NewID != nil {
		return s.NewID()
	}
	return socialutil.NewID()
}
