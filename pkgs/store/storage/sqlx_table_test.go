package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestPrepareAndIndexNames(t *testing.T) {
	db := sqlx.MustOpen("sqlite", ":memory:")
	t.Cleanup(func() { _ = db.Close() })
	backend, err := PrepareSQLTable(db, "kv", "Items")
	if err != nil {
		t.Fatal(err)
	}
	if backend.Dialect() != SQLDialectSQLite || backend.Name() != "Items" || backend.Quoted() != `"Items"` {
		t.Fatalf("PrepareSQLTable() = %+v", backend)
	}
	first, err := SQLIndexName(backend, "expires_idx")
	if err != nil {
		t.Fatal(err)
	}
	lowercase, err := PrepareSQLTable(db, "kv", "items")
	if err != nil {
		t.Fatal(err)
	}
	second, err := SQLIndexName(lowercase, "expires_idx")
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(first) > 63 {
		t.Fatalf("SQLIndexName() = %q, %q", first, second)
	}
	if first != "gizclaw_kv_484498958f0e2dbe_expires_idx" {
		t.Fatalf("SQLIndexName() = %q, want stable persisted name", first)
	}
	hyphenated, err := PrepareSQLTable(db, "kv", "service-state")
	if err != nil || hyphenated.Quoted() != `"service-state"` {
		t.Fatalf("PrepareSQLTable(hyphenated KV prefix) = %+v, %v", hyphenated, err)
	}
	if _, err := QuoteSQLIdentifier(SQLDialectSQLite, "service-state"); err == nil {
		t.Fatal("QuoteSQLIdentifier accepted a hyphenated identifier")
	}
	if quoted, err := QuoteSQLTableName(SQLDialectSQLite, "service-state"); err != nil || quoted != `"service-state"` {
		t.Fatalf("QuoteSQLTableName() = %q, %v", quoted, err)
	}
}

func TestPrepareAndIdentifiersRejectUnsupportedInputs(t *testing.T) {
	if _, err := PrepareSQLTable(nil, "kv", "items"); err == nil {
		t.Fatal("PrepareSQLTable(nil) succeeded")
	}
	db := sqlx.MustOpen("sqlite", ":memory:")
	t.Cleanup(func() { _ = db.Close() })
	for _, kind := range []string{"", "other"} {
		if _, err := PrepareSQLTable(db, kind, "items"); err == nil {
			t.Fatalf("PrepareSQLTable(kind=%q) succeeded", kind)
		}
	}
	for _, value := range []string{"", "1a", "a-b", "a.b", strings.Repeat("x", 64)} {
		if err := ValidateSQLIdentifier(value); err == nil {
			t.Errorf("ValidateSQLIdentifier(%q) = nil", value)
		}
	}
	for _, value := range []string{"", "1a", "a/b", "a.b", strings.Repeat("x", 64)} {
		if err := ValidateSQLTableName(value); err == nil {
			t.Errorf("ValidateSQLTableName(%q) = nil", value)
		}
	}
	if _, err := PrepareSQLTable(db, "metrics", "metric-samples"); err == nil {
		t.Fatal("PrepareSQLTable accepted a hyphenated Metrics table")
	}
}

func TestExternalErrorAndUnixNano(t *testing.T) {
	underlying := errors.New("secret bound value")
	err := ExternalSQLError("storage: sql query", underlying)
	if !errors.Is(err, underlying) || err.Error() != "storage: sql query failed" {
		t.Fatalf("ExternalSQLError() = %v", err)
	}
	want := time.Unix(100, 123).UTC()
	nanoseconds, err := SQLUnixNano(want)
	if err != nil || !time.Unix(0, nanoseconds).UTC().Equal(want) {
		t.Fatalf("SQLUnixNano() = %d, %v", nanoseconds, err)
	}
	if _, err := SQLUnixNano(time.Date(2500, 1, 1, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("SQLUnixNano(out of range) succeeded")
	}
}

func TestSQLiteEnsureAndSchemaValidation(t *testing.T) {
	db := sqlx.MustOpen("sqlite", ":memory:")
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	backend, err := PrepareSQLTable(db, "metrics", "samples")
	if err != nil {
		t.Fatal(err)
	}
	index, err := SQLIndexName(backend, "metric_idx")
	if err != nil {
		t.Fatal(err)
	}
	quotedIndex, err := QuoteSQLIdentifier(backend.Dialect(), index)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS "samples" (sequence INTEGER PRIMARY KEY AUTOINCREMENT, metric TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS ` + quotedIndex + ` ON "samples" (metric, sequence)`,
	}
	for range 2 {
		if err := EnsureSQLTable(context.Background(), db, backend, statements...); err != nil {
			t.Fatal(err)
		}
	}
	columns, err := SQLColumns(context.Background(), db, backend)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]SQLColumn{
		"sequence": {Type: "INTEGER", Nullable: true, PrimaryKeyPosition: 1},
		"metric":   {Type: "TEXT", Nullable: false},
	}
	if err := ValidateSQLColumns(columns, want); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSQLSequenceIdentity(context.Background(), db, backend, "sequence"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSQLIndexColumns(context.Background(), db, backend, index, []string{"metric", "sequence"}); err != nil {
		t.Fatal(err)
	}
	var tables int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 1 {
		t.Fatalf("business table count = %d, want 1", tables)
	}
}

func TestSQLiteSchemaValidationRejectsIncompatibleDefinitions(t *testing.T) {
	db := sqlx.MustOpen("sqlite", ":memory:")
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	backend, err := PrepareSQLTable(db, "metrics", "samples")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE "samples" (sequence INTEGER PRIMARY KEY, metric TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX "samples_metric" ON "samples" (metric)`); err != nil {
		t.Fatal(err)
	}
	columns, err := SQLColumns(context.Background(), db, backend)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSQLColumns(columns, map[string]SQLColumn{"sequence": {Type: "INTEGER"}}); err == nil {
		t.Fatal("ValidateSQLColumns accepted a missing column")
	}
	if err := ValidateSQLColumns(columns, map[string]SQLColumn{
		"sequence": {Type: "INTEGER", Nullable: true, PrimaryKeyPosition: 1},
		"metric":   {Type: "TEXT", Nullable: false},
	}); err == nil {
		t.Fatal("ValidateSQLColumns accepted the wrong nullability")
	}
	if err := ValidateSQLSequenceIdentity(context.Background(), db, backend, "sequence"); err == nil {
		t.Fatal("ValidateSQLSequenceIdentity accepted a non-AUTOINCREMENT column")
	}
	if err := ValidateSQLIndexColumns(context.Background(), db, backend, "samples_metric", []string{"metric"}); err == nil {
		t.Fatal("ValidateSQLIndexColumns accepted a unique index")
	}
	if err := ValidateSQLIndexColumns(context.Background(), db, backend, "missing_index", []string{"metric"}); err == nil {
		t.Fatal("ValidateSQLIndexColumns accepted a missing index")
	}
}

func TestSQLTableHelpersRejectUnsupportedDialectAndCanceledInitialization(t *testing.T) {
	if _, err := QuoteSQLIdentifier(SQLDialect("other"), "items"); err == nil {
		t.Fatal("QuoteSQLIdentifier accepted unsupported dialect")
	}
	db := sqlx.MustOpen("sqlite", ":memory:")
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if err := EnsureSQLTable(context.Background(), db, SQLTable{}, `CREATE TABLE "invalid" (value BLOB)`); err == nil {
		t.Fatal("EnsureSQLTable accepted a zero SQLTable")
	}
	var invalidTableCount int
	if err := db.Get(&invalidTableCount, `SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'invalid'`); err != nil {
		t.Fatal(err)
	}
	if invalidTableCount != 0 {
		t.Fatal("EnsureSQLTable executed DDL for a zero SQLTable")
	}
	backend, err := PrepareSQLTable(db, "kv", "items")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SQLIndexName(backend, "bad-purpose"); err == nil {
		t.Fatal("SQLIndexName accepted invalid purpose")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := EnsureSQLTable(ctx, db, backend, `CREATE TABLE "items" (value BLOB)`); !errors.Is(err, context.Canceled) {
		t.Fatalf("EnsureSQLTable(canceled) error = %v, want context.Canceled", err)
	}
}

type testSQLStateError string

func (err testSQLStateError) Error() string    { return "database error" }
func (err testSQLStateError) SQLState() string { return string(err) }

func TestConcurrentDDLConflictClassification(t *testing.T) {
	for _, code := range []string{"23505", "42P07"} {
		if !isConcurrentDDLConflict(fmt.Errorf("wrapped: %w", testSQLStateError(code))) {
			t.Errorf("SQLSTATE %s was not classified as a concurrent DDL conflict", code)
		}
	}
	for _, err := range []error{testSQLStateError("23503"), errors.New("plain error")} {
		if isConcurrentDDLConflict(err) {
			t.Errorf("%v was classified as a concurrent DDL conflict", err)
		}
	}
}
