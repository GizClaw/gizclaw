package gameplay

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
)

type PeerGameplayPet struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	RuntimeProfileID string `json:"runtime_profile_id"`
	PetDefID         string `json:"pet_def_id"`
	WorkspaceID      string `json:"workspace_id"`
}

type PeerGameplaySnapshot struct {
	PublicKey string            `json:"public_key"`
	Pets      []PeerGameplayPet `json:"pets"`
}

// PeerRetirement adapts the optional Gameplay subsystem to Peer deletion. A
// server without a Gameplay database has no Gameplay-owned Peer rows, so the
// empty snapshot is both deterministic and immediately complete.
type PeerRetirement struct {
	Runtime *Runtime
}

func (r PeerRetirement) SnapshotPeerGameplay(ctx context.Context, publicKey string) (PeerGameplaySnapshot, error) {
	if publicKey == "" || publicKey != strings.TrimSpace(publicKey) {
		return PeerGameplaySnapshot{}, errors.New("gameplay: Peer public key is required and must be canonical")
	}
	if r.Runtime == nil || r.Runtime.DB == nil {
		return PeerGameplaySnapshot{PublicKey: publicKey}, nil
	}
	return r.Runtime.SnapshotPeerGameplay(ctx, publicKey)
}

func (r PeerRetirement) RetirePeerGameplay(ctx context.Context, snapshot PeerGameplaySnapshot) (bool, error) {
	if snapshot.PublicKey == "" || snapshot.PublicKey != strings.TrimSpace(snapshot.PublicKey) {
		return false, errors.New("gameplay: invalid Peer retirement snapshot")
	}
	if r.Runtime == nil || r.Runtime.DB == nil {
		if len(snapshot.Pets) != 0 {
			return false, errors.New("gameplay: retirement snapshot has Pets without a Gameplay database")
		}
		return true, nil
	}
	return r.Runtime.RetirePeerGameplay(ctx, snapshot)
}

// SnapshotPeerGameplay captures exact Pet identities before account mutation.
func (r *Runtime) SnapshotPeerGameplay(ctx context.Context, publicKey string) (PeerGameplaySnapshot, error) {
	if err := r.Migration(ctx); err != nil {
		return PeerGameplaySnapshot{}, err
	}
	db, err := r.db()
	if err != nil {
		return PeerGameplaySnapshot{}, err
	}
	if publicKey == "" || publicKey != strings.TrimSpace(publicKey) {
		return PeerGameplaySnapshot{}, errors.New("gameplay: Peer public key is required and must be canonical")
	}
	mu := r.accountMutex(publicKey)
	mu.Lock()
	defer mu.Unlock()
	rows, err := db.QueryContext(ctx, db.Rebind(petSelectSQL()+` WHERE owner_public_key = ? ORDER BY id`), publicKey)
	if err != nil {
		return PeerGameplaySnapshot{}, err
	}
	defer rows.Close()
	var pets []PeerGameplayPet
	for rows.Next() {
		pet, err := scanPet(rows)
		if err != nil {
			return PeerGameplaySnapshot{}, err
		}
		for name, id := range map[string]string{"Pet": pet.Id, "RuntimeProfile": pet.RuntimeProfileId, "PetDef": pet.PetDefId, "Workspace": pet.WorkspaceId} {
			if err := customid.ValidateResourceID(id); err != nil {
				return PeerGameplaySnapshot{}, fmt.Errorf("gameplay: invalid %s ID in Peer snapshot: %w", name, err)
			}
		}
		if pet.Name == "" || pet.Name != strings.TrimSpace(pet.Name) {
			return PeerGameplaySnapshot{}, errors.New("gameplay: invalid Pet name in Peer snapshot")
		}
		pets = append(pets, PeerGameplayPet{
			ID: pet.Id, Name: pet.Name, RuntimeProfileID: pet.RuntimeProfileId,
			PetDefID: pet.PetDefId, WorkspaceID: pet.WorkspaceId,
		})
	}
	if err := rows.Err(); err != nil {
		return PeerGameplaySnapshot{}, err
	}
	return PeerGameplaySnapshot{PublicKey: publicKey, Pets: pets}, nil
}

// RetirePeerGameplay creates/reuses every Pet child task. ready is false until
// the production Pet handler has removed every Pet and its mutable task.
func (r *Runtime) RetirePeerGameplay(ctx context.Context, snapshot PeerGameplaySnapshot) (ready bool, err error) {
	if snapshot.PublicKey == "" || snapshot.PublicKey != strings.TrimSpace(snapshot.PublicKey) {
		return false, errors.New("gameplay: invalid Peer retirement snapshot")
	}
	if err := r.Migration(ctx); err != nil {
		return false, err
	}
	mu := r.accountMutex(snapshot.PublicKey)
	mu.Lock()
	defer mu.Unlock()
	db, err := r.db()
	if err != nil {
		return false, err
	}
	for _, pet := range snapshot.Pets {
		if err := validatePeerGameplayPet(pet); err != nil {
			return false, err
		}
		current, err := findPetByOwnerID(ctx, db, snapshot.PublicKey, pet.ID)
		switch {
		case err == nil:
			if current.Name != pet.Name || current.RuntimeProfileId != pet.RuntimeProfileID || current.PetDefId != pet.PetDefID || current.WorkspaceId != pet.WorkspaceID {
				return false, errors.New("gameplay: Pet no longer matches Peer retirement snapshot")
			}
			if _, err := r.deletePetForAccountRetirement(ctx, snapshot.PublicKey, pet.ID); err != nil {
				return false, err
			}
		case errors.Is(err, sql.ErrNoRows):
			// The Pet handler may already have completed.
		default:
			return false, err
		}
		var taskStatus string
		err = db.QueryRowContext(ctx, db.Rebind(`SELECT task_status FROM gameplay_pending_deletions
			WHERE kind = ? AND owner_public_key = ? AND resource_id = ?`),
			pendingdeletion.KindPet, snapshot.PublicKey, pet.ID).Scan(&taskStatus)
		if err == nil {
			if taskStatus == string(pendingdeletion.StatusFailed) {
				return false, errors.New("gameplay: Pet deletion task failed")
			}
			ready = false
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
		if _, err := findPetByOwnerID(ctx, db, snapshot.PublicKey, pet.ID); err == nil {
			return false, errors.New("gameplay: Pet survived without its deletion task")
		} else if !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
	}
	for _, pet := range snapshot.Pets {
		var count int
		if err := db.QueryRowContext(ctx, db.Rebind(`SELECT
			(SELECT COUNT(*) FROM gameplay_pets WHERE owner_public_key = ? AND id = ?) +
			(SELECT COUNT(*) FROM gameplay_pending_deletions WHERE kind = ? AND owner_public_key = ? AND resource_id = ?)`),
			snapshot.PublicKey, pet.ID, pendingdeletion.KindPet, snapshot.PublicKey, pet.ID).Scan(&count); err != nil {
			return false, err
		}
		if count != 0 {
			return false, nil
		}
	}
	if err := r.purgePeerGameplay(ctx, snapshot.PublicKey); err != nil {
		return false, err
	}
	return true, nil
}

func validatePeerGameplayPet(pet PeerGameplayPet) error {
	if pet.Name == "" || pet.Name != strings.TrimSpace(pet.Name) {
		return errors.New("gameplay: invalid Pet snapshot name")
	}
	for name, id := range map[string]string{"Pet": pet.ID, "RuntimeProfile": pet.RuntimeProfileID, "PetDef": pet.PetDefID, "Workspace": pet.WorkspaceID} {
		if err := customid.ValidateResourceID(id); err != nil {
			return fmt.Errorf("gameplay: invalid %s ID in Pet snapshot: %w", name, err)
		}
	}
	return nil
}

func (r *Runtime) purgePeerGameplay(ctx context.Context, publicKey string) error {
	tx, err := r.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statements := []string{
		`DELETE FROM gameplay_pet_adoption_reservations WHERE owner_public_key = ?`,
		`DELETE FROM gameplay_pet_workspace_bindings WHERE owner_public_key = ?`,
		`DELETE FROM gameplay_pet_drive_ticks WHERE owner_public_key = ?`,
		`DELETE FROM gameplay_points_transactions WHERE owner_public_key = ?`,
		`DELETE FROM gameplay_badges WHERE owner_public_key = ?`,
		`DELETE FROM gameplay_game_results WHERE owner_public_key = ?`,
		`DELETE FROM gameplay_reward_grants WHERE owner_public_key = ?`,
		`DELETE FROM gameplay_drive_fact_outbox WHERE owner_public_key = ?`,
		`DELETE FROM gameplay_workspace_reward_windows WHERE beneficiary_public_key = ?`,
		`DELETE FROM gameplay_points_accounts WHERE owner_public_key = ?`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, tx.Rebind(statement), publicKey); err != nil {
			return err
		}
	}
	count, err := peerGameplayRowCount(ctx, tx, publicKey)
	if err != nil {
		return err
	}
	if count != 0 {
		return fmt.Errorf("gameplay: %d Peer-scoped rows remain after purge", count)
	}
	return tx.Commit()
}

func peerGameplayRowCount(ctx context.Context, db queryRebinder, publicKey string) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, db.Rebind(`SELECT
		(SELECT COUNT(*) FROM gameplay_pet_adoption_reservations WHERE owner_public_key = ?) +
		(SELECT COUNT(*) FROM gameplay_pet_workspace_bindings WHERE owner_public_key = ?) +
		(SELECT COUNT(*) FROM gameplay_pet_drive_ticks WHERE owner_public_key = ?) +
		(SELECT COUNT(*) FROM gameplay_points_accounts WHERE owner_public_key = ?) +
		(SELECT COUNT(*) FROM gameplay_points_transactions WHERE owner_public_key = ?) +
		(SELECT COUNT(*) FROM gameplay_badges WHERE owner_public_key = ?) +
		(SELECT COUNT(*) FROM gameplay_game_results WHERE owner_public_key = ?) +
		(SELECT COUNT(*) FROM gameplay_reward_grants WHERE owner_public_key = ?) +
		(SELECT COUNT(*) FROM gameplay_drive_fact_outbox WHERE owner_public_key = ?) +
		(SELECT COUNT(*) FROM gameplay_workspace_reward_windows WHERE beneficiary_public_key = ?)`),
		publicKey, publicKey, publicKey, publicKey, publicKey,
		publicKey, publicKey, publicKey, publicKey, publicKey).Scan(&count)
	return count, err
}

type accountRetirementContextKey struct{}

func (r *Runtime) deletePetForAccountRetirement(ctx context.Context, owner, id string) (apitypes.Pet, error) {
	return r.DeletePet(context.WithValue(ctx, accountRetirementContextKey{}, true), owner, id)
}

func accountRetirementFromContext(ctx context.Context) bool {
	value, _ := ctx.Value(accountRetirementContextKey{}).(bool)
	return value
}
