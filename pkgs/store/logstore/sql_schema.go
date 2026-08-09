package logstore

import (
	"context"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/jmoiron/sqlx"
)

var sqlLogIndexes = []struct {
	purpose string
	columns string
	want    []string
}{
	{purpose: "page_idx", columns: "timestamp_unix_nano, stream, id", want: []string{"timestamp_unix_nano", "stream", "id"}},
	{purpose: "selector_idx", columns: "stream, kind, severity", want: []string{"stream", "kind", "severity"}},
}

func ensureSQLSchema(ctx context.Context, db *sqlx.DB, table storage.SQLTable) error {
	payloadType := "BLOB"
	if table.Dialect() == storage.SQLDialectPostgreSQL {
		payloadType = "BYTEA"
	}
	statements := []string{fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s (stream TEXT NOT NULL, id TEXT NOT NULL, timestamp_unix_nano BIGINT NOT NULL, kind TEXT NOT NULL, severity TEXT NOT NULL, message TEXT NOT NULL, attributes_json TEXT NOT NULL, payload_json %s NOT NULL, PRIMARY KEY (stream, id))",
		table.Quoted(), payloadType,
	)}
	for _, index := range sqlLogIndexes {
		name, err := storage.SQLIndexName(table, index.purpose)
		if err != nil {
			return err
		}
		quoted, err := storage.QuoteSQLIdentifier(table.Dialect(), name)
		if err != nil {
			return err
		}
		statements = append(statements, fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s (%s)", quoted, table.Quoted(), index.columns))
	}
	return storage.EnsureSQLTable(ctx, db, table, statements...)
}
