package friendgroup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

const friendGroupRetirementPollInterval = 5 * time.Second

// DeletionHandler finalizes a Friend Group marker only after Social has
// durably handed its Workspace to the independent Workspace deletion flow.
type DeletionHandler struct {
	Server *Server
	Source pendingdeletion.KVSource
	Now    func() time.Time
}

func (h DeletionHandler) Kind() pendingdeletion.Kind {
	return pendingdeletion.KindFriendGroup
}

func (h DeletionHandler) Handle(ctx context.Context, claim pendingdeletion.Claim) error {
	if err := pendingdeletion.ValidateTask(claim.Task); err != nil {
		return pendingdeletion.Terminal("invalid_task_state", "Friend Group deletion task state is invalid", err)
	}
	descriptor, err := validateFriendGroupDeletionClaim(claim)
	if err != nil {
		return pendingdeletion.Terminal("invalid_friend_group_marker", "Friend Group deletion marker is invalid", err)
	}
	if h.Server == nil {
		return pendingdeletion.Retryable("service_unavailable", "Friend Group service is unavailable", errors.New("social: Friend Group service not configured"))
	}
	now := time.Now().UTC()
	if h.Now != nil {
		now = h.Now().UTC()
	}
	if claim.Phase == pendingdeletion.PhaseValidate {
		if err := h.verifyRetirement(ctx, descriptor.FriendGroupID); err != nil {
			return err
		}
		claim, err = h.Source.Checkpoint(ctx, claim, pendingdeletion.PhaseFinalize, now)
		if err != nil {
			return err
		}
	}
	if claim.Phase != pendingdeletion.PhaseFinalize {
		return pendingdeletion.Terminal("invalid_phase", "Friend Group deletion phase is invalid", nil)
	}
	if err := h.verifyRetirement(ctx, descriptor.FriendGroupID); err != nil {
		return err
	}
	if err := h.Source.Finalize(ctx, claim, now, nil); err != nil {
		if errors.Is(err, pendingdeletion.ErrConflict) {
			return err
		}
		return pendingdeletion.Retryable("store_error", "Friend Group deletion finalization failed", err)
	}
	return nil
}

func validateFriendGroupDeletionClaim(claim pendingdeletion.Claim) (retiredFriendGroupDataDescriptor, error) {
	if claim.Source != pendingDeletionSourceName || claim.Record.Kind != pendingdeletion.KindFriendGroup {
		return retiredFriendGroupDataDescriptor{}, errors.New("social: unsupported pending deletion source or kind")
	}
	if claim.Record.Reason != pendingdeletion.ReasonFriendGroupDelete || claim.Record.DescriptorVersion != pendingdeletion.DescriptorVersion {
		return retiredFriendGroupDataDescriptor{}, errors.New("social: unsupported Friend Group deletion reason or descriptor version")
	}
	if claim.Record.OwnerPublicKey != nil {
		return retiredFriendGroupDataDescriptor{}, errors.New("social: Friend Group deletion marker must be ownerless")
	}
	if err := customid.ValidateResourceID(claim.Record.ResourceID); err != nil {
		return retiredFriendGroupDataDescriptor{}, fmt.Errorf("social: invalid Friend Group deletion resource ID: %w", err)
	}
	var descriptor retiredFriendGroupDataDescriptor
	if err := json.Unmarshal(claim.Record.Descriptor, &descriptor); err != nil {
		return retiredFriendGroupDataDescriptor{}, fmt.Errorf("social: decode Friend Group deletion descriptor: %w", err)
	}
	if descriptor.FriendGroupID != claim.Record.ResourceID {
		return retiredFriendGroupDataDescriptor{}, errors.New("social: Friend Group deletion descriptor does not match marker identity")
	}
	if err := customid.ValidateResourceID(descriptor.FriendGroupID); err != nil {
		return retiredFriendGroupDataDescriptor{}, fmt.Errorf("social: invalid Friend Group ID in deletion descriptor: %w", err)
	}
	fingerprint, err := pendingdeletion.Fingerprint(claim.Record)
	if err != nil || fingerprint != claim.MarkerFingerprint {
		return retiredFriendGroupDataDescriptor{}, errors.New("social: Friend Group deletion marker fingerprint mismatch")
	}
	return descriptor, nil
}

func (h DeletionHandler) verifyRetirement(ctx context.Context, friendGroupID string) error {
	store, err := h.Server.relationshipStore()
	if err != nil {
		return pendingdeletion.Retryable("store_unavailable", "Friend Group relationship store is unavailable", err)
	}
	if _, err := store.Get(ctx, groupRetirementIntentKey(friendGroupID)); err == nil {
		return pendingdeletion.Deferred("retirement_in_progress", "Friend Group retirement is still completing", friendGroupRetirementPollInterval)
	} else if !errors.Is(err, kv.ErrNotFound) {
		return pendingdeletion.Retryable("store_error", "Friend Group retirement intent could not be read", err)
	}
	receipt, err := h.Server.readRetirementReceipt(ctx, friendGroupID)
	if errors.Is(err, kv.ErrNotFound) {
		return pendingdeletion.Deferred("retirement_receipt_pending", "Friend Group retirement receipt is not committed yet", friendGroupRetirementPollInterval)
	}
	if err != nil {
		return pendingdeletion.Terminal("retirement_receipt_invalid", "Friend Group retirement receipt is invalid", err)
	}
	if err := validateFriendGroupRetirementReceipt(receipt, friendGroupID); err != nil {
		return pendingdeletion.Terminal("retirement_receipt_invalid", "Friend Group retirement receipt is invalid", err)
	}
	if err := h.verifyControlPlaneAbsent(ctx, friendGroupID); err != nil {
		return err
	}
	return nil
}

func validateFriendGroupRetirementReceipt(receipt retirementReceipt, friendGroupID string) error {
	if receipt.FriendGroupID != friendGroupID || receipt.Name == "" || receipt.Name != strings.TrimSpace(receipt.Name) ||
		receipt.Owner == "" || receipt.Owner != strings.TrimSpace(receipt.Owner) ||
		receipt.WorkspaceName == "" || receipt.WorkspaceName != strings.TrimSpace(receipt.WorkspaceName) || receipt.DeletedAt.IsZero() {
		return errors.New("social: Friend Group retirement receipt identity is invalid")
	}
	if err := customid.ValidateResourceID(receipt.WorkspaceID); err != nil {
		return fmt.Errorf("social: invalid retired Workspace ID: %w", err)
	}
	return nil
}

func (h DeletionHandler) verifyControlPlaneAbsent(ctx context.Context, friendGroupID string) error {
	type exactRecord struct {
		name  string
		store kv.Store
		key   kv.Key
	}
	groups, err := h.Server.groupsStore()
	if err != nil {
		return pendingdeletion.Retryable("store_unavailable", "Friend Group store is unavailable", err)
	}
	invites, err := h.Server.groupInviteTokensStore()
	if err != nil {
		return pendingdeletion.Retryable("store_unavailable", "Friend Group invite store is unavailable", err)
	}
	relationships, err := h.Server.relationshipStore()
	if err != nil {
		return pendingdeletion.Retryable("store_unavailable", "Friend Group relationship store is unavailable", err)
	}
	for _, record := range []exactRecord{
		{name: "group", store: groups, key: socialutil.GroupKey(friendGroupID)},
		{name: "invite token", store: invites, key: socialutil.GroupInviteTokenKey(friendGroupID)},
		{name: "Workspace binding", store: relationships, key: workspaceBindingKey(friendGroupID)},
	} {
		if _, err := record.store.Get(ctx, record.key); err == nil {
			return pendingdeletion.Deferred("social_cleanup_incomplete", "Friend Group "+record.name+" remains after retirement", friendGroupRetirementPollInterval)
		} else if !errors.Is(err, kv.ErrNotFound) {
			return pendingdeletion.Retryable("store_error", "Friend Group "+record.name+" could not be read", err)
		}
	}

	members, err := h.Server.membersStore()
	if err != nil {
		return pendingdeletion.Retryable("store_unavailable", "Friend Group member store is unavailable", err)
	}
	memberPrefix := append(append(kv.Key{}, socialutil.GroupMembersRoot...), socialutil.EscapeStoreSegment(friendGroupID))
	if err := requireNoEntries(ctx, members, memberPrefix, "member"); err != nil {
		return err
	}
	belongs, err := h.Server.belongsStore()
	if err != nil {
		return pendingdeletion.Retryable("store_unavailable", "Friend Group belongs store is unavailable", err)
	}
	for entry, err := range belongs.List(ctx, socialutil.GroupBelongsRoot) {
		if err != nil {
			return pendingdeletion.Retryable("store_error", "Friend Group belongs rows could not be listed", err)
		}
		var member friendGroupMemberRecord
		if err := json.Unmarshal(entry.Value, &member); err != nil {
			return pendingdeletion.Terminal("control_plane_corrupt", "Friend Group belongs row is invalid", err)
		}
		if member.FriendGroupID == friendGroupID {
			return pendingdeletion.Deferred("social_cleanup_incomplete", "Friend Group belongs row remains after retirement", friendGroupRetirementPollInterval)
		}
	}
	for entry, err := range belongs.List(ctx, socialutil.GroupNamesRoot) {
		if err != nil {
			return pendingdeletion.Retryable("store_error", "Friend Group membership-name indexes could not be listed", err)
		}
		if string(entry.Value) == friendGroupID {
			return pendingdeletion.Deferred("social_cleanup_incomplete", "Friend Group membership-name index remains after retirement", friendGroupRetirementPollInterval)
		}
	}
	return nil
}

func requireNoEntries(ctx context.Context, store kv.Store, prefix kv.Key, name string) error {
	for _, err := range store.List(ctx, prefix) {
		if err != nil {
			return pendingdeletion.Retryable("store_error", "Friend Group "+name+" rows could not be listed", err)
		}
		return pendingdeletion.Deferred("social_cleanup_incomplete", "Friend Group "+name+" row remains after retirement", friendGroupRetirementPollInterval)
	}
	return nil
}

var _ pendingdeletion.Handler = DeletionHandler{}
