package friend

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/sfu"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/internal/keyedlock"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/google/uuid"
)

type WorkspaceService interface {
	CreateSystemWorkspace(context.Context, adminhttp.WorkspaceUpsert) (apitypes.Workspace, bool, error)
	DeleteSystemWorkspace(context.Context, string) (apitypes.Workspace, error)
	RetireSystemWorkspaceByID(context.Context, string, socialutil.SFUWorkspaceKind, string) (apitypes.Workspace, error)
}

type ProfileService interface {
	GetSelfInfo(context.Context, giznet.PublicKey) (apitypes.DeviceInfo, error)
}

// PresenceService reads a Peer's shared connection presence. lastSeenAt is the
// zero time when no transition has been observed or the presence read failed.
type PresenceService = socialutil.PresenceService

// ErrSFUNotConfigured reports that the Server has no SFU URL, so no Friend
// Workspace can be bound to an SFU Room.
var ErrSFUNotConfigured = errors.New("social: SFU is not configured")

// Server owns Friend relationships. Every record it writes lives in the shared
// Social KV, so any Server in the deployment can create, resolve, or retire a
// relationship without consulting Server-local state.
type Server struct {
	InviteTokens     kv.Store
	Friends          kv.Store
	Workspaces       WorkspaceService
	Profiles         ProfileService
	NotifyPeer       func(context.Context, string, *eventpb.PeerEvent)
	PeerAvailability func(context.Context, string) error
	// Pings reaches Friend devices connected to this Server for
	// server.friend.ping; nil disables pinging.
	Pings socialutil.PingDelivery
	// Presence reports Friend device presence for server.friend.list; nil
	// leaves online and last_seen_at out of every listed Friend.
	Presence PresenceService
	// SFUURL is the SFU endpoint recorded in every new Friend SFU binding.
	SFUURL string

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

// PreparePeerFriendsRetirement freezes new Friend admissions for owner and
// inventories relationships only after the reader has observed that barrier.
func (s *Server) PreparePeerFriendsRetirement(ctx context.Context, owner string) ([]PeerRetirementFriend, error) {
	store, err := s.friendsStore()
	if err != nil {
		return nil, err
	}
	if owner == "" || owner != strings.TrimSpace(owner) {
		return nil, errors.New("social: Peer public key is required and must be canonical")
	}
	unlock, err := s.lockPeers(ctx, owner)
	if err != nil {
		return nil, err
	}
	defer unlock()
	ctx, err = socialutil.FencePeerRetirement(ctx, store, owner)
	if err != nil {
		return nil, err
	}
	if s.InviteTokens != nil {
		if _, err := socialutil.FencePeerRetirement(ctx, s.InviteTokens, owner); err != nil {
			return nil, err
		}
	}
	return snapshotPeerFriends(ctx, store, owner)
}

// SnapshotPeerFriends reads the current inventory without closing admissions.
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
	return snapshotPeerFriends(ctx, store, owner)
}

func snapshotPeerFriends(ctx context.Context, store kv.Store, owner string) ([]PeerRetirementFriend, error) {
	ids, err := store.ListMembers(ctx, friendCollectionKey(owner))
	if err != nil {
		return nil, err
	}
	slices.Sort(ids)
	var out []PeerRetirementFriend
	for _, id := range ids {
		record, err := socialutil.ReadJSONValue[friendRecord](ctx, store, socialutil.FriendKey(owner, id))
		if err != nil {
			return nil, err
		}
		if record.RelationID != id {
			return nil, errors.New("social: friend collection identity mismatch")
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
	}
	ctx = context.WithValue(ctx, peerRetirementSnapshotContextKey{}, snapshot)
	_, err = s.deleteFriendByRelationID(ctx, owner, snapshot.RelationID)
	return err
}

var (
	relationMutationGates keyedlock.Locker[string]
	peerMutationGates     keyedlock.Locker[string]
)

type peerRetirementSnapshotContextKey struct{}

func peerRetirementFromContext(ctx context.Context) bool {
	_, ok := ctx.Value(peerRetirementSnapshotContextKey{}).(PeerRetirementFriend)
	return ok
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
	WorkspaceOwner string       `json:"workspace_owner,omitempty"`
	Relationship   friendRecord `json:"relationship"`
	DeletedAt      time.Time    `json:"deleted_at"`
	CancelCreation bool         `json:"cancel_creation,omitempty"`
}

// creationIntent mints every identity of one relationship incarnation up
// front: the Workspace name and the SFU Room token. Retries and reconciliation
// reuse the persisted intent, so a committed relationship always carries the
// Room identity minted for its incarnation.
type creationIntent struct {
	RelationID     string    `json:"relation_id"`
	FirstPeer      string    `json:"first_peer"`
	SecondPeer     string    `json:"second_peer"`
	WorkspaceOwner string    `json:"workspace_owner"`
	IncarnationID  string    `json:"incarnation_id"`
	Workspace      string    `json:"workspace_name"`
	SFUURL         string    `json:"sfu_url"`
	RoomToken      string    `json:"room_token"`
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
	WorkspaceOwner string    `json:"workspace_owner,omitempty"`
	DeletedAt      time.Time `json:"deleted_at"`
	CancelCreation bool      `json:"cancel_creation,omitempty"`
}

// workspaceBinding is the canonical relationship-level record shared by both
// directional Friend rows. It carries the SFU Room binding so every Server can
// materialize the same Workspace and attach to the same Room.
type workspaceBinding struct {
	RelationID     string                `json:"relation_id"`
	WorkspaceID    string                `json:"workspace_id"`
	WorkspaceName  string                `json:"workspace_name"`
	WorkspaceOwner string                `json:"workspace_owner"`
	SFU            socialutil.SFUBinding `json:"sfu"`
}

func (binding workspaceBinding) sfuWorkspaceBinding(members []string) socialutil.SFUWorkspaceBinding {
	return socialutil.SFUWorkspaceBinding{
		WorkspaceID:   binding.WorkspaceID,
		WorkspaceName: binding.WorkspaceName,
		Kind:          socialutil.SFUWorkspaceKindFriend,
		SocialID:      binding.RelationID,
		Owner:         binding.WorkspaceOwner,
		Members:       members,
		SFU:           binding.SFU,
	}
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
var errFriendChanged = errors.New("social: Friend relationship changed concurrently; retry")

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
	return s.CreateFriendInviteTokenWithTTL(ctx, owner, 0)
}

// CreateFriendInviteTokenWithTTL returns the owner's active invite token or
// creates one. A zero ttl keeps the server.friend.invite_token.create
// behavior: an active token is returned unchanged and a new one lives for
// socialutil.DefaultInviteTokenTTL. A non-zero ttl must pass
// socialutil.ValidateInviteTokenTTL; an active token keeps its value and its
// expiry is extended to now+ttl, never shortened.
func (s *Server) CreateFriendInviteTokenWithTTL(ctx context.Context, owner string, ttl time.Duration) (rpcapi.FriendInviteTokenCreateResponse, error) {
	store, err := s.inviteTokensStore()
	if err != nil {
		return rpcapi.FriendInviteTokenCreateResponse{}, err
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return rpcapi.FriendInviteTokenCreateResponse{}, errors.New("social: peer public key is required")
	}
	if ttl != 0 {
		if err := socialutil.ValidateInviteTokenTTL(ttl); err != nil {
			return rpcapi.FriendInviteTokenCreateResponse{}, err
		}
	}
	for range 16 {
		if err := checkPeerFriendAdmissions(ctx, store, owner); err != nil {
			return rpcapi.FriendInviteTokenCreateResponse{}, err
		}
		data, err := socialutil.UpdateInviteToken(ctx, store, socialutil.FriendInviteTokenKey(owner),
			[]kv.Condition{socialutil.PeerRetirementCondition(owner)}, func(current []byte) ([]byte, error) {
				var record inviteTokenRecord
				if current != nil {
					if err := json.Unmarshal(current, &record); err != nil {
						return nil, err
					}
				}
				now := s.now()
				if current != nil && strings.TrimSpace(record.InviteToken) != "" && record.ExpiresAt.After(now) {
					if record.PeerPublicKey != owner {
						return nil, errors.New("social: invite token owner mismatch")
					}
					if ttl != 0 && record.ExpiresAt.Before(now.Add(ttl)) {
						record.ExpiresAt = now.Add(ttl)
					}
				} else {
					lifetime := ttl
					if lifetime == 0 {
						lifetime = s.inviteTokenTTL()
					}
					record = inviteTokenRecord{PeerPublicKey: owner, InviteToken: s.newID(), CreatedAt: now, ExpiresAt: now.Add(lifetime)}
				}
				return json.Marshal(record)
			})
		if errors.Is(err, socialutil.ErrInviteTokenChanged) {
			ctx = kv.WithPrimaryReads(ctx)
			continue
		}
		if err != nil {
			return rpcapi.FriendInviteTokenCreateResponse{}, err
		}
		var record inviteTokenRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return rpcapi.FriendInviteTokenCreateResponse{}, err
		}
		return rpcapi.FriendInviteTokenCreateResponse{InviteToken: record.InviteToken, ExpiresAt: record.ExpiresAt}, nil
	}
	return rpcapi.FriendInviteTokenCreateResponse{}, socialutil.ErrInviteTokenChanged
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
	if err := socialutil.DeleteInviteToken(ctx, store, socialutil.FriendInviteTokenKey(owner)); err != nil && !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendInviteTokenClearResponse{}, err
	}
	return rpcapi.FriendInviteTokenClearResponse{}, nil
}

func (s *Server) AddFriend(ctx context.Context, owner string, req rpcapi.FriendAddRequest) (rpcapi.FriendAddResponse, error) {
	item, _, err := s.AddFriendReportingExisting(ctx, owner, req)
	return item, err
}

// AddFriendReportingExisting is AddFriend that also reports whether the
// relationship was already active, in which case it is returned unchanged.
func (s *Server) AddFriendReportingExisting(ctx context.Context, owner string, req rpcapi.FriendAddRequest) (rpcapi.FriendObject, bool, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return rpcapi.FriendObject{}, false, errors.New("social: peer public key is required")
	}
	record, err := s.findInviteToken(ctx, req.InviteToken)
	if err != nil {
		return rpcapi.FriendObject{}, false, err
	}
	to := record.PeerPublicKey
	if owner == to {
		return rpcapi.FriendObject{}, false, ErrInviteTokenSelfOwned
	}
	store, err := s.friendsStore()
	if err != nil {
		return rpcapi.FriendObject{}, false, err
	}
	relationID := socialutil.RelationID(owner, to)
	unlock, err := s.lockRelationMutation(ctx, relationID, owner, to)
	if err != nil {
		return rpcapi.FriendObject{}, false, err
	}
	defer unlock()
	if existing, active, err := readActiveRelationship(ctx, store, owner, to); err != nil {
		return rpcapi.FriendObject{}, false, err
	} else if active {
		return existing, true, nil
	}
	item, err := s.createFriend(ctx, owner, to, to)
	return item, false, err
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
	relationID := socialutil.RelationID(owner, peerPublicKey)
	unlock, err := s.lockRelationMutation(ctx, relationID, owner, peerPublicKey)
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	defer unlock()
	return s.createFriend(ctx, owner, peerPublicKey, owner)
}

func (s *Server) lockRelation(ctx context.Context, relationID string) (func(), error) {
	return relationMutationGates.Acquire(ctx, relationID)
}

func (s *Server) lockRelationMutation(ctx context.Context, relationID string, peers ...string) (func(), error) {
	releasePeers, err := s.lockPeers(ctx, peers...)
	if err != nil {
		return nil, err
	}
	releaseRelation, err := s.lockRelation(ctx, relationID)
	if err != nil {
		releasePeers()
		return nil, err
	}
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

func (s *Server) AdminListFriends(ctx context.Context, cursor *string, limit *int) (adminhttp.AdminFriendListResponse, error) {
	store, err := s.friendsStore()
	if err != nil {
		return adminhttp.AdminFriendListResponse{}, err
	}
	_, pageLimit := socialutil.NormalizeListParams("", socialutil.IntValue(limit))
	keys, err := adminFriendPageKeys(ctx, store, socialutil.StringValue(cursor), pageLimit+1)
	if err != nil {
		return adminhttp.AdminFriendListResponse{}, err
	}
	hasNext := len(keys) > pageLimit
	if hasNext {
		keys = keys[:pageLimit]
	}
	items, err := loadAdminFriendRows(ctx, store, keys)
	if err != nil {
		return adminhttp.AdminFriendListResponse{}, err
	}
	var next *string
	if hasNext && len(keys) > 0 {
		value := adminFriendIndexCursor(keys[len(keys)-1])
		next = &value
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
	projected, err := s.adminFriendObject(ctx, owner, item)
	if errors.Is(err, kv.ErrNotFound) || errors.Is(err, errFriendChanged) {
		return s.adminFriendObject(kv.WithPrimaryReads(ctx), owner, item)
	}
	return projected, err
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
	projected, err := s.adminFriendObject(ctx, owner, item)
	if errors.Is(err, kv.ErrNotFound) || errors.Is(err, errFriendChanged) {
		return s.adminFriendObject(kv.WithPrimaryReads(ctx), owner, item)
	}
	return projected, err
}

func (s *Server) ListFriends(ctx context.Context, owner string, req rpcapi.FriendListRequest) (rpcapi.FriendListResponse, error) {
	store, err := s.friendsStore()
	if err != nil {
		return rpcapi.FriendListResponse{}, err
	}
	escapedCursor, limit := socialutil.NormalizeListParams(socialutil.StringValue(req.Cursor), socialutil.IntValue(req.Limit))
	ids, err := store.RangeOrderedMembers(ctx, friendPageKey(owner), kv.OrderedRange{After: &escapedCursor, Limit: limit + 1})
	if err != nil {
		return rpcapi.FriendListResponse{}, err
	}
	hasNext := len(ids) > limit
	ids = ids[:min(limit, len(ids))]
	var nextCursor *string
	if hasNext {
		nextCursor = new(socialutil.UnescapeStoreSegment(ids[len(ids)-1]))
	}
	items := make([]rpcapi.FriendObject, 0, len(ids))
	for _, escapedID := range ids {
		id := socialutil.UnescapeStoreSegment(escapedID)
		record, err := socialutil.ReadJSONValue[friendRecord](ctx, store, socialutil.FriendKey(owner, id))
		if errors.Is(err, kv.ErrNotFound) {
			continue
		}
		if err != nil {
			return rpcapi.FriendListResponse{}, err
		}
		if err := record.validate(); err != nil {
			return rpcapi.FriendListResponse{}, err
		}
		if record.RelationID != id {
			return rpcapi.FriendListResponse{}, errors.New("social: friend collection identity mismatch")
		}
		item := record.peerObject()
		s.addFriendListProfile(ctx, &item)
		items = append(items, item)
	}
	socialutil.AddFriendPresence(ctx, s.Presence, items)
	return rpcapi.FriendListResponse{Items: items, HasNext: hasNext, NextCursor: nextCursor}, nil
}

// addFriendListProfile adds the Friend's profile to one server.friend.list item.
// A Friend whose profile cannot be read is still listed, without a display name
// or emoji.
func (s *Server) addFriendListProfile(ctx context.Context, item *rpcapi.FriendObject) {
	peerPublicKey := socialutil.StringValue(item.PeerPublicKey)
	if s.Profiles == nil {
		return
	}
	var publicKey giznet.PublicKey
	if err := publicKey.UnmarshalText([]byte(peerPublicKey)); err != nil {
		return
	}
	info, err := s.Profiles.GetSelfInfo(ctx, publicKey)
	if err != nil {
		return
	}
	// Like server.friend.info.get, an explicitly empty name or emoji stays
	// present so clients can tell it from an unset one.
	item.DisplayName = info.Name
	item.Emoji = info.Emoji
}

func friendCollectionKey(owner string) kv.Key {
	return kv.Key{"friend-collections", socialutil.EscapeStoreSegment(strings.TrimSpace(owner))}
}

func peerFriendRevisionKey(owner string) kv.Key {
	return kv.Key{"friend-peer-revisions", socialutil.EscapeStoreSegment(strings.TrimSpace(owner))}
}

func checkPeerFriendAdmissions(ctx context.Context, store kv.Store, peers ...string) error {
	for _, peer := range peers {
		if _, err := store.Get(ctx, socialutil.PeerRetirementFenceKey(peer)); err == nil {
			return ownership.ErrOwnerRetired
		} else if !errors.Is(err, kv.ErrNotFound) {
			return err
		}
	}
	return nil
}

func (s *Server) checkFriendAvailability(ctx context.Context, peers ...string) error {
	if s.PeerAvailability != nil {
		for _, peer := range peers {
			if err := s.PeerAvailability(ctx, peer); err != nil {
				return err
			}
		}
	}
	return nil
}

// readPeerFriendCapacity rejects one more Friend for each Peer that already
// has socialutil.PeerFriendLimit Friends. It returns each Peer's relationship
// revision, read before its count, as the Conditions a creation must commit
// under; every creation advances them. Retirements leave the revisions alone
// because they only lower the count.
func readPeerFriendCapacity(ctx context.Context, store kv.Store, peers ...string) ([]kv.Condition, error) {
	conditions := make([]kv.Condition, 0, len(peers))
	for _, peer := range peers {
		key := peerFriendRevisionKey(peer)
		revision, err := store.Get(ctx, key)
		if errors.Is(err, kv.ErrNotFound) {
			revision = nil
		} else if err != nil {
			return nil, err
		}
		friends, err := store.ListMembers(ctx, friendCollectionKey(peer))
		if err != nil {
			return nil, err
		}
		if len(friends) >= socialutil.PeerFriendLimit {
			return nil, fmt.Errorf("%w: %d friends", ErrPeerFriendLimit, len(friends))
		}
		conditions = append(conditions, kv.Condition{Key: key, Expected: revision})
	}
	return conditions, nil
}

// peerFriendRevisionsChanged reports whether any revision guarded by
// conditions moved after readPeerFriendCapacity read it.
func peerFriendRevisionsChanged(ctx context.Context, store kv.Store, conditions []kv.Condition) (bool, error) {
	for _, condition := range conditions {
		current, err := store.Get(ctx, condition.Key)
		if errors.Is(err, kv.ErrNotFound) {
			current = nil
		} else if err != nil {
			return false, err
		}
		if !bytes.Equal(current, condition.Expected) || (current == nil) != (condition.Expected == nil) {
			return true, nil
		}
	}
	return false, nil
}

// WorkspaceRecipientsByID returns the peers whose reciprocal relationship
// binds the canonical Friend SFU Workspace.
func (s *Server) WorkspaceRecipientsByID(ctx context.Context, workspaceID string) ([]string, error) {
	if err := customid.ValidateResourceID(workspaceID); err != nil {
		return nil, err
	}
	store, err := s.friendsStore()
	if err != nil {
		return nil, err
	}
	locator, err := socialutil.ReadJSONValue[socialutil.WorkspaceBindingLocator](ctx, store, socialutil.WorkspaceLocatorIDKey(workspaceID))
	if errors.Is(err, kv.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := locator.Validate(); err != nil {
		return nil, err
	}
	first, _, ok := relationPeers(locator.ResourceID)
	if !ok {
		return nil, errors.New("social: invalid Friend relation locator")
	}
	binding, err := s.ResolveSFUWorkspaceBinding(ctx, workspaceID, first)
	if errors.Is(err, kv.ErrNotFound) || errors.Is(err, sfu.ErrRevoked) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return binding.Members, nil
}

func (s *Server) DeleteFriend(ctx context.Context, owner string, req rpcapi.FriendDeleteRequest) (rpcapi.FriendObject, error) {
	owner = strings.TrimSpace(owner)
	if req.Name == "" || req.Name != strings.TrimSpace(req.Name) {
		return rpcapi.FriendObject{}, errors.New("social: Friend name is required without surrounding whitespace")
	}
	return s.deleteFriendByRelationID(ctx, owner, socialutil.RelationID(owner, req.Name))
}

func (s *Server) deleteFriendByRelationID(ctx context.Context, owner, relationID string) (rpcapi.FriendObject, error) {
	item, err := s.deleteFriendByRelationIDAttempt(ctx, owner, relationID)
	if !errors.Is(err, errFriendChanged) {
		return item, err
	}
	// The failed attempt has released both Peer and relation locks.
	return s.deleteFriendByRelationIDAttempt(kv.WithPrimaryReads(ctx), owner, relationID)
}

func (s *Server) deleteFriendByRelationIDAttempt(ctx context.Context, owner, relationID string) (rpcapi.FriendObject, error) {
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
	bindingData, err := store.Get(ctx, workspaceBindingKey(relationID))
	if errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendObject{}, errFriendChanged
	}
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	var binding workspaceBinding
	if err := json.Unmarshal(bindingData, &binding); err != nil {
		return rpcapi.FriendObject{}, err
	}
	if err := validateWorkspaceBinding(binding, relationID); err != nil {
		return rpcapi.FriendObject{}, err
	}
	if binding.WorkspaceName != workspaceName {
		return rpcapi.FriendObject{}, errFriendChanged
	}
	if snapshot, ok := ctx.Value(peerRetirementSnapshotContextKey{}).(PeerRetirementFriend); ok &&
		(binding.WorkspaceID != snapshot.WorkspaceID || binding.WorkspaceName != snapshot.WorkspaceName) {
		return rpcapi.FriendObject{}, errors.New("social: Friend Workspace no longer matches Peer retirement snapshot")
	}
	conditions := []kv.Condition{{Key: workspaceBindingKey(relationID), Expected: bindingData}}
	for _, row := range []struct{ owner, peer string }{{owner, other}, {other, owner}} {
		key := socialutil.FriendKey(row.owner, relationID)
		data, err := store.Get(ctx, key)
		if errors.Is(err, kv.ErrNotFound) {
			return rpcapi.FriendObject{}, errFriendChanged
		}
		if err != nil {
			return rpcapi.FriendObject{}, err
		}
		var record friendRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return rpcapi.FriendObject{}, err
		}
		if err := record.validate(); err != nil {
			return rpcapi.FriendObject{}, err
		}
		if record.RelationID != relationID || record.PeerPublicKey != row.peer || record.WorkspaceName != workspaceName {
			return rpcapi.FriendObject{}, errFriendChanged
		}
		conditions = append(conditions, kv.Condition{Key: key, Expected: data})
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
	retired, err := store.ApplyMutation(ctx, kv.Mutation{
		Conditions:           conditions,
		Entries:              []kv.Entry{{Key: retirementIntentKey(relationID), Value: data}},
		AddMembers:           (socialutil.RecoveryIndex{Root: retirementIntentsRoot}).Add(relationID),
		DeleteKeys:           []kv.Key{socialutil.FriendKey(owner, relationID), socialutil.FriendKey(other, relationID), workspaceBindingKey(relationID)},
		RemoveMembers:        []kv.SetMembers{{Key: friendCollectionKey(owner), Members: []string{relationID}}, {Key: friendCollectionKey(other), Members: []string{relationID}}},
		RemoveOrderedMembers: []kv.SetMembers{adminFriendMembership(owner, relationID), adminFriendMembership(other, relationID), friendPageMembership(owner, relationID), friendPageMembership(other, relationID)},
	})
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	if !retired {
		return rpcapi.FriendObject{}, errFriendChanged
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
		if socialutil.StringValue(item.WorkspaceName) != creation.Workspace {
			return rpcapi.FriendObject{}, errFriendChanged
		}
		return s.retireActiveFriend(ctx, store, owner, relationID, item)
	}
	intent := retirementIntent{
		RelationID:     creation.RelationID,
		FirstPeer:      creation.FirstPeer,
		SecondPeer:     creation.SecondPeer,
		WorkspaceName:  creation.Workspace,
		WorkspaceOwner: creation.WorkspaceOwner,
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
	created, err := store.ApplyMutation(ctx, kv.Mutation{
		Conditions: []kv.Condition{{Key: creationDecisionKey(creation.RelationID, creation.IncarnationID)}},
		Entries:    []kv.Entry{{Key: creationDecisionKey(creation.RelationID, creation.IncarnationID), Value: decisionData}, {Key: retirementIntentKey(intent.RelationID), Value: data}},
		AddMembers: (socialutil.RecoveryIndex{Root: retirementIntentsRoot}).Add(intent.RelationID),
	})
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	if !created {
		ctx = kv.WithPrimaryReads(ctx)
		existing, err := store.Get(ctx, creationDecisionKey(creation.RelationID, creation.IncarnationID))
		if err != nil {
			return rpcapi.FriendObject{}, err
		}
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
			if !active || socialutil.StringValue(item.WorkspaceName) != creation.Workspace {
				return rpcapi.FriendObject{}, errFriendChanged
			}
			return s.retireActiveFriend(ctx, store, owner, relationID, item)
		}
		persisted, err := readRetirementIntent(ctx, store, relationID)
		if err == nil {
			persisted, err = validateRetirementIntent(persisted, relationID)
			if err != nil {
				return rpcapi.FriendObject{}, err
			}
			if persisted.CancelCreation && persisted.WorkspaceOwner == creation.WorkspaceOwner &&
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
	data, err := store.Get(ctx, retirementReceiptKey(relationID))
	if err == nil {
		var receipt retirementReceipt
		if err := json.Unmarshal(data, &receipt); err != nil {
			return rpcapi.FriendObject{}, err
		}
		if err := validateRetirementReceipt(receipt, relationID); err != nil {
			return rpcapi.FriendObject{}, err
		}
		if owner != receipt.FirstPeer && owner != receipt.SecondPeer {
			return rpcapi.FriendObject{}, kv.ErrNotFound
		}
		if snapshot, ok := ctx.Value(peerRetirementSnapshotContextKey{}).(PeerRetirementFriend); ok &&
			(snapshot.WorkspaceID != receipt.WorkspaceID || snapshot.WorkspaceName != receipt.WorkspaceName) {
			return rpcapi.FriendObject{}, errors.New("social: completed Friend retirement does not match snapshot")
		}
		// A historical receipt may coexist with a newer active incarnation.
		// Acknowledge it only if the writer confirms this pair is still absent.
		matched, err := store.ApplyMutation(ctx, kv.Mutation{Conditions: []kv.Condition{
			{Key: retirementReceiptKey(relationID), Expected: data},
			{Key: socialutil.FriendKey(receipt.FirstPeer, relationID)},
			{Key: socialutil.FriendKey(receipt.SecondPeer, relationID)},
			{Key: workspaceBindingKey(relationID)},
			{Key: creationIntentKey(relationID)},
			{Key: retirementIntentKey(relationID)},
		}})
		if err != nil {
			return rpcapi.FriendObject{}, err
		}
		if !matched {
			return rpcapi.FriendObject{}, errFriendChanged
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
	if binding.WorkspaceName != socialutil.StringValue(item.WorkspaceName) {
		return adminhttp.AdminFriendObject{}, errFriendChanged
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
	for range 2 {
		existing, active, err := readActiveRelationship(ctx, store, from, to)
		if err != nil {
			return rpcapi.FriendObject{}, err
		}
		if active {
			return existing, nil
		}
		if err := s.checkFriendAvailability(ctx, from, to); err != nil {
			return rpcapi.FriendObject{}, err
		}
		if err := checkPeerFriendAdmissions(ctx, store, from, to); err != nil {
			return rpcapi.FriendObject{}, err
		}
		// Reject before creating the Workspace; commitFriendCreation repeats
		// the check under the Peer revisions.
		if _, err := readPeerFriendCapacity(ctx, store, from, to); err != nil {
			return rpcapi.FriendObject{}, err
		}
		intent, err := s.getOrCreateCreationIntent(
			ctx,
			store,
			from,
			to,
			workspaceOwner,
		)
		if errors.Is(err, errFriendChanged) {
			ctx = kv.WithPrimaryReads(ctx)
			continue
		}
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
			ctx = kv.WithPrimaryReads(ctx)
			continue
		}
		workspace, err := s.ensureCreationWorkspace(ctx, intent)
		if errors.Is(err, errFriendChanged) {
			ctx = kv.WithPrimaryReads(ctx)
			continue
		}
		if err != nil {
			return rpcapi.FriendObject{}, err
		}
		item, err := s.commitFriendCreation(ctx, store, from, to, intent, workspace)
		if errors.Is(err, errFriendCapacityChanged) {
			// Reuse this incarnation and recheck capacity at the commit
			// boundary; a now-full Peer must cancel its pending Workspace.
			item, err = s.commitFriendCreation(kv.WithPrimaryReads(ctx), store, from, to, intent, workspace)
		}
		return item, err
	}
	return rpcapi.FriendObject{}, errFriendCapacityChanged
}

func readActiveRelationship(
	ctx context.Context,
	store kv.Store,
	from string,
	to string,
) (rpcapi.FriendObject, bool, error) {
	item, active, err := readActiveRelationshipSnapshot(ctx, store, from, to)
	if !errors.Is(err, errFriendChanged) {
		return item, active, err
	}
	return readActiveRelationshipSnapshot(kv.WithPrimaryReads(ctx), store, from, to)
}

func readActiveRelationshipSnapshot(ctx context.Context, store kv.Store, from, to string) (rpcapi.FriendObject, bool, error) {
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
		return rpcapi.FriendObject{}, false, errors.Join(errors.New(
			"social: reciprocal Friend relationship is incomplete",
		), errFriendChanged)
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
		return rpcapi.FriendObject{}, false, errors.Join(errors.New(
			"social: reciprocal Friend relationship is inconsistent",
		), errFriendChanged)
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
	sfuURL := strings.TrimSpace(s.SFUURL)
	if sfuURL == "" {
		return creationIntent{}, ErrSFUNotConfigured
	}
	incarnationID := strings.TrimSpace(s.newID())
	if incarnationID == "" {
		return creationIntent{}, errors.New("social: friend Workspace incarnation is empty")
	}
	roomToken, err := socialutil.NewRoomToken()
	if err != nil {
		return creationIntent{}, err
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
		SFUURL:         sfuURL,
		RoomToken:      roomToken,
		CreatedAt:      s.now(),
	}
	if _, err := validateCreationIntent(intent, relationID, workspaceOwner); err != nil {
		return creationIntent{}, err
	}
	data, err := json.Marshal(intent)
	if err != nil {
		return creationIntent{}, err
	}
	created, err := store.ApplyMutation(ctx, kv.Mutation{
		Conditions: []kv.Condition{{Key: creationIntentKey(relationID)}, {Key: socialutil.FriendKey(from, relationID)}, {Key: socialutil.FriendKey(to, relationID)}, socialutil.PeerRetirementCondition(from), socialutil.PeerRetirementCondition(to)},
		Entries:    []kv.Entry{{Key: creationIntentKey(relationID), Value: data}},
		AddMembers: (socialutil.RecoveryIndex{Root: creationIntentsRoot}).Add(relationID),
	})
	if err != nil {
		return creationIntent{}, err
	}
	if created {
		return intent, nil
	}
	ctx = kv.WithPrimaryReads(ctx)
	if err := checkPeerFriendAdmissions(ctx, store, from, to); err != nil {
		return creationIntent{}, err
	}
	if _, active, err := readActiveRelationship(ctx, store, from, to); err != nil {
		return creationIntent{}, err
	} else if active {
		return creationIntent{}, errFriendChanged
	}
	existing, err := store.Get(ctx, creationIntentKey(relationID))
	if err != nil {
		return creationIntent{}, err
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
		intent.SFUURL == "" ||
		intent.SFUURL != strings.TrimSpace(intent.SFUURL) ||
		intent.RoomToken == "" ||
		intent.RoomToken != strings.TrimSpace(intent.RoomToken) ||
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

func creationAdmissionConditions(intent creationIntent, data []byte) []kv.Condition {
	return []kv.Condition{
		{Key: creationIntentKey(intent.RelationID), Expected: data},
		{Key: creationDecisionKey(intent.RelationID, intent.IncarnationID)},
		{Key: workspaceBindingKey(intent.RelationID)},
		{Key: socialutil.FriendKey(intent.FirstPeer, intent.RelationID)},
		{Key: socialutil.FriendKey(intent.SecondPeer, intent.RelationID)},
		socialutil.PeerRetirementCondition(intent.FirstPeer),
		socialutil.PeerRetirementCondition(intent.SecondPeer),
	}
}

func (s *Server) ensureCreationWorkspace(ctx context.Context, intent creationIntent) (apitypes.Workspace, error) {
	if s == nil || s.Workspaces == nil {
		return apitypes.Workspace{}, errors.New("social: Workspace creation service not configured")
	}
	store, err := s.friendsStore()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	intentData, err := json.Marshal(intent)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	// Prove this is still the pending, undecided incarnation before a SQL
	// side effect. A frozen reader may retain an intent after its retirement.
	admitted, err := store.ApplyMutation(ctx, kv.Mutation{Conditions: creationAdmissionConditions(intent, intentData)})
	if err != nil {
		return apitypes.Workspace{}, err
	}
	if !admitted {
		ctx = kv.WithPrimaryReads(ctx)
		if err := checkPeerFriendAdmissions(ctx, store, intent.FirstPeer, intent.SecondPeer); err != nil {
			if errors.Is(err, ownership.ErrOwnerRetired) {
				_, cancelErr := s.cancelFriendCreation(ctx, store, intent.FirstPeer, intent.RelationID, intent)
				if cancelErr != nil {
					return apitypes.Workspace{}, errors.Join(err, cancelErr)
				}
				return apitypes.Workspace{}, errors.Join(errFriendCreationCancelled, err)
			}
			return apitypes.Workspace{}, err
		}
		state, err := s.reconcileCreationDecision(ctx, store, intent)
		if err != nil {
			return apitypes.Workspace{}, err
		}
		if state != "" {
			return apitypes.Workspace{}, errFriendCreationCancelled
		}
		return apitypes.Workspace{}, errFriendChanged
	}
	body := adminhttp.WorkspaceUpsert{
		Id:         intent.Workspace,
		Name:       intent.Workspace,
		WorkflowId: socialutil.SFUWorkflowID,
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
	if err := s.checkFriendAvailability(ctx, from, to); err != nil {
		if errors.Is(err, ownership.ErrOwnerRetired) {
			cancelErr := s.cancelRejectedFriendCreation(ctx, store, intent, workspace)
			return rpcapi.FriendObject{}, errors.Join(err, cancelErr)
		}
		return rpcapi.FriendObject{}, err
	}
	capacity, err := readPeerFriendCapacity(ctx, store, from, to)
	if errors.Is(err, ErrPeerFriendLimit) {
		// Cancel the pending creation so recovery does not keep retrying a
		// relationship that can no longer fit, and release its Workspace.
		if cancelErr := s.cancelRejectedFriendCreation(ctx, store, intent, workspace); cancelErr != nil {
			return rpcapi.FriendObject{}, errors.Join(err, cancelErr)
		}
		return rpcapi.FriendObject{}, err
	}
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
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
		RelationID:     relationID,
		WorkspaceID:    workspace.Id,
		WorkspaceName:  workspace.Name,
		WorkspaceOwner: intent.WorkspaceOwner,
		SFU: socialutil.SFUBinding{
			URL:        intent.SFUURL,
			RoomToken:  intent.RoomToken,
			Generation: 1,
		},
	}
	bindingData, err := json.Marshal(binding)
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	entries = append(entries, kv.Entry{Key: workspaceBindingKey(relationID), Value: bindingData})
	locatorEntries, err := socialutil.WorkspaceLocatorEntries(socialutil.WorkspaceBindingLocator{ResourceID: relationID, WorkspaceID: binding.WorkspaceID, WorkspaceName: binding.WorkspaceName})
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	entries = append(entries, locatorEntries...)

	entries = append(entries, kv.Entry{Key: creationDecisionKey(relationID, intent.IncarnationID), Value: decisionData})
	intentData, err := json.Marshal(intent)
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	conditions := append(creationAdmissionConditions(intent, intentData), capacity...)
	for _, condition := range capacity {
		entries = append(entries, kv.Entry{Key: condition.Key, Value: []byte(uuid.NewString())})
	}
	created, err := store.ApplyMutation(ctx, kv.Mutation{
		Conditions: conditions, Entries: entries,
		AddMembers:        []kv.SetMembers{{Key: friendCollectionKey(from), Members: []string{relationID}}, {Key: friendCollectionKey(to), Members: []string{relationID}}, adminFriendDirectoryMembership(from, relationID), adminFriendDirectoryMembership(to, relationID)},
		AddOrderedMembers: []kv.SetMembers{adminFriendMembership(from, relationID), adminFriendMembership(to, relationID), friendPageMembership(from, relationID), friendPageMembership(to, relationID)},
	})
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	if !created {
		ctx = kv.WithPrimaryReads(ctx)
		existingDecision, err := store.Get(ctx, creationDecisionKey(relationID, intent.IncarnationID))
		if errors.Is(err, kv.ErrNotFound) {
			if admissionErr := checkPeerFriendAdmissions(ctx, store, from, to); admissionErr != nil {
				if errors.Is(admissionErr, ownership.ErrOwnerRetired) {
					cancelErr := s.cancelRejectedFriendCreation(ctx, store, intent, workspace)
					return rpcapi.FriendObject{}, errors.Join(admissionErr, cancelErr)
				}
				return rpcapi.FriendObject{}, admissionErr
			}
			if changed, changedErr := peerFriendRevisionsChanged(ctx, store, capacity); changedErr != nil {
				return rpcapi.FriendObject{}, changedErr
			} else if changed {
				return rpcapi.FriendObject{}, errFriendCapacityChanged
			}
		}
		if err != nil {
			return rpcapi.FriendObject{}, err
		}
		if err := json.Unmarshal(existingDecision, &decision); err != nil {
			return rpcapi.FriendObject{}, err
		}
		if err := validateCreationDecision(decision, intent); err != nil {
			return rpcapi.FriendObject{}, err
		}
		switch decision.State {
		case creationDecisionCancelled:
			if err := s.completeFriendCreationCancellation(ctx, store, relationID, intent.WorkspaceOwner, intent.Workspace); err != nil {
				return rpcapi.FriendObject{}, err
			}
			if err := deleteCreationIntent(ctx, store, intent); err != nil {
				return rpcapi.FriendObject{}, err
			}
			return rpcapi.FriendObject{}, errFriendCreationCancelled
		case creationDecisionCommitted:
			existing, active, err := readActiveRelationship(ctx, store, from, to)
			if err != nil {
				return rpcapi.FriendObject{}, err
			}
			if !active || socialutil.StringValue(existing.WorkspaceName) != intent.Workspace {
				if err := s.compensateRetiredCreation(ctx, store, intent, workspace); err != nil {
					return rpcapi.FriendObject{}, err
				}
				return rpcapi.FriendObject{}, errFriendCreationCancelled
			}
			persistedBinding, bindingErr := readWorkspaceBinding(ctx, store, relationID)
			if bindingErr != nil {
				return rpcapi.FriendObject{}, bindingErr
			}
			if persistedBinding.WorkspaceID != workspace.Id || persistedBinding.WorkspaceName != intent.Workspace ||
				persistedBinding.SFU.RoomToken != intent.RoomToken {
				return rpcapi.FriendObject{}, errors.New("social: committed Friend creation has inconsistent Workspace binding")
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

func (s *Server) cancelRejectedFriendCreation(ctx context.Context, store kv.Store, intent creationIntent, workspace apitypes.Workspace) error {
	_, err := s.cancelFriendCreation(ctx, store, intent.FirstPeer, intent.RelationID, intent)
	if !errors.Is(err, errFriendChanged) {
		return err
	}
	// A late creator must not retire a replacement active relationship. Prove
	// an immutable decision exists before refreshing and cleaning its own SQL
	// Workspace, including when the first observation came from a replica.
	undecided, probeErr := store.ApplyMutation(ctx, kv.Mutation{Conditions: []kv.Condition{{Key: creationDecisionKey(intent.RelationID, intent.IncarnationID)}}})
	if probeErr != nil {
		return probeErr
	}
	if undecided {
		return err
	}
	ctx = kv.WithPrimaryReads(ctx)
	decision, err := readCreationDecision(ctx, store, intent)
	if err != nil {
		return err
	}
	if err := validateCreationDecision(decision, intent); err != nil {
		return err
	}
	if decision.State == creationDecisionCancelled {
		_, err := s.reconcileCreationDecision(ctx, store, intent)
		return err
	}
	return s.compensateRetiredCreation(ctx, store, intent, workspace)
}

// compensateRetiredCreation is used only after a rejected creation CAS proved
// the immutable committed decision belongs to an incarnation no longer active.
// Its Workspace name is unique to that incarnation; a newer binding is kept.
func (s *Server) compensateRetiredCreation(ctx context.Context, store kv.Store, intent creationIntent, workspace apitypes.Workspace) error {
	binding, err := readWorkspaceBinding(ctx, store, intent.RelationID)
	if err == nil && (binding.WorkspaceName == intent.Workspace || binding.WorkspaceID == workspace.Id) {
		return errFriendChanged
	}
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return err
	}
	if _, err := s.Workspaces.RetireSystemWorkspaceByID(ctx, workspace.Id, socialutil.SFUWorkspaceKindFriend, intent.RelationID); err != nil {
		return err
	}
	return deleteCreationIntent(ctx, store, intent)
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
		if err := s.completeFriendCreationCancellation(ctx, store, intent.RelationID, intent.WorkspaceOwner, intent.Workspace); err != nil {
			return "", err
		}
		return decision.State, deleteCreationIntent(ctx, store, intent)
	}
	// A committed decision is immutable even after that incarnation retires.
	// Removing only this exact creation intent preserves any newer lifecycle.
	return decision.State, deleteCreationIntent(ctx, store, intent)
}

// completeFriendCreationCancellation consumes the shared cancellation handoff
// only after this Server successfully deletes that exact local Workspace. A
// missing foreign SQL row cannot establish cleanup of another Server's copy.
func (s *Server) completeFriendCreationCancellation(ctx context.Context, store kv.Store, relationID, workspaceOwner, workspaceName string) error {
	if workspaceOwner == "" || workspaceOwner != strings.TrimSpace(workspaceOwner) || relationPeer(workspaceOwner, relationID) == "" {
		return errors.New("social: cancelled Friend Workspace owner is missing or inconsistent")
	}
	_, deleteErr := s.Workspaces.DeleteSystemWorkspace(ownership.WithOwner(ctx, workspaceOwner), workspaceName)
	if deleteErr == sql.ErrNoRows {
		// The raw SQL sentinel is the local lookup/relookup's absence result.
		// Nested runtime errors stay wrapped and are never treated as absence.
		// A completed exact cancellation receipt makes this a repeated local
		// absence, rather than evidence that the first foreign cleanup succeeded.
		completed, err := completedFriendCancellation(ctx, store, relationID, workspaceOwner, workspaceName)
		if err != nil {
			return err
		}
		if !completed {
			return deleteErr
		}
		return nil
	}
	if deleteErr != nil && !errors.Is(deleteErr, kv.ErrNotFound) {
		return deleteErr
	}
	intent, err := readRetirementIntent(ctx, store, relationID)
	if errors.Is(err, kv.ErrNotFound) || err == nil && (!intent.CancelCreation || intent.WorkspaceOwner != workspaceOwner || intent.WorkspaceName != workspaceName) {
		// The successful local delete is known. Refresh an unavailable or
		// different postwrite handoff before deciding which receipt may commit.
		ctx = kv.WithPrimaryReads(ctx)
		intent, err = readRetirementIntent(ctx, store, relationID)
	}
	if errors.Is(err, kv.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	intent, err = validateRetirementIntent(intent, relationID)
	if err != nil {
		return err
	}
	if !intent.CancelCreation || intent.WorkspaceOwner != workspaceOwner || intent.WorkspaceName != workspaceName {
		return nil
	}
	return s.completeFriendRetirementReceipt(ctx, store, intent)
}

func completedFriendCancellation(ctx context.Context, store kv.Store, relationID, workspaceOwner, workspaceName string) (bool, error) {
	key := retirementReceiptKey(relationID)
	for range 2 {
		data, err := store.Get(ctx, key)
		if errors.Is(err, kv.ErrNotFound) {
			data = nil
		} else if err != nil {
			return false, err
		}
		matched, err := store.ApplyMutation(ctx, kv.Mutation{Conditions: []kv.Condition{{Key: key, Expected: data}}})
		if err != nil {
			return false, err
		}
		if !matched {
			ctx = kv.WithPrimaryReads(ctx)
			continue
		}
		if data == nil {
			return false, nil
		}
		var receipt retirementReceipt
		if err := json.Unmarshal(data, &receipt); err != nil {
			return false, err
		}
		if err := validateRetirementReceipt(receipt, relationID); err != nil {
			return false, err
		}
		return receipt.CancelCreation && receipt.WorkspaceOwner == workspaceOwner && receipt.WorkspaceName == workspaceName, nil
	}
	return false, errFriendChanged
}

func (s *Server) completeFriendRetirement(ctx context.Context, store kv.Store, intent retirementIntent) error {
	if s == nil || s.Workspaces == nil {
		return errors.New("social: Workspace retirement service not configured")
	}
	if _, err := validateRetirementIntent(intent, intent.RelationID); err != nil {
		return err
	}
	if intent.CancelCreation {
		return s.completeFriendCreationCancellation(ctx, store, intent.RelationID, intent.WorkspaceOwner, intent.WorkspaceName)
	}
	if _, err := s.Workspaces.RetireSystemWorkspaceByID(
		ctx,
		intent.WorkspaceID,
		socialutil.SFUWorkspaceKindFriend,
		intent.RelationID,
	); err != nil {
		return err
	}
	return s.completeFriendRetirementReceipt(ctx, store, intent)
}

func (s *Server) completeFriendRetirementReceipt(ctx context.Context, store kv.Store, intent retirementIntent) error {
	receipt := retirementReceipt{
		RelationID:     intent.RelationID,
		FirstPeer:      intent.FirstPeer,
		SecondPeer:     intent.SecondPeer,
		WorkspaceID:    intent.WorkspaceID,
		WorkspaceName:  intent.WorkspaceName,
		WorkspaceOwner: intent.WorkspaceOwner,
		DeletedAt:      intent.DeletedAt,
		CancelCreation: intent.CancelCreation,
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	for range 2 {
		applied, err := commitFriendRetirementReceipt(ctx, store, intent, data)
		if errors.Is(err, errFriendChanged) {
			ctx = kv.WithPrimaryReads(ctx)
			continue
		}
		if err != nil {
			return err
		}
		if applied && !intent.CancelCreation {
			s.notifyFriendRetirement(ctx, intent)
		}
		return nil
	}
	return errFriendChanged
}

func commitFriendRetirementReceipt(ctx context.Context, store kv.Store, intent retirementIntent, data []byte) (bool, error) {
	intentKey := retirementIntentKey(intent.RelationID)
	stored, err := store.Get(ctx, intentKey)
	if errors.Is(err, kv.ErrNotFound) {
		ctx = kv.WithPrimaryReads(ctx)
		stored, err = store.Get(ctx, intentKey)
	}
	if errors.Is(err, kv.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var current retirementIntent
	if err := json.Unmarshal(stored, &current); err != nil {
		return false, err
	}
	current, err = validateRetirementIntent(current, intent.RelationID)
	if err != nil {
		return false, err
	}
	if current.WorkspaceID != intent.WorkspaceID ||
		current.WorkspaceName != intent.WorkspaceName ||
		current.WorkspaceOwner != intent.WorkspaceOwner ||
		!current.DeletedAt.Equal(intent.DeletedAt) ||
		current.CancelCreation != intent.CancelCreation {
		return false, nil
	}
	applied, err := store.ApplyMutation(ctx, kv.Mutation{
		Conditions:    []kv.Condition{{Key: intentKey, Expected: stored}},
		Entries:       []kv.Entry{{Key: retirementReceiptKey(intent.RelationID), Value: data}},
		DeleteKeys:    []kv.Key{intentKey},
		RemoveMembers: (socialutil.RecoveryIndex{Root: retirementIntentsRoot}).Remove(intent.RelationID),
	})
	if err != nil {
		return false, err
	}
	if !applied {
		return false, errFriendChanged
	}
	return true, nil
}

// ReconcileCreationIntents completes Workspace-first relationship creations
// that stopped before the reciprocal Friend rows committed. Individual
// failures remain indexed for retry while the remaining intents are attempted.
func (s *Server) ReconcileCreationIntents(ctx context.Context) error {
	store, err := s.friendsStore()
	if err != nil {
		return err
	}
	return (socialutil.RecoveryIndex{Root: creationIntentsRoot}).Reconcile(ctx, store, func(ctx context.Context, relationID string) error {
		listed, err := socialutil.ReadJSONValue[creationIntent](ctx, store, creationIntentKey(relationID))
		if errors.Is(err, kv.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if listed.RelationID != relationID {
			return errors.New("social: recovery intent identity mismatch")
		}
		unlock, lockErr := s.lockRelationMutation(ctx, relationID, listed.FirstPeer, listed.SecondPeer)
		if lockErr != nil {
			return lockErr
		}
		defer unlock()
		current, readErr := readCreationIntent(ctx, store, relationID)
		if errors.Is(readErr, kv.ErrNotFound) {
			return nil
		}
		if readErr != nil {
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
					if errors.Is(err, errFriendCreationCancelled) {
						return nil
					}
					if err == nil {
						_, err = s.commitFriendCreation(
							ctx,
							store,
							current.FirstPeer,
							current.SecondPeer,
							current,
							workspace,
						)
						// A limit rejection already cancelled the creation; a
						// capacity race leaves it for the next pass.
						if errors.Is(err, errFriendCreationCancelled) || errors.Is(err, ErrPeerFriendLimit) ||
							errors.Is(err, errFriendCapacityChanged) {
							err = nil
						}
					}
				}
			}
		}
		return err
	})
}

// ReconcileRetirementIntents completes relationship-first deletions that
// committed before the process could persist their Workspace PendingDeletion.
// Individual failures do not prevent recovery of other indexed deletions.
func (s *Server) ReconcileRetirementIntents(ctx context.Context) error {
	store, err := s.friendsStore()
	if err != nil {
		return err
	}
	return (socialutil.RecoveryIndex{Root: retirementIntentsRoot}).Reconcile(ctx, store, func(ctx context.Context, relationID string) error {
		intent, err := socialutil.ReadJSONValue[retirementIntent](ctx, store, retirementIntentKey(relationID))
		if errors.Is(err, kv.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if intent.RelationID != relationID {
			return errors.New("social: recovery intent identity mismatch")
		}
		unlock, lockErr := s.lockRelationMutation(ctx, relationID, intent.FirstPeer, intent.SecondPeer)
		if lockErr != nil {
			return lockErr
		}
		defer unlock()
		current, readErr := readRetirementIntent(ctx, store, relationID)
		if errors.Is(readErr, kv.ErrNotFound) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
		current, err = validateRetirementIntent(current, relationID)
		if err != nil {
			return err
		}
		return s.completeFriendRetirement(ctx, store, current)
	})
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
	if err := validateWorkspaceBinding(binding, relationID); err != nil {
		return workspaceBinding{}, err
	}
	return binding, nil
}

func validateWorkspaceBinding(binding workspaceBinding, relationID string) error {
	if binding.RelationID != relationID ||
		binding.WorkspaceID == "" || binding.WorkspaceID != strings.TrimSpace(binding.WorkspaceID) ||
		binding.WorkspaceName == "" || binding.WorkspaceName != strings.TrimSpace(binding.WorkspaceName) ||
		binding.WorkspaceOwner == "" || binding.WorkspaceOwner != strings.TrimSpace(binding.WorkspaceOwner) ||
		binding.SFU.Generation == 0 {
		return fmt.Errorf("social: invalid Friend Workspace binding %q", relationID)
	}
	if err := binding.SFU.Validate(); err != nil {
		return fmt.Errorf("social: invalid Friend Workspace binding %q: %w", relationID, err)
	}
	return nil
}

func readCreationIntent(ctx context.Context, store kv.Store, relationID string) (creationIntent, error) {
	return socialutil.ReadJSONValue[creationIntent](ctx, store, creationIntentKey(relationID))
}

func deleteCreationIntent(
	ctx context.Context,
	store kv.Store,
	intent creationIntent,
) error {
	for range 2 {
		matched, err := deleteCreationIntentSnapshot(ctx, store, intent)
		if err != nil || matched {
			return err
		}
		ctx = kv.WithPrimaryReads(ctx)
	}
	return errFriendChanged
}

func deleteCreationIntentSnapshot(ctx context.Context, store kv.Store, intent creationIntent) (bool, error) {
	key := creationIntentKey(intent.RelationID)
	data, err := store.Get(ctx, key)
	if errors.Is(err, kv.ErrNotFound) {
		data, err = store.Get(kv.WithPrimaryReads(ctx), key)
	}
	if errors.Is(err, kv.ErrNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	var current creationIntent
	if err := json.Unmarshal(data, &current); err != nil {
		return false, err
	}
	current, err = validateCreationIntent(
		current,
		intent.RelationID,
		current.WorkspaceOwner,
	)
	if err != nil {
		return false, err
	}
	if current.IncarnationID != intent.IncarnationID ||
		current.Workspace != intent.Workspace {
		return true, nil
	}
	matched, err := store.ApplyMutation(ctx, kv.Mutation{
		Conditions: []kv.Condition{{Key: key, Expected: data}}, DeleteKeys: []kv.Key{key},
		RemoveMembers: (socialutil.RecoveryIndex{Root: creationIntentsRoot}).Remove(intent.RelationID),
	})
	return matched, err
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
		(intent.CancelCreation && (intent.WorkspaceOwner != strings.TrimSpace(intent.WorkspaceOwner) ||
			(intent.WorkspaceOwner != intent.FirstPeer && intent.WorkspaceOwner != intent.SecondPeer))) ||
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
		(receipt.CancelCreation && (receipt.WorkspaceOwner != strings.TrimSpace(receipt.WorkspaceOwner) ||
			(receipt.WorkspaceOwner != receipt.FirstPeer && receipt.WorkspaceOwner != receipt.SecondPeer))) ||
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

	data, err := socialutil.ReadInviteToken(ctx, store, socialutil.FriendInviteTokensRoot, inviteToken)
	if errors.Is(err, kv.ErrNotFound) {
		return inviteTokenRecord{}, ErrInviteTokenUnavailable
	}
	if err != nil {
		return inviteTokenRecord{}, inviteTokenLookupError(err)
	}
	var record inviteTokenRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return inviteTokenRecord{}, inviteTokenLookupError(err)
	}
	if !record.ExpiresAt.After(s.now()) {
		return inviteTokenRecord{}, ErrInviteTokenUnavailable
	}
	if record.CreatedAt.IsZero() || !record.CreatedAt.Before(record.ExpiresAt) || record.PeerPublicKey == "" || record.PeerPublicKey != strings.TrimSpace(record.PeerPublicKey) {
		return inviteTokenRecord{}, inviteTokenLookupError(errors.New("social: persisted Friend invite token is invalid"))
	}
	return record, nil
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

func friendPageKey(owner string) kv.Key {
	return kv.Key{"friend-pages", socialutil.EscapeStoreSegment(strings.TrimSpace(owner))}
}

func friendPageMembership(owner, id string) kv.SetMembers {
	return kv.SetMembers{Key: friendPageKey(owner), Members: []string{socialutil.EscapeStoreSegment(id)}}
}
