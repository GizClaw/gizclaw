package gameplay

import (
	"context"
	"errors"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
)

// DeleteWorkspaceData removes only Gameplay rows whose primary scope is the
// exact Workspace. Pet rows and Pet-to-Workspace bindings are intentionally
// owned by Pet/Peer retirement and remain untouched.
func (r *Runtime) DeleteWorkspaceData(ctx context.Context, workspaceID string) error {
	if r == nil || r.DB == nil {
		return errors.New("gameplay: database not configured")
	}
	if err := customid.ValidateResourceID(workspaceID); err != nil {
		return fmt.Errorf("gameplay: invalid Workspace ID: %w", err)
	}
	tx, err := r.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := r.lockWorkspaceRewardSourceTx(ctx, tx, workspaceID); err != nil {
		return err
	}
	for _, statement := range []string{
		`DELETE FROM gameplay_drive_fact_outbox WHERE workspace_id = ?`,
		`DELETE FROM gameplay_workspace_reward_windows WHERE workspace_id = ?`,
		`DELETE FROM gameplay_workspace_reward_sources WHERE workspace_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, tx.Rebind(statement), workspaceID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// WorkspaceDataAbsent verifies the same exact-Workspace cleanup boundary.
func (r *Runtime) WorkspaceDataAbsent(ctx context.Context, workspaceID string) (bool, error) {
	if r == nil || r.DB == nil {
		return false, errors.New("gameplay: database not configured")
	}
	if err := customid.ValidateResourceID(workspaceID); err != nil {
		return false, fmt.Errorf("gameplay: invalid Workspace ID: %w", err)
	}
	var count int
	err := r.DB.QueryRowContext(ctx, r.DB.Rebind(`SELECT
		(SELECT COUNT(*) FROM gameplay_drive_fact_outbox WHERE workspace_id = ?) +
		(SELECT COUNT(*) FROM gameplay_workspace_reward_windows WHERE workspace_id = ?) +
		(SELECT COUNT(*) FROM gameplay_workspace_reward_sources WHERE workspace_id = ?)`),
		workspaceID, workspaceID, workspaceID).Scan(&count)
	return count == 0, err
}
