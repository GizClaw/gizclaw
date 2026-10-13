package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/jmoiron/sqlx"
)

// ErrHistoryMigrationRequired reports legacy conversation data that must be
// migrated offline before the Server opens the new History store.
var ErrHistoryMigrationRequired = errors.New("workspace history migration required")

// CheckHistoryMigration checks the selected SQL database for legacy History
// records without changing any schema or data. PostgreSQL inspection is limited
// to the connection's current schema and queries partition parents only. Table
// names come from the catalog, not defaults or the new configuration. Empty
// legacy tables and generic log archives without History records do not block.
func CheckHistoryMigration(ctx context.Context, db *sqlx.DB) error {
	if db == nil {
		return errors.New("history migration check: SQL database is nil")
	}
	var statement string
	switch db.DriverName() {
	case "postgres":
		statement = `SELECT n.nspname AS schema, c.relname AS name
			FROM pg_catalog.pg_class c
			JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
			WHERE n.nspname=current_schema() AND c.relkind IN ('r','p')
			  AND NOT EXISTS (SELECT 1 FROM pg_catalog.pg_inherits WHERE inhrelid=c.oid)
			  AND (SELECT count(*) FROM pg_catalog.pg_attribute a
			       WHERE a.attrelid=c.oid AND NOT a.attisdropped AND a.attnum>0
			         AND a.attname IN ('stream','timestamp_unix_nano','payload_json'))=3
			ORDER BY c.relname`
	case "sqlite":
		statement = `SELECT 'main' AS schema, t.name FROM main.sqlite_master t
			WHERE t.type='table'
			  AND (SELECT count(*) FROM pragma_table_info(t.name) p
			       WHERE p.name IN ('stream','timestamp_unix_nano','payload_json'))=3
			ORDER BY t.name`
	default:
		return fmt.Errorf("history migration check: unsupported SQL driver %q", db.DriverName())
	}
	// Materialize catalog names before probing rows so a one-connection pool
	// never waits on a catalog cursor it still owns.
	var tables []struct {
		Schema string
		Name   string
	}
	if err := db.SelectContext(ctx, &tables, statement); err != nil {
		return storage.ExternalSQLError("history migration check: inspect legacy tables", err)
	}
	var pending []string
	for _, table := range tables {
		quoted := quoteHistoryCatalogName(table.Schema) + "." + quoteHistoryCatalogName(table.Name)
		var exists bool
		if err := db.GetContext(ctx, &exists, "SELECT EXISTS(SELECT 1 FROM "+quoted+" WHERE stream LIKE 'workspace.history/%' OR stream IN ('eino.history','flowcraft.history'))"); err != nil {
			return storage.ExternalSQLError(fmt.Sprintf("history migration check: inspect table %s", quoted), err)
		}
		if exists {
			pending = append(pending, quoted)
		}
	}
	if len(pending) != 0 {
		return fmt.Errorf("%w: legacy records remain in %s; stop all Server writers and run gizclaw history-migrate <dir> --workspace-table <old Workspace parent> --eino-table <old Eino parent> before startup", ErrHistoryMigrationRequired, strings.Join(pending, ", "))
	}
	return nil
}

func quoteHistoryCatalogName(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
