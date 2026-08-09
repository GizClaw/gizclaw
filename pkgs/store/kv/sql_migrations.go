package kv

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/store/internal/sqlmigration"
	"github.com/pressly/goose/v3"
)

func sqlMigrations(namespace sqlmigration.Namespace) []*goose.Migration {
	return []*goose.Migration{goose.NewGoMigration(1, &goose.GoFunc{RunTx: func(ctx context.Context, tx *sql.Tx) error {
		table, err := sqlmigration.Quote(namespace.Dialect, namespace.Table)
		if err != nil {
			return err
		}
		columnType := "BLOB"
		if namespace.Dialect == sqlmigration.PostgreSQL {
			columnType = "BYTEA"
		}
		if err := sqlmigration.TxExec(ctx, tx, fmt.Sprintf(
			"CREATE TABLE %s (encoded_key %s NOT NULL PRIMARY KEY, value %s NOT NULL, expires_at_unix_nano BIGINT NULL)",
			table, columnType, columnType,
		)); err != nil {
			return err
		}
		indexName := strings.TrimSuffix(namespace.VersionTable, "_schema_versions") + "_expires_idx"
		quotedIndex, err := sqlmigration.Quote(namespace.Dialect, indexName)
		if err != nil {
			return err
		}
		return sqlmigration.TxExec(ctx, tx, fmt.Sprintf(
			"CREATE INDEX %s ON %s (expires_at_unix_nano)", quotedIndex, table,
		))
	}}, nil)}
}
