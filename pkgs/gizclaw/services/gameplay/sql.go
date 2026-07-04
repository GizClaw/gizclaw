package gameplay

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

type rowScanner interface {
	Scan(dest ...any) error
}

func petSelectSQL() string {
	return `SELECT owner_public_key, id, ruleset_name, petdef_id, display_name, workspace_name, workflow_name, life_json, ability_json, exp, level, last_active_at, created_at, updated_at FROM gameplay_pets`
}

func scanPet(row rowScanner) (apitypes.Pet, error) {
	var pet apitypes.Pet
	var workflowName sql.NullString
	var lifeJSON, abilityJSON string
	var lastActiveAt, createdAt, updatedAt string
	err := row.Scan(&pet.OwnerPublicKey, &pet.Id, &pet.RulesetName, &pet.PetdefId, &pet.DisplayName, &pet.WorkspaceName, &workflowName, &lifeJSON, &abilityJSON, &pet.Exp, &pet.Level, &lastActiveAt, &createdAt, &updatedAt)
	if err != nil {
		return apitypes.Pet{}, err
	}
	if workflowName.Valid {
		pet.WorkflowName = &workflowName.String
	}
	if err := unmarshalJSON(lifeJSON, &pet.Life); err != nil {
		return apitypes.Pet{}, err
	}
	if err := unmarshalJSON(abilityJSON, &pet.Ability); err != nil {
		return apitypes.Pet{}, err
	}
	pet.LastActiveAt = parseTime(lastActiveAt)
	pet.CreatedAt = parseTime(createdAt)
	pet.UpdatedAt = parseTime(updatedAt)
	return pet, nil
}

func insertPet(ctx context.Context, tx *sql.Tx, pet apitypes.Pet) error {
	lifeJSON, err := marshalJSON(pet.Life)
	if err != nil {
		return err
	}
	abilityJSON, err := marshalJSON(pet.Ability)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO gameplay_pets (owner_public_key, id, ruleset_name, petdef_id, display_name, workspace_name, workflow_name, life_json, ability_json, exp, level, last_active_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		pet.OwnerPublicKey, pet.Id, pet.RulesetName, pet.PetdefId, pet.DisplayName, pet.WorkspaceName, valueOrZero(pet.WorkflowName), lifeJSON, abilityJSON, pet.Exp, pet.Level, formatTime(pet.LastActiveAt), formatTime(pet.CreatedAt), formatTime(pet.UpdatedAt))
	return err
}

func updatePet(ctx context.Context, tx *sql.Tx, pet apitypes.Pet) error {
	lifeJSON, err := marshalJSON(pet.Life)
	if err != nil {
		return err
	}
	abilityJSON, err := marshalJSON(pet.Ability)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE gameplay_pets SET display_name = ?, life_json = ?, ability_json = ?, exp = ?, level = ?, last_active_at = ?, updated_at = ? WHERE owner_public_key = ? AND id = ?`,
		pet.DisplayName, lifeJSON, abilityJSON, pet.Exp, pet.Level, formatTime(pet.LastActiveAt), formatTime(pet.UpdatedAt), pet.OwnerPublicKey, pet.Id)
	return err
}

func pointsAccountSelectSQL() string {
	return `SELECT owner_public_key, ruleset_name, balance, created_at, updated_at FROM gameplay_points_accounts`
}

func scanPointsAccount(row rowScanner) (apitypes.PointsAccount, error) {
	var account apitypes.PointsAccount
	var createdAt, updatedAt string
	if err := row.Scan(&account.OwnerPublicKey, &account.RulesetName, &account.Balance, &createdAt, &updatedAt); err != nil {
		return apitypes.PointsAccount{}, err
	}
	account.CreatedAt = parseTime(createdAt)
	account.UpdatedAt = parseTime(updatedAt)
	return account, nil
}

func insertPointsAccount(ctx context.Context, tx *sql.Tx, account apitypes.PointsAccount) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO gameplay_points_accounts (owner_public_key, ruleset_name, balance, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		account.OwnerPublicKey, account.RulesetName, account.Balance, formatTime(account.CreatedAt), formatTime(account.UpdatedAt))
	return err
}

func pointsTransactionSelectSQL() string {
	return `SELECT owner_public_key, id, ruleset_name, pet_id, game_result_id, reward_grant_id, delta, balance_after, reason, created_at FROM gameplay_points_transactions`
}

func scanPointsTransaction(row rowScanner) (apitypes.PointsTransaction, error) {
	var item apitypes.PointsTransaction
	var petID, gameResultID, rewardGrantID sql.NullString
	var createdAt string
	err := row.Scan(&item.OwnerPublicKey, &item.Id, &item.RulesetName, &petID, &gameResultID, &rewardGrantID, &item.Delta, &item.BalanceAfter, &item.Reason, &createdAt)
	if err != nil {
		return apitypes.PointsTransaction{}, err
	}
	item.PetId = nullStringPtr(petID)
	item.GameResultId = nullStringPtr(gameResultID)
	item.RewardGrantId = nullStringPtr(rewardGrantID)
	item.CreatedAt = parseTime(createdAt)
	return item, nil
}

func insertPointsTransaction(ctx context.Context, tx *sql.Tx, item apitypes.PointsTransaction) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO gameplay_points_transactions (owner_public_key, id, ruleset_name, pet_id, game_result_id, reward_grant_id, delta, balance_after, reason, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.OwnerPublicKey, item.Id, item.RulesetName, nullableString(item.PetId), nullableString(item.GameResultId), nullableString(item.RewardGrantId), item.Delta, item.BalanceAfter, item.Reason, formatTime(item.CreatedAt))
	return err
}

func badgeSelectSQL() string {
	return `SELECT owner_public_key, id, badge_def_id, exp, level, active, progress, created_at, updated_at FROM gameplay_badges`
}

func scanBadge(row rowScanner) (apitypes.Badge, error) {
	var item apitypes.Badge
	var active int
	var createdAt, updatedAt string
	if err := row.Scan(&item.OwnerPublicKey, &item.Id, &item.BadgeDefId, &item.Exp, &item.Level, &active, &item.Progress, &createdAt, &updatedAt); err != nil {
		return apitypes.Badge{}, err
	}
	item.Active = active != 0
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, nil
}

func upsertBadge(ctx context.Context, tx *sql.Tx, item apitypes.Badge) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO gameplay_badges (owner_public_key, id, badge_def_id, exp, level, active, progress, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(owner_public_key, id) DO UPDATE SET exp = excluded.exp, level = excluded.level, active = excluded.active, progress = excluded.progress, updated_at = excluded.updated_at`,
		item.OwnerPublicKey, item.Id, item.BadgeDefId, item.Exp, item.Level, boolInt(item.Active), item.Progress, formatTime(item.CreatedAt), formatTime(item.UpdatedAt))
	return err
}

func gameResultSelectSQL() string {
	return `SELECT owner_public_key, id, ruleset_name, pet_id, game_def_id, score, outcome, payload_json, created_at FROM gameplay_game_results`
}

func scanGameResult(row rowScanner) (apitypes.GameResult, error) {
	var item apitypes.GameResult
	var score sql.NullInt64
	var outcome, payloadJSON sql.NullString
	var createdAt string
	if err := row.Scan(&item.OwnerPublicKey, &item.Id, &item.RulesetName, &item.PetId, &item.GameDefId, &score, &outcome, &payloadJSON, &createdAt); err != nil {
		return apitypes.GameResult{}, err
	}
	if score.Valid {
		item.Score = &score.Int64
	}
	item.Outcome = nullStringPtr(outcome)
	if payloadJSON.Valid && payloadJSON.String != "" {
		var payload apitypes.GameplayMetadata
		if err := unmarshalJSON(payloadJSON.String, &payload); err != nil {
			return apitypes.GameResult{}, err
		}
		item.Payload = &payload
	}
	item.CreatedAt = parseTime(createdAt)
	return item, nil
}

func insertGameResult(ctx context.Context, tx *sql.Tx, item apitypes.GameResult) error {
	payloadJSON := sql.NullString{}
	if item.Payload != nil {
		data, err := marshalJSON(*item.Payload)
		if err != nil {
			return err
		}
		payloadJSON = sql.NullString{String: data, Valid: true}
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO gameplay_game_results (owner_public_key, id, ruleset_name, pet_id, game_def_id, score, outcome, payload_json, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.OwnerPublicKey, item.Id, item.RulesetName, item.PetId, item.GameDefId, nullableInt64(item.Score), nullableString(item.Outcome), payloadJSON, formatTime(item.CreatedAt))
	return err
}

func rewardGrantSelectSQL() string {
	return `SELECT owner_public_key, id, ruleset_name, pet_id, game_result_id, points_delta, pet_exp_delta, badge_exp_delta_json, reason, created_at FROM gameplay_reward_grants`
}

func scanRewardGrant(row rowScanner) (apitypes.RewardGrant, error) {
	var item apitypes.RewardGrant
	var petID, gameResultID, reason sql.NullString
	var badgeExpJSON string
	var createdAt string
	if err := row.Scan(&item.OwnerPublicKey, &item.Id, &item.RulesetName, &petID, &gameResultID, &item.PointsDelta, &item.PetExpDelta, &badgeExpJSON, &reason, &createdAt); err != nil {
		return apitypes.RewardGrant{}, err
	}
	item.PetId = nullStringPtr(petID)
	item.GameResultId = nullStringPtr(gameResultID)
	item.Reason = nullStringPtr(reason)
	if err := unmarshalJSON(badgeExpJSON, &item.BadgeExpDelta); err != nil {
		return apitypes.RewardGrant{}, err
	}
	item.CreatedAt = parseTime(createdAt)
	return item, nil
}

func insertRewardGrant(ctx context.Context, tx *sql.Tx, item apitypes.RewardGrant) error {
	badgeExpJSON, err := marshalJSON(item.BadgeExpDelta)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO gameplay_reward_grants (owner_public_key, id, ruleset_name, pet_id, game_result_id, points_delta, pet_exp_delta, badge_exp_delta_json, reason, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.OwnerPublicKey, item.Id, item.RulesetName, nullableString(item.PetId), nullableString(item.GameResultId), item.PointsDelta, item.PetExpDelta, badgeExpJSON, nullableString(item.Reason), formatTime(item.CreatedAt))
	return err
}

func listOwnerRows[T any](ctx context.Context, r *Runtime, owner, table string, req apitypes.GameplayListRequest, scan func(rowScanner) (T, error)) ([]T, bool, *string, error) {
	if err := requireOwner(owner); err != nil {
		return nil, false, nil, err
	}
	if err := r.Migration(ctx); err != nil {
		return nil, false, nil, err
	}
	cursor, limit := normalizeRuntimeListParams(req.Cursor, req.Limit)
	query := fmt.Sprintf(`SELECT * FROM %s WHERE owner_public_key = ?`, table)
	args := []any{owner}
	if cursor != "" {
		query += ` AND id > ?`
		args = append(args, cursor)
	}
	query += ` ORDER BY id LIMIT ?`
	args = append(args, limit+1)
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, nil, err
	}
	defer rows.Close()
	items := make([]T, 0, limit)
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, false, nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, nil, err
	}
	hasNext := len(items) > limit
	if hasNext {
		items = items[:limit]
	}
	var next *string
	if hasNext && len(items) > 0 {
		id := runtimeItemID(items[len(items)-1])
		next = &id
	}
	return items, hasNext, next, nil
}

func normalizeRuntimeListParams(cursor *string, limit *int) (string, int) {
	normalizedLimit := defaultListLimit
	if limit != nil && *limit > 0 {
		normalizedLimit = *limit
	}
	if normalizedLimit > maxListLimit {
		normalizedLimit = maxListLimit
	}
	return strings.TrimSpace(valueOrZero(cursor)), normalizedLimit
}

func runtimeItemID(item any) string {
	switch v := item.(type) {
	case apitypes.Pet:
		return v.Id
	case apitypes.Badge:
		return v.Id
	case apitypes.PointsTransaction:
		return v.Id
	case apitypes.GameResult:
		return v.Id
	case apitypes.RewardGrant:
		return v.Id
	default:
		return ""
	}
}

func marshalJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func unmarshalJSON(data string, out any) error {
	if strings.TrimSpace(data) == "" {
		data = "{}"
	}
	return json.Unmarshal([]byte(data), out)
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, value)
	return t
}

func nullableString(v *string) sql.NullString {
	if v == nil || strings.TrimSpace(*v) == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *v, Valid: true}
}

func nullStringPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func nullableInt64(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
