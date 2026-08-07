package gameplay

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

const gameplayPendingDeletionSource = "gameplay"
const pendingDeletionTimeLayout = "2006-01-02T15:04:05.000000000Z"

func formatPendingDeletionTime(value time.Time) string {
	return value.UTC().Format(pendingDeletionTimeLayout)
}

func (s PendingDeletionSource) Name() string {
	return gameplayPendingDeletionSource
}

func (s PendingDeletionSource) Kinds() []pendingdeletion.Kind {
	return []pendingdeletion.Kind{pendingdeletion.KindPet}
}

func (s PendingDeletionSource) Validate() error {
	if s.DB == nil {
		return errors.New("gameplay: database not configured")
	}
	return nil
}

func pendingDeletionTaskSelectSQL() string {
	return `SELECT deletion_id, kind, owner_public_key, resource_id, reason, deleted_at,
		descriptor_version, descriptor_json, marker_fingerprint, task_status, task_phase,
		failure_count, next_attempt_at, lease_token, lease_deadline,
		last_error_code, last_error_message, updated_at
		FROM gameplay_pending_deletions`
}

func scanPendingDeletionTask(row rowScanner) (pendingdeletion.Task, error) {
	var task pendingdeletion.Task
	var owner, deletedAt, descriptorJSON, nextAttemptAt, leaseDeadline, updatedAt string
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
	task.Source = gameplayPendingDeletionSource
	task.Record.OwnerPublicKey = &owner
	task.Record.DeletedAt = parseTime(deletedAt)
	task.Record.Descriptor = []byte(descriptorJSON)
	task.NextAttemptAt = parseTime(nextAttemptAt)
	task.LeaseDeadline = parseTime(leaseDeadline)
	task.UpdatedAt = parseTime(updatedAt)
	return task, nil
}

func (s PendingDeletionSource) ScanDue(ctx context.Context, now time.Time, limit int, cursor string) ([]pendingdeletion.Reference, string, error) {
	if s.DB == nil {
		return nil, "", errors.New("gameplay: database not configured")
	}
	if limit <= 0 {
		return nil, "", fmt.Errorf("gameplay: pending deletion scan limit must be positive")
	}
	rows, err := s.DB.QueryxContext(ctx, s.DB.Rebind(pendingDeletionTaskSelectSQL()+`
		WHERE deletion_id > ? AND (
			(task_status IN (?, ?) AND next_attempt_at <= ?)
			OR (task_status = ? AND lease_deadline <= ?)
		)
		ORDER BY deletion_id LIMIT ?`),
		cursor, pendingdeletion.StatusQueued, pendingdeletion.StatusRetryWait, formatPendingDeletionTime(now),
		pendingdeletion.StatusRunning, formatPendingDeletionTime(now), limit)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	refs := make([]pendingdeletion.Reference, 0, limit)
	next := ""
	for rows.Next() {
		task, scanErr := scanPendingDeletionTask(rows)
		if scanErr != nil {
			return nil, "", scanErr
		}
		refs = append(refs, pendingdeletion.Reference{
			Source: task.Source, DeletionID: task.Record.DeletionID,
			MarkerFingerprint: task.MarkerFingerprint,
		})
		next = task.Record.DeletionID
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	if len(refs) < limit {
		next = ""
	}
	return refs, next, nil
}

func (s PendingDeletionSource) Claim(ctx context.Context, ref pendingdeletion.Reference, now time.Time, leaseDuration time.Duration) (pendingdeletion.Claim, bool, error) {
	if s.DB == nil {
		return pendingdeletion.Claim{}, false, errors.New("gameplay: database not configured")
	}
	if ref.Source != s.Name() || strings.TrimSpace(ref.DeletionID) == "" || strings.TrimSpace(ref.MarkerFingerprint) == "" {
		return pendingdeletion.Claim{}, false, fmt.Errorf("gameplay: invalid pending deletion reference")
	}
	token, err := newPendingDeletionLeaseToken()
	if err != nil {
		return pendingdeletion.Claim{}, false, err
	}
	deadline := now.Add(leaseDuration)
	result, err := s.DB.ExecContext(ctx, s.DB.Rebind(`UPDATE gameplay_pending_deletions
		SET task_status = ?, lease_token = ?, lease_deadline = ?, updated_at = ?
		WHERE deletion_id = ? AND marker_fingerprint = ? AND (
			(task_status IN (?, ?) AND next_attempt_at <= ?)
			OR (task_status = ? AND lease_deadline <= ?)
		)`),
		pendingdeletion.StatusRunning, token, formatPendingDeletionTime(deadline), formatPendingDeletionTime(now),
		ref.DeletionID, ref.MarkerFingerprint,
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
		return pendingdeletion.Claim{}, false, fmt.Errorf("gameplay: claimed %d pending deletion rows", rows)
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

func (s PendingDeletionSource) Renew(ctx context.Context, claim pendingdeletion.Claim, now time.Time, leaseDuration time.Duration) error {
	return s.conditionalTaskUpdate(ctx, claim, now, `lease_deadline = ?, updated_at = ?`, formatPendingDeletionTime(now.Add(leaseDuration)), formatPendingDeletionTime(now))
}

func (s PendingDeletionSource) Checkpoint(ctx context.Context, claim pendingdeletion.Claim, phase pendingdeletion.Phase, now time.Time) (pendingdeletion.Claim, error) {
	if err := pendingdeletion.ValidatePhase(phase); err != nil {
		return pendingdeletion.Claim{}, fmt.Errorf("gameplay: invalid pending deletion phase: %w", err)
	}
	if err := s.conditionalTaskUpdate(ctx, claim, now, `task_phase = ?, updated_at = ?`, phase, formatPendingDeletionTime(now)); err != nil {
		return pendingdeletion.Claim{}, err
	}
	claim.Phase = phase
	claim.UpdatedAt = now
	return claim, nil
}

func (s PendingDeletionSource) Defer(ctx context.Context, claim pendingdeletion.Claim, code, message string, nextAttempt, now time.Time) error {
	return s.conditionalTaskUpdate(ctx, claim, now,
		`task_status = ?, next_attempt_at = ?, lease_token = '', lease_deadline = '', last_error_code = ?, last_error_message = ?, updated_at = ?`,
		pendingdeletion.StatusRetryWait, formatPendingDeletionTime(nextAttempt), code, message, formatPendingDeletionTime(now))
}

func (s PendingDeletionSource) Fail(ctx context.Context, claim pendingdeletion.Claim, code, message string, terminal bool, nextAttempt, now time.Time, maxAttempts int) error {
	status := pendingdeletion.StatusRetryWait
	if terminal || claim.FailureCount+1 >= maxAttempts {
		status = pendingdeletion.StatusFailed
	}
	return s.conditionalTaskUpdate(ctx, claim, now,
		`task_status = ?, failure_count = CASE WHEN failure_count < 0 THEN 1 ELSE failure_count + 1 END, next_attempt_at = ?, lease_token = '', lease_deadline = '', last_error_code = ?, last_error_message = ?, updated_at = ?`,
		status, formatPendingDeletionTime(nextAttempt), code, message, formatPendingDeletionTime(now))
}

func (s PendingDeletionSource) conditionalTaskUpdate(ctx context.Context, claim pendingdeletion.Claim, now time.Time, setClause string, args ...any) error {
	if s.DB == nil {
		return errors.New("gameplay: database not configured")
	}
	query := `UPDATE gameplay_pending_deletions SET ` + setClause + `
		WHERE deletion_id = ? AND marker_fingerprint = ? AND task_status = ?
			AND lease_token = ? AND lease_deadline > ?`
	args = append(args, claim.Record.DeletionID, claim.MarkerFingerprint, pendingdeletion.StatusRunning, claim.LeaseToken, formatPendingDeletionTime(now))
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

func (s PendingDeletionSource) GetTask(ctx context.Context, deletionID string) (pendingdeletion.Task, error) {
	if s.DB == nil {
		return pendingdeletion.Task{}, errors.New("gameplay: database not configured")
	}
	task, err := scanPendingDeletionTask(s.DB.QueryRowxContext(ctx, s.DB.Rebind(pendingDeletionTaskSelectSQL()+` WHERE deletion_id = ?`), deletionID))
	if errors.Is(err, sql.ErrNoRows) {
		return pendingdeletion.Task{}, pendingdeletion.ErrNotFound
	}
	return task, err
}

func (s PendingDeletionSource) ListTasks(ctx context.Context, options pendingdeletion.SourceListOptions) ([]pendingdeletion.Task, error) {
	if s.DB == nil {
		return nil, errors.New("gameplay: database not configured")
	}
	if options.Limit <= 0 {
		return nil, fmt.Errorf("gameplay: pending deletion list limit must be positive")
	}
	conditions := []string{"1 = 1"}
	args := []any{}
	if len(options.Kinds) > 0 {
		if !options.Kinds[pendingdeletion.KindPet] {
			return nil, nil
		}
		conditions = append(conditions, "kind = ?")
		args = append(args, pendingdeletion.KindPet)
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

func (s PendingDeletionSource) ActiveStats(ctx context.Context, _ time.Time) (int64, time.Time, error) {
	if s.DB == nil {
		return 0, time.Time{}, errors.New("gameplay: database not configured")
	}
	var depth int64
	var oldest sql.NullString
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*), MIN(task_created_at) FROM gameplay_pending_deletions`).Scan(&depth, &oldest); err != nil {
		return 0, time.Time{}, err
	}
	if !oldest.Valid || oldest.String == "" {
		return depth, time.Time{}, nil
	}
	value, err := time.Parse(time.RFC3339Nano, oldest.String)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("gameplay: parse oldest pending deletion time: %w", err)
	}
	return depth, value.UTC(), nil
}

func (s PendingDeletionSource) Retry(ctx context.Context, deletionID string, now time.Time) (pendingdeletion.Task, error) {
	if s.DB == nil {
		return pendingdeletion.Task{}, errors.New("gameplay: database not configured")
	}
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return pendingdeletion.Task{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE gameplay_pending_deletions
		SET task_status = ?, failure_count = 0, next_attempt_at = ?, lease_token = '', lease_deadline = '',
			last_error_code = '', last_error_message = '', updated_at = ?
		WHERE deletion_id = ? AND task_status = ?`),
		pendingdeletion.StatusQueued, formatPendingDeletionTime(now), formatPendingDeletionTime(now), deletionID, pendingdeletion.StatusFailed)
	if err != nil {
		return pendingdeletion.Task{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return pendingdeletion.Task{}, err
	}
	if rows == 1 {
		task, err := scanPendingDeletionTask(tx.QueryRowxContext(ctx, tx.Rebind(pendingDeletionTaskSelectSQL()+` WHERE deletion_id = ?`), deletionID))
		if err != nil {
			return pendingdeletion.Task{}, err
		}
		if err := tx.Commit(); err != nil {
			return pendingdeletion.Task{}, err
		}
		return task, nil
	}
	var status pendingdeletion.Status
	err = tx.QueryRowContext(ctx, tx.Rebind(`SELECT task_status FROM gameplay_pending_deletions WHERE deletion_id = ?`), deletionID).Scan(&status)
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
		return "", fmt.Errorf("gameplay: generate pending deletion lease token: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

func migratePendingDeletionTasks(ctx context.Context, db sqlDialectExecutor) error {
	columns := []struct {
		name       string
		definition string
	}{
		{"marker_fingerprint", "TEXT NOT NULL DEFAULT ''"},
		{"task_created_at", "TEXT NOT NULL DEFAULT ''"},
		{"task_status", "TEXT NOT NULL DEFAULT 'queued'"},
		{"task_phase", "TEXT NOT NULL DEFAULT 'validate'"},
		{"failure_count", "INTEGER NOT NULL DEFAULT 0"},
		{"next_attempt_at", "TEXT NOT NULL DEFAULT ''"},
		{"lease_token", "TEXT NOT NULL DEFAULT ''"},
		{"lease_deadline", "TEXT NOT NULL DEFAULT ''"},
		{"last_error_code", "TEXT NOT NULL DEFAULT ''"},
		{"last_error_message", "TEXT NOT NULL DEFAULT ''"},
		{"updated_at", "TEXT NOT NULL DEFAULT ''"},
	}
	for _, column := range columns {
		exists, err := sqlColumnExists(ctx, db, "gameplay_pending_deletions", column.name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.ExecContext(ctx, `ALTER TABLE gameplay_pending_deletions ADD COLUMN `+column.name+` `+column.definition); err != nil {
			return fmt.Errorf("gameplay: add pending deletion column %s: %w", column.name, err)
		}
	}
	rows, err := db.QueryContext(ctx, `SELECT deletion_id, kind, owner_public_key, resource_id, reason, deleted_at, descriptor_version, descriptor_json
		FROM gameplay_pending_deletions WHERE marker_fingerprint = '' OR task_created_at = '' OR next_attempt_at = '' OR updated_at = ''`)
	if err != nil {
		return err
	}
	type backfill struct {
		id, fingerprint, deletedAt string
	}
	var updates []backfill
	for rows.Next() {
		var record pendingdeletion.Record
		var owner, deletedAt, descriptor string
		if err := rows.Scan(&record.DeletionID, &record.Kind, &owner, &record.ResourceID, &record.Reason, &deletedAt, &record.DescriptorVersion, &descriptor); err != nil {
			rows.Close()
			return err
		}
		record.OwnerPublicKey = &owner
		record.DeletedAt = parseTime(deletedAt)
		record.Descriptor = []byte(descriptor)
		fingerprint, err := pendingdeletion.StoredFingerprint(record)
		if err != nil {
			rows.Close()
			return fmt.Errorf("gameplay: backfill pending deletion %q: %w", record.DeletionID, err)
		}
		updates = append(updates, backfill{id: record.DeletionID, fingerprint: fingerprint, deletedAt: formatPendingDeletionTime(record.DeletedAt)})
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, update := range updates {
		if _, err := db.ExecContext(ctx, db.Rebind(`UPDATE gameplay_pending_deletions
			SET marker_fingerprint = ?,
				task_created_at = CASE WHEN task_created_at = '' THEN ? ELSE task_created_at END,
				next_attempt_at = CASE WHEN next_attempt_at = '' THEN ? ELSE next_attempt_at END,
				updated_at = CASE WHEN updated_at = '' THEN ? ELSE updated_at END
			WHERE deletion_id = ?`), update.fingerprint, update.deletedAt, update.deletedAt, update.deletedAt, update.id); err != nil {
			return err
		}
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS gameplay_pending_deletions_due_idx ON gameplay_pending_deletions(task_status, next_attempt_at, lease_deadline, deletion_id)`); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS gameplay_pending_deletions_admin_idx ON gameplay_pending_deletions(task_created_at, deletion_id)`)
	return err
}

var _ pendingdeletion.Source = PendingDeletionSource{}
