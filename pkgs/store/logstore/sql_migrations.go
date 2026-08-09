package logstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/store/internal/sqlmigration"
	"github.com/pressly/goose/v3"
)

func logSQLMigrations(namespace sqlmigration.Namespace) []*goose.Migration {
	return []*goose.Migration{goose.NewGoMigration(1, &goose.GoFunc{RunTx: func(ctx context.Context, tx *sql.Tx) error {
		table, err := sqlmigration.Quote(namespace.Dialect, namespace.Table)
		if err != nil {
			return err
		}
		payloadType := "BLOB"
		if namespace.Dialect == sqlmigration.PostgreSQL {
			payloadType = "BYTEA"
		}
		if err := sqlmigration.TxExec(ctx, tx, fmt.Sprintf(
			"CREATE TABLE %s (stream TEXT NOT NULL, id TEXT NOT NULL, timestamp_unix_nano BIGINT NOT NULL, kind TEXT NOT NULL, severity TEXT NOT NULL, message TEXT NOT NULL, attributes_json TEXT NOT NULL, payload_json %s NOT NULL, PRIMARY KEY (stream, id))",
			table, payloadType,
		)); err != nil {
			return err
		}
		base := strings.TrimSuffix(namespace.VersionTable, "_schema_versions")
		for _, indexSpec := range []struct {
			suffix, columns string
		}{
			{suffix: "page_idx", columns: "timestamp_unix_nano, stream, id"},
			{suffix: "selector_idx", columns: "stream, kind, severity"},
		} {
			index, err := sqlmigration.Quote(namespace.Dialect, base+"_"+indexSpec.suffix)
			if err != nil {
				return err
			}
			if err := sqlmigration.TxExec(ctx, tx, fmt.Sprintf("CREATE INDEX %s ON %s (%s)", index, table, indexSpec.columns)); err != nil {
				return err
			}
		}
		return nil
	}}, nil)}
}
