package metrics

import (
	"context"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/store/internal/sqlbackend"
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

func ensureSQLSchema(ctx context.Context, db *sqlx.DB, backend sqlbackend.Backend) error {
	sequence := "INTEGER PRIMARY KEY AUTOINCREMENT"
	if backend.Dialect == sqlbackend.PostgreSQL {
		sequence = "BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY"
	}
	statements := []string{fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s (sequence %s, metric TEXT NOT NULL, series_key TEXT NOT NULL, labels_json TEXT NOT NULL, timestamp_unix_nano BIGINT NOT NULL, value_bits BIGINT NOT NULL)",
		backend.Quoted, sequence,
	)}
	for _, index := range sqlMetricIndexes {
		name, err := sqlbackend.IndexName(backend, index.purpose)
		if err != nil {
			return err
		}
		quoted, err := sqlbackend.Quote(backend.Dialect, name)
		if err != nil {
			return err
		}
		statements = append(statements, fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s (%s)", quoted, backend.Quoted, index.columns))
	}
	return sqlbackend.Ensure(ctx, db, backend, statements...)
}
