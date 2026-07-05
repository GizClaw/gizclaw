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

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

type ACL interface {
	PutRole(context.Context, string, apitypes.ACLPermissionList) (apitypes.ACLRole, error)
	PutPolicyBinding(context.Context, string, float64, apitypes.ACLPolicy) (apitypes.ACLPolicyBinding, error)
	DeletePolicyBinding(context.Context, string) (apitypes.ACLPolicyBinding, error)
}

type WorkspaceService interface {
	CreateWorkspace(context.Context, adminservice.CreateWorkspaceRequestObject) (adminservice.CreateWorkspaceResponseObject, error)
	DeleteWorkspace(context.Context, adminservice.DeleteWorkspaceRequestObject) (adminservice.DeleteWorkspaceResponseObject, error)
}

type Server struct {
	Groups        kv.Store
	InviteTokens  kv.Store
	Members       kv.Store
	Belongs       kv.Store
	Messages      kv.Store
	MessageAssets objectstore.ObjectStore
	ACL           ACL
	Workspaces    WorkspaceService

	MessageDefaultTTL    time.Duration
	MessageMaxTTL        time.Duration
	MessageMaxAudioBytes int64

	Now   func() time.Time
	NewID func() string
}

type inviteTokenRecord struct {
	FriendGroupID string    `json:"friend_group_id"`
	InviteToken   string    `json:"invite_token"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

func (s *Server) CreateFriendGroup(ctx context.Context, owner string, req rpcapi.FriendGroupCreateRequest) (rpcapi.FriendGroupObject, error) {
	friendGroups, err := s.groupsStore()
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
	role := rpcapi.FriendGroupMemberRoleOwner
	workspaceName := socialutil.GroupWorkspaceName(id)
	group := rpcapi.FriendGroupObject{
		Id:                     &id,
		Name:                   &name,
		Description:            socialutil.OptionalString(strings.TrimSpace(socialutil.StringValue(req.Description))),
		CreatedByPeerPublicKey: &owner,
		WorkspaceName:          &workspaceName,
		CreatedAt:              &now,
		UpdatedAt:              &now,
	}
	createdWorkspace, err := s.ensureGroupWorkspace(ctx, workspaceName, owner)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if err := socialutil.WriteJSON(ctx, friendGroups, socialutil.GroupKey(id), group); err != nil {
		if createdWorkspace {
			_ = s.deleteWorkspace(ctx, workspaceName)
		}
		_ = s.revokeWorkspace(ctx, workspaceName, owner)
		return rpcapi.FriendGroupObject{}, err
	}
	if _, err := s.writeMember(ctx, id, owner, role); err != nil {
		if createdWorkspace {
			_ = s.deleteWorkspace(ctx, workspaceName)
		}
		_ = s.revokeWorkspace(ctx, workspaceName, owner)
		_ = friendGroups.Delete(ctx, socialutil.GroupKey(id))
		return rpcapi.FriendGroupObject{}, err
	}
	group.MyRole = &role
	return group, nil
}

func (s *Server) AdminCreateFriendGroup(ctx context.Context, name string, description *string) (rpcapi.FriendGroupObject, error) {
	friendGroups, err := s.groupsStore()
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return rpcapi.FriendGroupObject{}, errors.New("social: friend group name is required")
	}
	now := s.now()
	id := s.newID()
	workspaceName := socialutil.GroupWorkspaceName(id)
	group := rpcapi.FriendGroupObject{
		Id:            &id,
		Name:          &name,
		Description:   socialutil.OptionalString(strings.TrimSpace(socialutil.StringValue(description))),
		WorkspaceName: &workspaceName,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}
	createdWorkspace, err := s.ensureGroupWorkspace(ctx, workspaceName, "")
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if err := socialutil.WriteJSON(ctx, friendGroups, socialutil.GroupKey(id), group); err != nil {
		if createdWorkspace {
			_ = s.deleteWorkspace(ctx, workspaceName)
		}
		return rpcapi.FriendGroupObject{}, err
	}
	return group, nil
}

func (s *Server) AdminApplyFriendGroup(ctx context.Context, friendGroupID, name string, description *string) (rpcapi.FriendGroupObject, error) {
	friendGroups, err := s.groupsStore()
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if err := customid.ValidateField("friend group id", friendGroupID); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return rpcapi.FriendGroupObject{}, errors.New("social: friend group name is required")
	}
	if _, err := socialutil.ReadJSONValue[rpcapi.FriendGroupObject](ctx, friendGroups, socialutil.GroupKey(friendGroupID)); err == nil {
		return s.putFriendGroup(ctx, friendGroupID, &name, description)
	} else if !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendGroupObject{}, err
	}
	now := s.now()
	workspaceName := socialutil.GroupWorkspaceName(friendGroupID)
	group := rpcapi.FriendGroupObject{
		Id:            &friendGroupID,
		Name:          &name,
		Description:   socialutil.OptionalString(strings.TrimSpace(socialutil.StringValue(description))),
		WorkspaceName: &workspaceName,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}
	createdWorkspace, err := s.ensureGroupWorkspace(ctx, workspaceName, "")
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if err := socialutil.WriteJSON(ctx, friendGroups, socialutil.GroupKey(friendGroupID), group); err != nil {
		if createdWorkspace {
			_ = s.deleteWorkspace(ctx, workspaceName)
		}
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
	group, err := socialutil.ReadJSONValue[rpcapi.FriendGroupObject](ctx, store, socialutil.GroupKey(friendGroupID))
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	return s.withMyRole(ctx, owner, group)
}

func (s *Server) AdminGetFriendGroup(ctx context.Context, friendGroupID string) (rpcapi.FriendGroupObject, error) {
	store, err := s.groupsStore()
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	friendGroupID = strings.TrimSpace(friendGroupID)
	if friendGroupID == "" {
		return rpcapi.FriendGroupObject{}, errors.New("social: group id is required")
	}
	return socialutil.ReadJSONValue[rpcapi.FriendGroupObject](ctx, store, socialutil.GroupKey(friendGroupID))
}

func (s *Server) ListFriendGroups(ctx context.Context, owner string, req rpcapi.FriendGroupListRequest) (rpcapi.FriendGroupListResponse, error) {
	owner = strings.TrimSpace(owner)
	store, err := s.groupsStore()
	if err != nil {
		return rpcapi.FriendGroupListResponse{}, err
	}
	belongs, err := s.belongsStore()
	if err != nil {
		return rpcapi.FriendGroupListResponse{}, err
	}
	prefix := append(append(kv.Key{}, socialutil.GroupBelongsRoot...), socialutil.EscapeStoreSegment(owner))
	entries, err := socialutil.ListPage(ctx, belongs, prefix, socialutil.StringValue(req.Cursor), socialutil.IntValue(req.Limit))
	if err != nil {
		return rpcapi.FriendGroupListResponse{}, err
	}
	items := make([]rpcapi.FriendGroupObject, 0, len(entries.Items))
	for _, entry := range entries.Items {
		var member rpcapi.FriendGroupMemberObject
		if err := json.Unmarshal(entry.Value, &member); err != nil {
			return rpcapi.FriendGroupListResponse{}, err
		}
		friendGroupID := socialutil.StringValue(member.FriendGroupId)
		if friendGroupID == "" {
			friendGroupID = socialutil.UnescapeStoreSegment(entry.Key[len(entry.Key)-1])
		}
		item, err := socialutil.ReadJSONValue[rpcapi.FriendGroupObject](ctx, store, socialutil.GroupKey(friendGroupID))
		if err != nil {
			return rpcapi.FriendGroupListResponse{}, err
		}
		role := socialutil.GroupRole(member)
		item.MyRole = &role
		items = append(items, item)
	}
	return rpcapi.FriendGroupListResponse{Items: items, HasNext: entries.HasNext, NextCursor: entries.NextCursor}, nil
}

func (s *Server) AdminListFriendGroups(ctx context.Context, req rpcapi.FriendGroupListRequest) (rpcapi.FriendGroupListResponse, error) {
	store, err := s.groupsStore()
	if err != nil {
		return rpcapi.FriendGroupListResponse{}, err
	}
	entries, err := socialutil.ListPage(ctx, store, socialutil.GroupsRoot, socialutil.StringValue(req.Cursor), socialutil.IntValue(req.Limit))
	if err != nil {
		return rpcapi.FriendGroupListResponse{}, err
	}
	items := make([]rpcapi.FriendGroupObject, 0, len(entries.Items))
	for _, entry := range entries.Items {
		var item rpcapi.FriendGroupObject
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return rpcapi.FriendGroupListResponse{}, err
		}
		items = append(items, item)
	}
	return rpcapi.FriendGroupListResponse{Items: items, HasNext: entries.HasNext, NextCursor: entries.NextCursor}, nil
}

func (s *Server) PutFriendGroup(ctx context.Context, owner string, req rpcapi.FriendGroupPutRequest) (rpcapi.FriendGroupObject, error) {
	friendGroupID := strings.TrimSpace(req.Id)
	if friendGroupID == "" {
		return rpcapi.FriendGroupObject{}, errors.New("social: group id is required")
	}
	if err := s.requireRole(ctx, owner, friendGroupID, rpcapi.FriendGroupMemberRoleOwner); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	group, err := s.putFriendGroup(ctx, friendGroupID, req.Name, req.Description)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	return s.withMyRole(ctx, owner, group)
}

func (s *Server) AdminPutFriendGroup(ctx context.Context, friendGroupID string, name, description *string) (rpcapi.FriendGroupObject, error) {
	friendGroupID = strings.TrimSpace(friendGroupID)
	if friendGroupID == "" {
		return rpcapi.FriendGroupObject{}, errors.New("social: group id is required")
	}
	return s.putFriendGroup(ctx, friendGroupID, name, description)
}

func (s *Server) DeleteFriendGroup(ctx context.Context, owner string, req rpcapi.FriendGroupDeleteRequest) (rpcapi.FriendGroupObject, error) {
	friendGroupID := strings.TrimSpace(req.Id)
	if friendGroupID == "" {
		return rpcapi.FriendGroupObject{}, errors.New("social: group id is required")
	}
	if err := s.requireRole(ctx, owner, friendGroupID, rpcapi.FriendGroupMemberRoleOwner); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	return s.deleteFriendGroup(ctx, friendGroupID)
}

func (s *Server) AdminDeleteFriendGroup(ctx context.Context, friendGroupID string) (rpcapi.FriendGroupObject, error) {
	friendGroupID = strings.TrimSpace(friendGroupID)
	if friendGroupID == "" {
		return rpcapi.FriendGroupObject{}, errors.New("social: group id is required")
	}
	return s.deleteFriendGroup(ctx, friendGroupID)
}

func (s *Server) GetFriendGroupInviteToken(ctx context.Context, owner string, req rpcapi.FriendGroupInviteTokenGetRequest) (rpcapi.FriendGroupInviteTokenGetResponse, error) {
	store, err := s.groupInviteTokensStore()
	if err != nil {
		return rpcapi.FriendGroupInviteTokenGetResponse{}, err
	}
	friendGroupID := strings.TrimSpace(req.FriendGroupId)
	if err := s.requireRole(ctx, owner, friendGroupID, rpcapi.FriendGroupMemberRoleOwner); err != nil {
		return rpcapi.FriendGroupInviteTokenGetResponse{}, err
	}
	record, ok, err := s.activeGroupInviteToken(ctx, store, friendGroupID)
	if err != nil || !ok {
		return rpcapi.FriendGroupInviteTokenGetResponse{}, err
	}
	return rpcapi.FriendGroupInviteTokenGetResponse{InviteToken: &record.InviteToken, ExpiresAt: &record.ExpiresAt}, nil
}

func (s *Server) CreateFriendGroupInviteToken(ctx context.Context, owner string, req rpcapi.FriendGroupInviteTokenCreateRequest) (rpcapi.FriendGroupInviteTokenCreateResponse, error) {
	store, err := s.groupInviteTokensStore()
	if err != nil {
		return rpcapi.FriendGroupInviteTokenCreateResponse{}, err
	}
	friendGroupID := strings.TrimSpace(req.FriendGroupId)
	if err := s.requireRole(ctx, owner, friendGroupID, rpcapi.FriendGroupMemberRoleOwner); err != nil {
		return rpcapi.FriendGroupInviteTokenCreateResponse{}, err
	}
	if record, ok, err := s.activeGroupInviteToken(ctx, store, friendGroupID); err != nil {
		return rpcapi.FriendGroupInviteTokenCreateResponse{}, err
	} else if ok {
		return rpcapi.FriendGroupInviteTokenCreateResponse{InviteToken: record.InviteToken, ExpiresAt: record.ExpiresAt}, nil
	}
	now := s.now()
	record := inviteTokenRecord{
		FriendGroupID: friendGroupID,
		InviteToken:   s.newID(),
		CreatedAt:     now,
		ExpiresAt:     now.Add(s.inviteTokenTTL()),
	}
	if strings.TrimSpace(record.InviteToken) == "" {
		return rpcapi.FriendGroupInviteTokenCreateResponse{}, errors.New("social: invite token is empty")
	}
	if err := socialutil.WriteJSON(ctx, store, socialutil.GroupInviteTokenKey(friendGroupID), record); err != nil {
		return rpcapi.FriendGroupInviteTokenCreateResponse{}, err
	}
	return rpcapi.FriendGroupInviteTokenCreateResponse{InviteToken: record.InviteToken, ExpiresAt: record.ExpiresAt}, nil
}

func (s *Server) ClearFriendGroupInviteToken(ctx context.Context, owner string, req rpcapi.FriendGroupInviteTokenClearRequest) (rpcapi.FriendGroupInviteTokenClearResponse, error) {
	store, err := s.groupInviteTokensStore()
	if err != nil {
		return rpcapi.FriendGroupInviteTokenClearResponse{}, err
	}
	friendGroupID := strings.TrimSpace(req.FriendGroupId)
	if err := s.requireRole(ctx, owner, friendGroupID, rpcapi.FriendGroupMemberRoleOwner); err != nil {
		return rpcapi.FriendGroupInviteTokenClearResponse{}, err
	}
	if err := store.Delete(ctx, socialutil.GroupInviteTokenKey(friendGroupID)); err != nil && !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendGroupInviteTokenClearResponse{}, err
	}
	return rpcapi.FriendGroupInviteTokenClearResponse{}, nil
}

func (s *Server) AdminGetFriendGroupInviteToken(ctx context.Context, friendGroupID string) (rpcapi.FriendGroupInviteTokenGetResponse, error) {
	if _, err := s.AdminGetFriendGroup(ctx, friendGroupID); err != nil {
		return rpcapi.FriendGroupInviteTokenGetResponse{}, err
	}
	store, err := s.groupInviteTokensStore()
	if err != nil {
		return rpcapi.FriendGroupInviteTokenGetResponse{}, err
	}
	record, ok, err := s.activeGroupInviteToken(ctx, store, strings.TrimSpace(friendGroupID))
	if err != nil || !ok {
		return rpcapi.FriendGroupInviteTokenGetResponse{}, err
	}
	return rpcapi.FriendGroupInviteTokenGetResponse{InviteToken: &record.InviteToken, ExpiresAt: &record.ExpiresAt}, nil
}

func (s *Server) AdminPutFriendGroupInviteToken(ctx context.Context, friendGroupID, inviteToken string, expiresAt time.Time) (rpcapi.FriendGroupInviteTokenCreateResponse, error) {
	if _, err := s.AdminGetFriendGroup(ctx, friendGroupID); err != nil {
		return rpcapi.FriendGroupInviteTokenCreateResponse{}, err
	}
	store, err := s.groupInviteTokensStore()
	if err != nil {
		return rpcapi.FriendGroupInviteTokenCreateResponse{}, err
	}
	friendGroupID = strings.TrimSpace(friendGroupID)
	inviteToken = strings.TrimSpace(inviteToken)
	if inviteToken == "" || !expiresAt.After(s.now()) {
		return rpcapi.FriendGroupInviteTokenCreateResponse{}, errors.New("social: active invite token and expires_at are required")
	}
	record := inviteTokenRecord{
		FriendGroupID: friendGroupID,
		InviteToken:   inviteToken,
		CreatedAt:     s.now(),
		ExpiresAt:     expiresAt.UTC(),
	}
	if err := socialutil.WriteJSON(ctx, store, socialutil.GroupInviteTokenKey(friendGroupID), record); err != nil {
		return rpcapi.FriendGroupInviteTokenCreateResponse{}, err
	}
	return rpcapi.FriendGroupInviteTokenCreateResponse{InviteToken: record.InviteToken, ExpiresAt: record.ExpiresAt}, nil
}

func (s *Server) AdminDeleteFriendGroupInviteToken(ctx context.Context, friendGroupID string) (rpcapi.FriendGroupInviteTokenClearResponse, error) {
	if _, err := s.AdminGetFriendGroup(ctx, friendGroupID); err != nil {
		return rpcapi.FriendGroupInviteTokenClearResponse{}, err
	}
	store, err := s.groupInviteTokensStore()
	if err != nil {
		return rpcapi.FriendGroupInviteTokenClearResponse{}, err
	}
	if err := store.Delete(ctx, socialutil.GroupInviteTokenKey(strings.TrimSpace(friendGroupID))); err != nil && !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendGroupInviteTokenClearResponse{}, err
	}
	return rpcapi.FriendGroupInviteTokenClearResponse{}, nil
}

func (s *Server) JoinFriendGroup(ctx context.Context, owner string, req rpcapi.FriendGroupJoinRequest) (rpcapi.FriendGroupJoinResponse, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return rpcapi.FriendGroupJoinResponse{}, errors.New("social: peer public key is required")
	}
	record, err := s.findGroupInviteToken(ctx, strings.TrimSpace(req.InviteToken))
	if err != nil {
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	friendGroupID := strings.TrimSpace(record.FriendGroupID)
	if friendGroupID == "" {
		return rpcapi.FriendGroupJoinResponse{}, errors.New("social: invite token group is empty")
	}
	if existing, err := s.groupMember(ctx, friendGroupID, owner); err == nil {
		group, err := s.GetFriendGroup(ctx, owner, rpcapi.FriendGroupGetRequest{Id: friendGroupID})
		if err != nil {
			return rpcapi.FriendGroupJoinResponse{}, err
		}
		return rpcapi.FriendGroupJoinResponse{Group: group, Member: existing}, nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	member, err := s.writeMember(ctx, friendGroupID, owner, rpcapi.FriendGroupMemberRoleMember)
	if err != nil {
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	workspaceName, err := s.workspaceName(ctx, friendGroupID)
	if err != nil {
		s.restoreMember(ctx, friendGroupID, owner, rpcapi.FriendGroupMemberObject{}, kv.ErrNotFound)
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	if err := s.grantWorkspace(ctx, workspaceName, owner); err != nil {
		s.restoreMember(ctx, friendGroupID, owner, rpcapi.FriendGroupMemberObject{}, kv.ErrNotFound)
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	group, err := s.GetFriendGroup(ctx, owner, rpcapi.FriendGroupGetRequest{Id: friendGroupID})
	if err != nil {
		_ = s.revokeWorkspace(ctx, workspaceName, owner)
		s.restoreMember(ctx, friendGroupID, owner, rpcapi.FriendGroupMemberObject{}, kv.ErrNotFound)
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	return rpcapi.FriendGroupJoinResponse{Group: group, Member: member}, nil
}

func (s *Server) AddFriendGroupMember(ctx context.Context, owner string, req rpcapi.FriendGroupMemberAddRequest) (rpcapi.FriendGroupMemberObject, error) {
	req.FriendGroupId = strings.TrimSpace(req.FriendGroupId)
	req.PeerPublicKey = strings.TrimSpace(req.PeerPublicKey)
	if !req.Role.Valid() {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: invalid group member role")
	}
	if req.Role == rpcapi.FriendGroupMemberMutableRole("admin") {
		if err := s.requireRole(ctx, owner, req.FriendGroupId, rpcapi.FriendGroupMemberRoleOwner); err != nil {
			return rpcapi.FriendGroupMemberObject{}, err
		}
	} else if err := s.requireAdmin(ctx, owner, req.FriendGroupId); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	current, currentErr := s.groupMember(ctx, req.FriendGroupId, req.PeerPublicKey)
	if currentErr != nil && !errors.Is(currentErr, kv.ErrNotFound) {
		return rpcapi.FriendGroupMemberObject{}, currentErr
	}
	if currentErr == nil && socialutil.GroupRole(current) == rpcapi.FriendGroupMemberRoleOwner {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: cannot change owner role")
	}
	member, err := s.writeMember(ctx, req.FriendGroupId, req.PeerPublicKey, rpcapi.FriendGroupMemberRole(req.Role))
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	workspaceName, err := s.workspaceName(ctx, req.FriendGroupId)
	if err != nil {
		s.restoreMember(ctx, req.FriendGroupId, req.PeerPublicKey, current, currentErr)
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := s.grantWorkspace(ctx, workspaceName, req.PeerPublicKey); err != nil {
		s.restoreMember(ctx, req.FriendGroupId, req.PeerPublicKey, current, currentErr)
		return rpcapi.FriendGroupMemberObject{}, err
	}
	return member, nil
}

func (s *Server) PutFriendGroupMember(ctx context.Context, owner string, req rpcapi.FriendGroupMemberPutRequest) (rpcapi.FriendGroupMemberObject, error) {
	req.FriendGroupId = strings.TrimSpace(req.FriendGroupId)
	req.Id = strings.TrimSpace(req.Id)
	if !req.Role.Valid() {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: invalid group member role")
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
	return member, nil
}

func (s *Server) DeleteFriendGroupMember(ctx context.Context, owner string, req rpcapi.FriendGroupMemberDeleteRequest) (rpcapi.FriendGroupMemberObject, error) {
	req.FriendGroupId = strings.TrimSpace(req.FriendGroupId)
	req.Id = strings.TrimSpace(req.Id)
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
	members, err := s.membersStore()
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := members.Delete(ctx, socialutil.GroupMemberKey(req.FriendGroupId, req.Id)); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	belongs, err := s.belongsStore()
	if err != nil {
		_ = socialutil.WriteJSON(ctx, members, socialutil.GroupMemberKey(req.FriendGroupId, req.Id), current)
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := belongs.Delete(ctx, socialutil.GroupBelongKey(req.Id, req.FriendGroupId)); err != nil && !errors.Is(err, kv.ErrNotFound) {
		_ = socialutil.WriteJSON(ctx, members, socialutil.GroupMemberKey(req.FriendGroupId, req.Id), current)
		return rpcapi.FriendGroupMemberObject{}, err
	}
	workspaceName, err := s.workspaceName(ctx, req.FriendGroupId)
	if err != nil {
		s.restoreMember(ctx, req.FriendGroupId, req.Id, current, nil)
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := s.revokeWorkspace(ctx, workspaceName, req.Id); err != nil {
		s.restoreMember(ctx, req.FriendGroupId, req.Id, current, nil)
		return rpcapi.FriendGroupMemberObject{}, err
	}
	return current, nil
}

func (s *Server) ListFriendGroupMembers(ctx context.Context, owner string, req rpcapi.FriendGroupMemberListRequest) (rpcapi.FriendGroupMemberListResponse, error) {
	if err := s.requireRead(ctx, owner, socialutil.StringValue(req.FriendGroupId)); err != nil {
		return rpcapi.FriendGroupMemberListResponse{}, err
	}
	return s.listFriendGroupMembers(ctx, socialutil.StringValue(req.FriendGroupId), socialutil.StringValue(req.Cursor), socialutil.IntValue(req.Limit))
}

func (s *Server) AdminListFriendGroupMembers(ctx context.Context, friendGroupID string, req rpcapi.FriendGroupMemberListRequest) (rpcapi.FriendGroupMemberListResponse, error) {
	if _, err := s.AdminGetFriendGroup(ctx, friendGroupID); err != nil {
		return rpcapi.FriendGroupMemberListResponse{}, err
	}
	return s.listFriendGroupMembers(ctx, friendGroupID, socialutil.StringValue(req.Cursor), socialutil.IntValue(req.Limit))
}

func (s *Server) AdminPutFriendGroupMember(ctx context.Context, friendGroupID, peerID string, role rpcapi.FriendGroupMemberRole) (rpcapi.FriendGroupMemberObject, error) {
	friendGroupID = strings.TrimSpace(friendGroupID)
	peerID = strings.TrimSpace(peerID)
	if friendGroupID == "" || peerID == "" {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: friend group id and peer public key are required")
	}
	if !role.Valid() {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: invalid group member role")
	}
	workspaceName, err := s.workspaceName(ctx, friendGroupID)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	current, currentErr := s.groupMember(ctx, friendGroupID, peerID)
	if currentErr != nil && !errors.Is(currentErr, kv.ErrNotFound) {
		return rpcapi.FriendGroupMemberObject{}, currentErr
	}
	member, err := s.writeMember(ctx, friendGroupID, peerID, role)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := s.grantWorkspace(ctx, workspaceName, peerID); err != nil {
		s.restoreMember(ctx, friendGroupID, peerID, current, currentErr)
		return rpcapi.FriendGroupMemberObject{}, err
	}
	return member, nil
}

func (s *Server) AdminGetFriendGroupMember(ctx context.Context, friendGroupID, peerID string) (rpcapi.FriendGroupMemberObject, error) {
	if _, err := s.AdminGetFriendGroup(ctx, friendGroupID); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	return s.groupMember(ctx, strings.TrimSpace(friendGroupID), strings.TrimSpace(peerID))
}

func (s *Server) AdminDeleteFriendGroupMember(ctx context.Context, friendGroupID, peerID string) (rpcapi.FriendGroupMemberObject, error) {
	friendGroupID = strings.TrimSpace(friendGroupID)
	peerID = strings.TrimSpace(peerID)
	current, err := s.groupMember(ctx, friendGroupID, peerID)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	members, err := s.membersStore()
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := members.Delete(ctx, socialutil.GroupMemberKey(friendGroupID, peerID)); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	belongs, err := s.belongsStore()
	if err != nil {
		_ = socialutil.WriteJSON(ctx, members, socialutil.GroupMemberKey(friendGroupID, peerID), current)
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := belongs.Delete(ctx, socialutil.GroupBelongKey(peerID, friendGroupID)); err != nil && !errors.Is(err, kv.ErrNotFound) {
		_ = socialutil.WriteJSON(ctx, members, socialutil.GroupMemberKey(friendGroupID, peerID), current)
		return rpcapi.FriendGroupMemberObject{}, err
	}
	workspaceName, err := s.workspaceName(ctx, friendGroupID)
	if err != nil {
		s.restoreMember(ctx, friendGroupID, peerID, current, nil)
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := s.revokeWorkspace(ctx, workspaceName, peerID); err != nil {
		s.restoreMember(ctx, friendGroupID, peerID, current, nil)
		return rpcapi.FriendGroupMemberObject{}, err
	}
	return current, nil
}

func (s *Server) listFriendGroupMembers(ctx context.Context, friendGroupID, cursor string, limit int) (rpcapi.FriendGroupMemberListResponse, error) {
	store, err := s.membersStore()
	if err != nil {
		return rpcapi.FriendGroupMemberListResponse{}, err
	}
	entries, err := socialutil.ListPage(ctx, store, append(socialutil.GroupMembersRoot, socialutil.EscapeStoreSegment(strings.TrimSpace(friendGroupID))), cursor, limit)
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

// Deprecated: send chatroom content through the active workspace runtime and use workspace history for storage.
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
		Id:                  &id,
		FriendGroupId:       &req.FriendGroupId,
		SenderPeerPublicKey: &owner,
		AudioPath:           &path,
		AudioContentType:    &req.AudioContentType,
		AudioSizeBytes:      &size,
		TtlSeconds:          &ttlSeconds,
		ExpiresAt:           &expiresAt,
		CreatedAt:           &now,
	}
	if err := socialutil.WriteJSON(ctx, store, socialutil.GroupMessageKey(req.FriendGroupId, id), item); err != nil {
		_ = s.MessageAssets.Delete(path)
		return rpcapi.FriendGroupMessageObject{}, err
	}
	return item, nil
}

// Deprecated: read chatroom records through workspace history get/audio.get.
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

// Deprecated: read chatroom records through workspace history list/get.
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
	members, err := s.membersStore()
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	belongs, err := s.belongsStore()
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	friendGroupID = strings.TrimSpace(friendGroupID)
	peerID = strings.TrimSpace(peerID)
	if friendGroupID == "" || peerID == "" {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: friend group id and peer public key are required")
	}
	if !role.Valid() {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: invalid group member role")
	}
	now := s.now()
	current, currentErr := socialutil.ReadJSONValue[rpcapi.FriendGroupMemberObject](ctx, members, socialutil.GroupMemberKey(friendGroupID, peerID))
	var item rpcapi.FriendGroupMemberObject
	if currentErr == nil && current.CreatedAt != nil {
		nowCreated := *current.CreatedAt
		current.Role = &role
		current.UpdatedAt = &now
		current.CreatedAt = &nowCreated
		item = current
	} else {
		if currentErr != nil && !errors.Is(currentErr, kv.ErrNotFound) {
			return rpcapi.FriendGroupMemberObject{}, currentErr
		}
		item = rpcapi.FriendGroupMemberObject{Id: &peerID, FriendGroupId: &friendGroupID, PeerPublicKey: &peerID, Role: &role, CreatedAt: &now, UpdatedAt: &now}
	}
	if err := socialutil.WriteJSON(ctx, members, socialutil.GroupMemberKey(friendGroupID, peerID), item); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := socialutil.WriteJSON(ctx, belongs, socialutil.GroupBelongKey(peerID, friendGroupID), item); err != nil {
		s.restoreMember(ctx, friendGroupID, peerID, current, currentErr)
		return rpcapi.FriendGroupMemberObject{}, err
	}
	return item, nil
}

func (s *Server) putFriendGroup(ctx context.Context, friendGroupID string, name, description *string) (rpcapi.FriendGroupObject, error) {
	store, err := s.groupsStore()
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	group, err := socialutil.ReadJSONValue[rpcapi.FriendGroupObject](ctx, store, socialutil.GroupKey(friendGroupID))
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if name != nil {
		v := strings.TrimSpace(*name)
		if v == "" {
			return rpcapi.FriendGroupObject{}, errors.New("social: friend group name is required")
		}
		group.Name = &v
	}
	if description != nil {
		group.Description = socialutil.OptionalString(strings.TrimSpace(*description))
	}
	now := s.now()
	group.UpdatedAt = &now
	if err := socialutil.WriteJSON(ctx, store, socialutil.GroupKey(friendGroupID), group); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	return group, nil
}

func (s *Server) deleteFriendGroup(ctx context.Context, friendGroupID string) (rpcapi.FriendGroupObject, error) {
	friendGroups, err := s.groupsStore()
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	group, err := socialutil.ReadJSONValue[rpcapi.FriendGroupObject](ctx, friendGroups, socialutil.GroupKey(friendGroupID))
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	members, err := s.listAllMembers(ctx, friendGroupID)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	workspaceName := socialutil.StringValue(group.WorkspaceName)
	if workspaceName == "" {
		workspaceName = socialutil.GroupWorkspaceName(friendGroupID)
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
	if err := s.deleteBelongs(ctx, friendGroupID, members); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if s.Messages != nil {
		if err := socialutil.DeletePrefix(ctx, s.Messages, append(socialutil.GroupMessagesRoot, socialutil.EscapeStoreSegment(friendGroupID))); err != nil {
			return rpcapi.FriendGroupObject{}, err
		}
	}
	if s.InviteTokens != nil {
		if err := s.InviteTokens.Delete(ctx, socialutil.GroupInviteTokenKey(friendGroupID)); err != nil && !errors.Is(err, kv.ErrNotFound) {
			return rpcapi.FriendGroupObject{}, err
		}
	}
	if err := s.deleteWorkspaceBindings(ctx, workspaceName, members); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if err := s.deleteWorkspace(ctx, workspaceName); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if err := friendGroups.Delete(ctx, socialutil.GroupKey(friendGroupID)); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	return group, nil
}

func (s *Server) withMyRole(ctx context.Context, owner string, group rpcapi.FriendGroupObject) (rpcapi.FriendGroupObject, error) {
	member, err := s.groupMember(ctx, socialutil.StringValue(group.Id), owner)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	role := socialutil.GroupRole(member)
	group.MyRole = &role
	return group, nil
}

func (s *Server) requireRead(ctx context.Context, owner, friendGroupID string) error {
	if _, err := s.groupMember(ctx, friendGroupID, owner); err != nil {
		return err
	}
	return nil
}

func (s *Server) requireUse(ctx context.Context, owner, friendGroupID string) error {
	if _, err := s.groupMember(ctx, friendGroupID, owner); err != nil {
		return err
	}
	return nil
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
	return nil
}

func (s *Server) requireRole(ctx context.Context, owner, friendGroupID string, required rpcapi.FriendGroupMemberRole) error {
	member, err := s.groupMember(ctx, friendGroupID, owner)
	if err != nil {
		return err
	}
	if socialutil.GroupRole(member) != required {
		return fmt.Errorf("social: friend group role %s required", required)
	}
	return nil
}

func (s *Server) ensureGroupWorkspace(ctx context.Context, workspaceName string, owner string) (bool, error) {
	created := false
	if s.Workspaces != nil {
		body := adminservice.WorkspaceUpsert{
			Name:         workspaceName,
			WorkflowName: socialutil.ChatRoomWorkflowName,
			Parameters:   socialutil.ChatRoomWorkspaceParameters(apitypes.ChatRoomModeGroup),
		}
		resp, err := s.Workspaces.CreateWorkspace(ctx, adminservice.CreateWorkspaceRequestObject{Body: &body})
		if err != nil {
			return false, err
		}
		switch resp.(type) {
		case adminservice.CreateWorkspace200JSONResponse:
			created = true
		case adminservice.CreateWorkspace409JSONResponse:
		default:
			return false, errors.New("social: create group chat workspace failed")
		}
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return created, nil
	}
	if err := s.grantWorkspace(ctx, workspaceName, owner); err != nil {
		if created {
			_ = s.deleteWorkspace(ctx, workspaceName)
		}
		return false, err
	}
	return created, nil
}

func (s *Server) workspaceName(ctx context.Context, friendGroupID string) (string, error) {
	store, err := s.groupsStore()
	if err != nil {
		return "", err
	}
	group, err := socialutil.ReadJSONValue[rpcapi.FriendGroupObject](ctx, store, socialutil.GroupKey(friendGroupID))
	if err != nil {
		return "", err
	}
	if value := socialutil.StringValue(group.WorkspaceName); value != "" {
		return value, nil
	}
	return socialutil.GroupWorkspaceName(friendGroupID), nil
}

func (s *Server) grantWorkspace(ctx context.Context, workspaceName string, peers ...string) error {
	if s == nil || s.ACL == nil {
		return nil
	}
	roleName, permissions := socialutil.WorkspaceACLRole()
	if _, err := s.ACL.PutRole(ctx, roleName, permissions); err != nil {
		return err
	}
	for _, peerID := range peers {
		peerID = strings.TrimSpace(peerID)
		if peerID == "" {
			continue
		}
		if _, err := s.ACL.PutPolicyBinding(ctx, socialutil.WorkspaceACLBindingID(workspaceName, peerID), 0, apitypes.ACLPolicy{
			Subject:  acl.PublicKeySubject(peerID),
			Resource: acl.WorkspaceResource(workspaceName),
			Role:     roleName,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) revokeWorkspace(ctx context.Context, workspaceName string, peers ...string) error {
	if s == nil || s.ACL == nil {
		return nil
	}
	for _, peerID := range peers {
		peerID = strings.TrimSpace(peerID)
		if peerID == "" {
			continue
		}
		if _, err := s.ACL.DeletePolicyBinding(ctx, socialutil.WorkspaceACLBindingID(workspaceName, peerID)); err != nil && !errors.Is(err, acl.ErrPolicyBindingNotFound) {
			return err
		}
	}
	return nil
}

func (s *Server) deleteWorkspaceBindings(ctx context.Context, workspaceName string, members []rpcapi.FriendGroupMemberObject) error {
	if s == nil || s.ACL == nil {
		return nil
	}
	for _, member := range members {
		if err := s.revokeWorkspace(ctx, workspaceName, socialutil.StringValue(member.PeerPublicKey)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) deleteBelongs(ctx context.Context, friendGroupID string, members []rpcapi.FriendGroupMemberObject) error {
	belongs, err := s.belongsStore()
	if err != nil {
		return err
	}
	for _, member := range members {
		peerID := socialutil.StringValue(member.PeerPublicKey)
		if peerID == "" {
			continue
		}
		if err := belongs.Delete(ctx, socialutil.GroupBelongKey(peerID, friendGroupID)); err != nil && !errors.Is(err, kv.ErrNotFound) {
			return err
		}
	}
	return nil
}

func (s *Server) deleteWorkspace(ctx context.Context, workspaceName string) error {
	if s == nil || s.Workspaces == nil {
		return nil
	}
	resp, err := s.Workspaces.DeleteWorkspace(ctx, adminservice.DeleteWorkspaceRequestObject{Name: workspaceName})
	if err != nil {
		return err
	}
	switch resp.(type) {
	case adminservice.DeleteWorkspace200JSONResponse, adminservice.DeleteWorkspace404JSONResponse:
		return nil
	default:
		return errors.New("social: delete group chat workspace failed")
	}
}

func (s *Server) restoreMember(ctx context.Context, friendGroupID, peerID string, current rpcapi.FriendGroupMemberObject, currentErr error) {
	members, membersErr := s.membersStore()
	belongs, belongsErr := s.belongsStore()
	if membersErr != nil || belongsErr != nil {
		return
	}
	if currentErr == nil {
		_ = socialutil.WriteJSON(ctx, members, socialutil.GroupMemberKey(friendGroupID, peerID), current)
		_ = socialutil.WriteJSON(ctx, belongs, socialutil.GroupBelongKey(peerID, friendGroupID), current)
		return
	}
	_ = members.Delete(ctx, socialutil.GroupMemberKey(friendGroupID, peerID))
	_ = belongs.Delete(ctx, socialutil.GroupBelongKey(peerID, friendGroupID))
}

func (s *Server) groupMember(ctx context.Context, friendGroupID, peerID string) (rpcapi.FriendGroupMemberObject, error) {
	store, err := s.membersStore()
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	return socialutil.ReadJSONValue[rpcapi.FriendGroupMemberObject](ctx, store, socialutil.GroupMemberKey(friendGroupID, peerID))
}

func (s *Server) activeGroupInviteToken(ctx context.Context, store kv.Store, friendGroupID string) (inviteTokenRecord, bool, error) {
	if strings.TrimSpace(friendGroupID) == "" {
		return inviteTokenRecord{}, false, errors.New("social: group id is required")
	}
	record, err := socialutil.ReadJSONValue[inviteTokenRecord](ctx, store, socialutil.GroupInviteTokenKey(friendGroupID))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return inviteTokenRecord{}, false, nil
		}
		return inviteTokenRecord{}, false, err
	}
	if strings.TrimSpace(record.InviteToken) == "" || !record.ExpiresAt.After(s.now()) {
		_ = store.Delete(ctx, socialutil.GroupInviteTokenKey(friendGroupID))
		return inviteTokenRecord{}, false, nil
	}
	return record, true, nil
}

func (s *Server) findGroupInviteToken(ctx context.Context, inviteToken string) (inviteTokenRecord, error) {
	inviteToken = strings.TrimSpace(inviteToken)
	if inviteToken == "" {
		return inviteTokenRecord{}, errors.New("social: invite token is required")
	}
	store, err := s.groupInviteTokensStore()
	if err != nil {
		return inviteTokenRecord{}, err
	}
	now := s.now()
	for entry, err := range store.List(ctx, socialutil.GroupInviteTokensRoot) {
		if err != nil {
			return inviteTokenRecord{}, err
		}
		var record inviteTokenRecord
		if err := json.Unmarshal(entry.Value, &record); err != nil {
			return inviteTokenRecord{}, err
		}
		if strings.TrimSpace(record.InviteToken) == "" || !record.ExpiresAt.After(now) {
			_ = store.Delete(ctx, entry.Key)
			continue
		}
		if record.InviteToken == inviteToken {
			return record, nil
		}
	}
	return inviteTokenRecord{}, errors.New("social: invite token not found")
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

func (s *Server) groupInviteTokensStore() (kv.Store, error) {
	if s == nil || s.InviteTokens == nil {
		return nil, errors.New("social: friend group invite token service not configured")
	}
	return s.InviteTokens, nil
}

func (s *Server) membersStore() (kv.Store, error) {
	if s == nil || s.Members == nil {
		return nil, errors.New("social: group member service not configured")
	}
	return s.Members, nil
}

func (s *Server) belongsStore() (kv.Store, error) {
	if s == nil {
		return nil, errors.New("social: group belong service not configured")
	}
	if s.Belongs != nil {
		return s.Belongs, nil
	}
	if s.Members != nil {
		return s.Members, nil
	}
	return nil, errors.New("social: group belong service not configured")
}

func (s *Server) messagesStore() (kv.Store, error) {
	if s == nil || s.Messages == nil {
		return nil, errors.New("social: friend group message service not configured")
	}
	return s.Messages, nil
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

func (s *Server) inviteTokenTTL() time.Duration {
	return socialutil.DefaultInviteTokenTTL
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
