package logstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/internal/sqlbackend"
	"github.com/jmoiron/sqlx"
)

const sqlLogInitializationTimeout = 30 * time.Second

// SQLStore implements MutableStore over one SQLite or PostgreSQL table. The
// supplied pool is borrowed and is never closed by this adapter.
type SQLStore struct {
	db      *sqlx.DB
	backend sqlbackend.Backend
	quoted  string

	mu     sync.RWMutex
	closed bool
}

// NewSQLStoreWithDB constructs a table-scoped mutable SQL Log Store.
func NewSQLStoreWithDB(db *sqlx.DB, table string) (*SQLStore, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sqlLogInitializationTimeout)
	defer cancel()
	return newSQLStoreWithDB(ctx, db, table)
}

func newSQLStoreWithDB(ctx context.Context, db *sqlx.DB, table string) (*SQLStore, error) {
	backend, err := sqlbackend.Prepare(db, "log", table)
	if err != nil {
		return nil, fmt.Errorf("logstore: sql table %q: %w", table, err)
	}
	if err := ensureSQLSchema(ctx, db, backend); err != nil {
		return nil, fmt.Errorf("logstore: initialize sql table %q: %w", table, err)
	}
	store := &SQLStore{db: db, backend: backend, quoted: backend.Quoted}
	if err := store.checkSchema(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

func (store *SQLStore) checkSchema(ctx context.Context) error {
	columns, err := sqlbackend.Columns(ctx, store.db, store.backend)
	if err != nil {
		return fmt.Errorf("logstore: inspect sql table %q: %w", store.backend.Table, err)
	}
	payloadType := "BLOB"
	if store.backend.Dialect == sqlbackend.PostgreSQL {
		payloadType = "BYTEA"
	}
	want := map[string]sqlbackend.Column{
		"stream":              {Type: "TEXT", Nullable: false, PrimaryKeyPosition: 1},
		"id":                  {Type: "TEXT", Nullable: false, PrimaryKeyPosition: 2},
		"timestamp_unix_nano": {Type: "BIGINT", Nullable: false},
		"kind":                {Type: "TEXT", Nullable: false},
		"severity":            {Type: "TEXT", Nullable: false},
		"message":             {Type: "TEXT", Nullable: false},
		"attributes_json":     {Type: "TEXT", Nullable: false},
		"payload_json":        {Type: payloadType, Nullable: false},
	}
	if err := sqlbackend.ValidateColumns(columns, want); err != nil {
		return fmt.Errorf("logstore: incompatible sql table %q: %w", store.backend.Table, err)
	}
	for _, index := range sqlLogIndexes {
		name, err := sqlbackend.IndexName(store.backend, index.purpose)
		if err != nil {
			return fmt.Errorf("logstore: derive sql table %q index: %w", store.backend.Table, err)
		}
		if err := sqlbackend.ValidateIndexColumns(ctx, store.db, store.backend, name, index.want); err != nil {
			return fmt.Errorf("logstore: incompatible sql table %q index %q: %w", store.backend.Table, name, err)
		}
	}
	return nil
}

func (store *SQLStore) lock() (func(), error) {
	if store == nil {
		return nil, errors.New("logstore: sql store is nil")
	}
	store.mu.RLock()
	if store.closed || store.db == nil {
		store.mu.RUnlock()
		return nil, errors.New("logstore: sql store is closed")
	}
	return store.mu.RUnlock, nil
}

// Append stores a complete validated batch atomically.
func (store *SQLStore) Append(ctx context.Context, records []Record) ([]RecordKey, error) {
	unlock, err := store.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return []RecordKey{}, nil
	}
	type preparedRecord struct {
		record     Record
		attributes string
	}
	prepared := make([]preparedRecord, 0, len(records))
	seen := make(map[RecordKey]struct{}, len(records))
	for _, record := range records {
		if err := ValidateRecord(record); err != nil {
			return nil, err
		}
		if _, exists := seen[record.Key()]; exists {
			return nil, errors.New("logstore: duplicate record key in append")
		}
		seen[record.Key()] = struct{}{}
		attributes := record.Attributes
		if attributes == nil {
			attributes = map[string]string{}
		}
		encoded, err := json.Marshal(attributes)
		if err != nil {
			return nil, fmt.Errorf("logstore: encode sql attributes: %w", err)
		}
		if _, err := sqlbackend.UnixNano(record.Time); err != nil {
			return nil, fmt.Errorf("logstore: record time: %w", err)
		}
		prepared = append(prepared, preparedRecord{record: cloneRecord(record), attributes: string(encoded)})
	}
	tx, err := store.beginWrite(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	query := sqlbackend.Rebind(store.db, "INSERT INTO "+store.quoted+" (stream, id, timestamp_unix_nano, kind, severity, message, attributes_json, payload_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?)")
	statement, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return nil, sqlbackend.ExternalError("logstore: prepare sql append", err)
	}
	defer statement.Close()
	for _, item := range prepared {
		record := item.record
		payload := append([]byte{}, record.Payload...)
		timestamp, _ := sqlbackend.UnixNano(record.Time)
		if _, err := statement.ExecContext(ctx, record.Stream, record.ID, timestamp, record.Kind, record.Severity, record.Message, item.attributes, payload); err != nil {
			return nil, sqlbackend.ExternalError("logstore: append sql record", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, sqlbackend.ExternalError("logstore: commit sql append", err)
	}
	keys := make([]RecordKey, len(records))
	for index, record := range records {
		keys[index] = record.Key()
	}
	return keys, nil
}

// Query returns one stable time, stream, and ID ordered page.
func (store *SQLStore) Query(ctx context.Context, query Query) (Page, error) {
	if err := ValidateQuery(query); err != nil {
		return Page{}, err
	}
	if _, err := sqlbackend.UnixNano(query.Start); err != nil {
		return Page{}, fmt.Errorf("logstore: query start: %w", err)
	}
	if _, err := sqlbackend.UnixNano(query.End); err != nil {
		return Page{}, fmt.Errorf("logstore: query end: %w", err)
	}
	unlock, err := store.lock()
	if err != nil {
		return Page{}, err
	}
	defer unlock()
	bound := normalizeSQLQuery(query)
	var position *sqlPosition
	if query.Cursor != "" {
		cursor, err := decodeSQLCursor(query.Cursor)
		if err != nil {
			return Page{}, err
		}
		if !equalSQLQuery(cursor.Query, bound) {
			return Page{}, fmt.Errorf("%w: query fields changed", ErrCursorMismatch)
		}
		position = &cursor.Position
	}
	statement, args, err := store.buildQuery(bound, position)
	if err != nil {
		return Page{}, err
	}
	rows, err := store.db.QueryContext(ctx, sqlbackend.Rebind(store.db, statement), args...)
	if err != nil {
		return Page{}, sqlbackend.ExternalError("logstore: query sql records", err)
	}
	defer rows.Close()
	records := make([]Record, 0, query.Limit+1)
	for rows.Next() {
		var record Record
		var timestamp int64
		var attributesJSON string
		var payload []byte
		if err := rows.Scan(&record.Stream, &record.ID, &timestamp, &record.Kind, &record.Severity, &record.Message, &attributesJSON, &payload); err != nil {
			return Page{}, sqlbackend.ExternalError("logstore: scan sql record", err)
		}
		record.Time = time.Unix(0, timestamp).UTC()
		record.Attributes = map[string]string{}
		if err := json.Unmarshal([]byte(attributesJSON), &record.Attributes); err != nil {
			return Page{}, fmt.Errorf("logstore: decode sql attributes: %w", err)
		}
		record.Payload = append(json.RawMessage(nil), payload...)
		if !matchesSQLRecord(record, bound) {
			continue
		}
		if err := ValidateRecord(record); err != nil {
			return Page{}, fmt.Errorf("logstore: invalid sql record: %w", err)
		}
		records = append(records, record)
		if len(records) > query.Limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return Page{}, sqlbackend.ExternalError("logstore: query sql rows", err)
	}
	page := Page{}
	if len(records) > query.Limit {
		page.HasNext = true
		records = records[:query.Limit]
	}
	page.Records = records
	if page.HasNext {
		last := records[len(records)-1]
		cursor, err := encodeSQLCursor(sqlCursor{
			Version: 1,
			Query:   bound,
			Position: sqlPosition{
				TimeUnixNano: last.Time.UnixNano(), Stream: last.Stream, ID: last.ID,
			},
		})
		if err != nil {
			return Page{}, fmt.Errorf("logstore: encode sql cursor: %w", err)
		}
		page.NextCursor = cursor
	}
	if err := ValidatePage(page, query.Limit); err != nil {
		return Page{}, err
	}
	return page, nil
}

// Replace changes mutable fields without changing a record key or time.
func (store *SQLStore) Replace(ctx context.Context, record Record) error {
	if err := ValidateRecord(record); err != nil {
		return err
	}
	unlock, err := store.lock()
	if err != nil {
		return err
	}
	defer unlock()
	attributes := record.Attributes
	if attributes == nil {
		attributes = map[string]string{}
	}
	encoded, err := json.Marshal(attributes)
	if err != nil {
		return fmt.Errorf("logstore: encode sql attributes: %w", err)
	}
	tx, err := store.beginWrite(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stored, err := store.recordTime(ctx, tx, record.Key())
	if err != nil {
		return err
	}
	timestamp, err := sqlbackend.UnixNano(record.Time)
	if err != nil {
		return fmt.Errorf("logstore: record time: %w", err)
	}
	if stored != timestamp {
		return errors.New("logstore: replace cannot change record time")
	}
	query := sqlbackend.Rebind(store.db, "UPDATE "+store.quoted+" SET kind = ?, severity = ?, message = ?, attributes_json = ?, payload_json = ? WHERE stream = ? AND id = ? AND timestamp_unix_nano = ?")
	payload := append([]byte{}, record.Payload...)
	result, err := tx.ExecContext(ctx, query, record.Kind, record.Severity, record.Message, string(encoded), payload, record.Stream, record.ID, stored)
	if err != nil {
		return sqlbackend.ExternalError("logstore: replace sql record", err)
	}
	if err := requireOneRow(result); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return sqlbackend.ExternalError("logstore: commit sql replace", err)
	}
	return nil
}

// Delete removes one existing record.
func (store *SQLStore) Delete(ctx context.Context, key RecordKey) error {
	if err := ValidateRecordKey(key); err != nil {
		return err
	}
	unlock, err := store.lock()
	if err != nil {
		return err
	}
	defer unlock()
	tx, err := store.beginWrite(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	query := sqlbackend.Rebind(store.db, "DELETE FROM "+store.quoted+" WHERE stream = ? AND id = ?")
	result, err := tx.ExecContext(ctx, query, key.Stream, key.ID)
	if err != nil {
		return sqlbackend.ExternalError("logstore: delete sql record", err)
	}
	if err := requireOneRow(result); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return sqlbackend.ExternalError("logstore: commit sql delete", err)
	}
	return nil
}

// Close marks this logical Store closed and leaves the pool open.
func (store *SQLStore) Close() error {
	if store == nil {
		return nil
	}
	store.mu.Lock()
	store.closed = true
	store.mu.Unlock()
	return nil
}

func (store *SQLStore) beginWrite(ctx context.Context) (*sqlx.Tx, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tx, err := store.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, sqlbackend.ExternalError("logstore: begin sql transaction", err)
	}
	if store.backend.Dialect == sqlbackend.PostgreSQL {
		if _, err := tx.ExecContext(ctx, "LOCK TABLE "+store.quoted+" IN SHARE ROW EXCLUSIVE MODE"); err != nil {
			_ = tx.Rollback()
			return nil, sqlbackend.ExternalError("logstore: lock sql table", err)
		}
	}
	return tx, nil
}

func (store *SQLStore) buildQuery(query sqlBoundQuery, position *sqlPosition) (string, []any, error) {
	startNano, err := sqlbackend.UnixNano(time.UnixMilli(query.StartMS))
	if err != nil {
		return "", nil, fmt.Errorf("logstore: query start: %w", err)
	}
	endNano, err := sqlbackend.UnixNano(time.UnixMilli(query.EndMS))
	if err != nil {
		return "", nil, fmt.Errorf("logstore: query end: %w", err)
	}
	parts := []string{"timestamp_unix_nano >= ?", "timestamp_unix_nano < ?"}
	args := []any{startNano, endNano}
	appendSet := func(column string, values []string) {
		if len(values) == 0 {
			return
		}
		parts = append(parts, column+" IN ("+strings.TrimSuffix(strings.Repeat("?,", len(values)), ",")+")")
		for _, value := range values {
			args = append(args, value)
		}
	}
	appendSet("stream", query.Streams)
	appendSet("kind", query.Kinds)
	appendSet("severity", query.Severities)
	if position != nil {
		operator := ">"
		if query.Order == OrderDesc {
			operator = "<"
		}
		parts = append(parts, "(timestamp_unix_nano "+operator+" ? OR (timestamp_unix_nano = ? AND (stream "+operator+" ? OR (stream = ? AND id "+operator+" ?))))")
		args = append(args, position.TimeUnixNano, position.TimeUnixNano, position.Stream, position.Stream, position.ID)
	}
	direction := "ASC"
	if query.Order == OrderDesc {
		direction = "DESC"
	}
	statement := "SELECT stream, id, timestamp_unix_nano, kind, severity, message, attributes_json, payload_json FROM " + store.quoted + " WHERE " + strings.Join(parts, " AND ") + " ORDER BY timestamp_unix_nano " + direction + ", stream " + direction + ", id " + direction
	return statement, args, nil
}

func matchesSQLRecord(record Record, query sqlBoundQuery) bool {
	if query.Text != "" && !strings.Contains(record.Message, query.Text) {
		return false
	}
	for _, matcher := range query.Matchers {
		value, exists := record.Attributes[matcher.Name]
		matched := false
		switch matcher.Op {
		case MatchEqual:
			matched = exists && value == matcher.Value
		case MatchNotEqual:
			matched = exists && value != matcher.Value
		case MatchExists:
			matched = exists
		case MatchNotExists:
			matched = !exists
		}
		if !matched {
			return false
		}
	}
	return true
}

func (store *SQLStore) recordTime(ctx context.Context, tx *sqlx.Tx, key RecordKey) (int64, error) {
	query := sqlbackend.Rebind(store.db, "SELECT timestamp_unix_nano FROM "+store.quoted+" WHERE stream = ? AND id = ?")
	var timestamp int64
	if err := tx.QueryRowContext(ctx, query, key.Stream, key.ID).Scan(&timestamp); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, sqlbackend.ExternalError("logstore: find sql record", err)
	}
	return timestamp, nil
}

func requireOneRow(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return sqlbackend.ExternalError("logstore: inspect sql mutation", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	if count != 1 {
		return fmt.Errorf("logstore: sql mutation affected %d records", count)
	}
	return nil
}

var (
	_ ImmutableStore = (*SQLStore)(nil)
	_ MutableStore   = (*SQLStore)(nil)
)
