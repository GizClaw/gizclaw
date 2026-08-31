package friendgroup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/internal/keyedlock"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type WorkspaceService interface {
	CreateSystemWorkspace(context.Context, adminhttp.WorkspaceUpsert) (apitypes.Workspace, bool, error)
	DeleteSystemWorkspace(context.Context, string) (apitypes.Workspace, error)
	GetRetiredSystemWorkspaceByID(context.Context, string, apitypes.ChatRoomMode, string) (apitypes.Workspace, error)
	RetireSystemWorkspaceByID(context.Context, string, apitypes.ChatRoomMode, string) (apitypes.Workspace, error)
}

type AssignmentService interface {
	Lookup(context.Context, giznet.PublicKey) (apitypes.PeerAssignment, error)
}

var ErrCrossServerFriendGroupMembership = errors.New("cross-server friend group membership is not supported")

type Server struct {
	Groups                 kv.Store
	InviteTokens           kv.Store
	Members                kv.Store
	Belongs                kv.Store
	Workspaces             WorkspaceService
	RuntimeProfileForOwner func(context.Context, string) (apitypes.RuntimeProfile, error)
	NotifyPeer             func(context.Context, string, *eventpb.PeerEvent)
	PeerAvailability       func(context.Context, string) error
	PeerAssignments        AssignmentService
	ServerPublicKey        giznet.PublicKey

	// RelationshipStore is the shared transaction boundary for Group,
	// membership, belongs, invite-token, and retirement-intent records.
	RelationshipStore        kv.Store
	GroupRelationshipPrefix  kv.Key
	InviteRelationshipPrefix kv.Key
	MemberRelationshipPrefix kv.Key
	BelongRelationshipPrefix kv.Key

	Now   func() time.Time
	NewID func() string
}

type PeerRetirementGroup struct {
	FriendGroupID   string                       `json:"friend_group_id"`
	WorkspaceID     string                       `json:"workspace_id"`
	WorkspaceName   string                       `json:"workspace_name"`
	OwnerPublicKey  string                       `json:"owner_public_key"`
	PeerPublicKey   string                       `json:"peer_public_key"`
	FriendGroupName string                       `json:"friend_group_name"`
	Role            rpcapi.FriendGroupMemberRole `json:"role"`
}

func (s *Server) SnapshotPeerGroups(ctx context.Context, peerID string) ([]PeerRetirementGroup, error) {
	if peerID == "" || peerID != strings.TrimSpace(peerID) {
		return nil, errors.New("social: Peer public key is required and must be canonical")
	}
	unlock, err := s.lockPeers(ctx, peerID)
	if err != nil {
		return nil, err
	}
	defer unlock()
	belongs, err := s.belongsStore()
	if err != nil {
		return nil, err
	}
	groups, err := s.groupsStore()
	if err != nil {
		return nil, err
	}
	prefix := append(append(kv.Key{}, socialutil.GroupBelongsRoot...), socialutil.EscapeStoreSegment(peerID))
	var out []PeerRetirementGroup
	for entry, err := range belongs.List(ctx, prefix) {
		if err != nil {
			return nil, err
		}
		var member friendGroupMemberRecord
		if err := json.Unmarshal(entry.Value, &member); err != nil {
			return nil, err
		}
		if err := member.validate(); err != nil || member.PeerPublicKey != peerID {
			return nil, errors.New("social: invalid Friend Group membership in Peer retirement snapshot")
		}
		group, err := socialutil.ReadJSONValue[rpcapi.FriendGroupObject](ctx, groups, socialutil.GroupKey(member.FriendGroupID))
		if err != nil {
			return nil, err
		}
		owner := socialutil.StringValue(group.CreatedByPeerPublicKey)
		if owner == "" || owner != strings.TrimSpace(owner) {
			return nil, errors.New("social: Friend Group has invalid owner")
		}
		binding, err := s.readWorkspaceBinding(ctx, member.FriendGroupID)
		if err != nil {
			return nil, err
		}
		out = append(out, PeerRetirementGroup{
			FriendGroupID: member.FriendGroupID, WorkspaceID: binding.WorkspaceID, WorkspaceName: binding.WorkspaceName,
			OwnerPublicKey: owner, PeerPublicKey: peerID, FriendGroupName: member.FriendGroupName, Role: member.Role,
		})
	}
	return out, nil
}

func (s *Server) RetirePeerGroup(ctx context.Context, snapshot PeerRetirementGroup) error {
	if snapshot.FriendGroupID == "" || snapshot.WorkspaceID == "" || snapshot.WorkspaceName == "" ||
		snapshot.OwnerPublicKey == "" || snapshot.PeerPublicKey == "" || snapshot.FriendGroupName == "" || !snapshot.Role.Valid() {
		return errors.New("social: invalid Friend Group Peer retirement snapshot")
	}
	groups, err := s.groupsStore()
	if err != nil {
		return err
	}
	group, err := socialutil.ReadJSONValue[rpcapi.FriendGroupObject](ctx, groups, socialutil.GroupKey(snapshot.FriendGroupID))
	if errors.Is(err, kv.ErrNotFound) {
		if snapshot.OwnerPublicKey != snapshot.PeerPublicKey {
			return errors.New("social: foreign Friend Group disappeared during Peer retirement")
		}
		receipt, receiptErr := s.readRetirementReceipt(ctx, snapshot.FriendGroupID)
		if receiptErr != nil {
			return receiptErr
		}
		if receipt.Owner != snapshot.OwnerPublicKey || receipt.Name != snapshot.FriendGroupName ||
			receipt.WorkspaceID != snapshot.WorkspaceID || receipt.WorkspaceName != snapshot.WorkspaceName {
			return errors.New("social: completed Friend Group retirement does not match snapshot")
		}
		return nil
	}
	if err != nil {
		return err
	}
	if socialutil.StringValue(group.CreatedByPeerPublicKey) != snapshot.OwnerPublicKey || socialutil.StringValue(group.WorkspaceName) != snapshot.WorkspaceName {
		return errors.New("social: Friend Group no longer matches Peer retirement snapshot")
	}
	binding, err := s.readWorkspaceBinding(ctx, snapshot.FriendGroupID)
	if err != nil {
		return err
	}
	if binding.WorkspaceID != snapshot.WorkspaceID || binding.WorkspaceName != snapshot.WorkspaceName {
		return errors.New("social: Friend Group Workspace no longer matches Peer retirement snapshot")
	}
	member, err := s.groupMember(ctx, snapshot.FriendGroupID, snapshot.PeerPublicKey)
	if errors.Is(err, kv.ErrNotFound) && snapshot.OwnerPublicKey != snapshot.PeerPublicKey {
		return nil
	}
	if err != nil {
		return err
	}
	if socialutil.StringValue(member.FriendGroupName) != snapshot.FriendGroupName || socialutil.GroupRole(member) != snapshot.Role {
		return errors.New("social: Friend Group membership no longer matches Peer retirement snapshot")
	}
	if snapshot.OwnerPublicKey == snapshot.PeerPublicKey {
		_, err = s.AdminDeleteFriendGroup(context.WithValue(ctx, peerRetirementContextKey{}, true), snapshot.FriendGroupID)
		return err
	}
	_, err = s.AdminDeleteFriendGroupMember(context.WithValue(ctx, peerRetirementContextKey{}, true), snapshot.FriendGroupID, snapshot.PeerPublicKey)
	return err
}

var (
	groupMutationMu   [64]sync.Mutex
	peerMutationGates keyedlock.Locker[string]
)

type peerRetirementContextKey struct{}

func peerRetirementFromContext(ctx context.Context) bool {
	value, _ := ctx.Value(peerRetirementContextKey{}).(bool)
	return value
}

type inviteTokenRecord struct {
	FriendGroupID string    `json:"friend_group_id"`
	InviteToken   string    `json:"invite_token"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

// friendGroupMemberRecord is the canonical persistence shape. The Peer RPC
// member object is projected from it and uses the peer public key as name.
type friendGroupMemberRecord struct {
	FriendGroupID   string                       `json:"friend_group_id"`
	PeerPublicKey   string                       `json:"peer_public_key"`
	FriendGroupName string                       `json:"friend_group_name"`
	Role            rpcapi.FriendGroupMemberRole `json:"role"`
	CreatedAt       time.Time                    `json:"created_at"`
	UpdatedAt       time.Time                    `json:"updated_at"`
}

func (record friendGroupMemberRecord) validate() error {
	if record.FriendGroupID == "" || record.FriendGroupID != strings.TrimSpace(record.FriendGroupID) ||
		record.PeerPublicKey == "" || record.PeerPublicKey != strings.TrimSpace(record.PeerPublicKey) ||
		record.FriendGroupName == "" || record.FriendGroupName != strings.TrimSpace(record.FriendGroupName) ||
		!record.Role.Valid() || record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() {
		return errors.New("social: persisted Friend Group member is invalid")
	}
	return nil
}

func (record friendGroupMemberRecord) peerObject() rpcapi.FriendGroupMemberObject {
	name := record.PeerPublicKey
	peerPublicKey := record.PeerPublicKey
	friendGroupName := record.FriendGroupName
	role := record.Role
	createdAt := record.CreatedAt
	updatedAt := record.UpdatedAt
	return rpcapi.FriendGroupMemberObject{
		Name:            name,
		PeerPublicKey:   &peerPublicKey,
		FriendGroupName: &friendGroupName,
		Role:            &role,
		CreatedAt:       &createdAt,
		UpdatedAt:       &updatedAt,
	}
}

func (s *Server) ensureGroupMutationAvailable(ctx context.Context, group rpcapi.FriendGroupObject, peerIDs ...string) error {
	if peerRetirementFromContext(ctx) || s == nil || s.PeerAvailability == nil {
		return nil
	}
	owner := socialutil.StringValue(group.CreatedByPeerPublicKey)
	if owner == "" || owner != strings.TrimSpace(owner) {
		return errors.New("social: Friend Group owner is invalid")
	}
	if err := s.PeerAvailability(ctx, owner); err != nil {
		return err
	}
	seen := map[string]struct{}{owner: {}}
	for _, publicKey := range peerIDs {
		if publicKey == "" || publicKey != strings.TrimSpace(publicKey) {
			return errors.New("social: Friend Group Peer public key must be canonical")
		}
		if _, ok := seen[publicKey]; ok {
			continue
		}
		seen[publicKey] = struct{}{}
		if err := s.PeerAvailability(ctx, publicKey); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) requireLocalPeers(ctx context.Context, peers ...string) error {
	if s == nil || s.PeerAssignments == nil || s.ServerPublicKey.IsZero() {
		return nil
	}
	for _, peerText := range peers {
		var publicKey giznet.PublicKey
		if err := publicKey.UnmarshalText([]byte(strings.TrimSpace(peerText))); err != nil || publicKey.IsZero() {
			return errors.New("social: invalid Peer public key")
		}
		assignment, err := s.PeerAssignments.Lookup(ctx, publicKey)
		if err != nil {
			return err
		}
		if assignment.ServerPublicKey != s.ServerPublicKey.String() {
			return ErrCrossServerFriendGroupMembership
		}
	}
	return nil
}

type peerMutationLockedContextKey struct{}

func peerMutationLocked(ctx context.Context) bool {
	locked, _ := ctx.Value(peerMutationLockedContextKey{}).(bool)
	return locked
}

func (s *Server) lockGroupPeers(ctx context.Context, group rpcapi.FriendGroupObject, peerIDs ...string) (context.Context, func(), error) {
	owner := strings.TrimSpace(socialutil.StringValue(group.CreatedByPeerPublicKey))
	peers := append([]string{owner}, peerIDs...)
	release, err := s.lockPeers(ctx, peers...)
	if err != nil {
		return ctx, nil, err
	}
	if err := s.ensureGroupMutationAvailable(ctx, group, peerIDs...); err != nil {
		release()
		return ctx, nil, err
	}
	return context.WithValue(ctx, peerMutationLockedContextKey{}, true), release, nil
}

func (s *Server) lockPeers(ctx context.Context, peers ...string) (func(), error) {
	keys := append([]string(nil), peers...)
	for index := range keys {
		keys[index] = strings.TrimSpace(keys[index])
	}
	slices.Sort(keys)
	keys = slices.Compact(keys)
	releases := make([]func(), 0, len(keys))
	for _, key := range keys {
		if key == "" {
			continue
		}
		release, err := peerMutationGates.Acquire(ctx, key)
		if err != nil {
			for index := len(releases) - 1; index >= 0; index-- {
				releases[index]()
			}
			return nil, err
		}
		releases = append(releases, release)
	}
	return func() {
		for index := len(releases) - 1; index >= 0; index-- {
			releases[index]()
		}
	}, nil
}

func (s *Server) lockRetirementPeers(ctx context.Context, intent retirementIntent) (context.Context, func(), error) {
	peers := make([]string, 0, len(intent.Members))
	for _, member := range intent.Members {
		peers = append(peers, member.PeerPublicKey)
	}
	return s.lockGroupPeers(ctx, intent.FriendGroup, peers...)
}

func friendGroupMemberRecordFromObject(friendGroupID string, item rpcapi.FriendGroupMemberObject) friendGroupMemberRecord {
	return friendGroupMemberRecord{
		FriendGroupID:   friendGroupID,
		PeerPublicKey:   socialutil.StringValue(item.PeerPublicKey),
		FriendGroupName: socialutil.StringValue(item.FriendGroupName),
		Role:            socialutil.GroupRole(item),
		CreatedAt:       socialutil.TimeValue(item.CreatedAt),
		UpdatedAt:       socialutil.TimeValue(item.UpdatedAt),
	}
}

type retirementIntent struct {
	FriendGroupID string                    `json:"friend_group_id"`
	FriendGroup   rpcapi.FriendGroupObject  `json:"friend_group"`
	Members       []friendGroupMemberRecord `json:"members"`
	WorkspaceID   string                    `json:"workspace_id"`
	WorkspaceName string                    `json:"workspace_name"`
	DeletedAt     time.Time                 `json:"deleted_at"`
}

type workspaceBinding struct {
	FriendGroupID string `json:"friend_group_id"`
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
}

type retirementReceipt struct {
	FriendGroupID string    `json:"friend_group_id"`
	Name          string    `json:"name,omitempty"`
	WorkspaceID   string    `json:"workspace_id"`
	WorkspaceName string    `json:"workspace_name"`
	Owner         string    `json:"owner"`
	DeletedAt     time.Time `json:"deleted_at"`
}

type retiredFriendGroupDataDescriptor struct {
	FriendGroupID      string   `json:"friend_group_id"`
	MessageStorePrefix []string `json:"message_store_prefix,omitempty"`
	MessageAssetPrefix string   `json:"message_asset_prefix,omitempty"`
}

var (
	retirementIntentsRoot  = kv.Key{"social-retirement-intents", "friend-groups"}
	retirementReceiptsRoot = kv.Key{"social-retirement-receipts", "friend-groups"}
	workspaceBindingsRoot  = kv.Key{"social-workspace-bindings", "friend-groups"}
)

var (
	errFriendGroupPendingDeletion     = errors.New("social: friend group is pending deletion")
	ErrFriendGroupMemberAlreadyExists = errors.New("social: friend group member already exists")
)

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
	if err := s.requireLocalPeers(ctx, owner); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	now := s.now()
	id := s.newID()
	unlock := s.lockGroup(id)
	defer unlock()
	if err := s.rejectDataPendingDeletion(ctx, id); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	role := rpcapi.FriendGroupMemberRoleOwner
	workspaceName := socialutil.GroupWorkspaceName(id)
	group := rpcapi.FriendGroupObject{
		Name:                   name,
		DisplayName:            req.DisplayName,
		Description:            socialutil.OptionalString(strings.TrimSpace(socialutil.StringValue(req.Description))),
		CreatedByPeerPublicKey: &owner,
		WorkspaceName:          &workspaceName,
		CreatedAt:              &now,
		UpdatedAt:              &now,
	}
	ctx, releasePeers, err := s.lockGroupPeers(ctx, group, owner)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	defer releasePeers()
	workspace, createdWorkspace, err := s.ensureGroupWorkspace(ctx, workspaceName, owner)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if err := socialutil.WriteJSON(ctx, friendGroups, socialutil.GroupKey(id), group); err != nil {
		if createdWorkspace {
			_ = s.deleteWorkspace(ctx, workspaceName)
		}
		return rpcapi.FriendGroupObject{}, err
	}
	if err := s.writeWorkspaceBinding(ctx, workspaceBinding{FriendGroupID: id, WorkspaceID: workspace.Id, WorkspaceName: workspace.Name}); err != nil {
		_ = friendGroups.Delete(ctx, socialutil.GroupKey(id))
		if createdWorkspace {
			_ = s.deleteWorkspace(ctx, workspaceName)
		}
		return rpcapi.FriendGroupObject{}, err
	}
	if _, err := s.writeMember(ctx, id, owner, role, name); err != nil {
		if createdWorkspace {
			_ = s.deleteWorkspace(ctx, workspaceName)
		}
		_ = friendGroups.Delete(ctx, socialutil.GroupKey(id))
		_ = s.deleteWorkspaceBinding(ctx, id)
		return rpcapi.FriendGroupObject{}, err
	}
	group.MyRole = &role
	s.notifyGroup(
		ctx,
		id,
		workspaceName,
		eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_CREATED,
		[]string{owner},
		now,
	)
	return group, nil
}

func (s *Server) AdminCreateFriendGroup(ctx context.Context, id, owner, name string, displayName, description *string) (adminhttp.AdminFriendGroupObject, error) {
	friendGroups, err := s.groupsStore()
	if err != nil {
		return adminhttp.AdminFriendGroupObject{}, err
	}
	owner = strings.TrimSpace(owner)
	if err := customid.ValidateFriendGroupID(id); err != nil {
		return adminhttp.AdminFriendGroupObject{}, fmt.Errorf("social: friend group %w", err)
	}
	name = strings.TrimSpace(name)
	if owner == "" || name == "" {
		return adminhttp.AdminFriendGroupObject{}, errors.New("social: friend group owner and name are required")
	}
	if err := s.requireLocalPeers(ctx, owner); err != nil {
		return adminhttp.AdminFriendGroupObject{}, err
	}
	if err := customid.ValidateMembershipName(id, owner); err != nil {
		return adminhttp.AdminFriendGroupObject{}, fmt.Errorf("social: %w", err)
	}
	unlock := s.lockGroup(id)
	defer unlock()
	now := s.now()
	if err := s.rejectDataPendingDeletion(ctx, id); err != nil {
		return adminhttp.AdminFriendGroupObject{}, err
	}
	workspaceName := socialutil.GroupWorkspaceName(id)
	group := rpcapi.FriendGroupObject{
		Name:                   name,
		DisplayName:            socialutil.OptionalString(strings.TrimSpace(socialutil.StringValue(displayName))),
		Description:            socialutil.OptionalString(strings.TrimSpace(socialutil.StringValue(description))),
		CreatedByPeerPublicKey: &owner,
		WorkspaceName:          &workspaceName,
		CreatedAt:              &now,
		UpdatedAt:              &now,
	}
	ctx, releasePeers, err := s.lockGroupPeers(ctx, group, owner)
	if err != nil {
		return adminhttp.AdminFriendGroupObject{}, err
	}
	defer releasePeers()
	groupData, err := json.Marshal(group)
	if err != nil {
		return adminhttp.AdminFriendGroupObject{}, err
	}
	_, created, err := kv.CreateIfAbsent(
		ctx,
		friendGroups,
		kv.Entry{Key: socialutil.GroupKey(id), Value: groupData},
		nil,
	)
	if err != nil {
		return adminhttp.AdminFriendGroupObject{}, err
	}
	if !created {
		return adminhttp.AdminFriendGroupObject{}, fmt.Errorf("%w: friend group id %q", socialutil.ErrResourceAlreadyExists, id)
	}
	workspace, createdWorkspace, err := s.ensureGroupWorkspace(ctx, workspaceName, owner)
	if err != nil {
		_ = friendGroups.Delete(ctx, socialutil.GroupKey(id))
		return adminhttp.AdminFriendGroupObject{}, err
	}
	if err := s.writeWorkspaceBinding(ctx, workspaceBinding{FriendGroupID: id, WorkspaceID: workspace.Id, WorkspaceName: workspace.Name}); err != nil {
		_ = friendGroups.Delete(ctx, socialutil.GroupKey(id))
		if createdWorkspace {
			_ = s.deleteWorkspace(ctx, workspaceName)
		}
		return adminhttp.AdminFriendGroupObject{}, err
	}
	role := rpcapi.FriendGroupMemberRoleOwner
	if _, err := s.writeMember(ctx, id, owner, role, name); err != nil {
		_ = friendGroups.Delete(ctx, socialutil.GroupKey(id))
		_ = s.deleteWorkspaceBinding(ctx, id)
		if createdWorkspace {
			_ = s.deleteWorkspace(ctx, workspaceName)
		}
		return adminhttp.AdminFriendGroupObject{}, err
	}
	projected, err := s.adminFriendGroupObject(ctx, id, group)
	if err != nil {
		return adminhttp.AdminFriendGroupObject{}, err
	}
	s.notifyGroup(
		ctx,
		id,
		workspaceName,
		eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_CREATED,
		[]string{owner},
		now,
	)
	return projected, nil
}

func (s *Server) AdminGetFriendGroupObject(ctx context.Context, friendGroupID string) (adminhttp.AdminFriendGroupObject, error) {
	item, err := s.AdminGetFriendGroup(ctx, friendGroupID)
	if err != nil {
		return adminhttp.AdminFriendGroupObject{}, err
	}
	return s.adminFriendGroupObject(ctx, friendGroupID, item)
}

func (s *Server) AdminListFriendGroupObjects(ctx context.Context, req rpcapi.FriendGroupListRequest) (adminhttp.AdminFriendGroupListResponse, error) {
	store, err := s.groupsStore()
	if err != nil {
		return adminhttp.AdminFriendGroupListResponse{}, err
	}
	entries, err := socialutil.ListPage(ctx, store, socialutil.GroupsRoot, socialutil.StringValue(req.Cursor), socialutil.IntValue(req.Limit))
	if err != nil {
		return adminhttp.AdminFriendGroupListResponse{}, err
	}
	items := make([]adminhttp.AdminFriendGroupObject, 0, len(entries.Items))
	for _, entry := range entries.Items {
		var item rpcapi.FriendGroupObject
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return adminhttp.AdminFriendGroupListResponse{}, err
		}
		id := socialutil.UnescapeStoreSegment(entry.Key[len(entry.Key)-1])
		projected, err := s.adminFriendGroupObject(ctx, id, item)
		if err != nil {
			return adminhttp.AdminFriendGroupListResponse{}, err
		}
		items = append(items, projected)
	}
	return adminhttp.AdminFriendGroupListResponse{Items: items, HasNext: entries.HasNext, NextCursor: entries.NextCursor}, nil
}

func (s *Server) adminFriendGroupObject(ctx context.Context, id string, item rpcapi.FriendGroupObject) (adminhttp.AdminFriendGroupObject, error) {
	binding, err := s.workspaceBinding(ctx, id)
	if err != nil {
		return adminhttp.AdminFriendGroupObject{}, err
	}
	return adminhttp.AdminFriendGroupObject{
		Id: id, Name: item.Name, DisplayName: item.DisplayName, Description: item.Description,
		CreatedByPeerPublicKey: socialutil.StringValue(item.CreatedByPeerPublicKey),
		WorkspaceId:            &binding.WorkspaceID, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}, nil
}

func (s *Server) workspaceBinding(ctx context.Context, friendGroupID string) (workspaceBinding, error) {
	binding, err := s.readWorkspaceBinding(ctx, friendGroupID)
	if err == nil {
		return binding, nil
	}
	if !errors.Is(err, kv.ErrNotFound) {
		return workspaceBinding{}, err
	}
	if intent, intentErr := s.readRetirementIntent(ctx, friendGroupID); intentErr == nil {
		return workspaceBinding{FriendGroupID: friendGroupID, WorkspaceID: intent.WorkspaceID, WorkspaceName: intent.WorkspaceName}, nil
	} else if !errors.Is(intentErr, kv.ErrNotFound) {
		return workspaceBinding{}, intentErr
	}
	receipt, receiptErr := s.readRetirementReceipt(ctx, friendGroupID)
	if receiptErr != nil {
		return workspaceBinding{}, receiptErr
	}
	return workspaceBinding{FriendGroupID: friendGroupID, WorkspaceID: receipt.WorkspaceID, WorkspaceName: receipt.WorkspaceName}, nil
}

func (s *Server) AdminFriendGroupObject(ctx context.Context, id string, item rpcapi.FriendGroupObject) (adminhttp.AdminFriendGroupObject, error) {
	return s.adminFriendGroupObject(ctx, id, item)
}

func (s *Server) lockGroup(friendGroupID string) func() {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(friendGroupID))
	mu := &groupMutationMu[hash.Sum32()%uint32(len(groupMutationMu))]
	mu.Lock()
	return mu.Unlock
}

func (s *Server) GetFriendGroup(ctx context.Context, owner string, req rpcapi.FriendGroupGetRequest) (rpcapi.FriendGroupObject, error) {
	store, err := s.groupsStore()
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	friendGroupID, err := s.resolveFriendGroupName(ctx, owner, req.Name)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if friendGroupID == "" {
		return rpcapi.FriendGroupObject{}, errors.New("social: group name is required")
	}
	if err := s.requireRead(ctx, owner, friendGroupID); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	group, err := socialutil.ReadJSONValue[rpcapi.FriendGroupObject](ctx, store, socialutil.GroupKey(friendGroupID))
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	return s.withMyRoleAndName(ctx, owner, friendGroupID, group)
}

// ResolveFriendGroupWorkspace returns the authoritative system Workspace for
// a current Group member. It loads the Group before checking membership so a
// stale membership record cannot grant access after Group retirement.
func (s *Server) ResolveFriendGroupWorkspace(ctx context.Context, owner, friendGroupID string) (string, error) {
	store, err := s.groupsStore()
	if err != nil {
		return "", err
	}
	owner = strings.TrimSpace(owner)
	if err := customid.ValidateFriendGroupID(friendGroupID); err != nil {
		return "", fmt.Errorf("social: invalid group id: %w", err)
	}
	if owner == "" {
		return "", errors.New("social: group id and peer public key are required")
	}
	group, err := socialutil.ReadJSONValue[rpcapi.FriendGroupObject](ctx, store, socialutil.GroupKey(friendGroupID))
	if err != nil {
		return "", err
	}
	if err := s.rejectDataPendingDeletion(ctx, friendGroupID); err != nil {
		if errors.Is(err, errFriendGroupPendingDeletion) {
			return "", kv.ErrNotFound
		}
		return "", err
	}
	if _, err := s.groupMember(ctx, friendGroupID, owner); err != nil {
		return "", err
	}
	workspaceName := strings.TrimSpace(socialutil.StringValue(group.WorkspaceName))
	if workspaceName == "" {
		return "", errors.New("social: friend group workspace binding is missing")
	}
	return workspaceName, nil
}

func (s *Server) ResolveFriendGroupWorkspaceByName(ctx context.Context, owner, name string) (string, error) {
	friendGroupID, err := s.resolveFriendGroupName(ctx, owner, name)
	if err != nil {
		return "", err
	}
	return s.ResolveFriendGroupWorkspace(ctx, owner, friendGroupID)
}

func (s *Server) ResolveFriendGroupWorkspaceIDByName(ctx context.Context, owner, name string) (string, error) {
	friendGroupID, err := s.resolveFriendGroupName(ctx, owner, name)
	if err != nil {
		return "", err
	}
	if _, err := s.ResolveFriendGroupWorkspace(ctx, owner, friendGroupID); err != nil {
		return "", err
	}
	binding, err := s.workspaceBinding(ctx, friendGroupID)
	if err != nil {
		return "", err
	}
	return binding.WorkspaceID, nil
}

func (s *Server) AdminGetFriendGroup(ctx context.Context, friendGroupID string) (rpcapi.FriendGroupObject, error) {
	store, err := s.groupsStore()
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if err := customid.ValidateFriendGroupID(friendGroupID); err != nil {
		return rpcapi.FriendGroupObject{}, fmt.Errorf("social: friend group %w", err)
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
		var member friendGroupMemberRecord
		if err := json.Unmarshal(entry.Value, &member); err != nil {
			return rpcapi.FriendGroupListResponse{}, err
		}
		if err := member.validate(); err != nil {
			return rpcapi.FriendGroupListResponse{}, err
		}
		friendGroupID := socialutil.UnescapeStoreSegment(entry.Key[len(entry.Key)-1])
		item, err := socialutil.ReadJSONValue[rpcapi.FriendGroupObject](ctx, store, socialutil.GroupKey(friendGroupID))
		if err != nil {
			return rpcapi.FriendGroupListResponse{}, err
		}
		role := member.Role
		item.MyRole = &role
		item.Name = member.FriendGroupName
		items = append(items, item)
	}
	return rpcapi.FriendGroupListResponse{Items: items, HasNext: entries.HasNext, NextCursor: entries.NextCursor}, nil
}

// WorkspaceRecipientsByID returns current members of the Group Chatroom bound
// to the canonical Workspace without inferring the group identifier from its
// peer-visible name.
func (s *Server) WorkspaceRecipientsByID(ctx context.Context, workspaceID string) ([]string, error) {
	bindings, err := s.relationshipStore()
	if err != nil {
		return nil, err
	}
	if err := customid.ValidateResourceID(workspaceID); err != nil {
		return nil, fmt.Errorf("social: invalid workspace id: %w", err)
	}
	for entry, err := range bindings.List(ctx, workspaceBindingsRoot) {
		if err != nil {
			return nil, err
		}
		var binding workspaceBinding
		if err := json.Unmarshal(entry.Value, &binding); err != nil {
			return nil, err
		}
		if binding.WorkspaceID != workspaceID {
			continue
		}
		members, err := s.listAllMembers(ctx, binding.FriendGroupID)
		if err != nil {
			return nil, err
		}
		recipients := make([]string, 0, len(members))
		for _, member := range members {
			recipients = append(recipients, member.PeerPublicKey)
		}
		return recipients, nil
	}
	return nil, kv.ErrNotFound
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
	friendGroupID, err := s.resolveFriendGroupName(ctx, owner, req.Name)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	unlock := s.lockGroup(friendGroupID)
	defer unlock()
	if err := s.requireRole(ctx, owner, friendGroupID, rpcapi.FriendGroupMemberRoleOwner); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	group, err := s.putFriendGroup(ctx, friendGroupID, req.DisplayName, req.Description)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	s.notifyCurrentGroup(
		ctx,
		friendGroupID,
		socialutil.StringValue(group.WorkspaceName),
		eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_METADATA_UPDATED,
	)
	return s.withMyRoleAndName(ctx, owner, friendGroupID, group)
}

func (s *Server) AdminPutFriendGroup(ctx context.Context, friendGroupID string, displayName, description *string) (rpcapi.FriendGroupObject, error) {
	if err := customid.ValidateFriendGroupID(friendGroupID); err != nil {
		return rpcapi.FriendGroupObject{}, fmt.Errorf("social: friend group %w", err)
	}
	unlock := s.lockGroup(friendGroupID)
	defer unlock()
	group, err := s.putFriendGroup(ctx, friendGroupID, displayName, description)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	s.notifyCurrentGroup(
		ctx,
		friendGroupID,
		socialutil.StringValue(group.WorkspaceName),
		eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_METADATA_UPDATED,
	)
	return group, nil
}

func (s *Server) DeleteFriendGroup(ctx context.Context, owner string, req rpcapi.FriendGroupDeleteRequest) (rpcapi.FriendGroupObject, error) {
	friendGroupID, err := s.resolveFriendGroupName(ctx, owner, req.Name)
	if err != nil {
		if !errors.Is(err, kv.ErrNotFound) {
			return rpcapi.FriendGroupObject{}, err
		}
		friendGroupID, err = s.resolveRetiredFriendGroupName(ctx, owner, req.Name)
		if err != nil {
			return rpcapi.FriendGroupObject{}, err
		}
	}
	unlock := s.lockGroup(friendGroupID)
	defer unlock()
	if err := s.requireRole(ctx, owner, friendGroupID, rpcapi.FriendGroupMemberRoleOwner); err != nil {
		intent, intentErr := s.readRetirementIntent(ctx, friendGroupID)
		if intentErr != nil {
			if errors.Is(intentErr, kv.ErrNotFound) {
				if _, groupErr := s.AdminGetFriendGroup(ctx, friendGroupID); groupErr == nil {
					return rpcapi.FriendGroupObject{}, err
				} else if !errors.Is(groupErr, kv.ErrNotFound) {
					return rpcapi.FriendGroupObject{}, groupErr
				}
				completed, completedErr := s.completedFriendGroupDeletion(ctx, owner, friendGroupID)
				if completedErr != nil {
					return rpcapi.FriendGroupObject{}, err
				}
				completed.Name = req.Name
				return completed, nil
			}
			return rpcapi.FriendGroupObject{}, intentErr
		}
		if strings.TrimSpace(socialutil.StringValue(intent.FriendGroup.CreatedByPeerPublicKey)) != strings.TrimSpace(owner) {
			return rpcapi.FriendGroupObject{}, err
		}
		lockedCtx, releasePeers, lockErr := s.lockRetirementPeers(ctx, intent)
		if lockErr != nil {
			return rpcapi.FriendGroupObject{}, lockErr
		}
		defer releasePeers()
		ctx = lockedCtx
		return s.completeFriendGroupRetirement(ctx, friendGroupID, intent)
	}
	return s.deleteFriendGroup(ctx, friendGroupID)
}

func (s *Server) AdminDeleteFriendGroup(ctx context.Context, friendGroupID string) (rpcapi.FriendGroupObject, error) {
	if err := customid.ValidateFriendGroupID(friendGroupID); err != nil {
		return rpcapi.FriendGroupObject{}, fmt.Errorf("social: friend group %w", err)
	}
	unlock := s.lockGroup(friendGroupID)
	defer unlock()
	return s.deleteFriendGroup(ctx, friendGroupID)
}

func (s *Server) GetFriendGroupInviteToken(ctx context.Context, owner string, req rpcapi.FriendGroupInviteTokenGetRequest) (rpcapi.FriendGroupInviteTokenGetResponse, error) {
	store, err := s.groupInviteTokensStore()
	if err != nil {
		return rpcapi.FriendGroupInviteTokenGetResponse{}, err
	}
	friendGroupID, err := s.resolveFriendGroupName(ctx, owner, req.FriendGroupName)
	if err != nil {
		return rpcapi.FriendGroupInviteTokenGetResponse{}, err
	}
	unlock := s.lockGroup(friendGroupID)
	defer unlock()
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
	friendGroupID, err := s.resolveFriendGroupName(ctx, owner, req.FriendGroupName)
	if err != nil {
		return rpcapi.FriendGroupInviteTokenCreateResponse{}, err
	}
	unlock := s.lockGroup(friendGroupID)
	defer unlock()
	if err := s.requireRole(ctx, owner, friendGroupID, rpcapi.FriendGroupMemberRoleOwner); err != nil {
		return rpcapi.FriendGroupInviteTokenCreateResponse{}, err
	}
	group, err := s.AdminGetFriendGroup(ctx, friendGroupID)
	if err != nil {
		return rpcapi.FriendGroupInviteTokenCreateResponse{}, err
	}
	lockedCtx, releasePeers, err := s.lockGroupPeers(ctx, group)
	if err != nil {
		return rpcapi.FriendGroupInviteTokenCreateResponse{}, err
	}
	defer releasePeers()
	ctx = lockedCtx
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
	friendGroupID, err := s.resolveFriendGroupName(ctx, owner, req.FriendGroupName)
	if err != nil {
		return rpcapi.FriendGroupInviteTokenClearResponse{}, err
	}
	unlock := s.lockGroup(friendGroupID)
	defer unlock()
	if err := s.requireRole(ctx, owner, friendGroupID, rpcapi.FriendGroupMemberRoleOwner); err != nil {
		return rpcapi.FriendGroupInviteTokenClearResponse{}, err
	}
	group, err := s.AdminGetFriendGroup(ctx, friendGroupID)
	if err != nil {
		return rpcapi.FriendGroupInviteTokenClearResponse{}, err
	}
	lockedCtx, releasePeers, err := s.lockGroupPeers(ctx, group)
	if err != nil {
		return rpcapi.FriendGroupInviteTokenClearResponse{}, err
	}
	defer releasePeers()
	ctx = lockedCtx
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
	record, ok, err := s.activeGroupInviteToken(ctx, store, friendGroupID)
	if err != nil || !ok {
		return rpcapi.FriendGroupInviteTokenGetResponse{}, err
	}
	return rpcapi.FriendGroupInviteTokenGetResponse{InviteToken: &record.InviteToken, ExpiresAt: &record.ExpiresAt}, nil
}

func (s *Server) AdminPutFriendGroupInviteToken(ctx context.Context, friendGroupID, inviteToken string, expiresAt time.Time) (rpcapi.FriendGroupInviteTokenCreateResponse, error) {
	if err := customid.ValidateFriendGroupID(friendGroupID); err != nil {
		return rpcapi.FriendGroupInviteTokenCreateResponse{}, fmt.Errorf("social: friend group %w", err)
	}
	unlock := s.lockGroup(friendGroupID)
	defer unlock()
	group, err := s.AdminGetFriendGroup(ctx, friendGroupID)
	if err != nil {
		return rpcapi.FriendGroupInviteTokenCreateResponse{}, err
	}
	lockedCtx, releasePeers, err := s.lockGroupPeers(ctx, group)
	if err != nil {
		return rpcapi.FriendGroupInviteTokenCreateResponse{}, err
	}
	defer releasePeers()
	ctx = lockedCtx
	store, err := s.groupInviteTokensStore()
	if err != nil {
		return rpcapi.FriendGroupInviteTokenCreateResponse{}, err
	}
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
	if err := customid.ValidateFriendGroupID(friendGroupID); err != nil {
		return rpcapi.FriendGroupInviteTokenClearResponse{}, fmt.Errorf("social: friend group %w", err)
	}
	unlock := s.lockGroup(friendGroupID)
	defer unlock()
	group, err := s.AdminGetFriendGroup(ctx, friendGroupID)
	if err != nil {
		return rpcapi.FriendGroupInviteTokenClearResponse{}, err
	}
	lockedCtx, releasePeers, err := s.lockGroupPeers(ctx, group)
	if err != nil {
		return rpcapi.FriendGroupInviteTokenClearResponse{}, err
	}
	defer releasePeers()
	ctx = lockedCtx
	store, err := s.groupInviteTokensStore()
	if err != nil {
		return rpcapi.FriendGroupInviteTokenClearResponse{}, err
	}
	if err := store.Delete(ctx, socialutil.GroupInviteTokenKey(friendGroupID)); err != nil && !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendGroupInviteTokenClearResponse{}, err
	}
	return rpcapi.FriendGroupInviteTokenClearResponse{}, nil
}

func (s *Server) JoinFriendGroup(ctx context.Context, owner string, req rpcapi.FriendGroupJoinRequest) (rpcapi.FriendGroupJoinResponse, error) {
	owner = strings.TrimSpace(owner)
	name := strings.TrimSpace(req.Name)
	if owner == "" || name == "" {
		return rpcapi.FriendGroupJoinResponse{}, errors.New("social: peer public key and group name are required")
	}
	record, err := s.findGroupInviteToken(ctx, strings.TrimSpace(req.InviteToken))
	if err != nil {
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	friendGroupID := strings.TrimSpace(record.FriendGroupID)
	if friendGroupID == "" {
		return rpcapi.FriendGroupJoinResponse{}, errors.New("social: invite token group is empty")
	}
	group, err := s.AdminGetFriendGroup(ctx, friendGroupID)
	if err != nil {
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	if err := s.requireLocalPeers(ctx, socialutil.StringValue(group.CreatedByPeerPublicKey), owner); err != nil {
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	if existingID, err := s.resolveFriendGroupName(ctx, owner, name); err == nil && existingID != friendGroupID {
		return rpcapi.FriendGroupJoinResponse{}, errors.New("social: friend group name already exists")
	} else if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	unlock := s.lockGroup(friendGroupID)
	defer unlock()
	if existing, err := s.groupMember(ctx, friendGroupID, owner); err == nil {
		if socialutil.StringValue(existing.FriendGroupName) != name {
			return rpcapi.FriendGroupJoinResponse{}, errors.New("social: friend group membership name is immutable")
		}
		group, err := s.GetFriendGroup(ctx, owner, rpcapi.FriendGroupGetRequest{Name: name})
		if err != nil {
			return rpcapi.FriendGroupJoinResponse{}, err
		}
		return rpcapi.FriendGroupJoinResponse{Group: group, Member: existing}, nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	member, err := s.writeMember(ctx, friendGroupID, owner, rpcapi.FriendGroupMemberRoleMember, name)
	if err != nil {
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	group, err = s.GetFriendGroup(ctx, owner, rpcapi.FriendGroupGetRequest{Name: name})
	if err != nil {
		s.restoreMember(ctx, friendGroupID, owner, friendGroupMemberRecord{}, kv.ErrNotFound)
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	s.notifyCurrentGroup(
		ctx,
		friendGroupID,
		socialutil.StringValue(group.WorkspaceName),
		eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_MEMBER_ADDED,
		owner,
	)
	return rpcapi.FriendGroupJoinResponse{Group: group, Member: member}, nil
}

func (s *Server) AddFriendGroupMember(ctx context.Context, owner string, req rpcapi.FriendGroupMemberAddRequest) (rpcapi.FriendGroupMemberObject, error) {
	friendGroupID, err := s.resolveFriendGroupName(ctx, owner, req.FriendGroupName)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	group, err := s.AdminGetFriendGroup(ctx, friendGroupID)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := s.requireLocalPeers(ctx, socialutil.StringValue(group.CreatedByPeerPublicKey), owner, req.PeerPublicKey); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	memberName := strings.TrimSpace(req.MemberName)
	req.PeerPublicKey = strings.TrimSpace(req.PeerPublicKey)
	if memberName == "" || !req.Role.Valid() {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: invalid group member role")
	}
	if existingID, err := s.resolveFriendGroupName(ctx, req.PeerPublicKey, memberName); err == nil {
		if existingID != friendGroupID {
			return rpcapi.FriendGroupMemberObject{}, errors.New("social: friend group name already exists for target member")
		}
	} else if !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	unlock := s.lockGroup(friendGroupID)
	defer unlock()
	if req.Role == rpcapi.FriendGroupMemberMutableRole("admin") {
		if err := s.requireRole(ctx, owner, friendGroupID, rpcapi.FriendGroupMemberRoleOwner); err != nil {
			return rpcapi.FriendGroupMemberObject{}, err
		}
	} else if err := s.requireAdmin(ctx, owner, friendGroupID); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	current, currentErr := s.groupMember(ctx, friendGroupID, req.PeerPublicKey)
	if currentErr != nil && !errors.Is(currentErr, kv.ErrNotFound) {
		return rpcapi.FriendGroupMemberObject{}, currentErr
	}
	if currentErr == nil && socialutil.GroupRole(current) == rpcapi.FriendGroupMemberRoleOwner {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: cannot change owner role")
	}
	member, err := s.writeMember(ctx, friendGroupID, req.PeerPublicKey, rpcapi.FriendGroupMemberRole(req.Role), memberName)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	change := eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_MEMBER_ADDED
	if currentErr == nil {
		change = eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_MEMBER_ROLE_CHANGED
	}
	s.notifyCurrentGroup(ctx, friendGroupID, "", change, req.PeerPublicKey)
	return member, nil
}

func (s *Server) PutFriendGroupMember(ctx context.Context, owner string, req rpcapi.FriendGroupMemberPutRequest) (rpcapi.FriendGroupMemberObject, error) {
	friendGroupID, err := s.resolveFriendGroupName(ctx, owner, req.FriendGroupName)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	group, err := s.AdminGetFriendGroup(ctx, friendGroupID)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := s.requireLocalPeers(ctx, socialutil.StringValue(group.CreatedByPeerPublicKey), owner, req.Name); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	req.Name = strings.TrimSpace(req.Name)
	if !req.Role.Valid() {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: invalid group member role")
	}
	unlock := s.lockGroup(friendGroupID)
	defer unlock()
	if err := s.requireRole(ctx, owner, friendGroupID, rpcapi.FriendGroupMemberRoleOwner); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	current, err := s.groupMember(ctx, friendGroupID, req.Name)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if current.Role != nil && *current.Role == rpcapi.FriendGroupMemberRoleOwner {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: cannot change owner role")
	}
	member, err := s.writeMember(ctx, friendGroupID, req.Name, rpcapi.FriendGroupMemberRole(req.Role))
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	s.notifyCurrentGroup(
		ctx,
		friendGroupID,
		"",
		eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_MEMBER_ROLE_CHANGED,
		req.Name,
	)
	return member, nil
}

func (s *Server) DeleteFriendGroupMember(ctx context.Context, owner string, req rpcapi.FriendGroupMemberDeleteRequest) (rpcapi.FriendGroupMemberObject, error) {
	friendGroupID, err := s.resolveFriendGroupName(ctx, owner, req.FriendGroupName)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	req.Name = strings.TrimSpace(req.Name)
	unlock := s.lockGroup(friendGroupID)
	defer unlock()
	group, err := s.AdminGetFriendGroup(ctx, friendGroupID)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	lockedCtx, releasePeers, err := s.lockGroupPeers(ctx, group, req.Name)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	defer releasePeers()
	ctx = lockedCtx
	current, err := s.groupMember(ctx, friendGroupID, req.Name)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	role := socialutil.GroupRole(current)
	switch role {
	case rpcapi.FriendGroupMemberRoleOwner:
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: cannot delete friend group owner")
	case rpcapi.FriendGroupMemberRoleAdmin:
		if err := s.requireRole(ctx, owner, friendGroupID, rpcapi.FriendGroupMemberRoleOwner); err != nil {
			return rpcapi.FriendGroupMemberObject{}, err
		}
	default:
		if owner != req.Name {
			if err := s.requireAdmin(ctx, owner, friendGroupID); err != nil {
				return rpcapi.FriendGroupMemberObject{}, err
			}
		}
	}
	recipients := s.groupRecipients(ctx, friendGroupID, req.Name)
	members, err := s.membersStore()
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := members.Delete(ctx, socialutil.GroupMemberKey(friendGroupID, req.Name)); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	belongs, err := s.belongsStore()
	if err != nil {
		_ = socialutil.WriteJSON(ctx, members, socialutil.GroupMemberKey(friendGroupID, req.Name), friendGroupMemberRecordFromObject(friendGroupID, current))
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := belongs.Delete(ctx, socialutil.GroupBelongKey(req.Name, friendGroupID)); err != nil && !errors.Is(err, kv.ErrNotFound) {
		_ = socialutil.WriteJSON(ctx, members, socialutil.GroupMemberKey(friendGroupID, req.Name), friendGroupMemberRecordFromObject(friendGroupID, current))
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := belongs.Delete(ctx, socialutil.GroupNameKey(req.Name, socialutil.StringValue(current.FriendGroupName))); err != nil && !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	s.notifyGroupWithNames(
		ctx,
		friendGroupID,
		"",
		eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_MEMBER_REMOVED,
		recipients,
		s.now(),
		map[string]string{req.Name: socialutil.StringValue(current.FriendGroupName)},
		req.Name,
	)
	return current, nil
}

func (s *Server) ListFriendGroupMembers(ctx context.Context, owner string, req rpcapi.FriendGroupMemberListRequest) (rpcapi.FriendGroupMemberListResponse, error) {
	friendGroupID, err := s.resolveFriendGroupName(ctx, owner, socialutil.StringValue(req.FriendGroupName))
	if err != nil {
		return rpcapi.FriendGroupMemberListResponse{}, err
	}
	if err := s.requireRead(ctx, owner, friendGroupID); err != nil {
		return rpcapi.FriendGroupMemberListResponse{}, err
	}
	return s.listFriendGroupMembers(ctx, friendGroupID, socialutil.StringValue(req.Cursor), socialutil.IntValue(req.Limit))
}

func (s *Server) AdminListFriendGroupMembers(ctx context.Context, friendGroupID string, req rpcapi.FriendGroupMemberListRequest) (rpcapi.FriendGroupMemberListResponse, error) {
	if _, err := s.AdminGetFriendGroup(ctx, friendGroupID); err != nil {
		return rpcapi.FriendGroupMemberListResponse{}, err
	}
	return s.listFriendGroupMembers(ctx, friendGroupID, socialutil.StringValue(req.Cursor), socialutil.IntValue(req.Limit))
}

func (s *Server) AdminCreateFriendGroupMember(ctx context.Context, friendGroupID, peerID, name string, role rpcapi.FriendGroupMemberRole) (rpcapi.FriendGroupMemberObject, error) {
	if err := customid.ValidateMembershipName(friendGroupID, peerID); err != nil {
		return rpcapi.FriendGroupMemberObject{}, fmt.Errorf("social: friend group %w", err)
	}
	if peerID == "" || peerID != strings.TrimSpace(peerID) {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: friend group id and peer public key are required")
	}
	if !role.Valid() {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: invalid group member role")
	}
	group, err := s.AdminGetFriendGroup(ctx, friendGroupID)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := s.requireLocalPeers(ctx, socialutil.StringValue(group.CreatedByPeerPublicKey), peerID); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	unlock := s.lockGroup(friendGroupID)
	defer unlock()
	group, err = s.AdminGetFriendGroup(ctx, friendGroupID)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	lockedCtx, releasePeers, err := s.lockGroupPeers(ctx, group, peerID)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	defer releasePeers()
	ctx = lockedCtx
	member, err := s.createMember(ctx, friendGroupID, peerID, role, name)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	s.notifyCurrentGroup(ctx, friendGroupID, "", eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_MEMBER_ADDED, peerID)
	return member, nil
}

func (s *Server) AdminPutFriendGroupMember(ctx context.Context, friendGroupID, peerID, name string, role rpcapi.FriendGroupMemberRole) (rpcapi.FriendGroupMemberObject, error) {
	if err := customid.ValidateMembershipName(friendGroupID, peerID); err != nil {
		return rpcapi.FriendGroupMemberObject{}, fmt.Errorf("social: friend group %w", err)
	}
	if peerID == "" || peerID != strings.TrimSpace(peerID) {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: friend group id and peer public key are required")
	}
	if !role.Valid() {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: invalid group member role")
	}
	group, err := s.AdminGetFriendGroup(ctx, friendGroupID)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := s.requireLocalPeers(ctx, socialutil.StringValue(group.CreatedByPeerPublicKey), peerID); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	unlock := s.lockGroup(friendGroupID)
	defer unlock()
	group, err = s.AdminGetFriendGroup(ctx, friendGroupID)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	lockedCtx, releasePeers, err := s.lockGroupPeers(ctx, group, peerID)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	defer releasePeers()
	ctx = lockedCtx
	_, currentErr := s.groupMember(ctx, friendGroupID, peerID)
	if currentErr != nil && !errors.Is(currentErr, kv.ErrNotFound) {
		return rpcapi.FriendGroupMemberObject{}, currentErr
	}
	member, err := s.writeMember(ctx, friendGroupID, peerID, role, name)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	change := eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_MEMBER_ADDED
	if currentErr == nil {
		change = eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_MEMBER_ROLE_CHANGED
	}
	s.notifyCurrentGroup(ctx, friendGroupID, "", change, peerID)
	return member, nil
}
func (s *Server) AdminGetFriendGroupMember(ctx context.Context, friendGroupID, peerID string) (rpcapi.FriendGroupMemberObject, error) {
	if err := customid.ValidateMembershipName(friendGroupID, peerID); err != nil {
		return rpcapi.FriendGroupMemberObject{}, fmt.Errorf("social: friend group %w", err)
	}
	if peerID == "" || peerID != strings.TrimSpace(peerID) {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: peer public key is required without surrounding whitespace")
	}
	if _, err := s.AdminGetFriendGroup(ctx, friendGroupID); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	return s.groupMember(ctx, friendGroupID, peerID)
}

func (s *Server) AdminDeleteFriendGroupMember(ctx context.Context, friendGroupID, peerID string) (rpcapi.FriendGroupMemberObject, error) {
	if err := customid.ValidateMembershipName(friendGroupID, peerID); err != nil {
		return rpcapi.FriendGroupMemberObject{}, fmt.Errorf("social: friend group %w", err)
	}
	if peerID == "" || peerID != strings.TrimSpace(peerID) {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: peer public key is required without surrounding whitespace")
	}
	unlock := s.lockGroup(friendGroupID)
	defer unlock()
	group, err := s.AdminGetFriendGroup(ctx, friendGroupID)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	lockedCtx, releasePeers, err := s.lockGroupPeers(ctx, group, peerID)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	defer releasePeers()
	ctx = lockedCtx
	current, err := s.groupMember(ctx, friendGroupID, peerID)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	recipients := s.groupRecipients(ctx, friendGroupID, peerID)
	members, err := s.membersStore()
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := members.Delete(ctx, socialutil.GroupMemberKey(friendGroupID, peerID)); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	belongs, err := s.belongsStore()
	if err != nil {
		_ = socialutil.WriteJSON(ctx, members, socialutil.GroupMemberKey(friendGroupID, peerID), friendGroupMemberRecordFromObject(friendGroupID, current))
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := belongs.Delete(ctx, socialutil.GroupBelongKey(peerID, friendGroupID)); err != nil && !errors.Is(err, kv.ErrNotFound) {
		_ = socialutil.WriteJSON(ctx, members, socialutil.GroupMemberKey(friendGroupID, peerID), friendGroupMemberRecordFromObject(friendGroupID, current))
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := belongs.Delete(ctx, socialutil.GroupNameKey(peerID, socialutil.StringValue(current.FriendGroupName))); err != nil && !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	s.notifyGroupWithNames(
		ctx,
		friendGroupID,
		"",
		eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_MEMBER_REMOVED,
		recipients,
		s.now(),
		map[string]string{peerID: socialutil.StringValue(current.FriendGroupName)},
		peerID,
	)
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
		var record friendGroupMemberRecord
		if err := json.Unmarshal(entry.Value, &record); err != nil {
			return rpcapi.FriendGroupMemberListResponse{}, err
		}
		if err := record.validate(); err != nil {
			return rpcapi.FriendGroupMemberListResponse{}, err
		}
		items = append(items, record.peerObject())
	}
	return rpcapi.FriendGroupMemberListResponse{Items: items, HasNext: entries.HasNext, NextCursor: entries.NextCursor}, nil
}

func (s *Server) writeMember(ctx context.Context, friendGroupID, peerID string, role rpcapi.FriendGroupMemberRole, localNames ...string) (rpcapi.FriendGroupMemberObject, error) {
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
	if !peerMutationLocked(ctx) {
		group, err := s.AdminGetFriendGroup(ctx, friendGroupID)
		if err != nil {
			return rpcapi.FriendGroupMemberObject{}, err
		}
		lockedCtx, releasePeers, err := s.lockGroupPeers(ctx, group, peerID)
		if err != nil {
			return rpcapi.FriendGroupMemberObject{}, err
		}
		defer releasePeers()
		ctx = lockedCtx
	}
	localName := ""
	if len(localNames) > 0 {
		localName = strings.TrimSpace(localNames[0])
	}
	now := s.now()
	current, currentErr := socialutil.ReadJSONValue[friendGroupMemberRecord](ctx, members, socialutil.GroupMemberKey(friendGroupID, peerID))
	var item friendGroupMemberRecord
	if currentErr == nil {
		if err := current.validate(); err != nil {
			return rpcapi.FriendGroupMemberObject{}, err
		}
		if localName == "" {
			localName = current.FriendGroupName
		}
		if localName != "" && current.FriendGroupName != localName {
			return rpcapi.FriendGroupMemberObject{}, errors.New("social: friend group membership name is immutable")
		}
		current.Role = role
		current.UpdatedAt = now
		item = current
	} else {
		if currentErr != nil && !errors.Is(currentErr, kv.ErrNotFound) {
			return rpcapi.FriendGroupMemberObject{}, currentErr
		}
		if localName == "" {
			return rpcapi.FriendGroupMemberObject{}, errors.New("social: friend group membership name is required")
		}
		item = friendGroupMemberRecord{FriendGroupID: friendGroupID, FriendGroupName: localName, PeerPublicKey: peerID, Role: role, CreatedAt: now, UpdatedAt: now}
	}
	if existingID, err := belongs.Get(ctx, socialutil.GroupNameKey(peerID, localName)); err == nil && string(existingID) != friendGroupID {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: friend group name already exists")
	} else if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := socialutil.WriteJSON(ctx, members, socialutil.GroupMemberKey(friendGroupID, peerID), item); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := socialutil.WriteJSON(ctx, belongs, socialutil.GroupBelongKey(peerID, friendGroupID), item); err != nil {
		s.restoreMember(ctx, friendGroupID, peerID, current, currentErr)
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := belongs.Set(ctx, socialutil.GroupNameKey(peerID, item.FriendGroupName), []byte(friendGroupID)); err != nil {
		s.restoreMember(ctx, friendGroupID, peerID, current, currentErr)
		return rpcapi.FriendGroupMemberObject{}, err
	}
	return item.peerObject(), nil
}

func (s *Server) createMember(ctx context.Context, friendGroupID, peerID string, role rpcapi.FriendGroupMemberRole, localName string) (rpcapi.FriendGroupMemberObject, error) {
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
	localName = strings.TrimSpace(localName)
	if friendGroupID == "" || peerID == "" {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: friend group id and peer public key are required")
	}
	if localName == "" {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: friend group membership name is required")
	}
	if !role.Valid() {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: invalid group member role")
	}
	now := s.now()
	item := friendGroupMemberRecord{
		FriendGroupID:   friendGroupID,
		FriendGroupName: localName,
		PeerPublicKey:   peerID,
		Role:            role,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	data, err := json.Marshal(item)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	store, prefixes, ok := kv.SharedAtomicStore(members, belongs)
	if !ok {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: group member stores do not share an atomic store")
	}
	memberKey := s.relationshipKey(prefixes[0], socialutil.GroupMemberKey(friendGroupID, peerID))
	belongKey := s.relationshipKey(prefixes[1], socialutil.GroupBelongKey(peerID, friendGroupID))
	nameKey := s.relationshipKey(prefixes[1], socialutil.GroupNameKey(peerID, localName))
	conflict, _, created, err := kv.CreateIfAllAbsent(ctx, store, []kv.Entry{
		{Key: memberKey, Value: data},
		{Key: nameKey, Value: []byte(friendGroupID)},
	}, []kv.Entry{{Key: belongKey, Value: data}})
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if !created {
		if slices.Equal(conflict, memberKey) {
			return rpcapi.FriendGroupMemberObject{}, ErrFriendGroupMemberAlreadyExists
		}
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: friend group name already exists")
	}
	return item.peerObject(), nil
}

func (s *Server) putFriendGroup(ctx context.Context, friendGroupID string, displayName, description *string) (rpcapi.FriendGroupObject, error) {
	store, err := s.groupsStore()
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	group, err := socialutil.ReadJSONValue[rpcapi.FriendGroupObject](ctx, store, socialutil.GroupKey(friendGroupID))
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if !peerMutationLocked(ctx) {
		lockedCtx, releasePeers, err := s.lockGroupPeers(ctx, group)
		if err != nil {
			return rpcapi.FriendGroupObject{}, err
		}
		defer releasePeers()
		ctx = lockedCtx
	}
	if displayName != nil {
		group.DisplayName = socialutil.OptionalString(strings.TrimSpace(*displayName))
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
	if s == nil || s.Workspaces == nil {
		return rpcapi.FriendGroupObject{}, errors.New("social: Workspace retirement service not configured")
	}
	friendGroups, err := s.groupsStore()
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	group, err := socialutil.ReadJSONValue[rpcapi.FriendGroupObject](ctx, friendGroups, socialutil.GroupKey(friendGroupID))
	if err != nil {
		if !errors.Is(err, kv.ErrNotFound) {
			return rpcapi.FriendGroupObject{}, err
		}
		intent, intentErr := s.readRetirementIntent(ctx, friendGroupID)
		if intentErr != nil {
			if errors.Is(intentErr, kv.ErrNotFound) {
				return s.completedFriendGroupDeletion(ctx, "", friendGroupID)
			}
			return rpcapi.FriendGroupObject{}, intentErr
		}
		lockedCtx, releasePeers, lockErr := s.lockRetirementPeers(ctx, intent)
		if lockErr != nil {
			return rpcapi.FriendGroupObject{}, lockErr
		}
		defer releasePeers()
		return s.completeFriendGroupRetirement(lockedCtx, friendGroupID, intent)
	}
	members, err := s.listAllMembers(ctx, friendGroupID)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	peerIDs := make([]string, 0, len(members))
	for _, member := range members {
		peerIDs = append(peerIDs, member.PeerPublicKey)
	}
	lockedCtx, releasePeers, err := s.lockGroupPeers(ctx, group, peerIDs...)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	defer releasePeers()
	ctx = lockedCtx
	workspaceName := socialutil.StringValue(group.WorkspaceName)
	if workspaceName == "" {
		return rpcapi.FriendGroupObject{}, errors.New("social: FriendGroup Workspace name is missing")
	}
	binding, err := s.readWorkspaceBinding(ctx, friendGroupID)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if binding.WorkspaceName != workspaceName {
		return rpcapi.FriendGroupObject{}, errors.New("social: FriendGroup Workspace binding is inconsistent")
	}
	store, err := s.relationshipStore()
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if !kv.SupportsCreateIfAbsent(store) {
		return rpcapi.FriendGroupObject{}, fmt.Errorf(
			"social: friend group relationship store: %w",
			kv.ErrCreateIfAbsentUnsupported,
		)
	}
	intent := retirementIntent{
		FriendGroupID: friendGroupID,
		FriendGroup:   group,
		Members:       members,
		WorkspaceID:   binding.WorkspaceID,
		WorkspaceName: workspaceName,
		DeletedAt:     s.now(),
	}
	data, err := json.Marshal(intent)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	deleteKeys := []kv.Key{
		s.relationshipKey(s.GroupRelationshipPrefix, socialutil.GroupKey(friendGroupID)),
		s.relationshipKey(s.InviteRelationshipPrefix, socialutil.GroupInviteTokenKey(friendGroupID)),
		workspaceBindingKey(friendGroupID),
	}
	for _, member := range members {
		peerID := strings.TrimSpace(member.PeerPublicKey)
		deleteKeys = append(
			deleteKeys,
			s.relationshipKey(s.MemberRelationshipPrefix, socialutil.GroupMemberKey(friendGroupID, peerID)),
			s.relationshipKey(s.BelongRelationshipPrefix, socialutil.GroupBelongKey(peerID, friendGroupID)),
			s.relationshipKey(s.BelongRelationshipPrefix, socialutil.GroupNameKey(peerID, member.FriendGroupName)),
		)
	}
	if err := store.BatchMutate(
		ctx,
		[]kv.Entry{{Key: groupRetirementIntentKey(friendGroupID), Value: data}},
		deleteKeys,
	); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	return s.completeFriendGroupRetirement(ctx, friendGroupID, intent)
}

func (s *Server) completedFriendGroupDeletion(
	ctx context.Context,
	owner string,
	friendGroupID string,
) (rpcapi.FriendGroupObject, error) {
	receipt, err := s.readRetirementReceipt(ctx, friendGroupID)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	retired, err := s.Workspaces.GetRetiredSystemWorkspaceByID(
		ctx,
		receipt.WorkspaceID,
		apitypes.ChatRoomModeGroup,
		friendGroupID,
	)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if strings.TrimSpace(owner) != "" &&
		receipt.Owner != strings.TrimSpace(owner) {
		return rpcapi.FriendGroupObject{}, kv.ErrNotFound
	}
	if err := s.ensureDataPendingDeletion(ctx, friendGroupID, s.now()); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	return rpcapi.FriendGroupObject{
		Name:                   friendGroupID,
		WorkspaceName:          &receipt.WorkspaceName,
		CreatedByPeerPublicKey: retired.OwnerPublicKey,
	}, nil
}

func (s *Server) completeFriendGroupRetirement(ctx context.Context, friendGroupID string, intent retirementIntent) (rpcapi.FriendGroupObject, error) {
	if s == nil || s.Workspaces == nil {
		return rpcapi.FriendGroupObject{}, errors.New("social: Workspace retirement service not configured")
	}
	if err := s.ensureDataPendingDeletion(ctx, friendGroupID, intent.DeletedAt); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if _, err := s.Workspaces.RetireSystemWorkspaceByID(
		ctx,
		intent.WorkspaceID,
		apitypes.ChatRoomModeGroup,
		friendGroupID,
	); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	store, err := s.relationshipStore()
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	receipt := retirementReceipt{
		FriendGroupID: friendGroupID,
		Name:          intent.FriendGroup.Name,
		WorkspaceID:   intent.WorkspaceID,
		WorkspaceName: intent.WorkspaceName,
		Owner:         socialutil.StringValue(intent.FriendGroup.CreatedByPeerPublicKey),
		DeletedAt:     intent.DeletedAt,
	}
	receiptData, err := json.Marshal(receipt)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if err := store.BatchMutate(ctx, []kv.Entry{{Key: groupRetirementReceiptKey(friendGroupID), Value: receiptData}}, []kv.Key{groupRetirementIntentKey(friendGroupID)}); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	s.notifyFriendGroupRetirement(ctx, friendGroupID, intent)
	return intent.FriendGroup, nil
}

func (s *Server) ensureDataPendingDeletion(
	ctx context.Context,
	friendGroupID string,
	deletedAt time.Time,
) error {
	store, err := s.relationshipStore()
	if err != nil {
		return err
	}
	friendGroupID = strings.TrimSpace(friendGroupID)
	descriptor := retiredFriendGroupDataDescriptor{
		FriendGroupID: friendGroupID,
	}
	record, err := pendingdeletion.New(
		pendingdeletion.KindFriendGroup,
		friendGroupID,
		nil,
		pendingdeletion.ReasonFriendGroupDelete,
		descriptor,
		deletedAt,
	)
	if err != nil {
		return err
	}
	stored, _, err := pendingdeletion.CreateOrGet(ctx, store, record)
	if err != nil {
		return err
	}
	if stored.Reason != pendingdeletion.ReasonFriendGroupDelete {
		return fmt.Errorf(
			"social: Friend Group PendingDeletion %q has reason %q",
			friendGroupID,
			stored.Reason,
		)
	}
	var storedDescriptor retiredFriendGroupDataDescriptor
	if err := json.Unmarshal(stored.Descriptor, &storedDescriptor); err != nil {
		return fmt.Errorf(
			"social: decode Friend Group PendingDeletion descriptor %q: %w",
			friendGroupID,
			err,
		)
	}
	if strings.TrimSpace(storedDescriptor.FriendGroupID) != friendGroupID {
		return fmt.Errorf(
			"social: Friend Group PendingDeletion descriptor %q does not match Friend Group identity",
			friendGroupID,
		)
	}
	return nil
}

func (s *Server) rejectDataPendingDeletion(ctx context.Context, friendGroupID string) error {
	if s == nil || s.RelationshipStore == nil {
		return nil
	}
	pending, err := pendingdeletion.HasLocator(
		ctx,
		s.RelationshipStore,
		pendingdeletion.KindFriendGroup,
		strings.TrimSpace(friendGroupID),
	)
	if err != nil {
		return err
	}
	if pending {
		return fmt.Errorf(
			"%w: friend group %q cannot be reused",
			errFriendGroupPendingDeletion,
			friendGroupID,
		)
	}
	return nil
}

// ReconcileRetirementIntents completes relationship-first deletions that
// committed before the process could persist their Workspace PendingDeletion.
func (s *Server) ReconcileRetirementIntents(ctx context.Context) error {
	store, err := s.relationshipStore()
	if err != nil {
		return err
	}
	for entry, err := range store.List(ctx, retirementIntentsRoot) {
		if err != nil {
			return err
		}
		if len(entry.Key) != len(retirementIntentsRoot)+1 {
			continue
		}
		var intent retirementIntent
		if err := json.Unmarshal(entry.Value, &intent); err != nil {
			return err
		}
		friendGroupID := strings.TrimSpace(
			socialutil.UnescapeStoreSegment(entry.Key[len(retirementIntentsRoot)]),
		)
		if friendGroupID == "" || strings.TrimSpace(intent.FriendGroupID) != friendGroupID {
			return fmt.Errorf("social: invalid Friend Group retirement intent %q", friendGroupID)
		}
		unlock := s.lockGroup(friendGroupID)
		current, readErr := s.readRetirementIntent(ctx, friendGroupID)
		if errors.Is(readErr, kv.ErrNotFound) {
			unlock()
			continue
		}
		if readErr != nil {
			unlock()
			return readErr
		}
		lockedCtx, releasePeers, lockErr := s.lockRetirementPeers(ctx, current)
		if lockErr != nil {
			unlock()
			return lockErr
		}
		_, err = s.completeFriendGroupRetirement(lockedCtx, friendGroupID, current)
		releasePeers()
		unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) notifyFriendGroupRetirement(ctx context.Context, friendGroupID string, intent retirementIntent) {
	recipients := make([]string, 0, len(intent.Members))
	recipientNames := make(map[string]string, len(intent.Members))
	for _, member := range intent.Members {
		peerPublicKey := member.PeerPublicKey
		recipients = append(recipients, peerPublicKey)
		recipientNames[peerPublicKey] = member.FriendGroupName
	}
	s.notifyGroupWithNames(
		ctx,
		friendGroupID,
		intent.WorkspaceName,
		eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_DELETED,
		recipients,
		intent.DeletedAt,
		recipientNames,
	)
}

func (s *Server) notifyCurrentGroup(
	ctx context.Context,
	friendGroupID string,
	workspaceName string,
	change eventpb.FriendGroupChange,
	extraRecipients ...string,
) {
	s.notifyGroup(
		ctx,
		friendGroupID,
		workspaceName,
		change,
		s.groupRecipients(ctx, friendGroupID, extraRecipients...),
		s.now(),
		firstRecipient(extraRecipients),
	)
}

func firstRecipient(recipients []string) string {
	if len(recipients) == 0 {
		return ""
	}
	return strings.TrimSpace(recipients[0])
}

func (s *Server) groupRecipients(ctx context.Context, friendGroupID string, extraRecipients ...string) []string {
	recipients := append([]string(nil), extraRecipients...)
	members, err := s.listAllMembers(ctx, friendGroupID)
	if err != nil {
		return recipients
	}
	for _, member := range members {
		recipients = append(recipients, member.PeerPublicKey)
	}
	return recipients
}

func (s *Server) notifyGroup(
	ctx context.Context,
	friendGroupID string,
	workspaceName string,
	change eventpb.FriendGroupChange,
	recipients []string,
	at time.Time,
	affectedPeerPublicKey ...string,
) {
	s.notifyGroupWithNames(ctx, friendGroupID, workspaceName, change, recipients, at, nil, affectedPeerPublicKey...)
}

func (s *Server) notifyGroupWithNames(
	ctx context.Context,
	friendGroupID string,
	workspaceName string,
	change eventpb.FriendGroupChange,
	recipients []string,
	at time.Time,
	recipientNames map[string]string,
	affectedPeerPublicKey ...string,
) {
	if s == nil || s.NotifyPeer == nil {
		return
	}
	friendGroupID = strings.TrimSpace(friendGroupID)
	workspaceName = strings.TrimSpace(workspaceName)
	if workspaceName == "" {
		workspaceName = socialutil.GroupWorkspaceName(friendGroupID)
	}
	affectedPeer := firstRecipient(affectedPeerPublicKey)
	if recipientNames == nil {
		recipientNames = make(map[string]string)
	}
	if members, err := s.listAllMembers(ctx, friendGroupID); err == nil {
		for _, member := range members {
			peerPublicKey := member.PeerPublicKey
			if _, exists := recipientNames[peerPublicKey]; !exists {
				recipientNames[peerPublicKey] = member.FriendGroupName
			}
		}
	}
	seen := make(map[string]struct{}, len(recipients))
	for _, publicKey := range recipients {
		publicKey = strings.TrimSpace(publicKey)
		if publicKey == "" {
			continue
		}
		if _, exists := seen[publicKey]; exists {
			continue
		}
		friendGroupName := strings.TrimSpace(recipientNames[publicKey])
		if friendGroupName == "" {
			continue
		}
		seen[publicKey] = struct{}{}
		s.NotifyPeer(ctx, publicKey, &eventpb.PeerEvent{
			Version: eventpb.Version,
			Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_FRIEND_GROUP_UPDATED,
			Payload: &eventpb.PeerEvent_FriendGroupUpdated{
				FriendGroupUpdated: &eventpb.FriendGroupUpdated{
					FriendGroupName:       friendGroupName,
					WorkspaceName:         workspaceName,
					Change:                change,
					RevisionUnixMs:        at.UnixMilli(),
					AffectedPeerPublicKey: affectedPeer,
				},
			},
		})
	}
}

func (s *Server) readRetirementIntent(ctx context.Context, friendGroupID string) (retirementIntent, error) {
	store, err := s.relationshipStore()
	if err != nil {
		return retirementIntent{}, err
	}
	return socialutil.ReadJSONValue[retirementIntent](ctx, store, groupRetirementIntentKey(friendGroupID))
}

func (s *Server) relationshipStore() (kv.Store, error) {
	if s == nil || s.RelationshipStore == nil {
		return nil, errors.New("social: atomic friend group relationship store not configured")
	}
	return s.RelationshipStore, nil
}

func (s *Server) relationshipKey(prefix, key kv.Key) kv.Key {
	out := append(kv.Key{}, prefix...)
	return append(out, key...)
}

func groupRetirementIntentKey(friendGroupID string) kv.Key {
	return append(append(kv.Key{}, retirementIntentsRoot...), socialutil.EscapeStoreSegment(friendGroupID))
}

func groupRetirementReceiptKey(friendGroupID string) kv.Key {
	return append(append(kv.Key{}, retirementReceiptsRoot...), socialutil.EscapeStoreSegment(friendGroupID))
}

func workspaceBindingKey(friendGroupID string) kv.Key {
	return append(append(kv.Key{}, workspaceBindingsRoot...), socialutil.EscapeStoreSegment(friendGroupID))
}

func (s *Server) writeWorkspaceBinding(ctx context.Context, binding workspaceBinding) error {
	if binding.FriendGroupID == "" || binding.WorkspaceID == "" || binding.WorkspaceName == "" {
		return errors.New("social: FriendGroup Workspace binding identity is required")
	}
	store, err := s.relationshipStore()
	if err != nil {
		return err
	}
	return socialutil.WriteJSON(ctx, store, workspaceBindingKey(binding.FriendGroupID), binding)
}

func (s *Server) readWorkspaceBinding(ctx context.Context, friendGroupID string) (workspaceBinding, error) {
	store, err := s.relationshipStore()
	if err != nil {
		return workspaceBinding{}, err
	}
	binding, err := socialutil.ReadJSONValue[workspaceBinding](ctx, store, workspaceBindingKey(friendGroupID))
	if err != nil {
		return workspaceBinding{}, err
	}
	if binding.FriendGroupID != friendGroupID || binding.WorkspaceID == "" || binding.WorkspaceName == "" {
		return workspaceBinding{}, fmt.Errorf("social: invalid FriendGroup Workspace binding %q", friendGroupID)
	}
	return binding, nil
}

func (s *Server) deleteWorkspaceBinding(ctx context.Context, friendGroupID string) error {
	store, err := s.relationshipStore()
	if err != nil {
		return err
	}
	err = store.Delete(ctx, workspaceBindingKey(friendGroupID))
	if errors.Is(err, kv.ErrNotFound) {
		return nil
	}
	return err
}

func (s *Server) readRetirementReceipt(ctx context.Context, friendGroupID string) (retirementReceipt, error) {
	store, err := s.relationshipStore()
	if err != nil {
		return retirementReceipt{}, err
	}
	receipt, err := socialutil.ReadJSONValue[retirementReceipt](ctx, store, groupRetirementReceiptKey(friendGroupID))
	if err != nil {
		return retirementReceipt{}, err
	}
	if receipt.FriendGroupID != friendGroupID || receipt.Name == "" || receipt.WorkspaceID == "" || receipt.WorkspaceName == "" || receipt.Owner == "" || receipt.DeletedAt.IsZero() {
		return retirementReceipt{}, fmt.Errorf("social: invalid FriendGroup retirement receipt %q", friendGroupID)
	}
	return receipt, nil
}

func (s *Server) resolveRetiredFriendGroupName(ctx context.Context, owner, name string) (string, error) {
	store, err := s.relationshipStore()
	if err != nil {
		return "", err
	}
	owner = strings.TrimSpace(owner)
	name = strings.TrimSpace(name)
	for entry, err := range store.List(ctx, retirementReceiptsRoot) {
		if err != nil {
			return "", err
		}
		var receipt retirementReceipt
		if err := json.Unmarshal(entry.Value, &receipt); err != nil {
			return "", fmt.Errorf("social: decode FriendGroup retirement receipt: %w", err)
		}
		if receipt.Owner == owner && receipt.Name == name {
			if _, err := s.readRetirementReceipt(ctx, receipt.FriendGroupID); err != nil {
				return "", err
			}
			return receipt.FriendGroupID, nil
		}
	}
	return "", kv.ErrNotFound
}

func (s *Server) withMyRoleAndName(ctx context.Context, owner, friendGroupID string, group rpcapi.FriendGroupObject) (rpcapi.FriendGroupObject, error) {
	member, err := s.groupMember(ctx, friendGroupID, owner)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	role := socialutil.GroupRole(member)
	group.MyRole = &role
	group.Name = socialutil.StringValue(member.FriendGroupName)
	return group, nil
}

func (s *Server) resolveFriendGroupName(ctx context.Context, owner, name string) (string, error) {
	store, err := s.belongsStore()
	if err != nil {
		return "", err
	}
	owner = strings.TrimSpace(owner)
	name = strings.TrimSpace(name)
	if owner == "" || name == "" {
		return "", kv.ErrNotFound
	}
	id, err := store.Get(ctx, socialutil.GroupNameKey(owner, name))
	if err != nil {
		return "", err
	}
	return string(id), nil
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

func (s *Server) ensureGroupWorkspace(ctx context.Context, workspaceName, owner string) (apitypes.Workspace, bool, error) {
	created := false
	if s.Workspaces != nil {
		if s.RuntimeProfileForOwner == nil {
			return apitypes.Workspace{}, false, errors.New("social: runtime profile resolver is not configured")
		}
		profile, err := s.RuntimeProfileForOwner(ctx, owner)
		if err != nil {
			return apitypes.Workspace{}, false, err
		}
		body := adminhttp.WorkspaceUpsert{
			Id:         workspaceName,
			Name:       workspaceName,
			WorkflowId: profile.Spec.Workflows.System.GroupChatroom,
			Parameters: socialutil.ChatRoomWorkspaceParameters(apitypes.ChatRoomModeGroup),
		}
		workspace, wasCreated, err := s.Workspaces.CreateSystemWorkspace(ownership.WithOwner(ctx, owner), body)
		if err != nil {
			return apitypes.Workspace{}, false, err
		}
		if workspace.Id == "" || workspace.Name != workspaceName {
			return apitypes.Workspace{}, false, errors.New("social: created FriendGroup Workspace has invalid identity")
		}
		created = wasCreated
		return workspace, created, nil
	}
	return apitypes.Workspace{}, created, nil
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

func (s *Server) deleteBelongs(ctx context.Context, friendGroupID string, members []friendGroupMemberRecord) error {
	belongs, err := s.belongsStore()
	if err != nil {
		return err
	}
	for _, member := range members {
		peerID := member.PeerPublicKey
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
	_, err := s.Workspaces.DeleteSystemWorkspace(ctx, workspaceName)
	if errors.Is(err, kv.ErrNotFound) {
		return nil
	}
	return err
}

func (s *Server) restoreMember(ctx context.Context, friendGroupID, peerID string, current friendGroupMemberRecord, currentErr error) {
	members, membersErr := s.membersStore()
	belongs, belongsErr := s.belongsStore()
	if membersErr != nil || belongsErr != nil {
		return
	}
	if currentErr == nil {
		_ = socialutil.WriteJSON(ctx, members, socialutil.GroupMemberKey(friendGroupID, peerID), current)
		_ = socialutil.WriteJSON(ctx, belongs, socialutil.GroupBelongKey(peerID, friendGroupID), current)
		_ = belongs.Set(ctx, socialutil.GroupNameKey(peerID, current.FriendGroupName), []byte(friendGroupID))
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
	record, err := socialutil.ReadJSONValue[friendGroupMemberRecord](ctx, store, socialutil.GroupMemberKey(friendGroupID, peerID))
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := record.validate(); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	return record.peerObject(), nil
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

func (s *Server) listAllMembers(ctx context.Context, friendGroupID string) ([]friendGroupMemberRecord, error) {
	store, err := s.membersStore()
	if err != nil {
		return nil, err
	}
	prefix := append(append(kv.Key{}, socialutil.GroupMembersRoot...), socialutil.EscapeStoreSegment(friendGroupID))
	out := make([]friendGroupMemberRecord, 0)
	for entry, err := range store.List(ctx, prefix) {
		if err != nil {
			return nil, err
		}
		var item friendGroupMemberRecord
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return nil, err
		}
		if err := item.validate(); err != nil {
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
