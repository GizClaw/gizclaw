package acl

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"

	"github.com/jmoiron/sqlx"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

const postgresMigrationLockID int64 = 0x47495A434C41574C

func (s *Server) Migration(ctx context.Context) error {
	if s == nil {
		return errors.New("acl: nil server")
	}
	if s.DB == nil {
		return errors.New("acl: sql db not configured")
	}
	if err := validateDialect(s.DB.DriverName()); err != nil {
		return err
	}
	files, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	if tx.DriverName() == "postgres" {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`SELECT pg_advisory_xact_lock(?)`), postgresMigrationLockID); err != nil {
			return fmt.Errorf("acl: acquire migration lock: %w", err)
		}
	}
	for _, file := range files {
		data, err := migrationFS.ReadFile(file)
		if err != nil {
			return fmt.Errorf("acl migration %s: %w", file, err)
		}
		if _, err := tx.ExecContext(ctx, string(data)); err != nil {
			return fmt.Errorf("acl migration %s: %w", file, err)
		}
		if file == "migrations/0001_create_acl_tables.sql" {
			if err := ensurePolicyBindingDisplayOrderColumn(ctx, tx); err != nil {
				return fmt.Errorf("acl migration %s: %w", file, err)
			}
		}
	}
	return tx.Commit()
}

func ensurePolicyBindingDisplayOrderColumn(ctx context.Context, tx *sqlx.Tx) error {
	exists, err := policyBindingDisplayOrderColumnExists(ctx, tx)
	if err != nil || exists {
		return err
	}
	_, err = tx.ExecContext(ctx, `ALTER TABLE acl_policy_bindings ADD COLUMN display_order REAL NOT NULL DEFAULT 0`)
	return err
}

func policyBindingDisplayOrderColumnExists(ctx context.Context, tx *sqlx.Tx) (bool, error) {
	switch tx.DriverName() {
	case "sqlite":
		rows, err := tx.QueryContext(ctx, `PRAGMA table_info(acl_policy_bindings)`)
		if err != nil {
			return false, err
		}
		defer rows.Close()
		for rows.Next() {
			var cid int
			var name string
			var typ string
			var notNull int
			var defaultValue any
			var pk int
			if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
				return false, err
			}
			if name == "display_order" {
				return true, nil
			}
		}
		return false, rows.Err()
	case "postgres":
		var exists bool
		err := tx.QueryRowContext(ctx, tx.Rebind(`
SELECT EXISTS (
	SELECT 1
	FROM information_schema.columns
	WHERE table_schema = current_schema()
	  AND table_name = ?
	  AND column_name = ?
)`), "acl_policy_bindings", "display_order").Scan(&exists)
		return exists, err
	default:
		return false, fmt.Errorf("acl: unsupported sql dialect %q", tx.DriverName())
	}
}

func validateDialect(driverName string) error {
	switch driverName {
	case "sqlite", "postgres":
		return nil
	default:
		return fmt.Errorf("acl: unsupported sql dialect %q", driverName)
	}
}
