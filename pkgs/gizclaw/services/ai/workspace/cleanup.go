package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/iconasset"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type WorkspaceQuiescer interface {
	QuiesceWorkspace(context.Context, string) error
}

type GameplayWorkspaceCleanup interface {
	DeleteWorkspaceData(context.Context, string) error
	WorkspaceDataAbsent(context.Context, string) (bool, error)
}

// DeletionHandler owns Workspace artifact cleanup and record finalization.
type DeletionHandler struct {
	Server   *Server
	Source   pendingdeletion.KVSource
	Quiescer WorkspaceQuiescer
	Gameplay GameplayWorkspaceCleanup
	Now      func() time.Time
}

type validatedDeletion struct {
	ID             string
	Name           string
	OwnerPublicKey *string
	HasIcon        bool
	System         bool
	Kind           *socialutil.SFUWorkspaceKind
}

func (h DeletionHandler) Kind() pendingdeletion.Kind {
	return pendingdeletion.KindWorkspace
}

func (h DeletionHandler) Handle(ctx context.Context, claim pendingdeletion.Claim) error {
	if err := pendingdeletion.ValidateTask(claim.Task); err != nil {
		return pendingdeletion.Terminal("invalid_task_state", "Workspace deletion task state is invalid", err)
	}
	descriptor, err := validateWorkspaceDeletionClaim(claim)
	if err != nil {
		return pendingdeletion.Terminal("invalid_workspace_marker", "Workspace deletion marker is invalid", err)
	}
	if h.Server == nil {
		return pendingdeletion.Retryable("service_unavailable", "Workspace service is unavailable", errors.New("workspace: service not configured"))
	}
	now := time.Now().UTC()
	if h.Now != nil {
		now = h.Now().UTC()
	}
	if claim.Phase == pendingdeletion.PhaseValidate {
		if err := h.validateRetainedWorkspace(ctx, descriptor); err != nil {
			return err
		}
		if err := h.cleanupArtifacts(ctx, descriptor); err != nil {
			return err
		}
		claim, err = h.Source.Checkpoint(ctx, claim, pendingdeletion.PhaseFinalize, now)
		if err != nil {
			return err
		}
	}
	if claim.Phase != pendingdeletion.PhaseFinalize {
		return pendingdeletion.Terminal("invalid_phase", "Workspace deletion phase is invalid", nil)
	}
	if err := h.validateRetainedWorkspace(ctx, descriptor); err != nil {
		return err
	}
	if err := h.cleanupArtifacts(ctx, descriptor); err != nil {
		return err
	}
	if err := h.verifyArtifactsAbsent(ctx, descriptor); err != nil {
		return err
	}

	unlock := h.Server.IconLocks.LockOwner(descriptor.ID)
	defer unlock()
	if err := h.validateRetainedWorkspace(ctx, descriptor); err != nil {
		return err
	}
	deleteKeys, err := h.finalizationKeys(ctx, descriptor)
	if err != nil {
		return err
	}
	if err := h.Source.Finalize(ctx, claim, now, deleteKeys); err != nil {
		if errors.Is(err, pendingdeletion.ErrConflict) {
			return err
		}
		return pendingdeletion.Retryable("store_error", "Workspace deletion finalization failed", err)
	}
	return nil
}

func validateWorkspaceDeletionClaim(claim pendingdeletion.Claim) (validatedDeletion, error) {
	if claim.Source != pendingDeletionSourceName || claim.Record.Kind != pendingdeletion.KindWorkspace {
		return validatedDeletion{}, errors.New("workspace: unsupported pending deletion source or kind")
	}
	if claim.Record.DescriptorVersion != pendingdeletion.DescriptorVersion {
		return validatedDeletion{}, errors.New("workspace: unsupported deletion descriptor version")
	}
	if err := customid.ValidateResourceID(claim.Record.ResourceID); err != nil {
		return validatedDeletion{}, fmt.Errorf("workspace: invalid deletion resource ID: %w", err)
	}
	var out validatedDeletion
	switch claim.Record.Reason {
	case pendingdeletion.ReasonResourceDelete, pendingdeletion.ReasonPeerDelete:
		var descriptor workspaceDeletionDescriptor
		if err := json.Unmarshal(claim.Record.Descriptor, &descriptor); err != nil {
			return validatedDeletion{}, fmt.Errorf("workspace: decode user Workspace deletion descriptor: %w", err)
		}
		out = validatedDeletion{
			ID: descriptor.ID, Name: descriptor.Name, OwnerPublicKey: cloneString(descriptor.OwnerPublicKey),
			HasIcon: descriptor.HasIcon, System: descriptor.System,
		}
		if claim.Record.OwnerPublicKey == nil || descriptor.OwnerPublicKey == nil || *claim.Record.OwnerPublicKey != *descriptor.OwnerPublicKey {
			return validatedDeletion{}, errors.New("workspace: Workspace deletion owner mismatch")
		}
		if claim.Record.Reason == pendingdeletion.ReasonResourceDelete && descriptor.System {
			return validatedDeletion{}, errors.New("workspace: generic deletion cannot retire a system Workspace")
		}
		if claim.Record.Reason == pendingdeletion.ReasonPeerDelete && !descriptor.System {
			return validatedDeletion{}, errors.New("workspace: Peer child deletion requires a system Workspace")
		}
	case pendingdeletion.ReasonFriendRelationshipDelete, pendingdeletion.ReasonFriendGroupDelete:
		if claim.Record.OwnerPublicKey != nil {
			return validatedDeletion{}, errors.New("workspace: Social Workspace marker must be ownerless")
		}
		var descriptor socialRetirementDescriptor
		if err := json.Unmarshal(claim.Record.Descriptor, &descriptor); err != nil {
			return validatedDeletion{}, fmt.Errorf("workspace: decode Social Workspace deletion descriptor: %w", err)
		}
		expectedKind := socialutil.SFUWorkspaceKindFriend
		if claim.Record.Reason == pendingdeletion.ReasonFriendGroupDelete {
			expectedKind = socialutil.SFUWorkspaceKindFriendGroup
		}
		if descriptor.WorkspaceKind != expectedKind || descriptor.SocialResourceID == "" {
			return validatedDeletion{}, errors.New("workspace: Social Workspace reason and domain binding mismatch")
		}
		if err := customid.ValidateResourceID(descriptor.SocialResourceID); err != nil {
			return validatedDeletion{}, fmt.Errorf("workspace: invalid Social resource ID: %w", err)
		}
		out = validatedDeletion{
			ID: descriptor.ID, Name: descriptor.Name, OwnerPublicKey: cloneString(descriptor.OwnerPublicKey),
			HasIcon: descriptor.HasIcon, System: true, Kind: &expectedKind,
		}
	default:
		return validatedDeletion{}, fmt.Errorf("workspace: unsupported deletion reason %q", claim.Record.Reason)
	}
	if out.ID != claim.Record.ResourceID || out.Name == "" || out.Name != strings.TrimSpace(out.Name) {
		return validatedDeletion{}, errors.New("workspace: deletion descriptor identity mismatch")
	}
	if err := customid.ValidateResourceID(out.ID); err != nil {
		return validatedDeletion{}, fmt.Errorf("workspace: invalid descriptor ID: %w", err)
	}
	if out.OwnerPublicKey != nil && (*out.OwnerPublicKey == "" || *out.OwnerPublicKey != strings.TrimSpace(*out.OwnerPublicKey)) {
		return validatedDeletion{}, errors.New("workspace: deletion descriptor owner is not canonical")
	}
	fingerprint, err := pendingdeletion.Fingerprint(claim.Record)
	if err != nil || fingerprint != claim.MarkerFingerprint {
		return validatedDeletion{}, errors.New("workspace: deletion marker fingerprint mismatch")
	}
	return out, nil
}

func (h DeletionHandler) validateRetainedWorkspace(ctx context.Context, descriptor validatedDeletion) error {
	store, err := h.Server.store()
	if err != nil {
		return pendingdeletion.Retryable("store_unavailable", "Workspace store is unavailable", err)
	}
	item, err := getWorkspaceByID(ctx, store, descriptor.ID)
	if errors.Is(err, kv.ErrNotFound) {
		return h.validateLegacyIndexes(ctx, store, descriptor)
	}
	if err != nil {
		return pendingdeletion.Retryable("store_error", "Workspace record could not be read", err)
	}
	if item.Id != descriptor.ID || item.Name != descriptor.Name || workspaceIsSystem(item) != descriptor.System ||
		!equalOptionalString(item.OwnerPublicKey, descriptor.OwnerPublicKey) || (item.Icon != nil) != descriptor.HasIcon {
		return pendingdeletion.Terminal("replacement_ambiguous", "Workspace no longer matches its deletion marker", nil)
	}
	if descriptor.Kind != nil && item.Parameters != nil {
		return pendingdeletion.Terminal("workspace_class_mismatch", "Retained Social Workspace gained parameters", nil)
	}
	return nil
}

func (h DeletionHandler) validateLegacyIndexes(ctx context.Context, store kv.Store, descriptor validatedDeletion) error {
	checks := []struct {
		key kv.Key
	}{
		{key: workspaceScopeNameKey(descriptor.OwnerPublicKey, descriptor.Name)},
	}
	if descriptor.OwnerPublicKey != nil && !descriptor.System {
		checks = append(checks, struct{ key kv.Key }{key: workspaceByOwnerKey(*descriptor.OwnerPublicKey, descriptor.Name)})
	}
	for _, check := range checks {
		value, err := store.Get(ctx, check.key)
		if errors.Is(err, kv.ErrNotFound) {
			continue
		}
		if err != nil {
			return pendingdeletion.Retryable("store_error", "Workspace index could not be read", err)
		}
		if string(value) != descriptor.ID {
			return pendingdeletion.Terminal("replacement_ambiguous", "Workspace index belongs to a replacement", nil)
		}
	}
	return nil
}

func (h DeletionHandler) cleanupArtifacts(ctx context.Context, descriptor validatedDeletion) error {
	if h.Quiescer != nil {
		if err := h.Quiescer.QuiesceWorkspace(ctx, descriptor.ID); err != nil {
			return pendingdeletion.Retryable("quiesce_failed", "Workspace runtime could not be quiesced", err)
		}
	}
	if h.Gameplay != nil {
		if err := h.Gameplay.DeleteWorkspaceData(ctx, descriptor.ID); err != nil {
			return pendingdeletion.Retryable("gameplay_cleanup_failed", "Workspace Gameplay data could not be deleted", err)
		}
	}
	if h.Server.RuntimeStore != nil {
		if err := h.Server.RuntimeStore.DeleteWorkspaceRuntime(ctx, descriptor.ID); err != nil {
			return pendingdeletion.Retryable("runtime_cleanup_failed", "Workspace runtime data could not be deleted", err)
		}
	}
	if descriptor.HasIcon && h.Server.Assets == nil {
		return pendingdeletion.Retryable("asset_store_unavailable", "Workspace icon store is unavailable", errors.New("workspace: asset store not configured"))
	}
	if h.Server.Assets != nil {
		for _, format := range []iconasset.Format{iconasset.FormatPixa, iconasset.FormatPNG} {
			if err := h.Server.Assets.Delete(iconasset.ObjectName(descriptor.ID, format)); err != nil {
				return pendingdeletion.Retryable("asset_cleanup_failed", "Workspace icon could not be deleted", err)
			}
		}
	}
	return nil
}

func (h DeletionHandler) verifyArtifactsAbsent(ctx context.Context, descriptor validatedDeletion) error {
	if h.Gameplay != nil {
		absent, err := h.Gameplay.WorkspaceDataAbsent(ctx, descriptor.ID)
		if err != nil {
			return pendingdeletion.Retryable("gameplay_verify_failed", "Workspace Gameplay cleanup could not be verified", err)
		}
		if !absent {
			return pendingdeletion.Retryable("gameplay_residual", "Workspace Gameplay data remains", nil)
		}
	}
	if h.Server.RuntimeStore != nil {
		cleanupStore, ok := h.Server.RuntimeStore.(RuntimeCleanupStore)
		if !ok {
			return pendingdeletion.Terminal("runtime_verification_unavailable", "Workspace runtime store cannot verify physical deletion", nil)
		}
		absent, err := cleanupStore.WorkspaceRuntimeAbsent(ctx, descriptor.ID)
		if err != nil {
			return pendingdeletion.Retryable("runtime_verify_failed", "Workspace runtime cleanup could not be verified", err)
		}
		if !absent {
			return pendingdeletion.Retryable("runtime_residual", "Workspace runtime data remains", nil)
		}
	}
	if h.Server.Assets != nil {
		for _, format := range []iconasset.Format{iconasset.FormatPixa, iconasset.FormatPNG} {
			reader, err := h.Server.Assets.Get(iconasset.ObjectName(descriptor.ID, format))
			if err == nil {
				reader.Close()
				return pendingdeletion.Retryable("asset_residual", "Workspace icon remains", nil)
			}
			if !errors.Is(err, fs.ErrNotExist) {
				return pendingdeletion.Retryable("asset_verify_failed", "Workspace icon cleanup could not be verified", err)
			}
		}
	}
	return nil
}

func (h DeletionHandler) finalizationKeys(ctx context.Context, descriptor validatedDeletion) ([]kv.Key, error) {
	store, err := h.Server.store()
	if err != nil {
		return nil, pendingdeletion.Retryable("store_unavailable", "Workspace store is unavailable", err)
	}
	if err := h.validateLegacyIndexes(ctx, store, descriptor); err != nil {
		return nil, err
	}
	keys := []kv.Key{
		workspaceKey(descriptor.ID),
		workspaceScopeNameKey(descriptor.OwnerPublicKey, descriptor.Name),
	}
	if descriptor.OwnerPublicKey != nil && !descriptor.System {
		keys = append(keys, workspaceByOwnerKey(*descriptor.OwnerPublicKey, descriptor.Name))
	}
	return keys, nil
}

func equalOptionalString(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

var _ pendingdeletion.Handler = DeletionHandler{}
