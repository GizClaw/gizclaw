package kv

import (
	"context"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/jmoiron/sqlx"
)

func ensureSQLSchema(ctx context.Context, db *sqlx.DB, table storage.SQLTable) error {
	binaryType := "BLOB"
	if table.Dialect() == storage.SQLDialectPostgreSQL {
		binaryType = "BYTEA"
	}
	indexName, err := storage.SQLIndexName(table, "expires_idx")
	if err != nil {
		return err
	}
	quotedIndex, err := storage.QuoteSQLIdentifier(table.Dialect(), indexName)
	if err != nil {
		return err
	}
	membersName, err := storage.SQLIndexName(table, "members")
	if err != nil {
		return err
	}
	members, err := storage.QuoteSQLIdentifier(table.Dialect(), membersName)
	if err != nil {
		return err
	}
	return storage.EnsureSQLTable(
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
