package sqlbackend

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestPrepareAndIndexNames(t *testing.T) {
	db := sqlx.MustOpen("sqlite", ":memory:")
	t.Cleanup(func() { _ = db.Close() })
	backend, err := Prepare(db, "kv", "Items")
	if err != nil {
		t.Fatal(err)
	}
	if backend.Dialect != SQLite || backend.Table != "Items" || backend.Quoted != `"Items"` {
		t.Fatalf("Prepare() = %+v", backend)
	}
	first, err := IndexName(backend, "expires_idx")
	if err != nil {
		t.Fatal(err)
	}
	second, err := IndexName(Backend{Dialect: SQLite, Kind: "kv", Table: "items"}, "expires_idx")
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(first) > 63 {
		t.Fatalf("IndexName() = %q, %q", first, second)
	}
}

func TestPrepareAndIdentifiersRejectUnsupportedInputs(t *testing.T) {
	if _, err := Prepare(nil, "kv", "items"); err == nil {
		t.Fatal("Prepare(nil) succeeded")
	}
	db := sqlx.MustOpen("sqlite", ":memory:")
	t.Cleanup(func() { _ = db.Close() })
	for _, kind := range []string{"", "other"} {
		if _, err := Prepare(db, kind, "items"); err == nil {
			t.Fatalf("Prepare(kind=%q) succeeded", kind)
		}
	}
	for _, value := range []string{"", "1a", "a-b", "a.b", strings.Repeat("x", 64)} {
		if err := ValidateIdentifier(value); err == nil {
			t.Errorf("ValidateIdentifier(%q) = nil", value)
		}
	}
}

func TestExternalErrorAndUnixNano(t *testing.T) {
	underlying := errors.New("secret bound value")
	err := ExternalError("sqlbackend: query", underlying)
	if !errors.Is(err, underlying) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("ExternalError() = %v", err)
	}
	want := time.Unix(100, 123).UTC()
	nanoseconds, err := UnixNano(want)
	if err != nil || !time.Unix(0, nanoseconds).UTC().Equal(want) {
		t.Fatalf("UnixNano() = %d, %v", nanoseconds, err)
	}
	if _, err := UnixNano(time.Date(2500, 1, 1, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("UnixNano(out of range) succeeded")
	}
}

func TestSQLiteEnsureAndSchemaValidation(t *testing.T) {
	db := sqlx.MustOpen("sqlite", ":memory:")
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	backend, err := Prepare(db, "metrics", "samples")
	if err != nil {
		t.Fatal(err)
	}
	index, err := IndexName(backend, "metric_idx")
	if err != nil {
		t.Fatal(err)
	}
	quotedIndex, err := Quote(backend.Dialect, index)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS "samples" (sequence INTEGER PRIMARY KEY AUTOINCREMENT, metric TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS ` + quotedIndex + ` ON "samples" (metric, sequence)`,
	}
	for range 2 {
		if err := Ensure(context.Background(), db, backend, statements...); err != nil {
			t.Fatal(err)
		}
	}
	columns, err := Columns(context.Background(), db, backend)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Column{
		"sequence": {Type: "INTEGER", Nullable: true, PrimaryKeyPosition: 1},
		"metric":   {Type: "TEXT", Nullable: false},
	}
	if err := ValidateColumns(columns, want); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSequenceIdentity(context.Background(), db, backend, "sequence"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateIndexColumns(context.Background(), db, backend, index, []string{"metric", "sequence"}); err != nil {
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
	backend, err := Prepare(db, "metrics", "samples")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE "samples" (sequence INTEGER PRIMARY KEY, metric TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX "samples_metric" ON "samples" (metric)`); err != nil {
		t.Fatal(err)
	}
	columns, err := Columns(context.Background(), db, backend)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateColumns(columns, map[string]Column{"sequence": {Type: "INTEGER"}}); err == nil {
		t.Fatal("ValidateColumns accepted a missing column")
	}
	if err := ValidateColumns(columns, map[string]Column{
		"sequence": {Type: "INTEGER", Nullable: true, PrimaryKeyPosition: 1},
		"metric":   {Type: "TEXT", Nullable: false},
	}); err == nil {
		t.Fatal("ValidateColumns accepted the wrong nullability")
	}
	if err := ValidateSequenceIdentity(context.Background(), db, backend, "sequence"); err == nil {
		t.Fatal("ValidateSequenceIdentity accepted a non-AUTOINCREMENT column")
	}
	if err := ValidateIndexColumns(context.Background(), db, backend, "samples_metric", []string{"metric"}); err == nil {
		t.Fatal("ValidateIndexColumns accepted a unique index")
	}
	if err := ValidateIndexColumns(context.Background(), db, backend, "missing_index", []string{"metric"}); err == nil {
		t.Fatal("ValidateIndexColumns accepted a missing index")
	}
}

func TestBackendHelpersRejectUnsupportedDialectAndCanceledInitialization(t *testing.T) {
	if _, err := Quote(Dialect("other"), "items"); err == nil {
		t.Fatal("Quote accepted unsupported dialect")
	}
	if _, err := IndexName(Backend{Dialect: SQLite, Kind: "kv", Table: "items"}, "bad-purpose"); err == nil {
		t.Fatal("IndexName accepted invalid purpose")
	}
	db := sqlx.MustOpen("sqlite", ":memory:")
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	backend, err := Prepare(db, "kv", "items")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Ensure(ctx, db, backend, `CREATE TABLE "items" (value BLOB)`); !errors.Is(err, context.Canceled) {
		t.Fatalf("Ensure(canceled) error = %v, want context.Canceled", err)
	}
}
