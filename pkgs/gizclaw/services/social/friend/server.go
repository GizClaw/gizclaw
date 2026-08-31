package friend

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
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/internal/keyedlock"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type WorkspaceService interface {
	CreateSystemWorkspace(context.Context, adminhttp.WorkspaceUpsert) (apitypes.Workspace, bool, error)
	DeleteSystemWorkspace(context.Context, string) (apitypes.Workspace, error)
	RetireSystemWorkspaceByID(context.Context, string, apitypes.ChatRoomMode, string) (apitypes.Workspace, error)
}

type ProfileService interface {
	GetSelfInfo(context.Context, giznet.PublicKey) (apitypes.DeviceInfo, error)
}

type AssignmentService interface {
	Lookup(context.Context, giznet.PublicKey) (apitypes.PeerAssignment, error)
}

var ErrCrossServerFriendCreation = errors.New("cross-server friend creation is not supported")

type Server struct {
	InviteTokens           kv.Store
	Friends                kv.Store
	Workspaces             WorkspaceService
	Profiles               ProfileService
	RuntimeProfileForOwner func(context.Context, string) (apitypes.RuntimeProfile, error)
	NotifyPeer             func(context.Context, string, *eventpb.PeerEvent)
	PeerAvailability       func(context.Context, string) error
	PeerAssignments        AssignmentService
	ServerPublicKey        giznet.PublicKey

	Now   func() time.Time
	NewID func() string
}

type PeerRetirementFriend struct {
	RelationID    string `json:"relation_id"`
	FirstPeer     string `json:"first_peer"`
	SecondPeer    string `json:"second_peer"`
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
}

func (s *Server) SnapshotPeerFriends(ctx context.Context, owner string) ([]PeerRetirementFriend, error) {
	store, err := s.friendsStore()
	if err != nil {
		return nil, err
	}
	if err := socialutil.RequireOwner(owner); err != nil {
		return nil, err
	}
	if owner != strings.TrimSpace(owner) {
		return nil, errors.New("social: Peer public key must be canonical")
	}
	unlock, err := s.lockPeers(ctx, owner)
	if err != nil {
		return nil, err
	}
	defer unlock()
	var out []PeerRetirementFriend
	for entry, err := range store.List(ctx, socialutil.OwnerPrefix(socialutil.FriendsRoot, owner)) {
		if err != nil {
			return nil, err
		}
		var record friendRecord
		if err := json.Unmarshal(entry.Value, &record); err != nil {
			return nil, err
		}
		if err := record.validate(); err != nil {
			return nil, err
		}
		other := record.PeerPublicKey
		first, second := owner, other
		if first > second {
			first, second = second, first
		}
		if record.RelationID != socialutil.RelationID(first, second) {
			return nil, errors.New("social: Friend relation identity is inconsistent")
		}
		binding, err := readWorkspaceBinding(ctx, store, record.RelationID)
		if err != nil {
			return nil, err
		}
		if binding.WorkspaceName != record.WorkspaceName {
			return nil, errors.New("social: Friend Workspace binding is inconsistent")
		}
		out = append(out, PeerRetirementFriend{
			RelationID: record.RelationID, FirstPeer: first, SecondPeer: second,
			WorkspaceID: binding.WorkspaceID, WorkspaceName: binding.WorkspaceName,
		})
	}
	return out, nil
}

func (s *Server) RetirePeerFriend(ctx context.Context, owner string, snapshot PeerRetirementFriend) error {
	if owner == "" || owner != strings.TrimSpace(owner) {
		return errors.New("social: retiring Peer public key must be canonical")
	}
	if owner != snapshot.FirstPeer && owner != snapshot.SecondPeer {
		return errors.New("social: retiring Peer does not belong to Friend snapshot")
	}
	if snapshot.RelationID != socialutil.RelationID(snapshot.FirstPeer, snapshot.SecondPeer) ||
		snapshot.WorkspaceID == "" || snapshot.WorkspaceName == "" {
		return errors.New("social: invalid Friend Peer retirement snapshot")
	}
	store, err := s.friendsStore()
	if err != nil {
		return err
	}
	other := snapshot.FirstPeer
	if owner == other {
		other = snapshot.SecondPeer
	}
	active, activeErr := s.getFriendRelationByID(ctx, owner, snapshot.RelationID)
	if activeErr == nil {
		if socialutil.StringValue(active.PeerPublicKey) != other || socialutil.StringValue(active.WorkspaceName) != snapshot.WorkspaceName {
			return errors.New("social: Friend no longer matches Peer retirement snapshot")
		}
		binding, err := readWorkspaceBinding(ctx, store, snapshot.RelationID)
		if err != nil {
			return err
		}
		if binding.WorkspaceID != snapshot.WorkspaceID || binding.WorkspaceName != snapshot.WorkspaceName {
			return errors.New("social: Friend Workspace no longer matches Peer retirement snapshot")
		}
	} else if !errors.Is(activeErr, kv.ErrNotFound) {
		return activeErr
	} else {
		receipt, err := readRetirementReceipt(ctx, store, snapshot.RelationID)
		if err != nil {
			return err
		}
		if receipt.FirstPeer != snapshot.FirstPeer || receipt.SecondPeer != snapshot.SecondPeer ||
			receipt.WorkspaceID != snapshot.WorkspaceID || receipt.WorkspaceName != snapshot.WorkspaceName {
			return errors.New("social: completed Friend retirement does not match snapshot")
		}
		return nil
	}
	_, err = s.deleteFriendByRelationID(context.WithValue(ctx, peerRetirementContextKey{}, true), owner, snapshot.RelationID)
	return err
}

var (
	relationMutationMu [64]sync.Mutex
	peerMutationGates  keyedlock.Locker[string]
)

type peerRetirementContextKey struct{}

func peerRetirementFromContext(ctx context.Context) bool {
	value, _ := ctx.Value(peerRetirementContextKey{}).(bool)
	return value
}

func (s *Server) GetFriendInfo(ctx context.Context, owner string, req rpcapi.FriendInfoGetRequest) (rpcapi.FriendInfoGetResponse, error) {
	relation, err := s.GetFriendRelation(ctx, owner, req.Name)
	if err != nil {
		return rpcapi.FriendInfoGetResponse{}, err
	}
	if s.Profiles == nil {
		return rpcapi.FriendInfoGetResponse{}, errors.New("social: profile service not configured")
	}
	id := strings.TrimSpace(socialutil.StringValue(relation.PeerPublicKey))
	var publicKey giznet.PublicKey
	if err := publicKey.UnmarshalText([]byte(id)); err != nil {
		return rpcapi.FriendInfoGetResponse{}, err
	}
	info, err := s.Profiles.GetSelfInfo(ctx, publicKey)
	if err != nil {
		return rpcapi.FriendInfoGetResponse{}, err
	}
	return rpcapi.FriendInfoGetResponse{Name: id, Value: rpcapi.FriendInfo{DisplayName: info.Name, Emoji: info.Emoji}}, nil
}

type inviteTokenRecord struct {
	PeerPublicKey string    `json:"peer_public_key"`
	InviteToken   string    `json:"invite_token"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

// friendRecord is the canonical persistence shape for one owner's view of a
// Friend relationship. Peer RPC objects are projections and are never stored.
type friendRecord struct {
	RelationID    string    `json:"relation_id"`
	PeerPublicKey string    `json:"peer_public_key"`
	WorkspaceName string    `json:"workspace_name"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (record friendRecord) validate() error {
	if record.RelationID == "" || record.RelationID != strings.TrimSpace(record.RelationID) ||
		record.PeerPublicKey == "" || record.PeerPublicKey != strings.TrimSpace(record.PeerPublicKey) ||
		record.WorkspaceName == "" || record.WorkspaceName != strings.TrimSpace(record.WorkspaceName) ||
		record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() {
		return errors.New("social: persisted Friend relationship is invalid")
	}
	return nil
}

func (record friendRecord) peerObject() rpcapi.FriendObject {
	name := record.PeerPublicKey
	peerPublicKey := record.PeerPublicKey
	workspaceName := record.WorkspaceName
	createdAt := record.CreatedAt
	updatedAt := record.UpdatedAt
	return rpcapi.FriendObject{
		Name:          name,
		PeerPublicKey: &peerPublicKey,
		WorkspaceName: &workspaceName,
		CreatedAt:     &createdAt,
		UpdatedAt:     &updatedAt,
	}
}

type retirementIntent struct {
	RelationID     string       `json:"relation_id"`
	FirstPeer      string       `json:"first_peer"`
	SecondPeer     string       `json:"second_peer"`
	WorkspaceID    string       `json:"workspace_id"`
	WorkspaceName  string       `json:"workspace_name"`
	Relationship   friendRecord `json:"relationship"`
	DeletedAt      time.Time    `json:"deleted_at"`
	CancelCreation bool         `json:"cancel_creation,omitempty"`
}

type creationIntent struct {
	RelationID     string    `json:"relation_id"`
	FirstPeer      string    `json:"first_peer"`
	SecondPeer     string    `json:"second_peer"`
	WorkspaceOwner string    `json:"workspace_owner"`
	IncarnationID  string    `json:"incarnation_id"`
	Workspace      string    `json:"workspace_name"`
	Workflow       string    `json:"workflow_name"`
	CreatedAt      time.Time `json:"created_at"`
}

// creationDecision is the immutable, per-incarnation winner between committing
// reciprocal Friend rows and cancelling a still-pending creation. It remains
// durable so a delayed creator cannot commit after cancellation has completed.
type creationDecision struct {
	RelationID    string `json:"relation_id"`
	IncarnationID string `json:"incarnation_id"`
	Workspace     string `json:"workspace_name"`
	State         string `json:"state"`
}

type retirementReceipt struct {
	RelationID     string    `json:"relation_id"`
	FirstPeer      string    `json:"first_peer"`
	SecondPeer     string    `json:"second_peer"`
	WorkspaceID    string    `json:"workspace_id"`
	WorkspaceName  string    `json:"workspace_name"`
	DeletedAt      time.Time `json:"deleted_at"`
	CancelCreation bool      `json:"cancel_creation,omitempty"`
}

type workspaceBinding struct {
	RelationID    string `json:"relation_id"`
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
}

var (
	creationIntentsRoot    = kv.Key{"friend-creation-intents"}
	creationDecisionsRoot  = kv.Key{"friend-creation-decisions"}
	retirementIntentsRoot  = kv.Key{"friend-retirement-intents"}
	retirementReceiptsRoot = kv.Key{"friend-retirement-receipts"}
	workspaceBindingsRoot  = kv.Key{"friend-workspace-bindings"}
)

const (
	creationDecisionCommitted = "committed"
	creationDecisionCancelled = "cancelled"
)

var errFriendCreationCancelled = errors.New("social: Friend creation was cancelled")

func (s *Server) GetFriendInviteToken(ctx context.Context, owner string, _ rpcapi.FriendInviteTokenGetRequest) (rpcapi.FriendInviteTokenGetResponse, error) {
	store, err := s.inviteTokensStore()
	if err != nil {
		return rpcapi.FriendInviteTokenGetResponse{}, err
	}
	record, ok, err := s.activeInviteToken(ctx, store, strings.TrimSpace(owner))
	if err != nil || !ok {
		return rpcapi.FriendInviteTokenGetResponse{}, err
	}
	return rpcapi.FriendInviteTokenGetResponse{InviteToken: &record.InviteToken, ExpiresAt: &record.ExpiresAt}, nil
}

func (s *Server) CreateFriendInviteToken(ctx context.Context, owner string, _ rpcapi.FriendInviteTokenCreateRequest) (rpcapi.FriendInviteTokenCreateResponse, error) {
	store, err := s.inviteTokensStore()
	if err != nil {
		return rpcapi.FriendInviteTokenCreateResponse{}, err
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return rpcapi.FriendInviteTokenCreateResponse{}, errors.New("social: peer public key is required")
	}
	if record, ok, err := s.activeInviteToken(ctx, store, owner); err != nil {
		return rpcapi.FriendInviteTokenCreateResponse{}, err
	} else if ok {
		return rpcapi.FriendInviteTokenCreateResponse{InviteToken: record.InviteToken, ExpiresAt: record.ExpiresAt}, nil
	}
	now := s.now()
	record := inviteTokenRecord{
		PeerPublicKey: owner,
		InviteToken:   s.newID(),
		CreatedAt:     now,
		ExpiresAt:     now.Add(s.inviteTokenTTL()),
	}
	if strings.TrimSpace(record.InviteToken) == "" {
		return rpcapi.FriendInviteTokenCreateResponse{}, errors.New("social: invite token is empty")
	}
	if err := socialutil.WriteJSON(ctx, store, socialutil.FriendInviteTokenKey(owner), record); err != nil {
		return rpcapi.FriendInviteTokenCreateResponse{}, err
	}
	return rpcapi.FriendInviteTokenCreateResponse{InviteToken: record.InviteToken, ExpiresAt: record.ExpiresAt}, nil
}

func (s *Server) ClearFriendInviteToken(ctx context.Context, owner string, _ rpcapi.FriendInviteTokenClearRequest) (rpcapi.FriendInviteTokenClearResponse, error) {
	store, err := s.inviteTokensStore()
	if err != nil {
		return rpcapi.FriendInviteTokenClearResponse{}, err
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return rpcapi.FriendInviteTokenClearResponse{}, errors.New("social: peer public key is required")
	}
	if err := store.Delete(ctx, socialutil.FriendInviteTokenKey(owner)); err != nil && !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendInviteTokenClearResponse{}, err
	}
	return rpcapi.FriendInviteTokenClearResponse{}, nil
}

func (s *Server) AddFriend(ctx context.Context, owner string, req rpcapi.FriendAddRequest) (rpcapi.FriendAddResponse, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return rpcapi.FriendAddResponse{}, errors.New("social: peer public key is required")
	}
	record, err := s.findInviteToken(ctx, req.InviteToken)
	if err != nil {
		return rpcapi.FriendAddResponse{}, err
	}
	to := record.PeerPublicKey
	if owner == to {
		return rpcapi.FriendAddResponse{}, ErrInviteTokenSelfOwned
	}
	if err := s.requireLocalPeers(ctx, owner, to); err != nil {
		return rpcapi.FriendAddResponse{}, err
	}
	relationID := socialutil.RelationID(owner, to)
	unlock, err := s.lockRelationMutation(ctx, relationID, owner, to)
	if err != nil {
		return rpcapi.FriendAddResponse{}, err
	}
	defer unlock()
	return s.createFriend(ctx, owner, to, to)
}

func (s *Server) AdminCreateFriend(ctx context.Context, owner string, peerPublicKey string) (rpcapi.FriendObject, error) {
	owner = strings.TrimSpace(owner)
	peerPublicKey = strings.TrimSpace(peerPublicKey)
	if owner == "" || peerPublicKey == "" {
		return rpcapi.FriendObject{}, errors.New("social: friend peers are required")
	}
	if owner == peerPublicKey {
		return rpcapi.FriendObject{}, errors.New("social: cannot friend self")
	}
	if err := s.requireLocalPeers(ctx, owner, peerPublicKey); err != nil {
		return rpcapi.FriendObject{}, err
	}
	relationID := socialutil.RelationID(owner, peerPublicKey)
	unlock, err := s.lockRelationMutation(ctx, relationID, owner, peerPublicKey)
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	defer unlock()
	return s.createFriend(ctx, owner, peerPublicKey, owner)
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
			return ErrCrossServerFriendCreation
		}
	}
	return nil
}

func (s *Server) lockRelation(relationID string) func() {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(relationID))
	mu := &relationMutationMu[hash.Sum32()%uint32(len(relationMutationMu))]
	mu.Lock()
	return mu.Unlock
}

func (s *Server) lockRelationMutation(ctx context.Context, relationID string, peers ...string) (func(), error) {
	releasePeers, err := s.lockPeers(ctx, peers...)
	if err != nil {
		return nil, err
	}
	releaseRelation := s.lockRelation(relationID)
	return func() {
		releaseRelation()
		releasePeers()
	}, nil
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

func (s *Server) AdminListFriends(ctx context.Context, cursor *string, limit *int) (adminhttp.AdminFriendListResponse, error) {
	store, err := s.friendsStore()
	if err != nil {
		return adminhttp.AdminFriendListResponse{}, err
	}
	_, pageLimit := socialutil.NormalizeListParams("", socialutil.IntValue(limit))
	entries, err := kv.ListAfter(ctx, store, socialutil.FriendsRoot, adminFriendCursorAfter(socialutil.StringValue(cursor)), pageLimit+1)
	if err != nil {
		return adminhttp.AdminFriendListResponse{}, err
	}
	hasNext := len(entries) > pageLimit
	if hasNext {
		entries = entries[:pageLimit]
	}
	items := make([]adminhttp.AdminFriendObject, 0, len(entries))
	for _, entry := range entries {
		owner, ok := adminFriendOwner(entry.Key)
		if !ok {
			continue
		}
		var record friendRecord
		if err := json.Unmarshal(entry.Value, &record); err != nil {
			return adminhttp.AdminFriendListResponse{}, err
		}
		if err := record.validate(); err != nil {
			return adminhttp.AdminFriendListResponse{}, err
		}
		projected, err := s.adminFriendObject(ctx, owner, record.peerObject())
		if err != nil {
			return adminhttp.AdminFriendListResponse{}, err
		}
		items = append(items, projected)
	}
	var next *string
	if hasNext && len(entries) > 0 {
		cursor := adminFriendCursor(entries[len(entries)-1].Key)
		if cursor != "" {
			next = &cursor
		}
	}
	return adminhttp.AdminFriendListResponse{Items: items, HasNext: hasNext, NextCursor: next}, nil
}

func (s *Server) AdminCreateFriendResource(ctx context.Context, id, owner, peerPublicKey string) (adminhttp.AdminFriendObject, error) {
	if err := customid.ValidateResourceID(id); err != nil {
		return adminhttp.AdminFriendObject{}, fmt.Errorf("social: friend %w", err)
	}
	owner = strings.TrimSpace(owner)
	peerPublicKey = strings.TrimSpace(peerPublicKey)
	if id != socialutil.RelationID(owner, peerPublicKey) {
		return adminhttp.AdminFriendObject{}, errors.New("social: friend id must match the deterministic relation id")
	}
	if owner == "" || peerPublicKey == "" || owner == peerPublicKey {
		return adminhttp.AdminFriendObject{}, errors.New("social: friend peers must be distinct and non-empty")
	}
	unlock, err := s.lockRelationMutation(ctx, id, owner, peerPublicKey)
	if err != nil {
		return adminhttp.AdminFriendObject{}, err
	}
	defer unlock()
	store, err := s.friendsStore()
	if err != nil {
		return adminhttp.AdminFriendObject{}, err
	}
	if _, active, err := readActiveRelationship(ctx, store, owner, peerPublicKey); err != nil {
		return adminhttp.AdminFriendObject{}, err
	} else if active {
		return adminhttp.AdminFriendObject{}, fmt.Errorf("%w: friend id %q", socialutil.ErrResourceAlreadyExists, id)
	}
	item, err := s.createFriend(ctx, owner, peerPublicKey, owner)
	if err != nil {
		return adminhttp.AdminFriendObject{}, err
	}
	return s.adminFriendObject(ctx, owner, item)
}

func (s *Server) AdminGetFriend(ctx context.Context, owner, id string) (adminhttp.AdminFriendObject, error) {
	if err := customid.ValidateResourceID(id); err != nil {
		return adminhttp.AdminFriendObject{}, fmt.Errorf("social: friend %w", err)
	}
	owner = strings.TrimSpace(owner)
	item, err := s.getFriendRelationByID(ctx, owner, id)
	if err != nil {
		return adminhttp.AdminFriendObject{}, err
	}
	return s.adminFriendObject(ctx, owner, item)
}

func (s *Server) AdminDeleteFriend(ctx context.Context, owner, id string) (adminhttp.AdminFriendObject, error) {
	if err := customid.ValidateResourceID(id); err != nil {
		return adminhttp.AdminFriendObject{}, fmt.Errorf("social: friend %w", err)
	}
	owner = strings.TrimSpace(owner)
	item, err := s.deleteFriendByRelationID(ctx, owner, id)
	if err != nil {
		return adminhttp.AdminFriendObject{}, err
	}
	return s.adminFriendObject(ctx, owner, item)
}

func (s *Server) ListFriends(ctx context.Context, owner string, req rpcapi.FriendListRequest) (rpcapi.FriendListResponse, error) {
	store, err := s.friendsStore()
	if err != nil {
		return rpcapi.FriendListResponse{}, err
	}
	entries, err := socialutil.ListPage(ctx, store, socialutil.OwnerPrefix(socialutil.FriendsRoot, owner), socialutil.StringValue(req.Cursor), socialutil.IntValue(req.Limit))
	if err != nil {
		return rpcapi.FriendListResponse{}, err
	}
	items := make([]rpcapi.FriendObject, 0, len(entries.Items))
	for _, entry := range entries.Items {
		var record friendRecord
		if err := json.Unmarshal(entry.Value, &record); err != nil {
			return rpcapi.FriendListResponse{}, err
		}
		if err := record.validate(); err != nil {
			return rpcapi.FriendListResponse{}, err
		}
		items = append(items, record.peerObject())
	}
	return rpcapi.FriendListResponse{Items: items, HasNext: entries.HasNext, NextCursor: entries.NextCursor}, nil
}

// WorkspaceRecipientsByID returns the peers whose reciprocal relationship
// binds the canonical Direct Chatroom Workspace.
func (s *Server) WorkspaceRecipientsByID(ctx context.Context, workspaceID string) ([]string, error) {
	store, err := s.friendsStore()
	if err != nil {
		return nil, err
	}
	if err := customid.ValidateResourceID(workspaceID); err != nil {
		return nil, fmt.Errorf("social: invalid workspace id: %w", err)
	}
	seen := make(map[string]struct{})
	for entry, err := range store.List(ctx, socialutil.FriendsRoot) {
		if err != nil {
			return nil, err
		}
		var record friendRecord
		if err := json.Unmarshal(entry.Value, &record); err != nil {
			return nil, err
		}
		if err := record.validate(); err != nil {
			return nil, err
		}
		if len(entry.Key) < 3 {
			continue
		}
		owner := socialutil.UnescapeStoreSegment(entry.Key[1])
		binding, err := readWorkspaceBinding(ctx, store, record.RelationID)
		if errors.Is(err, kv.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if binding.WorkspaceID != workspaceID {
			continue
		}
		seen[owner] = struct{}{}
	}
	recipients := make([]string, 0, len(seen))
	for publicKey := range seen {
		recipients = append(recipients, publicKey)
	}
	return recipients, nil
}

func (s *Server) DeleteFriend(ctx context.Context, owner string, req rpcapi.FriendDeleteRequest) (rpcapi.FriendObject, error) {
	owner = strings.TrimSpace(owner)
	if req.Name == "" || req.Name != strings.TrimSpace(req.Name) {
		return rpcapi.FriendObject{}, errors.New("social: Friend name is required without surrounding whitespace")
	}
	return s.deleteFriendByRelationID(ctx, owner, socialutil.RelationID(owner, req.Name))
}

func (s *Server) deleteFriendByRelationID(ctx context.Context, owner, relationID string) (rpcapi.FriendObject, error) {
	if s == nil || s.Workspaces == nil {
		return rpcapi.FriendObject{}, errors.New("social: Workspace retirement service not configured")
	}
	store, err := s.friendsStore()
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	peers := []string{owner}
	if current, readErr := s.getFriendRelationByID(ctx, owner, relationID); readErr == nil {
		peers = append(peers, socialutil.StringValue(current.PeerPublicKey))
	} else if !errors.Is(readErr, kv.ErrNotFound) {
		return rpcapi.FriendObject{}, readErr
	} else if creation, creationErr := readCreationIntent(ctx, store, relationID); creationErr == nil {
		peers = append(peers, creation.FirstPeer, creation.SecondPeer)
	} else if !errors.Is(creationErr, kv.ErrNotFound) {
		return rpcapi.FriendObject{}, creationErr
	} else if retirement, retirementErr := readRetirementIntent(ctx, store, relationID); retirementErr == nil {
		peers = append(peers, retirement.FirstPeer, retirement.SecondPeer)
	} else if !errors.Is(retirementErr, kv.ErrNotFound) {
		return rpcapi.FriendObject{}, retirementErr
	}
	unlock, err := s.lockRelationMutation(ctx, relationID, peers...)
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	defer unlock()
	item, err := s.getFriendRelationByID(ctx, owner, relationID)
	if err != nil {
		if !errors.Is(err, kv.ErrNotFound) {
			return rpcapi.FriendObject{}, err
		}
		creation, creationErr := readCreationIntent(ctx, store, relationID)
		if creationErr == nil {
			return s.cancelFriendCreation(ctx, store, owner, relationID, creation)
		}
		if !errors.Is(creationErr, kv.ErrNotFound) {
			return rpcapi.FriendObject{}, creationErr
		}
		intent, intentErr := readRetirementIntent(ctx, store, relationID)
		if intentErr != nil {
			if errors.Is(intentErr, kv.ErrNotFound) {
				return s.completedFriendDeletion(ctx, owner, relationID)
			}
			return rpcapi.FriendObject{}, intentErr
		}
		intent, err = validateRetirementIntent(intent, relationID)
		if err != nil {
			return rpcapi.FriendObject{}, err
		}
		if owner != intent.FirstPeer && owner != intent.SecondPeer {
			return rpcapi.FriendObject{}, kv.ErrNotFound
		}
		if err := s.completeFriendRetirement(ctx, store, intent); err != nil {
			return rpcapi.FriendObject{}, err
		}
		return friendObjectForRetirement(owner, intent), nil
	}
	if !peerRetirementFromContext(ctx) && s.PeerAvailability != nil {
		if err := s.PeerAvailability(ctx, owner); err != nil {
			return rpcapi.FriendObject{}, err
		}
		if err := s.PeerAvailability(ctx, socialutil.StringValue(item.PeerPublicKey)); err != nil {
			return rpcapi.FriendObject{}, err
		}
	}
	return s.retireActiveFriend(ctx, store, owner, relationID, item)
}

func (s *Server) retireActiveFriend(
	ctx context.Context,
	store kv.Store,
	owner string,
	relationID string,
	item rpcapi.FriendObject,
) (rpcapi.FriendObject, error) {
	other := strings.TrimSpace(socialutil.StringValue(item.PeerPublicKey))
	workspaceName := strings.TrimSpace(socialutil.StringValue(item.WorkspaceName))
	if workspaceName == "" {
		return rpcapi.FriendObject{}, errors.New("social: active Friend relationship has no Workspace name")
	}
	binding, err := readWorkspaceBinding(ctx, store, relationID)
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	if binding.WorkspaceName != workspaceName {
		return rpcapi.FriendObject{}, errors.New("social: Friend Workspace binding is inconsistent")
	}
	first, second := owner, other
	if first > second {
		first, second = second, first
	}
	intent := retirementIntent{
		RelationID:    relationID,
		FirstPeer:     first,
		SecondPeer:    second,
		WorkspaceID:   binding.WorkspaceID,
		WorkspaceName: workspaceName,
		Relationship: friendRecord{
			RelationID:    relationID,
			PeerPublicKey: other,
			WorkspaceName: workspaceName,
			CreatedAt:     socialutil.TimeValue(item.CreatedAt),
			UpdatedAt:     socialutil.TimeValue(item.UpdatedAt),
		},
		DeletedAt: s.now(),
	}
	data, err := json.Marshal(intent)
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	if err := store.BatchMutate(
		ctx,
		[]kv.Entry{{Key: retirementIntentKey(relationID), Value: data}},
		[]kv.Key{
			socialutil.FriendKey(owner, relationID),
			socialutil.FriendKey(other, relationID),
			workspaceBindingKey(relationID),
		},
	); err != nil {
		return rpcapi.FriendObject{}, err
	}
	if err := s.completeFriendRetirement(ctx, store, intent); err != nil {
		return rpcapi.FriendObject{}, err
	}
	return item, nil
}

func (s *Server) cancelFriendCreation(
	ctx context.Context,
	store kv.Store,
	owner string,
	relationID string,
	creation creationIntent,
) (rpcapi.FriendObject, error) {
	creation, err := validateCreationIntent(
		creation,
		relationID,
		creation.WorkspaceOwner,
	)
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	if owner != creation.FirstPeer && owner != creation.SecondPeer {
		return rpcapi.FriendObject{}, kv.ErrNotFound
	}
	other := creation.FirstPeer
	if owner == creation.FirstPeer {
		other = creation.SecondPeer
	}
	if item, active, err := readActiveRelationship(
		ctx,
		store,
		owner,
		other,
	); err != nil {
		return rpcapi.FriendObject{}, err
	} else if active {
		return s.retireActiveFriend(ctx, store, owner, relationID, item)
	}
	intent := retirementIntent{
		RelationID:     creation.RelationID,
		FirstPeer:      creation.FirstPeer,
		SecondPeer:     creation.SecondPeer,
		WorkspaceName:  creation.Workspace,
		DeletedAt:      s.now(),
		CancelCreation: true,
	}
	data, err := json.Marshal(intent)
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	decision := creationDecision{
		RelationID:    creation.RelationID,
		IncarnationID: creation.IncarnationID,
		Workspace:     creation.Workspace,
		State:         creationDecisionCancelled,
	}
	decisionData, err := json.Marshal(decision)
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	existing, created, err := kv.CreateIfAbsent(
		ctx,
		store,
		kv.Entry{
			Key:   creationDecisionKey(creation.RelationID, creation.IncarnationID),
			Value: decisionData,
		},
		[]kv.Entry{{Key: retirementIntentKey(intent.RelationID), Value: data}},
	)
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	if !created {
		if err := json.Unmarshal(existing, &decision); err != nil {
			return rpcapi.FriendObject{}, err
		}
		if err := validateCreationDecision(decision, creation); err != nil {
			return rpcapi.FriendObject{}, err
		}
		if decision.State == creationDecisionCommitted {
			item, active, err := readActiveRelationship(ctx, store, owner, other)
			if err != nil {
				return rpcapi.FriendObject{}, err
			}
			if !active {
				return rpcapi.FriendObject{}, errors.New(
					"social: committed Friend creation is missing reciprocal rows",
				)
			}
			return s.retireActiveFriend(ctx, store, owner, relationID, item)
		}
		persisted, err := readRetirementIntent(ctx, store, relationID)
		if err == nil {
			persisted, err = validateRetirementIntent(persisted, relationID)
			if err != nil {
				return rpcapi.FriendObject{}, err
			}
			if persisted.CancelCreation &&
				persisted.WorkspaceName == creation.Workspace {
				intent = persisted
			} else {
				if err := deleteCreationIntent(ctx, store, creation); err != nil {
					return rpcapi.FriendObject{}, err
				}
				return friendObjectForRetirement(owner, intent), nil
			}
		} else if errors.Is(err, kv.ErrNotFound) {
			if err := deleteCreationIntent(ctx, store, creation); err != nil {
				return rpcapi.FriendObject{}, err
			}
			return friendObjectForRetirement(owner, intent), nil
		} else {
			return rpcapi.FriendObject{}, err
		}
	}
	if err := deleteCreationIntent(ctx, store, creation); err != nil {
		return rpcapi.FriendObject{}, err
	}
	if err := s.completeFriendRetirement(ctx, store, intent); err != nil {
		return rpcapi.FriendObject{}, err
	}
	return friendObjectForRetirement(owner, intent), nil
}

func (s *Server) completedFriendDeletion(
	ctx context.Context,
	owner string,
	relationID string,
) (rpcapi.FriendObject, error) {
	peer := relationPeer(owner, relationID)
	if peer == "" {
		return rpcapi.FriendObject{}, kv.ErrNotFound
	}
	store, err := s.friendsStore()
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	receipt, err := readRetirementReceipt(ctx, store, relationID)
	if err == nil {
		if err := validateRetirementReceipt(receipt, relationID); err != nil {
			return rpcapi.FriendObject{}, err
		}
		if owner != receipt.FirstPeer && owner != receipt.SecondPeer {
			return rpcapi.FriendObject{}, kv.ErrNotFound
		}
		return friendObjectForReceipt(owner, receipt), nil
	}
	if !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendObject{}, err
	}
	return rpcapi.FriendObject{}, kv.ErrNotFound
}

func (s *Server) GetFriendRelation(ctx context.Context, owner, name string) (rpcapi.FriendObject, error) {
	owner = strings.TrimSpace(owner)
	if name == "" || name != strings.TrimSpace(name) {
		return rpcapi.FriendObject{}, errors.New("social: Friend name is required without surrounding whitespace")
	}
	return s.getFriendRelationByID(ctx, owner, socialutil.RelationID(owner, name))
}

func (s *Server) getFriendRelationByID(ctx context.Context, owner, relationID string) (rpcapi.FriendObject, error) {
	store, err := s.friendsStore()
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	record, err := socialutil.ReadJSONValue[friendRecord](ctx, store, socialutil.FriendKey(owner, relationID))
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	if err := record.validate(); err != nil {
		return rpcapi.FriendObject{}, err
	}
	if record.RelationID != relationID {
		return rpcapi.FriendObject{}, errors.New("social: persisted Friend relationship ID does not match its key")
	}
	return record.peerObject(), nil
}

func relationPeer(owner, relationID string) string {
	owner = strings.TrimSpace(owner)
	parts := strings.Split(strings.TrimSpace(relationID), ":")
	if len(parts) != 2 {
		return ""
	}
	switch {
	case parts[0] == owner:
		return parts[1]
	case parts[1] == owner:
		return parts[0]
	default:
		return ""
	}
}

func (s *Server) adminFriendObject(ctx context.Context, owner string, item rpcapi.FriendObject) (adminhttp.AdminFriendObject, error) {
	owner = strings.TrimSpace(owner)
	peerPublicKey := strings.TrimSpace(socialutil.StringValue(item.PeerPublicKey))
	id := socialutil.RelationID(owner, peerPublicKey)
	binding, err := s.workspaceBinding(ctx, id)
	if err != nil {
		return adminhttp.AdminFriendObject{}, err
	}
	return adminhttp.AdminFriendObject{
		OwnerPublicKey: owner,
		Id:             id,
		PeerPublicKey:  peerPublicKey,
		WorkspaceId:    binding.WorkspaceID,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}, nil
}

func (s *Server) workspaceBinding(ctx context.Context, relationID string) (workspaceBinding, error) {
	store, err := s.friendsStore()
	if err != nil {
		return workspaceBinding{}, err
	}
	binding, err := readWorkspaceBinding(ctx, store, relationID)
	if err == nil {
		return binding, nil
	}
	if !errors.Is(err, kv.ErrNotFound) {
		return workspaceBinding{}, err
	}
	if intent, intentErr := readRetirementIntent(ctx, store, relationID); intentErr == nil {
		return workspaceBinding{RelationID: relationID, WorkspaceID: intent.WorkspaceID, WorkspaceName: intent.WorkspaceName}, nil
	} else if !errors.Is(intentErr, kv.ErrNotFound) {
		return workspaceBinding{}, intentErr
	}
	receipt, receiptErr := readRetirementReceipt(ctx, store, relationID)
	if receiptErr != nil {
		return workspaceBinding{}, receiptErr
	}
	return workspaceBinding{RelationID: relationID, WorkspaceID: receipt.WorkspaceID, WorkspaceName: receipt.WorkspaceName}, nil
}

func adminFriendOwner(key kv.Key) (string, bool) {
	if len(key) < 3 {
		return "", false
	}
	return socialutil.UnescapeStoreSegment(key[1]), true
}

func adminFriendCursor(key kv.Key) string {
	if len(key) < 3 {
		return ""
	}
	return key[1] + "/" + key[2]
}

func adminFriendCursorAfter(cursor string) kv.Key {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return nil
	}
	parts := strings.Split(cursor, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil
	}
	return append(append(kv.Key{}, socialutil.FriendsRoot...), parts[0], parts[1])
}

func (s *Server) createFriend(
	ctx context.Context,
	from string,
	to string,
	workspaceOwner string,
) (rpcapi.FriendObject, error) {
	store, err := s.friendsStore()
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	for {
		existing, active, err := readActiveRelationship(ctx, store, from, to)
		if err != nil {
			return rpcapi.FriendObject{}, err
		}
		if active {
			return existing, nil
		}
		if s.PeerAvailability != nil {
			if err := s.PeerAvailability(ctx, from); err != nil {
				return rpcapi.FriendObject{}, err
			}
			if err := s.PeerAvailability(ctx, to); err != nil {
				return rpcapi.FriendObject{}, err
			}
		}
		intent, err := s.getOrCreateCreationIntent(
			ctx,
			store,
			from,
			to,
			workspaceOwner,
		)
		if err != nil {
			return rpcapi.FriendObject{}, err
		}
		decisionState, err := s.reconcileCreationDecision(ctx, store, intent)
		if err != nil {
			return rpcapi.FriendObject{}, err
		}
		if decisionState == creationDecisionCancelled {
			return rpcapi.FriendObject{}, errFriendCreationCancelled
		}
		if decisionState != "" {
			continue
		}
		workspace, err := s.ensureCreationWorkspace(ctx, intent)
		if err != nil {
			return rpcapi.FriendObject{}, err
		}
		return s.commitFriendCreation(ctx, store, from, to, intent, workspace)
	}
}

func readActiveRelationship(
	ctx context.Context,
	store kv.Store,
	from string,
	to string,
) (rpcapi.FriendObject, bool, error) {
	relationID := socialutil.RelationID(from, to)
	fromItem, fromErr := socialutil.ReadJSONValue[friendRecord](
		ctx,
		store,
		socialutil.FriendKey(from, relationID),
	)
	toItem, toErr := socialutil.ReadJSONValue[friendRecord](
		ctx,
		store,
		socialutil.FriendKey(to, relationID),
	)
	fromMissing := errors.Is(fromErr, kv.ErrNotFound)
	toMissing := errors.Is(toErr, kv.ErrNotFound)
	if fromMissing && toMissing {
		return rpcapi.FriendObject{}, false, nil
	}
	if fromErr != nil && !fromMissing {
		return rpcapi.FriendObject{}, false, fromErr
	}
	if toErr != nil && !toMissing {
		return rpcapi.FriendObject{}, false, toErr
	}
	if fromMissing != toMissing {
		return rpcapi.FriendObject{}, false, errors.New(
			"social: reciprocal Friend relationship is incomplete",
		)
	}
	if err := fromItem.validate(); err != nil {
		return rpcapi.FriendObject{}, false, err
	}
	if err := toItem.validate(); err != nil {
		return rpcapi.FriendObject{}, false, err
	}
	fromWorkspace := strings.TrimSpace(fromItem.WorkspaceName)
	toWorkspace := strings.TrimSpace(toItem.WorkspaceName)
	if fromItem.RelationID != relationID || toItem.RelationID != relationID ||
		strings.TrimSpace(fromItem.PeerPublicKey) != to ||
		strings.TrimSpace(toItem.PeerPublicKey) != from ||
		fromWorkspace != toWorkspace {
		return rpcapi.FriendObject{}, false, errors.New(
			"social: reciprocal Friend relationship is inconsistent",
		)
	}
	if fromWorkspace == "" {
		return rpcapi.FriendObject{}, false, errors.New(
			"social: reciprocal Friend relationship has no Workspace name",
		)
	}
	return fromItem.peerObject(), true, nil
}

func (s *Server) getOrCreateCreationIntent(
	ctx context.Context,
	store kv.Store,
	from string,
	to string,
	workspaceOwner string,
) (creationIntent, error) {
	relationID := socialutil.RelationID(from, to)
	if intent, err := readCreationIntent(ctx, store, relationID); err == nil {
		return validateCreationIntent(intent, relationID, workspaceOwner)
	} else if !errors.Is(err, kv.ErrNotFound) {
		return creationIntent{}, err
	}
	if s == nil || s.Workspaces == nil {
		return creationIntent{}, errors.New("social: Workspace creation service not configured")
	}
	if s.RuntimeProfileForOwner == nil {
		return creationIntent{}, errors.New("social: runtime profile resolver is not configured")
	}
	profile, err := s.RuntimeProfileForOwner(ctx, workspaceOwner)
	if err != nil {
		return creationIntent{}, err
	}
	incarnationID := strings.TrimSpace(s.newID())
	if incarnationID == "" {
		return creationIntent{}, errors.New("social: friend Workspace incarnation is empty")
	}
	first, second := from, to
	if first > second {
		first, second = second, first
	}
	intent := creationIntent{
		RelationID:     relationID,
		FirstPeer:      first,
		SecondPeer:     second,
		WorkspaceOwner: strings.TrimSpace(workspaceOwner),
		IncarnationID:  incarnationID,
		Workspace:      socialutil.DirectWorkspaceIncarnationName(relationID, incarnationID),
		Workflow:       strings.TrimSpace(profile.Spec.Workflows.System.FriendChatroom),
		CreatedAt:      s.now(),
	}
	if _, err := validateCreationIntent(intent, relationID, workspaceOwner); err != nil {
		return creationIntent{}, err
	}
	data, err := json.Marshal(intent)
	if err != nil {
		return creationIntent{}, err
	}
	existing, created, err := kv.CreateIfAbsent(
		ctx,
		store,
		kv.Entry{Key: creationIntentKey(relationID), Value: data},
		nil,
	)
	if err != nil {
		return creationIntent{}, err
	}
	if created {
		return intent, nil
	}
	if err := json.Unmarshal(existing, &intent); err != nil {
		return creationIntent{}, err
	}
	return validateCreationIntent(intent, relationID, workspaceOwner)
}

func validateCreationIntent(
	intent creationIntent,
	relationID string,
	workspaceOwner string,
) (creationIntent, error) {
	first, second := intent.FirstPeer, intent.SecondPeer
	if first > second {
		return creationIntent{}, errors.New("social: Friend creation intent peers are not canonical")
	}
	if intent.RelationID != strings.TrimSpace(intent.RelationID) ||
		intent.RelationID != relationID ||
		first == "" ||
		second == "" ||
		first != strings.TrimSpace(first) ||
		second != strings.TrimSpace(second) ||
		socialutil.RelationID(first, second) != relationID ||
		intent.WorkspaceOwner != strings.TrimSpace(intent.WorkspaceOwner) ||
		intent.WorkspaceOwner != strings.TrimSpace(workspaceOwner) ||
		(intent.WorkspaceOwner != first && intent.WorkspaceOwner != second) ||
		intent.IncarnationID == "" ||
		intent.IncarnationID != strings.TrimSpace(intent.IncarnationID) ||
		intent.Workflow == "" ||
		intent.Workflow != strings.TrimSpace(intent.Workflow) ||
		intent.Workspace != strings.TrimSpace(intent.Workspace) ||
		intent.Workspace != socialutil.DirectWorkspaceIncarnationName(
			relationID,
			intent.IncarnationID,
		) ||
		intent.CreatedAt.IsZero() {
		return creationIntent{}, fmt.Errorf(
			"social: invalid Friend creation intent %q",
			relationID,
		)
	}
	return intent, nil
}

func (s *Server) ensureCreationWorkspace(ctx context.Context, intent creationIntent) (apitypes.Workspace, error) {
	if s == nil || s.Workspaces == nil {
		return apitypes.Workspace{}, errors.New("social: Workspace creation service not configured")
	}
	body := adminhttp.WorkspaceUpsert{
		Id:         intent.Workspace,
		Name:       intent.Workspace,
		WorkflowId: intent.Workflow,
		Parameters: socialutil.ChatRoomWorkspaceParameters(apitypes.ChatRoomModeDirect),
	}
	workspace, _, err := s.Workspaces.CreateSystemWorkspace(
		ownership.WithOwner(ctx, intent.WorkspaceOwner),
		body,
	)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	if strings.TrimSpace(workspace.Id) == "" || workspace.Name != intent.Workspace {
		return apitypes.Workspace{}, errors.New("social: created Friend Workspace has invalid identity")
	}
	return workspace, nil
}

func (s *Server) commitFriendCreation(
	ctx context.Context,
	store kv.Store,
	from string,
	to string,
	intent creationIntent,
	workspace apitypes.Workspace,
) (rpcapi.FriendObject, error) {
	relationID := socialutil.RelationID(from, to)
	entries := make([]kv.Entry, 0, 2)
	var ownerRow friendRecord
	now := s.now()
	for _, row := range []struct{ owner, peer string }{{from, to}, {to, from}} {
		item := friendRecord{
			RelationID:    relationID,
			PeerPublicKey: row.peer,
			WorkspaceName: intent.Workspace,
			CreatedAt:     intent.CreatedAt,
			UpdatedAt:     now,
		}
		if row.owner == from {
			ownerRow = item
		}
		data, err := json.Marshal(item)
		if err != nil {
			return rpcapi.FriendObject{}, err
		}
		entries = append(entries, kv.Entry{
			Key:   socialutil.FriendKey(row.owner, relationID),
			Value: data,
		})
	}
	decision := creationDecision{
		RelationID:    intent.RelationID,
		IncarnationID: intent.IncarnationID,
		Workspace:     intent.Workspace,
		State:         creationDecisionCommitted,
	}
	decisionData, err := json.Marshal(decision)
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	binding := workspaceBinding{
		RelationID:    relationID,
		WorkspaceID:   workspace.Id,
		WorkspaceName: workspace.Name,
	}
	bindingData, err := json.Marshal(binding)
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	entries = append(entries, kv.Entry{Key: workspaceBindingKey(relationID), Value: bindingData})
	existingDecision, created, err := kv.CreateIfAbsent(
		ctx,
		store,
		kv.Entry{
			Key:   creationDecisionKey(relationID, intent.IncarnationID),
			Value: decisionData,
		},
		entries,
	)
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	if !created {
		if err := json.Unmarshal(existingDecision, &decision); err != nil {
			return rpcapi.FriendObject{}, err
		}
		if err := validateCreationDecision(decision, intent); err != nil {
			return rpcapi.FriendObject{}, err
		}
		switch decision.State {
		case creationDecisionCancelled:
			if _, err := s.Workspaces.DeleteSystemWorkspace(
				ctx,
				intent.Workspace,
			); err != nil && !errors.Is(err, kv.ErrNotFound) {
				return rpcapi.FriendObject{}, err
			}
			if err := deleteCreationIntent(ctx, store, intent); err != nil {
				return rpcapi.FriendObject{}, err
			}
			return rpcapi.FriendObject{}, errFriendCreationCancelled
		case creationDecisionCommitted:
			persistedBinding, bindingErr := readWorkspaceBinding(ctx, store, relationID)
			if bindingErr != nil {
				return rpcapi.FriendObject{}, bindingErr
			}
			if persistedBinding.WorkspaceID != workspace.Id || persistedBinding.WorkspaceName != intent.Workspace {
				return rpcapi.FriendObject{}, errors.New("social: committed Friend creation has inconsistent Workspace binding")
			}
			existing, active, err := readActiveRelationship(ctx, store, from, to)
			if err != nil {
				return rpcapi.FriendObject{}, err
			}
			if !active {
				if err := deleteCreationIntent(ctx, store, intent); err != nil {
					return rpcapi.FriendObject{}, err
				}
				return rpcapi.FriendObject{}, errFriendCreationCancelled
			}
			if socialutil.StringValue(existing.WorkspaceName) != intent.Workspace {
				return rpcapi.FriendObject{}, errors.New(
					"social: committed Friend creation is missing reciprocal rows",
				)
			}
			if err := deleteCreationIntent(ctx, store, intent); err != nil {
				return rpcapi.FriendObject{}, err
			}
			return existing, nil
		}
	}
	s.notifyRelationship(
		ctx,
		from,
		to,
		intent.Workspace,
		eventpb.FriendRelationshipChange_FRIEND_RELATIONSHIP_CHANGE_CREATED,
		now,
	)
	if err := deleteCreationIntent(ctx, store, intent); err != nil {
		return rpcapi.FriendObject{}, err
	}
	return ownerRow.peerObject(), nil
}

func (s *Server) reconcileCreationDecision(
	ctx context.Context,
	store kv.Store,
	intent creationIntent,
) (string, error) {
	decision, err := readCreationDecision(ctx, store, intent)
	if errors.Is(err, kv.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if err := validateCreationDecision(decision, intent); err != nil {
		return "", err
	}
	if decision.State == creationDecisionCancelled {
		if s == nil || s.Workspaces == nil {
			return "", errors.New(
				"social: Workspace creation service not configured",
			)
		}
		if _, err := s.Workspaces.DeleteSystemWorkspace(
			ctx,
			intent.Workspace,
		); err != nil && !errors.Is(err, kv.ErrNotFound) {
			return "", err
		}
		return decision.State, deleteCreationIntent(ctx, store, intent)
	}
	existing, active, err := readActiveRelationship(
		ctx,
		store,
		intent.FirstPeer,
		intent.SecondPeer,
	)
	if err != nil {
		return "", err
	}
	if active &&
		socialutil.StringValue(existing.WorkspaceName) != intent.Workspace {
		return "", errors.New(
			"social: active Friend relationship conflicts with creation decision",
		)
	}
	return decision.State, deleteCreationIntent(ctx, store, intent)
}

func (s *Server) completeFriendRetirement(ctx context.Context, store kv.Store, intent retirementIntent) error {
	if s == nil || s.Workspaces == nil {
		return errors.New("social: Workspace retirement service not configured")
	}
	if _, err := validateRetirementIntent(intent, intent.RelationID); err != nil {
		return err
	}
	if intent.CancelCreation {
		if _, err := s.Workspaces.DeleteSystemWorkspace(
			ctx,
			intent.WorkspaceName,
		); err != nil && !errors.Is(err, kv.ErrNotFound) {
			return err
		}
	} else {
		if _, err := s.Workspaces.RetireSystemWorkspaceByID(
			ctx,
			intent.WorkspaceID,
			apitypes.ChatRoomModeDirect,
			intent.RelationID,
		); err != nil {
			return err
		}
	}
	receipt := retirementReceipt{
		RelationID:     intent.RelationID,
		FirstPeer:      intent.FirstPeer,
		SecondPeer:     intent.SecondPeer,
		WorkspaceID:    intent.WorkspaceID,
		WorkspaceName:  intent.WorkspaceName,
		DeletedAt:      intent.DeletedAt,
		CancelCreation: intent.CancelCreation,
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	intentKey := retirementIntentKey(intent.RelationID)
	stored, err := store.Get(ctx, intentKey)
	if errors.Is(err, kv.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	var current retirementIntent
	if err := json.Unmarshal(stored, &current); err != nil {
		return err
	}
	current, err = validateRetirementIntent(current, intent.RelationID)
	if err != nil {
		return err
	}
	if current.WorkspaceID != intent.WorkspaceID ||
		current.WorkspaceName != intent.WorkspaceName ||
		!current.DeletedAt.Equal(intent.DeletedAt) ||
		current.CancelCreation != intent.CancelCreation {
		return nil
	}
	matched, err := kv.CompareAndMutate(
		ctx,
		store,
		intentKey,
		stored,
		[]kv.Entry{{Key: retirementReceiptKey(intent.RelationID), Value: data}},
		[]kv.Key{intentKey},
	)
	if err != nil {
		return err
	}
	if matched && !intent.CancelCreation {
		s.notifyFriendRetirement(ctx, intent)
	}
	return nil
}

// ReconcileCreationIntents completes Workspace-first relationship creations
// that stopped before the reciprocal Friend rows committed.
func (s *Server) ReconcileCreationIntents(ctx context.Context) error {
	store, err := s.friendsStore()
	if err != nil {
		return err
	}
	for entry, err := range store.List(ctx, creationIntentsRoot) {
		if err != nil {
			return err
		}
		if len(entry.Key) != len(creationIntentsRoot)+1 {
			continue
		}
		relationID := strings.TrimSpace(
			socialutil.UnescapeStoreSegment(entry.Key[len(creationIntentsRoot)]),
		)
		var listed creationIntent
		if err := json.Unmarshal(entry.Value, &listed); err != nil {
			return err
		}
		if relationID == "" || strings.TrimSpace(listed.RelationID) != relationID {
			return fmt.Errorf("social: invalid Friend creation intent %q", relationID)
		}
		unlock, lockErr := s.lockRelationMutation(ctx, relationID, listed.FirstPeer, listed.SecondPeer)
		if lockErr != nil {
			return lockErr
		}
		current, readErr := readCreationIntent(ctx, store, relationID)
		if errors.Is(readErr, kv.ErrNotFound) {
			unlock()
			continue
		}
		if readErr != nil {
			unlock()
			return readErr
		}
		current, err = validateCreationIntent(
			current,
			relationID,
			current.WorkspaceOwner,
		)
		if err == nil {
			var decisionState string
			decisionState, err = s.reconcileCreationDecision(ctx, store, current)
			if err == nil && decisionState == "" {
				var existing rpcapi.FriendObject
				var active bool
				existing, active, err = readActiveRelationship(
					ctx,
					store,
					current.FirstPeer,
					current.SecondPeer,
				)
				if err == nil && active {
					if socialutil.StringValue(existing.WorkspaceName) != current.Workspace {
						err = errors.New(
							"social: active Friend relationship conflicts with creation intent",
						)
					} else {
						err = deleteCreationIntent(ctx, store, current)
					}
				} else if err == nil {
					var workspace apitypes.Workspace
					workspace, err = s.ensureCreationWorkspace(ctx, current)
					if err == nil {
						_, err = s.commitFriendCreation(
							ctx,
							store,
							current.FirstPeer,
							current.SecondPeer,
							current,
							workspace,
						)
						if errors.Is(err, errFriendCreationCancelled) {
							err = nil
						}
					}
				}
			}
		}
		unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

// ReconcileRetirementIntents completes relationship-first deletions that
// committed before the process could persist their Workspace PendingDeletion.
func (s *Server) ReconcileRetirementIntents(ctx context.Context) error {
	store, err := s.friendsStore()
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
		relationID := strings.TrimSpace(
			socialutil.UnescapeStoreSegment(entry.Key[len(retirementIntentsRoot)]),
		)
		if relationID == "" || strings.TrimSpace(intent.RelationID) != relationID {
			return fmt.Errorf("social: invalid Friend retirement intent %q", relationID)
		}
		unlock, lockErr := s.lockRelationMutation(ctx, relationID, intent.FirstPeer, intent.SecondPeer)
		if lockErr != nil {
			return lockErr
		}
		current, readErr := readRetirementIntent(ctx, store, relationID)
		if errors.Is(readErr, kv.ErrNotFound) {
			unlock()
			continue
		}
		if readErr != nil {
			unlock()
			return readErr
		}
		current, err = validateRetirementIntent(current, relationID)
		if err != nil {
			unlock()
			return err
		}
		err = s.completeFriendRetirement(ctx, store, current)
		unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) notifyFriendRetirement(ctx context.Context, intent retirementIntent) {
	s.notifyRelationship(
		ctx,
		intent.FirstPeer,
		intent.SecondPeer,
		intent.WorkspaceName,
		eventpb.FriendRelationshipChange_FRIEND_RELATIONSHIP_CHANGE_DELETED,
		intent.DeletedAt,
	)
}

func (s *Server) notifyRelationship(
	ctx context.Context,
	first string,
	second string,
	workspaceName string,
	change eventpb.FriendRelationshipChange,
	at time.Time,
) {
	if s == nil || s.NotifyPeer == nil {
		return
	}
	for _, recipient := range []struct {
		publicKey string
		peer      string
	}{
		{publicKey: first, peer: second},
		{publicKey: second, peer: first},
	} {
		s.NotifyPeer(ctx, recipient.publicKey, &eventpb.PeerEvent{
			Version: eventpb.Version,
			Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_FRIEND_RELATIONSHIP_UPDATED,
			Payload: &eventpb.PeerEvent_FriendRelationshipUpdated{
				FriendRelationshipUpdated: &eventpb.FriendRelationshipUpdated{
					PeerPublicKey:  recipient.peer,
					WorkspaceName:  workspaceName,
					Change:         change,
					RevisionUnixMs: at.UnixMilli(),
				},
			},
		})
	}
}

func retirementIntentKey(relationID string) kv.Key {
	return append(append(kv.Key{}, retirementIntentsRoot...), socialutil.EscapeStoreSegment(relationID))
}

func creationIntentKey(relationID string) kv.Key {
	return append(append(kv.Key{}, creationIntentsRoot...), socialutil.EscapeStoreSegment(relationID))
}

func creationDecisionKey(relationID string, incarnationID string) kv.Key {
	return append(
		append(kv.Key{}, creationDecisionsRoot...),
		socialutil.EscapeStoreSegment(relationID),
		socialutil.EscapeStoreSegment(incarnationID),
	)
}

func retirementReceiptKey(relationID string) kv.Key {
	return append(
		append(kv.Key{}, retirementReceiptsRoot...),
		socialutil.EscapeStoreSegment(relationID),
	)
}

func workspaceBindingKey(relationID string) kv.Key {
	return append(
		append(kv.Key{}, workspaceBindingsRoot...),
		socialutil.EscapeStoreSegment(relationID),
	)
}

func readWorkspaceBinding(ctx context.Context, store kv.Store, relationID string) (workspaceBinding, error) {
	binding, err := socialutil.ReadJSONValue[workspaceBinding](ctx, store, workspaceBindingKey(relationID))
	if err != nil {
		return workspaceBinding{}, err
	}
	if binding.RelationID != relationID ||
		binding.WorkspaceID == "" || binding.WorkspaceID != strings.TrimSpace(binding.WorkspaceID) ||
		binding.WorkspaceName == "" || binding.WorkspaceName != strings.TrimSpace(binding.WorkspaceName) {
		return workspaceBinding{}, fmt.Errorf("social: invalid Friend Workspace binding %q", relationID)
	}
	return binding, nil
}

func readCreationIntent(ctx context.Context, store kv.Store, relationID string) (creationIntent, error) {
	return socialutil.ReadJSONValue[creationIntent](ctx, store, creationIntentKey(relationID))
}

func deleteCreationIntent(
	ctx context.Context,
	store kv.Store,
	intent creationIntent,
) error {
	key := creationIntentKey(intent.RelationID)
	data, err := store.Get(ctx, key)
	if errors.Is(err, kv.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	var current creationIntent
	if err := json.Unmarshal(data, &current); err != nil {
		return err
	}
	current, err = validateCreationIntent(
		current,
		intent.RelationID,
		current.WorkspaceOwner,
	)
	if err != nil {
		return err
	}
	if current.IncarnationID != intent.IncarnationID ||
		current.Workspace != intent.Workspace {
		return nil
	}
	_, err = kv.CompareAndMutate(ctx, store, key, data, nil, []kv.Key{key})
	return err
}

func readCreationDecision(
	ctx context.Context,
	store kv.Store,
	intent creationIntent,
) (creationDecision, error) {
	return socialutil.ReadJSONValue[creationDecision](
		ctx,
		store,
		creationDecisionKey(intent.RelationID, intent.IncarnationID),
	)
}

func validateCreationDecision(decision creationDecision, intent creationIntent) error {
	if decision.RelationID != intent.RelationID ||
		decision.IncarnationID != intent.IncarnationID ||
		decision.Workspace != intent.Workspace ||
		(decision.State != creationDecisionCommitted &&
			decision.State != creationDecisionCancelled) {
		return fmt.Errorf(
			"social: invalid Friend creation decision %q",
			intent.RelationID,
		)
	}
	return nil
}

func readRetirementIntent(ctx context.Context, store kv.Store, relationID string) (retirementIntent, error) {
	return socialutil.ReadJSONValue[retirementIntent](ctx, store, retirementIntentKey(relationID))
}

func readRetirementReceipt(ctx context.Context, store kv.Store, relationID string) (retirementReceipt, error) {
	return socialutil.ReadJSONValue[retirementReceipt](ctx, store, retirementReceiptKey(relationID))
}

func validateRetirementIntent(
	intent retirementIntent,
	relationID string,
) (retirementIntent, error) {
	if intent.RelationID != strings.TrimSpace(intent.RelationID) ||
		intent.RelationID != relationID ||
		intent.FirstPeer == "" ||
		intent.SecondPeer == "" ||
		intent.FirstPeer != strings.TrimSpace(intent.FirstPeer) ||
		intent.SecondPeer != strings.TrimSpace(intent.SecondPeer) ||
		intent.FirstPeer > intent.SecondPeer ||
		socialutil.RelationID(intent.FirstPeer, intent.SecondPeer) != relationID ||
		(!intent.CancelCreation && (intent.WorkspaceID == "" || intent.WorkspaceID != strings.TrimSpace(intent.WorkspaceID))) ||
		intent.WorkspaceName == "" ||
		intent.WorkspaceName != strings.TrimSpace(intent.WorkspaceName) ||
		intent.DeletedAt.IsZero() {
		return retirementIntent{}, fmt.Errorf(
			"social: invalid Friend retirement intent %q",
			relationID,
		)
	}
	return intent, nil
}

func validateRetirementReceipt(receipt retirementReceipt, relationID string) error {
	if receipt.RelationID != strings.TrimSpace(receipt.RelationID) ||
		receipt.RelationID != relationID ||
		receipt.FirstPeer == "" ||
		receipt.SecondPeer == "" ||
		receipt.FirstPeer != strings.TrimSpace(receipt.FirstPeer) ||
		receipt.SecondPeer != strings.TrimSpace(receipt.SecondPeer) ||
		receipt.FirstPeer > receipt.SecondPeer ||
		socialutil.RelationID(receipt.FirstPeer, receipt.SecondPeer) != relationID ||
		(!receipt.CancelCreation && (receipt.WorkspaceID == "" || receipt.WorkspaceID != strings.TrimSpace(receipt.WorkspaceID))) ||
		receipt.WorkspaceName == "" ||
		receipt.WorkspaceName != strings.TrimSpace(receipt.WorkspaceName) ||
		receipt.DeletedAt.IsZero() {
		return fmt.Errorf("social: invalid Friend retirement receipt %q", relationID)
	}
	return nil
}

func friendObjectForRetirement(owner string, intent retirementIntent) rpcapi.FriendObject {
	item := intent.Relationship.peerObject()
	peer := intent.FirstPeer
	if owner == intent.FirstPeer {
		peer = intent.SecondPeer
	}
	item.Name = peer
	item.PeerPublicKey = &peer
	item.WorkspaceName = &intent.WorkspaceName
	return item
}

func friendObjectForReceipt(owner string, receipt retirementReceipt) rpcapi.FriendObject {
	peer := receipt.FirstPeer
	if owner == receipt.FirstPeer {
		peer = receipt.SecondPeer
	}
	return rpcapi.FriendObject{
		Name:          peer,
		PeerPublicKey: &peer,
		WorkspaceName: &receipt.WorkspaceName,
	}
}

func (s *Server) activeInviteToken(ctx context.Context, store kv.Store, owner string) (inviteTokenRecord, bool, error) {
	if owner == "" {
		return inviteTokenRecord{}, false, errors.New("social: peer public key is required")
	}
	record, err := socialutil.ReadJSONValue[inviteTokenRecord](ctx, store, socialutil.FriendInviteTokenKey(owner))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return inviteTokenRecord{}, false, nil
		}
		return inviteTokenRecord{}, false, err
	}
	if strings.TrimSpace(record.InviteToken) == "" || !record.ExpiresAt.After(s.now()) {
		_ = store.Delete(ctx, socialutil.FriendInviteTokenKey(owner))
		return inviteTokenRecord{}, false, nil
	}
	return record, true, nil
}

func (s *Server) findInviteToken(ctx context.Context, inviteToken string) (inviteTokenRecord, error) {
	if strings.TrimSpace(inviteToken) == "" {
		return inviteTokenRecord{}, ErrInviteTokenRequired
	}
	store, err := s.inviteTokensStore()
	if err != nil {
		return inviteTokenRecord{}, inviteTokenLookupError(err)
	}
	now := s.now()
	for entry, err := range store.List(ctx, socialutil.FriendInviteTokensRoot) {
		if err != nil {
			return inviteTokenRecord{}, inviteTokenLookupError(err)
		}
		var record inviteTokenRecord
		if err := json.Unmarshal(entry.Value, &record); err != nil {
			return inviteTokenRecord{}, inviteTokenLookupError(err)
		}
		if !record.ExpiresAt.After(now) {
			if err := store.Delete(ctx, entry.Key); err != nil {
				return inviteTokenRecord{}, inviteTokenLookupError(err)
			}
			continue
		}
		if strings.TrimSpace(record.InviteToken) == "" || record.CreatedAt.IsZero() ||
			!record.CreatedAt.Before(record.ExpiresAt) || record.PeerPublicKey == "" ||
			record.PeerPublicKey != strings.TrimSpace(record.PeerPublicKey) {
			return inviteTokenRecord{}, inviteTokenLookupError(errors.New("social: persisted Friend invite token is invalid"))
		}
		if record.InviteToken == inviteToken {
			return record, nil
		}
	}
	return inviteTokenRecord{}, ErrInviteTokenUnavailable
}

func inviteTokenLookupError(err error) error {
	return fmt.Errorf("%w: %w", ErrInviteTokenLookupFailed, err)
}

func (s *Server) inviteTokensStore() (kv.Store, error) {
	if s == nil || s.InviteTokens == nil {
		return nil, errors.New("social: friend invite token service not configured")
	}
	return s.InviteTokens, nil
}

func (s *Server) friendsStore() (kv.Store, error) {
	if s == nil || s.Friends == nil {
		return nil, errors.New("social: friend service not configured")
	}
	return s.Friends, nil
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
