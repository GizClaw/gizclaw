package workspace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/jmoiron/sqlx"
)

type workspaceSQLDeletionSource struct{ DB *sqlx.DB }

func initializeWorkspaceDeletionSQL(ctx context.Context, tx *sqlx.Tx) error {
	for _, query := range []string{
		`CREATE TABLE IF NOT EXISTS workspace_pending_deletions(
   deletion_id TEXT NOT NULL PRIMARY KEY, kind TEXT NOT NULL CHECK(kind='workspace'),
   owner_public_key TEXT, resource_id TEXT NOT NULL UNIQUE, workspace_incarnation TEXT NOT NULL,
   reason TEXT NOT NULL, deleted_at TEXT NOT NULL, descriptor_version INTEGER NOT NULL,
   descriptor_json TEXT NOT NULL, marker_fingerprint TEXT NOT NULL, task_created_at TEXT NOT NULL,
   task_status TEXT NOT NULL, task_phase TEXT NOT NULL, failure_count INTEGER NOT NULL DEFAULT 0,
   next_attempt_at TEXT NOT NULL, lease_token TEXT NOT NULL DEFAULT '', lease_deadline TEXT NOT NULL DEFAULT '',
   last_error_code TEXT NOT NULL DEFAULT '', last_error_message TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS workspace_pending_deletions_due_idx ON workspace_pending_deletions(kind,task_status,next_attempt_at,deletion_id)`,
		`CREATE INDEX IF NOT EXISTS workspace_pending_deletions_lease_idx ON workspace_pending_deletions(kind,task_status,lease_deadline,deletion_id)`,
		`CREATE INDEX IF NOT EXISTS workspace_pending_deletions_admin_idx ON workspace_pending_deletions(task_created_at,deletion_id)`,
	} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return nil
}

func (s workspaceSQLDeletionSource) Get(ctx context.Context, id string) (pendingdeletion.Record, error) {
	task, err := s.GetTask(ctx, id)
	if err != nil {
		return pendingdeletion.Record{}, err
	}
	if err := task.Record.Validate(); err != nil {
		return pendingdeletion.Record{}, err
	}
	return task.Record, nil
}

func (s workspaceSQLDeletionSource) GetByResource(ctx context.Context, id string) (pendingdeletion.Record, error) {
	if err := s.Validate(); err != nil {
		return pendingdeletion.Record{}, err
	}
	task, err := scanPendingDeletionTask(s.DB.QueryRowContext(ctx, s.DB.Rebind(pendingDeletionTaskSelectSQL()+" WHERE resource_id=?"), id))
	if errors.Is(err, sql.ErrNoRows) {
		return pendingdeletion.Record{}, pendingdeletion.ErrNotFound
	}
	if err != nil {
		return pendingdeletion.Record{}, err
	}
	if err := task.Record.Validate(); err != nil {
		return pendingdeletion.Record{}, err
	}
	return task.Record, nil
}

func (s workspaceSQLDeletionSource) HasLocator(ctx context.Context, locator pendingdeletion.Locator) (bool, error) {
	if err := s.Validate(); err != nil {
		return false, err
	}
	if locator.Kind != pendingdeletion.KindWorkspace {
		return false, nil
	}
	query := "SELECT 1 FROM workspace_pending_deletions WHERE resource_id=?"
	args := []any{locator.ResourceID}
	if locator.OwnerPublicKey != nil {
		query += " AND owner_public_key=?"
		args = append(args, *locator.OwnerPublicKey)
	}
	var found int
	err := s.DB.QueryRowContext(ctx, s.DB.Rebind(query), args...).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s workspaceSQLDeletionSource) CreateOrGet(ctx context.Context, record pendingdeletion.Record) (pendingdeletion.Record, bool, error) {
	if err := s.Validate(); err != nil {
		return pendingdeletion.Record{}, false, err
	}
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return pendingdeletion.Record{}, false, err
	}
	defer tx.Rollback()
	stored, created, err := createWorkspaceDeletionTx(ctx, tx, record)
	if err != nil {
		return pendingdeletion.Record{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return pendingdeletion.Record{}, false, err
	}
	return stored, created, nil
}

// createWorkspaceDeletionTx shares the caller's transaction with reward fencing.
func createWorkspaceDeletionTx(ctx context.Context, tx *sqlx.Tx, record pendingdeletion.Record) (pendingdeletion.Record, bool, error) {
	if err := record.Validate(); err != nil {
		return pendingdeletion.Record{}, false, err
	}
	if record.Kind != pendingdeletion.KindWorkspace {
		return pendingdeletion.Record{}, false, errors.New("workspace: wrong deletion kind")
	}
	fingerprint, err := pendingdeletion.Fingerprint(record)
	if err != nil {
		return pendingdeletion.Record{}, false, err
	}
	descriptor, err := validateWorkspaceDeletionClaim(pendingdeletion.Claim{Task: pendingdeletion.Task{Source: pendingDeletionSourceName, Record: record, MarkerFingerprint: fingerprint}})
	if err != nil {
		return pendingdeletion.Record{}, false, err
	}
	// Acquire the Workspace write lock before reading its current deletion state.
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE workspaces SET revision=revision WHERE id=?`), record.ResourceID)
	if err != nil {
		return pendingdeletion.Record{}, false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return pendingdeletion.Record{}, false, err
	}
	if count != 1 {
		return pendingdeletion.Record{}, false, ErrWorkspaceDeleted
	}
	var owner, pending sql.NullString
	var incarnation, name, icon string
	var system int
	if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT owner_public_key,pending_deletion_id,incarnation,name,system,icon_json FROM workspaces WHERE id=?`), record.ResourceID).Scan(&owner, &pending, &incarnation, &name, &system, &icon); err != nil {
		return pendingdeletion.Record{}, false, err
	}
	if owner.Valid != (descriptor.OwnerPublicKey != nil) || (owner.Valid && owner.String != *descriptor.OwnerPublicKey) {
		return pendingdeletion.Record{}, false, errors.New("workspace: deletion owner does not match record")
	}
	if name != descriptor.Name || (system == 1) != descriptor.System || (strings.TrimSpace(icon) != "null") != descriptor.HasIcon {
		return pendingdeletion.Record{}, false, errWorkspaceSQLConflict
	}
	if pending.Valid {
		task, err := scanPendingDeletionTask(tx.QueryRowContext(ctx, tx.Rebind(pendingDeletionTaskSelectSQL()+` WHERE deletion_id=? AND resource_id=?`), pending.String, record.ResourceID))
		if err != nil {
			return pendingdeletion.Record{}, false, err
		}
		if err := task.Record.Validate(); err != nil {
			return pendingdeletion.Record{}, false, err
		}
		return task.Record, false, nil
	}
	stamp := formatPendingDeletionTime(record.DeletedAt)
	var ownerValue any
	if record.OwnerPublicKey != nil {
		ownerValue = *record.OwnerPublicKey
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO workspace_pending_deletions(deletion_id,kind,owner_public_key,resource_id,workspace_incarnation,reason,deleted_at,descriptor_version,descriptor_json,marker_fingerprint,task_created_at,task_status,task_phase,next_attempt_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`), record.DeletionID, record.Kind, ownerValue, record.ResourceID, incarnation, record.Reason, stamp, record.DescriptorVersion, string(record.Descriptor), fingerprint, stamp, pendingdeletion.StatusQueued, pendingdeletion.PhaseValidate, stamp, stamp); err != nil {
		return pendingdeletion.Record{}, false, err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE workspaces SET pending_deletion_id=?,revision=revision+1 WHERE id=?`), record.DeletionID, record.ResourceID); err != nil {
		return pendingdeletion.Record{}, false, err
	}
	return record, true, nil
}

// Finalize removes the retained record, label indexes, and task in one transaction.
func (s workspaceSQLDeletionSource) Finalize(ctx context.Context, claim pendingdeletion.Claim, now time.Time) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if claim.Source != s.Name() || claim.Record.Kind != pendingdeletion.KindWorkspace || claim.Phase != pendingdeletion.PhaseFinalize {
		return pendingdeletion.ErrConflict
	}
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE workspace_pending_deletions SET lease_deadline=lease_deadline WHERE deletion_id=? AND resource_id=? AND marker_fingerprint=? AND task_status=? AND task_phase=? AND lease_token=? AND lease_deadline>?`), claim.Record.DeletionID, claim.Record.ResourceID, claim.MarkerFingerprint, pendingdeletion.StatusRunning, pendingdeletion.PhaseFinalize, claim.LeaseToken, formatPendingDeletionTime(now))
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return pendingdeletion.ErrConflict
	}
	var incarnation string
	if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT workspace_incarnation FROM workspace_pending_deletions WHERE deletion_id=?`), claim.Record.DeletionID).Scan(&incarnation); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM workspace_labels WHERE workspace_id=?`), claim.Record.ResourceID); err != nil {
		return err
	}
	result, err = tx.ExecContext(ctx, tx.Rebind(`DELETE FROM workspaces WHERE id=? AND incarnation=? AND pending_deletion_id=?`), claim.Record.ResourceID, incarnation, claim.Record.DeletionID)
	if err != nil {
		return err
	}
	count, err = result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("%w: retained Workspace changed", pendingdeletion.ErrConflict)
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM workspace_pending_deletions WHERE deletion_id=?`), claim.Record.DeletionID); err != nil {
		return err
	}
	return tx.Commit()
}
