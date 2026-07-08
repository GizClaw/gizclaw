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
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

const defaultPetWorkflowName = "chatroom"

type Runtime struct {
	DB          *sql.DB
	Catalog     *Catalog
	Workspaces  workspace.WorkspaceAdminService
	ACL         ACL
	Now         func() time.Time
	NewID       func() string
	PickWeight  func(total int64) int64
	DecayPeriod time.Duration
}

type ACL interface {
	PutRole(context.Context, string, apitypes.ACLPermissionList) (apitypes.ACLRole, error)
	PutPolicyBinding(context.Context, string, float64, apitypes.ACLPolicy) (apitypes.ACLPolicyBinding, error)
	DeletePolicyBinding(context.Context, string) (apitypes.ACLPolicyBinding, error)
}

func (r *Runtime) Migration(ctx context.Context) error {
	db, err := r.db()
	if err != nil {
		return err
	}
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS gameplay_pets (
			owner_public_key TEXT NOT NULL,
			id TEXT NOT NULL,
			ruleset_name TEXT NOT NULL,
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
			ruleset_name TEXT NOT NULL,
			balance INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(owner_public_key, ruleset_name)
		)`,
		`CREATE TABLE IF NOT EXISTS gameplay_points_transactions (
			owner_public_key TEXT NOT NULL,
			id TEXT NOT NULL,
			ruleset_name TEXT NOT NULL,
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
			ruleset_name TEXT NOT NULL,
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
			ruleset_name TEXT NOT NULL,
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
	for _, stmt := range []string{
		`ALTER TABLE gameplay_points_transactions ADD COLUMN source_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE gameplay_points_transactions ADD COLUMN source_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE gameplay_game_results ADD COLUMN max_score INTEGER`,
		`ALTER TABLE gameplay_game_results ADD COLUMN difficulty TEXT`,
		`ALTER TABLE gameplay_game_results ADD COLUMN duration_ms INTEGER`,
		`ALTER TABLE gameplay_game_results ADD COLUMN idempotency_key TEXT`,
		`ALTER TABLE gameplay_game_results ADD COLUMN occurred_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE gameplay_reward_grants ADD COLUMN life_delta_json TEXT NOT NULL DEFAULT '{}'`,
		`ALTER TABLE gameplay_reward_grants ADD COLUMN ability_delta_json TEXT NOT NULL DEFAULT '{}'`,
		`ALTER TABLE gameplay_reward_grants ADD COLUMN source_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE gameplay_reward_grants ADD COLUMN source_id TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil && !isDuplicateColumnError(err) {
			return err
		}
	}
	if _, err := db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS gameplay_game_results_idempotency_idx ON gameplay_game_results(owner_public_key, ruleset_name, idempotency_key) WHERE idempotency_key IS NOT NULL AND idempotency_key <> ''`); err != nil {
		return err
	}
	return nil
}

func (r *Runtime) GetGameRuleset(ctx context.Context, name string) (apitypes.GameRuleset, error) {
	return r.resolveRuleset(ctx, name)
}

func (r *Runtime) AdoptPet(ctx context.Context, owner string, req apitypes.PetAdoptRequest) (apitypes.PetAdoptResponse, error) {
	if err := requireOwner(owner); err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	if err := r.Migration(ctx); err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	ruleset, err := r.resolveRuleset(ctx, valueOrZero(req.RulesetName))
	if err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	poolEntry, err := r.pickPetDef(ruleset.Spec.PetPool)
	if err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	petDef, err := r.Catalog.GetPetDefByID(ctx, poolEntry.PetdefId)
	if err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	workflowName := selectedWorkflow(ruleset, petDef, poolEntry)
	petID := r.newID()
	workspaceName := "pet-" + petID
	displayName := strings.TrimSpace(valueOrZero(req.DisplayName))
	if displayName == "" {
		displayName = petDef.Spec.DisplayName
	}
	if err := r.createPetWorkspace(ctx, workspaceName, workflowName); err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	created := false
	defer func() {
		if !created && r.Workspaces != nil {
			_ = r.revokePetWorkspace(ctx, workspaceName, owner)
			_, _ = r.Workspaces.DeleteWorkspace(context.WithoutCancel(ctx), adminhttp.DeleteWorkspaceRequestObject{Name: workspaceName})
		}
	}()
	if err := r.grantPetWorkspace(ctx, workspaceName, owner); err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	now := r.now()
	pet := apitypes.Pet{
		Id:             petID,
		OwnerPublicKey: owner,
		RulesetName:    ruleset.Name,
		PetdefId:       petDef.Id,
		DisplayName:    displayName,
		WorkspaceName:  workspaceName,
		WorkflowName:   stringPtr(workflowName),
		Life:           cloneStatMap(valueOrZero(petDef.Spec.InitialLife)),
		Ability:        cloneStatMap(valueOrZero(petDef.Spec.InitialAbility)),
		Exp:            0,
		Level:          1,
		LastActiveAt:   now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	db, err := r.db()
	if err != nil {
		return apitypes.PetAdoptResponse{}, err
	}
	tx, err := db.BeginTx(ctx, nil)
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
	items, hasNext, next, err := listOwnerRows(ctx, r, owner, "gameplay_pets", req, scanPet)
	return apitypes.PetListResponse{Items: items, HasNext: hasNext, NextCursor: next}, err
}

func (r *Runtime) GetPet(ctx context.Context, owner, id string) (apitypes.Pet, error) {
	db, err := r.db()
	if err != nil {
		return apitypes.Pet{}, err
	}
	return scanPet(db.QueryRowContext(ctx, petSelectSQL()+` WHERE owner_public_key = ? AND id = ?`, owner, strings.TrimSpace(id)))
}

func (r *Runtime) OwnerHasPetDef(ctx context.Context, owner, petDefID string) (bool, error) {
	db, err := r.db()
	if err != nil {
		return false, err
	}
	var exists int
	err = db.QueryRowContext(ctx, `SELECT 1 FROM gameplay_pets WHERE owner_public_key = ? AND petdef_id = ? LIMIT 1`, owner, strings.TrimSpace(petDefID)).Scan(&exists)
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
	if _, err := db.ExecContext(ctx, `UPDATE gameplay_pets SET display_name = ?, updated_at = ? WHERE owner_public_key = ? AND id = ?`, pet.DisplayName, formatTime(pet.UpdatedAt), owner, pet.Id); err != nil {
		return apitypes.Pet{}, err
	}
	return pet, nil
}

func (r *Runtime) DeletePet(ctx context.Context, owner, id string) (apitypes.Pet, error) {
	pet, err := r.GetPet(ctx, owner, id)
	if err != nil {
		return apitypes.Pet{}, err
	}
	db, err := r.db()
	if err != nil {
		return apitypes.Pet{}, err
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM gameplay_pets WHERE owner_public_key = ? AND id = ?`, owner, pet.Id); err != nil {
		return apitypes.Pet{}, err
	}
	_ = r.revokePetWorkspace(context.WithoutCancel(ctx), pet.WorkspaceName, owner)
	if r.Workspaces != nil {
		_, _ = r.Workspaces.DeleteWorkspace(context.WithoutCancel(ctx), adminhttp.DeleteWorkspaceRequestObject{Name: pet.WorkspaceName})
	}
	return pet, nil
}

func (r *Runtime) DrivePet(ctx context.Context, owner string, req apitypes.PetDriveRequest) (apitypes.PetDriveResponse, error) {
	if err := r.Migration(ctx); err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	pet, err := r.GetPet(ctx, owner, req.PetId)
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	ruleset, err := r.resolveRuleset(ctx, pet.RulesetName)
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	db, err := r.db()
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	defer tx.Rollback()
	account, err := r.ensureAccountTx(ctx, tx, owner, ruleset)
	if err != nil {
		return apitypes.PetDriveResponse{}, err
	}
	now := r.now()
	r.applyTimeDecay(&pet, ruleset, now)
	var transactions []apitypes.PointsTransaction
	var badges []apitypes.Badge
	var grants []apitypes.RewardGrant
	action := strings.TrimSpace(valueOrZero(req.Action))
	if action != "" {
		cost := actionCost(ruleset, action)
		if cost > 0 {
			txn, err := r.applyPointsTx(ctx, tx, &account, -cost, ruleset.Name, pet.Id, "", "", "pet.drive."+action, "pet_action", action)
			if err != nil {
				return apitypes.PetDriveResponse{}, err
			}
			transactions = append(transactions, txn)
		}
	}
	var result *apitypes.GameResult
	reward := mergeRewards(defaultReward(ruleset), actionReward(ruleset, action))
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
			Id:             r.newID(),
			OwnerPublicKey: owner,
			RulesetName:    ruleset.Name,
			PetId:          pet.Id,
			GameDefId:      req.GameResult.GameDefId,
			Score:          req.GameResult.Score,
			MaxScore:       req.GameResult.MaxScore,
			Difficulty:     req.GameResult.Difficulty,
			Outcome:        req.GameResult.Outcome,
			DurationMs:     req.GameResult.DurationMs,
			IdempotencyKey: req.GameResult.IdempotencyKey,
			Payload:        req.GameResult.Payload,
			OccurredAt:     occurredAt,
			CreatedAt:      now,
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
			Id:             r.newID(),
			OwnerPublicKey: owner,
			RulesetName:    ruleset.Name,
			PetId:          &pet.Id,
			PointsDelta:    int64Value(reward.PointsDelta),
			PetExpDelta:    int64Value(reward.PetExpDelta),
			BadgeExpDelta:  mapValue(reward.BadgeExpDelta),
			LifeDelta:      reward.LifeDelta,
			AbilityDelta:   reward.AbilityDelta,
			SourceType:     sourceType,
			SourceId:       sourceID,
			Reason:         stringPtr(rewardReason(action, result)),
			CreatedAt:      now,
		}
		if result != nil {
			grant.GameResultId = &result.Id
		}
		applyStatDelta(pet.Life, reward.LifeDelta)
		applyStatDelta(pet.Ability, reward.AbilityDelta)
		pet.Exp += grant.PetExpDelta
		pet.Level = petLevel(pet.Exp)
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

func (r *Runtime) GetPoints(ctx context.Context, owner, rulesetName string) (apitypes.PointsAccount, error) {
	if err := r.Migration(ctx); err != nil {
		return apitypes.PointsAccount{}, err
	}
	ruleset, err := r.resolveRuleset(ctx, rulesetName)
	if err != nil {
		return apitypes.PointsAccount{}, err
	}
	db, err := r.db()
	if err != nil {
		return apitypes.PointsAccount{}, err
	}
	tx, err := db.BeginTx(ctx, nil)
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
	items, hasNext, next, err := listOwnerRows(ctx, r, owner, "gameplay_points_transactions", req, scanPointsTransaction)
	return apitypes.PointsTransactionListResponse{Items: items, HasNext: hasNext, NextCursor: next}, err
}

func (r *Runtime) GetPointsTransaction(ctx context.Context, owner, id string) (apitypes.PointsTransaction, error) {
	db, err := r.db()
	if err != nil {
		return apitypes.PointsTransaction{}, err
	}
	return scanPointsTransaction(db.QueryRowContext(ctx, pointsTransactionSelectSQL()+` WHERE owner_public_key = ? AND id = ?`, owner, strings.TrimSpace(id)))
}

func (r *Runtime) ListBadges(ctx context.Context, owner string, req apitypes.GameplayListRequest) (apitypes.BadgeListResponse, error) {
	items, hasNext, next, err := listOwnerRows(ctx, r, owner, "gameplay_badges", req, scanBadge)
	return apitypes.BadgeListResponse{Items: items, HasNext: hasNext, NextCursor: next}, err
}

func (r *Runtime) GetBadge(ctx context.Context, owner, id string) (apitypes.Badge, error) {
	db, err := r.db()
	if err != nil {
		return apitypes.Badge{}, err
	}
	return scanBadge(db.QueryRowContext(ctx, badgeSelectSQL()+` WHERE owner_public_key = ? AND id = ?`, owner, strings.TrimSpace(id)))
}

func (r *Runtime) OwnerHasBadgeDef(ctx context.Context, owner, badgeDefID string) (bool, error) {
	db, err := r.db()
	if err != nil {
		return false, err
	}
	var exists int
	err = db.QueryRowContext(ctx, `SELECT 1 FROM gameplay_badges WHERE owner_public_key = ? AND badge_def_id = ? LIMIT 1`, owner, strings.TrimSpace(badgeDefID)).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (r *Runtime) ListGameResults(ctx context.Context, owner string, req apitypes.GameplayListRequest) (apitypes.GameResultListResponse, error) {
	items, hasNext, next, err := listOwnerRows(ctx, r, owner, "gameplay_game_results", req, scanGameResult)
	return apitypes.GameResultListResponse{Items: items, HasNext: hasNext, NextCursor: next}, err
}

func (r *Runtime) GetGameResult(ctx context.Context, owner, id string) (apitypes.GameResult, error) {
	db, err := r.db()
	if err != nil {
		return apitypes.GameResult{}, err
	}
	return scanGameResult(db.QueryRowContext(ctx, gameResultSelectSQL()+` WHERE owner_public_key = ? AND id = ?`, owner, strings.TrimSpace(id)))
}

func (r *Runtime) ListRewardGrants(ctx context.Context, owner string, req apitypes.GameplayListRequest) (apitypes.RewardGrantListResponse, error) {
	items, hasNext, next, err := listOwnerRows(ctx, r, owner, "gameplay_reward_grants", req, scanRewardGrant)
	return apitypes.RewardGrantListResponse{Items: items, HasNext: hasNext, NextCursor: next}, err
}

func (r *Runtime) GetRewardGrant(ctx context.Context, owner, id string) (apitypes.RewardGrant, error) {
	db, err := r.db()
	if err != nil {
		return apitypes.RewardGrant{}, err
	}
	return scanRewardGrant(db.QueryRowContext(ctx, rewardGrantSelectSQL()+` WHERE owner_public_key = ? AND id = ?`, owner, strings.TrimSpace(id)))
}

func (r *Runtime) resolveRuleset(ctx context.Context, name string) (apitypes.GameRuleset, error) {
	if r == nil || r.Catalog == nil {
		return apitypes.GameRuleset{}, errors.New("gameplay: catalog is not configured")
	}
	name = strings.TrimSpace(name)
	if name != "" {
		ruleset, err := r.Catalog.GetGameRulesetByName(ctx, name)
		if err != nil {
			return apitypes.GameRuleset{}, err
		}
		if !ruleset.Spec.Enabled {
			return apitypes.GameRuleset{}, fmt.Errorf("game ruleset %q is disabled", ruleset.Name)
		}
		return ruleset, nil
	}
	store, err := r.Catalog.store(r.Catalog.GameRulesets, "game rulesets")
	if err != nil {
		return apitypes.GameRuleset{}, err
	}
	items, _, _, err := listJSON[apitypes.GameRuleset](ctx, store, gameRulesetsRoot, "", maxListLimit)
	if err != nil {
		return apitypes.GameRuleset{}, err
	}
	for _, item := range items {
		if item.Spec.Enabled {
			return item, nil
		}
	}
	return apitypes.GameRuleset{}, fmt.Errorf("gameplay: no enabled game ruleset: %w", kv.ErrNotFound)
}

func (r *Runtime) pickPetDef(pool []apitypes.GameRulesetPetPoolEntry) (apitypes.GameRulesetPetPoolEntry, error) {
	var total int64
	for _, entry := range pool {
		if entry.Weight > 0 {
			total += entry.Weight
		}
	}
	if total <= 0 {
		return apitypes.GameRulesetPetPoolEntry{}, errors.New("pet pool has no positive weight")
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

func (r *Runtime) createPetWorkspace(ctx context.Context, name, workflowName string) error {
	if r == nil || r.Workspaces == nil {
		return errors.New("gameplay: workspace service is not configured")
	}
	input := apitypes.WorkspaceInputModePushToTalk
	var parameters apitypes.WorkspaceParameters
	if err := parameters.FromFlowcraftWorkspaceParameters(apitypes.FlowcraftWorkspaceParameters{
		AgentType: apitypes.FlowcraftWorkspaceParametersAgentTypeFlowcraft,
		Input:     &input,
	}); err != nil {
		return err
	}
	body := adminhttp.WorkspaceUpsert{Name: name, WorkflowName: workflowName, Parameters: &parameters}
	resp, err := r.Workspaces.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
	if err != nil {
		return err
	}
	switch v := resp.(type) {
	case adminhttp.CreateWorkspace200JSONResponse:
		return nil
	case adminhttp.CreateWorkspace409JSONResponse:
		return fmt.Errorf("create pet workspace: %s", v.Error.Message)
	case adminhttp.CreateWorkspace400JSONResponse:
		return fmt.Errorf("create pet workspace: %s", v.Error.Message)
	case adminhttp.CreateWorkspace500JSONResponse:
		return fmt.Errorf("create pet workspace: %s", v.Error.Message)
	default:
		return fmt.Errorf("create pet workspace: unexpected response %T", resp)
	}
}

func (r *Runtime) grantPetWorkspace(ctx context.Context, workspaceName, owner string) error {
	if r == nil || r.ACL == nil {
		return nil
	}
	roleName, permissions := socialutil.WorkspaceACLRole()
	if _, err := r.ACL.PutRole(ctx, roleName, permissions); err != nil {
		return err
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return nil
	}
	_, err := r.ACL.PutPolicyBinding(ctx, petWorkspaceACLBindingID(workspaceName, owner), 0, apitypes.ACLPolicy{
		Subject: apitypes.ACLSubject{
			Kind: apitypes.ACLSubjectKindPk,
			Id:   owner,
		},
		Resource: apitypes.ACLResource{
			Kind: apitypes.ACLResourceKindWorkspace,
			Id:   workspaceName,
		},
		Role: roleName,
	})
	return err
}

func (r *Runtime) revokePetWorkspace(ctx context.Context, workspaceName, owner string) error {
	if r == nil || r.ACL == nil {
		return nil
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return nil
	}
	if _, err := r.ACL.DeletePolicyBinding(ctx, petWorkspaceACLBindingID(workspaceName, owner)); err != nil && !errors.Is(err, acl.ErrPolicyBindingNotFound) {
		return err
	}
	return nil
}

func petWorkspaceACLBindingID(workspaceName, owner string) string {
	return "gameplay-pet-workspace:" + socialutil.EscapeStoreSegment(workspaceName) + ":" + socialutil.EscapeStoreSegment(owner)
}

func (r *Runtime) ensureAccountTx(ctx context.Context, tx *sql.Tx, owner string, ruleset apitypes.GameRuleset) (apitypes.PointsAccount, error) {
	account, err := scanPointsAccount(tx.QueryRowContext(ctx, pointsAccountSelectSQL()+` WHERE owner_public_key = ? AND ruleset_name = ?`, owner, ruleset.Name))
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
	account = apitypes.PointsAccount{OwnerPublicKey: owner, RulesetName: ruleset.Name, Balance: initial, CreatedAt: now, UpdatedAt: now}
	if err := insertPointsAccount(ctx, tx, account); err != nil {
		return apitypes.PointsAccount{}, err
	}
	return account, nil
}

func (r *Runtime) applyPointsTx(ctx context.Context, tx *sql.Tx, account *apitypes.PointsAccount, delta int64, rulesetName, petID, gameResultID, rewardGrantID, reason, sourceType, sourceID string) (apitypes.PointsTransaction, error) {
	return r.recordPointsTx(ctx, tx, account, delta, rulesetName, petID, gameResultID, rewardGrantID, reason, sourceType, sourceID, false)
}

func (r *Runtime) recordPointsTx(ctx context.Context, tx *sql.Tx, account *apitypes.PointsAccount, delta int64, rulesetName, petID, gameResultID, rewardGrantID, reason, sourceType, sourceID string, recordZero bool) (apitypes.PointsTransaction, error) {
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
	if _, err := tx.ExecContext(ctx, `UPDATE gameplay_points_accounts SET balance = ?, updated_at = ? WHERE owner_public_key = ? AND ruleset_name = ?`, account.Balance, formatTime(account.UpdatedAt), account.OwnerPublicKey, account.RulesetName); err != nil {
		return apitypes.PointsTransaction{}, err
	}
	txn := apitypes.PointsTransaction{
		Id:             r.newID(),
		OwnerPublicKey: account.OwnerPublicKey,
		RulesetName:    rulesetName,
		PetId:          optionalString(petID),
		GameResultId:   optionalString(gameResultID),
		RewardGrantId:  optionalString(rewardGrantID),
		Delta:          delta,
		BalanceAfter:   next,
		Reason:         reason,
		SourceType:     sourceType,
		SourceId:       sourceID,
		CreatedAt:      now,
	}
	return txn, insertPointsTransaction(ctx, tx, txn)
}

func (r *Runtime) applyBadgeExp(ctx context.Context, tx *sql.Tx, owner, badgeDefID string, delta int64, now time.Time) (apitypes.Badge, error) {
	if badgeDefID == "" || delta == 0 {
		return apitypes.Badge{}, nil
	}
	if _, err := r.Catalog.GetBadgeDefByID(ctx, badgeDefID); err != nil {
		return apitypes.Badge{}, err
	}
	badge, err := scanBadge(tx.QueryRowContext(ctx, badgeSelectSQL()+` WHERE owner_public_key = ? AND id = ?`, owner, badgeDefID))
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

func (r *Runtime) validateGameResult(ctx context.Context, ruleset apitypes.GameRuleset, gameDefID string) error {
	gameDefID = strings.TrimSpace(gameDefID)
	if gameDefID == "" {
		return errors.New("game_def_id is required")
	}
	if ruleset.Spec.GameDefIds != nil && len(*ruleset.Spec.GameDefIds) > 0 {
		found := false
		for _, id := range *ruleset.Spec.GameDefIds {
			if id == gameDefID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("game def %q is not in ruleset %q", gameDefID, ruleset.Name)
		}
	}
	_, err := r.Catalog.GetGameDefByID(ctx, gameDefID)
	return err
}

func (r *Runtime) applyTimeDecay(pet *apitypes.Pet, ruleset apitypes.GameRuleset, now time.Time) {
	if pet == nil || ruleset.Spec.Drive == nil || ruleset.Spec.Drive.LifeDecayPerHour == nil {
		return
	}
	period := r.DecayPeriod
	if period <= 0 {
		period = time.Hour
	}
	elapsed := now.Sub(pet.LastActiveAt)
	if elapsed < period {
		return
	}
	steps := int64(elapsed / period)
	for k, v := range *ruleset.Spec.Drive.LifeDecayPerHour {
		pet.Life[k] -= v * steps
		if pet.Life[k] < 0 {
			pet.Life[k] = 0
		}
	}
}

func (r *Runtime) db() (*sql.DB, error) {
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

func selectedWorkflow(ruleset apitypes.GameRuleset, petDef apitypes.PetDef, pool apitypes.GameRulesetPetPoolEntry) string {
	for _, candidate := range []string{valueOrZero(pool.WorkflowName), valueOrZero(petDef.Spec.WorkflowName), valueOrZero(ruleset.Spec.DefaultWorkflowName), defaultPetWorkflowName} {
		if strings.TrimSpace(candidate) != "" {
			return strings.TrimSpace(candidate)
		}
	}
	return defaultPetWorkflowName
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

func isDuplicateColumnError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column") || strings.Contains(msg, "already exists")
}
