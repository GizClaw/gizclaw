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
	"github.com/GizClaw/gizclaw-go/pkgs/internal/keyedlock"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type WorkspaceService interface {
	CreateSystemWorkspace(context.Context, adminhttp.WorkspaceUpsert) (apitypes.Workspace, bool, error)
	DeleteSystemWorkspace(context.Context, string) (apitypes.Workspace, error)
	GetRetiredSystemWorkspaceByID(context.Context, string, socialutil.SFUWorkspaceKind, string) (apitypes.Workspace, error)
	RetireSystemWorkspaceByID(context.Context, string, socialutil.SFUWorkspaceKind, string) (apitypes.Workspace, error)
}

var (
	// ErrFriendGroupFull reports that the Group already holds
	// socialutil.FriendGroupMemberLimit members, including the owner.
	ErrFriendGroupFull = errors.New("social: friend group is full")
	// ErrPeerFriendGroupLimit reports that the Peer already belongs to
	// socialutil.PeerFriendGroupLimit Friend Groups.
	ErrPeerFriendGroupLimit = errors.New("social: peer friend group limit reached")
	// ErrSFUNotConfigured reports that the Server has no SFU URL, so no Friend
	// Group Workspace can be bound to an SFU Room.
	ErrSFUNotConfigured = errors.New("social: SFU is not configured")
)

// Server owns Friend Groups. Every record it writes lives in the shared Social
// KV, so any Server in the deployment can create, join, resolve, or retire a
// Group without consulting Server-local state.
type Server struct {
	Groups           kv.Store
	InviteTokens     kv.Store
	Members          kv.Store
	Belongs          kv.Store
	Workspaces       WorkspaceService
	NotifyPeer       func(context.Context, string, *eventpb.PeerEvent)
	PeerAvailability func(context.Context, string) error
	// Profiles and Pings serve server.friend_group.ping: Profiles names the
	// rallying member and Pings reaches member devices connected to this
	// Server. A nil Pings disables rallies.
	Profiles ProfileService
	Pings    socialutil.PingDelivery
	// Presence reports member device presence for server.friend_group.members.list;
	// nil leaves online and last_seen_at out of every listed member.
	Presence socialutil.PresenceService
	// SFUURL is the SFU endpoint recorded in every new Friend Group SFU binding.
	SFUURL string

	// RelationshipStore scopes Workspace bindings, retirement records and
	// pending-deletion work to the configured namespace. It must share an
	// atomic transaction boundary with Groups, Members, Belongs and InviteTokens;
	// it must not discard its namespace when that boundary is resolved.
	RelationshipStore kv.Store

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
	ids, err := belongs.ListMembers(ctx, belongCollectionKey(peerID))
	if err != nil {
		return nil, err
	}
	slices.Sort(ids)
	var out []PeerRetirementGroup
	for _, id := range ids {
		member, err := socialutil.ReadJSONValue[friendGroupMemberRecord](ctx, belongs, socialutil.GroupBelongKey(peerID, id))
		if err != nil {
			return nil, err
		}
		if member.FriendGroupID != id {
			return nil, errors.New("social: group collection identity mismatch")
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
			for index := range slices.Backward(releases) {
				releases[index]()
			}
			return nil, err
		}
		releases = append(releases, release)
	}
	return func() {
		for index := range slices.Backward(releases) {
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

// workspaceBinding is the canonical Group-level Workspace record. It carries
// the SFU Room binding so every Server can materialize the same Workspace and
// attach to the same Room. Generation increases on every binding replacement.
type workspaceBinding struct {
	FriendGroupID string                `json:"friend_group_id"`
	WorkspaceID   string                `json:"workspace_id"`
	WorkspaceName string                `json:"workspace_name"`
	Owner         string                `json:"owner"`
	SFU           socialutil.SFUBinding `json:"sfu"`
}

func (binding workspaceBinding) sfuWorkspaceBinding(members []string) socialutil.SFUWorkspaceBinding {
	return socialutil.SFUWorkspaceBinding{
		WorkspaceID:   binding.WorkspaceID,
		WorkspaceName: binding.WorkspaceName,
		Kind:          socialutil.SFUWorkspaceKindFriendGroup,
		SocialID:      binding.FriendGroupID,
		Owner:         binding.Owner,
		Members:       members,
		SFU:           binding.SFU,
	}
}

type retirementReceipt struct {
	FriendGroupID string                    `json:"friend_group_id"`
	Name          string                    `json:"name,omitempty"`
	WorkspaceID   string                    `json:"workspace_id"`
	WorkspaceName string                    `json:"workspace_name"`
	Owner         string                    `json:"owner"`
	DeletedAt     time.Time                 `json:"deleted_at"`
	Members       []friendGroupMemberRecord `json:"members"`
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
	// ErrFriendGroupPendingDeletion reports a Group whose retirement has
	// started and whose data can no longer be read or changed.
	ErrFriendGroupPendingDeletion     = errors.New("social: friend group is pending deletion")
	ErrFriendGroupMemberAlreadyExists = errors.New("social: friend group member already exists")
	// ErrFriendGroupPermissionDenied reports a caller whose role in the Group
	// does not permit the operation.
	ErrFriendGroupPermissionDenied = errors.New("social: friend group permission denied")
	// ErrFriendGroupNameExists reports a Peer-local Group name already bound
	// to a different Group.
	ErrFriendGroupNameExists = errors.New("social: friend group name already exists")
	// ErrFriendGroupMembershipNameImmutable reports an attempt to rename an
	// existing membership through join or member writes.
	ErrFriendGroupMembershipNameImmutable = errors.New("social: friend group membership name is immutable")
	// ErrFriendGroupOwnerCannotBeRemoved reports an attempt to delete the owner
	// membership; the owner deletes the Group instead.
	ErrFriendGroupOwnerCannotBeRemoved = errors.New("social: cannot delete friend group owner")
	// ErrFriendGroupOwnerCannotLeave reports the owner leaving its own Group.
	ErrFriendGroupOwnerCannotLeave = errors.New("social: friend group owner cannot leave")
	// ErrFriendGroupOwnerRoleImmutable reports an attempt to change the owner
	// membership role.
	ErrFriendGroupOwnerRoleImmutable = errors.New("social: cannot change owner role")
	// ErrInviteTokenUnavailable reports a missing or expired Group invite token.
	ErrInviteTokenUnavailable = errors.New("social: invite token not found")
	// ErrFriendGroupMemberNotFound reports a target Peer that is not a member
	// of the Group. It wraps kv.ErrNotFound.
	ErrFriendGroupMemberNotFound = fmt.Errorf("social: friend group member not found: %w", kv.ErrNotFound)
	// ErrInvalidMemberRole reports a role outside the writable member roles.
	ErrInvalidMemberRole = errors.New("social: invalid group member role")
)

func (s *Server) CreateFriendGroup(ctx context.Context, owner string, req rpcapi.FriendGroupCreateRequest) (rpcapi.FriendGroupObject, error) {
	if _, err := s.groupsStore(); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	owner = strings.TrimSpace(owner)
	name := strings.TrimSpace(req.Name)
	if owner == "" || name == "" {
		return rpcapi.FriendGroupObject{}, errors.New("social: friend group owner and name are required")
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
	if err := s.checkGroupCreate(ctx, id, owner, name); err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	binding, err := s.newWorkspaceBinding(id, owner)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	workspace, createdWorkspace, err := s.ensureGroupWorkspace(ctx, workspaceName, owner)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	binding.WorkspaceID, binding.WorkspaceName = workspace.Id, workspace.Name
	if err := s.createGroupWithOwner(ctx, id, group, binding, owner, name); err != nil {
		if createdWorkspace && !errors.Is(err, errGroupCreateUncertain) {
			err = errors.Join(err, s.deleteWorkspace(ctx, workspaceName))
		}
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
	if _, err := s.groupsStore(); err != nil {
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
	if err := s.checkGroupCreate(ctx, id, owner, name); err != nil {
		return adminhttp.AdminFriendGroupObject{}, err
	}
	binding, err := s.newWorkspaceBinding(id, owner)
	if err != nil {
		return adminhttp.AdminFriendGroupObject{}, err
	}
	workspace, createdWorkspace, err := s.ensureGroupWorkspace(ctx, workspaceName, owner)
	if err != nil {
		return adminhttp.AdminFriendGroupObject{}, err
	}
	binding.WorkspaceID, binding.WorkspaceName = workspace.Id, workspace.Name
	if err := s.createGroupWithOwner(ctx, id, group, binding, owner, name); err != nil {
		if createdWorkspace && !errors.Is(err, errGroupCreateUncertain) {
			err = errors.Join(err, s.deleteWorkspace(ctx, workspaceName))
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
	entries, err := listAdminGroupRecords(ctx, store, socialutil.StringValue(req.Cursor), socialutil.IntValue(req.Limit))
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
		if errors.Is(err, ErrFriendGroupPendingDeletion) {
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
	escapedCursor, limit := socialutil.NormalizeListParams(socialutil.StringValue(req.Cursor), socialutil.IntValue(req.Limit))
	ids, err := belongs.RangeOrderedMembers(ctx, belongPageKey(owner), kv.OrderedRange{After: &escapedCursor, Limit: limit + 1})
	if err != nil {
		return rpcapi.FriendGroupListResponse{}, err
	}
	hasNext := len(ids) > limit
	ids = ids[:min(limit, len(ids))]
	var nextCursor *string
	if hasNext {
		nextCursor = new(socialutil.UnescapeStoreSegment(ids[len(ids)-1]))
	}
	items := make([]rpcapi.FriendGroupObject, 0, len(ids))
	for _, escapedID := range ids {
		id := socialutil.UnescapeStoreSegment(escapedID)
		member, err := socialutil.ReadJSONValue[friendGroupMemberRecord](ctx, belongs, socialutil.GroupBelongKey(owner, id))
		if errors.Is(err, kv.ErrNotFound) {
			continue
		}
		if err != nil {
			return rpcapi.FriendGroupListResponse{}, err
		}
		if err := member.validate(); err != nil {
			return rpcapi.FriendGroupListResponse{}, err
		}
		if member.FriendGroupID != id || member.PeerPublicKey != owner {
			return rpcapi.FriendGroupListResponse{}, errors.New("social: group collection identity mismatch")
		}
		item, err := socialutil.ReadJSONValue[rpcapi.FriendGroupObject](ctx, store, socialutil.GroupKey(id))
		if errors.Is(err, kv.ErrNotFound) {
			continue
		}
		if err != nil {
			return rpcapi.FriendGroupListResponse{}, err
		}
		item.MyRole = new(member.Role)
		item.Name = member.FriendGroupName
		items = append(items, item)
	}
	return rpcapi.FriendGroupListResponse{Items: items, HasNext: hasNext, NextCursor: nextCursor}, nil
}

func (s *Server) WorkspaceRecipientsByID(ctx context.Context, workspaceID string) ([]string, error) {
	if err := customid.ValidateResourceID(workspaceID); err != nil {
		return nil, fmt.Errorf("social: invalid workspace id: %w", err)
	}
	store, err := s.relationshipStore()
	if err != nil {
		return nil, err
	}
	locator, err := socialutil.ReadJSONValue[socialutil.WorkspaceBindingLocator](ctx, store, socialutil.WorkspaceLocatorIDKey(workspaceID))
	if err != nil {
		return nil, err
	}
	if err := locator.Validate(); err != nil {
		return nil, err
	}
	if locator.WorkspaceID != workspaceID {
		return nil, errors.New("social: workspace locator identity mismatch")
	}
	binding, err := s.readWorkspaceBinding(ctx, locator.ResourceID)
	if err != nil {
		return nil, err
	}
	if binding.WorkspaceID != workspaceID || binding.WorkspaceName != locator.WorkspaceName {
		return nil, kv.ErrNotFound
	}
	return s.memberPublicKeys(ctx, binding.FriendGroupID)
}

func (s *Server) AdminListFriendGroups(ctx context.Context, req rpcapi.FriendGroupListRequest) (rpcapi.FriendGroupListResponse, error) {
	store, err := s.groupsStore()
	if err != nil {
		return rpcapi.FriendGroupListResponse{}, err
	}
	entries, err := listAdminGroupRecords(ctx, store, socialutil.StringValue(req.Cursor), socialutil.IntValue(req.Limit))
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
	return s.CreateFriendGroupInviteTokenWithTTL(ctx, owner, req, 0)
}

// CreateFriendGroupInviteTokenWithTTL returns the Group's active invite token
// or creates one; only the owner may call it. A zero ttl keeps the
// server.friend_group.invite_token.create behavior: an active token is
// returned unchanged and a new one lives for socialutil.DefaultInviteTokenTTL.
// A non-zero ttl must pass socialutil.ValidateInviteTokenTTL; an active token
// keeps its value and its expiry is extended to now+ttl, never shortened.
func (s *Server) CreateFriendGroupInviteTokenWithTTL(ctx context.Context, owner string, req rpcapi.FriendGroupInviteTokenCreateRequest, ttl time.Duration) (rpcapi.FriendGroupInviteTokenCreateResponse, error) {
	if ttl != 0 {
		if err := socialutil.ValidateInviteTokenTTL(ttl); err != nil {
			return rpcapi.FriendGroupInviteTokenCreateResponse{}, err
		}
	}
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
	now := s.now()
	record, ok, err := s.activeGroupInviteToken(ctx, store, friendGroupID)
	if err != nil {
		return rpcapi.FriendGroupInviteTokenCreateResponse{}, err
	}
	if ok {
		if ttl == 0 || !record.ExpiresAt.Before(now.Add(ttl)) {
			return rpcapi.FriendGroupInviteTokenCreateResponse{InviteToken: record.InviteToken, ExpiresAt: record.ExpiresAt}, nil
		}
		record.ExpiresAt = now.Add(ttl)
	} else {
		if ttl == 0 {
			ttl = s.inviteTokenTTL()
		}
		record = inviteTokenRecord{
			FriendGroupID: friendGroupID,
			InviteToken:   s.newID(),
			CreatedAt:     now,
			ExpiresAt:     now.Add(ttl),
		}
	}
	if strings.TrimSpace(record.InviteToken) == "" {
		return rpcapi.FriendGroupInviteTokenCreateResponse{}, errors.New("social: invite token is empty")
	}
	if err := socialutil.WriteInviteToken(ctx, store, socialutil.GroupInviteTokenKey(friendGroupID), record); err != nil {
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
	if err := socialutil.DeleteInviteToken(ctx, store, socialutil.GroupInviteTokenKey(friendGroupID)); err != nil && !errors.Is(err, kv.ErrNotFound) {
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
	if err := socialutil.WriteInviteToken(ctx, store, socialutil.GroupInviteTokenKey(friendGroupID), record); err != nil {
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
	if err := socialutil.DeleteInviteToken(ctx, store, socialutil.GroupInviteTokenKey(friendGroupID)); err != nil && !errors.Is(err, kv.ErrNotFound) {
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
	if existingID, err := s.resolveFriendGroupName(ctx, owner, name); err == nil && existingID != friendGroupID {
		return rpcapi.FriendGroupJoinResponse{}, ErrFriendGroupNameExists
	} else if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	unlock := s.lockGroup(friendGroupID)
	defer unlock()
	if existing, err := s.groupMember(ctx, friendGroupID, owner); err == nil {
		if socialutil.StringValue(existing.FriendGroupName) != name {
			return rpcapi.FriendGroupJoinResponse{}, ErrFriendGroupMembershipNameImmutable
		}
		group, err := s.GetFriendGroup(ctx, owner, rpcapi.FriendGroupGetRequest{Name: name})
		if err != nil {
			return rpcapi.FriendGroupJoinResponse{}, err
		}
		return rpcapi.FriendGroupJoinResponse{Group: group, Member: existing}, nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	if err := s.requireMemberCapacity(ctx, friendGroupID); err != nil {
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	member, err := s.writeMember(ctx, friendGroupID, owner, rpcapi.FriendGroupMemberRoleMember, name)
	if err != nil {
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	group, err = s.GetFriendGroup(ctx, owner, rpcapi.FriendGroupGetRequest{Name: name})
	if err != nil {
		err = errors.Join(err, s.removeMember(ctx, friendGroupID, owner, member))
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
	if _, err := s.AdminGetFriendGroup(ctx, friendGroupID); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	memberName := strings.TrimSpace(req.MemberName)
	req.PeerPublicKey = strings.TrimSpace(req.PeerPublicKey)
	if memberName == "" || !req.Role.Valid() {
		return rpcapi.FriendGroupMemberObject{}, ErrInvalidMemberRole
	}
	if existingID, err := s.resolveFriendGroupName(ctx, req.PeerPublicKey, memberName); err == nil {
		if existingID != friendGroupID {
			return rpcapi.FriendGroupMemberObject{}, fmt.Errorf("%w for target member", ErrFriendGroupNameExists)
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
		return rpcapi.FriendGroupMemberObject{}, ErrFriendGroupOwnerRoleImmutable
	}
	if currentErr != nil {
		if err := s.requireMemberCapacity(ctx, friendGroupID); err != nil {
			return rpcapi.FriendGroupMemberObject{}, err
		}
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
	if _, err := s.AdminGetFriendGroup(ctx, friendGroupID); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	req.Name = strings.TrimSpace(req.Name)
	if !req.Role.Valid() {
		return rpcapi.FriendGroupMemberObject{}, ErrInvalidMemberRole
	}
	unlock := s.lockGroup(friendGroupID)
	defer unlock()
	if err := s.requireRole(ctx, owner, friendGroupID, rpcapi.FriendGroupMemberRoleOwner); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	current, err := s.targetMember(ctx, friendGroupID, req.Name)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if current.Role != nil && *current.Role == rpcapi.FriendGroupMemberRoleOwner {
		return rpcapi.FriendGroupMemberObject{}, ErrFriendGroupOwnerRoleImmutable
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
	return s.deleteFriendGroupMember(ctx, owner, req, false)
}

// LeaveFriendGroup removes the caller's own membership from the Group it
// names friendGroupName. Members and admins may leave; the owner receives
// ErrFriendGroupOwnerCannotLeave and deletes the Group instead.
func (s *Server) LeaveFriendGroup(ctx context.Context, owner, friendGroupName string) (rpcapi.FriendGroupMemberObject, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: peer public key is required")
	}
	return s.deleteFriendGroupMember(ctx, owner, rpcapi.FriendGroupMemberDeleteRequest{FriendGroupName: friendGroupName, Name: owner}, true)
}

// deleteFriendGroupMember removes req.Name from the Group. A leave removes the
// caller itself, which needs no role beyond the membership being removed.
func (s *Server) deleteFriendGroupMember(ctx context.Context, owner string, req rpcapi.FriendGroupMemberDeleteRequest, leave bool) (rpcapi.FriendGroupMemberObject, error) {
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
	current, err := s.targetMember(ctx, friendGroupID, req.Name)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	role := socialutil.GroupRole(current)
	switch {
	case role == rpcapi.FriendGroupMemberRoleOwner && leave:
		return rpcapi.FriendGroupMemberObject{}, ErrFriendGroupOwnerCannotLeave
	case role == rpcapi.FriendGroupMemberRoleOwner:
		return rpcapi.FriendGroupMemberObject{}, ErrFriendGroupOwnerCannotBeRemoved
	case leave:
		// A member or admin removing itself needs no further role.
	case role == rpcapi.FriendGroupMemberRoleAdmin:
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
	if err := s.removeMember(ctx, friendGroupID, req.Name, current); err != nil {
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
	page, err := s.listFriendGroupMembers(ctx, friendGroupID, socialutil.StringValue(req.Cursor), socialutil.IntValue(req.Limit))
	if err != nil {
		return rpcapi.FriendGroupMemberListResponse{}, err
	}
	for i := range page.Items {
		item := &page.Items[i]
		item.Online, item.LastSeenAt = socialutil.PresenceFields(ctx, s.Presence, socialutil.StringValue(item.PeerPublicKey))
	}
	return page, nil
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
		return rpcapi.FriendGroupMemberObject{}, ErrInvalidMemberRole
	}
	group, err := s.AdminGetFriendGroup(ctx, friendGroupID)
	if err != nil {
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
	if err := s.requireMemberCapacity(ctx, friendGroupID); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
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
		return rpcapi.FriendGroupMemberObject{}, ErrInvalidMemberRole
	}
	group, err := s.AdminGetFriendGroup(ctx, friendGroupID)
	if err != nil {
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
	if currentErr != nil {
		if err := s.requireMemberCapacity(ctx, friendGroupID); err != nil {
			return rpcapi.FriendGroupMemberObject{}, err
		}
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
	if err := s.removeMember(ctx, friendGroupID, peerID, current); err != nil {
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
	escapedCursor, limit := socialutil.NormalizeListParams(cursor, limit)
	store, err := s.membersStore()
	if err != nil {
		return rpcapi.FriendGroupMemberListResponse{}, err
	}
	peers, err := store.ListMembers(ctx, memberCollectionKey(strings.TrimSpace(friendGroupID)))
	if err != nil {
		return rpcapi.FriendGroupMemberListResponse{}, err
	}
	slices.SortFunc(peers, func(a, b string) int {
		return strings.Compare(socialutil.EscapeStoreSegment(a), socialutil.EscapeStoreSegment(b))
	})
	items := make([]rpcapi.FriendGroupMemberObject, 0, min(limit+1, len(peers)))
	for _, peer := range peers {
		if escapedCursor != "" && socialutil.EscapeStoreSegment(peer) <= escapedCursor {
			continue
		}
		record, err := socialutil.ReadJSONValue[friendGroupMemberRecord](ctx, store, socialutil.GroupMemberKey(friendGroupID, peer))
		if errors.Is(err, kv.ErrNotFound) {
			continue
		}
		if err != nil {
			return rpcapi.FriendGroupMemberListResponse{}, err
		}
		if err := record.validate(); err != nil {
			return rpcapi.FriendGroupMemberListResponse{}, err
		}
		if record.FriendGroupID != friendGroupID || record.PeerPublicKey != peer {
			return rpcapi.FriendGroupMemberListResponse{}, errors.New("social: member collection identity mismatch")
		}
		items = append(items, record.peerObject())
		if len(items) > limit {
			break
		}
	}
	if len(items) > limit {
		return rpcapi.FriendGroupMemberListResponse{Items: items[:limit], HasNext: true, NextCursor: new(items[limit-1].Name)}, nil
	}
	return rpcapi.FriendGroupMemberListResponse{Items: items}, nil
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
		return rpcapi.FriendGroupMemberObject{}, ErrInvalidMemberRole
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
	guard, err := s.readGroupMutation(ctx, friendGroupID, members, belongs)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	now := s.now()
	currentData, currentErr := members.Get(ctx, socialutil.GroupMemberKey(friendGroupID, peerID))
	var current friendGroupMemberRecord
	if currentErr == nil {
		currentErr = json.Unmarshal(currentData, &current)
	}
	var item friendGroupMemberRecord
	if currentErr == nil {
		if err := current.validate(); err != nil {
			return rpcapi.FriendGroupMemberObject{}, err
		}
		if localName == "" {
			localName = current.FriendGroupName
		}
		if localName != "" && current.FriendGroupName != localName {
			return rpcapi.FriendGroupMemberObject{}, ErrFriendGroupMembershipNameImmutable
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
		return rpcapi.FriendGroupMemberObject{}, ErrFriendGroupNameExists
	} else if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if errors.Is(currentErr, kv.ErrNotFound) {
		return s.createMember(ctx, friendGroupID, peerID, role, localName)
	}
	prefixes := guard.prefixes
	data, err := json.Marshal(item)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	memberKey := s.relationshipKey(prefixes[0], socialutil.GroupMemberKey(friendGroupID, peerID))
	changed, err := guard.apply(ctx, kv.Mutation{
		Conditions:        []kv.Condition{{Key: memberKey, Expected: currentData}},
		Entries:           []kv.Entry{{Key: memberKey, Value: data}, {Key: s.relationshipKey(prefixes[1], socialutil.GroupBelongKey(peerID, friendGroupID)), Value: data}, {Key: s.relationshipKey(prefixes[1], socialutil.GroupNameKey(peerID, localName)), Value: []byte(friendGroupID)}},
		AddMembers:        []kv.SetMembers{{Key: s.relationshipKey(prefixes[0], memberCollectionKey(friendGroupID)), Members: []string{peerID}}, {Key: s.relationshipKey(prefixes[1], belongCollectionKey(peerID)), Members: []string{friendGroupID}}},
		AddOrderedMembers: []kv.SetMembers{{Key: s.relationshipKey(prefixes[1], belongPageKey(peerID)), Members: []string{socialutil.EscapeStoreSegment(friendGroupID)}}},
	}, false)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if !changed {
		return rpcapi.FriendGroupMemberObject{}, errors.New("social: group membership changed concurrently")
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
		return rpcapi.FriendGroupMemberObject{}, ErrInvalidMemberRole
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
	guard, err := s.readGroupMutation(ctx, friendGroupID, members, belongs)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if err := s.requireMemberCapacity(ctx, friendGroupID); err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	peerRevision, err := s.readPeerGroupCapacity(ctx, peerID)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	store, prefixes := guard.store, guard.prefixes
	memberKey := s.relationshipKey(prefixes[0], socialutil.GroupMemberKey(friendGroupID, peerID))
	belongKey := s.relationshipKey(prefixes[1], socialutil.GroupBelongKey(peerID, friendGroupID))
	nameKey := s.relationshipKey(prefixes[1], socialutil.GroupNameKey(peerID, localName))
	mutation := kv.Mutation{
		Conditions:        []kv.Condition{{Key: memberKey}, {Key: nameKey}},
		Entries:           []kv.Entry{{Key: memberKey, Value: data}, {Key: nameKey, Value: []byte(friendGroupID)}, {Key: belongKey, Value: data}},
		AddMembers:        []kv.SetMembers{{Key: s.relationshipKey(prefixes[0], memberCollectionKey(friendGroupID)), Members: []string{peerID}}, {Key: s.relationshipKey(prefixes[1], belongCollectionKey(peerID)), Members: []string{friendGroupID}}},
		AddOrderedMembers: []kv.SetMembers{{Key: s.relationshipKey(prefixes[1], belongPageKey(peerID)), Members: []string{socialutil.EscapeStoreSegment(friendGroupID)}}},
	}
	s.admitPeerToGroup(&mutation, prefixes[1], peerID, peerRevision)
	created, err := guard.apply(ctx, mutation, false)
	if err != nil {
		return rpcapi.FriendGroupMemberObject{}, err
	}
	if !created {
		if _, err := store.Get(ctx, memberKey); err == nil {
			return rpcapi.FriendGroupMemberObject{}, ErrFriendGroupMemberAlreadyExists
		} else if !errors.Is(err, kv.ErrNotFound) {
			return rpcapi.FriendGroupMemberObject{}, err
		}
		if _, err := store.Get(ctx, nameKey); err == nil {
			return rpcapi.FriendGroupMemberObject{}, ErrFriendGroupNameExists
		} else if !errors.Is(err, kv.ErrNotFound) {
			return rpcapi.FriendGroupMemberObject{}, err
		}
		return rpcapi.FriendGroupMemberObject{}, ErrGroupChanged
	}

	return item.peerObject(), nil
}

func (s *Server) putFriendGroup(ctx context.Context, friendGroupID string, displayName, description *string) (rpcapi.FriendGroupObject, error) {
	guard, err := s.readGroupMutation(ctx, friendGroupID)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	var group rpcapi.FriendGroupObject
	if err := json.Unmarshal(guard.groupData, &group); err != nil {
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
	data, err := json.Marshal(group)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	changed, err := guard.apply(ctx, kv.Mutation{Entries: []kv.Entry{{Key: guard.groupKey, Value: data}}}, false)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if !changed {
		return rpcapi.FriendGroupObject{}, ErrGroupChanged
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
	store, err := s.relationshipStore()
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	memberStore, err := s.membersStore()
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	belongStore, err := s.belongsStore()
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	inviteStore, err := s.groupInviteTokensStore()
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	guard, err := s.readGroupMutation(ctx, friendGroupID, memberStore, belongStore, inviteStore, store)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if err := json.Unmarshal(guard.groupData, &group); err != nil {
		return rpcapi.FriendGroupObject{}, err
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
		s.relationshipKey(guard.groupPrefix, socialutil.GroupKey(friendGroupID)),
		s.relationshipKey(guard.prefixes[2], socialutil.GroupInviteTokenKey(friendGroupID)),
		s.relationshipKey(guard.prefixes[3], workspaceBindingKey(friendGroupID)),
	}
	for _, member := range members {
		peerID := strings.TrimSpace(member.PeerPublicKey)
		deleteKeys = append(
			deleteKeys,
			s.relationshipKey(guard.prefixes[0], socialutil.GroupMemberKey(friendGroupID, peerID)),
			s.relationshipKey(guard.prefixes[1], socialutil.GroupBelongKey(peerID, friendGroupID)),
			s.relationshipKey(guard.prefixes[1], socialutil.GroupNameKey(peerID, member.FriendGroupName)),
		)
	}
	deleteKeys = append(deleteKeys, s.relationshipKey(guard.prefixes[0], memberCollectionKey(friendGroupID)))
	removals := make([]kv.SetMembers, 0, len(members))
	orderedRemovals := make([]kv.SetMembers, 0, len(members)+1)
	for _, member := range members {
		removals = append(removals, kv.SetMembers{Key: s.relationshipKey(guard.prefixes[1], belongCollectionKey(member.PeerPublicKey)), Members: []string{friendGroupID}})
		orderedRemovals = append(orderedRemovals, kv.SetMembers{Key: s.relationshipKey(guard.prefixes[1], belongPageKey(member.PeerPublicKey)), Members: []string{socialutil.EscapeStoreSegment(friendGroupID)}})
	}
	inviteKey := s.relationshipKey(guard.prefixes[2], socialutil.GroupInviteTokenKey(friendGroupID))
	inviteData, err := guard.store.Get(ctx, inviteKey)
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendGroupObject{}, err
	}
	if inviteData != nil {
		var invite inviteTokenRecord
		if err := json.Unmarshal(inviteData, &invite); err != nil {
			return rpcapi.FriendGroupObject{}, err
		}
		if invite.InviteToken != "" {
			deleteKeys = append(deleteKeys, s.relationshipKey(guard.prefixes[2],
				socialutil.InviteTokenIndexKey(socialutil.GroupInviteTokensRoot, invite.InviteToken)))
		}
	}
	adminMembership := adminGroupMembership(friendGroupID)
	adminMembership.Key = s.relationshipKey(guard.groupPrefix, adminMembership.Key)
	recoveryMembers := (socialutil.RecoveryIndex{Root: retirementIntentsRoot}).Add(friendGroupID)
	for i := range recoveryMembers {
		recoveryMembers[i].Key = s.relationshipKey(guard.prefixes[3], recoveryMembers[i].Key)
	}
	committed, err := guard.apply(ctx, kv.Mutation{
		Conditions: []kv.Condition{{Key: inviteKey, Expected: inviteData}},
		Entries:    []kv.Entry{{Key: s.relationshipKey(guard.prefixes[3], groupRetirementIntentKey(friendGroupID)), Value: data}},
		AddMembers: recoveryMembers,
		DeleteKeys: deleteKeys, RemoveMembers: removals,
		RemoveOrderedMembers: append(orderedRemovals, adminMembership),
	}, true)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if !committed {
		return rpcapi.FriendGroupObject{}, ErrGroupChanged
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
		socialutil.SFUWorkspaceKindFriendGroup,
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
		socialutil.SFUWorkspaceKindFriendGroup,
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
		Members:       intent.Members,
	}
	receiptData, err := json.Marshal(receipt)
	if err != nil {
		return rpcapi.FriendGroupObject{}, err
	}
	if err := s.commitRetirementReceipt(ctx, store, receipt, receiptData); err != nil {
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
			ErrFriendGroupPendingDeletion,
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
	for friendGroupID, err := range (socialutil.RecoveryIndex{Root: retirementIntentsRoot}).IDs(ctx, store) {
		if err != nil {
			return err
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
	members, err := s.memberPublicKeys(ctx, friendGroupID)
	if err != nil {
		return recipients
	}
	return append(recipients, members...)
}

func (s *Server) memberPublicKeys(ctx context.Context, friendGroupID string) ([]string, error) {
	store, err := s.membersStore()
	if err != nil {
		return nil, err
	}
	members, err := store.ListMembers(ctx, memberCollectionKey(friendGroupID))
	if err != nil {
		return nil, err
	}
	slices.Sort(members)
	return members, nil
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
	intent, err := socialutil.ReadJSONValue[retirementIntent](ctx, store, groupRetirementIntentKey(friendGroupID))
	if err != nil {
		return retirementIntent{}, err
	}
	if intent.FriendGroupID != friendGroupID {
		return retirementIntent{}, errors.New("social: retirement intent identity mismatch")
	}
	return intent, nil
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

// newWorkspaceBinding mints the SFU Room identity of a new Group lifecycle.
// The token is random and never derived from the Group ID, so Group ID reuse
// protection and Room identity uniqueness stay independent.
func (s *Server) newWorkspaceBinding(friendGroupID, owner string) (workspaceBinding, error) {
	sfuURL := strings.TrimSpace(s.SFUURL)
	if sfuURL == "" {
		return workspaceBinding{}, ErrSFUNotConfigured
	}
	roomToken, err := socialutil.NewRoomToken()
	if err != nil {
		return workspaceBinding{}, err
	}
	return workspaceBinding{
		FriendGroupID: friendGroupID,
		Owner:         owner,
		SFU:           socialutil.SFUBinding{URL: sfuURL, RoomToken: roomToken, Generation: 1},
	}, nil
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
	if err := validateWorkspaceBinding(binding, friendGroupID); err != nil {
		return workspaceBinding{}, err
	}
	return binding, nil
}

func validateWorkspaceBinding(binding workspaceBinding, friendGroupID string) error {
	if binding.FriendGroupID == "" || binding.FriendGroupID != friendGroupID ||
		binding.WorkspaceID == "" || binding.WorkspaceName == "" ||
		binding.Owner == "" || binding.Owner != strings.TrimSpace(binding.Owner) ||
		binding.SFU.Generation == 0 {
		return fmt.Errorf("social: invalid FriendGroup Workspace binding %q", friendGroupID)
	}
	if err := binding.SFU.Validate(); err != nil {
		return fmt.Errorf("social: invalid FriendGroup Workspace binding %q: %w", friendGroupID, err)
	}
	return nil
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
	data, err := store.Get(ctx, retiredGroupNameKey(owner, name))
	if err != nil {
		return "", err
	}
	receipt, err := s.readRetirementReceipt(ctx, string(data))
	if err != nil {
		return "", err
	}
	if receipt.Owner != owner || receipt.Name != name {
		return "", errors.New("social: retired Friend Group name index identity mismatch")
	}
	return receipt.FriendGroupID, nil
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
		return fmt.Errorf("%w: admin required", ErrFriendGroupPermissionDenied)
	}
	return nil
}

func (s *Server) requireRole(ctx context.Context, owner, friendGroupID string, required rpcapi.FriendGroupMemberRole) error {
	member, err := s.groupMember(ctx, friendGroupID, owner)
	if err != nil {
		return err
	}
	if socialutil.GroupRole(member) != required {
		return fmt.Errorf("%w: role %s required", ErrFriendGroupPermissionDenied, required)
	}
	return nil
}

func (s *Server) ensureGroupWorkspace(ctx context.Context, workspaceName, owner string) (apitypes.Workspace, bool, error) {
	created := false
	if s.Workspaces != nil {
		body := adminhttp.WorkspaceUpsert{
			Id:         workspaceName,
			Name:       workspaceName,
			WorkflowId: socialutil.SFUWorkflowID,
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

// targetMember reads the membership a member write targets, reporting a
// missing one as ErrFriendGroupMemberNotFound.
func (s *Server) targetMember(ctx context.Context, friendGroupID, peerID string) (rpcapi.FriendGroupMemberObject, error) {
	member, err := s.groupMember(ctx, friendGroupID, peerID)
	if errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendGroupMemberObject{}, ErrFriendGroupMemberNotFound
	}
	return member, err
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

	data, err := socialutil.ReadInviteToken(ctx, store, socialutil.GroupInviteTokensRoot, inviteToken)
	if errors.Is(err, kv.ErrNotFound) {
		return inviteTokenRecord{}, ErrInviteTokenUnavailable
	}
	if err != nil {
		return inviteTokenRecord{}, err
	}
	var record inviteTokenRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return inviteTokenRecord{}, err
	}
	if !record.ExpiresAt.After(s.now()) {
		return inviteTokenRecord{}, ErrInviteTokenUnavailable
	}
	return record, nil
}

func (s *Server) listAllMembers(ctx context.Context, friendGroupID string) ([]friendGroupMemberRecord, error) {
	store, err := s.membersStore()
	if err != nil {
		return nil, err
	}
	peers, err := store.ListMembers(ctx, memberCollectionKey(friendGroupID))
	if err != nil {
		return nil, err
	}
	slices.Sort(peers)
	out := make([]friendGroupMemberRecord, 0, len(peers))
	for _, peer := range peers {
		item, err := socialutil.ReadJSONValue[friendGroupMemberRecord](ctx, store, socialutil.GroupMemberKey(friendGroupID, peer))
		if errors.Is(err, kv.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := item.validate(); err != nil {
			return nil, err
		}
		if item.FriendGroupID != friendGroupID || item.PeerPublicKey != peer {
			return nil, errors.New("social: group member index identity mismatch")
		}
		out = append(out, item)
	}
	return out, nil
}

func memberCollectionKey(group string) kv.Key {
	return kv.Key{"member-collections", socialutil.EscapeStoreSegment(group)}
}
func belongCollectionKey(peer string) kv.Key {
	return kv.Key{"group-collections", socialutil.EscapeStoreSegment(peer)}
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

func retiredGroupNameKey(owner, name string) kv.Key {
	return kv.Key{"retired-group-names", socialutil.EscapeStoreSegment(owner), socialutil.EscapeStoreSegment(name)}
}

// commitRetirementReceipt publishes the name index with the receipt. An older
// worker cannot replace the index of a later deletion that reused the name.
func (s *Server) commitRetirementReceipt(ctx context.Context, store kv.Store, receipt retirementReceipt, data []byte) error {
	nameKey := retiredGroupNameKey(receipt.Owner, receipt.Name)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		current, err := store.Get(ctx, nameKey)
		if err != nil && !errors.Is(err, kv.ErrNotFound) {
			return err
		}
		entries := []kv.Entry{{Key: groupRetirementReceiptKey(receipt.FriendGroupID), Value: data}}
		publish := true
		if err == nil {
			previous, err := s.readRetirementReceipt(ctx, string(current))
			if err != nil {
				return err
			}
			if previous.Owner != receipt.Owner || previous.Name != receipt.Name {
				return errors.New("social: retired Friend Group name index identity mismatch")
			}
			publish = previous.DeletedAt.Before(receipt.DeletedAt) || (previous.DeletedAt.Equal(receipt.DeletedAt) && previous.FriendGroupID <= receipt.FriendGroupID)
		}
		if publish {
			entries = append(entries, kv.Entry{Key: nameKey, Value: []byte(receipt.FriendGroupID)})
		}
		committed, err := store.ApplyMutation(ctx, kv.Mutation{
			Conditions: []kv.Condition{{Key: nameKey, Expected: current}},
			Entries:    entries, DeleteKeys: []kv.Key{groupRetirementIntentKey(receipt.FriendGroupID)},
			RemoveMembers: (socialutil.RecoveryIndex{Root: retirementIntentsRoot}).Remove(receipt.FriendGroupID),
		})
		if err != nil {
			return err
		}
		if committed {
			return nil
		}
	}
}

func belongPageKey(peer string) kv.Key {
	return kv.Key{"group-pages", socialutil.EscapeStoreSegment(strings.TrimSpace(peer))}
}
