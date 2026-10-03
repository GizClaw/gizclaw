// Package peerusage owns retained hourly model consumption for GizClaw Peers.
package peerusage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/jmoiron/sqlx"
)

const (
	// Retention is the retained hourly consumption window.
	Retention = 90 * 24 * time.Hour
	TableName = "peer_model_usage_hourly"
)

// Snapshot is one writer epoch's cumulative hourly quantity. Repeating the
// same snapshot is idempotent, including when a commit response was lost.
type Snapshot struct {
	Peer     giznet.PublicKey
	ModelID  string
	Hour     time.Time
	WriterID string
	Quantity int64
}

// HourlyUsage is the sum of writer epochs for one Peer/model/hour bucket.
type HourlyUsage struct {
	Peer     giznet.PublicKey
	ModelID  string
	Hour     time.Time
	Quantity int64
}

// Store borrows a SQLite/PostgreSQL pool. It never closes that pool.
type Store struct {
	db    *sqlx.DB
	table storage.SQLTable
	Now   func() time.Time
}

// NewStore initializes and validates the owned SQL schema before returning.
func NewStore(ctx context.Context, db *sqlx.DB) (*Store, error) {
	table, err := storage.PrepareSQLTable(db, "sql", TableName)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, table: table}
	partition := ""
	if table.Dialect() == storage.SQLDialectPostgreSQL {
		partition = " PARTITION BY RANGE (hour_unix_nano)"
	}
	ddl := "CREATE TABLE IF NOT EXISTS " + table.Quoted() + " (peer_public_key TEXT NOT NULL, model_id TEXT NOT NULL, hour_unix_nano BIGINT NOT NULL, writer_id TEXT NOT NULL, quantity BIGINT NOT NULL CHECK(quantity >= 0), PRIMARY KEY(peer_public_key, model_id, hour_unix_nano, writer_id))" + partition
	indexName, err := storage.SQLIndexName(table, "peer_hour_idx")
	if err != nil {
		return nil, err
	}
	quotedIndex, err := storage.QuoteSQLIdentifier(table.Dialect(), indexName)
	if err != nil {
		return nil, err
	}
	indexDDL := "CREATE INDEX IF NOT EXISTS " + quotedIndex + " ON " + table.Quoted() + " (peer_public_key, hour_unix_nano, model_id)"
	if err := storage.EnsureSQLTable(ctx, db, table, ddl, indexDDL); err != nil {
		return nil, err
	}
	columns, err := storage.SQLColumns(ctx, db, table)
	if err != nil {
		return nil, err
	}
	want := map[string]storage.SQLColumn{
		"peer_public_key": {Type: "TEXT", Nullable: false, PrimaryKeyPosition: 1},
		"model_id":        {Type: "TEXT", Nullable: false, PrimaryKeyPosition: 2},
		"hour_unix_nano":  {Type: "BIGINT", Nullable: false, PrimaryKeyPosition: 3},
		"writer_id":       {Type: "TEXT", Nullable: false, PrimaryKeyPosition: 4},
		"quantity":        {Type: "BIGINT", Nullable: false},
	}
	if err := storage.ValidateSQLColumns(columns, want); err != nil {
		return nil, fmt.Errorf("peerusage: incompatible hourly schema: %w", err)
	}
	if err := storage.ValidateSQLIndexColumns(ctx, db, table, indexName, []string{"peer_public_key", "hour_unix_nano", "model_id"}); err != nil {
		return nil, err
	}
	if table.Dialect() == storage.SQLDialectPostgreSQL {
		if err := s.partitions().Check(ctx, db); err != nil {
			return nil, err
		}
	}
	if err := s.Maintain(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s *Store) cutoff() time.Time { return s.now().Truncate(time.Hour).Add(-Retention) }
func (s *Store) partitions() storage.SQLDailyPartitions {
	return storage.SQLDailyPartitions{Table: s.table, Column: "hour_unix_nano", Prefix: TableName}
}

// Maintain reclaims expired rows/partitions even when no new usage arrives.
func (s *Store) Maintain(ctx context.Context) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return storage.ExternalSQLError("peerusage: begin maintenance", err)
	}
	defer tx.Rollback()
	if err := s.maintain(ctx, tx, []time.Time{s.now()}); err != nil {
		return err
	}
	return storage.ExternalSQLError("peerusage: commit maintenance", tx.Commit())
}

func (s *Store) maintain(ctx context.Context, tx *sqlx.Tx, days []time.Time) error {
	if s.table.Dialect() == storage.SQLDialectPostgreSQL {
		return s.partitions().Maintain(ctx, tx, s.cutoff(), days, nil)
	}
	cutoff, err := storage.SQLUnixNano(s.cutoff())
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "DELETE FROM "+s.table.Quoted()+" WHERE hour_unix_nano < ?", cutoff)
	return storage.ExternalSQLError("peerusage: delete expired hours", err)
}

// Write stores cumulative snapshots atomically. A smaller or repeated snapshot
// never subtracts or duplicates usage. Independent writer epochs remain additive.
func (s *Store) Write(ctx context.Context, snapshots []Snapshot) error {
	if len(snapshots) == 0 {
		return nil
	}
	days := make([]time.Time, 0, len(snapshots))
	hours := make([]int64, len(snapshots))
	for i, item := range snapshots {
		if item.Peer.IsZero() || strings.TrimSpace(item.ModelID) == "" || len(item.ModelID) > 256 || strings.TrimSpace(item.WriterID) == "" || len(item.WriterID) > 128 || item.Quantity < 0 {
			return fmt.Errorf("peerusage: invalid hourly snapshot")
		}
		if !item.Hour.Equal(item.Hour.UTC().Truncate(time.Hour)) {
			return fmt.Errorf("peerusage: snapshot hour must be an exact UTC hour")
		}
		if item.Hour.Before(s.cutoff()) {
			return fmt.Errorf("peerusage: snapshot is outside retention")
		}
		if item.Hour.After(s.now().Truncate(time.Hour)) {
			return fmt.Errorf("peerusage: snapshot hour is in the future")
		}
		value, err := storage.SQLUnixNano(item.Hour)
		if err != nil {
			return err
		}
		hours[i] = value
		days = append(days, item.Hour)
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return storage.ExternalSQLError("peerusage: begin hourly write", err)
	}
	defer tx.Rollback()
	if err := s.maintain(ctx, tx, days); err != nil {
		return err
	}
	query := s.db.Rebind("INSERT INTO " + s.table.Quoted() + " (peer_public_key, model_id, hour_unix_nano, writer_id, quantity) VALUES (?, ?, ?, ?, ?) ON CONFLICT(peer_public_key, model_id, hour_unix_nano, writer_id) DO UPDATE SET quantity=excluded.quantity WHERE " + s.table.Quoted() + ".quantity < excluded.quantity")
	for i, item := range snapshots {
		if _, err := tx.ExecContext(ctx, query, item.Peer.String(), item.ModelID, hours[i], item.WriterID, item.Quantity); err != nil {
			return storage.ExternalSQLError("peerusage: write hourly snapshot", err)
		}
	}
	return storage.ExternalSQLError("peerusage: commit hourly snapshots", tx.Commit())
}

// Query returns persisted hourly quantities for a Peer and [start,end). Empty
// modelID selects all models; expiration is enforced independently of cleanup.
func (s *Store) Query(ctx context.Context, peer giznet.PublicKey, modelID string, start, end time.Time) ([]HourlyUsage, error) {
	if peer.IsZero() || !start.Before(end) {
		return nil, fmt.Errorf("peerusage: invalid hourly query")
	}
	start = maxTime(start, s.cutoff())
	if !start.Before(end) {
		return nil, nil
	}
	lower, err := storage.SQLUnixNano(start)
	if err != nil {
		return nil, err
	}
	upper, err := storage.SQLUnixNano(end)
	if err != nil {
		return nil, err
	}
	query := "SELECT model_id, hour_unix_nano, SUM(quantity) FROM " + s.table.Quoted() + " WHERE peer_public_key=? AND hour_unix_nano>=? AND hour_unix_nano<?"
	args := []any{peer.String(), lower, upper}
	if modelID != "" {
		query += " AND model_id=?"
		args = append(args, modelID)
	}
	query += " GROUP BY model_id, hour_unix_nano ORDER BY hour_unix_nano, model_id"
	rows, err := s.db.QueryContext(ctx, s.db.Rebind(query), args...)
	if err != nil {
		return nil, storage.ExternalSQLError("peerusage: query hourly usage", err)
	}
	defer rows.Close()
	var out []HourlyUsage
	for rows.Next() {
		var item HourlyUsage
		var hour int64
		item.Peer = peer
		if err := rows.Scan(&item.ModelID, &hour, &item.Quantity); err != nil {
			return nil, storage.ExternalSQLError("peerusage: read hourly usage", err)
		}
		item.Hour = time.Unix(0, hour).UTC()
		out = append(out, item)
	}
	return out, storage.ExternalSQLError("peerusage: read hourly rows", rows.Err())
}

func maxTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return b
	}
	return a
}
