// Package graphstate persists scoped Eino Graph checkpoints in SQL.
package graphstate

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/jmoiron/sqlx"
)

// ErrRetired indicates that a Workspace's checkpoint scope has been retired.
var ErrRetired = errors.New("graph state: workspace scope is retired")

// Store is the checkpoint view of one owner, Workspace, and Agent.
// Its database connection pool belongs to the host.
type Store struct {
	db        *sqlx.DB
	owner     string
	workspace string
	agent     string
	initial   []byte
}

// Initialize creates the checkpoint tables once during host startup.
func Initialize(ctx context.Context, db *sqlx.DB) error {
	if db == nil {
		return errors.New("graph state: database is not configured")
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, query := range []string{
		`CREATE TABLE IF NOT EXISTS graph_state_scopes(owner_id TEXT NOT NULL,workspace_id TEXT NOT NULL,retired INTEGER NOT NULL DEFAULT 0 CHECK(retired IN (0,1)),PRIMARY KEY(owner_id,workspace_id))`,
		`CREATE TABLE IF NOT EXISTS graph_states(owner_id TEXT NOT NULL,workspace_id TEXT NOT NULL,agent_id TEXT NOT NULL,context_id TEXT NOT NULL,state_json TEXT NOT NULL,revision TEXT NOT NULL DEFAULT '',PRIMARY KEY(owner_id,workspace_id,agent_id,context_id),FOREIGN KEY(owner_id,workspace_id) REFERENCES graph_state_scopes(owner_id,workspace_id))`,
	} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// OpenScope registers an active Workspace checkpoint scope and returns its Agent view.
// A retired Workspace cannot be reopened by a stale Agent.
func OpenScope(ctx context.Context, db *sqlx.DB, owner, workspace, agent string, initial ...map[string]any) (*Store, error) {
	if db == nil {
		return nil, errors.New("graph state: database is not configured")
	}
	if strings.TrimSpace(workspace) == "" || strings.TrimSpace(agent) == "" {
		return nil, errors.New("graph state: Workspace and Agent IDs are required")
	}
	if len(initial) > 1 {
		return nil, errors.New("graph state: at most one initial snapshot is allowed")
	}
	var defaults []byte
	if len(initial) == 1 {
		var err error
		defaults, err = encodeSnapshot(initial[0], "")
		if err != nil {
			return nil, err
		}
	}
	result, err := db.ExecContext(ctx, db.Rebind(`INSERT INTO graph_state_scopes(owner_id,workspace_id,retired) VALUES (?,?,0) ON CONFLICT(owner_id,workspace_id) DO UPDATE SET workspace_id=excluded.workspace_id WHERE graph_state_scopes.retired=0`), owner, workspace)
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
	return &Store{db: db, owner: owner, workspace: workspace, agent: agent, initial: defaults}, nil
}

// loadCheckpoint reads one checkpoint; an absent or retired checkpoint returns nil.
func (s *Store) loadCheckpoint(ctx context.Context, contextID string) ([]byte, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("graph state: store is not configured")
	}
	var value string
	err := s.db.QueryRowContext(ctx, s.db.Rebind(`SELECT b.state_json FROM graph_states b JOIN graph_state_scopes w ON w.owner_id=b.owner_id AND w.workspace_id=b.workspace_id WHERE b.owner_id=? AND b.workspace_id=? AND b.agent_id=? AND b.context_id=? AND w.retired=0`), s.owner, s.workspace, s.agent, contextID).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []byte(value), nil
}

// RetireWorkspace deletes all of a Workspace's checkpoints and rejects stale writes.
// The small retirement marker remains after checkpoint deletion.
func RetireWorkspace(ctx context.Context, db *sqlx.DB, owner, workspace string) error {
	if db == nil {
		return errors.New("graph state: database is not configured")
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO graph_state_scopes(owner_id,workspace_id,retired) VALUES (?,?,1) ON CONFLICT(owner_id,workspace_id) DO UPDATE SET retired=1`), owner, workspace); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM graph_states WHERE owner_id=? AND workspace_id=?`), owner, workspace); err != nil {
		return err
	}
	return tx.Commit()
}
