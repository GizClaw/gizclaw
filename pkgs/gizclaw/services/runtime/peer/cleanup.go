package peer

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

const peerRetirementPollInterval = 5 * time.Second

type SocialRetirement interface {
	SnapshotPeerSocial(context.Context, string) (social.PeerSnapshot, error)
	RetirePeerSocial(context.Context, social.PeerSnapshot) (social.PeerRetirementResult, error)
}

type WorkspaceRetirement interface {
	SnapshotPeerWorkspaces(context.Context, string) (workspace.PeerRetirementSnapshot, error)
	RetirePeerWorkspaces(context.Context, workspace.PeerRetirementSnapshot) ([]string, error)
}

type PeerAPIKeyCleanup interface {
	CleanupPeer(context.Context, string) error
}

type OwnerBindingCleanup interface {
	DeleteOwnerProfileBinding(context.Context, string) error
}

// PeerMemoryCleanup removes shared memories after child Workspaces have retired.
type PeerMemoryCleanup interface {
	PurgePeerMemory(context.Context, string) error
	PeerMemoryAbsent(context.Context, string) (bool, error)
}

type PeerQuiescer interface {
	QuiescePeer(context.Context, giznet.PublicKey) error
}

// Version 3 binds an unprojected KV generation to its regional SQL authority.
const retirementPlanVersion = 3

type retirementPlan struct {
	HomeServer        string                           `json:"home_server,omitempty"`
	PeerFingerprint   string                           `json:"peer_fingerprint,omitempty"`
	Version           int                              `json:"version"`
	MarkerFingerprint string                           `json:"marker_fingerprint"`
	Peer              apitypes.Peer                    `json:"peer"`
	Social            social.PeerSnapshot              `json:"social"`
	Workspaces        workspace.PeerRetirementSnapshot `json:"workspaces"`
	WorkspaceIDs      []string                         `json:"workspace_ids"`
	FriendGroupIDs    []string                         `json:"friend_group_ids"`
}

type DeletionHandler struct {
	Server            *Server
	Assignments       DeletionAssignments
	Source            pendingdeletion.KVSource
	Social            SocialRetirement
	Workspaces        WorkspaceRetirement
	APIKeys           PeerAPIKeyCleanup
	RuntimeProfiles   OwnerBindingCleanup
	Memory            PeerMemoryCleanup
	Quiescer          PeerQuiescer
	WorkspaceLookup   pendingdeletion.LookupSource
	FriendGroupLookup pendingdeletion.LookupSource
	Now               func() time.Time
}

func (h DeletionHandler) Kind() pendingdeletion.Kind { return pendingdeletion.KindPeer }

func (h DeletionHandler) Handle(ctx context.Context, claim pendingdeletion.Claim) error {
	ctx = kv.WithPrimaryReads(ctx)
	if err := h.CheckTask(ctx, claim.Task); err != nil {
		return err
	}
	publicKey, err := validatePeerDeletionClaim(claim)
	if err != nil {
		return pendingdeletion.Terminal("invalid_peer_marker", "Peer deletion marker is invalid", err)
	}
	if h.Server == nil || h.Social == nil || h.Workspaces == nil || h.APIKeys == nil || h.RuntimeProfiles == nil || h.Memory == nil || h.Quiescer == nil || h.WorkspaceLookup == nil || h.FriendGroupLookup == nil {
		return pendingdeletion.Retryable("service_unavailable", "Peer retirement adapter is unavailable", nil)
	}
	plan, err := h.loadOrCreatePlan(ctx, claim, publicKey)
	if err != nil {
		return err
	}
	current, raw, err := h.retainedPeer(ctx, publicKey)
	if err != nil {
		return err
	}
	if err := h.checkPlan(plan, current, raw); err != nil {
		return err
	}
	claim, err = h.Source.Checkpoint(ctx, claim, claim.Phase, h.currentTime())
	if err != nil {
		return err
	}
	if err := h.Quiescer.QuiescePeer(ctx, publicKey); err != nil {
		return pendingdeletion.Retryable("quiesce_failed", "Peer connections could not be quiesced", err)
	}
	if _, err := h.Social.RetirePeerSocial(ctx, plan.Social); err != nil {
		return pendingdeletion.Retryable("social_cleanup_failed", "Peer Social cleanup failed", err)
	}
	if _, err := h.Workspaces.RetirePeerWorkspaces(ctx, plan.Workspaces); err != nil {
		return pendingdeletion.Retryable("workspace_cleanup_failed", "Peer Workspace handoff failed", err)
	}
	if err := h.APIKeys.CleanupPeer(ctx, publicKey.String()); err != nil {
		return pendingdeletion.Retryable("api_key_cleanup_failed", "Peer API key cleanup failed", err)
	}
	// Workspace cleanup resolves the Memory binding it purges through the
	// owner's RuntimeProfile, so the binding outlives every child Workspace.
	if pending, err := childDeletionPending(ctx, h.WorkspaceLookup, pendingdeletion.KindWorkspace, plan.WorkspaceIDs); err != nil {
		return pendingdeletion.Retryable("workspace_verify_failed", "Peer Workspace cleanup could not be verified", err)
	} else if pending {
		return pendingdeletion.Deferred("workspace_cleanup_pending", "Peer Workspace cleanup is still completing", peerRetirementPollInterval)
	}
	if err := h.Memory.PurgePeerMemory(ctx, publicKey.String()); err != nil {
		return pendingdeletion.Retryable("memory_cleanup_failed", "Peer shared memory could not be purged", err)
	}
	absent, err := h.Memory.PeerMemoryAbsent(ctx, publicKey.String())
	if err != nil {
		return pendingdeletion.Retryable("memory_verify_failed", "Peer shared memory cleanup could not be verified", err)
	}
	if !absent {
		return pendingdeletion.Deferred("memory_cleanup_pending", "Peer shared memory cleanup is still completing", peerRetirementPollInterval)
	}
	if err := h.RuntimeProfiles.DeleteOwnerProfileBinding(ctx, publicKey.String()); err != nil {
		return pendingdeletion.Retryable("binding_cleanup_failed", "Peer RuntimeProfile binding cleanup failed", err)
	}
	if pending, err := childDeletionPending(ctx, h.FriendGroupLookup, pendingdeletion.KindFriendGroup, plan.FriendGroupIDs); err != nil {
		return pendingdeletion.Retryable("friend_group_verify_failed", "Peer Friend Group cleanup could not be verified", err)
	} else if pending {
		return pendingdeletion.Deferred("friend_group_cleanup_pending", "Peer Friend Group cleanup is still completing", peerRetirementPollInterval)
	}
	if claim.Phase == pendingdeletion.PhaseValidate {
		claim, err = h.Source.Checkpoint(ctx, claim, pendingdeletion.PhaseFinalize, h.currentTime())
		if err != nil {
			return err
		}
	}
	if claim.Phase != pendingdeletion.PhaseFinalize {
		return pendingdeletion.Terminal("invalid_phase", "Peer deletion phase is invalid", nil)
	}
	return h.finalize(ctx, claim, plan, publicKey)
}

func validatePeerDeletionClaim(claim pendingdeletion.Claim) (giznet.PublicKey, error) {
	if err := pendingdeletion.ValidateTask(claim.Task); err != nil {
		return giznet.PublicKey{}, err
	}
	if claim.Source != PendingDeletionSourceName || claim.Record.Kind != pendingdeletion.KindPeer ||
		(claim.Record.Reason != pendingdeletion.ReasonAdminDelete && claim.Record.Reason != pendingdeletion.ReasonPeerDelete) ||
		claim.Record.OwnerPublicKey == nil || *claim.Record.OwnerPublicKey != claim.Record.ResourceID {
		return giznet.PublicKey{}, errors.New("peer: deletion claim identity mismatch")
	}
	var descriptor struct {
		PublicKey string `json:"public_key"`
	}
	if err := json.Unmarshal(claim.Record.Descriptor, &descriptor); err != nil || descriptor.PublicKey != claim.Record.ResourceID {
		return giznet.PublicKey{}, errors.New("peer: deletion descriptor identity mismatch")
	}
	var publicKey giznet.PublicKey
	if err := publicKey.UnmarshalText([]byte(descriptor.PublicKey)); err != nil || publicKey.IsZero() || publicKey.String() != descriptor.PublicKey {
		return giznet.PublicKey{}, errors.New("peer: deletion public key is not canonical")
	}
	fingerprint, err := pendingdeletion.Fingerprint(claim.Record)
	if err != nil || fingerprint != claim.MarkerFingerprint {
		return giznet.PublicKey{}, errors.New("peer: deletion marker fingerprint mismatch")
	}
	return publicKey, nil
}

func (h DeletionHandler) loadOrCreatePlan(ctx context.Context, claim pendingdeletion.Claim, publicKey giznet.PublicKey) (retirementPlan, error) {
	store, err := h.Server.store()
	if err != nil {
		return retirementPlan{}, pendingdeletion.Retryable("store_unavailable", "Peer store is unavailable", err)
	}
	key := peerRetirementPlanKey(claim.Record.DeletionID)
	if data, err := store.Get(ctx, key); err == nil {
		plan, err := validateRetirementPlan(data, claim, publicKey)
		if err != nil {
			return retirementPlan{}, err
		}
		current, raw, err := h.retainedPeer(ctx, publicKey)
		if err != nil {
			return retirementPlan{}, err
		}
		if err := h.checkPlan(plan, current, raw); err != nil {
			return retirementPlan{}, err
		}
		if plan.Version == retirementPlanVersion {
			return plan, nil
		}
		return h.adoptLegacyPlan(ctx, claim, publicKey, data, current, raw)
	} else if !errors.Is(err, kv.ErrNotFound) {
		return retirementPlan{}, pendingdeletion.Retryable("store_error", "Peer retirement plan could not be read", err)
	}
	peerRecord, raw, err := h.retainedPeer(ctx, publicKey)
	if err != nil {
		return retirementPlan{}, err
	}
	socialSnapshot, err := h.Social.SnapshotPeerSocial(ctx, publicKey.String())
	if err != nil {
		return retirementPlan{}, pendingdeletion.Retryable("social_snapshot_failed", "Peer Social snapshot failed", err)
	}
	workspaceSnapshot, err := h.Workspaces.SnapshotPeerWorkspaces(ctx, publicKey.String())
	if err != nil {
		return retirementPlan{}, pendingdeletion.Retryable("workspace_snapshot_failed", "Peer Workspace snapshot failed", err)
	}
	plan := retirementPlan{HomeServer: h.Server.ServerPublicKey.String(), PeerFingerprint: peerFingerprint(raw), Version: retirementPlanVersion, MarkerFingerprint: claim.MarkerFingerprint, Peer: peerRecord, Social: socialSnapshot, Workspaces: workspaceSnapshot}
	for _, item := range socialSnapshot.Friends {
		plan.WorkspaceIDs = append(plan.WorkspaceIDs, item.WorkspaceID)
	}
	for _, item := range socialSnapshot.Groups {
		if item.OwnerPublicKey == publicKey.String() {
			plan.WorkspaceIDs = append(plan.WorkspaceIDs, item.WorkspaceID)
			plan.FriendGroupIDs = append(plan.FriendGroupIDs, item.FriendGroupID)
		}
	}
	for _, item := range workspaceSnapshot.Workspaces {
		plan.WorkspaceIDs = append(plan.WorkspaceIDs, item.ID)
	}
	plan.WorkspaceIDs = sortedUnique(plan.WorkspaceIDs)
	plan.FriendGroupIDs = sortedUnique(plan.FriendGroupIDs)
	data, err := json.Marshal(plan)
	if err != nil {
		return retirementPlan{}, pendingdeletion.Terminal("plan_encode_failed", "Peer retirement plan could not be encoded", err)
	}
	existing, created, err := kv.CreateIfAbsent(ctx, store, kv.Entry{Key: key, Value: data}, nil)
	if err != nil {
		return retirementPlan{}, pendingdeletion.Retryable("plan_store_failed", "Peer retirement plan could not be stored", err)
	}
	if created {
		return plan, nil
	}
	return validateRetirementPlan(existing, claim, publicKey)
}

func validateRetirementPlan(data []byte, claim pendingdeletion.Claim, publicKey giznet.PublicKey) (retirementPlan, error) {
	var plan retirementPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return retirementPlan{}, pendingdeletion.Terminal("retirement_plan_invalid", "Peer retirement plan is invalid", err)
	}
	if plan.Version != retirementPlanVersion && plan.Version != 2 {
		return retirementPlan{}, pendingdeletion.Terminal(
			"retirement_plan_unsupported",
			"Peer retirement plan version is unsupported; retain it for evidence-backed repair",
			nil,
		)
	}
	if plan.MarkerFingerprint != claim.MarkerFingerprint || plan.Peer.PublicKey != publicKey.String() ||
		plan.Social.PublicKey != publicKey.String() || plan.Workspaces.PublicKey != publicKey.String() {
		return retirementPlan{}, pendingdeletion.Terminal("retirement_plan_invalid", "Peer retirement plan is invalid", nil)
	}
	return plan, nil
}

func childDeletionPending(ctx context.Context, source pendingdeletion.LookupSource, kind pendingdeletion.Kind, ids []string) (bool, error) {
	for _, id := range ids {
		pending, err := source.HasLocator(ctx, pendingdeletion.Locator{Kind: kind, ResourceID: id})
		if err != nil {
			return false, err
		}
		if pending {
			return true, nil
		}
	}
	return false, nil
}

func (h DeletionHandler) finalize(ctx context.Context, claim pendingdeletion.Claim, plan retirementPlan, publicKey giznet.PublicKey) error {
	unlock := h.Server.IconLocks.LockRecord(publicKey.String())
	defer unlock()
	if err := h.checkHome(ctx, publicKey); err != nil {
		return err
	}
	current, raw, err := h.retainedPeer(ctx, publicKey)
	if err != nil {
		return err
	}
	if err := h.checkPlan(plan, current, raw); err != nil {
		return err
	}
	deletes := append(indexKeys(plan.Peer), peerRetirementPlanKey(claim.Record.DeletionID))
	if err := h.Source.FinalizeGuarded(ctx, claim, h.currentTime(), []kv.Condition{{Key: peerKey(publicKey.String()), Expected: raw}}, []kv.Entry{{Key: peerKey(publicKey.String()), Value: encodedPeerTombstone}}, deletes, identifierSets(plan.Peer)...); err != nil {
		if errors.Is(err, pendingdeletion.ErrConflict) {
			return err
		}
		return pendingdeletion.Retryable("store_error", "Peer tombstone finalization failed", err)
	}
	return nil
}

func peerRetirementPlanKey(deletionID string) kv.Key {
	return kv.Key{"peer-retirement-plans", deletionID}
}

func sortedUnique(values []string) []string {
	sort.Strings(values)
	out := values[:0]
	for _, value := range values {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

var _ pendingdeletion.Handler = DeletionHandler{}

func (h DeletionHandler) currentTime() time.Time {
	if h.Now != nil {
		return h.Now().UTC()
	}
	return time.Now().UTC()
}

// adoptLegacyPlan only adopts a dependency-free legacy plan after inspecting
// the real home SQL and Social stores. It cannot erase a broader old snapshot.
func (h DeletionHandler) adoptLegacyPlan(ctx context.Context, claim pendingdeletion.Claim, key giznet.PublicKey, old []byte, current apitypes.Peer, raw []byte) (retirementPlan, error) {
	snapshot, err := h.Workspaces.SnapshotPeerWorkspaces(ctx, key.String())
	if err != nil {
		return retirementPlan{}, pendingdeletion.Retryable("workspace_snapshot_failed", "Home Workspace snapshot failed", err)
	}
	socialSnapshot, err := h.Social.SnapshotPeerSocial(ctx, key.String())
	if err != nil {
		return retirementPlan{}, pendingdeletion.Retryable("social_snapshot_failed", "Home Social snapshot failed", err)
	}
	if len(snapshot.Workspaces)+len(socialSnapshot.Contacts)+len(socialSnapshot.Friends)+len(socialSnapshot.Groups) != 0 {
		return retirementPlan{}, pendingdeletion.Terminal("retirement_scope_ambiguous", "Home data is absent from the legacy retirement plan; retain the plan for regional repair", nil)
	}
	plan := retirementPlan{Version: retirementPlanVersion, HomeServer: h.Server.ServerPublicKey.String(), PeerFingerprint: peerFingerprint(raw), MarkerFingerprint: claim.MarkerFingerprint, Peer: current, Workspaces: snapshot, Social: socialSnapshot}
	data, err := json.Marshal(plan)
	if err != nil {
		return retirementPlan{}, err
	}
	applied, err := h.Server.Store.ApplyMutation(ctx, kv.Mutation{
		Conditions: []kv.Condition{{Key: peerRetirementPlanKey(claim.Record.DeletionID), Expected: old}, {Key: peerKey(key.String()), Expected: raw}},
		Entries:    []kv.Entry{{Key: peerRetirementPlanKey(claim.Record.DeletionID), Value: data}},
	})
	if err != nil {
		return retirementPlan{}, pendingdeletion.Retryable("plan_store_failed", "Home retirement plan could not be saved", err)
	}
	if !applied {
		return retirementPlan{}, pendingdeletion.ErrConflict
	}
	return plan, nil
}
