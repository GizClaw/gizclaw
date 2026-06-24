package acl

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func (s *Server) Migration(ctx context.Context) error {
	if s == nil {
		return errors.New("acl: nil server")
	}
	if s.DB == nil {
		return errors.New("acl: sql db not configured")
	}
	files, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()
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

func ensurePolicyBindingDisplayOrderColumn(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(acl_policy_bindings)`)
	if err != nil {
		return err
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
			return err
		}
		if name == "display_order" {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `ALTER TABLE acl_policy_bindings ADD COLUMN display_order REAL NOT NULL DEFAULT 0`)
	return err
}
