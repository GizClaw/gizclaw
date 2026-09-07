package workspace

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/jmoiron/sqlx"
)

const workspaceSQLTimeLayout = "2006-01-02T15:04:05.000000000Z"

var errWorkspaceSQLConflict = errors.New("workspace: record changed or identity already exists")

type workspaceSQLVersion struct {
	incarnation string
	revision    int64
}

// Initialize creates the business record and exact label indexes.
// Hosts call it during startup, before exposing Workspace operations.
func Initialize(ctx context.Context, db *sqlx.DB) error {
	if db == nil {
		return errors.New("workspace: database is not configured")
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, query := range []string{
		`CREATE TABLE IF NOT EXISTS workspaces (
   id TEXT NOT NULL PRIMARY KEY, owner_public_key TEXT, name TEXT NOT NULL,
   workflow_id TEXT NOT NULL, system INTEGER NOT NULL CHECK(system IN (0,1)),
   created_at TEXT NOT NULL, updated_at TEXT NOT NULL, last_active_at TEXT NOT NULL,
   parameters_json TEXT NOT NULL, toolkit_json TEXT NOT NULL, icon_json TEXT NOT NULL,
   labels_json TEXT NOT NULL, incarnation TEXT NOT NULL, revision BIGINT NOT NULL,
   pending_deletion_id TEXT)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS workspaces_owned_name_idx ON workspaces(owner_public_key,name) WHERE owner_public_key IS NOT NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS workspaces_ownerless_name_idx ON workspaces(name) WHERE owner_public_key IS NULL`,
		`CREATE INDEX IF NOT EXISTS workspaces_owner_id_idx ON workspaces(owner_public_key,id)`,
		`CREATE INDEX IF NOT EXISTS workspaces_workflow_id_idx ON workspaces(workflow_id,id)`,
		`CREATE TABLE IF NOT EXISTS workspace_labels(workspace_id TEXT NOT NULL,label_key TEXT NOT NULL,label_value TEXT NOT NULL,PRIMARY KEY(workspace_id,label_key),FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS workspace_labels_value_idx ON workspace_labels(label_key,label_value,workspace_id)`,
	} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	if err := initializeWorkspaceDeletionSQL(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

const workspaceSQLColumns = `w.id,w.owner_public_key,w.name,w.workflow_id,w.system,w.created_at,w.updated_at,w.last_active_at,w.parameters_json,w.toolkit_json,w.icon_json,w.labels_json,w.incarnation,w.revision`

type workspaceSQLScanner interface{ Scan(...any) error }

func scanSQLWorkspace(row workspaceSQLScanner) (apitypes.Workspace, workspaceSQLVersion, error) {
	var item apitypes.Workspace
	var version workspaceSQLVersion
	var owner sql.NullString
	var system int
	var created, updated, active, parameters, toolkit, icon, labels string
	if err := row.Scan(&item.Id, &owner, &item.Name, &item.WorkflowId, &system, &created, &updated, &active, &parameters, &toolkit, &icon, &labels, &version.incarnation, &version.revision); err != nil {
		return item, version, err
	}
	if owner.Valid {
		item.OwnerPublicKey = &owner.String
	}
	item.System = new(system == 1)
	for _, field := range []struct {
		raw  string
		dest *time.Time
	}{{created, &item.CreatedAt}, {updated, &item.UpdatedAt}, {active, &item.LastActiveAt}} {
		value, err := time.Parse(time.RFC3339Nano, field.raw)
		if err != nil {
			return item, version, fmt.Errorf("workspace: invalid stored timestamp: %w", err)
		}
		*field.dest = value
	}
	for _, field := range []struct {
		raw  string
		dest any
	}{{parameters, &item.Parameters}, {toolkit, &item.Toolkit}, {icon, &item.Icon}, {labels, &item.Labels}} {
		if err := json.Unmarshal([]byte(field.raw), field.dest); err != nil {
			return item, version, fmt.Errorf("workspace: invalid stored configuration: %w", err)
		}
	}
	item, err := validateStoredWorkspace(item)
	return item, version, err
}

func getSQLWorkspaceByID(ctx context.Context, db *sqlx.DB, id string) (apitypes.Workspace, workspaceSQLVersion, error) {
	return scanSQLWorkspace(db.QueryRowContext(ctx, db.Rebind("SELECT "+workspaceSQLColumns+" FROM workspaces w WHERE w.id=?"), id))
}

func getSQLWorkspaceByName(ctx context.Context, db *sqlx.DB, owner *string, name string) (apitypes.Workspace, workspaceSQLVersion, error) {
	query := "SELECT " + workspaceSQLColumns + " FROM workspaces w WHERE w.name=? AND w.owner_public_key IS NULL"
	args := []any{name}
	if owner != nil {
		query = "SELECT " + workspaceSQLColumns + " FROM workspaces w WHERE w.name=? AND w.owner_public_key=?"
		args = append(args, *owner)
	}
	return scanSQLWorkspace(db.QueryRowContext(ctx, db.Rebind(query), args...))
}

func workspaceSQLValues(item apitypes.Workspace) ([]any, error) {
	if _, err := validateStoredWorkspace(item); err != nil {
		return nil, err
	}
	var owner any
	if item.OwnerPublicKey != nil {
		owner = *item.OwnerPublicKey
	}
	system := 0
	if workspaceIsSystem(item) {
		system = 1
	}
	args := []any{item.Id, owner, item.Name, item.WorkflowId, system, item.CreatedAt.UTC().Format(workspaceSQLTimeLayout), item.UpdatedAt.UTC().Format(workspaceSQLTimeLayout), item.LastActiveAt.UTC().Format(workspaceSQLTimeLayout)}
	for _, value := range []any{item.Parameters, item.Toolkit, item.Icon, cloneLabelsOrEmpty(item.Labels)} {
		data, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		args = append(args, string(data))
	}
	return args, nil
}

func replaceSQLWorkspaceLabels(ctx context.Context, tx *sqlx.Tx, item apitypes.Workspace) error {
	if _, err := tx.ExecContext(ctx, tx.Rebind("DELETE FROM workspace_labels WHERE workspace_id=?"), item.Id); err != nil {
		return err
	}
	if item.Labels == nil || len(*item.Labels) == 0 {
		return nil
	}
	keys := make([]string, 0, len(*item.Labels))
	for key := range *item.Labels {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	values := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys)*3)
	for _, key := range keys {
		values = append(values, "(?,?,?)")
		args = append(args, item.Id, key, (*item.Labels)[key])
	}
	_, err := tx.ExecContext(ctx, tx.Rebind("INSERT INTO workspace_labels(workspace_id,label_key,label_value) VALUES "+strings.Join(values, ",")), args...)
	return err
}

func createSQLWorkspace(ctx context.Context, db *sqlx.DB, item apitypes.Workspace) error {
	args, err := workspaceSQLValues(item)
	if err != nil {
		return err
	}
	args = append(args, rand.Text(), int64(1))
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO workspaces(id,owner_public_key,name,workflow_id,system,created_at,updated_at,last_active_at,parameters_json,toolkit_json,icon_json,labels_json,incarnation,revision) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT DO NOTHING`), args...)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return errWorkspaceSQLConflict
	}
	if err := replaceSQLWorkspaceLabels(ctx, tx, item); err != nil {
		return err
	}
	return tx.Commit()
}

// updateSQLWorkspace compares the record generation and revision, preserves its
// identity, and cannot revive a record after deletion or retirement.
func updateSQLWorkspace(ctx context.Context, db *sqlx.DB, item apitypes.Workspace, version workspaceSQLVersion) error {
	values, err := workspaceSQLValues(item)
	if err != nil {
		return err
	}
	// Identity and creation time are immutable; configuration and activity update together.
	args := []any{values[3], values[6], values[7], values[8], values[9], values[10], values[11], item.Id, version.incarnation, version.revision}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE workspaces SET workflow_id=?,updated_at=?,last_active_at=?,parameters_json=?,toolkit_json=?,icon_json=?,labels_json=?,revision=revision+1 WHERE id=? AND incarnation=? AND revision=? AND pending_deletion_id IS NULL`), args...)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return errWorkspaceSQLConflict
	}
	if err := replaceSQLWorkspaceLabels(ctx, tx, item); err != nil {
		return err
	}
	return tx.Commit()
}

type workspaceSQLFilter struct {
	owner        *string
	ordinaryOnly bool
	activeOnly   bool
	labels       map[string]string
}

func listSQLWorkspaces(ctx context.Context, db *sqlx.DB, filter workspaceSQLFilter, cursor string, limit int) ([]apitypes.Workspace, bool, *string, error) {
	if limit <= 0 || limit > maxListLimit {
		return nil, false, nil, errors.New("workspace: invalid SQL page size")
	}
	var query strings.Builder
	query.WriteString("SELECT " + workspaceSQLColumns + " FROM workspaces w WHERE w.id>?")
	args := []any{cursor}
	if filter.owner != nil {
		query.WriteString(" AND w.owner_public_key=?")
		args = append(args, *filter.owner)
	}
	if filter.ordinaryOnly {
		query.WriteString(" AND w.system=0")
	}
	if filter.activeOnly {
		query.WriteString(" AND w.pending_deletion_id IS NULL")
	}
	keys := make([]string, 0, len(filter.labels))
	for key := range filter.labels {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		query.WriteString(" AND EXISTS (SELECT 1 FROM workspace_labels l WHERE l.workspace_id=w.id AND l.label_key=? AND l.label_value=?)")
		args = append(args, key, filter.labels[key])
	}
	query.WriteString(" ORDER BY w.id LIMIT ?")
	args = append(args, limit+1)
	rows, err := db.QueryContext(ctx, db.Rebind(query.String()), args...)
	if err != nil {
		return nil, false, nil, err
	}
	defer rows.Close()
	items := make([]apitypes.Workspace, 0, limit+1)
	for rows.Next() {
		item, _, err := scanSQLWorkspace(rows)
		if err != nil {
			return nil, false, nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, nil, err
	}
	if len(items) <= limit {
		return items, false, nil, nil
	}
	next := items[limit-1].Id
	return items[:limit], true, &next, nil
}

func listAllSQLWorkspaces(ctx context.Context, db *sqlx.DB, filter workspaceSQLFilter) ([]apitypes.Workspace, error) {
	items := make([]apitypes.Workspace, 0)
	cursor := ""
	for {
		page, more, next, err := listSQLWorkspaces(ctx, db, filter, cursor, maxListLimit)
		if err != nil {
			return nil, err
		}
		items = append(items, page...)
		if !more {
			return items, nil
		}
		if next == nil || *next <= cursor {
			return nil, errors.New("workspace: invalid SQL continuation")
		}
		cursor = *next
	}
}

func (s *Server) workspaceForMutation(ctx context.Context, id string) (apitypes.Workspace, workspaceSQLVersion, error) {
	db, err := s.store()
	if err != nil {
		return apitypes.Workspace{}, workspaceSQLVersion{}, err
	}
	if err := s.ensureWorkspaceAvailable(ctx, id); err != nil {
		return apitypes.Workspace{}, workspaceSQLVersion{}, err
	}
	item, version, err := getSQLWorkspaceByID(ctx, db, id)
	if err != nil {
		return item, version, err
	}
	if err := s.ensureWorkspaceOwnerAvailable(ctx, item); err != nil {
		return item, version, err
	}
	return item, version, nil
}

func workspaceDeletionByResource(ctx context.Context, db *sqlx.DB, id string) (pendingdeletion.Record, error) {
	record, err := NewPendingDeletionSource(db).GetByResource(ctx, id)
	if errors.Is(err, pendingdeletion.ErrNotFound) {
		return record, sql.ErrNoRows
	}
	return record, err
}

func deleteSQLWorkspace(ctx context.Context, db *sqlx.DB, id string, version workspaceSQLVersion) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE workspaces SET revision=revision WHERE id=? AND incarnation=? AND revision=? AND pending_deletion_id IS NULL`), id, version.incarnation, version.revision)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errWorkspaceSQLConflict
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM workspace_labels WHERE workspace_id=?`), id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM workspaces WHERE id=?`), id); err != nil {
		return err
	}
	return tx.Commit()
}

func bumpSQLWorkspaceActivity(ctx context.Context, db *sqlx.DB, id string, active time.Time) error {
	stamp := active.UTC().Format(workspaceSQLTimeLayout)
	result, err := db.ExecContext(ctx, db.Rebind(`UPDATE workspaces SET last_active_at=CASE WHEN last_active_at<? THEN ? ELSE last_active_at END,revision=revision+CASE WHEN last_active_at<? THEN 1 ELSE 0 END WHERE id=? AND pending_deletion_id IS NULL`), stamp, stamp, stamp, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errWorkspaceSQLConflict
	}
	return nil
}
