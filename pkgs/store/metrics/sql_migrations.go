package metrics

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/store/internal/sqlmigration"
	"github.com/pressly/goose/v3"
)

func metricSQLMigrations(namespace sqlmigration.Namespace) []*goose.Migration {
	return []*goose.Migration{goose.NewGoMigration(1, &goose.GoFunc{RunTx: func(ctx context.Context, tx *sql.Tx) error {
		table, err := sqlmigration.Quote(namespace.Dialect, namespace.Table)
		if err != nil {
			return err
		}
		sequence := "INTEGER PRIMARY KEY AUTOINCREMENT"
		if namespace.Dialect == sqlmigration.PostgreSQL {
			sequence = "BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY"
		}
		if err := sqlmigration.TxExec(ctx, tx, fmt.Sprintf(
			"CREATE TABLE %s (sequence %s, metric TEXT NOT NULL, series_key TEXT NOT NULL, labels_json TEXT NOT NULL, timestamp_unix_nano BIGINT NOT NULL, value_bits BIGINT NOT NULL)",
			table, sequence,
		)); err != nil {
			return err
		}
		base := strings.TrimSuffix(namespace.VersionTable, "_schema_versions")
		for _, indexSpec := range []struct {
			suffix, columns string
		}{
			{suffix: "series_idx", columns: "metric, series_key, timestamp_unix_nano, sequence"},
			{suffix: "metric_idx", columns: "metric, timestamp_unix_nano"},
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
