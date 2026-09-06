package workspace

import "github.com/jmoiron/sqlx"

const pendingDeletionSourceName = "workspace"

// NewPendingDeletionSource binds cleanup to the Workspace SQL transaction boundary.
func NewPendingDeletionSource(db *sqlx.DB) workspaceSQLDeletionSource {
	return workspaceSQLDeletionSource{DB: db}
}
