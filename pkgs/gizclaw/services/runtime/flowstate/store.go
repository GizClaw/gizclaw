// Package flowstate persists scoped Flowcraft Board checkpoints in SQL.
package flowstate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

// ErrRetired indicates that a Workspace's checkpoint scope has been retired.
var ErrRetired = errors.New("flowcraft state: workspace scope is retired")

// Store is the checkpoint view of one owner, Workspace, and Agent.
// Its database connection pool belongs to the host.
type Store struct {
	db        *sqlx.DB
	owner     string
	workspace string
	agent     string
}

// Initialize creates the checkpoint tables once during host startup.
func Initialize(ctx context.Context, db *sqlx.DB) error {
	if db == nil {
		return errors.New("flowcraft state: database is not configured")
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, query := range []string{
		`CREATE TABLE IF NOT EXISTS flowcraft_state_scopes(owner_id TEXT NOT NULL,workspace_id TEXT NOT NULL,retired INTEGER NOT NULL DEFAULT 0 CHECK(retired IN (0,1)),PRIMARY KEY(owner_id,workspace_id))`,
		`CREATE TABLE IF NOT EXISTS flowcraft_board_states(owner_id TEXT NOT NULL,workspace_id TEXT NOT NULL,agent_id TEXT NOT NULL,context_id TEXT NOT NULL,state_json TEXT NOT NULL,PRIMARY KEY(owner_id,workspace_id,agent_id,context_id),FOREIGN KEY(owner_id,workspace_id) REFERENCES flowcraft_state_scopes(owner_id,workspace_id))`,
	} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// OpenScope registers an active Workspace checkpoint scope and returns its Agent view.
// A retired Workspace cannot be reopened by a stale Agent.
func OpenScope(ctx context.Context, db *sqlx.DB, owner, workspace, agent string) (*Store, error) {
	if db == nil {
		return nil, errors.New("flowcraft state: database is not configured")
	}
	if strings.TrimSpace(workspace) == "" || strings.TrimSpace(agent) == "" {
		return nil, errors.New("flowcraft state: Workspace and Agent IDs are required")
	}
	result, err := db.ExecContext(ctx, db.Rebind(`INSERT INTO flowcraft_state_scopes(owner_id,workspace_id,retired) VALUES (?,?,0) ON CONFLICT(owner_id,workspace_id) DO UPDATE SET workspace_id=excluded.workspace_id WHERE flowcraft_state_scopes.retired=0`), owner, workspace)
	if err != nil {
		return nil, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, ErrRetired
	}
	return &Store{db: db, owner: owner, workspace: workspace, agent: agent}, nil
}

// LoadState reads one checkpoint; an absent or retired checkpoint returns nil.
func (s *Store) LoadState(ctx context.Context, contextID string) ([]byte, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("flowcraft state: store is not configured")
	}
	var value string
	err := s.db.QueryRowContext(ctx, s.db.Rebind(`SELECT b.state_json FROM flowcraft_board_states b JOIN flowcraft_state_scopes w ON w.owner_id=b.owner_id AND w.workspace_id=b.workspace_id WHERE b.owner_id=? AND b.workspace_id=? AND b.agent_id=? AND b.context_id=? AND w.retired=0`), s.owner, s.workspace, s.agent, contextID).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []byte(value), nil
}

// SaveState atomically replaces one checkpoint while fencing Workspace retirement.
func (s *Store) SaveState(ctx context.Context, contextID string, data []byte) error {
	if s == nil || s.db == nil {
		return errors.New("flowcraft state: store is not configured")
	}
	if !json.Valid(data) {
		return errors.New("flowcraft state: invalid checkpoint JSON")
	}
	lock := ""
	if s.db.DriverName() == "postgres" || s.db.DriverName() == "pgx" {
		lock = " FOR UPDATE"
	}
	query := `WITH active_scope AS (SELECT owner_id,workspace_id FROM flowcraft_state_scopes WHERE owner_id=? AND workspace_id=? AND retired=0` + lock + `) INSERT INTO flowcraft_board_states(owner_id,workspace_id,agent_id,context_id,state_json) SELECT owner_id,workspace_id,?,?,? FROM active_scope WHERE 1=1 ON CONFLICT(owner_id,workspace_id,agent_id,context_id) DO UPDATE SET state_json=excluded.state_json`
	result, err := s.db.ExecContext(ctx, s.db.Rebind(query), s.owner, s.workspace, s.agent, contextID, string(data))
	if err != nil {
		return fmt.Errorf("flowcraft state: save checkpoint: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrRetired
	}
	return nil
}

// RetireWorkspace deletes all of a Workspace's checkpoints and rejects stale writes.
// The small retirement marker remains after checkpoint deletion.
func RetireWorkspace(ctx context.Context, db *sqlx.DB, owner, workspace string) error {
	if db == nil {
		return errors.New("flowcraft state: database is not configured")
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO flowcraft_state_scopes(owner_id,workspace_id,retired) VALUES (?,?,1) ON CONFLICT(owner_id,workspace_id) DO UPDATE SET retired=1`), owner, workspace); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM flowcraft_board_states WHERE owner_id=? AND workspace_id=?`), owner, workspace); err != nil {
		return err
	}
	return tx.Commit()
}
