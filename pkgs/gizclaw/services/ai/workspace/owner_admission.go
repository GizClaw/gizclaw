package workspace

import (
	"context"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/jmoiron/sqlx"
)

// lockWorkspaceOwner serializes SQL admission with retirement inventory capture.
// The retired value is permanent; even another Server using a stale Peer view
// cannot insert an owned Workspace after the retirement snapshot commits.
func lockWorkspaceOwner(ctx context.Context, tx *sqlx.Tx, owner string, retire bool) error {
	retired := 0
	if retire {
		retired = 1
	}
	var current int
	err := tx.QueryRowContext(ctx, tx.Rebind(`INSERT INTO workspace_owner_state(owner_public_key,retired) VALUES (?,?) ON CONFLICT(owner_public_key) DO UPDATE SET retired=CASE WHEN workspace_owner_state.retired=1 THEN 1 ELSE excluded.retired END RETURNING retired`), owner, retired).Scan(&current)
	if err != nil {
		return err
	}
	if !retire && current != 0 {
		return errors.Join(ErrPeerPendingDeletion, ownership.ErrOwnerRetired)
	}
	return nil
}
