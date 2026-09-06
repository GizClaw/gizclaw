package flowstate

import (
	"context"
	"errors"

	"github.com/jmoiron/sqlx"
)

// WorkspaceCleanup adapts the SQL state store to Workspace artifact cleanup.
type WorkspaceCleanup struct{ DB *sqlx.DB }

// DeleteWorkspaceState retires the Workspace and deletes its checkpoints.
func (c WorkspaceCleanup) DeleteWorkspaceState(ctx context.Context, owner, workspace string) error {
	return RetireWorkspace(ctx, c.DB, owner, workspace)
}

// WorkspaceStateAbsent verifies both deletion and the permanent stale-writer fence.
func (c WorkspaceCleanup) WorkspaceStateAbsent(ctx context.Context, owner, workspace string) (bool, error) {
	if c.DB == nil {
		return false, errors.New("flowcraft state: database is not configured")
	}
	var absent bool
	err := c.DB.QueryRowContext(ctx, c.DB.Rebind(`SELECT EXISTS(SELECT 1 FROM flowcraft_state_scopes WHERE owner_id=? AND workspace_id=? AND retired=1) AND NOT EXISTS(SELECT 1 FROM flowcraft_board_states WHERE owner_id=? AND workspace_id=?)`), owner, workspace, owner, workspace).Scan(&absent)
	return absent, err
}
