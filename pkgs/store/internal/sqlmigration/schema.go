package sqlmigration

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/jmoiron/sqlx"
)

// Column describes one persisted SQL column for constructor-time validation.
type Column struct {
	Type               string
	Nullable           bool
	PrimaryKeyPosition int
}

// Columns returns the exact current-schema columns for a validated table.
func Columns(ctx context.Context, db *sqlx.DB, namespace Namespace) (map[string]Column, error) {
	columns := map[string]Column{}
	switch namespace.Dialect {
	case SQLite:
		quoted, err := Quote(namespace.Dialect, namespace.Table)
		if err != nil {
			return nil, err
		}
		rows, err := db.QueryContext(ctx, "PRAGMA table_info("+quoted+")")
		if err != nil {
			return nil, fmt.Errorf("sqlmigration: inspect sqlite columns for %q: %w", namespace.Table, err)
		}
		defer rows.Close()
		for rows.Next() {
			var cid, notNull, primaryKey int
			var name, columnType string
			var defaultValue sql.NullString
			if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
				return nil, fmt.Errorf("sqlmigration: scan sqlite columns for %q: %w", namespace.Table, err)
			}
			columns[name] = Column{Type: strings.ToUpper(columnType), Nullable: notNull == 0, PrimaryKeyPosition: primaryKey}
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("sqlmigration: inspect sqlite column rows for %q: %w", namespace.Table, err)
		}
	case PostgreSQL:
		rows, err := db.QueryContext(ctx, `
			SELECT column_name, data_type, is_nullable = 'YES'
			FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = $1
			ORDER BY ordinal_position`, namespace.Table)
		if err != nil {
			return nil, fmt.Errorf("sqlmigration: inspect postgres columns for %q: %w", namespace.Table, err)
		}
		for rows.Next() {
			var name, columnType string
			var nullable bool
			if err := rows.Scan(&name, &columnType, &nullable); err != nil {
				rows.Close()
				return nil, fmt.Errorf("sqlmigration: scan postgres columns for %q: %w", namespace.Table, err)
			}
			columns[name] = Column{Type: strings.ToUpper(columnType), Nullable: nullable}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("sqlmigration: inspect postgres column rows for %q: %w", namespace.Table, err)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("sqlmigration: close postgres column rows for %q: %w", namespace.Table, err)
		}
		primaryRows, err := db.QueryContext(ctx, `
			SELECT kcu.column_name, kcu.ordinal_position
			FROM information_schema.table_constraints tc
			JOIN information_schema.key_column_usage kcu
			  ON tc.constraint_name = kcu.constraint_name
			 AND tc.constraint_schema = kcu.constraint_schema
			WHERE tc.table_schema = current_schema()
			  AND tc.table_name = $1
			  AND tc.constraint_type = 'PRIMARY KEY'`, namespace.Table)
		if err != nil {
			return nil, fmt.Errorf("sqlmigration: inspect postgres primary key for %q: %w", namespace.Table, err)
		}
		defer primaryRows.Close()
		for primaryRows.Next() {
			var name string
			var position int
			if err := primaryRows.Scan(&name, &position); err != nil {
				return nil, fmt.Errorf("sqlmigration: scan postgres primary key for %q: %w", namespace.Table, err)
			}
			column := columns[name]
			column.PrimaryKeyPosition = position
			columns[name] = column
		}
		if err := primaryRows.Err(); err != nil {
			return nil, fmt.Errorf("sqlmigration: inspect postgres primary key rows for %q: %w", namespace.Table, err)
		}
	default:
		return nil, fmt.Errorf("sqlmigration: unsupported dialect %q", namespace.Dialect)
	}
	return columns, nil
}

// ValidateIndexColumns requires an exact, non-unique, non-partial index column
// order.
func ValidateIndexColumns(ctx context.Context, db *sqlx.DB, namespace Namespace, name string, want []string) error {
	if err := ValidateIdentifier(name); err != nil {
		return err
	}
	var rows *sql.Rows
	var err error
	switch namespace.Dialect {
	case SQLite:
		table, quoteErr := Quote(namespace.Dialect, namespace.Table)
		if quoteErr != nil {
			return quoteErr
		}
		metadataRows, metadataErr := db.QueryContext(ctx, "PRAGMA index_list("+table+")")
		if metadataErr != nil {
			return fmt.Errorf("sqlmigration: inspect sqlite index %q metadata: %w", name, metadataErr)
		}
		found := false
		for metadataRows.Next() {
			var sequence, unique, partial int
			var indexName, origin string
			if err := metadataRows.Scan(&sequence, &indexName, &unique, &origin, &partial); err != nil {
				metadataRows.Close()
				return fmt.Errorf("sqlmigration: scan sqlite index %q metadata: %w", name, err)
			}
			if strings.EqualFold(indexName, name) {
				found = true
				if unique != 0 || partial != 0 || origin != "c" {
					metadataRows.Close()
					return fmt.Errorf("index %q must be a non-unique, non-partial created index", name)
				}
			}
		}
		if err := metadataRows.Err(); err != nil {
			metadataRows.Close()
			return fmt.Errorf("sqlmigration: inspect sqlite index %q metadata rows: %w", name, err)
		}
		if err := metadataRows.Close(); err != nil {
			return fmt.Errorf("sqlmigration: close sqlite index %q metadata rows: %w", name, err)
		}
		if !found {
			return fmt.Errorf("index %q is missing", name)
		}
		quoted, quoteErr := Quote(namespace.Dialect, name)
		if quoteErr != nil {
			return quoteErr
		}
		rows, err = db.QueryContext(ctx, "PRAGMA index_info("+quoted+")")
	case PostgreSQL:
		rows, err = db.QueryContext(ctx, `
			SELECT attribute.attname, index_metadata.indisunique, index_metadata.indpred IS NOT NULL
			FROM pg_class data_table
			JOIN pg_namespace namespace ON namespace.oid = data_table.relnamespace
			JOIN pg_index index_metadata ON index_metadata.indrelid = data_table.oid
			JOIN pg_class index_table ON index_table.oid = index_metadata.indexrelid
			CROSS JOIN LATERAL unnest(index_metadata.indkey) WITH ORDINALITY AS key(attnum, position)
			JOIN pg_attribute attribute
			  ON attribute.attrelid = data_table.oid AND attribute.attnum = key.attnum
			WHERE namespace.nspname = current_schema()
			  AND data_table.relname = $1
			  AND index_table.relname = $2
			ORDER BY key.position`, namespace.Table, name)
	default:
		return fmt.Errorf("sqlmigration: unsupported dialect %q", namespace.Dialect)
	}
	if err != nil {
		return fmt.Errorf("sqlmigration: inspect index %q columns: %w", name, err)
	}
	defer rows.Close()
	columns := []string{}
	unique, partial := false, false
	for rows.Next() {
		var column string
		if namespace.Dialect == SQLite {
			var sequence, cid int
			if err := rows.Scan(&sequence, &cid, &column); err != nil {
				return fmt.Errorf("sqlmigration: scan sqlite index %q columns: %w", name, err)
			}
		} else if err := rows.Scan(&column, &unique, &partial); err != nil {
			return fmt.Errorf("sqlmigration: scan postgres index %q columns: %w", name, err)
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlmigration: inspect index %q column rows: %w", name, err)
	}
	if !slices.Equal(columns, want) {
		return fmt.Errorf("index %q columns are %v, want %v", name, columns, want)
	}
	if namespace.Dialect == PostgreSQL && (unique || partial) {
		return fmt.Errorf("index %q must be non-unique and non-partial", name)
	}
	return nil
}

// ValidateColumns requires the exact named column contract.
func ValidateColumns(got, want map[string]Column) error {
	if len(got) != len(want) {
		return fmt.Errorf("column count is %d, want %d", len(got), len(want))
	}
	for name, expected := range want {
		actual, exists := got[name]
		if !exists {
			return fmt.Errorf("column %q is missing", name)
		}
		if actual != expected {
			return fmt.Errorf("column %q is %+v, want %+v", name, actual, expected)
		}
	}
	return nil
}

// ValidateSequenceIdentity requires the dialect-specific generated sequence
// contract used to break equal-timestamp Metrics ties.
func ValidateSequenceIdentity(ctx context.Context, db *sqlx.DB, namespace Namespace, column string) error {
	if err := ValidateIdentifier(column); err != nil {
		return err
	}
	switch namespace.Dialect {
	case SQLite:
		var definition string
		if err := db.QueryRowContext(ctx,
			`SELECT sql FROM sqlite_master WHERE type = 'table' AND lower(name) = lower(?)`,
			namespace.Table,
		).Scan(&definition); err != nil {
			return fmt.Errorf("sqlmigration: inspect sqlite table %q identity: %w", namespace.Table, err)
		}
		pattern := `(?i)(?:^|[,(]\s*)"?` + regexp.QuoteMeta(column) + `"?\s+INTEGER\s+PRIMARY\s+KEY\s+AUTOINCREMENT(?:\s|,|\))`
		if !regexp.MustCompile(pattern).MatchString(definition) {
			return fmt.Errorf("column %q is not INTEGER PRIMARY KEY AUTOINCREMENT", column)
		}
	case PostgreSQL:
		var generation sql.NullString
		err := db.QueryRowContext(ctx, `
			SELECT identity_generation
			FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2`,
			namespace.Table, column,
		).Scan(&generation)
		if err != nil {
			return fmt.Errorf("sqlmigration: inspect postgres table %q identity: %w", namespace.Table, err)
		}
		if !generation.Valid || generation.String != "ALWAYS" {
			return fmt.Errorf("column %q identity generation is not ALWAYS", column)
		}
	default:
		return fmt.Errorf("sqlmigration: unsupported dialect %q", namespace.Dialect)
	}
	return nil
}
