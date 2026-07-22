package gameplay

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/jmoiron/sqlx"
)

const defaultPetWorkflowName = "pet-care"

var (
	// ErrPetDead is returned when a Drive targets a terminal Pet.
	ErrPetDead = errors.New("gameplay: pet is dead")
	// ErrPetIDConflict is returned when a caller-assigned Pet ID is reserved by another adoption context or retained history.
	ErrPetIDConflict = errors.New("gameplay: pet id is already reserved")
	// ErrInvalidPetID is returned when a caller-assigned Pet ID is not canonical.
	ErrInvalidPetID          = errors.New("gameplay: invalid pet id")
	errInsufficientPoints    = errors.New("gameplay: insufficient points")
	errPetWorkspaceNotFound  = errors.New("gameplay: pet workspace binding not found")
	errPetWorkspaceAmbiguous = errors.New("gameplay: pet workspace binding is ambiguous")
)

type Runtime struct {
	DB          *sqlx.DB
	Catalog     *Catalog
	Workflows   WorkflowService
	Workspaces  workspace.SystemWorkspaceService
	Now         func() time.Time
	NewID       func() string
	PickWeight  func(total int64) int64
	DecayPeriod time.Duration
	adoptMu     [64]sync.Mutex
	driveMu     [64]sync.Mutex
}

type WorkflowService interface {
	GetWorkflow(context.Context, adminhttp.GetWorkflowRequestObject) (adminhttp.GetWorkflowResponseObject, error)
}

func (r *Runtime) Migration(ctx context.Context) error {
	db, err := r.db()
	if err != nil {
		return err
	}
	if err := validateSQLDialect(db.DriverName()); err != nil {
		return err
	}
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS gameplay_pets (
			owner_public_key TEXT NOT NULL,
			id TEXT NOT NULL,
			runtime_profile_name TEXT NOT NULL,
			petdef_id TEXT NOT NULL,
			display_name TEXT NOT NULL,
			workspace_name TEXT NOT NULL,
			stats_json TEXT NOT NULL,
			progression_json TEXT NOT NULL,
			lifecycle TEXT NOT NULL,
			died_at TEXT,
			state_settled_at TEXT NOT NULL,
			last_active_at TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(owner_public_key, id)
		)`,
		`CREATE TABLE IF NOT EXISTS gameplay_pet_adoption_reservations (
			owner_public_key TEXT NOT NULL,
			pet_id TEXT NOT NULL,
			runtime_profile_name TEXT NOT NULL,
			petdef_id TEXT NOT NULL,
			display_name TEXT NOT NULL,
			workspace_name TEXT NOT NULL,
			workflow_name TEXT NOT NULL,
			voice_alias TEXT NOT NULL,
			adoption_cost INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			PRIMARY KEY(owner_public_key, pet_id)
		)`,
		`CREATE TABLE IF NOT EXISTS gameplay_pet_workspace_bindings (
			owner_public_key TEXT NOT NULL,
			pet_id TEXT NOT NULL,
			runtime_profile_name TEXT NOT NULL,
			workspace_name TEXT NOT NULL,
			created_at TEXT NOT NULL,
			PRIMARY KEY(owner_public_key, pet_id)
		)`,
		`CREATE TABLE IF NOT EXISTS gameplay_pet_drive_ticks (
			owner_public_key TEXT NOT NULL,
			runtime_profile_name TEXT NOT NULL,
			idempotency_key TEXT NOT NULL,
			pet_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			PRIMARY KEY(owner_public_key, runtime_profile_name, idempotency_key)
		)`,
		`CREATE TABLE IF NOT EXISTS gameplay_points_accounts (
			owner_public_key TEXT NOT NULL,
			runtime_profile_name TEXT NOT NULL,
			balance INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(owner_public_key, runtime_profile_name)
		)`,
		`CREATE TABLE IF NOT EXISTS gameplay_points_transactions (
			owner_public_key TEXT NOT NULL,
			id TEXT NOT NULL,
			runtime_profile_name TEXT NOT NULL,
			pet_id TEXT,
			game_result_id TEXT,
			reward_grant_id TEXT,
			delta INTEGER NOT NULL,
			balance_after INTEGER NOT NULL,
			reason TEXT NOT NULL,
			source_type TEXT NOT NULL DEFAULT '',
			source_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			PRIMARY KEY(owner_public_key, id)
		)`,
		`CREATE TABLE IF NOT EXISTS gameplay_badges (
			owner_public_key TEXT NOT NULL,
			id TEXT NOT NULL,
			badge_def_id TEXT NOT NULL,
			exp INTEGER NOT NULL,
			level INTEGER NOT NULL,
			active INTEGER NOT NULL,
			progress INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(owner_public_key, id)
		)`,
		`CREATE TABLE IF NOT EXISTS gameplay_game_results (
			owner_public_key TEXT NOT NULL,
			id TEXT NOT NULL,
			runtime_profile_name TEXT NOT NULL,
			pet_id TEXT NOT NULL,
			game_def_id TEXT NOT NULL,
			score INTEGER,
			max_score INTEGER,
			difficulty TEXT,
			outcome TEXT,
			duration_ms INTEGER,
			idempotency_key TEXT,
			payload_json TEXT,
			occurred_at TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			PRIMARY KEY(owner_public_key, id)
		)`,
		`CREATE TABLE IF NOT EXISTS gameplay_reward_grants (
			owner_public_key TEXT NOT NULL,
			id TEXT NOT NULL,
			runtime_profile_name TEXT NOT NULL,
			pet_id TEXT,
			game_result_id TEXT,
			points_delta INTEGER NOT NULL,
			pet_exp_delta INTEGER NOT NULL,
			badge_exp_delta_json TEXT NOT NULL,
			source_type TEXT NOT NULL DEFAULT '',
			source_id TEXT NOT NULL DEFAULT '',
			reason TEXT,
			created_at TEXT NOT NULL,
			PRIMARY KEY(owner_public_key, id)
		)`,
		`CREATE TABLE IF NOT EXISTS gameplay_pending_deletions (
			deletion_id TEXT NOT NULL PRIMARY KEY,
			kind TEXT NOT NULL,
			owner_public_key TEXT NOT NULL,
			resource_id TEXT NOT NULL,
			reason TEXT NOT NULL,
			deleted_at TEXT NOT NULL,
			descriptor_version INTEGER NOT NULL,
			descriptor_json TEXT NOT NULL
		)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if _, err := db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS gameplay_game_results_idempotency_idx ON gameplay_game_results(owner_public_key, runtime_profile_name, idempotency_key) WHERE idempotency_key IS NOT NULL AND idempotency_key <> ''`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS gameplay_reward_grants_source_idx ON gameplay_reward_grants(owner_public_key, runtime_profile_name, source_type, source_id) WHERE source_id <> ''`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS gameplay_points_transactions_pet_adoption_idx ON gameplay_points_transactions(owner_public_key, source_id) WHERE source_type = 'pet' AND reason = 'pet.adopt'`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS gameplay_pending_deletions_locator_idx ON gameplay_pending_deletions(kind, owner_public_key, resource_id, deletion_id)`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS gameplay_pet_workspace_bindings_owner_idx ON gameplay_pet_workspace_bindings(owner_public_key, runtime_profile_name, workspace_name)`); err != nil {
		return err
	}
	return nil
}

func (r *Runtime) AdoptPet(ctx context.Context, owner string, req apitypes.PetAdoptRequest) (apitypes.PetAdoptResponse, error) {
	if err := requireOwner(owner); err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	if err := r.Migration(ctx); err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	if req.Id == nil {
		ruleset, err := r.resolveProfileRules(ctx, "")
		if err != nil {
			return apitypes.PetAdoptResponse{}, err
		}
		petID := r.newID()
		response, workspaceCreated, err := r.createPetAdoption(ctx, owner, req, ruleset, petID, "pet-"+petID, false)
		if err != nil && workspaceCreated && r.Workspaces != nil {
			_, _ = r.Workspaces.DeleteSystemWorkspace(context.WithoutCancel(ctx), "pet-"+petID)
		}
		return response, err
	}

	petID := *req.Id
	if err := customid.ValidateField("id", petID); err != nil {
		return apitypes.PetAdoptResponse{}, fmt.Errorf("%w: %w", ErrInvalidPetID, err)
	}
	profileRules, err := pointsRulesFromContext(ctx, "")
	if err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	mu := r.adoptionMutex(owner + "\x00" + petID)
	mu.Lock()
	defer mu.Unlock()
	if response, found, err := r.completedAdoptionResponse(ctx, owner, profileRules.Name, petID); found || err != nil {
		return response, err
	}
	ruleset, err := r.resolveProfileRules(ctx, profileRules.Name)
	if err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	reservation, reservationCreated, err := r.reservePetAdoption(ctx, owner, req, ruleset, petID)
	if err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	if reservation.RuntimeProfileName != ruleset.Name {
		return apitypes.PetAdoptResponse{}, fmt.Errorf("%w: %q belongs to RuntimeProfile %q", ErrPetIDConflict, petID, reservation.RuntimeProfileName)
	}
	response, createErr := r.createReservedPetAdoption(ctx, reservation, ruleset)
	if createErr == nil {
		return response, nil
	}
	recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	response, found, recoveryErr := r.awaitCompletedAdoptionResponse(recoveryCtx, owner, profileRules.Name, petID)
	cancel()
	if found {
		return response, recoveryErr
	}
	if recoveryErr != nil {
		return apitypes.PetAdoptResponse{}, recoveryErr
	}
	if reservationCreated && errors.Is(createErr, errInsufficientPoints) {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cleanupCancel()
		if _, err := deletePetAdoptionReservationIfIncomplete(cleanupCtx, r.DB, owner, petID); err != nil {
			return apitypes.PetAdoptResponse{}, errors.Join(createErr, fmt.Errorf("release failed adoption reservation: %w", err))
		}
	}
	return apitypes.PetAdoptResponse{}, createErr
}

func (r *Runtime) reservePetAdoption(ctx context.Context, owner string, req apitypes.PetAdoptRequest, ruleset ProfileRules, petID string) (petAdoptionReservation, bool, error) {
	db, err := r.db()
	if err != nil {
		return petAdoptionReservation{}, false, err
	}
	reserved, err := findPetAdoptionReservation(ctx, db, owner, petID)
	if err == nil {
		return reserved, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return petAdoptionReservation{}, false, err
	}
	poolEntry, err := r.pickPetDef(ruleset.Spec.PetPool)
	if err != nil {
		return petAdoptionReservation{}, false, err
	}
	petDef, err := r.Catalog.GetPetDefByID(ctx, poolEntry.PetDefID)
	if err != nil {
		return petAdoptionReservation{}, false, err
	}
	displayName := strings.TrimSpace(valueOrZero(req.DisplayName))
	if displayName == "" {
		displayName = petDefDisplayName(petDef)
	}
	reservation := petAdoptionReservation{
		OwnerPublicKey: owner, PetID: petID, RuntimeProfileName: ruleset.Name,
		PetDefID: petDef.Id, DisplayName: displayName, WorkspaceName: petWorkspaceName(owner, petID),
		WorkflowName: defaultPetWorkflowName, VoiceAlias: poolEntry.VoiceAlias,
		AdoptionCost: int64Value(poolEntry.AdoptionCost), CreatedAt: r.now(),
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return petAdoptionReservation{}, false, err
	}
	defer tx.Rollback()
	inserted, err := insertPetAdoptionReservationIfAbsent(ctx, tx, reservation)
	if err != nil {
		return petAdoptionReservation{}, false, err
	}
	reserved, err = findPetAdoptionReservation(ctx, tx, owner, petID)
	if err != nil {
		return petAdoptionReservation{}, false, err
	}
	if reserved.RuntimeProfileName != ruleset.Name {
		return reserved, inserted, nil
	}
	if err := r.preflightPetAdoptionTx(ctx, tx, owner, ruleset, reserved.AdoptionCost); err != nil {
		return petAdoptionReservation{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return petAdoptionReservation{}, false, err
	}
	return reserved, inserted, nil
}

func (r *Runtime) createReservedPetAdoption(ctx context.Context, reservation petAdoptionReservation, ruleset ProfileRules) (apitypes.PetAdoptResponse, error) {
	db, err := r.db()
	if err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	defer tx.Rollback()
	account, err := r.ensureAccountTx(ctx, tx, reservation.OwnerPublicKey, ruleset)
	if err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	if err := lockPointsAccountTx(ctx, tx, &account); err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	if account.Balance < reservation.AdoptionCost {
		return apitypes.PetAdoptResponse{}, errInsufficientPoints
	}
	if _, err := r.createPetWorkspace(ctx, reservation.WorkspaceName, reservation.WorkflowName, reservation.VoiceAlias); err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	now := r.now()
	pet := apitypes.Pet{
		Id: reservation.PetID, OwnerPublicKey: reservation.OwnerPublicKey,
		RuntimeProfileName: reservation.RuntimeProfileName, PetdefId: reservation.PetDefID,
		DisplayName: reservation.DisplayName, WorkspaceName: reservation.WorkspaceName,
		Stats: initialPetStats(), Progression: initialPetProgression(), Lifecycle: apitypes.PetLifecycleAlive,
		StateSettledAt: now, LastActiveAt: now, CreatedAt: now, UpdatedAt: now,
	}
	txn, err := r.recordPointsTx(ctx, tx, &account, -reservation.AdoptionCost, ruleset.Name, pet.Id, "", "", "pet.adopt", "pet", pet.Id, true)
	if err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	if err := insertPet(ctx, tx, pet); err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	if err := tx.Commit(); err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	return apitypes.PetAdoptResponse{Pet: pet, Points: account, Transaction: txn}, nil
}

func (r *Runtime) preflightPetAdoptionTx(ctx context.Context, tx *sqlx.Tx, owner string, ruleset ProfileRules, adoptionCost int64) error {
	account, err := r.ensureAccountTx(ctx, tx, owner, ruleset)
	if err != nil {
		return err
	}
	if account.Balance < adoptionCost {
		return errInsufficientPoints
	}
	return nil
}

func (r *Runtime) createPetAdoption(ctx context.Context, owner string, req apitypes.PetAdoptRequest, ruleset ProfileRules, petID, workspaceName string, acceptExistingWorkspace bool) (apitypes.PetAdoptResponse, bool, error) {
	poolEntry, err := r.pickPetDef(ruleset.Spec.PetPool)
	if err != nil {
		return apitypes.PetAdoptResponse{}, false, err
	}
	petDef, err := r.Catalog.GetPetDefByID(ctx, poolEntry.PetDefID)
	if err != nil {
		return apitypes.PetAdoptResponse{}, false, err
	}
	workflowName := defaultPetWorkflowName
	displayName := strings.TrimSpace(valueOrZero(req.DisplayName))
	if displayName == "" {
		displayName = petDefDisplayName(petDef)
	}
	workspaceCreated, err := r.createPetWorkspace(ctx, workspaceName, workflowName, poolEntry.VoiceAlias)
	if err != nil {
		return apitypes.PetAdoptResponse{}, false, err
	}
	if !workspaceCreated && !acceptExistingWorkspace {
		return apitypes.PetAdoptResponse{}, false, fmt.Errorf("create pet workspace %q: workspace already exists", workspaceName)
	}
	now := r.now()
	pet := apitypes.Pet{
		Id:                 petID,
		OwnerPublicKey:     owner,
		RuntimeProfileName: ruleset.Name,
		PetdefId:           petDef.Id,
		DisplayName:        displayName,
		WorkspaceName:      workspaceName,
		Stats:              initialPetStats(),
		Progression:        initialPetProgression(),
		Lifecycle:          apitypes.PetLifecycleAlive,
		StateSettledAt:     now,
		LastActiveAt:       now,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	response, err := r.commitPetAdoption(ctx, pet, ruleset, int64Value(poolEntry.AdoptionCost))
	return response, workspaceCreated, err
}

func (r *Runtime) commitPetAdoption(ctx context.Context, pet apitypes.Pet, ruleset ProfileRules, adoptionCost int64) (apitypes.PetAdoptResponse, error) {
	db, err := r.db()
	if err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	defer tx.Rollback()
	account, err := r.ensureAccountTx(ctx, tx, pet.OwnerPublicKey, ruleset)
	if err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	txn, err := r.recordPointsTx(ctx, tx, &account, -adoptionCost, ruleset.Name, pet.Id, "", "", "pet.adopt", "pet", pet.Id, true)
	if err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	if err := insertPet(ctx, tx, pet); err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	if err := tx.Commit(); err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	return apitypes.PetAdoptResponse{Pet: pet, Points: account, Transaction: txn}, nil
}

func (r *Runtime) awaitCompletedAdoptionResponse(ctx context.Context, owner, runtimeProfileName, petID string) (apitypes.PetAdoptResponse, bool, error) {
	for {
		response, found, err := r.completedAdoptionResponse(ctx, owner, runtimeProfileName, petID)
		if err != nil && ctx.Err() != nil {
			return apitypes.PetAdoptResponse{}, false, nil
		}
		if found || err != nil {
			return response, found, err
		}
		timer := time.NewTimer(20 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return apitypes.PetAdoptResponse{}, false, nil
		case <-timer.C:
		}
	}
}

func (r *Runtime) completedAdoptionResponse(ctx context.Context, owner, runtimeProfileName, petID string) (apitypes.PetAdoptResponse, bool, error) {
	db, err := r.db()
	if err != nil {
		return apitypes.PetAdoptResponse{}, false, err
	}
	txOptions := &sql.TxOptions{ReadOnly: true}
	if db.DriverName() == "postgres" {
		txOptions.Isolation = sql.LevelRepeatableRead
	}
	readTx, err := db.BeginTxx(ctx, txOptions)
	if err != nil {
		return apitypes.PetAdoptResponse{}, false, err
	}
	defer readTx.Rollback()
	pet, petErr := findPetByOwnerID(ctx, readTx, owner, petID)
	txn, txnErr := findPetAdoptionTransaction(ctx, readTx, owner, petID)
	if errors.Is(petErr, sql.ErrNoRows) {
		if txnErr == nil {
			return apitypes.PetAdoptResponse{}, true, fmt.Errorf("%w: %q belonged to a deleted Pet", ErrPetIDConflict, petID)
		}
		if errors.Is(txnErr, sql.ErrNoRows) {
			return apitypes.PetAdoptResponse{}, false, nil
		}
		return apitypes.PetAdoptResponse{}, false, txnErr
	}
	if petErr != nil {
		return apitypes.PetAdoptResponse{}, false, petErr
	}
	if pet.RuntimeProfileName != runtimeProfileName {
		return apitypes.PetAdoptResponse{}, true, fmt.Errorf("%w: %q belongs to RuntimeProfile %q", ErrPetIDConflict, petID, pet.RuntimeProfileName)
	}
	if txnErr != nil {
		return apitypes.PetAdoptResponse{}, true, fmt.Errorf("load adoption transaction for Pet %q: %w", petID, txnErr)
	}
	if txn.RuntimeProfileName != runtimeProfileName {
		return apitypes.PetAdoptResponse{}, true, fmt.Errorf("adoption transaction for Pet %q belongs to RuntimeProfile %q", petID, txn.RuntimeProfileName)
	}
	account, err := findPointsAccount(ctx, readTx, owner, runtimeProfileName)
	if err != nil {
		return apitypes.PetAdoptResponse{}, true, fmt.Errorf("load points account for Pet %q: %w", petID, err)
	}
	return apitypes.PetAdoptResponse{Pet: pet, Points: account, Transaction: txn}, true, nil
}

func petWorkspaceName(owner, petID string) string {
	digest := sha256.Sum256([]byte(owner + "\x00" + petID))
	return fmt.Sprintf("pet-%x", digest[:20])
}

func (r *Runtime) ListPets(ctx context.Context, owner string, req apitypes.GameplayListRequest) (apitypes.PetListResponse, error) {
	items, hasNext, next, err := listOwnerRows(ctx, r, owner, "gameplay_pets", true, req, scanPet)
	return apitypes.PetListResponse{Items: items, HasNext: hasNext, NextCursor: next}, err
}

func (r *Runtime) GetPet(ctx context.Context, owner, id string) (apitypes.Pet, error) {
	db, err := r.db()
	if err != nil {
		return apitypes.Pet{}, err
	}
	query, args := profileScopedOwnerIDQuery(ctx, petSelectSQL(), owner, id)
	return scanPet(db.QueryRowContext(ctx, db.Rebind(query), args...))
}

func profileScopedOwnerIDQuery(ctx context.Context, selectSQL, owner, id string) (string, []any) {
	query := selectSQL + ` WHERE owner_public_key = ? AND id = ?`
	args := []any{strings.TrimSpace(owner), strings.TrimSpace(id)}
	if profile, ok := runtimeProfileFromContext(ctx); ok {
		if profileName := strings.TrimSpace(profile.Name); profileName != "" {
			query += ` AND runtime_profile_name = ?`
			args = append(args, profileName)
		}
	}
	return query, args
}

// ResolvePetContext resolves the one adopted pet bound to a Workspace and its
// PetDef. Missing and ambiguous bindings are rejected because the Workspace
// name is the Pet runtime identity.
func (r *Runtime) ResolvePetContext(ctx context.Context, workspaceName string) (apitypes.Pet, apitypes.PetDef, error) {
	db, err := r.db()
	if err != nil {
		return apitypes.Pet{}, apitypes.PetDef{}, err
	}
	workspaceName = strings.TrimSpace(workspaceName)
	if workspaceName == "" {
		return apitypes.Pet{}, apitypes.PetDef{}, errors.New("gameplay: workspace name is required")
	}
	rows, err := db.QueryContext(ctx, db.Rebind(petSelectSQL()+` WHERE workspace_name = ? LIMIT 2`), workspaceName)
	if err != nil {
		return apitypes.Pet{}, apitypes.PetDef{}, err
	}
	defer rows.Close()
	pets := make([]apitypes.Pet, 0, 2)
	for rows.Next() {
		pet, err := scanPet(rows)
		if err != nil {
			return apitypes.Pet{}, apitypes.PetDef{}, err
		}
		pets = append(pets, pet)
	}
	if err := rows.Err(); err != nil {
		return apitypes.Pet{}, apitypes.PetDef{}, err
	}
	if len(pets) == 0 {
		return apitypes.Pet{}, apitypes.PetDef{}, fmt.Errorf("%w for workspace %q: %w", errPetWorkspaceNotFound, workspaceName, sql.ErrNoRows)
	}
	if len(pets) > 1 {
		return apitypes.Pet{}, apitypes.PetDef{}, fmt.Errorf("%w for workspace %q", errPetWorkspaceAmbiguous, workspaceName)
	}
	if r.Catalog == nil {
		return apitypes.Pet{}, apitypes.PetDef{}, errors.New("gameplay: catalog is not configured")
	}
	petDef, err := r.Catalog.GetPetDefByID(ctx, pets[0].PetdefId)
	if err != nil {
		return apitypes.Pet{}, apitypes.PetDef{}, err
	}
	return pets[0], petDef, nil
}

func (r *Runtime) OwnerHasPetDef(ctx context.Context, owner, petDefID string) (bool, error) {
	db, err := r.db()
	if err != nil {
		return false, err
	}
	var exists int
	err = db.QueryRowContext(ctx, db.Rebind(`SELECT 1 FROM gameplay_pets WHERE owner_public_key = ? AND petdef_id = ? LIMIT 1`), owner, strings.TrimSpace(petDefID)).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// ListPetWorkspaceNames returns every Pet Workspace retained for the owner
// under the active RuntimeProfile, including Workspaces whose Pet was deleted.
func (r *Runtime) ListPetWorkspaceNames(ctx context.Context, owner string) ([]string, error) {
	if r == nil || r.DB == nil {
		return nil, nil
	}
	profile, ok := runtimeProfileFromContext(ctx)
	profileName := strings.TrimSpace(profile.Name)
	if !ok || profileName == "" {
		return nil, nil
	}
	owner = strings.TrimSpace(owner)
	rows, err := r.DB.QueryContext(ctx, r.DB.Rebind(`SELECT workspace_name FROM gameplay_pet_workspace_bindings WHERE owner_public_key = ? AND runtime_profile_name = ?
		UNION SELECT workspace_name FROM gameplay_pets WHERE owner_public_key = ? AND runtime_profile_name = ?
		ORDER BY workspace_name`), owner, profileName, owner, profileName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// OwnerHasPetWorkspace reports whether the Workspace is retained for one of
// the caller's adopted pets under the active RuntimeProfile. Pet Workspaces are
// system-managed, so this durable domain relationship supplies access after
// Pet deletion without crossing RuntimeProfile boundaries.
func (r *Runtime) OwnerHasPetWorkspace(ctx context.Context, owner, workspaceName string) (bool, error) {
	if r == nil || r.DB == nil {
		return false, nil
	}
	profile, ok := runtimeProfileFromContext(ctx)
	profileName := strings.TrimSpace(profile.Name)
	if !ok || profileName == "" {
		return false, nil
	}
	var exists int
	owner = strings.TrimSpace(owner)
	workspaceName = strings.TrimSpace(workspaceName)
	err := r.DB.QueryRowContext(ctx, r.DB.Rebind(`SELECT 1 FROM gameplay_pet_workspace_bindings WHERE owner_public_key = ? AND runtime_profile_name = ? AND workspace_name = ?
		UNION SELECT 1 FROM gameplay_pets WHERE owner_public_key = ? AND runtime_profile_name = ? AND workspace_name = ?
		LIMIT 1`), owner, profileName, workspaceName, owner, profileName, workspaceName).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (r *Runtime) PutPet(ctx context.Context, owner string, req apitypes.PetPutRequest) (apitypes.Pet, error) {
	pet, err := r.GetPet(ctx, owner, req.Id)
	if err != nil {
		return apitypes.Pet{}, err
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		return apitypes.Pet{}, errors.New("display_name is required")
	}
	pet.DisplayName = displayName
	pet.UpdatedAt = r.now()
	db, err := r.db()
	if err != nil {
		return apitypes.Pet{}, err
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`UPDATE gameplay_pets SET display_name = ?, updated_at = ? WHERE owner_public_key = ? AND id = ? AND runtime_profile_name = ?`), pet.DisplayName, formatTime(pet.UpdatedAt), owner, pet.Id, pet.RuntimeProfileName); err != nil {
		return apitypes.Pet{}, err
	}
	return pet, nil
}

func (r *Runtime) DeletePet(ctx context.Context, owner, id string) (apitypes.Pet, error) {
	db, err := r.db()
	if err != nil {
		return apitypes.Pet{}, err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return apitypes.Pet{}, err
	}
	defer tx.Rollback()
	query, args := profileScopedOwnerIDQuery(ctx, petSelectSQL(), owner, id)
	pet, err := scanPet(tx.QueryRowContext(ctx, db.Rebind(query), args...))
	if err != nil {
		return apitypes.Pet{}, err
	}
	descriptor := struct {
		OwnerPublicKey string `json:"owner_public_key"`
		PetID          string `json:"pet_id"`
		RuntimeProfile string `json:"runtime_profile_name"`
		PetDefID       string `json:"petdef_id"`
		WorkspaceName  string `json:"workspace_name"`
	}{
		OwnerPublicKey: pet.OwnerPublicKey,
		PetID:          pet.Id,
		RuntimeProfile: pet.RuntimeProfileName,
		PetDefID:       pet.PetdefId,
		WorkspaceName:  pet.WorkspaceName,
	}
	record, err := pendingdeletion.New(pendingdeletion.KindPet, pet.Id, &pet.OwnerPublicKey, pendingdeletion.ReasonResourceDelete, descriptor, r.now())
	if err != nil {
		return apitypes.Pet{}, err
	}
	if err := ensurePetWorkspaceBinding(ctx, tx, pet); err != nil {
		return apitypes.Pet{}, fmt.Errorf("delete pet %q Workspace binding: %w", pet.Id, err)
	}
	if _, err := tx.ExecContext(ctx, db.Rebind(`INSERT INTO gameplay_pending_deletions (deletion_id, kind, owner_public_key, resource_id, reason, deleted_at, descriptor_version, descriptor_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`), record.DeletionID, record.Kind, pet.OwnerPublicKey, record.ResourceID, record.Reason, formatTime(record.DeletedAt), record.DescriptorVersion, string(record.Descriptor)); err != nil {
		return apitypes.Pet{}, fmt.Errorf("delete pet %q pending deletion: %w", pet.Id, err)
	}
	result, err := tx.ExecContext(ctx, db.Rebind(`DELETE FROM gameplay_pets WHERE owner_public_key = ? AND id = ? AND runtime_profile_name = ?`), pet.OwnerPublicKey, pet.Id, pet.RuntimeProfileName)
	if err != nil {
		return apitypes.Pet{}, fmt.Errorf("delete pet %q row: %w", pet.Id, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return apitypes.Pet{}, fmt.Errorf("delete pet %q rows affected: %w", pet.Id, err)
	}
	if affected != 1 {
		return apitypes.Pet{}, sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return apitypes.Pet{}, fmt.Errorf("delete pet %q commit: %w", pet.Id, err)
	}
	return pet, nil
}

func (r *Runtime) DrivePet(ctx context.Context, owner string, req apitypes.PetDriveRequest) (apitypes.PetDriveResponse, error) {
	if err := r.Migration(ctx); err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	mu := r.driveMutex(owner + "\x00" + req.PetId)
	mu.Lock()
	defer mu.Unlock()
	pet, err := r.GetPet(ctx, owner, req.PetId)
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	ruleset, err := r.resolveProfileRules(ctx, pet.RuntimeProfileName)
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	hasBehavior := req.Behavior != nil
	hasGame := req.GameResult != nil
	if hasBehavior && hasGame {
		return apitypes.PetDriveResponse{}, errors.New("gameplay: exactly one behavior or game_result is required")
	}
	if !hasBehavior && !hasGame {
		key := strings.TrimSpace(valueOrZero(req.IdempotencyKey))
		if key != "" {
			if response, found, err := r.completedEmptyDriveResponse(ctx, owner, ruleset.Name, key, pet); err != nil {
				return apitypes.PetDriveResponse{}, err
			} else if found {
				return response, nil
			}
		}
		return r.commitEmptyDrive(ctx, owner, req.PetId, key, ruleset)
	}
	if pet.Lifecycle == apitypes.PetLifecycleDead {
		return apitypes.PetDriveResponse{}, ErrPetDead
	}
	var behavior apitypes.PetBehavior
	var actionPolicy apitypes.RuntimeProfilePetActionSpec
	if hasBehavior {
		behavior = *req.Behavior
		var exists bool
		actionPolicy, exists = ruleset.Spec.Actions[behavior]
		if !exists || !behavior.Valid() {
			return apitypes.PetDriveResponse{}, fmt.Errorf("gameplay: unsupported pet behavior %q", behavior)
		}
	}
	var gameRule ProfileGameRule
	if hasGame {
		gameDefID := strings.TrimSpace(req.GameResult.GameDefId)
		var configured bool
		gameRule, configured = ruleset.Spec.Games[gameDefID]
		if !configured {
			return r.ignoredGameResponse(ctx, owner, pet)
		}
		if err := r.validateConfiguredGame(ctx, gameRule); err != nil {
			return apitypes.PetDriveResponse{}, err
		}
		if key := strings.TrimSpace(valueOrZero(req.GameResult.IdempotencyKey)); key != "" {
			if response, found, err := r.completedGameResponse(ctx, owner, ruleset.Name, key, pet.Id, gameRule.GameDefID); err != nil {
				return apitypes.PetDriveResponse{}, err
			} else if found {
				return response, nil
			}
		}
	}
	if hasBehavior {
		if key := strings.TrimSpace(valueOrZero(req.IdempotencyKey)); key != "" {
			if response, found, err := r.completedBehaviorResponse(ctx, owner, ruleset.Name, key, pet, behavior); err != nil {
				return apitypes.PetDriveResponse{}, err
			} else if found {
				return response, nil
			}
		}
	}

	var evaluated apitypes.GameRewardSpec
	if hasGame {
		account, err := r.readPointsAccount(ctx, owner, ruleset.Name)
		if err != nil {
			return apitypes.PetDriveResponse{}, err
		}
		preview := pet
		settlePetTime(&preview, r.now(), ruleset.Spec.Time)
		if preview.Stats.Life > 0 && preview.Stats.Energy >= float64(gameRule.Policy.EnergyCost) && account.Balance >= gameRule.Policy.PointsCost {
			evaluator := rewardEvaluatorFromContext(ctx)
			if evaluator == nil {
				return apitypes.PetDriveResponse{}, errors.New("gameplay: reward evaluator is not configured")
			}
			request, err := r.rewardEvaluationRequest(ctx, gameRule, req.GameResult, owner, pet, ruleset)
			if err != nil {
				return apitypes.PetDriveResponse{}, err
			}
			evaluated, err = evaluator.Evaluate(ctx, request)
			if err != nil {
				return apitypes.PetDriveResponse{}, err
			}
			if err := validateGameReward(evaluated, gameRule, ruleset.Spec.BadgeDefs); err != nil {
				return apitypes.PetDriveResponse{}, err
			}
		}
	}
	return r.commitDrive(ctx, owner, req, ruleset, behavior, actionPolicy, gameRule, evaluated)
}

func (r *Runtime) commitEmptyDrive(ctx context.Context, owner, petID, key string, ruleset ProfileRules) (apitypes.PetDriveResponse, error) {
	db, err := r.db()
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	defer tx.Rollback()
	now := r.now()
	replay := false
	if key != "" {
		inserted, err := insertPetDriveTick(ctx, tx, petDriveTick{
			OwnerPublicKey: owner, RuntimeProfileName: ruleset.Name,
			IdempotencyKey: key, PetID: petID, CreatedAt: now,
		})
		if err != nil {
			return apitypes.PetDriveResponse{}, err
		}
		if !inserted {
			completed, err := findPetDriveTick(ctx, tx, owner, ruleset.Name, key)
			if err != nil {
				return apitypes.PetDriveResponse{}, err
			}
			if completed.PetID != petID {
				return apitypes.PetDriveResponse{}, errors.New("gameplay: idempotency key belongs to another pet")
			}
			replay = true
		}
	}
	pet, err := scanPet(tx.QueryRowContext(ctx, tx.Rebind(petSelectSQL()+` WHERE owner_public_key = ? AND id = ?`), owner, petID))
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	account, err := r.ensureAccountTx(ctx, tx, owner, ruleset)
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	if replay {
		return emptyDriveResponse(pet, account), nil
	}
	if pet.Lifecycle != apitypes.PetLifecycleDead {
		settlePetTime(&pet, now, ruleset.Spec.Time)
		pet.UpdatedAt = pet.StateSettledAt
		if pet.Stats.Life == 0 {
			pet.Lifecycle = apitypes.PetLifecycleDead
			diedAt := pet.StateSettledAt
			pet.DiedAt = &diedAt
		}
		if err := updatePet(ctx, tx, pet); err != nil {
			return apitypes.PetDriveResponse{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	return emptyDriveResponse(pet, account), nil
}

func (r *Runtime) commitDrive(
	ctx context.Context,
	owner string,
	req apitypes.PetDriveRequest,
	ruleset ProfileRules,
	behavior apitypes.PetBehavior,
	actionPolicy apitypes.RuntimeProfilePetActionSpec,
	gameRule ProfileGameRule,
	evaluated apitypes.GameRewardSpec,
) (apitypes.PetDriveResponse, error) {
	db, err := r.db()
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	defer tx.Rollback()
	pet, err := scanPet(tx.QueryRowContext(ctx, tx.Rebind(petSelectSQL()+` WHERE owner_public_key = ? AND id = ?`), owner, req.PetId))
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	if pet.Lifecycle == apitypes.PetLifecycleDead {
		return apitypes.PetDriveResponse{}, ErrPetDead
	}
	account, err := r.ensureAccountTx(ctx, tx, owner, ruleset)
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	now := r.now()
	settlePetTime(&pet, now, ruleset.Spec.Time)
	pet.UpdatedAt = pet.StateSettledAt
	if pet.Stats.Life == 0 {
		pet.Lifecycle = apitypes.PetLifecycleDead
		diedAt := pet.StateSettledAt
		pet.DiedAt = &diedAt
		if err := updatePet(ctx, tx, pet); err != nil {
			return apitypes.PetDriveResponse{}, err
		}
		if err := tx.Commit(); err != nil {
			return apitypes.PetDriveResponse{}, err
		}
		return emptyDriveResponse(pet, account), nil
	}

	energyCost := actionPolicy.EnergyCost
	pointsCost := int64(0)
	if req.GameResult != nil {
		energyCost = gameRule.Policy.EnergyCost
		pointsCost = gameRule.Policy.PointsCost
	}
	if pet.Stats.Energy < float64(energyCost) || account.Balance < pointsCost {
		if err := updatePet(ctx, tx, pet); err != nil {
			return apitypes.PetDriveResponse{}, err
		}
		if err := tx.Commit(); err != nil {
			return apitypes.PetDriveResponse{}, err
		}
		if pet.Stats.Energy < float64(energyCost) {
			return apitypes.PetDriveResponse{}, errors.New("gameplay: insufficient energy")
		}
		return apitypes.PetDriveResponse{}, errors.New("gameplay: insufficient points")
	}

	pet.Stats.Energy = clampPetStat(pet.Stats.Energy - float64(energyCost))
	transactions := []apitypes.PointsTransaction{}
	badges := []apitypes.Badge{}
	grants := []apitypes.RewardGrant{}
	var result *apitypes.GameResult
	var grant apitypes.RewardGrant
	if req.GameResult != nil {
		occurredAt := now
		if req.GameResult.OccurredAt != nil {
			occurredAt = req.GameResult.OccurredAt.UTC()
		}
		gameResult := apitypes.GameResult{
			Id: r.newID(), OwnerPublicKey: owner, RuntimeProfileName: ruleset.Name,
			PetId: pet.Id, GameDefId: gameRule.GameDefID, Score: req.GameResult.Score,
			MaxScore: req.GameResult.MaxScore, Difficulty: req.GameResult.Difficulty,
			Outcome: req.GameResult.Outcome, DurationMs: req.GameResult.DurationMs,
			IdempotencyKey: req.GameResult.IdempotencyKey, Payload: req.GameResult.Payload,
			OccurredAt: occurredAt, CreatedAt: now,
		}
		if err := insertGameResult(ctx, tx, gameResult); err != nil {
			return apitypes.PetDriveResponse{}, err
		}
		result = &gameResult
		if pointsCost > 0 {
			transaction, err := r.applyPointsTx(ctx, tx, &account, -pointsCost, ruleset.Name, pet.Id, gameResult.Id, "", "game.play", "game_result", gameResult.Id)
			if err != nil {
				return apitypes.PetDriveResponse{}, err
			}
			transactions = append(transactions, transaction)
		}
		badgeDelta := make(map[string]int64, len(evaluated.BadgeExpDelta))
		for alias, delta := range evaluated.BadgeExpDelta {
			if delta == 0 {
				continue
			}
			badgeDelta[ruleset.Spec.BadgeDefs[alias]] = delta
		}
		grant = apitypes.RewardGrant{
			Id: r.newID(), OwnerPublicKey: owner, RuntimeProfileName: ruleset.Name,
			PetId: &pet.Id, GameResultId: &gameResult.Id, PetExpDelta: evaluated.PetExpDelta,
			BadgeExpDelta: badgeDelta, SourceType: "game_result", SourceId: gameResult.Id,
			Reason: stringPtr(strings.TrimSpace(evaluated.Reason)), CreatedAt: now,
		}
	} else {
		applyCareBehavior(&pet, behavior, actionPolicy.StatDelta)
		expDelta := energyCost / ruleset.Spec.Experience.EnergyPerPetExp
		sourceID := strings.TrimSpace(valueOrZero(req.IdempotencyKey))
		grantID := r.newID()
		if sourceID == "" {
			sourceID = grantID
		}
		grant = apitypes.RewardGrant{
			Id: grantID, OwnerPublicKey: owner, RuntimeProfileName: ruleset.Name,
			PetId: &pet.Id, PetExpDelta: expDelta, BadgeExpDelta: map[string]int64{},
			SourceType: "pet_behavior", SourceId: sourceID,
			Reason: stringPtr("behavior." + string(behavior)), CreatedAt: now,
		}
	}
	applyPetExp(&pet, grant.PetExpDelta, ruleset.Spec.Experience.Leveling)
	if err := insertRewardGrant(ctx, tx, grant); err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	grants = append(grants, grant)
	for badgeDefID, delta := range grant.BadgeExpDelta {
		badge, err := r.applyBadgeExp(ctx, tx, owner, badgeDefID, delta, now)
		if err != nil {
			return apitypes.PetDriveResponse{}, err
		}
		badges = append(badges, badge)
	}
	pet.LastActiveAt = now
	pet.UpdatedAt = now
	if err := updatePet(ctx, tx, pet); err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	if err := tx.Commit(); err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	return apitypes.PetDriveResponse{Pet: pet, Points: account, GameResult: result, Badges: badges, RewardGrants: grants, Transactions: transactions}, nil
}

func emptyDriveResponse(pet apitypes.Pet, account apitypes.PointsAccount) apitypes.PetDriveResponse {
	return apitypes.PetDriveResponse{
		Pet: pet, Points: account, Badges: []apitypes.Badge{},
		RewardGrants: []apitypes.RewardGrant{}, Transactions: []apitypes.PointsTransaction{},
	}
}

func (r *Runtime) ignoredGameResponse(ctx context.Context, owner string, pet apitypes.Pet) (apitypes.PetDriveResponse, error) {
	account, err := r.readPointsAccount(ctx, owner, pet.RuntimeProfileName)
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	return emptyDriveResponse(pet, account), nil
}

func (r *Runtime) readPointsAccount(ctx context.Context, owner, profile string) (apitypes.PointsAccount, error) {
	db, err := r.db()
	if err != nil {
		return apitypes.PointsAccount{}, err
	}
	return scanPointsAccount(db.QueryRowContext(ctx, db.Rebind(pointsAccountSelectSQL()+` WHERE owner_public_key = ? AND runtime_profile_name = ?`), owner, profile))
}

func (r *Runtime) validateConfiguredGame(ctx context.Context, rule ProfileGameRule) error {
	if strings.TrimSpace(rule.GameDefID) == "" {
		return errors.New("gameplay: game_def_id is required")
	}
	_, err := r.Catalog.GetGameDefByID(ctx, rule.GameDefID)
	return err
}

func (r *Runtime) rewardEvaluationRequest(ctx context.Context, rule ProfileGameRule, input *apitypes.PetDriveGameResultInput, owner string, pet apitypes.Pet, ruleset ProfileRules) (RewardEvaluationRequest, error) {
	gameDef, err := r.Catalog.GetGameDefByID(ctx, rule.GameDefID)
	if err != nil {
		return RewardEvaluationRequest{}, err
	}
	badges := make([]BadgeRewardCriterion, 0, len(ruleset.Spec.BadgeDefs))
	aliases := make([]string, 0, len(ruleset.Spec.BadgeDefs))
	for alias := range ruleset.Spec.BadgeDefs {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		badgeDef, err := r.Catalog.GetBadgeDefByID(ctx, ruleset.Spec.BadgeDefs[alias])
		if err != nil {
			return RewardEvaluationRequest{}, err
		}
		criterion := BadgeRewardCriterion{Alias: alias, DisplayName: badgeDef.Spec.DisplayName, Metadata: badgeDef.Spec.Metadata}
		if badgeDef.Spec.Description != nil {
			criterion.Description = *badgeDef.Spec.Description
		}
		if badgeDef.Spec.Tags != nil {
			criterion.Tags = append([]string(nil), (*badgeDef.Spec.Tags)...)
		}
		badges = append(badges, criterion)
	}
	now := r.now()
	occurredAt := now
	if input.OccurredAt != nil {
		occurredAt = input.OccurredAt.UTC()
	}
	result := apitypes.GameResult{
		OwnerPublicKey: owner, RuntimeProfileName: ruleset.Name, PetId: pet.Id,
		GameDefId: rule.GameDefID, Score: input.Score, MaxScore: input.MaxScore,
		Difficulty: input.Difficulty, Outcome: input.Outcome, DurationMs: input.DurationMs,
		IdempotencyKey: input.IdempotencyKey, Payload: input.Payload, OccurredAt: occurredAt,
	}
	return RewardEvaluationRequest{
		ModelAlias: rule.Policy.Reward.Model, Prompt: rule.Policy.Reward.Prompt,
		GameDef: gameDef, GameResult: result, Badges: badges, PetExpMax: rule.Policy.Reward.PetExpMax,
		BadgeExpMaxPerBadge: rule.Policy.Reward.BadgeExpMaxPerBadge,
	}, nil
}

func (r *Runtime) completedGameResponse(ctx context.Context, owner, profile, key, petID, gameDefID string) (apitypes.PetDriveResponse, bool, error) {
	db, err := r.db()
	if err != nil {
		return apitypes.PetDriveResponse{}, false, err
	}
	result, err := scanGameResult(db.QueryRowContext(ctx, db.Rebind(gameResultSelectSQL()+` WHERE owner_public_key = ? AND runtime_profile_name = ? AND idempotency_key = ?`), owner, profile, strings.TrimSpace(key)))
	if errors.Is(err, sql.ErrNoRows) {
		return apitypes.PetDriveResponse{}, false, nil
	}
	if err != nil {
		return apitypes.PetDriveResponse{}, false, err
	}
	if result.PetId != petID || result.GameDefId != gameDefID {
		return apitypes.PetDriveResponse{}, false, errors.New("gameplay: idempotency key belongs to another game Drive")
	}
	response, err := r.completedDriveResponse(ctx, owner, profile, result.PetId, &result, "game_result", result.Id)
	return response, true, err
}

func (r *Runtime) completedEmptyDriveResponse(ctx context.Context, owner, profile, key string, pet apitypes.Pet) (apitypes.PetDriveResponse, bool, error) {
	db, err := r.db()
	if err != nil {
		return apitypes.PetDriveResponse{}, false, err
	}
	tick, err := findPetDriveTick(ctx, db, owner, profile, key)
	if errors.Is(err, sql.ErrNoRows) {
		return apitypes.PetDriveResponse{}, false, nil
	}
	if err != nil {
		return apitypes.PetDriveResponse{}, false, err
	}
	if tick.PetID != pet.Id {
		return apitypes.PetDriveResponse{}, false, errors.New("gameplay: idempotency key belongs to another pet")
	}
	account, err := r.readPointsAccount(ctx, owner, profile)
	if err != nil {
		return apitypes.PetDriveResponse{}, false, err
	}
	return emptyDriveResponse(pet, account), true, nil
}

func (r *Runtime) completedBehaviorResponse(ctx context.Context, owner, profile, key string, pet apitypes.Pet, behavior apitypes.PetBehavior) (apitypes.PetDriveResponse, bool, error) {
	db, err := r.db()
	if err != nil {
		return apitypes.PetDriveResponse{}, false, err
	}
	grant, err := scanRewardGrant(db.QueryRowContext(ctx, db.Rebind(rewardGrantSelectSQL()+` WHERE owner_public_key = ? AND runtime_profile_name = ? AND source_type = 'pet_behavior' AND source_id = ?`), owner, profile, strings.TrimSpace(key)))
	if errors.Is(err, sql.ErrNoRows) {
		return apitypes.PetDriveResponse{}, false, nil
	}
	if err != nil {
		return apitypes.PetDriveResponse{}, false, err
	}
	if grant.PetId == nil || *grant.PetId != pet.Id {
		return apitypes.PetDriveResponse{}, false, errors.New("gameplay: idempotency key belongs to another pet")
	}
	if grant.Reason == nil || *grant.Reason != "behavior."+string(behavior) {
		return apitypes.PetDriveResponse{}, false, errors.New("gameplay: idempotency key belongs to another behavior")
	}
	response, err := r.completedDriveResponse(ctx, owner, profile, pet.Id, nil, "pet_behavior", strings.TrimSpace(key))
	return response, true, err
}

func (r *Runtime) completedDriveResponse(ctx context.Context, owner, profile, petID string, result *apitypes.GameResult, sourceType, sourceID string) (apitypes.PetDriveResponse, error) {
	pet, err := r.GetPet(ctx, owner, petID)
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	account, err := r.readPointsAccount(ctx, owner, profile)
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	db, err := r.db()
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	grant, err := scanRewardGrant(db.QueryRowContext(ctx, db.Rebind(rewardGrantSelectSQL()+` WHERE owner_public_key = ? AND runtime_profile_name = ? AND source_type = ? AND source_id = ?`), owner, profile, sourceType, sourceID))
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	badges := make([]apitypes.Badge, 0, len(grant.BadgeExpDelta))
	badgeIDs := make([]string, 0, len(grant.BadgeExpDelta))
	for badgeID := range grant.BadgeExpDelta {
		badgeIDs = append(badgeIDs, badgeID)
	}
	sort.Strings(badgeIDs)
	for _, badgeID := range badgeIDs {
		badge, err := scanBadge(db.QueryRowContext(ctx, db.Rebind(badgeSelectSQL()+` WHERE owner_public_key = ? AND id = ?`), owner, badgeID))
		if err != nil {
			return apitypes.PetDriveResponse{}, err
		}
		badges = append(badges, badge)
	}
	transactions, err := listDriveTransactions(ctx, db, owner, result, grant.Id)
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	return apitypes.PetDriveResponse{
		Pet: pet, Points: account, GameResult: result, Badges: badges,
		RewardGrants: []apitypes.RewardGrant{grant}, Transactions: transactions,
	}, nil
}

func listDriveTransactions(ctx context.Context, db *sqlx.DB, owner string, result *apitypes.GameResult, grantID string) ([]apitypes.PointsTransaction, error) {
	query := pointsTransactionSelectSQL() + ` WHERE owner_public_key = ? AND reward_grant_id = ?`
	args := []any{owner, grantID}
	if result != nil {
		query = pointsTransactionSelectSQL() + ` WHERE owner_public_key = ? AND game_result_id = ?`
		args = []any{owner, result.Id}
	}
	rows, err := db.QueryContext(ctx, db.Rebind(query+` ORDER BY id`), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []apitypes.PointsTransaction{}
	for rows.Next() {
		item, err := scanPointsTransaction(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Runtime) GetPoints(ctx context.Context, owner, runtimeProfileName string) (apitypes.PointsAccount, error) {
	if err := r.Migration(ctx); err != nil {
		return apitypes.PointsAccount{}, err
	}
	if _, registered := runtimeProfileFromContext(ctx); !registered && strings.TrimSpace(runtimeProfileName) == "" {
		db, err := r.db()
		if err != nil {
			return apitypes.PointsAccount{}, err
		}
		return scanPointsAccount(db.QueryRowContext(ctx, db.Rebind(pointsAccountSelectSQL()+` WHERE owner_public_key = ? ORDER BY runtime_profile_name LIMIT 1`), strings.TrimSpace(owner)))
	}
	ruleset, err := pointsRulesFromContext(ctx, runtimeProfileName)
	if err != nil {
		return apitypes.PointsAccount{}, err
	}
	db, err := r.db()
	if err != nil {
		return apitypes.PointsAccount{}, err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return apitypes.PointsAccount{}, err
	}
	defer tx.Rollback()
	account, err := r.ensureAccountTx(ctx, tx, owner, ruleset)
	if err != nil {
		return apitypes.PointsAccount{}, err
	}
	return account, tx.Commit()
}

func (r *Runtime) ListPointsTransactions(ctx context.Context, owner string, req apitypes.GameplayListRequest) (apitypes.PointsTransactionListResponse, error) {
	items, hasNext, next, err := listOwnerRows(ctx, r, owner, "gameplay_points_transactions", true, req, scanPointsTransaction)
	return apitypes.PointsTransactionListResponse{Items: items, HasNext: hasNext, NextCursor: next}, err
}

func (r *Runtime) GetPointsTransaction(ctx context.Context, owner, id string) (apitypes.PointsTransaction, error) {
	db, err := r.db()
	if err != nil {
		return apitypes.PointsTransaction{}, err
	}
	query, args := profileScopedOwnerIDQuery(ctx, pointsTransactionSelectSQL(), owner, id)
	return scanPointsTransaction(db.QueryRowContext(ctx, db.Rebind(query), args...))
}

func (r *Runtime) ListBadges(ctx context.Context, owner string, req apitypes.GameplayListRequest) (apitypes.BadgeListResponse, error) {
	items, hasNext, next, err := listOwnerRows(ctx, r, owner, "gameplay_badges", false, req, scanBadge)
	return apitypes.BadgeListResponse{Items: items, HasNext: hasNext, NextCursor: next}, err
}

func (r *Runtime) GetBadge(ctx context.Context, owner, id string) (apitypes.Badge, error) {
	db, err := r.db()
	if err != nil {
		return apitypes.Badge{}, err
	}
	return scanBadge(db.QueryRowContext(ctx, db.Rebind(badgeSelectSQL()+` WHERE owner_public_key = ? AND id = ?`), owner, strings.TrimSpace(id)))
}

func (r *Runtime) OwnerHasBadgeDef(ctx context.Context, owner, badgeDefID string) (bool, error) {
	db, err := r.db()
	if err != nil {
		return false, err
	}
	var exists int
	err = db.QueryRowContext(ctx, db.Rebind(`SELECT 1 FROM gameplay_badges WHERE owner_public_key = ? AND badge_def_id = ? LIMIT 1`), owner, strings.TrimSpace(badgeDefID)).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (r *Runtime) ListGameResults(ctx context.Context, owner string, req apitypes.GameplayListRequest) (apitypes.GameResultListResponse, error) {
	items, hasNext, next, err := listOwnerRows(ctx, r, owner, "gameplay_game_results", true, req, scanGameResult)
	return apitypes.GameResultListResponse{Items: items, HasNext: hasNext, NextCursor: next}, err
}

func (r *Runtime) GetGameResult(ctx context.Context, owner, id string) (apitypes.GameResult, error) {
	db, err := r.db()
	if err != nil {
		return apitypes.GameResult{}, err
	}
	query, args := profileScopedOwnerIDQuery(ctx, gameResultSelectSQL(), owner, id)
	return scanGameResult(db.QueryRowContext(ctx, db.Rebind(query), args...))
}

func (r *Runtime) ListRewardGrants(ctx context.Context, owner string, req apitypes.GameplayListRequest) (apitypes.RewardGrantListResponse, error) {
	items, hasNext, next, err := listOwnerRows(ctx, r, owner, "gameplay_reward_grants", true, req, scanRewardGrant)
	return apitypes.RewardGrantListResponse{Items: items, HasNext: hasNext, NextCursor: next}, err
}

func (r *Runtime) GetRewardGrant(ctx context.Context, owner, id string) (apitypes.RewardGrant, error) {
	db, err := r.db()
	if err != nil {
		return apitypes.RewardGrant{}, err
	}
	query, args := profileScopedOwnerIDQuery(ctx, rewardGrantSelectSQL(), owner, id)
	return scanRewardGrant(db.QueryRowContext(ctx, db.Rebind(query), args...))
}

func (r *Runtime) resolveProfileRules(ctx context.Context, name string) (ProfileRules, error) {
	rules, err := profileRulesFromContext(ctx, name)
	if err != nil {
		return ProfileRules{}, err
	}
	if r == nil || r.Catalog == nil {
		return ProfileRules{}, errors.New("gameplay: catalog is not configured")
	}

	petPool := make([]ProfilePetPoolEntry, 0, len(rules.Spec.PetPool))
	for _, entry := range rules.Spec.PetPool {
		if _, err := r.Catalog.GetPetDefByID(ctx, entry.PetDefID); err != nil {
			if errors.Is(err, kv.ErrNotFound) {
				continue
			}
			return ProfileRules{}, err
		}
		petPool = append(petPool, entry)
	}
	rules.Spec.PetPool = petPool

	games := make(map[string]ProfileGameRule, len(rules.Spec.Games))
	for id, rule := range rules.Spec.Games {
		if _, err := r.Catalog.GetGameDefByID(ctx, id); err != nil {
			if errors.Is(err, kv.ErrNotFound) {
				continue
			}
			return ProfileRules{}, err
		}
		games[id] = rule
	}
	rules.Spec.Games = games

	badgeDefs := make(map[string]string, len(rules.Spec.BadgeDefs))
	for alias, id := range rules.Spec.BadgeDefs {
		if _, err := r.Catalog.GetBadgeDefByID(ctx, id); err != nil {
			if errors.Is(err, kv.ErrNotFound) {
				continue
			}
			return ProfileRules{}, err
		}
		badgeDefs[alias] = id
	}
	rules.Spec.BadgeDefs = badgeDefs
	return rules, nil
}

func (r *Runtime) pickPetDef(pool []ProfilePetPoolEntry) (ProfilePetPoolEntry, error) {
	var total int64
	for _, entry := range pool {
		if entry.Weight > 0 {
			total += entry.Weight
		}
	}
	if total <= 0 {
		return ProfilePetPoolEntry{}, errors.New("pet pool has no positive weight")
	}
	pick := r.pickWeight(total)
	var cursor int64
	for _, entry := range pool {
		cursor += entry.Weight
		if pick < cursor {
			return entry, nil
		}
	}
	return pool[len(pool)-1], nil
}

func (r *Runtime) pickWeight(total int64) int64 {
	if r != nil && r.PickWeight != nil {
		pick := r.PickWeight(total)
		if pick < 0 {
			return 0
		}
		if pick >= total {
			return total - 1
		}
		return pick
	}
	n, err := rand.Int(rand.Reader, big.NewInt(total))
	if err != nil {
		return 0
	}
	return n.Int64()
}

func (r *Runtime) createPetWorkspace(ctx context.Context, name, workflowName, voiceAlias string) (bool, error) {
	if r == nil || r.Workspaces == nil {
		return false, errors.New("gameplay: workspace service is not configured")
	}
	if err := r.validatePetWorkflow(ctx, workflowName); err != nil {
		return false, err
	}
	voiceAlias = strings.TrimSpace(voiceAlias)
	if voiceAlias == "" {
		return false, errors.New("gameplay: pet voice alias is required")
	}
	input := apitypes.WorkspaceInputModePushToTalk
	var parameters apitypes.WorkspaceParameters
	if err := parameters.FromPetWorkspaceParameters(apitypes.PetWorkspaceParameters{
		AgentType: apitypes.PetWorkspaceParametersAgentTypePet,
		Input:     &input,
		Voice: apitypes.PetVoiceParameters{
			VoiceId: voiceAlias,
		},
	}); err != nil {
		return false, err
	}
	body := adminhttp.WorkspaceUpsert{Name: name, WorkflowName: workflowName, Parameters: &parameters}
	workspace, created, err := r.Workspaces.CreateSystemWorkspace(ctx, body)
	if err != nil {
		return false, err
	}
	if !created {
		if err := validateExistingPetWorkspace(workspace, workflowName, input, voiceAlias); err != nil {
			return false, fmt.Errorf("create pet workspace %q: %w", name, err)
		}
	}
	return created, nil
}

func validateExistingPetWorkspace(workspace apitypes.Workspace, workflowName string, input apitypes.WorkspaceInputMode, voiceAlias string) error {
	if workspace.WorkflowName != workflowName {
		return fmt.Errorf("existing system Workspace uses workflow %q, want %q", workspace.WorkflowName, workflowName)
	}
	if workspace.Parameters == nil {
		return errors.New("existing system Workspace has no Pet parameters")
	}
	parameters, err := workspace.Parameters.AsPetWorkspaceParameters()
	if err != nil {
		return fmt.Errorf("decode existing system Workspace Pet parameters: %w", err)
	}
	if parameters.AgentType != apitypes.PetWorkspaceParametersAgentTypePet || parameters.Input == nil || *parameters.Input != input || parameters.Voice.VoiceId != voiceAlias {
		return errors.New("existing system Workspace parameters do not match the adoption selection")
	}
	return nil
}

func (r *Runtime) validatePetWorkflow(ctx context.Context, name string) error {
	if r == nil || r.Workflows == nil {
		return errors.New("gameplay: workflow service is not configured")
	}
	resp, err := r.Workflows.GetWorkflow(ctx, adminhttp.GetWorkflowRequestObject{Name: name})
	if err != nil {
		return fmt.Errorf("get pet workflow %q: %w", name, err)
	}
	switch v := resp.(type) {
	case adminhttp.GetWorkflow200JSONResponse:
		doc := apitypes.Workflow(v)
		if doc.Spec.Driver != apitypes.WorkflowDriverPet {
			return fmt.Errorf("workflow %q uses driver %q, want %q", name, doc.Spec.Driver, apitypes.WorkflowDriverPet)
		}
		return nil
	case adminhttp.GetWorkflow404JSONResponse:
		return fmt.Errorf("get pet workflow %q: %s", name, v.Error.Message)
	case adminhttp.GetWorkflow500JSONResponse:
		return fmt.Errorf("get pet workflow %q: %s", name, v.Error.Message)
	default:
		return fmt.Errorf("get pet workflow %q: unexpected response %T", name, resp)
	}
}

func (r *Runtime) ensureAccountTx(ctx context.Context, tx *sqlx.Tx, owner string, ruleset ProfileRules) (apitypes.PointsAccount, error) {
	account, err := scanPointsAccount(tx.QueryRowContext(ctx, tx.Rebind(pointsAccountSelectSQL()+` WHERE owner_public_key = ? AND runtime_profile_name = ?`), owner, ruleset.Name))
	if err == nil {
		return account, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return apitypes.PointsAccount{}, err
	}
	now := r.now()
	initial := int64(0)
	if ruleset.Spec.Points != nil {
		initial = int64Value(ruleset.Spec.Points.InitialBalance)
	}
	account = apitypes.PointsAccount{OwnerPublicKey: owner, RuntimeProfileName: ruleset.Name, Balance: initial, CreatedAt: now, UpdatedAt: now}
	inserted, err := insertPointsAccount(ctx, tx, account)
	if err != nil {
		return apitypes.PointsAccount{}, err
	}
	if inserted {
		return account, nil
	}
	return scanPointsAccount(tx.QueryRowContext(ctx, tx.Rebind(pointsAccountSelectSQL()+` WHERE owner_public_key = ? AND runtime_profile_name = ?`), owner, ruleset.Name))
}

func lockPointsAccountTx(ctx context.Context, tx *sqlx.Tx, account *apitypes.PointsAccount) error {
	return tx.QueryRowContext(ctx, tx.Rebind(`UPDATE gameplay_points_accounts SET balance = balance WHERE owner_public_key = ? AND runtime_profile_name = ? RETURNING balance`),
		account.OwnerPublicKey, account.RuntimeProfileName).Scan(&account.Balance)
}

func (r *Runtime) applyPointsTx(ctx context.Context, tx *sqlx.Tx, account *apitypes.PointsAccount, delta int64, runtimeProfileName, petID, gameResultID, rewardGrantID, reason, sourceType, sourceID string) (apitypes.PointsTransaction, error) {
	return r.recordPointsTx(ctx, tx, account, delta, runtimeProfileName, petID, gameResultID, rewardGrantID, reason, sourceType, sourceID, false)
}

func (r *Runtime) recordPointsTx(ctx context.Context, tx *sqlx.Tx, account *apitypes.PointsAccount, delta int64, runtimeProfileName, petID, gameResultID, rewardGrantID, reason, sourceType, sourceID string, recordZero bool) (apitypes.PointsTransaction, error) {
	if delta == 0 && !recordZero {
		return apitypes.PointsTransaction{}, nil
	}
	now := r.now()
	var next int64
	err := tx.QueryRowContext(ctx, tx.Rebind(`UPDATE gameplay_points_accounts SET balance = balance + ?, updated_at = ? WHERE owner_public_key = ? AND runtime_profile_name = ? AND balance + ? >= 0 RETURNING balance`),
		delta, formatTime(now), account.OwnerPublicKey, account.RuntimeProfileName, delta).Scan(&next)
	if errors.Is(err, sql.ErrNoRows) {
		return apitypes.PointsTransaction{}, errInsufficientPoints
	}
	if err != nil {
		return apitypes.PointsTransaction{}, err
	}
	account.Balance = next
	account.UpdatedAt = now
	txn := apitypes.PointsTransaction{
		Id:                 r.newID(),
		OwnerPublicKey:     account.OwnerPublicKey,
		RuntimeProfileName: runtimeProfileName,
		PetId:              optionalString(petID),
		GameResultId:       optionalString(gameResultID),
		RewardGrantId:      optionalString(rewardGrantID),
		Delta:              delta,
		BalanceAfter:       next,
		Reason:             reason,
		SourceType:         sourceType,
		SourceId:           sourceID,
		CreatedAt:          now,
	}
	return txn, insertPointsTransaction(ctx, tx, txn)
}

func (r *Runtime) applyBadgeExp(ctx context.Context, tx *sqlx.Tx, owner, badgeDefID string, delta int64, now time.Time) (apitypes.Badge, error) {
	if badgeDefID == "" || delta == 0 {
		return apitypes.Badge{}, nil
	}
	if _, err := r.Catalog.GetBadgeDefByID(ctx, badgeDefID); err != nil {
		return apitypes.Badge{}, err
	}
	badge, err := scanBadge(tx.QueryRowContext(ctx, tx.Rebind(badgeSelectSQL()+` WHERE owner_public_key = ? AND id = ?`), owner, badgeDefID))
	if errors.Is(err, sql.ErrNoRows) {
		badge = apitypes.Badge{Id: badgeDefID, OwnerPublicKey: owner, BadgeDefId: badgeDefID, CreatedAt: now}
	} else if err != nil {
		return apitypes.Badge{}, err
	}
	badge.Exp += delta
	if badge.Exp < 0 {
		badge.Exp = 0
	}
	badge.Level = badge.Exp / 100
	badge.Active = badge.Exp >= 100
	badge.Progress = badge.Exp % 100
	badge.UpdatedAt = now
	return badge, upsertBadge(ctx, tx, badge)
}

func (r *Runtime) db() (*sqlx.DB, error) {
	if r == nil || r.DB == nil {
		return nil, errors.New("gameplay: sql db is not configured")
	}
	return r.DB, nil
}

func (r *Runtime) now() time.Time {
	if r != nil && r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

func (r *Runtime) newID() string {
	if r != nil && r.NewID != nil {
		return r.NewID()
	}
	return socialutil.NewID()
}

func petDefDisplayName(petDef apitypes.PetDef) string {
	if catalog, ok := petDef.I18n.AdditionalProperties[petDef.I18n.DefaultLocale]; ok && catalog.DisplayName != nil && strings.TrimSpace(*catalog.DisplayName) != "" {
		return strings.TrimSpace(*catalog.DisplayName)
	}
	for _, catalog := range petDef.I18n.AdditionalProperties {
		if catalog.DisplayName != nil && strings.TrimSpace(*catalog.DisplayName) != "" {
			return strings.TrimSpace(*catalog.DisplayName)
		}
	}
	return petDef.Id
}

func requireOwner(owner string) error {
	if strings.TrimSpace(owner) == "" {
		return errors.New("owner public key is required")
	}
	return nil
}

func validateSQLDialect(driverName string) error {
	switch driverName {
	case "sqlite", "postgres":
		return nil
	default:
		return fmt.Errorf("gameplay: unsupported sql dialect %q", driverName)
	}
}

func (r *Runtime) driveMutex(key string) *sync.Mutex {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(key))
	return &r.driveMu[hash.Sum32()%uint32(len(r.driveMu))]
}

func (r *Runtime) adoptionMutex(key string) *sync.Mutex {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(key))
	return &r.adoptMu[hash.Sum32()%uint32(len(r.adoptMu))]
}

func sqlColumnExists(ctx context.Context, db *sqlx.DB, table, column string) (bool, error) {
	switch db.DriverName() {
	case "sqlite":
		rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
		if err != nil {
			return false, err
		}
		defer rows.Close()
		for rows.Next() {
			var cid int
			var name string
			var typ string
			var notNull int
			var defaultValue any
			var pk int
			if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
				return false, err
			}
			if name == column {
				return true, nil
			}
		}
		return false, rows.Err()
	case "postgres":
		var exists bool
		err := db.QueryRowContext(ctx, db.Rebind(`
SELECT EXISTS (
	SELECT 1
	FROM information_schema.columns
	WHERE table_schema = current_schema()
	  AND table_name = ?
	  AND column_name = ?
)`), table, column).Scan(&exists)
		return exists, err
	default:
		return false, fmt.Errorf("gameplay: unsupported sql dialect %q", db.DriverName())
	}
}
