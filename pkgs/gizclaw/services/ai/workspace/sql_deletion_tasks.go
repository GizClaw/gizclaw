package workspace

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
)

const pendingDeletionTimeLayout = "2006-01-02T15:04:05.000000000Z"

func formatPendingDeletionTime(value time.Time) string {
	return value.UTC().Format(pendingDeletionTimeLayout)
}

func (s workspaceSQLDeletionSource) Name() string {
	return pendingDeletionSourceName
}

func (s workspaceSQLDeletionSource) Kinds() []pendingdeletion.Kind {
	return []pendingdeletion.Kind{pendingdeletion.KindWorkspace}
}

func (s workspaceSQLDeletionSource) Validate() error {
	if s.DB == nil {
		return errors.New("workspace: database not configured")
	}
	return nil
}

func pendingDeletionTaskSelectSQL() string {
	return `SELECT deletion_id, kind, owner_public_key, resource_id, reason, deleted_at,
		descriptor_version, descriptor_json, marker_fingerprint, task_status, task_phase,
		failure_count, next_attempt_at, lease_token, lease_deadline,
		last_error_code, last_error_message, updated_at
		FROM workspace_pending_deletions`
}

func scanPendingDeletionTask(row workspaceSQLScanner) (pendingdeletion.Task, error) {
	var task pendingdeletion.Task
	var owner sql.NullString
	var deletedAt, descriptorJSON, nextAttemptAt, leaseDeadline, updatedAt string
	err := row.Scan(
		&task.Record.DeletionID, &task.Record.Kind, &owner, &task.Record.ResourceID,
		&task.Record.Reason, &deletedAt, &task.Record.DescriptorVersion, &descriptorJSON,
		&task.MarkerFingerprint, &task.Status, &task.Phase, &task.FailureCount,
		&nextAttemptAt, &task.LeaseToken, &leaseDeadline,
		&task.LastErrorCode, &task.LastErrorMessage, &updatedAt,
	)
	if err != nil {
		return pendingdeletion.Task{}, err
	}
	task.Source = pendingDeletionSourceName
	if owner.Valid {
		task.Record.OwnerPublicKey = &owner.String
	}
	task.Record.Descriptor = []byte(descriptorJSON)
	for _, field := range []struct {
		raw  string
		dest *time.Time
	}{{deletedAt, &task.Record.DeletedAt}, {nextAttemptAt, &task.NextAttemptAt}, {leaseDeadline, &task.LeaseDeadline}, {updatedAt, &task.UpdatedAt}} {
		if field.raw == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339Nano, field.raw)
		if err != nil {
			return pendingdeletion.Task{}, fmt.Errorf("workspace: invalid task timestamp: %w", err)
		}
		*field.dest = parsed
	}
	return task, nil
}

const pendingDeletionDueSQL = `SELECT deletion_id, marker_fingerprint
	FROM workspace_pending_deletions
	WHERE kind = ? AND task_status IN (?, ?) AND next_attempt_at <= ? AND deletion_id > ?
	UNION ALL
	SELECT deletion_id, marker_fingerprint
	FROM workspace_pending_deletions
	WHERE kind = ? AND task_status = ? AND lease_deadline <= ? AND deletion_id > ?
	ORDER BY deletion_id LIMIT ?`

func (s workspaceSQLDeletionSource) ScanDue(ctx context.Context, now time.Time, limit int, cursor string) ([]pendingdeletion.Reference, string, error) {
	if s.DB == nil {
		return nil, "", errors.New("workspace: database not configured")
	}
	if limit <= 0 {
		return nil, "", fmt.Errorf("workspace: pending deletion scan limit must be positive")
	}
	rows, err := s.DB.QueryContext(ctx, s.DB.Rebind(pendingDeletionDueSQL),
		pendingdeletion.KindWorkspace, pendingdeletion.StatusQueued, pendingdeletion.StatusRetryWait, formatPendingDeletionTime(now), cursor,
		pendingdeletion.KindWorkspace, pendingdeletion.StatusRunning, formatPendingDeletionTime(now), cursor, limit)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	refs := make([]pendingdeletion.Reference, 0, limit)
	next := ""
	for rows.Next() {
		ref := pendingdeletion.Reference{Source: s.Name()}
		if err := rows.Scan(&ref.DeletionID, &ref.MarkerFingerprint); err != nil {
			return nil, "", err
		}
		refs = append(refs, ref)
		next = ref.DeletionID
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	if len(refs) < limit {
		next = ""
	}
	return refs, next, nil
}

func (s workspaceSQLDeletionSource) Claim(ctx context.Context, ref pendingdeletion.Reference, now time.Time, leaseDuration time.Duration) (pendingdeletion.Claim, bool, error) {
	if leaseDuration <= 0 {
		return pendingdeletion.Claim{}, false, errors.New("workspace: lease duration must be positive")
	}
	if s.DB == nil {
		return pendingdeletion.Claim{}, false, errors.New("workspace: database not configured")
	}
	if ref.Source != s.Name() || strings.TrimSpace(ref.DeletionID) == "" || strings.TrimSpace(ref.MarkerFingerprint) == "" {
		return pendingdeletion.Claim{}, false, fmt.Errorf("workspace: invalid pending deletion reference")
	}
	token, err := newPendingDeletionLeaseToken()
	if err != nil {
		return pendingdeletion.Claim{}, false, err
	}
	deadline := now.Add(leaseDuration)
	result, err := s.DB.ExecContext(ctx, s.DB.Rebind(`UPDATE workspace_pending_deletions
		SET task_status = ?, lease_token = ?, lease_deadline = ?, updated_at = ?
		WHERE deletion_id = ? AND kind = ? AND marker_fingerprint = ? AND (
			(task_status IN (?, ?) AND next_attempt_at <= ?)
			OR (task_status = ? AND lease_deadline <= ?)
		)`),
		pendingdeletion.StatusRunning, token, formatPendingDeletionTime(deadline), formatPendingDeletionTime(now),
		ref.DeletionID, pendingdeletion.KindWorkspace, ref.MarkerFingerprint,
		pendingdeletion.StatusQueued, pendingdeletion.StatusRetryWait, formatPendingDeletionTime(now),
		pendingdeletion.StatusRunning, formatPendingDeletionTime(now))
	if err != nil {
		return pendingdeletion.Claim{}, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return pendingdeletion.Claim{}, false, err
	}
	if rows == 0 {
		return pendingdeletion.Claim{}, false, nil
	}
	if rows != 1 {
		return pendingdeletion.Claim{}, false, fmt.Errorf("workspace: claimed %d pending deletion rows", rows)
	}
	task, err := s.GetTask(ctx, ref.DeletionID)
	if err != nil {
		return pendingdeletion.Claim{}, false, err
	}
	if task.LeaseToken != token || task.MarkerFingerprint != ref.MarkerFingerprint {
		return pendingdeletion.Claim{}, false, pendingdeletion.ErrConflict
	}
	return pendingdeletion.Claim{Task: task}, true, nil
}

func (s workspaceSQLDeletionSource) Renew(ctx context.Context, claim pendingdeletion.Claim, now time.Time, leaseDuration time.Duration) error {
	if leaseDuration <= 0 {
		return errors.New("workspace: lease duration must be positive")
	}
	return s.conditionalTaskUpdate(ctx, claim, now, `lease_deadline = ?, updated_at = ?`, formatPendingDeletionTime(now.Add(leaseDuration)), formatPendingDeletionTime(now))
}

func (s workspaceSQLDeletionSource) Checkpoint(ctx context.Context, claim pendingdeletion.Claim, phase pendingdeletion.Phase, now time.Time) (pendingdeletion.Claim, error) {
	if err := pendingdeletion.ValidatePhase(phase); err != nil {
		return pendingdeletion.Claim{}, fmt.Errorf("workspace: invalid pending deletion phase: %w", err)
	}
	if err := s.conditionalTaskUpdate(ctx, claim, now, `task_phase = ?, updated_at = ?`, phase, formatPendingDeletionTime(now)); err != nil {
		return pendingdeletion.Claim{}, err
	}
	claim.Phase = phase
	claim.UpdatedAt = now
	return claim, nil
}

func (s workspaceSQLDeletionSource) Defer(ctx context.Context, claim pendingdeletion.Claim, code, message string, nextAttempt, now time.Time) error {
	return s.conditionalTaskUpdate(ctx, claim, now,
		`task_status = ?, next_attempt_at = ?, lease_token = '', lease_deadline = '', last_error_code = ?, last_error_message = ?, updated_at = ?`,
		pendingdeletion.StatusRetryWait, formatPendingDeletionTime(nextAttempt), code, message, formatPendingDeletionTime(now))
}

func (s workspaceSQLDeletionSource) Fail(ctx context.Context, claim pendingdeletion.Claim, code, message string, terminal bool, nextAttempt, now time.Time, maxAttempts int) error {
	status := pendingdeletion.StatusRetryWait
	if terminal || claim.FailureCount+1 >= maxAttempts {
		status = pendingdeletion.StatusFailed
	}
	return s.conditionalTaskUpdate(ctx, claim, now,
		`task_status = ?, failure_count = CASE WHEN failure_count < 0 THEN 1 ELSE failure_count + 1 END, next_attempt_at = ?, lease_token = '', lease_deadline = '', last_error_code = ?, last_error_message = ?, updated_at = ?`,
		status, formatPendingDeletionTime(nextAttempt), code, message, formatPendingDeletionTime(now))
}

func (s workspaceSQLDeletionSource) conditionalTaskUpdate(ctx context.Context, claim pendingdeletion.Claim, now time.Time, setClause string, args ...any) error {
	if claim.Source != s.Name() || claim.Record.Kind != pendingdeletion.KindWorkspace {
		return pendingdeletion.ErrConflict
	}
	if s.DB == nil {
		return errors.New("workspace: database not configured")
	}
	query := `UPDATE workspace_pending_deletions SET ` + setClause + `
		WHERE deletion_id = ? AND kind = ? AND marker_fingerprint = ? AND task_status = ?
			AND lease_token = ? AND lease_deadline > ?`
	args = append(args, claim.Record.DeletionID, pendingdeletion.KindWorkspace, claim.MarkerFingerprint, pendingdeletion.StatusRunning, claim.LeaseToken, formatPendingDeletionTime(now))
	result, err := s.DB.ExecContext(ctx, s.DB.Rebind(query), args...)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return pendingdeletion.ErrConflict
	}
	return nil
}

func (s workspaceSQLDeletionSource) GetTask(ctx context.Context, deletionID string) (pendingdeletion.Task, error) {
	if s.DB == nil {
		return pendingdeletion.Task{}, errors.New("workspace: database not configured")
	}
	task, err := scanPendingDeletionTask(s.DB.QueryRowxContext(ctx, s.DB.Rebind(pendingDeletionTaskSelectSQL()+` WHERE deletion_id = ? AND kind = ?`), deletionID, pendingdeletion.KindWorkspace))
	if errors.Is(err, sql.ErrNoRows) {
		return pendingdeletion.Task{}, pendingdeletion.ErrNotFound
	}
	return task, err
}

func (s workspaceSQLDeletionSource) ListTasks(ctx context.Context, options pendingdeletion.SourceListOptions) ([]pendingdeletion.Task, error) {
	if s.DB == nil {
		return nil, errors.New("workspace: database not configured")
	}
	if options.Limit <= 0 {
		return nil, fmt.Errorf("workspace: pending deletion list limit must be positive")
	}
	conditions := []string{"kind = ?"}
	args := []any{pendingdeletion.KindWorkspace}
	if len(options.Kinds) > 0 {
		if !options.Kinds[pendingdeletion.KindWorkspace] {
			return nil, nil
		}
	}
	if len(options.Statuses) > 0 {
		statuses := make([]pendingdeletion.Status, 0, len(options.Statuses))
		for _, candidate := range []pendingdeletion.Status{pendingdeletion.StatusQueued, pendingdeletion.StatusRunning, pendingdeletion.StatusRetryWait, pendingdeletion.StatusFailed} {
			if options.Statuses[candidate] {
				statuses = append(statuses, candidate)
			}
		}
		if len(statuses) == 0 {
			return nil, nil
		}
		placeholders := make([]string, len(statuses))
		for i, status := range statuses {
			placeholders[i] = "?"
			args = append(args, status)
		}
		conditions = append(conditions, "task_status IN ("+strings.Join(placeholders, ",")+")")
	}
	if options.StartTime != nil {
		conditions = append(conditions, "task_created_at >= ?")
		args = append(args, formatPendingDeletionTime(*options.StartTime))
	}
	if options.EndTime != nil {
		conditions = append(conditions, "task_created_at < ?")
		args = append(args, formatPendingDeletionTime(*options.EndTime))
	}
	if options.AfterCreatedAt != nil {
		switch strings.Compare(s.Name(), options.AfterSource) {
		case -1:
			conditions = append(conditions, "task_created_at > ?")
			args = append(args, formatPendingDeletionTime(*options.AfterCreatedAt))
		case 0:
			conditions = append(conditions, "(task_created_at > ? OR (task_created_at = ? AND deletion_id > ?))")
			args = append(args, formatPendingDeletionTime(*options.AfterCreatedAt), formatPendingDeletionTime(*options.AfterCreatedAt), options.AfterDeletionID)
		default:
			conditions = append(conditions, "task_created_at >= ?")
			args = append(args, formatPendingDeletionTime(*options.AfterCreatedAt))
		}
	}
	args = append(args, options.Limit)
	rows, err := s.DB.QueryxContext(ctx, s.DB.Rebind(pendingDeletionTaskSelectSQL()+` WHERE `+strings.Join(conditions, " AND ")+` ORDER BY task_created_at, deletion_id LIMIT ?`), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := make([]pendingdeletion.Task, 0, options.Limit)
	for rows.Next() {
		task, scanErr := scanPendingDeletionTask(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s workspaceSQLDeletionSource) ActiveStats(ctx context.Context, _ time.Time) (int64, time.Time, error) {
	if s.DB == nil {
		return 0, time.Time{}, errors.New("workspace: database not configured")
	}
	var depth int64
	var oldest sql.NullString
	if err := s.DB.QueryRowContext(ctx, s.DB.Rebind(`SELECT COUNT(*), MIN(task_created_at) FROM workspace_pending_deletions WHERE kind = ?`), pendingdeletion.KindWorkspace).Scan(&depth, &oldest); err != nil {
		return 0, time.Time{}, err
	}
	if !oldest.Valid || oldest.String == "" {
		return depth, time.Time{}, nil
	}
	value, err := time.Parse(time.RFC3339Nano, oldest.String)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("workspace: parse oldest pending deletion time: %w", err)
	}
	return depth, value.UTC(), nil
}

func (s workspaceSQLDeletionSource) Retry(ctx context.Context, deletionID string, now time.Time) (pendingdeletion.Task, error) {
	if s.DB == nil {
		return pendingdeletion.Task{}, errors.New("workspace: database not configured")
	}
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return pendingdeletion.Task{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE workspace_pending_deletions
		SET task_status = ?, failure_count = 0, next_attempt_at = ?, lease_token = '', lease_deadline = '',
			last_error_code = '', last_error_message = '', updated_at = ?
		WHERE deletion_id = ? AND kind = ? AND task_status = ?`),
		pendingdeletion.StatusQueued, formatPendingDeletionTime(now), formatPendingDeletionTime(now), deletionID, pendingdeletion.KindWorkspace, pendingdeletion.StatusFailed)
	if err != nil {
		return pendingdeletion.Task{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return pendingdeletion.Task{}, err
	}
	if rows == 1 {
		task, err := scanPendingDeletionTask(tx.QueryRowxContext(ctx, tx.Rebind(pendingDeletionTaskSelectSQL()+` WHERE deletion_id = ? AND kind = ?`), deletionID, pendingdeletion.KindWorkspace))
		if err != nil {
			return pendingdeletion.Task{}, err
		}
		if err := tx.Commit(); err != nil {
			return pendingdeletion.Task{}, err
		}
		return task, nil
	}
	var status pendingdeletion.Status
	err = tx.QueryRowContext(ctx, tx.Rebind(`SELECT task_status FROM workspace_pending_deletions WHERE deletion_id = ? AND kind = ?`), deletionID, pendingdeletion.KindWorkspace).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return pendingdeletion.Task{}, pendingdeletion.ErrNotFound
	}
	if err != nil {
		return pendingdeletion.Task{}, err
	}
	return pendingdeletion.Task{}, pendingdeletion.ErrConflict
}

func newPendingDeletionLeaseToken() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("workspace: generate pending deletion lease token: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

var _ pendingdeletion.Source = workspaceSQLDeletionSource{}
