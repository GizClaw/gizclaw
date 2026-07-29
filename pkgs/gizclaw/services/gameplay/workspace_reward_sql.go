package gameplay

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/jmoiron/sqlx"
)

func workspaceRewardWindowColumns() string {
	return `id, workspace_name, workspace_kind, beneficiary_public_key, runtime_profile_name,
		runtime_profile_revision, policy_json, policy_digest, start_history_id, high_water_history_id,
		start_history_at, high_water_history_at, opened_at, last_activity_at, evaluate_after, state,
		attempt_count, next_attempt_at, claim_token, claim_until, transcript_digest, outcome, last_error,
		created_at, updated_at`
}

func workspaceRewardWindowSelectSQL() string {
	return `SELECT ` + workspaceRewardWindowColumns() + ` FROM gameplay_workspace_reward_windows`
}

func (r *Runtime) ensureWorkspaceRewardActivation(ctx context.Context) (time.Time, error) {
	db, err := r.db()
	if err != nil {
		return time.Time{}, err
	}
	now := r.now()
	if _, err := db.ExecContext(ctx, db.Rebind(`INSERT INTO gameplay_workspace_reward_activation
		(singleton, activated_at) VALUES (1, ?) ON CONFLICT(singleton) DO NOTHING`), formatTime(now)); err != nil {
		return time.Time{}, err
	}
	var raw string
	if err := db.QueryRowContext(ctx, `SELECT activated_at FROM gameplay_workspace_reward_activation WHERE singleton = 1`).Scan(&raw); err != nil {
		return time.Time{}, err
	}
	activation := parseTime(raw)
	if activation.IsZero() {
		return time.Time{}, errors.New("gameplay: invalid workspace reward activation boundary")
	}
	return activation, nil
}

func scanWorkspaceRewardWindow(row rowScanner) (workspaceRewardWindow, error) {
	var window workspaceRewardWindow
	var policyJSON string
	var startAt, highWaterAt, openedAt, lastActivityAt, evaluateAfter string
	var nextAttemptAt, claimUntil, createdAt, updatedAt string
	err := row.Scan(
		&window.ID, &window.WorkspaceName, &window.WorkspaceKind, &window.BeneficiaryPublicKey,
		&window.RuntimeProfileName, &window.RuntimeProfileRevision, &policyJSON, &window.PolicyDigest,
		&window.StartHistoryID, &window.HighWaterHistoryID, &startAt, &highWaterAt, &openedAt,
		&lastActivityAt, &evaluateAfter, &window.State, &window.AttemptCount, &nextAttemptAt,
		&window.ClaimToken, &claimUntil, &window.TranscriptDigest, &window.Outcome, &window.LastError,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return workspaceRewardWindow{}, err
	}
	if err := unmarshalJSON(policyJSON, &window.Policy); err != nil {
		return workspaceRewardWindow{}, fmt.Errorf("gameplay: decode workspace reward policy: %w", err)
	}
	digest, err := workspaceRewardPolicyDigest(window.Policy)
	if err != nil {
		return workspaceRewardWindow{}, err
	}
	if digest != window.PolicyDigest {
		return workspaceRewardWindow{}, errors.New("gameplay: workspace reward policy digest mismatch")
	}
	window.Policy.Digest = digest
	window.StartHistoryAt = parseTime(startAt)
	window.HighWaterHistoryAt = parseTime(highWaterAt)
	window.OpenedAt = parseTime(openedAt)
	window.LastActivityAt = parseTime(lastActivityAt)
	window.EvaluateAfter = parseTime(evaluateAfter)
	window.NextAttemptAt = parseTime(nextAttemptAt)
	window.ClaimUntil = parseTime(claimUntil)
	window.CreatedAt = parseTime(createdAt)
	window.UpdatedAt = parseTime(updatedAt)
	return window, nil
}

func (r *Runtime) getWorkspaceRewardSource(ctx context.Context, workspaceName string) (workspaceRewardSource, error) {
	db, err := r.db()
	if err != nil {
		return workspaceRewardSource{}, err
	}
	var source workspaceRewardSource
	var createdAt, updatedAt string
	err = db.QueryRowContext(ctx, db.Rebind(`SELECT workspace_name, scheduled_checkpoint, completed_checkpoint, created_at, updated_at
		FROM gameplay_workspace_reward_sources WHERE workspace_name = ?`), workspaceName).Scan(
		&source.WorkspaceName, &source.ScheduledCheckpoint, &source.CompletedCheckpoint, &createdAt, &updatedAt,
	)
	if err != nil {
		return workspaceRewardSource{}, err
	}
	source.CreatedAt = parseTime(createdAt)
	source.UpdatedAt = parseTime(updatedAt)
	return source, nil
}

func (r *Runtime) insertWorkspaceRewardSource(ctx context.Context, source workspaceRewardSource) error {
	db, err := r.db()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, db.Rebind(`INSERT INTO gameplay_workspace_reward_sources
		(workspace_name, scheduled_checkpoint, completed_checkpoint, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?) ON CONFLICT(workspace_name) DO NOTHING`),
		source.WorkspaceName, source.ScheduledCheckpoint, source.CompletedCheckpoint,
		formatTime(source.CreatedAt), formatTime(source.UpdatedAt),
	)
	return err
}

func (r *Runtime) updateWorkspaceRewardSource(ctx context.Context, source workspaceRewardSource) error {
	db, err := r.db()
	if err != nil {
		return err
	}
	result, err := db.ExecContext(ctx, db.Rebind(`UPDATE gameplay_workspace_reward_sources
		SET scheduled_checkpoint = ?, completed_checkpoint = ?, updated_at = ? WHERE workspace_name = ?`),
		source.ScheduledCheckpoint, source.CompletedCheckpoint, formatTime(source.UpdatedAt), source.WorkspaceName,
	)
	if err != nil {
		return err
	}
	return requireWorkspaceRewardRow(result, "source changed while updating")
}

func (r *Runtime) activeWorkspaceRewardWindow(ctx context.Context, workspaceName string) (workspaceRewardWindow, error) {
	db, err := r.db()
	if err != nil {
		return workspaceRewardWindow{}, err
	}
	return scanWorkspaceRewardWindow(db.QueryRowContext(ctx, db.Rebind(
		workspaceRewardWindowSelectSQL()+` WHERE workspace_name = ? AND state IN (?, ?, ?) ORDER BY created_at LIMIT 1`,
	), workspaceName, workspaceRewardPending, workspaceRewardClaimed, workspaceRewardRetry))
}

func (r *Runtime) insertWorkspaceRewardWindowAndUpdateSource(
	ctx context.Context,
	window workspaceRewardWindow,
	source workspaceRewardSource,
) error {
	db, err := r.db()
	if err != nil {
		return err
	}
	policyJSON, err := marshalJSON(window.Policy)
	if err != nil {
		return err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO gameplay_workspace_reward_windows
		(id, workspace_name, workspace_kind, beneficiary_public_key, runtime_profile_name,
		runtime_profile_revision, policy_json, policy_digest, start_history_id, high_water_history_id,
		start_history_at, high_water_history_at, opened_at, last_activity_at, evaluate_after, state,
		attempt_count, next_attempt_at, claim_token, claim_until, transcript_digest, outcome, last_error,
		created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		window.ID, window.WorkspaceName, window.WorkspaceKind, window.BeneficiaryPublicKey,
		window.RuntimeProfileName, window.RuntimeProfileRevision, policyJSON, window.PolicyDigest,
		window.StartHistoryID, window.HighWaterHistoryID, formatTime(window.StartHistoryAt),
		formatTime(window.HighWaterHistoryAt), formatTime(window.OpenedAt), formatTime(window.LastActivityAt),
		formatTime(window.EvaluateAfter), window.State, window.AttemptCount, formatTime(window.NextAttemptAt),
		window.ClaimToken, formatTime(window.ClaimUntil), window.TranscriptDigest, window.Outcome,
		window.LastError, formatTime(window.CreatedAt), formatTime(window.UpdatedAt),
	)
	if err != nil {
		return err
	}
	if err := updateWorkspaceRewardSourceTx(ctx, tx, source); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Runtime) updateWorkspaceRewardWindowAndSource(
	ctx context.Context,
	window workspaceRewardWindow,
	source workspaceRewardSource,
) error {
	db, err := r.db()
	if err != nil {
		return err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE gameplay_workspace_reward_windows
		SET high_water_history_id = ?, high_water_history_at = ?, last_activity_at = ?,
			evaluate_after = ?, updated_at = ?
		WHERE id = ? AND state = ?`),
		window.HighWaterHistoryID, formatTime(window.HighWaterHistoryAt), formatTime(window.LastActivityAt),
		formatTime(window.EvaluateAfter), formatTime(window.UpdatedAt), window.ID, workspaceRewardPending,
	)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return errors.New("gameplay: workspace reward window changed while scheduling")
	}
	if err := updateWorkspaceRewardSourceTx(ctx, tx, source); err != nil {
		return err
	}
	return tx.Commit()
}

func updateWorkspaceRewardSourceTx(ctx context.Context, tx *sqlx.Tx, source workspaceRewardSource) error {
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE gameplay_workspace_reward_sources
		SET scheduled_checkpoint = ?, completed_checkpoint = ?, updated_at = ? WHERE workspace_name = ?`),
		source.ScheduledCheckpoint, source.CompletedCheckpoint, formatTime(source.UpdatedAt), source.WorkspaceName,
	)
	if err != nil {
		return err
	}
	return requireWorkspaceRewardRow(result, "source changed while scheduling")
}

func (r *Runtime) claimWorkspaceRewardWindow(ctx context.Context) (workspaceRewardWindow, bool, error) {
	db, err := r.db()
	if err != nil {
		return workspaceRewardWindow{}, false, err
	}
	now := r.now()
	token := r.newID()
	query := `UPDATE gameplay_workspace_reward_windows SET
		state = ?, attempt_count = attempt_count + 1, claim_token = ?, claim_until = ?, updated_at = ?
		WHERE id = (
			SELECT id FROM gameplay_workspace_reward_windows
			WHERE (state = ? AND evaluate_after <= ?)
			   OR (state = ? AND next_attempt_at <= ?)
			   OR (state = ? AND claim_until <= ?)
			ORDER BY evaluate_after, created_at LIMIT 1
		)
		AND ((state = ? AND evaluate_after <= ?)
		  OR (state = ? AND next_attempt_at <= ?)
		  OR (state = ? AND claim_until <= ?))
		RETURNING ` + workspaceRewardWindowColumns()
	window, err := scanWorkspaceRewardWindow(db.QueryRowContext(ctx, db.Rebind(query),
		workspaceRewardClaimed, token, formatTime(now.Add(workspaceRewardClaimLease)), formatTime(now),
		workspaceRewardPending, formatTime(now),
		workspaceRewardRetry, formatTime(now),
		workspaceRewardClaimed, formatTime(now),
		workspaceRewardPending, formatTime(now),
		workspaceRewardRetry, formatTime(now),
		workspaceRewardClaimed, formatTime(now),
	))
	if errors.Is(err, sql.ErrNoRows) {
		return workspaceRewardWindow{}, false, nil
	}
	if err != nil {
		return workspaceRewardWindow{}, false, err
	}
	return window, true, nil
}

func (r *Runtime) setWorkspaceRewardTranscriptDigest(ctx context.Context, window workspaceRewardWindow, digest string) error {
	db, err := r.db()
	if err != nil {
		return err
	}
	result, err := db.ExecContext(ctx, db.Rebind(`UPDATE gameplay_workspace_reward_windows
		SET transcript_digest = ?, updated_at = ?
		WHERE id = ? AND state = ? AND claim_token = ? AND (transcript_digest = '' OR transcript_digest = ?)`),
		digest, formatTime(r.now()), window.ID, workspaceRewardClaimed, window.ClaimToken, digest,
	)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return errors.New("gameplay: workspace reward transcript digest conflict")
	}
	return nil
}

func (r *Runtime) completeWorkspaceRewardWithoutGrant(
	ctx context.Context,
	window workspaceRewardWindow,
	digest, outcome string,
) error {
	db, err := r.db()
	if err != nil {
		return err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := completeWorkspaceRewardWindowTx(ctx, tx, window, digest, outcome, r.now()); err != nil {
		return err
	}
	return tx.Commit()
}

func completeWorkspaceRewardWindowTx(
	ctx context.Context,
	tx *sqlx.Tx,
	window workspaceRewardWindow,
	digest, outcome string,
	now time.Time,
) error {
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE gameplay_workspace_reward_windows SET
		state = ?, transcript_digest = ?, outcome = ?, claim_token = '', claim_until = '',
		last_error = '', updated_at = ?
		WHERE id = ? AND state = ? AND claim_token = ?`),
		workspaceRewardCompleted, digest, outcome, formatTime(now), window.ID,
		workspaceRewardClaimed, window.ClaimToken,
	)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return errors.New("gameplay: workspace reward claim is no longer owned")
	}
	result, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE gameplay_workspace_reward_sources
		SET completed_checkpoint = ?, updated_at = ? WHERE workspace_name = ?`),
		window.HighWaterHistoryID, formatTime(now), window.WorkspaceName,
	)
	if err != nil {
		return err
	}
	return requireWorkspaceRewardRow(result, "source is unavailable while completing")
}

func (r *Runtime) settleWorkspaceReward(
	ctx context.Context,
	window workspaceRewardWindow,
	transcriptDigest string,
	evaluation workspaceRewardEvaluation,
) (apitypes.RewardGrant, bool, error) {
	lock := r.workspaceRewardMutexForSettlement(window.BeneficiaryPublicKey, window.RuntimeProfileName)
	lock.Lock()
	defer lock.Unlock()
	db, err := r.db()
	if err != nil {
		return apitypes.RewardGrant{}, false, err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return apitypes.RewardGrant{}, false, err
	}
	defer tx.Rollback()
	current, err := scanWorkspaceRewardWindow(tx.QueryRowContext(ctx, tx.Rebind(
		workspaceRewardWindowSelectSQL()+` WHERE id = ? AND state = ? AND claim_token = ?`,
	), window.ID, workspaceRewardClaimed, window.ClaimToken))
	if err != nil {
		return apitypes.RewardGrant{}, false, err
	}
	account, err := r.ensureAccountTx(ctx, tx, window.BeneficiaryPublicKey, ProfileRules{
		Name: window.RuntimeProfileName,
		Spec: ProfileRulesSpec{Points: &apitypes.RuntimeProfilePointsSpec{InitialBalance: &window.Policy.InitialPointsBalance}},
	})
	if err != nil {
		return apitypes.RewardGrant{}, false, err
	}
	if err := lockPointsAccountTx(ctx, tx, &account); err != nil {
		return apitypes.RewardGrant{}, false, err
	}
	usedPoints, usedBadgeExp, err := workspaceRewardRollingUsage(
		ctx, tx, window.BeneficiaryPublicKey, window.RuntimeProfileName,
		window.PolicyDigest, r.now().Add(-window.Policy.BudgetPeriod),
	)
	if err != nil {
		return apitypes.RewardGrant{}, false, err
	}
	pointsDelta := workspaceRewardPointsDelta(evaluation.Score, window.Policy)
	pointsDelta = min(pointsDelta, max(int64(0), window.Policy.PointsMax-usedPoints))
	badgeDelta := workspaceRewardBadgeDelta(evaluation, window.Policy)
	remainingBadgeExp := max(int64(0), window.Policy.BadgeExpMax-usedBadgeExp)
	for _, badgeID := range sortedWorkspaceRewardBadges(badgeDelta) {
		value := min(badgeDelta[badgeID], remainingBadgeExp)
		badgeDelta[badgeID] = value
		remainingBadgeExp -= value
	}
	for badgeID, value := range badgeDelta {
		if value == 0 {
			delete(badgeDelta, badgeID)
		}
	}
	if pointsDelta == 0 && len(badgeDelta) == 0 {
		if err := completeWorkspaceRewardWindowTx(ctx, tx, current, transcriptDigest, "budget_exhausted_or_non_qualifying", r.now()); err != nil {
			return apitypes.RewardGrant{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return apitypes.RewardGrant{}, false, err
		}
		return apitypes.RewardGrant{}, false, nil
	}
	now := r.now()
	reason := strings.TrimSpace(evaluation.Reason)
	grant := apitypes.RewardGrant{
		Id: r.newID(), OwnerPublicKey: window.BeneficiaryPublicKey,
		RuntimeProfileName: window.RuntimeProfileName, PointsDelta: pointsDelta,
		PetExpDelta: 0, BadgeExpDelta: badgeDelta, SourceType: "workspace_history_window",
		SourceId: window.ID, Reason: &reason, CreatedAt: now,
	}
	badgeJSON, err := marshalJSON(grant.BadgeExpDelta)
	if err != nil {
		return apitypes.RewardGrant{}, false, err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO gameplay_reward_grants
		(owner_public_key, id, runtime_profile_name, pet_id, game_result_id, points_delta,
		pet_exp_delta, badge_exp_delta_json, source_type, source_id, policy_digest, reason, created_at)
		VALUES (?, ?, ?, NULL, NULL, ?, 0, ?, ?, ?, ?, ?, ?)`),
		grant.OwnerPublicKey, grant.Id, grant.RuntimeProfileName, grant.PointsDelta, badgeJSON,
		grant.SourceType, grant.SourceId, window.PolicyDigest, reason, formatTime(now),
	)
	if err != nil {
		return apitypes.RewardGrant{}, false, err
	}
	if pointsDelta > 0 {
		if _, err := r.applyPointsTx(ctx, tx, &account, pointsDelta, window.RuntimeProfileName, "", "", grant.Id, "workspace.conversation.reward", grant.SourceType, grant.SourceId); err != nil {
			return apitypes.RewardGrant{}, false, err
		}
	}
	for _, badgeID := range sortedWorkspaceRewardBadges(badgeDelta) {
		if _, err := r.applyBadgeExp(ctx, tx, window.BeneficiaryPublicKey, badgeID, badgeDelta[badgeID], now); err != nil {
			return apitypes.RewardGrant{}, false, err
		}
	}
	if err := completeWorkspaceRewardWindowTx(ctx, tx, current, transcriptDigest, "rewarded", now); err != nil {
		return apitypes.RewardGrant{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return apitypes.RewardGrant{}, false, err
	}
	return grant, true, nil
}

func workspaceRewardRollingUsage(
	ctx context.Context,
	tx *sqlx.Tx,
	owner, profile, policyDigest string,
	since time.Time,
) (int64, int64, error) {
	rows, err := tx.QueryContext(ctx, tx.Rebind(`SELECT points_delta, badge_exp_delta_json
		FROM gameplay_reward_grants
		WHERE owner_public_key = ? AND runtime_profile_name = ? AND source_type = ?
		  AND policy_digest = ? AND created_at >= ?`),
		owner, profile, "workspace_history_window", policyDigest, formatTime(since),
	)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	var points, badgeExp int64
	for rows.Next() {
		var delta int64
		var badgeJSON string
		if err := rows.Scan(&delta, &badgeJSON); err != nil {
			return 0, 0, err
		}
		values := map[string]int64{}
		if err := unmarshalJSON(badgeJSON, &values); err != nil {
			return 0, 0, err
		}
		points += delta
		badgeExp += sumBadgeExp(values)
	}
	return points, badgeExp, rows.Err()
}

func workspaceRewardPointsDelta(score int64, policy WorkspaceRewardPolicySnapshot) int64 {
	if score < policy.QualifyingScore {
		return 0
	}
	var delta int64
	for _, tier := range policy.PointsTiers {
		if score < tier.MinScore {
			break
		}
		delta = tier.Delta
	}
	return delta
}

func workspaceRewardBadgeDelta(
	evaluation workspaceRewardEvaluation,
	policy WorkspaceRewardPolicySnapshot,
) map[string]int64 {
	if evaluation.Score < policy.QualifyingScore {
		return map[string]int64{}
	}
	resourceIDs := make(map[string]string, len(policy.Badges))
	for _, badge := range policy.Badges {
		resourceIDs[badge.Alias] = badge.ResourceID
	}
	values := make(map[string]int64, len(evaluation.Badges))
	for _, badge := range evaluation.Badges {
		if badge.Exp > 0 {
			values[resourceIDs[badge.Alias]] = badge.Exp
		}
	}
	return values
}

func (r *Runtime) retryWorkspaceRewardWindow(ctx context.Context, window workspaceRewardWindow, cause error) error {
	db, err := r.db()
	if err != nil {
		return err
	}
	now := r.now()
	errorClass := safeWorkspaceRewardErrorClass(cause)
	result, err := db.ExecContext(ctx, db.Rebind(`UPDATE gameplay_workspace_reward_windows SET
		state = ?, next_attempt_at = ?, claim_token = '', claim_until = '', last_error = ?, updated_at = ?
		WHERE id = ? AND state = ? AND claim_token = ?`),
		workspaceRewardRetry, formatTime(now.Add(workspaceRewardRetryDelay(window.AttemptCount))),
		errorClass, formatTime(now), window.ID, workspaceRewardClaimed, window.ClaimToken,
	)
	if err != nil {
		return err
	}
	if err := requireWorkspaceRewardRow(result, "claim is no longer owned while retrying"); err != nil {
		return err
	}
	slog.Warn("workspace reward deferred",
		"workspace", window.WorkspaceName,
		"beneficiary", window.BeneficiaryPublicKey,
		"profile", window.RuntimeProfileName,
		"policy_digest", window.PolicyDigest,
		"window", window.ID,
		"attempt", window.AttemptCount,
		"state", workspaceRewardRetry,
		"error_class", errorClass,
	)
	return nil
}

func (r *Runtime) blockWorkspaceRewardWindow(ctx context.Context, window workspaceRewardWindow, cause error) error {
	db, err := r.db()
	if err != nil {
		return err
	}
	now := r.now()
	errorClass := safeWorkspaceRewardErrorClass(cause)
	result, err := db.ExecContext(ctx, db.Rebind(`UPDATE gameplay_workspace_reward_windows SET
		state = ?, claim_token = '', claim_until = '', last_error = ?, updated_at = ?
		WHERE id = ? AND state = ? AND claim_token = ?`),
		workspaceRewardBlocked, errorClass, formatTime(now),
		window.ID, workspaceRewardClaimed, window.ClaimToken,
	)
	if err != nil {
		return err
	}
	if err := requireWorkspaceRewardRow(result, "claim is no longer owned while blocking"); err != nil {
		return err
	}
	slog.Error("workspace reward blocked",
		"workspace", window.WorkspaceName,
		"beneficiary", window.BeneficiaryPublicKey,
		"profile", window.RuntimeProfileName,
		"policy_digest", window.PolicyDigest,
		"window", window.ID,
		"attempt", window.AttemptCount,
		"state", workspaceRewardBlocked,
		"error_class", errorClass,
	)
	return nil
}

func (r *Runtime) releaseWorkspaceRewardClaims(ctx context.Context) {
	db, err := r.db()
	if err != nil {
		return
	}
	now := r.now()
	_, _ = db.ExecContext(ctx, db.Rebind(`UPDATE gameplay_workspace_reward_windows SET
		state = ?, next_attempt_at = ?, claim_token = '', claim_until = '', updated_at = ?
		WHERE state = ?`),
		workspaceRewardRetry, formatTime(now), formatTime(now), workspaceRewardClaimed,
	)
}

func requireWorkspaceRewardRow(result sql.Result, operation string) error {
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return fmt.Errorf("gameplay: workspace reward %s", operation)
	}
	return nil
}
