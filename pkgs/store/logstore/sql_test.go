package logstore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func newSQLiteLog(t *testing.T) (*SQLStore, *sqlx.DB) {
	t.Helper()
	db := sqlx.MustOpen("sqlite", ":memory:")
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewSQLStoreWithDB(db, "log_records")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, db
}

func TestSQLLogAppendQueryAndCursor(t *testing.T) {
	store, _ := newSQLiteLog(t)
	ctx := context.Background()
	start := time.UnixMilli(1000).UTC()
	records := []Record{
		{ID: "a", Time: start, Stream: "events", Kind: "message", Severity: "info", Message: "Hello needle", Attributes: map[string]string{"request.id": "one"}, Payload: json.RawMessage(`{"n":1}`)},
		{ID: "b", Time: start.Add(time.Nanosecond), Stream: "events", Kind: "message", Severity: "warn", Message: "needle", Attributes: map[string]string{"request.id": "two"}, Payload: json.RawMessage(` { "n": 2 } `)},
		{ID: "c", Time: start.Add(2 * time.Nanosecond), Stream: "events", Kind: "message", Severity: "warn", Message: "other", Attributes: map[string]string{"request.id": "three"}},
	}
	if keys, err := store.Append(ctx, records); err != nil || len(keys) != len(records) {
		t.Fatalf("Append() = %+v, %v", keys, err)
	}
	query := Query{Streams: []string{"events"}, Kinds: []string{"message"}, Text: "needle", Start: start, End: start.Add(time.Second), Limit: 1, Order: OrderAsc}
	first, err := store.Query(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Records) != 1 || first.Records[0].ID != "a" || !first.HasNext {
		t.Fatalf("first page = %+v", first)
	}
	query.Cursor = first.NextCursor
	query.Limit = 10
	second, err := store.Query(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Records) != 1 || second.Records[0].ID != "b" || second.HasNext {
		t.Fatalf("second page = %+v", second)
	}
	if string(second.Records[0].Payload) != string(records[1].Payload) {
		t.Fatalf("payload = %q, want byte-preserving %q", second.Records[0].Payload, records[1].Payload)
	}
}

func TestSQLLogMutationAndLifecycle(t *testing.T) {
	store, db := newSQLiteLog(t)
	ctx := context.Background()
	record := Record{ID: "one", Time: time.UnixMilli(1000).UTC(), Stream: "events", Kind: "created"}
	if _, err := store.Append(ctx, []Record{record}); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, record.Key())
	if err != nil || got.Key() != record.Key() || !got.Time.Equal(record.Time) {
		t.Fatalf("Get() = %+v, %v", got, err)
	}
	if _, err := store.Get(ctx, RecordKey{Stream: record.Stream, ID: "missing"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing Get error = %v", err)
	}
	if _, err := store.Append(ctx, []Record{record}); err == nil {
		t.Fatal("duplicate Append succeeded")
	} else if strings.Contains(err.Error(), record.Stream) || strings.Contains(err.Error(), record.ID) {
		t.Fatalf("duplicate Append leaked bound record values: %v", err)
	}
	changedTime := record
	changedTime.Time = changedTime.Time.Add(time.Nanosecond)
	if err := store.Replace(ctx, changedTime); err == nil {
		t.Fatal("Replace changed time")
	}
	record.Message = "updated"
	if err := store.Replace(ctx, record); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Get(ctx, record.Key()); err != nil || got.Message != "updated" {
		t.Fatalf("Get(replaced) = %+v, %v", got, err)
	}
	if err := store.Delete(ctx, record.Key()); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, record.Key()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Delete error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(ctx, nil); err == nil {
		t.Fatal("closed Store accepted Append")
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("Close closed borrowed pool: %v", err)
	}
}

func TestSQLLogTTLIsEnforcedByStore(t *testing.T) {
	db := sqlx.MustOpen("sqlite", ":memory:")
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewSQLStoreWithDBAndTTL(db, "log_records", 90*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	record := Record{ID: "expired", Time: time.UnixMilli(1000).UTC(), Stream: "history", Kind: "message"}
	beforeAppend := time.Now().UTC()
	if _, err := store.Append(ctx, []Record{record}); err != nil {
		t.Fatal(err)
	}
	afterAppend := time.Now().UTC()
	var expiresAt int64
	if err := db.Get(&expiresAt, "SELECT expires_at_unix_nano FROM log_records WHERE stream = ? AND id = ?", record.Stream, record.ID); err != nil {
		t.Fatal(err)
	}
	if expiresAt < beforeAppend.Add(90*24*time.Hour).UnixNano() || expiresAt > afterAppend.Add(90*24*time.Hour).UnixNano() {
		t.Fatalf("expires_at_unix_nano = %d, want write-time TTL in [%d, %d]", expiresAt, beforeAppend.Add(90*24*time.Hour).UnixNano(), afterAppend.Add(90*24*time.Hour).UnixNano())
	}
	if _, err := db.Exec("UPDATE log_records SET expires_at_unix_nano = ? WHERE stream = ? AND id = ?", time.Now().Add(-time.Second).UnixNano(), record.Stream, record.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, record.Key()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(expired) error = %v, want ErrNotFound", err)
	}
	page, err := store.Query(ctx, Query{Streams: []string{record.Stream}, Start: record.Time.Add(-time.Second), End: record.Time.Add(time.Second), Limit: 10, Order: OrderAsc})
	if err != nil || len(page.Records) != 0 {
		t.Fatalf("Query(expired) = %+v, %v", page, err)
	}
	live := Record{ID: "live", Time: record.Time.Add(time.Nanosecond), Stream: record.Stream, Kind: record.Kind}
	if _, err := store.Append(ctx, []Record{live}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM log_records WHERE id = ?", record.ID); err != nil || count != 0 {
		t.Fatalf("expired physical row count = %d, %v, want 0 after append cleanup", count, err)
	}
}

func TestPostgresAuxiliaryNamesAndPartitionBounds(t *testing.T) {
	short, err := postgresAuxiliaryName("workspace_history", "p20260901")
	if err != nil || short != "workspace_history_p20260901" {
		t.Fatalf("postgresAuxiliaryName(short) = %q, %v", short, err)
	}
	longBase := strings.Repeat("a", 63)
	first, err := postgresAuxiliaryName(longBase, "p20260901")
	if err != nil {
		t.Fatal(err)
	}
	second, err := postgresAuxiliaryName(longBase, "p20260902")
	if err != nil {
		t.Fatal(err)
	}
	if len(first) > 63 || len(second) > 63 || first == second {
		t.Fatalf("long auxiliary names = %q and %q", first, second)
	}

	db := sqlx.MustOpen("sqlite", ":memory:")
	t.Cleanup(func() { _ = db.Close() })
	table, err := storage.PrepareSQLTable(db, "log", "workspace_history")
	if err != nil {
		t.Fatal(err)
	}
	store := &SQLStore{table: table}
	partition, err := store.postgresPartition(time.Date(2026, 9, 1, 23, 0, 0, 0, time.FixedZone("test", 8*60*60)))
	if err != nil {
		t.Fatal(err)
	}
	if partition.name != "workspace_history_p20260901" || !partition.day.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("partition = %+v", partition)
	}
	if partition.upper-partition.lower != int64(24*time.Hour) {
		t.Fatalf("partition width = %d, want %d", partition.upper-partition.lower, 24*time.Hour)
	}
}

func TestSQLLogSelectorsCancellationCursorBindingAndSchema(t *testing.T) {
	store, db := newSQLiteLog(t)
	ctx := context.Background()
	start := time.UnixMilli(5000).UTC()
	records := []Record{
		{ID: "a", Time: start, Stream: "events", Kind: "message", Severity: "info", Message: "Needle", Attributes: map[string]string{"request.id": "one"}},
		{ID: "b", Time: start.Add(time.Millisecond), Stream: "events", Kind: "message", Severity: "warn", Message: "needle", Attributes: map[string]string{"request.id": "two"}},
	}
	if _, err := store.Append(ctx, records); err != nil {
		t.Fatal(err)
	}
	query := Query{
		Streams: []string{"events"}, Kinds: []string{"message"}, Severities: []string{"warn"},
		Matchers: []AttributeMatcher{{Name: "request.id", Op: MatchEqual, Value: "two"}},
		Text:     "needle", Start: start, End: start.Add(time.Second), Limit: 1, Order: OrderDesc,
	}
	page, err := store.Query(ctx, query)
	if err != nil || len(page.Records) != 1 || page.Records[0].ID != "b" {
		t.Fatalf("Query(selectors) = %+v, %v", page, err)
	}
	first, err := store.Query(ctx, Query{Start: start, End: start.Add(time.Second), Limit: 1, Order: OrderAsc})
	if err != nil || !first.HasNext {
		t.Fatalf("Query(first page) = %+v, %v", first, err)
	}
	if _, err := store.Query(ctx, Query{Start: start, End: start.Add(2 * time.Second), Limit: 1, Order: OrderAsc, Cursor: first.NextCursor}); !errors.Is(err, ErrCursorMismatch) {
		t.Fatalf("Query(changed cursor binding) error = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Query(canceled, query); !errors.Is(err, context.Canceled) {
		t.Fatalf("Query(canceled) error = %v", err)
	}
	index, err := storage.SQLIndexName(store.table, "page_idx")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP INDEX "` + index + `"`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE INDEX "` + index + `" ON "log_records" (stream)`); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSQLStoreWithDB(db, "log_records"); err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("NewSQLStoreWithDB() error = %v", err)
	}
}

func TestSQLLogRejectsOutOfRangeTime(t *testing.T) {
	store, _ := newSQLiteLog(t)
	_, err := store.Append(context.Background(), []Record{{
		ID: "one", Stream: "events", Kind: "created", Time: time.Date(2500, 1, 1, 0, 0, 0, 0, time.UTC),
	}})
	if err == nil || !strings.Contains(err.Error(), "outside signed nanosecond range") {
		t.Fatalf("Append() error = %v", err)
	}
}
