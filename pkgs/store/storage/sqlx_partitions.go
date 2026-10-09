package storage

import (
	"context"
	"database/sql"
	"fmt"
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

// SQLDailyPartitions maintains day children of a PostgreSQL range parent.
// Prefix is owned by the caller so existing relation names remain stable.
type SQLDailyPartitions struct {
	Table  SQLTable
	Column string
	Prefix string
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
	_, err = p.List(ctx, db)
	return err
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
		date := strings.TrimPrefix(name, p.Prefix+"_p")
		if len(date) != 8 || date == name {
			return nil, fmt.Errorf("storage: unmanaged daily partition %q", name)
		}
		day, err := time.Parse("20060102", date)
		if err != nil {
			return nil, fmt.Errorf("storage: invalid daily partition name %q", name)
		}
		expected, err := p.Partition(day)
		if err != nil {
			return nil, err
		}
		normalized := strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(strings.ReplaceAll(bound, "::bigint", ""), "'", "")), " "))
		want := fmt.Sprintf("for values from (%d) to (%d)", expected.Lower, expected.Upper)
		if normalized != want {
			return nil, fmt.Errorf("storage: incompatible daily partition bounds for %q", name)
		}
		out = append(out, expected)
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
		// Writers lock the parent before auxiliary identities. Take the parent
		// DDL lock before beforeDrop touches those rows, in the same order.
		if _, err := tx.ExecContext(ctx, "LOCK TABLE ONLY "+p.Table.Quoted()+" IN ACCESS EXCLUSIVE MODE"); err != nil {
			return ExternalSQLError("storage: protect daily partition retention", err)
		}
		if beforeDrop != nil {
			if err := beforeDrop(child); err != nil {
				return err
			}
		}
		name, err := QuoteSQLIdentifier(SQLDialectPostgreSQL, child.Name)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DROP TABLE "+name); err != nil {
			return ExternalSQLError("storage: drop daily partition", err)
		}
	}
	return p.Prepare(ctx, tx, required)
}

// Prepare creates only missing required days and their following days. Call it
// outside a write transaction that has reserved caller-owned identities.
// The advisory lock coordinates DDL, while ordinary writes remain concurrent.
func (p SQLDailyPartitions) Prepare(ctx context.Context, tx *sqlx.Tx, required []time.Time) error {
	if err := p.validate(); err != nil {
		return err
	}
	if err := LockPostgreSQLTable(ctx, tx, p.Table); err != nil {
		return err
	}
	children, err := p.List(ctx, tx)
	if err != nil {
		return err
	}
	seen := make(map[string]bool)
	for _, child := range children {
		seen[child.Name] = true
	}
	for _, at := range required {
		for _, day := range []time.Time{at, at.AddDate(0, 0, 1)} {
			child, err := p.Partition(day)
			if err != nil {
				return err
			}
			if seen[child.Name] {
				continue
			}
			seen[child.Name] = true
			name, err := QuoteSQLIdentifier(SQLDialectPostgreSQL, child.Name)
			if err != nil {
				return err
			}
			statement := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM (%d) TO (%d)", name, p.Table.Quoted(), child.Lower, child.Upper)
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return ExternalSQLError("storage: create daily partition", err)
			}
		}
	}
	_, err = p.List(ctx, tx)
	return err
}
