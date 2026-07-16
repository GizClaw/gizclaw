package logstore

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
)

const (
	clickHouseTimeout     = 30 * time.Second
	clickHouseCursorLimit = 16 * 1024
)

var clickHouseIdentifierRE = regexp.MustCompile("^[A-Za-z_][A-Za-z0-9_]*$")

// ClickHouseConfig configures a mutable ClickHouse log store.
type ClickHouseConfig struct {
	DSN      string
	Database string
	Table    string
}

// ClickHouseStore persists mutable records in one ClickHouse MergeTree table.
type ClickHouseStore struct {
	db        *sql.DB
	database  string
	table     string
	qualified string

	appendMu  sync.Mutex
	closeOnce sync.Once
	closeErr  error
}

// NewClickHouseStore connects to ClickHouse, ensures the record table, and
// validates the schema used by the mutable store.
func NewClickHouseStore(config ClickHouseConfig) (*ClickHouseStore, error) {
	config.DSN = strings.TrimSpace(config.DSN)
	config.Database = strings.TrimSpace(config.Database)
	config.Table = strings.TrimSpace(config.Table)
	if config.DSN == "" {
		return nil, errors.New("logstore: clickhouse dsn is required")
	}
	if config.Database != "" && !clickHouseIdentifierRE.MatchString(config.Database) {
		return nil, fmt.Errorf("logstore: invalid clickhouse database %q", config.Database)
	}
	if !clickHouseIdentifierRE.MatchString(config.Table) {
		return nil, fmt.Errorf("logstore: invalid clickhouse table %q", config.Table)
	}
	db, err := sql.Open("clickhouse", config.DSN)
	if err != nil {
		return nil, fmt.Errorf("logstore: open clickhouse: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), clickHouseTimeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("logstore: ping clickhouse: %w", err)
	}
	database := config.Database
	if database == "" {
		if err := db.QueryRowContext(ctx, "SELECT currentDatabase()").Scan(&database); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("logstore: resolve clickhouse database: %w", err)
		}
	}
	if !clickHouseIdentifierRE.MatchString(database) {
		_ = db.Close()
		return nil, fmt.Errorf("logstore: invalid current clickhouse database %q", database)
	}
	qualified := quoteClickHouseIdentifier(database) + "." + quoteClickHouseIdentifier(config.Table)
	ddl := fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s (id String, timestamp DateTime64(9, 'UTC'), stream String, kind String, severity String, message String, attributes Map(String, String), payload String) ENGINE = MergeTree PARTITION BY toYYYYMM(timestamp) ORDER BY (timestamp, stream, id)",
		qualified,
	)
	if _, err := db.ExecContext(ctx, ddl); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("logstore: create clickhouse table: %w", err)
	}
	store := &ClickHouseStore{
		db:        db,
		database:  database,
		table:     config.Table,
		qualified: qualified,
	}
	if err := store.checkSchema(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func quoteClickHouseIdentifier(value string) string {
	return "\"" + value + "\""
}

func (store *ClickHouseStore) checkSchema(ctx context.Context) error {
	rows, err := store.db.QueryContext(
		ctx,
		"SELECT name, type FROM system.columns WHERE database = ? AND table = ?",
		store.database,
		store.table,
	)
	if err != nil {
		return fmt.Errorf("logstore: inspect clickhouse schema: %w", err)
	}
	defer rows.Close()
	want := map[string]string{
		"id":         "String",
		"timestamp":  "DateTime64(9, 'UTC')",
		"stream":     "String",
		"kind":       "String",
		"severity":   "String",
		"message":    "String",
		"attributes": "Map(String, String)",
		"payload":    "String",
	}
	got := make(map[string]string, len(want))
	for rows.Next() {
		var name, columnType string
		if err := rows.Scan(&name, &columnType); err != nil {
			return fmt.Errorf("logstore: scan clickhouse schema: %w", err)
		}
		got[name] = columnType
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("logstore: inspect clickhouse schema rows: %w", err)
	}
	for name, columnType := range want {
		if got[name] != columnType {
			return fmt.Errorf(
				"logstore: incompatible clickhouse column %s: got %q, want %q",
				name,
				got[name],
				columnType,
			)
		}
	}
	return nil
}

// Append writes a validated batch and returns the accepted keys in input order.
func (store *ClickHouseStore) Append(ctx context.Context, records []Record) ([]RecordKey, error) {
	if len(records) == 0 {
		return []RecordKey{}, nil
	}
	if err := store.ready(); err != nil {
		return nil, err
	}
	seen := make(map[RecordKey]struct{}, len(records))
	for _, record := range records {
		if err := ValidateRecord(record); err != nil {
			return nil, err
		}
		key := record.Key()
		if _, duplicate := seen[key]; duplicate {
			return nil, fmt.Errorf("logstore: duplicate record key in append: stream %q id %q", key.Stream, key.ID)
		}
		seen[key] = struct{}{}
	}
	store.appendMu.Lock()
	defer store.appendMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, record := range records {
		key := record.Key()
		exists, err := store.recordCount(ctx, key)
		if err != nil {
			return nil, err
		}
		if exists != 0 {
			return nil, fmt.Errorf("logstore: duplicate record key: stream %q id %q", key.Stream, key.ID)
		}
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("logstore: begin clickhouse append: %w", err)
	}
	statement := fmt.Sprintf(
		"INSERT INTO %s (id, timestamp, stream, kind, severity, message, attributes, payload) SETTINGS async_insert = 0 VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		store.qualified,
	)
	prepared, err := tx.PrepareContext(ctx, statement)
	if err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("logstore: prepare clickhouse append: %w", err)
	}
	defer prepared.Close()
	for _, record := range records {
		attributes := record.Attributes
		if attributes == nil {
			attributes = map[string]string{}
		}
		if _, err := prepared.ExecContext(
			ctx,
			record.ID,
			record.Time.UTC(),
			record.Stream,
			record.Kind,
			record.Severity,
			record.Message,
			attributes,
			string(record.Payload),
		); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("logstore: append clickhouse record: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("logstore: commit clickhouse append: %w", err)
	}
	keys := make([]RecordKey, len(records))
	for index, record := range records {
		keys[index] = record.Key()
	}
	return keys, nil
}

// Query returns one stable time-and-ID ordered page.
func (store *ClickHouseStore) Query(ctx context.Context, query Query) (Page, error) {
	if err := ValidateQuery(query); err != nil {
		return Page{}, err
	}
	if err := store.ready(); err != nil {
		return Page{}, err
	}
	bound := normalizeClickHouseQuery(query)
	var position *clickHousePosition
	if query.Cursor != "" {
		cursor, err := decodeClickHouseCursor(query.Cursor)
		if err != nil {
			return Page{}, err
		}
		if !equalClickHouseQuery(cursor.Query, bound) {
			return Page{}, fmt.Errorf("%w: query fields changed", ErrCursorMismatch)
		}
		position = &cursor.Position
	}
	where, args := buildClickHouseWhere(bound, position)
	direction := "ASC"
	if query.Order == OrderDesc {
		direction = "DESC"
	}
	args = append(args, query.Limit+1)
	statement := fmt.Sprintf(
		"SELECT id, timestamp, stream, kind, severity, message, attributes, payload FROM %s WHERE %s ORDER BY timestamp %s, stream %s, id %s LIMIT ?",
		store.qualified,
		where,
		direction,
		direction,
		direction,
	)
	rows, err := store.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return Page{}, fmt.Errorf("logstore: query clickhouse records: %w", err)
	}
	defer rows.Close()
	records := make([]Record, 0, query.Limit+1)
	for rows.Next() {
		var record Record
		var payload string
		if err := rows.Scan(
			&record.ID,
			&record.Time,
			&record.Stream,
			&record.Kind,
			&record.Severity,
			&record.Message,
			&record.Attributes,
			&payload,
		); err != nil {
			return Page{}, fmt.Errorf("logstore: scan clickhouse record: %w", err)
		}
		record.Time = record.Time.UTC()
		record.Payload = json.RawMessage(payload)
		if err := ValidateRecord(record); err != nil {
			return Page{}, fmt.Errorf("logstore: invalid clickhouse record: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("logstore: query clickhouse rows: %w", err)
	}
	page := Page{}
	if len(records) > query.Limit {
		page.HasNext = true
		records = records[:query.Limit]
	}
	page.Records = records
	if page.HasNext {
		last := records[len(records)-1]
		cursor, err := encodeClickHouseCursor(clickHouseCursor{
			Version: 1,
			Query:   bound,
			Position: clickHousePosition{
				TimeUnixNano: last.Time.UnixNano(),
				Stream:       last.Stream,
				ID:           last.ID,
			},
		})
		if err != nil {
			return Page{}, fmt.Errorf("logstore: encode clickhouse cursor: %w", err)
		}
		page.NextCursor = cursor
	}
	if err := ValidatePage(page, query.Limit); err != nil {
		return Page{}, err
	}
	return page, nil
}

// Replace changes the mutable fields of one existing record. The record time
// is retained because it defines the stable query order and table partition.
func (store *ClickHouseStore) Replace(ctx context.Context, record Record) error {
	if err := ValidateRecord(record); err != nil {
		return err
	}
	if err := store.ready(); err != nil {
		return err
	}
	storedTime, err := store.recordTime(ctx, record.Key())
	if err != nil {
		return err
	}
	if !storedTime.Equal(record.Time.UTC()) {
		return errors.New("logstore: replace cannot change record time")
	}
	statement := fmt.Sprintf(
		"ALTER TABLE %s UPDATE kind = ?, severity = ?, message = ?, attributes = ?, payload = ? WHERE stream = ? AND id = ? SETTINGS mutations_sync = 1",
		store.qualified,
	)
	if _, err := store.db.ExecContext(
		ctx,
		statement,
		record.Kind,
		record.Severity,
		record.Message,
		record.Attributes,
		string(record.Payload),
		record.Stream,
		record.ID,
	); err != nil {
		return fmt.Errorf("logstore: replace clickhouse record: %w", err)
	}
	return nil
}

// Delete removes one existing record.
func (store *ClickHouseStore) Delete(ctx context.Context, key RecordKey) error {
	if err := ValidateRecordKey(key); err != nil {
		return err
	}
	if err := store.ready(); err != nil {
		return err
	}
	count, err := store.recordCount(ctx, key)
	if err != nil {
		return err
	}
	switch {
	case count == 0:
		return fmt.Errorf("%w: stream %q id %q", ErrNotFound, key.Stream, key.ID)
	case count > 1:
		return fmt.Errorf("logstore: duplicate clickhouse record key: stream %q id %q", key.Stream, key.ID)
	}
	statement := fmt.Sprintf(
		"ALTER TABLE %s DELETE WHERE stream = ? AND id = ? SETTINGS mutations_sync = 1",
		store.qualified,
	)
	if _, err := store.db.ExecContext(ctx, statement, key.Stream, key.ID); err != nil {
		return fmt.Errorf("logstore: delete clickhouse record: %w", err)
	}
	return nil
}

func (store *ClickHouseStore) recordTime(ctx context.Context, key RecordKey) (time.Time, error) {
	if err := ValidateRecordKey(key); err != nil {
		return time.Time{}, err
	}
	statement := fmt.Sprintf(
		"SELECT timestamp FROM %s WHERE stream = ? AND id = ? ORDER BY timestamp LIMIT 2",
		store.qualified,
	)
	rows, err := store.db.QueryContext(ctx, statement, key.Stream, key.ID)
	if err != nil {
		return time.Time{}, fmt.Errorf("logstore: find clickhouse record: %w", err)
	}
	defer rows.Close()
	var found []time.Time
	for rows.Next() {
		var value time.Time
		if err := rows.Scan(&value); err != nil {
			return time.Time{}, fmt.Errorf("logstore: scan clickhouse record time: %w", err)
		}
		found = append(found, value.UTC())
	}
	if err := rows.Err(); err != nil {
		return time.Time{}, fmt.Errorf("logstore: find clickhouse record rows: %w", err)
	}
	switch len(found) {
	case 0:
		return time.Time{}, fmt.Errorf("%w: stream %q id %q", ErrNotFound, key.Stream, key.ID)
	case 1:
		return found[0], nil
	default:
		return time.Time{}, fmt.Errorf("logstore: duplicate clickhouse record key: stream %q id %q", key.Stream, key.ID)
	}
}

func (store *ClickHouseStore) recordCount(ctx context.Context, key RecordKey) (uint64, error) {
	statement := fmt.Sprintf(
		"SELECT count() FROM %s WHERE stream = ? AND id = ?",
		store.qualified,
	)
	var count uint64
	if err := store.db.QueryRowContext(ctx, statement, key.Stream, key.ID).Scan(&count); err != nil {
		return 0, fmt.Errorf("logstore: count clickhouse record: %w", err)
	}
	return count, nil
}

func (store *ClickHouseStore) ready() error {
	if store == nil || store.db == nil || store.qualified == "" {
		return errors.New("logstore: clickhouse store is not initialized")
	}
	return nil
}

// Close closes the ClickHouse connection pool once.
func (store *ClickHouseStore) Close() error {
	if store == nil {
		return nil
	}
	store.closeOnce.Do(func() {
		if store.db != nil {
			store.closeErr = store.db.Close()
		}
	})
	return store.closeErr
}

type clickHouseBoundQuery struct {
	Streams    []string
	Kinds      []string
	Severities []string
	Matchers   []AttributeMatcher
	Text       string
	StartMS    int64
	EndMS      int64
	Order      Order
}

type clickHousePosition struct {
	TimeUnixNano int64
	Stream       string
	ID           string
}

type clickHouseCursor struct {
	Version  int
	Query    clickHouseBoundQuery
	Position clickHousePosition
}

func normalizeClickHouseQuery(query Query) clickHouseBoundQuery {
	bound := clickHouseBoundQuery{
		Streams:    append([]string(nil), query.Streams...),
		Kinds:      append([]string(nil), query.Kinds...),
		Severities: append([]string(nil), query.Severities...),
		Matchers:   append([]AttributeMatcher(nil), query.Matchers...),
		Text:       query.Text,
		StartMS:    query.Start.UnixMilli(),
		EndMS:      query.End.UnixMilli(),
		Order:      query.Order,
	}
	slices.Sort(bound.Streams)
	slices.Sort(bound.Kinds)
	slices.Sort(bound.Severities)
	for index := range bound.Matchers {
		if bound.Matchers[index].Op == MatchExists || bound.Matchers[index].Op == MatchNotExists {
			bound.Matchers[index].Value = ""
		}
	}
	slices.SortFunc(bound.Matchers, func(left, right AttributeMatcher) int {
		if value := strings.Compare(left.Name, right.Name); value != 0 {
			return value
		}
		if value := strings.Compare(string(left.Op), string(right.Op)); value != 0 {
			return value
		}
		return strings.Compare(left.Value, right.Value)
	})
	return bound
}

func equalClickHouseQuery(left, right clickHouseBoundQuery) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return string(leftJSON) == string(rightJSON)
}

func buildClickHouseWhere(query clickHouseBoundQuery, position *clickHousePosition) (string, []any) {
	parts := []string{"timestamp >= ?", "timestamp < ?"}
	args := []any{time.UnixMilli(query.StartMS).UTC(), time.UnixMilli(query.EndMS).UTC()}
	appendSet := func(column string, values []string) {
		if len(values) == 0 {
			return
		}
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(values)), ",")
		parts = append(parts, column+" IN ("+placeholders+")")
		for _, value := range values {
			args = append(args, value)
		}
	}
	appendSet("stream", query.Streams)
	appendSet("kind", query.Kinds)
	appendSet("severity", query.Severities)
	if query.Text != "" {
		parts = append(parts, "position(message, ?) > 0")
		args = append(args, query.Text)
	}
	for _, matcher := range query.Matchers {
		switch matcher.Op {
		case MatchEqual:
			parts = append(parts, "attributes[?] = ?")
			args = append(args, matcher.Name, matcher.Value)
		case MatchNotEqual:
			parts = append(parts, "mapContains(attributes, ?) AND attributes[?] != ?")
			args = append(args, matcher.Name, matcher.Name, matcher.Value)
		case MatchExists:
			parts = append(parts, "mapContains(attributes, ?)")
			args = append(args, matcher.Name)
		case MatchNotExists:
			parts = append(parts, "NOT mapContains(attributes, ?)")
			args = append(args, matcher.Name)
		}
	}
	if position != nil {
		operator := ">"
		if query.Order == OrderDesc {
			operator = "<"
		}
		value := time.Unix(0, position.TimeUnixNano).UTC()
		parts = append(parts, "(timestamp "+operator+" ? OR (timestamp = ? AND (stream "+operator+" ? OR (stream = ? AND id "+operator+" ?))))")
		args = append(args, value, value, position.Stream, position.Stream, position.ID)
	}
	return strings.Join(parts, " AND "), args
}

func encodeClickHouseCursor(cursor clickHouseCursor) (string, error) {
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(data)
	if len(encoded) > clickHouseCursorLimit {
		return "", errors.New("logstore: clickhouse cursor is too large")
	}
	return encoded, nil
}

func decodeClickHouseCursor(value string) (clickHouseCursor, error) {
	if len(value) > clickHouseCursorLimit {
		return clickHouseCursor{}, fmt.Errorf("%w: cursor is too large", ErrCursorMismatch)
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil || len(data) > clickHouseCursorLimit {
		return clickHouseCursor{}, fmt.Errorf("%w: cursor is malformed", ErrCursorMismatch)
	}
	var cursor clickHouseCursor
	if err := json.Unmarshal(data, &cursor); err != nil ||
		cursor.Version != 1 ||
		strings.TrimSpace(cursor.Position.Stream) == "" ||
		strings.TrimSpace(cursor.Position.ID) == "" {
		return clickHouseCursor{}, fmt.Errorf("%w: cursor is invalid", ErrCursorMismatch)
	}
	return cursor, nil
}

var (
	_ ImmutableStore = (*ClickHouseStore)(nil)
	_ MutableStore   = (*ClickHouseStore)(nil)
)
