package logstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"

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
	// Single-stream pages seek one stream's time range in order instead of
	// sorting every record of that stream.
	{purpose: "stream_page_idx", columns: "stream, timestamp_unix_nano, id", want: []string{"stream", "timestamp_unix_nano", "id"}},
	{purpose: "expiry_idx", columns: "expires_at_unix_nano", want: []string{"expires_at_unix_nano"}},
}

func ensureSQLSchema(ctx context.Context, db *sqlx.DB, table, keysTable storage.SQLTable, ttl time.Duration) error {
	payloadType := "BLOB"
	if table.Dialect() == storage.SQLDialectPostgreSQL {
		payloadType = "BYTEA"
	}
	partitioned := table.Dialect() == storage.SQLDialectPostgreSQL && ttl > 0
	var createTable string
	if partitioned {
		createTable = fmt.Sprintf(
			"CREATE TABLE IF NOT EXISTS %s (stream TEXT NOT NULL, id TEXT NOT NULL, timestamp_unix_nano BIGINT NOT NULL, expires_at_unix_nano BIGINT NOT NULL, kind TEXT NOT NULL, severity TEXT NOT NULL, message TEXT NOT NULL, attributes_json TEXT NOT NULL, payload_json %s NOT NULL) PARTITION BY RANGE (expires_at_unix_nano)",
			table.Quoted(), payloadType,
		)
	} else {
		createTable = fmt.Sprintf(
			"CREATE TABLE IF NOT EXISTS %s (stream TEXT NOT NULL, id TEXT NOT NULL, timestamp_unix_nano BIGINT NOT NULL, expires_at_unix_nano BIGINT, kind TEXT NOT NULL, severity TEXT NOT NULL, message TEXT NOT NULL, attributes_json TEXT NOT NULL, payload_json %s NOT NULL, PRIMARY KEY (stream, id))",
			table.Quoted(), payloadType,
		)
	}
	statements := []string{createTable}
	if partitioned {
		statements = append(statements, fmt.Sprintf(
			"CREATE TABLE IF NOT EXISTS %s (stream TEXT NOT NULL, id TEXT NOT NULL, expires_at_unix_nano BIGINT NOT NULL, PRIMARY KEY (stream, id))",
			keysTable.Quoted(),
		))
	}
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
	if partitioned {
		name, err := storage.SQLIndexName(keysTable, "expiry_idx")
		if err != nil {
			return err
		}
		quoted, err := storage.QuoteSQLIdentifier(keysTable.Dialect(), name)
		if err != nil {
			return err
		}
		statements = append(statements, fmt.Sprintf(
			"CREATE INDEX IF NOT EXISTS %s ON %s (expires_at_unix_nano)",
			quoted,
			keysTable.Quoted(),
		))
	}
	return storage.EnsureSQLTable(ctx, db, table, statements...)
}

const postgresAuxiliaryPrefixLimit = 53

func postgresAuxiliaryPrefix(base string) string {
	if len(base) <= postgresAuxiliaryPrefixLimit {
		return base
	}
	digest := sha256.Sum256([]byte("gizclaw/logstore/postgres/v1\x00" + base))
	return fmt.Sprintf("%s_%x", base[:postgresAuxiliaryPrefixLimit-9], digest[:4])
}

func postgresAuxiliaryName(base, suffix string) (string, error) {
	name := postgresAuxiliaryPrefix(base) + "_" + suffix
	if err := storage.ValidateSQLIdentifier(name); err != nil {
		return "", err
	}
	return name, nil
}

type postgresPartition struct {
	name  string
	day   time.Time
	lower int64
	upper int64
}

type sqlRowsQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (store *SQLStore) checkPostgresPartitionSchema(ctx context.Context) error {
	rows, err := store.db.QueryContext(ctx, `
		SELECT partitioned.partstrat, attribute.attname
		FROM pg_partitioned_table partitioned
		JOIN pg_class parent ON parent.oid = partitioned.partrelid
		JOIN pg_namespace backend ON backend.oid = parent.relnamespace
		CROSS JOIN LATERAL unnest(partitioned.partattrs) WITH ORDINALITY AS key(attnum, position)
		JOIN pg_attribute attribute ON attribute.attrelid = parent.oid AND attribute.attnum = key.attnum
		WHERE backend.nspname = current_schema() AND parent.relname = $1
		ORDER BY key.position`, store.table.Name())
	if err != nil {
		return fmt.Errorf("logstore: inspect postgres partition key: %w", err)
	}
	defer rows.Close()
	var strategies, columns []string
	for rows.Next() {
		var strategy, column string
		if err := rows.Scan(&strategy, &column); err != nil {
			return fmt.Errorf("logstore: scan postgres partition key: %w", err)
		}
		strategies = append(strategies, strategy)
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("logstore: inspect postgres partition key rows: %w", err)
	}
	if len(strategies) != 1 || strategies[0] != "r" || columns[0] != "expires_at_unix_nano" {
		return fmt.Errorf("logstore: incompatible postgres partition key: strategy=%v columns=%v", strategies, columns)
	}
	keyColumns, err := storage.SQLColumns(ctx, store.db, store.keysTable)
	if err != nil {
		return fmt.Errorf("logstore: inspect postgres key table: %w", err)
	}
	wantKeys := map[string]storage.SQLColumn{
		"stream":               {Type: "TEXT", Nullable: false, PrimaryKeyPosition: 1},
		"id":                   {Type: "TEXT", Nullable: false, PrimaryKeyPosition: 2},
		"expires_at_unix_nano": {Type: "BIGINT", Nullable: false},
	}
	if err := storage.ValidateSQLColumns(keyColumns, wantKeys); err != nil {
		return fmt.Errorf("logstore: incompatible postgres key table %q: %w", store.keysTable.Name(), err)
	}
	indexName, err := storage.SQLIndexName(store.keysTable, "expiry_idx")
	if err != nil {
		return fmt.Errorf("logstore: derive postgres key expiry index: %w", err)
	}
	if err := storage.ValidateSQLIndexColumns(ctx, store.db, store.keysTable, indexName, []string{"expires_at_unix_nano"}); err != nil {
		return fmt.Errorf("logstore: incompatible postgres key expiry index: %w", err)
	}
	partitions, err := store.postgresPartitions(ctx, store.db)
	if err != nil {
		return err
	}
	for _, partition := range partitions {
		if err := validatePostgresPartition(partition); err != nil {
			return err
		}
	}
	return nil
}

func (store *SQLStore) maintainPostgresPartitions(ctx context.Context) error {
	tx, err := store.db.BeginTxx(ctx, nil)
	if err != nil {
		return storage.ExternalSQLError("logstore: begin postgres partition maintenance", err)
	}
	defer tx.Rollback()
	if err := storage.LockPostgreSQLTable(ctx, tx, store.table); err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := store.maintainPostgresPartitionsLocked(ctx, tx, now, now.Add(store.ttl)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return storage.ExternalSQLError("logstore: commit postgres partition maintenance", err)
	}
	return nil
}

func (store *SQLStore) dailyPartitions() storage.SQLDailyPartitions {
	return storage.SQLDailyPartitions{Table: store.table, Column: "expires_at_unix_nano", Prefix: postgresAuxiliaryPrefix(store.table.Name())}
}

func (store *SQLStore) preparePostgresPartitions(ctx context.Context, expiresAt time.Time) error {
	tx, err := store.beginTransaction(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := store.dailyPartitions().Prepare(ctx, tx, []time.Time{expiresAt}); err != nil {
		return err
	}
	return storage.ExternalSQLError("logstore: commit postgres partition preparation", tx.Commit())
}

func (store *SQLStore) maintainPostgresPartitionsLocked(ctx context.Context, tx *sqlx.Tx, now, expiresAt time.Time) error {
	return store.dailyPartitions().Maintain(ctx, tx, now, []time.Time{expiresAt}, func(partition storage.SQLDailyPartition) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM "+store.quotedKeys+" WHERE expires_at_unix_nano >= $1 AND expires_at_unix_nano < $2", partition.Lower, partition.Upper)
		return storage.ExternalSQLError("logstore: delete expired postgres partition keys", err)
	})
}

func postgresPartitionDay(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func (store *SQLStore) postgresPartition(day time.Time) (postgresPartition, error) {
	day = postgresPartitionDay(day)
	name, err := postgresAuxiliaryName(store.table.Name(), "p"+day.Format("20060102"))
	if err != nil {
		return postgresPartition{}, fmt.Errorf("logstore: derive postgres partition name: %w", err)
	}
	lower, err := storage.SQLUnixNano(day)
	if err != nil {
		return postgresPartition{}, fmt.Errorf("logstore: postgres partition lower bound: %w", err)
	}
	upper, err := storage.SQLUnixNano(day.AddDate(0, 0, 1))
	if err != nil {
		return postgresPartition{}, fmt.Errorf("logstore: postgres partition upper bound: %w", err)
	}
	return postgresPartition{name: name, day: day, lower: lower, upper: upper}, nil
}

func (store *SQLStore) postgresPartitions(ctx context.Context, queryer sqlRowsQueryer) ([]postgresPartition, error) {
	children, err := store.dailyPartitions().List(ctx, queryer)
	if err != nil {
		return nil, err
	}
	out := make([]postgresPartition, 0, len(children))
	for _, child := range children {
		out = append(out, postgresPartition{name: child.Name, day: child.Day, lower: child.Lower, upper: child.Upper})
	}
	return out, nil
}

func validatePostgresPartition(partition postgresPartition) error {
	if partition.name == "" || partition.day.IsZero() || partition.lower >= partition.upper {
		return fmt.Errorf("logstore: invalid postgres partition metadata for %q", partition.name)
	}
	return nil
}
