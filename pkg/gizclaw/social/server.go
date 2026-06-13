package social

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/acl"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
	"github.com/GizClaw/gizclaw-go/pkg/store/objectstore"
)

const (
	defaultListLimit        = 50
	maxListLimit            = 200
	defaultFriendOTPTTL     = 10 * time.Minute
	defaultMessageTTL       = 24 * time.Hour
	defaultMessageMaxTTL    = 7 * 24 * time.Hour
	defaultCleanupInterval  = 5 * time.Minute
	defaultMaxAudioBytes    = 2 * 1024 * 1024
	defaultAudioContentType = "audio/opus"
	groupOwnerRoleName      = "social-friend-group-owner"
	groupAdminRoleName      = "social-friend-group-admin"
	groupMemberRoleName     = "social-friend-group-member"
)

var (
	contactsRoot       = kv.Key{"contacts"}
	friendRequestsRoot = kv.Key{"friend-requests"}
	friendsRoot        = kv.Key{"friends"}
	friendOTPRoot      = kv.Key{"friend-otps"}
	groupsRoot         = kv.Key{"friend-groups"}
	groupMembersRoot   = kv.Key{"friend-group-members"}
	groupMessagesRoot  = kv.Key{"friend-group-messages"}
)

type FriendGroupACL interface {
	PutRole(context.Context, string, apitypes.ACLPermissionList) (apitypes.ACLRole, error)
	PutPolicyBinding(context.Context, string, float64, apitypes.ACLPolicy) (apitypes.ACLPolicyBinding, error)
	DeletePolicyBinding(context.Context, string) (apitypes.ACLPolicyBinding, error)
	Authorize(context.Context, acl.AuthorizeRequest) error
}

type Server struct {
	Contacts            kv.Store
	FriendRequests      kv.Store
	Friends             kv.Store
	FriendGroups        kv.Store
	FriendGroupMembers  kv.Store
	FriendGroupMessages kv.Store
	MessageAssets       objectstore.ObjectStore
	ACL                 FriendGroupACL

	FriendOTPTTL           time.Duration
	MessageDefaultTTL      time.Duration
	MessageMaxTTL          time.Duration
	MessageCleanupInterval time.Duration
	MessageMaxAudioBytes   int64

	Now   func() time.Time
	NewID func() string
}

type friendOTPRecord struct {
	PeerID    string    `json:"peer_id"`
	CodeHash  string    `json:"code_hash"`
	ExpiresAt time.Time `json:"expires_at"`
	Consumed  bool      `json:"consumed"`
}

func (s *Server) ReportFriendOTP(ctx context.Context, peerID, code string) error {
	store, err := s.friendRequestsStore()
	if err != nil {
		return err
	}
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return errors.New("social: peer id is required")
	}
	if !isSixDigitCode(code) {
		return errors.New("social: friend otp must be exactly 6 digits")
	}
	record := friendOTPRecord{
		PeerID:    peerID,
		CodeHash:  hashCode(code),
		ExpiresAt: s.now().Add(s.friendOTPTTL()),
	}
	return writeJSON(ctx, store, friendOTPKey(peerID), record)
}

func (s *Server) ListContacts(ctx context.Context, owner string, req rpcapi.ContactListRequest) (rpcapi.ContactListResponse, error) {
	store, err := s.contactsStore()
	if err != nil {
		return rpcapi.ContactListResponse{}, err
	}
	prefix := ownerPrefix(contactsRoot, owner)
	entries, err := listPage(ctx, store, prefix, stringValue(req.Cursor), intValue(req.Limit))
	if err != nil {
		return rpcapi.ContactListResponse{}, err
	}
	items := make([]rpcapi.ContactObject, 0, len(entries.items))
	for _, entry := range entries.items {
		var item rpcapi.ContactObject
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return rpcapi.ContactListResponse{}, err
		}
		items = append(items, item)
	}
	return rpcapi.ContactListResponse{Items: items, HasNext: entries.hasNext, NextCursor: entries.nextCursor}, nil
}

func (s *Server) GetContact(ctx context.Context, owner string, req rpcapi.ContactGetRequest) (rpcapi.ContactObject, error) {
	store, err := s.contactsStore()
	if err != nil {
		return rpcapi.ContactObject{}, err
	}
	return readJSONValue[rpcapi.ContactObject](ctx, store, contactKey(owner, req.Id))
}

func (s *Server) CreateContact(ctx context.Context, owner string, req rpcapi.ContactCreateRequest) (rpcapi.ContactObject, error) {
	store, err := s.contactsStore()
	if err != nil {
		return rpcapi.ContactObject{}, err
	}
	if err := requireOwner(owner); err != nil {
		return rpcapi.ContactObject{}, err
	}
	displayName := strings.TrimSpace(stringValue(req.DisplayName))
	phoneNumber := strings.TrimSpace(stringValue(req.PhoneNumber))
	if displayName == "" && phoneNumber == "" {
		return rpcapi.ContactObject{}, errors.New("social: contact display_name or phone_number is required")
	}
	if phoneNumber != "" {
		if err := s.ensureUniqueContactPhone(ctx, owner, "", phoneNumber); err != nil {
			return rpcapi.ContactObject{}, err
		}
	}
	now := s.now()
	id := s.newID()
	item := rpcapi.ContactObject{Id: &id, CreatedAt: &now, UpdatedAt: &now}
	if displayName != "" {
		item.DisplayName = &displayName
	}
	if phoneNumber != "" {
		item.PhoneNumber = &phoneNumber
	}
	return item, writeJSON(ctx, store, contactKey(owner, id), item)
}

func (s *Server) PutContact(ctx context.Context, owner string, req rpcapi.ContactPutRequest) (rpcapi.ContactObject, error) {
	store, err := s.contactsStore()
	if err != nil {
		return rpcapi.ContactObject{}, err
	}
	item, err := readJSONValue[rpcapi.ContactObject](ctx, store, contactKey(owner, req.Id))
	if err != nil {
		return rpcapi.ContactObject{}, err
	}
	displayName := strings.TrimSpace(stringValue(item.DisplayName))
	phoneNumber := strings.TrimSpace(stringValue(item.PhoneNumber))
	if req.DisplayName != nil {
		displayName = strings.TrimSpace(*req.DisplayName)
	}
	if req.PhoneNumber != nil {
		phoneNumber = strings.TrimSpace(*req.PhoneNumber)
		if phoneNumber != "" {
			if err := s.ensureUniqueContactPhone(ctx, owner, req.Id, phoneNumber); err != nil {
				return rpcapi.ContactObject{}, err
			}
		}
	}
	if displayName == "" && phoneNumber == "" {
		return rpcapi.ContactObject{}, errors.New("social: contact display_name or phone_number is required")
	}
	item.DisplayName = optionalString(displayName)
	item.PhoneNumber = optionalString(phoneNumber)
	now := s.now()
	item.UpdatedAt = &now
	return item, writeJSON(ctx, store, contactKey(owner, req.Id), item)
}

func (s *Server) DeleteContact(ctx context.Context, owner string, req rpcapi.ContactDeleteRequest) (rpcapi.ContactObject, error) {
	store, err := s.contactsStore()
	if err != nil {
		return rpcapi.ContactObject{}, err
	}
	item, err := readJSONValue[rpcapi.ContactObject](ctx, store, contactKey(owner, req.Id))
	if err != nil {
		return rpcapi.ContactObject{}, err
	}
	return item, store.Delete(ctx, contactKey(owner, req.Id))
}

func (s *Server) CreateFriendRequest(ctx context.Context, owner string, req rpcapi.FriendRequestCreateRequest) (rpcapi.FriendRequestObject, error) {
	store, err := s.friendRequestsStore()
	if err != nil {
		return rpcapi.FriendRequestObject{}, err
	}
	owner = strings.TrimSpace(owner)
	to := strings.TrimSpace(req.ToPeerId)
	if owner == "" || to == "" {
		return rpcapi.FriendRequestObject{}, errors.New("social: friend request peers are required")
	}
	if owner == to {
		return rpcapi.FriendRequestObject{}, errors.New("social: cannot friend self")
	}
	if _, err := s.getFriendRelation(ctx, owner, relationID(owner, to)); err == nil {
		return rpcapi.FriendRequestObject{}, errors.New("social: peers are already friends")
	} else if !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendRequestObject{}, err
	}
	if existing, ok, err := s.pendingFriendRequest(ctx, owner, to); err != nil {
		return rpcapi.FriendRequestObject{}, err
	} else if ok {
		return existing, nil
	}
	if err := s.consumeFriendOTP(ctx, to, req.Code); err != nil {
		return rpcapi.FriendRequestObject{}, err
	}
	now := s.now()
	id := s.newID()
	state := rpcapi.FriendRequestStatePending
	item := rpcapi.FriendRequestObject{
		Id:         &id,
		FromPeerId: &owner,
		ToPeerId:   &to,
		Message:    optionalString(strings.TrimSpace(stringValue(req.Message))),
		State:      &state,
		CreatedAt:  &now,
		UpdatedAt:  &now,
	}
	return item, writeJSON(ctx, store, friendRequestKey(id), item)
}

func (s *Server) ListFriendRequests(ctx context.Context, owner string, req rpcapi.FriendRequestListRequest) (rpcapi.FriendRequestListResponse, error) {
	store, err := s.friendRequestsStore()
	if err != nil {
		return rpcapi.FriendRequestListResponse{}, err
	}
	box := "all"
	if req.Box != nil && string(*req.Box) != "" {
		if !req.Box.Valid() {
			return rpcapi.FriendRequestListResponse{}, errors.New("social: invalid friend request box")
		}
		box = string(*req.Box)
	}
	if req.State != nil && !req.State.Valid() {
		return rpcapi.FriendRequestListResponse{}, errors.New("social: invalid friend request state")
	}
	items := make([]rpcapi.FriendRequestObject, 0)
	for entry, err := range store.List(ctx, friendRequestsRoot) {
		if err != nil {
			return rpcapi.FriendRequestListResponse{}, err
		}
		var item rpcapi.FriendRequestObject
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return rpcapi.FriendRequestListResponse{}, err
		}
		if !friendRequestVisible(item, owner, box) {
			continue
		}
		if req.State != nil && (item.State == nil || *item.State != *req.State) {
			continue
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return compareByCreatedAtAsc(timeValue(items[i].CreatedAt), stringValue(items[i].Id), timeValue(items[j].CreatedAt), stringValue(items[j].Id))
	})
	page := pageItems(items, stringValue(req.Cursor), intValue(req.Limit), func(item rpcapi.FriendRequestObject) string {
		return stringValue(item.Id)
	})
	return rpcapi.FriendRequestListResponse{Items: page.items, HasNext: page.hasNext, NextCursor: page.nextCursor}, nil
}

func (s *Server) AcceptFriendRequest(ctx context.Context, owner string, req rpcapi.FriendRequestAcceptRequest) (rpcapi.FriendRequestObject, error) {
	return s.transitionFriendRequest(ctx, owner, req.Id, rpcapi.FriendRequestStateAccepted)
}

func (s *Server) RejectFriendRequest(ctx context.Context, owner string, req rpcapi.FriendRequestRejectRequest) (rpcapi.FriendRequestObject, error) {
	return s.transitionFriendRequest(ctx, owner, req.Id, rpcapi.FriendRequestStateRejected)
}

func (s *Server) ListFriends(ctx context.Context, owner string, req rpcapi.FriendListRequest) (rpcapi.FriendListResponse, error) {
	store, err := s.friendsStore()
	if err != nil {
		return rpcapi.FriendListResponse{}, err
	}
	entries, err := listPage(ctx, store, ownerPrefix(friendsRoot, owner), stringValue(req.Cursor), intValue(req.Limit))
	if err != nil {
		return rpcapi.FriendListResponse{}, err
	}
	items := make([]rpcapi.FriendObject, 0, len(entries.items))
	for _, entry := range entries.items {
		var item rpcapi.FriendObject
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return rpcapi.FriendListResponse{}, err
		}
		items = append(items, item)
	}
	return rpcapi.FriendListResponse{Items: items, HasNext: entries.hasNext, NextCursor: entries.nextCursor}, nil
}

func (s *Server) DeleteFriend(ctx context.Context, owner string, req rpcapi.FriendDeleteRequest) (rpcapi.FriendObject, error) {
	store, err := s.friendsStore()
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	item, err := s.getFriendRelation(ctx, owner, req.Id)
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	other := stringValue(item.PeerId)
	if err := store.BatchDelete(ctx, []kv.Key{friendKey(owner, req.Id), friendKey(other, req.Id)}); err != nil {
		return rpcapi.FriendObject{}, err
	}
	return item, nil
}

func (s *Server) CreateFriendGroup(ctx context.Context, owner string, req rpcapi.FriendGroupCreateRequest) (rpcapi.FriendGroupObject, error) {
	friendGroups, members, err := s.groupStores()
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
		Description:     optionalString(strings.TrimSpace(stringValue(req.Description))),
		CreatedByPeerId: &owner,
		CreatedAt:       &now,
		UpdatedAt:       &now,
	}
	if err := writeJSON(ctx, friendGroups, groupKey(id), group); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	role := rpcapi.FriendGroupMemberRoleOwner
	member := rpcapi.FriendGroupMemberObject{Id: &owner, FriendGroupId: &id, PeerId: &owner, Role: &role, CreatedAt: &now, UpdatedAt: &now}
	if err := writeJSON(ctx, members, groupMemberKey(id, owner), member); err != nil {
		_ = friendGroups.Delete(ctx, groupKey(id))
		return rpcapi.FriendGroupObject{}, err
	}
	if err := s.upsertFriendGroupACLBinding(ctx, id, owner, role); err != nil {
		_ = members.Delete(ctx, groupMemberKey(id, owner))
		_ = friendGroups.Delete(ctx, groupKey(id))
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
	if err := s.requireFriendGroupRead(ctx, owner, friendGroupID); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	return readJSONValue[rpcapi.FriendGroupObject](ctx, store, groupKey(friendGroupID))
}

func (s *Server) ListFriendGroups(ctx context.Context, owner string, req rpcapi.FriendGroupListRequest) (rpcapi.FriendGroupListResponse, error) {
	store, err := s.groupsStore()
	if err != nil {
		return rpcapi.FriendGroupListResponse{}, err
	}
	items := make([]rpcapi.FriendGroupObject, 0)
	for entry, err := range store.List(ctx, groupsRoot) {
		if err != nil {
			return rpcapi.FriendGroupListResponse{}, err
		}
		var item rpcapi.FriendGroupObject
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return rpcapi.FriendGroupListResponse{}, err
		}
		if _, err := s.groupMember(ctx, stringValue(item.Id), owner); err == nil {
			items = append(items, item)
		} else if !errors.Is(err, kv.ErrNotFound) {
			return rpcapi.FriendGroupListResponse{}, err
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return compareByCreatedAtAsc(timeValue(items[i].CreatedAt), stringValue(items[i].Id), timeValue(items[j].CreatedAt), stringValue(items[j].Id))
	})
	page := pageItems(items, stringValue(req.Cursor), intValue(req.Limit), func(item rpcapi.FriendGroupObject) string {
		return stringValue(item.Id)
	})
	return rpcapi.FriendGroupListResponse{Items: page.items, HasNext: page.hasNext, NextCursor: page.nextCursor}, nil
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
	if err := s.requireFriendGroupRole(ctx, owner, friendGroupID, rpcapi.FriendGroupMemberRoleOwner); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	group, err := readJSONValue[rpcapi.FriendGroupObject](ctx, store, groupKey(friendGroupID))
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
		group.Description = optionalString(strings.TrimSpace(*req.Description))
	}
	now := s.now()
	group.UpdatedAt = &now
	return group, writeJSON(ctx, store, groupKey(friendGroupID), group)
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
	if err := s.requireFriendGroupRole(ctx, owner, friendGroupID, rpcapi.FriendGroupMemberRoleOwner); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	group, err := readJSONValue[rpcapi.FriendGroupObject](ctx, friendGroups, groupKey(friendGroupID))
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	var members []rpcapi.FriendGroupMemberObject
	if s.ACL != nil {
		members, err = s.listAllFriendGroupMembers(ctx, friendGroupID)
		if err != nil {
			return rpcapi.FriendGroupObject{}, err
		}
	}
	if s.MessageAssets != nil {
		if err := s.MessageAssets.DeletePrefix(escapeStoreSegment(friendGroupID)); err != nil {
			return rpcapi.FriendGroupObject{}, err
		}
	}
	if s.FriendGroupMembers != nil {
		if err := deletePrefix(ctx, s.FriendGroupMembers, append(groupMembersRoot, escapeStoreSegment(friendGroupID))); err != nil {
			return rpcapi.FriendGroupObject{}, err
		}
	}
	if s.FriendGroupMessages != nil {
		if err := deletePrefix(ctx, s.FriendGroupMessages, append(groupMessagesRoot, escapeStoreSegment(friendGroupID))); err != nil {
			return rpcapi.FriendGroupObject{}, err
		}
	}
	if err := s.deleteFriendGroupACLBindings(ctx, friendGroupID, members); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if err := friendGroups.Delete(ctx, groupKey(friendGroupID)); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	return group, nil
}

func (s *Server) AddFriendGroupMember(ctx context.Context, owner string, req rpcapi.FriendGroupMemberAddRequest) (rpcapi.FriendGroupMemberObject, error) {
	req.FriendGroupId = strings.TrimSpace(req.FriendGroupId)
	req.PeerId = strings.TrimSpace(req.PeerId)
	store, err := s.groupMembersStore()
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if req.Role == rpcapi.FriendGroupMemberMutableRole("admin") {
		if err := s.requireFriendGroupRole(ctx, owner, req.FriendGroupId, rpcapi.FriendGroupMemberRoleOwner); err != nil {
			return rpcapi.FriendGroupMemberObject{}, err
		}
	} else if err := s.requireFriendGroupAdmin(ctx, owner, req.FriendGroupId); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	current, currentErr := s.groupMember(ctx, req.FriendGroupId, req.PeerId)
	if currentErr != nil && !errors.Is(currentErr, kv.ErrNotFound) {
		return rpcapi.FriendGroupMemberObject{}, currentErr
	}
	member, err := s.writeFriendGroupMember(ctx, req.FriendGroupId, req.PeerId, rpcapi.FriendGroupMemberRole(req.Role))
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := s.upsertFriendGroupACLBinding(ctx, req.FriendGroupId, req.PeerId, groupRole(member)); err != nil {
		if currentErr == nil {
			_ = writeJSON(ctx, store, groupMemberKey(req.FriendGroupId, req.PeerId), current)
		} else {
			_ = store.Delete(ctx, groupMemberKey(req.FriendGroupId, req.PeerId))
		}
		return rpcapi.FriendGroupMemberObject{}, err
	}
	return member, nil
}

func (s *Server) PutFriendGroupMember(ctx context.Context, owner string, req rpcapi.FriendGroupMemberPutRequest) (rpcapi.FriendGroupMemberObject, error) {
	req.FriendGroupId = strings.TrimSpace(req.FriendGroupId)
	req.Id = strings.TrimSpace(req.Id)
	store, err := s.groupMembersStore()
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := s.requireFriendGroupRole(ctx, owner, req.FriendGroupId, rpcapi.FriendGroupMemberRoleOwner); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	current, err := s.groupMember(ctx, req.FriendGroupId, req.Id)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if current.Role != nil && *current.Role == rpcapi.FriendGroupMemberRoleOwner {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: cannot change owner role")
	}
	member, err := s.writeFriendGroupMember(ctx, req.FriendGroupId, req.Id, rpcapi.FriendGroupMemberRole(req.Role))
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := s.upsertFriendGroupACLBinding(ctx, req.FriendGroupId, req.Id, groupRole(member)); err != nil {
		_ = writeJSON(ctx, store, groupMemberKey(req.FriendGroupId, req.Id), current)
		return rpcapi.FriendGroupMemberObject{}, err
	}
	return member, nil
}

func (s *Server) DeleteFriendGroupMember(ctx context.Context, owner string, req rpcapi.FriendGroupMemberDeleteRequest) (rpcapi.FriendGroupMemberObject, error) {
	req.FriendGroupId = strings.TrimSpace(req.FriendGroupId)
	req.Id = strings.TrimSpace(req.Id)
	store, err := s.groupMembersStore()
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	current, err := s.groupMember(ctx, req.FriendGroupId, req.Id)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	role := groupRole(current)
	switch role {
	case rpcapi.FriendGroupMemberRoleOwner:
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: cannot delete friend group owner")
	case rpcapi.FriendGroupMemberRoleAdmin:
		if err := s.requireFriendGroupRole(ctx, owner, req.FriendGroupId, rpcapi.FriendGroupMemberRoleOwner); err != nil {
			return rpcapi.FriendGroupMemberObject{}, err
		}
	default:
		if owner != req.Id {
			if err := s.requireFriendGroupAdmin(ctx, owner, req.FriendGroupId); err != nil {
				return rpcapi.FriendGroupMemberObject{}, err
			}
		}
	}
	if err := store.Delete(ctx, groupMemberKey(req.FriendGroupId, req.Id)); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := s.deleteFriendGroupACLBinding(ctx, req.FriendGroupId, req.Id); err != nil {
		_ = writeJSON(ctx, store, groupMemberKey(req.FriendGroupId, req.Id), current)
		return rpcapi.FriendGroupMemberObject{}, err
	}
	return current, nil
}

func (s *Server) ListFriendGroupMembers(ctx context.Context, owner string, req rpcapi.FriendGroupMemberListRequest) (rpcapi.FriendGroupMemberListResponse, error) {
	if err := s.requireFriendGroupRead(ctx, owner, stringValue(req.FriendGroupId)); err != nil {
		return rpcapi.FriendGroupMemberListResponse{}, err
	}
	store, err := s.groupMembersStore()
	if err != nil {
		return rpcapi.FriendGroupMemberListResponse{}, err
	}
	entries, err := listPage(ctx, store, append(groupMembersRoot, escapeStoreSegment(stringValue(req.FriendGroupId))), stringValue(req.Cursor), intValue(req.Limit))
	if err != nil {
		return rpcapi.FriendGroupMemberListResponse{}, err
	}
	items := make([]rpcapi.FriendGroupMemberObject, 0, len(entries.items))
	for _, entry := range entries.items {
		var item rpcapi.FriendGroupMemberObject
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return rpcapi.FriendGroupMemberListResponse{}, err
		}
		items = append(items, item)
	}
	return rpcapi.FriendGroupMemberListResponse{Items: items, HasNext: entries.hasNext, NextCursor: entries.nextCursor}, nil
}

func (s *Server) SendFriendGroupMessage(ctx context.Context, owner string, req rpcapi.FriendGroupMessageSendRequest) (rpcapi.FriendGroupMessageObject, error) {
	store, err := s.groupMessagesStore()
	if err != nil {
		return rpcapi.FriendGroupMessageObject{}, err
	}
	if s.MessageAssets == nil {
		return rpcapi.FriendGroupMessageObject{}, errors.New("social: friend group message asset store not configured")
	}
	req.FriendGroupId = strings.TrimSpace(req.FriendGroupId)
	if err := s.requireFriendGroupUse(ctx, owner, req.FriendGroupId); err != nil {
		return rpcapi.FriendGroupMessageObject{}, err
	}
	if req.AudioContentType != defaultAudioContentType {
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
	path := escapeStoreSegment(req.FriendGroupId) + "/" + escapeStoreSegment(id) + ".opus"
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
	if err := writeJSON(ctx, store, groupMessageKey(req.FriendGroupId, id), item); err != nil {
		_ = s.MessageAssets.Delete(path)
		return rpcapi.FriendGroupMessageObject{}, err
	}
	return item, nil
}

func (s *Server) GetFriendGroupMessage(ctx context.Context, owner string, req rpcapi.FriendGroupMessageGetRequest) (rpcapi.FriendGroupMessageObject, error) {
	req.FriendGroupId = strings.TrimSpace(req.FriendGroupId)
	req.Id = strings.TrimSpace(req.Id)
	if err := s.requireFriendGroupRead(ctx, owner, req.FriendGroupId); err != nil {
		return rpcapi.FriendGroupMessageObject{}, err
	}
	store, err := s.groupMessagesStore()
	if err != nil {
		return rpcapi.FriendGroupMessageObject{}, err
	}
	item, err := readJSONValue[rpcapi.FriendGroupMessageObject](ctx, store, groupMessageKey(req.FriendGroupId, req.Id))
	if err != nil {
		return rpcapi.FriendGroupMessageObject{}, err
	}
	if messageExpired(item, s.now()) {
		return rpcapi.FriendGroupMessageObject{}, kv.ErrNotFound
	}
	return item, nil
}

func (s *Server) ListFriendGroupMessages(ctx context.Context, owner string, req rpcapi.FriendGroupMessageListRequest) (rpcapi.FriendGroupMessageListResponse, error) {
	if req.FriendGroupId != nil {
		v := strings.TrimSpace(*req.FriendGroupId)
		req.FriendGroupId = &v
	}
	if err := s.requireFriendGroupRead(ctx, owner, stringValue(req.FriendGroupId)); err != nil {
		return rpcapi.FriendGroupMessageListResponse{}, err
	}
	store, err := s.groupMessagesStore()
	if err != nil {
		return rpcapi.FriendGroupMessageListResponse{}, err
	}
	items := make([]rpcapi.FriendGroupMessageObject, 0)
	for entry, err := range store.List(ctx, append(groupMessagesRoot, escapeStoreSegment(stringValue(req.FriendGroupId)))) {
		if err != nil {
			return rpcapi.FriendGroupMessageListResponse{}, err
		}
		var item rpcapi.FriendGroupMessageObject
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return rpcapi.FriendGroupMessageListResponse{}, err
		}
		if !messageExpired(item, s.now()) {
			items = append(items, item)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return compareByCreatedAtDesc(timeValue(items[i].CreatedAt), stringValue(items[i].Id), timeValue(items[j].CreatedAt), stringValue(items[j].Id))
	})
	page := pageItems(items, stringValue(req.Cursor), intValue(req.Limit), func(item rpcapi.FriendGroupMessageObject) string {
		return stringValue(item.Id)
	})
	return rpcapi.FriendGroupMessageListResponse{Items: page.items, HasNext: page.hasNext, NextCursor: page.nextCursor}, nil
}

func (s *Server) CleanupExpiredFriendGroupMessages(ctx context.Context) error {
	if s.FriendGroupMessages == nil {
		return errors.New("social: friend group message store not configured")
	}
	now := s.now()
	var deleteKeys []kv.Key
	var deleteObjects []string
	for entry, err := range s.FriendGroupMessages.List(ctx, groupMessagesRoot) {
		if err != nil {
			return err
		}
		var item rpcapi.FriendGroupMessageObject
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return err
		}
		if messageExpired(item, now) {
			deleteKeys = append(deleteKeys, entry.Key)
			if item.AudioPath != nil {
				deleteObjects = append(deleteObjects, *item.AudioPath)
			}
		}
	}
	if len(deleteKeys) > 0 {
		if err := s.FriendGroupMessages.BatchDelete(ctx, deleteKeys); err != nil {
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

type entryPage struct {
	items      []kv.Entry
	hasNext    bool
	nextCursor *string
}

type itemPage[T any] struct {
	items      []T
	hasNext    bool
	nextCursor *string
}

func listPage(ctx context.Context, store kv.Store, prefix kv.Key, cursor string, limit int) (entryPage, error) {
	cursor, limit = normalizeListParams(cursor, limit)
	entries, err := kv.ListAfter(ctx, store, prefix, cursorAfterKey(prefix, cursor), limit+1)
	if err != nil {
		return entryPage{}, err
	}
	hasNext := len(entries) > limit
	if hasNext {
		entries = entries[:limit]
	}
	var next *string
	if hasNext && len(entries) > 0 {
		v := unescapeStoreSegment(entries[len(entries)-1].Key[len(entries[len(entries)-1].Key)-1])
		next = &v
	}
	return entryPage{items: entries, hasNext: hasNext, nextCursor: next}, nil
}

func pageItems[T any](items []T, cursor string, limit int, id func(T) string) itemPage[T] {
	cursor = strings.TrimSpace(cursor)
	_, limit = normalizeListParams("", limit)
	start := 0
	if cursor != "" {
		for i, item := range items {
			if id(item) == cursor {
				start = i + 1
				break
			}
		}
	}
	if start > len(items) {
		start = len(items)
	}
	end := start + limit
	hasNext := end < len(items)
	if end > len(items) {
		end = len(items)
	}
	var next *string
	if hasNext && end > start {
		v := id(items[end-1])
		next = &v
	}
	return itemPage[T]{items: items[start:end], hasNext: hasNext, nextCursor: next}
}

func (s *Server) transitionFriendRequest(ctx context.Context, owner, id string, next rpcapi.FriendRequestState) (rpcapi.FriendRequestObject, error) {
	store, err := s.friendRequestsStore()
	if err != nil {
		return rpcapi.FriendRequestObject{}, err
	}
	item, err := readJSONValue[rpcapi.FriendRequestObject](ctx, store, friendRequestKey(id))
	if err != nil {
		return rpcapi.FriendRequestObject{}, err
	}
	if stringValue(item.ToPeerId) != owner {
		return rpcapi.FriendRequestObject{}, errors.New("social: only receiver can transition friend request")
	}
	if next == rpcapi.FriendRequestStateAccepted && item.State != nil && *item.State == rpcapi.FriendRequestStateAccepted {
		return item, nil
	}
	if item.State == nil || *item.State != rpcapi.FriendRequestStatePending {
		return rpcapi.FriendRequestObject{}, errors.New("social: friend request is not pending")
	}
	now := s.now()
	item.State = &next
	item.UpdatedAt = &now
	item.RespondedAt = &now
	if next == rpcapi.FriendRequestStateAccepted {
		if err := s.createFriendRows(ctx, item); err != nil {
			return rpcapi.FriendRequestObject{}, err
		}
	}
	if err := writeJSON(ctx, store, friendRequestKey(id), item); err != nil {
		if next == rpcapi.FriendRequestStateAccepted {
			rollbackErr := s.deleteFriendRows(ctx, item)
			return rpcapi.FriendRequestObject{}, errors.Join(err, rollbackErr)
		}
		return rpcapi.FriendRequestObject{}, err
	}
	return item, nil
}

func (s *Server) createFriendRows(ctx context.Context, req rpcapi.FriendRequestObject) error {
	store, err := s.friendsStore()
	if err != nil {
		return err
	}
	from, to, requestID := stringValue(req.FromPeerId), stringValue(req.ToPeerId), stringValue(req.Id)
	rel := relationID(from, to)
	now := s.now()
	entries := make([]kv.Entry, 0, 2)
	for _, row := range []struct{ owner, peer string }{{from, to}, {to, from}} {
		item := rpcapi.FriendObject{Id: &rel, PeerId: &row.peer, RequestId: &requestID, CreatedAt: &now, UpdatedAt: &now}
		data, err := json.Marshal(item)
		if err != nil {
			return err
		}
		entries = append(entries, kv.Entry{Key: friendKey(row.owner, rel), Value: data})
	}
	return store.BatchSet(ctx, entries)
}

func (s *Server) deleteFriendRows(ctx context.Context, req rpcapi.FriendRequestObject) error {
	store, err := s.friendsStore()
	if err != nil {
		return err
	}
	from, to := stringValue(req.FromPeerId), stringValue(req.ToPeerId)
	rel := relationID(from, to)
	return store.BatchDelete(ctx, []kv.Key{friendKey(from, rel), friendKey(to, rel)})
}

func (s *Server) consumeFriendOTP(ctx context.Context, peerID, code string) error {
	if !isSixDigitCode(code) {
		return errors.New("social: friend otp code must be exactly 6 digits")
	}
	store, err := s.friendRequestsStore()
	if err != nil {
		return err
	}
	record, err := readJSONValue[friendOTPRecord](ctx, store, friendOTPKey(peerID))
	if err != nil {
		return errors.New("social: friend otp not found")
	}
	if record.Consumed || !record.ExpiresAt.After(s.now()) || record.CodeHash != hashCode(code) {
		return errors.New("social: invalid friend otp")
	}
	record.Consumed = true
	return writeJSON(ctx, store, friendOTPKey(peerID), record)
}

func (s *Server) pendingFriendRequest(ctx context.Context, from, to string) (rpcapi.FriendRequestObject, bool, error) {
	store, err := s.friendRequestsStore()
	if err != nil {
		return rpcapi.FriendRequestObject{}, false, err
	}
	for entry, err := range store.List(ctx, friendRequestsRoot) {
		if err != nil {
			return rpcapi.FriendRequestObject{}, false, err
		}
		var item rpcapi.FriendRequestObject
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return rpcapi.FriendRequestObject{}, false, err
		}
		if stringValue(item.FromPeerId) == from && stringValue(item.ToPeerId) == to && item.State != nil && *item.State == rpcapi.FriendRequestStatePending {
			return item, true, nil
		}
	}
	return rpcapi.FriendRequestObject{}, false, nil
}

func (s *Server) ensureUniqueContactPhone(ctx context.Context, owner, currentID, phone string) error {
	if phone == "" {
		return nil
	}
	store, err := s.contactsStore()
	if err != nil {
		return err
	}
	normalized := normalizePhone(phone)
	for entry, err := range store.List(ctx, ownerPrefix(contactsRoot, owner)) {
		if err != nil {
			return err
		}
		var item rpcapi.ContactObject
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return err
		}
		if stringValue(item.Id) != currentID && normalizePhone(stringValue(item.PhoneNumber)) == normalized {
			return errors.New("social: contact phone_number already exists")
		}
	}
	return nil
}

func (s *Server) writeFriendGroupMember(ctx context.Context, friendGroupID, peerID string, role rpcapi.FriendGroupMemberRole) (rpcapi.FriendGroupMemberObject, error) {
	store, err := s.groupMembersStore()
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if !role.Valid() || role == rpcapi.FriendGroupMemberRoleOwner {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: invalid group member role")
	}
	now := s.now()
	current, err := readJSONValue[rpcapi.FriendGroupMemberObject](ctx, store, groupMemberKey(friendGroupID, peerID))
	if err == nil && current.CreatedAt != nil {
		nowCreated := *current.CreatedAt
		current.Role = &role
		current.UpdatedAt = &now
		current.CreatedAt = &nowCreated
		return current, writeJSON(ctx, store, groupMemberKey(friendGroupID, peerID), current)
	}
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	item := rpcapi.FriendGroupMemberObject{Id: &peerID, FriendGroupId: &friendGroupID, PeerId: &peerID, Role: &role, CreatedAt: &now, UpdatedAt: &now}
	return item, writeJSON(ctx, store, groupMemberKey(friendGroupID, peerID), item)
}

func (s *Server) requireFriendGroupRead(ctx context.Context, owner, friendGroupID string) error {
	if _, err := s.groupMember(ctx, friendGroupID, owner); err != nil {
		return err
	}
	return s.authorizeFriendGroup(ctx, owner, friendGroupID, apitypes.ACLPermissionFriendGroupRead)
}

func (s *Server) requireFriendGroupUse(ctx context.Context, owner, friendGroupID string) error {
	if _, err := s.groupMember(ctx, friendGroupID, owner); err != nil {
		return err
	}
	return s.authorizeFriendGroup(ctx, owner, friendGroupID, apitypes.ACLPermissionFriendGroupUse)
}

func (s *Server) requireFriendGroupAdmin(ctx context.Context, owner, friendGroupID string) error {
	member, err := s.groupMember(ctx, friendGroupID, owner)
	if err != nil {
		return err
	}
	role := groupRole(member)
	if role != rpcapi.FriendGroupMemberRoleOwner && role != rpcapi.FriendGroupMemberRoleAdmin {
		return errors.New("social: friend group admin required")
	}
	return s.authorizeFriendGroup(ctx, owner, friendGroupID, apitypes.ACLPermissionFriendGroupAdmin)
}

func (s *Server) requireFriendGroupRole(ctx context.Context, owner, friendGroupID string, required rpcapi.FriendGroupMemberRole) error {
	member, err := s.groupMember(ctx, friendGroupID, owner)
	if err != nil {
		return err
	}
	if groupRole(member) != required {
		return fmt.Errorf("social: friend group role %s required", required)
	}
	if required == rpcapi.FriendGroupMemberRoleOwner {
		return s.authorizeFriendGroup(ctx, owner, friendGroupID, apitypes.ACLPermissionFriendGroupAdmin)
	}
	return nil
}

func (s *Server) authorizeFriendGroup(ctx context.Context, owner, friendGroupID string, permission apitypes.ACLPermission) error {
	if s == nil || s.ACL == nil {
		return nil
	}
	return s.ACL.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject(strings.TrimSpace(owner)),
		Resource:   acl.FriendGroupResource(strings.TrimSpace(friendGroupID)),
		Permission: permission,
	})
}

func (s *Server) upsertFriendGroupACLBinding(ctx context.Context, friendGroupID, peerID string, role rpcapi.FriendGroupMemberRole) error {
	if s == nil || s.ACL == nil {
		return nil
	}
	roleName, permissions, err := groupACLRole(role)
	if err != nil {
		return err
	}
	if _, err := s.ACL.PutRole(ctx, roleName, permissions); err != nil {
		return err
	}
	_, err = s.ACL.PutPolicyBinding(ctx, groupACLBindingID(friendGroupID, peerID), 0, apitypes.ACLPolicy{
		Subject:  acl.PublicKeySubject(strings.TrimSpace(peerID)),
		Resource: acl.FriendGroupResource(strings.TrimSpace(friendGroupID)),
		Role:     roleName,
	})
	return err
}

func (s *Server) deleteFriendGroupACLBinding(ctx context.Context, friendGroupID, peerID string) error {
	if s == nil || s.ACL == nil {
		return nil
	}
	if _, err := s.ACL.DeletePolicyBinding(ctx, groupACLBindingID(friendGroupID, peerID)); err != nil && !errors.Is(err, acl.ErrPolicyBindingNotFound) {
		return err
	}
	return nil
}

func (s *Server) deleteFriendGroupACLBindings(ctx context.Context, friendGroupID string, members []rpcapi.FriendGroupMemberObject) error {
	if s == nil || s.ACL == nil {
		return nil
	}
	for _, member := range members {
		if err := s.deleteFriendGroupACLBinding(ctx, friendGroupID, stringValue(member.PeerId)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) groupMember(ctx context.Context, friendGroupID, peerID string) (rpcapi.FriendGroupMemberObject, error) {
	store, err := s.groupMembersStore()
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	return readJSONValue[rpcapi.FriendGroupMemberObject](ctx, store, groupMemberKey(friendGroupID, peerID))
}

func (s *Server) listAllFriendGroupMembers(ctx context.Context, friendGroupID string) ([]rpcapi.FriendGroupMemberObject, error) {
	store, err := s.groupMembersStore()
	if err != nil {
		return nil, err
	}
	prefix := append(append(kv.Key{}, groupMembersRoot...), escapeStoreSegment(friendGroupID))
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

func (s *Server) getFriendRelation(ctx context.Context, owner, id string) (rpcapi.FriendObject, error) {
	store, err := s.friendsStore()
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	return readJSONValue[rpcapi.FriendObject](ctx, store, friendKey(owner, id))
}

func (s *Server) contactsStore() (kv.Store, error) {
	if s == nil || s.Contacts == nil {
		return nil, errors.New("social: contact service not configured")
	}
	return s.Contacts, nil
}

func (s *Server) friendRequestsStore() (kv.Store, error) {
	if s == nil || s.FriendRequests == nil {
		return nil, errors.New("social: friend request service not configured")
	}
	return s.FriendRequests, nil
}

func (s *Server) friendsStore() (kv.Store, error) {
	if s == nil || s.Friends == nil {
		return nil, errors.New("social: friend service not configured")
	}
	return s.Friends, nil
}

func (s *Server) groupsStore() (kv.Store, error) {
	if s == nil || s.FriendGroups == nil {
		return nil, errors.New("social: friend group service not configured")
	}
	return s.FriendGroups, nil
}

func (s *Server) groupMembersStore() (kv.Store, error) {
	if s == nil || s.FriendGroupMembers == nil {
		return nil, errors.New("social: group member service not configured")
	}
	return s.FriendGroupMembers, nil
}

func (s *Server) groupMessagesStore() (kv.Store, error) {
	if s == nil || s.FriendGroupMessages == nil {
		return nil, errors.New("social: friend group message service not configured")
	}
	return s.FriendGroupMessages, nil
}

func (s *Server) groupStores() (kv.Store, kv.Store, error) {
	friendGroups, err := s.groupsStore()
	if err != nil {
		return nil, nil, err
	}
	members, err := s.groupMembersStore()
	if err != nil {
		return nil, nil, err
	}
	return friendGroups, members, nil
}

func requireOwner(owner string) error {
	if strings.TrimSpace(owner) == "" {
		return errors.New("social: owner is required")
	}
	return nil
}

func normalizeListParams(cursor string, limit int) (string, int) {
	normalizedCursor := escapeStoreSegment(strings.TrimSpace(cursor))
	normalizedLimit := defaultListLimit
	if limit > 0 {
		normalizedLimit = limit
	}
	if normalizedLimit > maxListLimit {
		normalizedLimit = maxListLimit
	}
	return normalizedCursor, normalizedLimit
}

func cursorAfterKey(prefix kv.Key, cursor string) kv.Key {
	if cursor == "" {
		return nil
	}
	return append(append(kv.Key{}, prefix...), cursor)
}

func readJSONValue[T any](ctx context.Context, store kv.Store, key kv.Key) (T, error) {
	var out T
	data, err := store.Get(ctx, key)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

func writeJSON(ctx context.Context, store kv.Store, key kv.Key, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return store.Set(ctx, key, data)
}

func deletePrefix(ctx context.Context, store kv.Store, prefix kv.Key) error {
	var keys []kv.Key
	for entry, err := range store.List(ctx, prefix) {
		if err != nil {
			return err
		}
		keys = append(keys, entry.Key)
	}
	if len(keys) == 0 {
		return nil
	}
	return store.BatchDelete(ctx, keys)
}

func ownerPrefix(root kv.Key, owner string) kv.Key {
	return append(append(kv.Key{}, root...), escapeStoreSegment(strings.TrimSpace(owner)))
}

func contactKey(owner, id string) kv.Key {
	return append(ownerPrefix(contactsRoot, owner), escapeStoreSegment(id))
}

func friendRequestKey(id string) kv.Key {
	return append(append(kv.Key{}, friendRequestsRoot...), escapeStoreSegment(id))
}

func friendOTPKey(peerID string) kv.Key {
	return append(append(kv.Key{}, friendOTPRoot...), escapeStoreSegment(peerID))
}

func friendKey(owner, id string) kv.Key {
	return append(ownerPrefix(friendsRoot, owner), escapeStoreSegment(id))
}

func groupKey(id string) kv.Key {
	return append(append(kv.Key{}, groupsRoot...), escapeStoreSegment(id))
}

func groupMemberKey(friendGroupID, peerID string) kv.Key {
	return append(append(kv.Key{}, groupMembersRoot...), escapeStoreSegment(friendGroupID), escapeStoreSegment(peerID))
}

func groupMessageKey(friendGroupID, id string) kv.Key {
	return append(append(kv.Key{}, groupMessagesRoot...), escapeStoreSegment(friendGroupID), escapeStoreSegment(id))
}

func relationID(a, b string) string {
	parts := []string{strings.TrimSpace(a), strings.TrimSpace(b)}
	sort.Strings(parts)
	return parts[0] + ":" + parts[1]
}

func friendRequestVisible(item rpcapi.FriendRequestObject, owner, box string) bool {
	in := stringValue(item.ToPeerId) == owner
	out := stringValue(item.FromPeerId) == owner
	switch box {
	case "incoming":
		return in
	case "outgoing":
		return out
	default:
		return in || out
	}
}

func groupRole(member rpcapi.FriendGroupMemberObject) rpcapi.FriendGroupMemberRole {
	if member.Role == nil {
		return ""
	}
	return *member.Role
}

func groupACLRole(role rpcapi.FriendGroupMemberRole) (string, apitypes.ACLPermissionList, error) {
	switch role {
	case rpcapi.FriendGroupMemberRoleOwner:
		return groupOwnerRoleName, apitypes.ACLPermissionList{
			apitypes.ACLPermissionFriendGroupRead,
			apitypes.ACLPermissionFriendGroupUse,
			apitypes.ACLPermissionFriendGroupAdmin,
		}, nil
	case rpcapi.FriendGroupMemberRoleAdmin:
		return groupAdminRoleName, apitypes.ACLPermissionList{
			apitypes.ACLPermissionFriendGroupRead,
			apitypes.ACLPermissionFriendGroupUse,
			apitypes.ACLPermissionFriendGroupAdmin,
		}, nil
	case rpcapi.FriendGroupMemberRoleMember:
		return groupMemberRoleName, apitypes.ACLPermissionList{
			apitypes.ACLPermissionFriendGroupRead,
			apitypes.ACLPermissionFriendGroupUse,
		}, nil
	default:
		return "", nil, errors.New("social: invalid group member role")
	}
}

func groupACLBindingID(friendGroupID, peerID string) string {
	return "social-friend-group:" + escapeStoreSegment(strings.TrimSpace(friendGroupID)) + ":" + escapeStoreSegment(strings.TrimSpace(peerID))
}

func messageExpired(item rpcapi.FriendGroupMessageObject, now time.Time) bool {
	return item.ExpiresAt != nil && !item.ExpiresAt.After(now)
}

func timeValue(v *time.Time) time.Time {
	if v == nil {
		return time.Time{}
	}
	return *v
}

func compareByCreatedAtAsc(aTime time.Time, aID string, bTime time.Time, bID string) bool {
	if aTime.Equal(bTime) {
		return aID < bID
	}
	return aTime.Before(bTime)
}

func compareByCreatedAtDesc(aTime time.Time, aID string, bTime time.Time, bID string) bool {
	if aTime.Equal(bTime) {
		return aID > bID
	}
	return aTime.After(bTime)
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

func (s *Server) friendOTPTTL() time.Duration {
	if s != nil && s.FriendOTPTTL > 0 {
		return s.FriendOTPTTL
	}
	return defaultFriendOTPTTL
}

func (s *Server) messageDefaultTTL() time.Duration {
	if s != nil && s.MessageDefaultTTL > 0 {
		return s.MessageDefaultTTL
	}
	return defaultMessageTTL
}

func (s *Server) messageMaxTTL() time.Duration {
	if s != nil && s.MessageMaxTTL > 0 {
		return s.MessageMaxTTL
	}
	return defaultMessageMaxTTL
}

func (s *Server) messageMaxAudioBytes() int64 {
	if s != nil && s.MessageMaxAudioBytes > 0 {
		return s.MessageMaxAudioBytes
	}
	return defaultMaxAudioBytes
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
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func intValue(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func intPtr(v int) *int {
	return &v
}

func optionalString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func isSixDigitCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func hashCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func normalizePhone(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func escapeStoreSegment(value string) string {
	return url.QueryEscape(strings.TrimSpace(value))
}

func unescapeStoreSegment(value string) string {
	decoded, err := url.QueryUnescape(value)
	if err != nil {
		return value
	}
	return decoded
}
