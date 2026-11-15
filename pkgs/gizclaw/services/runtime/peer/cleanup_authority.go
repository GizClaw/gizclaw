package peer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	current, raw, err := h.retainedPeer(ctx, task.Record, key)
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

func (h DeletionHandler) retainedPeer(ctx context.Context, record pendingdeletion.Record, key giznet.PublicKey) (apitypes.Peer, []byte, error) {
	if h.Server == nil || h.Server.Store == nil {
		return apitypes.Peer{}, nil, pendingdeletion.Retryable("store_unavailable", "Peer store is unavailable", nil)
	}
	raw, err := h.Server.Store.Get(ctx, peerKey(key.String()))
	if err != nil {
		raw, err = h.Server.Store.Get(kv.WithPrimaryReads(ctx), peerKey(key.String()))
	}
	if errors.Is(err, kv.ErrNotFound) {
		return apitypes.Peer{}, nil, pendingdeletion.Terminal("peer_record_missing", "Peer deletion state is unavailable; keep the deletion request for investigation", nil)
	}
	if err != nil {
		return apitypes.Peer{}, nil, pendingdeletion.Retryable("store_error", "Peer deletion state could not be read", err)
	}
	var descriptor peerDeletionDescriptor
	if err := json.Unmarshal(record.Descriptor, &descriptor); err != nil || descriptor.PublicKey != key.String() {
		return apitypes.Peer{}, nil, pendingdeletion.Terminal("invalid_peer_marker", "Peer deletion snapshot identity is invalid", err)
	}
	snapshot := descriptor.PeerRecord
	if len(snapshot) == 0 {
		data, snapshotErr := h.Server.Store.Get(ctx, peerDeletionSnapshotKey(record.DeletionID))
		if errors.Is(snapshotErr, kv.ErrNotFound) && isPeerTombstone(raw) {
			// The tombstone proves a prior adoption committed its snapshot.
			data, snapshotErr = h.Server.Store.Get(kv.WithPrimaryReads(ctx), peerDeletionSnapshotKey(record.DeletionID))
		}
		if snapshotErr == nil {
			snapshot, err = validatePeerDeletionSnapshot(data, record)
			if err != nil {
				return apitypes.Peer{}, nil, err
			}
		} else if !errors.Is(snapshotErr, kv.ErrNotFound) {
			return apitypes.Peer{}, nil, pendingdeletion.Retryable("store_error", "Retained Peer snapshot could not be read", snapshotErr)
		}
	}
	if len(snapshot) == 0 {
		// Older requests retain the active record until their first refreshed
		// plan atomically saves its bytes and publishes the tombstone.
		data, planErr := h.Server.Store.Get(ctx, peerRetirementPlanKey(record.DeletionID))
		if planErr != nil && !errors.Is(planErr, kv.ErrNotFound) {
			return apitypes.Peer{}, nil, pendingdeletion.Retryable("store_error", "Retained Peer plan could not be read", planErr)
		}
		if planErr == nil {
			var plan retirementPlan
			fingerprint, fingerprintErr := pendingdeletion.Fingerprint(record)
			if err := json.Unmarshal(data, &plan); err != nil || fingerprintErr != nil || plan.MarkerFingerprint != fingerprint || plan.Peer.PublicKey != key.String() {
				return apitypes.Peer{}, nil, pendingdeletion.Terminal("retirement_plan_invalid", "Retained Peer plan identity is invalid", err)
			}
			snapshot = plan.PeerRecord
		}
	}
	if len(snapshot) != 0 {
		if !isPeerTombstone(raw) {
			// A claimed request proves logical deletion was committed. Recheck
			// its old replica observation before diagnosing a replacement.
			raw, err = h.Server.Store.Get(kv.WithPrimaryReads(ctx), peerKey(key.String()))
			if err != nil {
				return apitypes.Peer{}, nil, pendingdeletion.Retryable("store_error", "Peer tombstone could not be verified", err)
			}
			if !isPeerTombstone(raw) {
				return apitypes.Peer{}, nil, pendingdeletion.Terminal("peer_replacement_ambiguous", "Peer tombstone was replaced; retain its deletion request", nil)
			}
		}
		raw = snapshot
	} else if isPeerTombstone(raw) {
		return apitypes.Peer{}, nil, pendingdeletion.Terminal("peer_record_missing", "Deletion request lacks the retained Peer snapshot", nil)
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
		return nil
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

// Legacy markers are immutable. Their admission snapshot lives beside the plan
// and is removed in the same guarded finalization as that original marker.
type peerDeletionSnapshot struct {
	MarkerFingerprint string `json:"marker_fingerprint"`
	PeerRecord        []byte `json:"peer_record"`
}

func peerDeletionSnapshotKey(deletionID string) kv.Key {
	return kv.Key{"peer-deletion-snapshots", deletionID}
}

func validatePeerDeletionSnapshot(data []byte, record pendingdeletion.Record) ([]byte, error) {
	var snapshot peerDeletionSnapshot
	fingerprint, err := pendingdeletion.Fingerprint(record)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &snapshot); err != nil || snapshot.MarkerFingerprint != fingerprint {
		return nil, pendingdeletion.Terminal("retirement_plan_invalid", "Retained Peer snapshot identity is invalid", err)
	}
	peer, err := decodePeer(snapshot.PeerRecord)
	if err != nil || peer.PublicKey != record.ResourceID || peer.CreatedAt.IsZero() || peer.UpdatedAt.IsZero() {
		return nil, pendingdeletion.Terminal("peer_record_invalid", "Retained Peer snapshot is invalid", err)
	}
	return snapshot.PeerRecord, nil
}

// expectedPlanPeer is evaluated after inventory preparation at both plan write
// sites. It preserves the old active-record CAS until snapshot evidence exists.
func (h DeletionHandler) expectedPlanPeer(ctx context.Context, record pendingdeletion.Record, priorHasSnapshot bool, raw []byte) ([]byte, error) {
	var descriptor peerDeletionDescriptor
	if err := json.Unmarshal(record.Descriptor, &descriptor); err != nil {
		return nil, pendingdeletion.Terminal("invalid_peer_marker", "Peer deletion descriptor is invalid", err)
	}
	if len(descriptor.PeerRecord) != 0 || priorHasSnapshot {
		return encodedPeerTombstone, nil
	}
	data, err := h.Server.Store.Get(ctx, peerDeletionSnapshotKey(record.DeletionID))
	if errors.Is(err, kv.ErrNotFound) {
		return raw, nil
	}
	if err != nil {
		return nil, pendingdeletion.Retryable("store_error", "Retained Peer snapshot could not be read", err)
	}
	snapshot, err := validatePeerDeletionSnapshot(data, record)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(snapshot, raw) {
		return nil, pendingdeletion.Terminal("peer_replacement_ambiguous", "Peer snapshot no longer matches its retirement plan", nil)
	}
	return encodedPeerTombstone, nil
}

func (s *Server) adoptPeerDeletion(ctx context.Context, store kv.Store, record pendingdeletion.Record, peer apitypes.Peer, raw []byte) error {
	fingerprint, err := pendingdeletion.Fingerprint(record)
	if err != nil {
		return err
	}
	var descriptor peerDeletionDescriptor
	if err := json.Unmarshal(record.Descriptor, &descriptor); err != nil {
		return err
	}
	if record.Kind != pendingdeletion.KindPeer || record.ResourceID != peer.PublicKey ||
		record.OwnerPublicKey == nil || *record.OwnerPublicKey != peer.PublicKey || descriptor.PublicKey != peer.PublicKey ||
		(record.Reason != pendingdeletion.ReasonPeerDelete && record.Reason != pendingdeletion.ReasonAdminDelete) {
		return errors.New("peer: existing deletion request identity mismatch")
	}
	if len(descriptor.PeerRecord) != 0 && !bytes.Equal(descriptor.PeerRecord, raw) {
		return pendingdeletion.ErrConflict
	}
	var key giznet.PublicKey
	if err := key.UnmarshalText([]byte(peer.PublicKey)); err != nil {
		return err
	}
	planKey := peerRetirementPlanKey(record.DeletionID)
	planData, err := store.Get(ctx, planKey)
	if err == nil {
		plan, err := validateRetirementPlan(planData, pendingdeletion.Claim{Task: pendingdeletion.Task{MarkerFingerprint: fingerprint}}, key)
		if err != nil {
			return err
		}
		if err := (DeletionHandler{Server: s}).checkPlan(plan, peer, raw); err != nil {
			return err
		}
		if len(plan.PeerRecord) != 0 && !bytes.Equal(plan.PeerRecord, raw) {
			return pendingdeletion.ErrConflict
		}
	} else if !errors.Is(err, kv.ErrNotFound) {
		return err
	}
	conditions := []kv.Condition{{Key: peerKey(peer.PublicKey), Expected: raw}, {Key: planKey, Expected: planData}}
	entries := []kv.Entry{{Key: peerKey(peer.PublicKey), Value: encodedPeerTombstone}}
	if len(descriptor.PeerRecord) == 0 {
		snapshotKey := peerDeletionSnapshotKey(record.DeletionID)
		data, err := store.Get(ctx, snapshotKey)
		if err == nil {
			snapshot, err := validatePeerDeletionSnapshot(data, record)
			if err != nil {
				return err
			}
			if !bytes.Equal(snapshot, raw) {
				return pendingdeletion.ErrConflict
			}
		} else if !errors.Is(err, kv.ErrNotFound) {
			return err
		} else {
			encoded, err := json.Marshal(peerDeletionSnapshot{MarkerFingerprint: fingerprint, PeerRecord: raw})
			if err != nil {
				return err
			}
			entries = append(entries, kv.Entry{Key: snapshotKey, Value: encoded})
		}
		conditions = append(conditions, kv.Condition{Key: snapshotKey, Expected: data})
	}
	matched, err := pendingdeletion.MutateRecordGuarded(ctx, store, record, conditions, entries...)
	if err != nil {
		return err
	}
	if !matched {
		return pendingdeletion.ErrConflict
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
	key, err := validatePeerDeletionClaim(pendingdeletion.Claim{Task: task})
	if err != nil {
		return false, err
	}
	if err := h.checkHome(ctx, key); err != nil {
		return false, err
	}
	current, raw, err := h.retainedPeer(ctx, task.Record, key)
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

// planPeer verifies an observation against a retained plan before cleanup. An
// inconsistent post-claim observation is rechecked once at the write authority.
func (h DeletionHandler) planPeer(ctx context.Context, record pendingdeletion.Record, plan retirementPlan, key giznet.PublicKey) ([]byte, error) {
	_, raw, err := h.planPeerRecord(ctx, record, plan, key)
	return raw, err
}

// planPeerRecord retains the accepted decoded observation for plan adoption.
func (h DeletionHandler) planPeerRecord(ctx context.Context, record pendingdeletion.Record, plan retirementPlan, key giznet.PublicKey) (apitypes.Peer, []byte, error) {
	var lastErr error
	for range 2 {
		current, raw, err := h.retainedPeer(ctx, record, key)
		if err == nil {
			err = h.checkPlan(plan, current, raw)
		}
		if err == nil {
			return current, raw, nil
		}
		lastErr = err
		ctx = kv.WithPrimaryReads(ctx)
	}
	return apitypes.Peer{}, nil, lastErr
}
