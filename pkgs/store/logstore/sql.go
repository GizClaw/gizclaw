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

	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/jmoiron/sqlx"
)

const sqlLogInitializationTimeout = 30 * time.Second

type preparedSQLRecord struct {
	record     Record
	attributes string
}

// SQLStore implements MutableStore over one logical SQLite table. The supplied
// pool is borrowed and is never closed by this adapter. PostgreSQL LogStore is
// unsupported; conversation history uses memory.HistoryStore.
type SQLStore struct {
	db     *sqlx.DB
	table  storage.SQLTable
	quoted string
	ttl    time.Duration

	mu     sync.RWMutex
	closed bool
}

// NewSQLStoreWithDB constructs a table-scoped mutable SQL Log Store.
func NewSQLStoreWithDB(db *sqlx.DB, table string) (*SQLStore, error) {
	return NewSQLStoreWithDBAndTTL(db, table, 0)
}

// NewSQLStoreWithDBAndTTL constructs a table-scoped mutable SQL Log Store
// whose driver owns record expiration.
func NewSQLStoreWithDBAndTTL(db *sqlx.DB, table string, ttl time.Duration) (*SQLStore, error) {
	if ttl < 0 {
		return nil, errors.New("logstore: sql ttl must be positive")
	}
	ctx, cancel := context.WithTimeout(context.Background(), sqlLogInitializationTimeout)
	defer cancel()
	return newSQLStoreWithDB(ctx, db, table, ttl)
}

func newSQLStoreWithDB(ctx context.Context, db *sqlx.DB, table string, ttl time.Duration) (*SQLStore, error) {
	sqlTable, err := storage.PrepareSQLTable(db, "log", table)
	if err != nil {
		return nil, err
	}
	if sqlTable.Dialect() != storage.SQLDialectSQLite {
		return nil, errors.New("logstore: PostgreSQL backend is unsupported; use memory.HistoryStore for conversation history")
	}
	if err := ensureSQLSchema(ctx, db, sqlTable); err != nil {
		return nil, err
	}
	store := &SQLStore{db: db, table: sqlTable, quoted: sqlTable.Quoted(), ttl: ttl}
	if err := store.checkSchema(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

func (store *SQLStore) checkSchema(ctx context.Context) error {
	columns, err := storage.SQLColumns(ctx, store.db, store.table)
	if err != nil {
		return err
	}
	want := map[string]storage.SQLColumn{
		"stream": {Type: "TEXT", PrimaryKeyPosition: 1}, "id": {Type: "TEXT", PrimaryKeyPosition: 2},
		"timestamp_unix_nano": {Type: "BIGINT"}, "expires_at_unix_nano": {Type: "BIGINT", Nullable: true},
		"kind": {Type: "TEXT"}, "severity": {Type: "TEXT"}, "message": {Type: "TEXT"}, "attributes_json": {Type: "TEXT"}, "payload_json": {Type: "BLOB"},
	}
	if err := storage.ValidateSQLColumns(columns, want); err != nil {
		return err
	}
	for _, index := range sqlLogIndexes {
		name, err := storage.SQLIndexName(store.table, index.purpose)
		if err != nil {
			return err
		}
		if err := storage.ValidateSQLIndexColumns(ctx, store.db, store.table, name, index.want); err != nil {
			return fmt.Errorf("logstore: incompatible index: %w", err)
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
	prepared := make([]preparedSQLRecord, 0, len(records))
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
		if _, err := storage.SQLUnixNano(record.Time); err != nil {
			return nil, fmt.Errorf("logstore: record time: %w", err)
		}
		prepared = append(prepared, preparedSQLRecord{record: cloneRecord(record), attributes: string(encoded)})
	}
	if _, err := store.appendSQLRecords(ctx, prepared); err != nil {
		return nil, err
	}
	keys := make([]RecordKey, len(records))
	for index, record := range records {
		keys[index] = record.Key()
	}
	return keys, nil
}

func (store *SQLStore) appendSQLRecords(ctx context.Context, prepared []preparedSQLRecord) (time.Time, error) {
	tx, err := store.beginTransaction(ctx)
	if err != nil {
		return time.Time{}, err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	expiration := now.Add(store.ttl)
	var expiresAt any
	if store.ttl > 0 {
		expiresAt, err = storage.SQLUnixNano(expiration)
		if err != nil {
			return expiration, fmt.Errorf("logstore: record expiration: %w", err)
		}
	}
	if store.ttl > 0 {
		cleanup := store.db.Rebind("DELETE FROM " + store.quoted + " WHERE expires_at_unix_nano IS NOT NULL AND expires_at_unix_nano <= ?")
		if _, err := tx.ExecContext(ctx, cleanup, now.UnixNano()); err != nil {
			return expiration, storage.ExternalSQLError("logstore: delete expired sql records", err)
		}
	}
	query := store.db.Rebind("INSERT INTO " + store.quoted + " (stream, id, timestamp_unix_nano, expires_at_unix_nano, kind, severity, message, attributes_json, payload_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)")
	statement, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return expiration, storage.ExternalSQLError("logstore: prepare sql append", err)
	}
	defer statement.Close()
	for _, item := range prepared {
		record := item.record
		payload := append([]byte{}, record.Payload...)
		timestamp, _ := storage.SQLUnixNano(record.Time)
		if _, err := statement.ExecContext(ctx, record.Stream, record.ID, timestamp, expiresAt, record.Kind, record.Severity, record.Message, item.attributes, payload); err != nil {
			return expiration, storage.ExternalSQLError("logstore: append sql record", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return expiration, storage.ExternalSQLError("logstore: commit sql append", err)
	}
	return expiration, nil
}

// Query returns one stable time, stream, and ID ordered page.
func (store *SQLStore) Query(ctx context.Context, query Query) (Page, error) {
	if err := ValidateQuery(query); err != nil {
		return Page{}, err
	}
	if _, err := storage.SQLUnixNano(query.Start); err != nil {
		return Page{}, fmt.Errorf("logstore: query start: %w", err)
	}
	if _, err := storage.SQLUnixNano(query.End); err != nil {
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
	statement, args, err := store.buildQuery(bound, position, time.Now().UTC().UnixNano(), query.Limit+1)
	if err != nil {
		return Page{}, err
	}
	rows, err := store.db.QueryContext(ctx, store.db.Rebind(statement), args...)
	if err != nil {
		return Page{}, storage.ExternalSQLError("logstore: query sql records", err)
	}
	defer rows.Close()
	records := make([]Record, 0, query.Limit+1)
	for rows.Next() {
		var record Record
		var timestamp int64
		var attributesJSON string
		var payload []byte
		if err := rows.Scan(&record.Stream, &record.ID, &timestamp, &record.Kind, &record.Severity, &record.Message, &attributesJSON, &payload); err != nil {
			return Page{}, storage.ExternalSQLError("logstore: scan sql record", err)
		}
		record.Time = time.Unix(0, timestamp).UTC()
		record.Attributes = map[string]string{}
		if err := json.Unmarshal([]byte(attributesJSON), &record.Attributes); err != nil {
			return Page{}, fmt.Errorf("logstore: decode sql attributes: %w", err)
		}
		record.Payload = append(json.RawMessage(nil), payload...)
		if err := ValidateRecord(record); err != nil {
			return Page{}, fmt.Errorf("logstore: invalid sql record: %w", err)
		}
		records = append(records, record)
		if len(records) > query.Limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return Page{}, storage.ExternalSQLError("logstore: query sql rows", err)
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

// Get returns one exact record key.
func (store *SQLStore) Get(ctx context.Context, key RecordKey) (Record, error) {
	if err := ValidateRecordKey(key); err != nil {
		return Record{}, err
	}
	unlock, err := store.lock()
	if err != nil {
		return Record{}, err
	}
	defer unlock()
	statement := store.db.Rebind("SELECT stream, id, timestamp_unix_nano, kind, severity, message, attributes_json, payload_json FROM " + store.quoted + " WHERE stream = ? AND id = ? AND (expires_at_unix_nano IS NULL OR expires_at_unix_nano > ?)")
	var record Record
	var timestamp int64
	var attributesJSON string
	var payload []byte
	err = store.db.QueryRowContext(ctx, statement, key.Stream, key.ID, time.Now().UTC().UnixNano()).Scan(
		&record.Stream,
		&record.ID,
		&timestamp,
		&record.Kind,
		&record.Severity,
		&record.Message,
		&attributesJSON,
		&payload,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, fmt.Errorf("%w: stream %q id %q", ErrNotFound, key.Stream, key.ID)
	}
	if err != nil {
		return Record{}, storage.ExternalSQLError("logstore: get sql record", err)
	}
	record.Time = time.Unix(0, timestamp).UTC()
	record.Attributes = map[string]string{}
	if err := json.Unmarshal([]byte(attributesJSON), &record.Attributes); err != nil {
		return Record{}, fmt.Errorf("logstore: decode sql attributes: %w", err)
	}
	record.Payload = append(json.RawMessage(nil), payload...)
	if err := ValidateRecord(record); err != nil {
		return Record{}, fmt.Errorf("logstore: invalid sql record: %w", err)
	}
	return record, nil
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
	tx, err := store.beginWrite(ctx, record.Key())
	if err != nil {
		return err
	}
	defer tx.Rollback()
	nowUnixNano := time.Now().UTC().UnixNano()
	stored, err := store.recordTime(ctx, tx, record.Key(), nowUnixNano)
	if err != nil {
		return err
	}
	timestamp, err := storage.SQLUnixNano(record.Time)
	if err != nil {
		return fmt.Errorf("logstore: record time: %w", err)
	}
	if stored != timestamp {
		return errors.New("logstore: replace cannot change record time")
	}
	query := store.db.Rebind("UPDATE " + store.quoted + " SET kind = ?, severity = ?, message = ?, attributes_json = ?, payload_json = ? WHERE stream = ? AND id = ? AND timestamp_unix_nano = ? AND (expires_at_unix_nano IS NULL OR expires_at_unix_nano > ?)")
	payload := append([]byte{}, record.Payload...)
	result, err := tx.ExecContext(ctx, query, record.Kind, record.Severity, record.Message, string(encoded), payload, record.Stream, record.ID, stored, nowUnixNano)
	if err != nil {
		return storage.ExternalSQLError("logstore: replace sql record", err)
	}
	if err := requireOneRow(result); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return storage.ExternalSQLError("logstore: commit sql replace", err)
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
	tx, err := store.beginWrite(ctx, key)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	query := store.db.Rebind("DELETE FROM " + store.quoted + " WHERE stream = ? AND id = ? AND (expires_at_unix_nano IS NULL OR expires_at_unix_nano > ?)")
	result, err := tx.ExecContext(ctx, query, key.Stream, key.ID, time.Now().UTC().UnixNano())
	if err != nil {
		return storage.ExternalSQLError("logstore: delete sql record", err)
	}
	if err := requireOneRow(result); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return storage.ExternalSQLError("logstore: commit sql delete", err)
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

// Maintain reclaims expired SQLite records without requiring new writes.
func (store *SQLStore) Maintain(ctx context.Context) error {
	unlock, err := store.lock()
	if err != nil {
		return err
	}
	defer unlock()
	if store.ttl == 0 {
		return ctx.Err()
	}
	_, err = store.db.ExecContext(ctx, store.db.Rebind("DELETE FROM "+store.quoted+" WHERE expires_at_unix_nano <= ?"), time.Now().UTC().UnixNano())
	return storage.ExternalSQLError("logstore: expire sql records", err)
}

func (store *SQLStore) beginWrite(ctx context.Context, _ RecordKey) (*sqlx.Tx, error) {
	return store.beginTransaction(ctx)
}

func (store *SQLStore) beginTransaction(ctx context.Context) (*sqlx.Tx, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tx, err := store.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, storage.ExternalSQLError("logstore: begin sql transaction", err)
	}
	return tx, nil
}

func (store *SQLStore) buildQuery(query sqlBoundQuery, position *sqlPosition, nowUnixNano int64, limit int) (string, []any, error) {
	startNano, err := storage.SQLUnixNano(time.UnixMilli(query.StartMS))
	if err != nil {
		return "", nil, fmt.Errorf("logstore: query start: %w", err)
	}
	endNano, err := storage.SQLUnixNano(time.UnixMilli(query.EndMS))
	if err != nil {
		return "", nil, fmt.Errorf("logstore: query end: %w", err)
	}
	parts := []string{"timestamp_unix_nano >= ?", "timestamp_unix_nano < ?", "(expires_at_unix_nano IS NULL OR expires_at_unix_nano > ?)"}
	args := []any{startNano, endNano, nowUnixNano}
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
	// SQLite filters in SQL before applying the page limit.
	// Iterate the flat JSON object so dotted attribute names remain literal keys.
	attributes := "json_each(attributes_json)"
	if query.Text != "" {
		predicate := "instr(message, ?) > 0"
		parts = append(parts, predicate)
		args = append(args, query.Text)
	}
	for _, matcher := range query.Matchers {
		predicate := "EXISTS (SELECT 1 FROM " + attributes + " AS attr WHERE attr.key = ?"
		args = append(args, matcher.Name)
		switch matcher.Op {
		case MatchEqual:
			predicate += " AND attr.value = ?"
			args = append(args, matcher.Value)
		case MatchNotEqual:
			predicate += " AND attr.value <> ?"
			args = append(args, matcher.Value)
		case MatchNotExists:
			predicate = "NOT " + predicate
		}
		parts = append(parts, predicate+")")
	}
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
	statement += " LIMIT ?"
	args = append(args, limit)
	return statement, args, nil
}

func (store *SQLStore) recordTime(ctx context.Context, tx *sqlx.Tx, key RecordKey, nowUnixNano int64) (int64, error) {
	query := store.db.Rebind("SELECT timestamp_unix_nano FROM " + store.quoted + " WHERE stream = ? AND id = ? AND (expires_at_unix_nano IS NULL OR expires_at_unix_nano > ?)")
	var timestamp int64
	if err := tx.QueryRowContext(ctx, query, key.Stream, key.ID, nowUnixNano).Scan(&timestamp); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, storage.ExternalSQLError("logstore: find sql record", err)
	}
	return timestamp, nil
}

func requireOneRow(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return storage.ExternalSQLError("logstore: inspect sql mutation", err)
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
	_ ImmutableStore     = (*SQLStore)(nil)
	_ MutableStore       = (*SQLStore)(nil)
	_ MutableRecordStore = (*SQLStore)(nil)
)
