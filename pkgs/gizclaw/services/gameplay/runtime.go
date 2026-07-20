package gameplay

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/jmoiron/sqlx"
)

const defaultPetWorkflowName = "pet-care"

var (
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
			workflow_name TEXT,
			life_json TEXT NOT NULL,
			ability_json TEXT NOT NULL,
			exp INTEGER NOT NULL,
			level INTEGER NOT NULL,
			last_active_at TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(owner_public_key, id)
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
			life_delta_json TEXT NOT NULL DEFAULT '{}',
			ability_delta_json TEXT NOT NULL DEFAULT '{}',
			source_type TEXT NOT NULL DEFAULT '',
			source_id TEXT NOT NULL DEFAULT '',
			reason TEXT,
			created_at TEXT NOT NULL,
			PRIMARY KEY(owner_public_key, id)
		)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	for _, migration := range []struct {
		table      string
		column     string
		definition string
	}{
		{"gameplay_points_transactions", "source_type", "TEXT NOT NULL DEFAULT ''"},
		{"gameplay_points_transactions", "source_id", "TEXT NOT NULL DEFAULT ''"},
		{"gameplay_game_results", "max_score", "INTEGER"},
		{"gameplay_game_results", "difficulty", "TEXT"},
		{"gameplay_game_results", "duration_ms", "INTEGER"},
		{"gameplay_game_results", "idempotency_key", "TEXT"},
		{"gameplay_game_results", "occurred_at", "TEXT NOT NULL DEFAULT ''"},
		{"gameplay_reward_grants", "life_delta_json", "TEXT NOT NULL DEFAULT '{}'"},
		{"gameplay_reward_grants", "ability_delta_json", "TEXT NOT NULL DEFAULT '{}'"},
		{"gameplay_reward_grants", "source_type", "TEXT NOT NULL DEFAULT ''"},
		{"gameplay_reward_grants", "source_id", "TEXT NOT NULL DEFAULT ''"},
	} {
		exists, err := sqlColumnExists(ctx, db, migration.table, migration.column)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", migration.table, migration.column, migration.definition)
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			// Another instance may have added the column after our catalog check.
			// Re-read schema state instead of matching driver-specific error text.
			exists, inspectErr := sqlColumnExists(ctx, db, migration.table, migration.column)
			if inspectErr != nil {
				return errors.Join(err, inspectErr)
			}
			if exists {
				continue
			}
			return err
		}
	}
	if _, err := db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS gameplay_game_results_idempotency_idx ON gameplay_game_results(owner_public_key, runtime_profile_name, idempotency_key) WHERE idempotency_key IS NOT NULL AND idempotency_key <> ''`); err != nil {
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
	ruleset, err := r.resolveProfileRules(ctx, "")
	if err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	poolEntry, err := r.pickPetDef(ruleset.Spec.PetPool)
	if err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	petDef, err := r.Catalog.GetPetDefByID(ctx, poolEntry.PetDefID)
	if err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	workflowName := defaultPetWorkflowName
	petID := r.newID()
	workspaceName := "pet-" + petID
	displayName := strings.TrimSpace(valueOrZero(req.DisplayName))
	if displayName == "" {
		displayName = petDefDisplayName(petDef)
	}
	if err := r.createPetWorkspace(ctx, workspaceName, workflowName, petDef); err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	created := false
	defer func() {
		if !created && r.Workspaces != nil {
			_, _ = r.Workspaces.DeleteSystemWorkspace(context.WithoutCancel(ctx), workspaceName)
		}
	}()
	now := r.now()
	pet := apitypes.Pet{
		Id:                 petID,
		OwnerPublicKey:     owner,
		RuntimeProfileName: ruleset.Name,
		PetdefId:           petDef.Id,
		DisplayName:        displayName,
		WorkspaceName:      workspaceName,
		Life:               initPetLife(petDef.Spec.Attr.Life),
		Progression:        initPetProgression(petDef.Spec.Attr.Progression),
		LastActiveAt:       now,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	db, err := r.db()
	if err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	defer tx.Rollback()
	account, err := r.ensureAccountTx(ctx, tx, owner, ruleset)
	if err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	cost := int64Value(poolEntry.AdoptionCost)
	txn, err := r.recordPointsTx(ctx, tx, &account, -cost, ruleset.Name, pet.Id, "", "", "pet.adopt", "pet", pet.Id, true)
	if err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	if err := insertPet(ctx, tx, pet); err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	if err := tx.Commit(); err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	created = true
	return apitypes.PetAdoptResponse{Pet: pet, Points: account, Transaction: txn}, nil
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

// OwnerHasPetWorkspace reports whether the Workspace belongs to one of the
// caller's adopted pets under the active RuntimeProfile. Pet Workspaces are
// system-managed, so this domain relationship supplies access independently of
// the Workspace owner field without crossing RuntimeProfile boundaries.
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
	err := r.DB.QueryRowContext(ctx, r.DB.Rebind(`SELECT 1 FROM gameplay_pets WHERE owner_public_key = ? AND runtime_profile_name = ? AND workspace_name = ? LIMIT 1`), strings.TrimSpace(owner), profileName, strings.TrimSpace(workspaceName)).Scan(&exists)
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
	pet, err := r.GetPet(ctx, owner, id)
	if err != nil {
		return apitypes.Pet{}, err
	}
	cleanupCtx := context.WithoutCancel(ctx)
	if r.Workspaces == nil {
		return apitypes.Pet{}, fmt.Errorf("delete pet %q: workspace service is not configured", pet.Id)
	}
	if _, err := r.Workspaces.DeleteSystemWorkspace(cleanupCtx, pet.WorkspaceName); err != nil {
		return apitypes.Pet{}, fmt.Errorf("delete pet %q workspace: %v", pet.Id, err)
	}
	db, err := r.db()
	if err != nil {
		return apitypes.Pet{}, r.restorePetAfterDeleteFailure(cleanupCtx, pet, owner, err)
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`DELETE FROM gameplay_pets WHERE owner_public_key = ? AND id = ? AND runtime_profile_name = ?`), owner, pet.Id, pet.RuntimeProfileName); err != nil {
		return apitypes.Pet{}, r.restorePetAfterDeleteFailure(cleanupCtx, pet, owner, err)
	}
	return pet, nil
}

func (r *Runtime) restorePetAfterDeleteFailure(ctx context.Context, pet apitypes.Pet, owner string, cause error) error {
	var rollbackErrs []error
	if r.Catalog == nil {
		rollbackErrs = append(rollbackErrs, errors.New("restore workspace: catalog service is not configured"))
	} else {
		petDef, err := r.Catalog.GetPetDefByID(ctx, pet.PetdefId)
		if err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("load PetDef: %w", err))
		} else {
			if err := r.createPetWorkspace(ctx, pet.WorkspaceName, defaultPetWorkflowName, petDef); err != nil {
				rollbackErrs = append(rollbackErrs, fmt.Errorf("restore workspace: %w", err))
			}
		}
	}
	if rollbackErr := errors.Join(rollbackErrs...); rollbackErr != nil {
		return fmt.Errorf("delete pet %q row: %w; rollback failed: %v", pet.Id, cause, rollbackErr)
	}
	return fmt.Errorf("delete pet %q row: %w", pet.Id, cause)
}

func (r *Runtime) DrivePet(ctx context.Context, owner string, req apitypes.PetDriveRequest) (apitypes.PetDriveResponse, error) {
	if err := r.Migration(ctx); err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	pet, err := r.GetPet(ctx, owner, req.PetId)
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	ruleset, err := r.resolveProfileRules(ctx, pet.RuntimeProfileName)
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	petDef, err := r.Catalog.GetPetDefByID(ctx, pet.PetdefId)
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	db, err := r.db()
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	defer tx.Rollback()
	account, err := r.ensureAccountTx(ctx, tx, owner, ruleset)
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	now := r.now()
	var transactions []apitypes.PointsTransaction
	var badges []apitypes.Badge
	var grants []apitypes.RewardGrant
	action := strings.TrimSpace(valueOrZero(req.Action))
	var actionSpec apitypes.PetDefActionSpec
	hasAction := false
	if action != "" {
		var ok bool
		actionSpec, ok = petDefAction(petDef, action)
		if !ok {
			return apitypes.PetDriveResponse{}, fmt.Errorf("pet action %q is not defined by petdef %q", action, petDef.Id)
		}
		hasAction = true
		if actionSpec.Cost > 0 {
			txn, err := r.applyPointsTx(ctx, tx, &account, -actionSpec.Cost, ruleset.Name, pet.Id, "", "", "pet.drive."+action, "pet_action", action)
			if err != nil {
				return apitypes.PetDriveResponse{}, err
			}
			transactions = append(transactions, txn)
		}
	}
	var result *apitypes.GameResult
	reward := mergeRewards(defaultReward(ruleset), petActionReward(ruleset, action))
	reward = mergeRewards(reward, actionEffectReward(actionSpec))
	if req.GameResult != nil {
		if err := r.validateGameResult(ctx, ruleset, req.GameResult.GameDefId); err != nil {
			return apitypes.PetDriveResponse{}, err
		}
		if key := strings.TrimSpace(valueOrZero(req.GameResult.IdempotencyKey)); key != "" {
			if _, err := findGameResultByIdempotencyKey(ctx, tx, owner, ruleset.Name, key); err == nil {
				return apitypes.PetDriveResponse{}, fmt.Errorf("game result idempotency_key %q was already recorded", key)
			} else if !errors.Is(err, sql.ErrNoRows) {
				return apitypes.PetDriveResponse{}, err
			}
		}
		occurredAt := now
		if req.GameResult.OccurredAt != nil {
			occurredAt = req.GameResult.OccurredAt.UTC()
		}
		gameResult := apitypes.GameResult{
			Id:                 r.newID(),
			OwnerPublicKey:     owner,
			RuntimeProfileName: ruleset.Name,
			PetId:              pet.Id,
			GameDefId:          req.GameResult.GameDefId,
			Score:              req.GameResult.Score,
			MaxScore:           req.GameResult.MaxScore,
			Difficulty:         req.GameResult.Difficulty,
			Outcome:            req.GameResult.Outcome,
			DurationMs:         req.GameResult.DurationMs,
			IdempotencyKey:     req.GameResult.IdempotencyKey,
			Payload:            req.GameResult.Payload,
			OccurredAt:         occurredAt,
			CreatedAt:          now,
		}
		if err := insertGameResult(ctx, tx, gameResult); err != nil {
			return apitypes.PetDriveResponse{}, err
		}
		result = &gameResult
		reward = mergeRewards(reward, gameReward(ruleset, req.GameResult.GameDefId))
	}
	if !rewardEmpty(reward) {
		sourceType, sourceID := rewardSource(action, result, pet.Id)
		grant := apitypes.RewardGrant{
			Id:                 r.newID(),
			OwnerPublicKey:     owner,
			RuntimeProfileName: ruleset.Name,
			PetId:              &pet.Id,
			PointsDelta:        int64Value(reward.PointsDelta),
			PetExpDelta:        int64Value(reward.PetExpDelta),
			BadgeExpDelta:      mapValue(reward.BadgeExpDelta),
			SourceType:         sourceType,
			SourceId:           sourceID,
			Reason:             stringPtr(rewardReason(action, result)),
			CreatedAt:          now,
		}
		if result != nil {
			grant.GameResultId = &result.Id
		}
		applyPetExp(&pet, grant.PetExpDelta)
		if err := insertRewardGrant(ctx, tx, grant); err != nil {
			return apitypes.PetDriveResponse{}, err
		}
		grants = append(grants, grant)
		if grant.PointsDelta != 0 {
			gameResultID := ""
			if result != nil {
				gameResultID = result.Id
			}
			txn, err := r.applyPointsTx(ctx, tx, &account, grant.PointsDelta, ruleset.Name, pet.Id, gameResultID, grant.Id, "reward.grant", "reward_grant", grant.Id)
			if err != nil {
				return apitypes.PetDriveResponse{}, err
			}
			transactions = append(transactions, txn)
		}
		for badgeID, delta := range grant.BadgeExpDelta {
			badge, err := r.applyBadgeExp(ctx, tx, owner, strings.TrimSpace(badgeID), delta, now)
			if err != nil {
				return apitypes.PetDriveResponse{}, err
			}
			badges = append(badges, badge)
		}
	}
	if hasAction {
		applyActionEffect(&pet, actionSpec)
	}
	pet.UpdatedAt = now
	pet.LastActiveAt = now
	if err := updatePet(ctx, tx, pet); err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	if err := tx.Commit(); err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	return apitypes.PetDriveResponse{Pet: pet, Points: account, GameResult: result, Badges: badges, RewardGrants: grants, Transactions: transactions}, nil
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
	ruleset, err := r.resolveProfileRules(ctx, runtimeProfileName)
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

	gameDefIDs := make([]string, 0, len(rules.Spec.GameDefIds))
	existingGameDefs := make(map[string]struct{}, len(rules.Spec.GameDefIds))
	for _, id := range rules.Spec.GameDefIds {
		if _, err := r.Catalog.GetGameDefByID(ctx, id); err != nil {
			if errors.Is(err, kv.ErrNotFound) {
				continue
			}
			return ProfileRules{}, err
		}
		gameDefIDs = append(gameDefIDs, id)
		existingGameDefs[id] = struct{}{}
	}
	rules.Spec.GameDefIds = gameDefIDs

	badgeDefIDs := make([]string, 0, len(rules.Spec.BadgeDefIds))
	existingBadgeDefs := make(map[string]struct{}, len(rules.Spec.BadgeDefIds))
	for _, id := range rules.Spec.BadgeDefIds {
		if _, err := r.Catalog.GetBadgeDefByID(ctx, id); err != nil {
			if errors.Is(err, kv.ErrNotFound) {
				continue
			}
			return ProfileRules{}, err
		}
		badgeDefIDs = append(badgeDefIDs, id)
		existingBadgeDefs[id] = struct{}{}
	}
	rules.Spec.BadgeDefIds = badgeDefIDs
	rules.Spec.Drive = filterResolvedDrive(rules.Spec.Drive, existingGameDefs, existingBadgeDefs)
	return rules, nil
}

func filterResolvedDrive(
	drive *apitypes.RuntimeProfileDriveSpec,
	existingGameDefs map[string]struct{},
	existingBadgeDefs map[string]struct{},
) *apitypes.RuntimeProfileDriveSpec {
	if drive == nil {
		return nil
	}
	out := &apitypes.RuntimeProfileDriveSpec{}
	if drive.Default != nil {
		reward := filterResolvedReward(*drive.Default, existingBadgeDefs)
		out.Default = &reward
	}
	if drive.Games != nil {
		rewards := make(map[string]apitypes.RuntimeProfileRewardSpec, len(*drive.Games))
		for gameDefID, reward := range *drive.Games {
			if _, ok := existingGameDefs[gameDefID]; !ok {
				continue
			}
			rewards[gameDefID] = filterResolvedReward(reward, existingBadgeDefs)
		}
		out.Games = &rewards
	}
	if drive.PetActions != nil {
		rewards := make(map[string]apitypes.RuntimeProfileRewardSpec, len(*drive.PetActions))
		for action, reward := range *drive.PetActions {
			rewards[action] = filterResolvedReward(reward, existingBadgeDefs)
		}
		out.PetActions = &rewards
	}
	return out
}

func filterResolvedReward(
	reward apitypes.RuntimeProfileRewardSpec,
	existingBadgeDefs map[string]struct{},
) apitypes.RuntimeProfileRewardSpec {
	if reward.BadgeExpDelta == nil {
		return reward
	}
	filtered := make(map[string]int64, len(*reward.BadgeExpDelta))
	for badgeDefID, delta := range *reward.BadgeExpDelta {
		if _, ok := existingBadgeDefs[badgeDefID]; ok {
			filtered[badgeDefID] = delta
		}
	}
	reward.BadgeExpDelta = &filtered
	return reward
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

func (r *Runtime) createPetWorkspace(ctx context.Context, name, workflowName string, petDef apitypes.PetDef) error {
	if r == nil || r.Workspaces == nil {
		return errors.New("gameplay: workspace service is not configured")
	}
	if err := r.validatePetWorkflow(ctx, workflowName); err != nil {
		return err
	}
	input := apitypes.WorkspaceInputModePushToTalk
	var parameters apitypes.WorkspaceParameters
	if err := parameters.FromPetWorkspaceParameters(apitypes.PetWorkspaceParameters{
		AgentType: apitypes.PetWorkspaceParametersAgentTypePet,
		Input:     &input,
		Voice: apitypes.PetVoiceParameters{
			VoiceId: petDef.Spec.Voice.VoiceId,
		},
	}); err != nil {
		return err
	}
	body := adminhttp.WorkspaceUpsert{Name: name, WorkflowName: workflowName, Parameters: &parameters}
	_, created, err := r.Workspaces.CreateSystemWorkspace(ctx, body)
	if err != nil {
		return err
	}
	if !created {
		return fmt.Errorf("create pet workspace %q: workspace already exists", name)
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
	if err := insertPointsAccount(ctx, tx, account); err != nil {
		return apitypes.PointsAccount{}, err
	}
	return account, nil
}

func (r *Runtime) applyPointsTx(ctx context.Context, tx *sqlx.Tx, account *apitypes.PointsAccount, delta int64, runtimeProfileName, petID, gameResultID, rewardGrantID, reason, sourceType, sourceID string) (apitypes.PointsTransaction, error) {
	return r.recordPointsTx(ctx, tx, account, delta, runtimeProfileName, petID, gameResultID, rewardGrantID, reason, sourceType, sourceID, false)
}

func (r *Runtime) recordPointsTx(ctx context.Context, tx *sqlx.Tx, account *apitypes.PointsAccount, delta int64, runtimeProfileName, petID, gameResultID, rewardGrantID, reason, sourceType, sourceID string, recordZero bool) (apitypes.PointsTransaction, error) {
	if delta == 0 && !recordZero {
		return apitypes.PointsTransaction{}, nil
	}
	next := account.Balance + delta
	if next < 0 {
		return apitypes.PointsTransaction{}, errors.New("gameplay: insufficient points")
	}
	now := r.now()
	account.Balance = next
	account.UpdatedAt = now
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE gameplay_points_accounts SET balance = ?, updated_at = ? WHERE owner_public_key = ? AND runtime_profile_name = ?`), account.Balance, formatTime(account.UpdatedAt), account.OwnerPublicKey, account.RuntimeProfileName); err != nil {
		return apitypes.PointsTransaction{}, err
	}
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

func (r *Runtime) validateGameResult(ctx context.Context, ruleset ProfileRules, gameDefID string) error {
	gameDefID = strings.TrimSpace(gameDefID)
	if gameDefID == "" {
		return errors.New("game_def_id is required")
	}
	found := false
	for _, id := range ruleset.Spec.GameDefIds {
		if id == gameDefID {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("game def %q is not in RuntimeProfile %q", gameDefID, ruleset.Name)
	}
	_, err := r.Catalog.GetGameDefByID(ctx, gameDefID)
	return err
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

func petLevel(exp int64) int64 {
	if exp < 0 {
		exp = 0
	}
	return exp/100 + 1
}

func validateSQLDialect(driverName string) error {
	switch driverName {
	case "sqlite", "postgres":
		return nil
	default:
		return fmt.Errorf("gameplay: unsupported sql dialect %q", driverName)
	}
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
