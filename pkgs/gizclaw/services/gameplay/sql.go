package gameplay

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/jmoiron/sqlx"
)

type rowScanner interface {
	Scan(dest ...any) error
}

type queryRebinder interface {
	Rebind(string) string
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type petAdoptionReservation struct {
	OwnerPublicKey     string
	PetID              string
	RuntimeProfileName string
	PetDefID           string
	DisplayName        string
	WorkspaceName      string
	WorkflowName       string
	VoiceAlias         string
	AdoptionCost       int64
	CreatedAt          time.Time
}

func findPetAdoptionReservation(ctx context.Context, db queryRebinder, owner, petID string) (petAdoptionReservation, error) {
	var reservation petAdoptionReservation
	var createdAt string
	err := db.QueryRowContext(ctx, db.Rebind(`SELECT owner_public_key, pet_id, runtime_profile_name, petdef_id, display_name, workspace_name, workflow_name, voice_alias, adoption_cost, created_at FROM gameplay_pet_adoption_reservations WHERE owner_public_key = ? AND pet_id = ?`), owner, petID).Scan(
		&reservation.OwnerPublicKey, &reservation.PetID, &reservation.RuntimeProfileName, &reservation.PetDefID,
		&reservation.DisplayName, &reservation.WorkspaceName, &reservation.WorkflowName, &reservation.VoiceAlias,
		&reservation.AdoptionCost, &createdAt,
	)
	if err != nil {
		return petAdoptionReservation{}, err
	}
	reservation.CreatedAt = parseTime(createdAt)
	return reservation, nil
}

func insertPetAdoptionReservation(ctx context.Context, tx *sqlx.Tx, reservation petAdoptionReservation) error {
	_, err := insertPetAdoptionReservationIfAbsent(ctx, tx, reservation)
	return err
}

func insertPetAdoptionReservationIfAbsent(ctx context.Context, tx *sqlx.Tx, reservation petAdoptionReservation) (bool, error) {
	result, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO gameplay_pet_adoption_reservations (owner_public_key, pet_id, runtime_profile_name, petdef_id, display_name, workspace_name, workflow_name, voice_alias, adoption_cost, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(owner_public_key, pet_id) DO NOTHING`),
		reservation.OwnerPublicKey, reservation.PetID, reservation.RuntimeProfileName, reservation.PetDefID,
		reservation.DisplayName, reservation.WorkspaceName, reservation.WorkflowName, reservation.VoiceAlias,
		reservation.AdoptionCost, formatTime(reservation.CreatedAt))
	if err != nil {
		return false, err
	}
	inserted, err := result.RowsAffected()
	return inserted == 1, err
}

func deletePetAdoptionReservationIfIncomplete(ctx context.Context, db *sqlx.DB, owner, petID string) (bool, error) {
	result, err := db.ExecContext(ctx, db.Rebind(`DELETE FROM gameplay_pet_adoption_reservations
		WHERE owner_public_key = ? AND pet_id = ?
		AND NOT EXISTS (SELECT 1 FROM gameplay_pets WHERE owner_public_key = ? AND id = ?)
		AND NOT EXISTS (SELECT 1 FROM gameplay_points_transactions WHERE owner_public_key = ? AND source_type = 'pet' AND source_id = ? AND reason = 'pet.adopt')`),
		owner, petID, owner, petID, owner, petID)
	if err != nil {
		return false, err
	}
	deleted, err := result.RowsAffected()
	return deleted == 1, err
}

type petDriveTick struct {
	OwnerPublicKey     string
	RuntimeProfileName string
	IdempotencyKey     string
	PetID              string
	CreatedAt          time.Time
}

func petSelectSQL() string {
	return `SELECT owner_public_key, id, runtime_profile_name, petdef_id, display_name, workspace_name, stats_json, progression_json, lifecycle, died_at, state_settled_at, last_active_at, created_at, updated_at FROM gameplay_pets`
}

func scanPet(row rowScanner) (apitypes.Pet, error) {
	var pet apitypes.Pet
	var statsJSON, progressionJSON string
	var diedAt sql.NullString
	var stateSettledAt, lastActiveAt, createdAt, updatedAt string
	err := row.Scan(&pet.OwnerPublicKey, &pet.Id, &pet.RuntimeProfileName, &pet.PetdefId, &pet.DisplayName, &pet.WorkspaceName, &statsJSON, &progressionJSON, &pet.Lifecycle, &diedAt, &stateSettledAt, &lastActiveAt, &createdAt, &updatedAt)
	if err != nil {
		return apitypes.Pet{}, err
	}
	if err := unmarshalJSON(statsJSON, &pet.Stats); err != nil {
		return apitypes.Pet{}, err
	}
	if err := unmarshalJSON(progressionJSON, &pet.Progression); err != nil {
		return apitypes.Pet{}, err
	}
	if diedAt.Valid {
		value := parseTime(diedAt.String)
		pet.DiedAt = &value
	}
	pet.StateSettledAt = parseTime(stateSettledAt)
	pet.LastActiveAt = parseTime(lastActiveAt)
	pet.CreatedAt = parseTime(createdAt)
	pet.UpdatedAt = parseTime(updatedAt)
	switch pet.Lifecycle {
	case apitypes.PetLifecycleAlive:
		if pet.DiedAt != nil {
			return apitypes.Pet{}, errors.New("gameplay: alive pet has died_at")
		}
	case apitypes.PetLifecycleDead:
		if pet.DiedAt == nil || pet.Stats.Life != 0 {
			return apitypes.Pet{}, errors.New("gameplay: dead pet requires died_at and zero life")
		}
	default:
		return apitypes.Pet{}, fmt.Errorf("gameplay: invalid pet lifecycle %q", pet.Lifecycle)
	}
	return pet, nil
}

func findPetByOwnerID(ctx context.Context, db queryRebinder, owner, id string) (apitypes.Pet, error) {
	return scanPet(db.QueryRowContext(ctx, db.Rebind(petSelectSQL()+` WHERE owner_public_key = ? AND id = ?`), owner, id))
}

func insertPet(ctx context.Context, tx *sqlx.Tx, pet apitypes.Pet) error {
	statsJSON, err := marshalJSON(pet.Stats)
	if err != nil {
		return err
	}
	progressionJSON, err := marshalJSON(pet.Progression)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO gameplay_pets (owner_public_key, id, runtime_profile_name, petdef_id, display_name, workspace_name, stats_json, progression_json, lifecycle, died_at, state_settled_at, last_active_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		pet.OwnerPublicKey, pet.Id, pet.RuntimeProfileName, pet.PetdefId, pet.DisplayName, pet.WorkspaceName, statsJSON, progressionJSON, pet.Lifecycle, nullableTime(pet.DiedAt), formatTime(pet.StateSettledAt), formatTime(pet.LastActiveAt), formatTime(pet.CreatedAt), formatTime(pet.UpdatedAt))
	return err
}

func updatePet(ctx context.Context, tx *sqlx.Tx, pet apitypes.Pet) error {
	statsJSON, err := marshalJSON(pet.Stats)
	if err != nil {
		return err
	}
	progressionJSON, err := marshalJSON(pet.Progression)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE gameplay_pets SET display_name = ?, stats_json = ?, progression_json = ?, lifecycle = ?, died_at = ?, state_settled_at = ?, last_active_at = ?, updated_at = ? WHERE owner_public_key = ? AND id = ?`),
		pet.DisplayName, statsJSON, progressionJSON, pet.Lifecycle, nullableTime(pet.DiedAt), formatTime(pet.StateSettledAt), formatTime(pet.LastActiveAt), formatTime(pet.UpdatedAt), pet.OwnerPublicKey, pet.Id)
	return err
}

func petDriveTickSelectSQL() string {
	return `SELECT owner_public_key, runtime_profile_name, idempotency_key, pet_id, created_at FROM gameplay_pet_drive_ticks`
}

func scanPetDriveTick(row rowScanner) (petDriveTick, error) {
	var tick petDriveTick
	var createdAt string
	if err := row.Scan(&tick.OwnerPublicKey, &tick.RuntimeProfileName, &tick.IdempotencyKey, &tick.PetID, &createdAt); err != nil {
		return petDriveTick{}, err
	}
	tick.CreatedAt = parseTime(createdAt)
	return tick, nil
}

func findPetDriveTick(ctx context.Context, db queryRebinder, owner, runtimeProfileName, key string) (petDriveTick, error) {
	return scanPetDriveTick(db.QueryRowContext(ctx, db.Rebind(petDriveTickSelectSQL()+` WHERE owner_public_key = ? AND runtime_profile_name = ? AND idempotency_key = ?`), owner, runtimeProfileName, strings.TrimSpace(key)))
}

func insertPetDriveTick(ctx context.Context, tx *sqlx.Tx, tick petDriveTick) (bool, error) {
	result, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO gameplay_pet_drive_ticks (owner_public_key, runtime_profile_name, idempotency_key, pet_id, created_at) VALUES (?, ?, ?, ?, ?) ON CONFLICT(owner_public_key, runtime_profile_name, idempotency_key) DO NOTHING`),
		tick.OwnerPublicKey, tick.RuntimeProfileName, strings.TrimSpace(tick.IdempotencyKey), tick.PetID, formatTime(tick.CreatedAt))
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func pointsAccountSelectSQL() string {
	return `SELECT owner_public_key, runtime_profile_name, balance, created_at, updated_at FROM gameplay_points_accounts`
}

func scanPointsAccount(row rowScanner) (apitypes.PointsAccount, error) {
	var account apitypes.PointsAccount
	var createdAt, updatedAt string
	if err := row.Scan(&account.OwnerPublicKey, &account.RuntimeProfileName, &account.Balance, &createdAt, &updatedAt); err != nil {
		return apitypes.PointsAccount{}, err
	}
	account.CreatedAt = parseTime(createdAt)
	account.UpdatedAt = parseTime(updatedAt)
	return account, nil
}

func findPointsAccount(ctx context.Context, db queryRebinder, owner, runtimeProfileName string) (apitypes.PointsAccount, error) {
	return scanPointsAccount(db.QueryRowContext(ctx, db.Rebind(pointsAccountSelectSQL()+` WHERE owner_public_key = ? AND runtime_profile_name = ?`), owner, runtimeProfileName))
}

func insertPointsAccount(ctx context.Context, tx *sqlx.Tx, account apitypes.PointsAccount) (bool, error) {
	result, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO gameplay_points_accounts (owner_public_key, runtime_profile_name, balance, created_at, updated_at) VALUES (?, ?, ?, ?, ?) ON CONFLICT(owner_public_key, runtime_profile_name) DO NOTHING`),
		account.OwnerPublicKey, account.RuntimeProfileName, account.Balance, formatTime(account.CreatedAt), formatTime(account.UpdatedAt))
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func pointsTransactionSelectSQL() string {
	return `SELECT owner_public_key, id, runtime_profile_name, pet_id, game_result_id, reward_grant_id, delta, balance_after, reason, source_type, source_id, created_at FROM gameplay_points_transactions`
}

func scanPointsTransaction(row rowScanner) (apitypes.PointsTransaction, error) {
	var item apitypes.PointsTransaction
	var petID, gameResultID, rewardGrantID sql.NullString
	var createdAt string
	err := row.Scan(&item.OwnerPublicKey, &item.Id, &item.RuntimeProfileName, &petID, &gameResultID, &rewardGrantID, &item.Delta, &item.BalanceAfter, &item.Reason, &item.SourceType, &item.SourceId, &createdAt)
	if err != nil {
		return apitypes.PointsTransaction{}, err
	}
	item.PetId = nullStringPtr(petID)
	item.GameResultId = nullStringPtr(gameResultID)
	item.RewardGrantId = nullStringPtr(rewardGrantID)
	item.CreatedAt = parseTime(createdAt)
	return item, nil
}

func insertPointsTransaction(ctx context.Context, tx *sqlx.Tx, item apitypes.PointsTransaction) error {
	_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO gameplay_points_transactions (owner_public_key, id, runtime_profile_name, pet_id, game_result_id, reward_grant_id, delta, balance_after, reason, source_type, source_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		item.OwnerPublicKey, item.Id, item.RuntimeProfileName, nullableString(item.PetId), nullableString(item.GameResultId), nullableString(item.RewardGrantId), item.Delta, item.BalanceAfter, item.Reason, item.SourceType, item.SourceId, formatTime(item.CreatedAt))
	return err
}

func findPetAdoptionTransaction(ctx context.Context, db queryRebinder, owner, petID string) (apitypes.PointsTransaction, error) {
	return scanPointsTransaction(db.QueryRowContext(ctx, db.Rebind(pointsTransactionSelectSQL()+` WHERE owner_public_key = ? AND source_type = 'pet' AND source_id = ? AND reason = 'pet.adopt'`), owner, petID))
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

func upsertBadge(ctx context.Context, tx *sqlx.Tx, item apitypes.Badge) error {
	_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO gameplay_badges (owner_public_key, id, badge_def_id, exp, level, active, progress, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(owner_public_key, id) DO UPDATE SET exp = excluded.exp, level = excluded.level, active = excluded.active, progress = excluded.progress, updated_at = excluded.updated_at`),
		item.OwnerPublicKey, item.Id, item.BadgeDefId, item.Exp, item.Level, boolInt(item.Active), item.Progress, formatTime(item.CreatedAt), formatTime(item.UpdatedAt))
	return err
}

func gameResultSelectSQL() string {
	return `SELECT owner_public_key, id, runtime_profile_name, pet_id, game_def_id, score, max_score, difficulty, outcome, duration_ms, idempotency_key, payload_json, occurred_at, created_at FROM gameplay_game_results`
}

func scanGameResult(row rowScanner) (apitypes.GameResult, error) {
	var item apitypes.GameResult
	var score, maxScore, durationMs sql.NullInt64
	var difficulty, outcome, idempotencyKey, payloadJSON sql.NullString
	var occurredAt, createdAt string
	if err := row.Scan(&item.OwnerPublicKey, &item.Id, &item.RuntimeProfileName, &item.PetId, &item.GameDefId, &score, &maxScore, &difficulty, &outcome, &durationMs, &idempotencyKey, &payloadJSON, &occurredAt, &createdAt); err != nil {
		return apitypes.GameResult{}, err
	}
	if score.Valid {
		item.Score = &score.Int64
	}
	if maxScore.Valid {
		item.MaxScore = &maxScore.Int64
	}
	item.Difficulty = nullStringPtr(difficulty)
	item.Outcome = nullStringPtr(outcome)
	if durationMs.Valid {
		item.DurationMs = &durationMs.Int64
	}
	item.IdempotencyKey = nullStringPtr(idempotencyKey)
	if payloadJSON.Valid && payloadJSON.String != "" {
		var payload apitypes.GameplayMetadata
		if err := unmarshalJSON(payloadJSON.String, &payload); err != nil {
			return apitypes.GameResult{}, err
		}
		item.Payload = &payload
	}
	item.OccurredAt = parseTime(occurredAt)
	item.CreatedAt = parseTime(createdAt)
	if item.OccurredAt.IsZero() {
		item.OccurredAt = item.CreatedAt
	}
	return item, nil
}

func insertGameResult(ctx context.Context, tx *sqlx.Tx, item apitypes.GameResult) error {
	payloadJSON := sql.NullString{}
	if item.Payload != nil {
		data, err := marshalJSON(*item.Payload)
		if err != nil {
			return err
		}
		payloadJSON = sql.NullString{String: data, Valid: true}
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO gameplay_game_results (owner_public_key, id, runtime_profile_name, pet_id, game_def_id, score, max_score, difficulty, outcome, duration_ms, idempotency_key, payload_json, occurred_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		item.OwnerPublicKey, item.Id, item.RuntimeProfileName, item.PetId, item.GameDefId, nullableInt64(item.Score), nullableInt64(item.MaxScore), nullableString(item.Difficulty), nullableString(item.Outcome), nullableInt64(item.DurationMs), nullableString(item.IdempotencyKey), payloadJSON, formatTime(item.OccurredAt), formatTime(item.CreatedAt))
	return err
}

func findGameResultByIdempotencyKey(ctx context.Context, tx *sqlx.Tx, owner, runtimeProfileName, key string) (apitypes.GameResult, error) {
	return scanGameResult(tx.QueryRowContext(ctx, tx.Rebind(gameResultSelectSQL()+` WHERE owner_public_key = ? AND runtime_profile_name = ? AND idempotency_key = ?`), owner, runtimeProfileName, strings.TrimSpace(key)))
}

func rewardGrantSelectSQL() string {
	return `SELECT owner_public_key, id, runtime_profile_name, pet_id, game_result_id, points_delta, pet_exp_delta, badge_exp_delta_json, source_type, source_id, reason, created_at FROM gameplay_reward_grants`
}

func scanRewardGrant(row rowScanner) (apitypes.RewardGrant, error) {
	var item apitypes.RewardGrant
	var petID, gameResultID, reason sql.NullString
	var badgeExpJSON string
	var createdAt string
	if err := row.Scan(&item.OwnerPublicKey, &item.Id, &item.RuntimeProfileName, &petID, &gameResultID, &item.PointsDelta, &item.PetExpDelta, &badgeExpJSON, &item.SourceType, &item.SourceId, &reason, &createdAt); err != nil {
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

func insertRewardGrant(ctx context.Context, tx *sqlx.Tx, item apitypes.RewardGrant) error {
	badgeExpJSON, err := marshalJSON(item.BadgeExpDelta)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO gameplay_reward_grants (owner_public_key, id, runtime_profile_name, pet_id, game_result_id, points_delta, pet_exp_delta, badge_exp_delta_json, source_type, source_id, reason, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		item.OwnerPublicKey, item.Id, item.RuntimeProfileName, nullableString(item.PetId), nullableString(item.GameResultId), item.PointsDelta, item.PetExpDelta, badgeExpJSON, item.SourceType, item.SourceId, nullableString(item.Reason), formatTime(item.CreatedAt))
	return err
}

func listOwnerRows[T any](ctx context.Context, r *Runtime, owner, table string, profileScoped bool, req apitypes.GameplayListRequest, scan func(rowScanner) (T, error)) ([]T, bool, *string, error) {
	if err := requireOwner(owner); err != nil {
		return nil, false, nil, err
	}
	if err := r.Migration(ctx); err != nil {
		return nil, false, nil, err
	}
	cursor, limit := normalizeRuntimeListParams(req.Cursor, req.Limit)
	query := fmt.Sprintf(`SELECT * FROM %s WHERE owner_public_key = ?`, table)
	args := []any{owner}
	if profile, registered := runtimeProfileFromContext(ctx); registered && profileScoped {
		profileName := strings.TrimSpace(profile.Name)
		if profileName == "" {
			return nil, false, nil, errors.New("gameplay: RuntimeProfile is required")
		}
		query += ` AND runtime_profile_name = ?`
		args = append(args, profileName)
	}
	if cursor != "" {
		query += ` AND id > ?`
		args = append(args, cursor)
	}
	query += ` ORDER BY id LIMIT ?`
	args = append(args, limit+1)
	rows, err := r.DB.QueryContext(ctx, r.DB.Rebind(query), args...)
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

func nullableTime(v *time.Time) sql.NullString {
	if v == nil || v.IsZero() {
		return sql.NullString{}
	}
	return sql.NullString{String: formatTime(*v), Valid: true}
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
