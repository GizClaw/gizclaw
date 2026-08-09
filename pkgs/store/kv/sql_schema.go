package kv

import (
	"context"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/store/internal/sqlbackend"
	"github.com/jmoiron/sqlx"
)

func ensureSQLSchema(ctx context.Context, db *sqlx.DB, backend sqlbackend.Backend) error {
	binaryType := "BLOB"
	if backend.Dialect == sqlbackend.PostgreSQL {
		binaryType = "BYTEA"
	}
	indexName, err := sqlbackend.IndexName(backend, "expires_idx")
	if err != nil {
		return err
	}
	quotedIndex, err := sqlbackend.Quote(backend.Dialect, indexName)
	if err != nil {
		return err
	}
	return sqlbackend.Ensure(
		ctx,
		db,
		backend,
		fmt.Sprintf(
			"CREATE TABLE IF NOT EXISTS %s (encoded_key %s NOT NULL PRIMARY KEY, value %s NOT NULL, expires_at_unix_nano BIGINT NULL)",
			backend.Quoted, binaryType, binaryType,
		),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s (expires_at_unix_nano)", quotedIndex, backend.Quoted),
	)
}
