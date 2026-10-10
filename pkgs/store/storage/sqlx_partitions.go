package storage

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// SQLDailyPartition is a managed UTC day with nanosecond range bounds.
type SQLDailyPartition struct {
	Name         string
	Day          time.Time
	Lower, Upper int64
}

// SQLDailyPartitions delegates UTC day partition creation and retention to
// pg_partman. The extension must be installed in the partman schema.
type SQLDailyPartitions struct {
	Table  SQLTable
	Column string
	Prefix string
	// Retention is the age of the partition key. Zero means the key is already
	// an expiration timestamp and must not receive another retention window.
	Retention time.Duration
}

type sqlPartitionQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (p SQLDailyPartitions) validate() error {
	if err := p.Table.validate(); err != nil {
		return err
	}
	if p.Table.Dialect() != SQLDialectPostgreSQL {
		return fmt.Errorf("storage: daily partitions require PostgreSQL")
	}
	if p.Retention < 0 {
		return fmt.Errorf("storage: partition retention must not be negative")
	}
	if err := ValidateSQLIdentifier(p.Column); err != nil {
		return err
	}
	if len(p.Prefix) > 53 {
		return fmt.Errorf("storage: daily partition prefix exceeds 53 bytes")
	}
	return ValidateSQLIdentifier(p.Prefix)
}

// Partition returns the managed child for the UTC day containing at.
func (p SQLDailyPartitions) Partition(at time.Time) (SQLDailyPartition, error) {
	if err := p.validate(); err != nil {
		return SQLDailyPartition{}, err
	}
	at = at.UTC()
	day := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
	lower, err := SQLUnixNano(day)
	if err != nil {
		return SQLDailyPartition{}, err
	}
	upper, err := SQLUnixNano(day.AddDate(0, 0, 1))
	if err != nil {
		return SQLDailyPartition{}, err
	}
	return SQLDailyPartition{Name: p.Prefix + "_p" + day.Format("20060102"), Day: day, Lower: lower, Upper: upper}, nil
}

// Check verifies the parent partition key and every attached managed child.
func (p SQLDailyPartitions) Check(ctx context.Context, db *sqlx.DB) error {
	if err := p.validate(); err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, `SELECT partitioned.partstrat, attribute.attname
	 FROM pg_partitioned_table partitioned JOIN pg_class parent ON parent.oid=partitioned.partrelid
	 JOIN pg_namespace backend ON backend.oid=parent.relnamespace
	 CROSS JOIN LATERAL unnest(partitioned.partattrs) WITH ORDINALITY AS key(attnum, position)
	 JOIN pg_attribute attribute ON attribute.attrelid=parent.oid AND attribute.attnum=key.attnum
	 WHERE backend.nspname=current_schema() AND parent.relname=$1 ORDER BY key.position`, p.Table.Name())
	if err != nil {
		return ExternalSQLError("storage: inspect daily partition key", err)
	}
	defer rows.Close()
	var strategies, columns []string
	for rows.Next() {
		var strategy, column string
		if err := rows.Scan(&strategy, &column); err != nil {
			return ExternalSQLError("storage: read daily partition key", err)
		}
		strategies = append(strategies, strategy)
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return ExternalSQLError("storage: read daily partition key", err)
	}
	if len(strategies) != 1 || strategies[0] != "r" || columns[0] != p.Column {
		return fmt.Errorf("storage: incompatible daily partition key")
	}
	if err := rows.Close(); err != nil {
		return ExternalSQLError("storage: close daily partition key", err)
	}
	if _, err = p.List(ctx, db); err != nil {
		return err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return ExternalSQLError("storage: begin pg_partman registration", err)
	}
	defer tx.Rollback()
	if err := p.register(ctx, tx); err != nil {
		return err
	}
	return ExternalSQLError("storage: commit pg_partman registration", tx.Commit())
}

// List verifies attached child names and exact bounds before returning them.
func (p SQLDailyPartitions) List(ctx context.Context, q sqlPartitionQueryer) ([]SQLDailyPartition, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, `SELECT child.relname, pg_get_expr(child.relpartbound, child.oid)
	 FROM pg_inherits inheritance JOIN pg_class parent ON parent.oid=inheritance.inhparent
	 JOIN pg_namespace backend ON backend.oid=parent.relnamespace JOIN pg_class child ON child.oid=inheritance.inhrelid
	 WHERE backend.nspname=current_schema() AND parent.relname=$1 ORDER BY child.relname`, p.Table.Name())
	if err != nil {
		return nil, ExternalSQLError("storage: list daily partitions", err)
	}
	defer rows.Close()
	var out []SQLDailyPartition
	for rows.Next() {
		var name, bound string
		if err := rows.Scan(&name, &bound); err != nil {
			return nil, ExternalSQLError("storage: read daily partition", err)
		}
		normalized := strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(strings.ReplaceAll(bound, "::bigint", ""), "'", "")), " "))
		matches := dailyPartitionBounds.FindStringSubmatch(normalized)
		if matches == nil {
			return nil, fmt.Errorf("storage: incompatible daily partition bounds for %q", name)
		}
		lower, err := strconv.ParseInt(matches[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("storage: invalid partition lower bound")
		}
		upper, err := strconv.ParseInt(matches[2], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("storage: invalid partition upper bound")
		}
		day := time.Unix(0, lower).UTC()
		if !day.Equal(day.Truncate(24*time.Hour)) || upper-lower != int64(24*time.Hour) {
			return nil, fmt.Errorf("storage: non-UTC-day partition %q", name)
		}
		out = append(out, SQLDailyPartition{Name: name, Day: day, Lower: lower, Upper: upper})
	}
	return out, ExternalSQLError("storage: list daily partition rows", rows.Err())
}

// Maintain runs under the parent's transaction advisory lock. Children wholly
// before cutoff are dropped; beforeDrop can clean caller-owned auxiliary rows
// in the same transaction. Required days and each following day are prepared.
func (p SQLDailyPartitions) Maintain(ctx context.Context, tx *sqlx.Tx, cutoff time.Time, required []time.Time, beforeDrop func(SQLDailyPartition) error) error {
	if err := p.validate(); err != nil {
		return err
	}
	if err := LockPostgreSQLTable(ctx, tx, p.Table); err != nil {
		return err
	}
	// Match pg_partman's maintenance order before acquiring the parent DDL
	// lock. Both its background worker and our callers can reclaim children.
	for _, operation := range []string{"run_maintenance", "drop_partition_time"} {
		if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", "pg_partman "+operation); err != nil {
			return ExternalSQLError("storage: await pg_partman maintenance", err)
		}
	}
	// Ordinary writers take a compatible parent lock before identities. Take
	// the DDL lock first and retain it through auxiliary cleanup and commit.
	if _, err := tx.ExecContext(ctx, "LOCK TABLE ONLY "+p.Table.Quoted()+" IN ACCESS EXCLUSIVE MODE"); err != nil {
		return ExternalSQLError("storage: protect pg_partman retention", err)
	}
	children, err := p.List(ctx, tx)
	if err != nil {
		return err
	}
	bound, err := SQLUnixNano(cutoff)
	if err != nil {
		return err
	}
	for _, child := range children {
		if child.Upper > bound {
			continue
		}
		if beforeDrop != nil {
			if err := beforeDrop(child); err != nil {
				return err
			}
		}
	}
	parent, err := p.parent(ctx, tx)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "SELECT partman.drop_partition_time($1, p_retention := interval '0 seconds', p_keep_table := false, p_reference_timestamp := $2)", parent, cutoff.UTC().Add(time.Microsecond)); err != nil {
		return ExternalSQLError("storage: pg_partman retention", err)
	}
	return p.Prepare(ctx, tx, required)
}

// Prepare creates only missing required days and their following days through
// pg_partman. Call it outside a write transaction that has reserved identities;
// ordinary writes to existing partitions do not call this DDL path.
func (p SQLDailyPartitions) Prepare(ctx context.Context, tx *sqlx.Tx, required []time.Time) error {
	if err := p.validate(); err != nil {
		return err
	}
	if err := LockPostgreSQLTable(ctx, tx, p.Table); err != nil {
		return err
	}
	parent, err := p.parent(ctx, tx)
	if err != nil {
		return err
	}
	children, err := p.List(ctx, tx)
	if err != nil {
		return err
	}
	seen := make(map[string]bool)
	for _, child := range children {
		seen[child.Day.Format("20060102")] = true
	}
	for _, at := range required {
		for _, day := range []time.Time{at, at.AddDate(0, 0, 1)} {
			child, err := p.Partition(day)
			if err != nil {
				return err
			}
			key := child.Day.Format("20060102")
			if seen[key] {
				continue
			}
			seen[key] = true
			if _, err := tx.ExecContext(ctx, "SELECT partman.create_partition_time($1, ARRAY[$2]::timestamptz[])", parent, child.Day); err != nil {
				return ExternalSQLError("storage: pg_partman create day partition", err)
			}
		}
	}
	_, err = p.List(ctx, tx)
	return err
}

var dailyPartitionBounds = regexp.MustCompile(`^for values from \((-?[0-9]+)\) to \((-?[0-9]+)\)$`)

func (p SQLDailyPartitions) parent(ctx context.Context, tx *sqlx.Tx) (string, error) {
	var schema string
	if err := tx.GetContext(ctx, &schema, "SELECT current_schema()"); err != nil {
		return "", ExternalSQLError("storage: resolve partition schema", err)
	}
	return schema + "." + p.Table.Name(), nil
}

func (p SQLDailyPartitions) register(ctx context.Context, tx *sqlx.Tx) error {
	if err := LockPostgreSQLTable(ctx, tx, p.Table); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "SET LOCAL TIME ZONE 'UTC'"); err != nil {
		return ExternalSQLError("storage: set partition timezone", err)
	}
	var installed bool
	if err := tx.GetContext(ctx, &installed, "SELECT EXISTS(SELECT 1 FROM pg_extension e JOIN pg_namespace n ON n.oid=e.extnamespace WHERE e.extname='pg_partman' AND n.nspname='partman')"); err != nil {
		return ExternalSQLError("storage: inspect pg_partman", err)
	}
	if !installed {
		return fmt.Errorf("storage: PostgreSQL retention requires pg_partman in schema partman")
	}
	parent, err := p.parent(ctx, tx)
	if err != nil {
		return err
	}
	var exists bool
	if err := tx.GetContext(ctx, &exists, "SELECT EXISTS(SELECT 1 FROM partman.part_config WHERE parent_table=$1)", parent); err != nil {
		return ExternalSQLError("storage: inspect partition registration", err)
	}
	if !exists {
		children, err := p.List(ctx, tx)
		if err != nil {
			return err
		}
		start := time.Now().UTC().Truncate(24 * time.Hour)
		for _, child := range children {
			if child.Day.Before(start) {
				start = child.Day
			}
			var name string
			if err := tx.GetContext(ctx, &name, "SELECT partman.check_name_length($1, p_suffix := $2, p_table_partition := true)", p.Table.Name(), child.Day.Format("20060102")); err != nil {
				return ExternalSQLError("storage: derive pg_partman child name", err)
			}
			if name != child.Name {
				quoted, err := QuoteSQLIdentifier(SQLDialectPostgreSQL, child.Name)
				if err != nil {
					return err
				}
				newName, err := QuoteSQLIdentifier(SQLDialectPostgreSQL, name)
				if err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, "ALTER TABLE "+quoted+" RENAME TO "+newName); err != nil {
					return ExternalSQLError("storage: adopt existing partition name", err)
				}
			}
		}
		if _, err := tx.ExecContext(ctx, "SELECT partman.create_parent($1, $2, '1 day', p_epoch := 'nanoseconds', p_premake := 2, p_start_partition := $3, p_default_table := false, p_jobmon := false)", parent, p.Column, start.Format(time.RFC3339)); err != nil {
			return ExternalSQLError("storage: register pg_partman partition parent", err)
		}
	}
	result, err := tx.ExecContext(ctx, "UPDATE partman.part_config SET retention=$2, retention_keep_table=false, infinite_time_partitions=true, inherit_privileges=true WHERE parent_table=$1 AND control=$3 AND epoch='nanoseconds' AND partition_interval='1 day'", parent, fmt.Sprintf("%f seconds", p.Retention.Seconds()), p.Column)
	if err != nil {
		return ExternalSQLError("storage: configure pg_partman retention", err)
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return fmt.Errorf("storage: incompatible pg_partman partition registration")
	}
	return nil
}
