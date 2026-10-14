package kv

import (
	"context"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/store/sqlmeta"
	"github.com/jmoiron/sqlx"
)

func ensureSQLSchema(ctx context.Context, db *sqlx.DB, table sqlmeta.SQLTable) error {
	binaryType := "BLOB"
	if table.Dialect() == sqlmeta.SQLDialectPostgreSQL {
		binaryType = "BYTEA"
	}
	indexName, err := sqlmeta.SQLIndexName(table, "expires_idx")
	if err != nil {
		return err
	}
	quotedIndex, err := sqlmeta.QuoteSQLIdentifier(table.Dialect(), indexName)
	if err != nil {
		return err
	}
	membersName, err := sqlmeta.SQLIndexName(table, "members")
	if err != nil {
		return err
	}
	members, err := sqlmeta.QuoteSQLIdentifier(table.Dialect(), membersName)
	if err != nil {
		return err
	}
	return sqlmeta.EnsureSQLTable(
		ctx,
		db,
		table,
		fmt.Sprintf(
			"CREATE TABLE IF NOT EXISTS %s (encoded_key %s NOT NULL PRIMARY KEY, value %s NOT NULL, value_kind BIGINT NOT NULL DEFAULT 0, expires_at_unix_nano BIGINT NULL)",
			table.Quoted(), binaryType, binaryType,
		),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s (expires_at_unix_nano)", quotedIndex, table.Quoted()),
		fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (encoded_key %s NOT NULL, member %s NOT NULL, PRIMARY KEY(encoded_key, member))", members, binaryType, binaryType),
	)
}
