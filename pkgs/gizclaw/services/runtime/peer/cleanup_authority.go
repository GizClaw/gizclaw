package peer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerroute"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// DeletionAssignments resolves fixed home authority without projecting SQL data.
type DeletionAssignments interface {
	Lookup(context.Context, giznet.PublicKey) (apitypes.PeerAssignment, error)
}

// CheckTask performs read-only admission for both worker claims and Admin retry.
// A foreign home never takes a lease or accesses this Server's SQL database.
func (h DeletionHandler) CheckTask(ctx context.Context, task pendingdeletion.Task) error {
	ctx = kv.WithPrimaryReads(ctx)
	key, err := validatePeerDeletionClaim(pendingdeletion.Claim{Task: task})
	if err != nil {
		return pendingdeletion.Terminal("invalid_peer_marker", "Peer deletion marker is invalid", err)
	}
	if task.Phase != pendingdeletion.PhaseValidate && task.Phase != pendingdeletion.PhaseFinalize {
		return pendingdeletion.Terminal("invalid_phase", "Peer deletion phase is invalid", nil)
	}
	if err := h.checkHome(ctx, key); err != nil {
		return err
	}
	current, raw, err := h.retainedPeer(ctx, key)
	if err != nil {
		return err
	}
	data, err := h.Server.Store.Get(ctx, peerRetirementPlanKey(task.Record.DeletionID))
	if errors.Is(err, kv.ErrNotFound) {
		return nil
	}
	if err != nil {
		return pendingdeletion.Retryable("store_error", "Peer retirement plan could not be read", err)
	}
	plan, err := validateRetirementPlan(data, pendingdeletion.Claim{Task: task}, key)
	if err != nil {
		return err
	}
	return h.checkPlan(plan, current, raw)
}

func (h DeletionHandler) checkHome(ctx context.Context, key giznet.PublicKey) error {
	if h.Server == nil || h.Server.ServerPublicKey.IsZero() || h.Assignments == nil {
		return pendingdeletion.Terminal("peer_home_unknown", "Peer deletion requires a fixed home Server assignment", nil)
	}
	assignment, err := h.Assignments.Lookup(ctx, key)
	if errors.Is(err, peerroute.ErrAssignmentNotFound) {
		return pendingdeletion.Terminal("peer_home_unknown", "Peer deletion requires a fixed home Server assignment", nil)
	}
	if err != nil {
		return pendingdeletion.Retryable("peer_home_unavailable", "Peer home assignment could not be read", err)
	}
	var home giznet.PublicKey
	if home.UnmarshalText([]byte(assignment.ServerPublicKey)) != nil || home.IsZero() || home.String() != assignment.ServerPublicKey ||
		assignment.PeerPublicKey != key.String() || assignment.Role != apitypes.PeerRoleClient || assignment.Version < 1 || assignment.UpdatedAt.IsZero() {
		return pendingdeletion.Terminal("peer_home_invalid", "Peer home assignment is invalid", nil)
	}
	if home != h.Server.ServerPublicKey {
		return pendingdeletion.ErrNotLocal
	}
	return nil
}

func (h DeletionHandler) retainedPeer(ctx context.Context, key giznet.PublicKey) (apitypes.Peer, []byte, error) {
	if h.Server == nil || h.Server.Store == nil {
		return apitypes.Peer{}, nil, pendingdeletion.Retryable("store_unavailable", "Peer store is unavailable", nil)
	}
	raw, err := h.Server.Store.Get(ctx, peerKey(key.String()))
	if errors.Is(err, kv.ErrNotFound) || (err == nil && isPeerTombstone(raw)) {
		return apitypes.Peer{}, nil, pendingdeletion.Terminal("peer_record_missing", "Retained Peer record is unavailable; keep the deletion fence for investigation", nil)
	}
	if err != nil {
		return apitypes.Peer{}, nil, pendingdeletion.Retryable("store_error", "Retained Peer could not be read", err)
	}
	current, err := decodePeer(raw)
	if err != nil || current.PublicKey != key.String() || current.CreatedAt.IsZero() || current.UpdatedAt.IsZero() {
		return apitypes.Peer{}, nil, pendingdeletion.Terminal("peer_record_invalid", "Retained Peer record is invalid", err)
	}
	return current, raw, nil
}

func (h DeletionHandler) checkPlan(plan retirementPlan, current apitypes.Peer, raw []byte) error {
	if plan.Version == retirementPlanVersion {
		if plan.HomeServer != h.Server.ServerPublicKey.String() {
			return pendingdeletion.Terminal("retirement_scope_ambiguous", "Retirement plan does not belong to this home Server", nil)
		}
		if plan.PeerFingerprint != peerFingerprint(raw) || !reflect.DeepEqual(plan.Peer, current) {
			return pendingdeletion.Terminal("peer_replacement_ambiguous", "Retained Peer no longer matches its retirement plan", nil)
		}
		return checkSocialRetirementScope(plan)
	}
	// Version 2 saved a SQL display projection. Accept only its precise known
	// discrepancy: a projected firmware on a KV record with no firmware field.
	// Any persisted firmware change, timestamp or other identity change rejects.
	expected := plan.Peer
	if current.FirmwareId == nil {
		expected.FirmwareId = nil
	}
	if !reflect.DeepEqual(current, expected) {
		return pendingdeletion.Terminal("peer_replacement_ambiguous", "Retained Peer no longer matches its retirement plan", nil)
	}
	// Old plans did not identify the SQL authority that produced their child
	// snapshot. Never treat absent local children as proof of remote completion.
	if len(plan.Social.Contacts)+len(plan.Social.Friends)+len(plan.Social.Groups)+len(plan.Workspaces.Workspaces)+len(plan.WorkspaceIDs)+len(plan.FriendGroupIDs) != 0 {
		return pendingdeletion.Terminal("retirement_scope_ambiguous", "Legacy retirement plan has regional dependencies without home provenance", nil)
	}
	return nil
}

func checkSocialRetirementScope(plan retirementPlan) error {
	// A shared relationship can have SQL Workspace replicas on more than one
	// Server, including former members' Servers. There is no replica-completion
	// ledger. Keep the Peer fence rather than infer completion from local SQL.
	hasWorkspace := len(plan.Social.Friends) != 0
	for _, group := range plan.Social.Groups {
		hasWorkspace = hasWorkspace || group.OwnerPublicKey == plan.Peer.PublicKey
	}
	if hasWorkspace {
		return pendingdeletion.Terminal("social_replica_scope_unknown", "Social Workspace replicas require regional completion evidence before Peer retirement", nil)
	}
	return nil
}

func peerFingerprint(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// CanRecoverFailed authorizes one automatic recovery only for a proven legacy
// Firmware display-projection failure. A version-3 plan cannot reset its retry
// budget through this policy; uncertain identity or regional data is retained.
func (h DeletionHandler) CanRecoverFailed(ctx context.Context, task pendingdeletion.Task) (bool, error) {
	if task.Status != pendingdeletion.StatusFailed || task.Phase != pendingdeletion.PhaseFinalize || task.LastErrorCode != "peer_replacement_ambiguous" {
		return false, nil
	}
	ctx = kv.WithPrimaryReads(ctx)
	key, err := validatePeerDeletionClaim(pendingdeletion.Claim{Task: task})
	if err != nil {
		return false, err
	}
	if err := h.checkHome(ctx, key); err != nil {
		return false, err
	}
	current, raw, err := h.retainedPeer(ctx, key)
	if err != nil {
		return false, err
	}
	if current.FirmwareId != nil {
		return false, nil
	}
	data, err := h.Server.Store.Get(ctx, peerRetirementPlanKey(task.Record.DeletionID))
	if errors.Is(err, kv.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	plan, err := validateRetirementPlan(data, pendingdeletion.Claim{Task: task}, key)
	if err != nil {
		return false, err
	}
	if plan.Version != 2 || plan.Peer.FirmwareId == nil || *plan.Peer.FirmwareId == "" {
		return false, nil
	}
	if err := h.checkPlan(plan, current, raw); err != nil {
		return false, err
	}
	if _, _, err := h.emptyLegacyHome(ctx, key); err != nil {
		return false, err
	}
	return true, nil
}

var _ pendingdeletion.FailedRecovery = DeletionHandler{}
