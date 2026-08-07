package gameplay

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/jmoiron/sqlx"
)

type petDeletionDescriptor struct {
	OwnerPublicKey string `json:"owner_public_key"`
	PetID          string `json:"pet_id"`
	RuntimeProfile string `json:"runtime_profile_id"`
	PetDefID       string `json:"pet_def_id"`
	WorkspaceID    string `json:"workspace_id"`
}

// PetDeletionHandler owns validation and domain-atomic Pet finalization.
type PetDeletionHandler struct {
	DB  *sqlx.DB
	Now func() time.Time
}

func (h PetDeletionHandler) Kind() pendingdeletion.Kind {
	return pendingdeletion.KindPet
}

func (h PetDeletionHandler) Handle(ctx context.Context, claim pendingdeletion.Claim) error {
	if err := pendingdeletion.ValidateTask(claim.Task); err != nil {
		return pendingdeletion.Terminal("invalid_task_state", "Pet deletion task state is invalid", err)
	}
	descriptor, err := validatePetDeletionClaim(claim)
	if err != nil {
		return pendingdeletion.Terminal("invalid_pet_marker", "Pet deletion marker is invalid", err)
	}
	if h.DB == nil {
		return pendingdeletion.Retryable("database_unavailable", "Gameplay database is unavailable", errors.New("gameplay: database not configured"))
	}
	now := time.Now().UTC()
	if h.Now != nil {
		now = h.Now().UTC()
	}
	if claim.Phase == pendingdeletion.PhaseValidate {
		claim, err = (PendingDeletionSource{DB: h.DB}).Checkpoint(ctx, claim, pendingdeletion.PhaseFinalize, now)
		if err != nil {
			return err
		}
	}
	if claim.Phase != pendingdeletion.PhaseFinalize {
		return pendingdeletion.Terminal("invalid_phase", "Pet deletion phase is invalid", nil)
	}
	if err := h.finalize(ctx, claim, descriptor, now); err != nil {
		var outcome *pendingdeletion.OutcomeError
		if errors.As(err, &outcome) || errors.Is(err, pendingdeletion.ErrConflict) {
			return err
		}
		return pendingdeletion.Retryable("database_error", "Pet deletion transaction failed", err)
	}
	return nil
}

func validatePetDeletionClaim(claim pendingdeletion.Claim) (petDeletionDescriptor, error) {
	if claim.Source != gameplayPendingDeletionSource || claim.Record.Kind != pendingdeletion.KindPet {
		return petDeletionDescriptor{}, errors.New("gameplay: unsupported pending deletion source or kind")
	}
	if claim.Record.Reason != pendingdeletion.ReasonResourceDelete || claim.Record.DescriptorVersion != pendingdeletion.DescriptorVersion {
		return petDeletionDescriptor{}, errors.New("gameplay: unsupported Pet deletion reason or descriptor version")
	}
	if claim.Record.OwnerPublicKey == nil || strings.TrimSpace(*claim.Record.OwnerPublicKey) == "" {
		return petDeletionDescriptor{}, errors.New("gameplay: Pet deletion owner is required")
	}
	var descriptor petDeletionDescriptor
	if err := json.Unmarshal(claim.Record.Descriptor, &descriptor); err != nil {
		return petDeletionDescriptor{}, fmt.Errorf("gameplay: decode Pet deletion descriptor: %w", err)
	}
	if descriptor.OwnerPublicKey != *claim.Record.OwnerPublicKey || descriptor.PetID != claim.Record.ResourceID ||
		strings.TrimSpace(descriptor.RuntimeProfile) == "" || strings.TrimSpace(descriptor.PetDefID) == "" ||
		strings.TrimSpace(descriptor.WorkspaceID) == "" {
		return petDeletionDescriptor{}, errors.New("gameplay: Pet deletion descriptor does not match marker identity")
	}
	if descriptor.OwnerPublicKey != strings.TrimSpace(descriptor.OwnerPublicKey) ||
		descriptor.RuntimeProfile != strings.TrimSpace(descriptor.RuntimeProfile) ||
		descriptor.PetDefID != strings.TrimSpace(descriptor.PetDefID) ||
		descriptor.WorkspaceID != strings.TrimSpace(descriptor.WorkspaceID) {
		return petDeletionDescriptor{}, errors.New("gameplay: Pet deletion descriptor is not canonical")
	}
	for name, id := range map[string]string{
		"Pet": descriptor.PetID, "RuntimeProfile": descriptor.RuntimeProfile,
		"PetDef": descriptor.PetDefID, "Workspace": descriptor.WorkspaceID,
	} {
		if err := customid.ValidateResourceID(id); err != nil {
			return petDeletionDescriptor{}, fmt.Errorf("gameplay: invalid %s ID in Pet deletion descriptor: %w", name, err)
		}
	}
	fingerprint, err := pendingdeletion.Fingerprint(claim.Record)
	if err != nil || fingerprint != claim.MarkerFingerprint {
		return petDeletionDescriptor{}, errors.New("gameplay: Pet deletion marker fingerprint mismatch")
	}
	return descriptor, nil
}

func (h PetDeletionHandler) finalize(ctx context.Context, claim pendingdeletion.Claim, descriptor petDeletionDescriptor, now time.Time) error {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := scanPendingDeletionTask(tx.QueryRowxContext(ctx, tx.Rebind(pendingDeletionTaskSelectSQL()+` WHERE deletion_id = ?`), claim.Record.DeletionID))
	if errors.Is(err, sql.ErrNoRows) {
		return pendingdeletion.ErrConflict
	}
	if err != nil {
		return err
	}
	if current.MarkerFingerprint != claim.MarkerFingerprint || current.LeaseToken != claim.LeaseToken ||
		current.Status != pendingdeletion.StatusRunning || !current.LeaseDeadline.After(now) {
		return pendingdeletion.ErrConflict
	}

	retained := pendingdeletion.IsDeterministic(claim.Record)
	pet, petErr := findPetByOwnerID(ctx, tx, descriptor.OwnerPublicKey, descriptor.PetID)
	if retained {
		if errors.Is(petErr, sql.ErrNoRows) {
			return pendingdeletion.Terminal("retained_pet_missing", "Retained Pet is missing", petErr)
		}
		if petErr != nil {
			return petErr
		}
		if pet.OwnerPublicKey != descriptor.OwnerPublicKey || pet.Id != descriptor.PetID ||
			pet.RuntimeProfileId != descriptor.RuntimeProfile || pet.PetDefId != descriptor.PetDefID ||
			pet.WorkspaceId != descriptor.WorkspaceID {
			return pendingdeletion.Terminal("retained_pet_mismatch", "Retained Pet no longer matches its deletion marker", nil)
		}
		result, deleteErr := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM gameplay_pets
			WHERE owner_public_key = ? AND id = ? AND runtime_profile_id = ? AND pet_def_id = ? AND workspace_id = ?`),
			descriptor.OwnerPublicKey, descriptor.PetID, descriptor.RuntimeProfile, descriptor.PetDefID, descriptor.WorkspaceID)
		if deleteErr != nil {
			return deleteErr
		}
		deleted, deleteErr := result.RowsAffected()
		if deleteErr != nil {
			return deleteErr
		}
		if deleted != 1 {
			return pendingdeletion.Terminal("pet_delete_conflict", "Retained Pet changed during finalization", nil)
		}
	} else {
		if petErr == nil {
			return pendingdeletion.Terminal("replacement_ambiguous", "Legacy marker may refer to a replacement Pet", nil)
		}
		if !errors.Is(petErr, sql.ErrNoRows) {
			return petErr
		}
	}

	locatorResult, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM gameplay_pending_deletion_locators
		WHERE kind = ? AND owner_public_key = ? AND resource_id = ? AND deletion_id = ?`),
		claim.Record.Kind, descriptor.OwnerPublicKey, descriptor.PetID, claim.Record.DeletionID)
	if err != nil {
		return err
	}
	locatorRows, err := locatorResult.RowsAffected()
	if err != nil {
		return err
	}
	if retained && locatorRows != 1 {
		return pendingdeletion.Terminal("locator_mismatch", "Pet deletion locator does not match marker", nil)
	}
	if locatorRows > 1 {
		return pendingdeletion.Terminal("locator_duplicate", "Pet deletion locator is duplicated", nil)
	}

	markerResult, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM gameplay_pending_deletions
		WHERE deletion_id = ? AND marker_fingerprint = ? AND task_status = ?
			AND lease_token = ? AND lease_deadline > ?`),
		claim.Record.DeletionID, claim.MarkerFingerprint, pendingdeletion.StatusRunning,
		claim.LeaseToken, formatPendingDeletionTime(now))
	if err != nil {
		return err
	}
	markerRows, err := markerResult.RowsAffected()
	if err != nil {
		return err
	}
	if markerRows != 1 {
		return pendingdeletion.ErrConflict
	}
	return tx.Commit()
}

var _ pendingdeletion.Handler = PetDeletionHandler{}
