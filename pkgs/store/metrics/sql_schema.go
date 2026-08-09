package metrics

import (
	"context"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/jmoiron/sqlx"
)

var sqlMetricIndexes = []struct {
	purpose string
	columns string
	want    []string
}{
	{purpose: "series_idx", columns: "metric, series_key, timestamp_unix_nano, sequence", want: []string{"metric", "series_key", "timestamp_unix_nano", "sequence"}},
	{purpose: "metric_idx", columns: "metric, timestamp_unix_nano", want: []string{"metric", "timestamp_unix_nano"}},
}

func ensureSQLSchema(ctx context.Context, db *sqlx.DB, table storage.SQLTable) error {
	sequence := "INTEGER PRIMARY KEY AUTOINCREMENT"
	if table.Dialect() == storage.SQLDialectPostgreSQL {
		sequence = "BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY"
	}
	statements := []string{fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s (sequence %s, metric TEXT NOT NULL, series_key TEXT NOT NULL, labels_json TEXT NOT NULL, timestamp_unix_nano BIGINT NOT NULL, value_bits BIGINT NOT NULL)",
		table.Quoted(), sequence,
	)}
	for _, index := range sqlMetricIndexes {
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
