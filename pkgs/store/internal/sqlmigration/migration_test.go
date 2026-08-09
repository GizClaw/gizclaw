package sqlmigration

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func TestPrepareStableNamespaces(t *testing.T) {
	db := sqlx.MustOpen("sqlite", ":memory:")
	t.Cleanup(func() { _ = db.Close() })
	first, err := Prepare(db, "kv", "ExampleTable")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Prepare(db, "kv", "exampletable")
	if err != nil {
		t.Fatal(err)
	}
	if first.VersionTable != second.VersionTable || first.LockID != second.LockID {
		t.Fatalf("SQLite namespaces differ: %+v, %+v", first, second)
	}
	if first.VersionTable != "gizclaw_kv_a204ef271fd26f98_schema_versions" {
		t.Fatalf("VersionTable = %q", first.VersionTable)
	}
	postgres := sqlx.MustOpen("postgres", "postgres://unused")
	t.Cleanup(func() { _ = postgres.Close() })
	postgresNamespace, err := Prepare(postgres, "kv", "ExampleTable")
	if err != nil {
		t.Fatal(err)
	}
	if postgresNamespace.VersionTable != "gizclaw_kv_997e5c06db57c9b8_schema_versions" || postgresNamespace.LockID != -7386365154321512008 {
		t.Fatalf("PostgreSQL namespace = %+v", postgresNamespace)
	}
	postgresLower, err := Prepare(postgres, "kv", "exampletable")
	if err != nil {
		t.Fatal(err)
	}
	if postgresLower.VersionTable == postgresNamespace.VersionTable {
		t.Fatal("PostgreSQL namespace folded table case")
	}
}

func TestValidateIdentifier(t *testing.T) {
	valid := []string{"a", "A_1", "_" + strings.Repeat("x", 62)}
	for _, value := range valid {
		if err := ValidateIdentifier(value); err != nil {
			t.Errorf("ValidateIdentifier(%q) = %v", value, err)
		}
	}
	invalid := []string{"", "1a", "a-b", "a.b", strings.Repeat("x", 64)}
	for _, value := range invalid {
		if err := ValidateIdentifier(value); err == nil {
			t.Errorf("ValidateIdentifier(%q) = nil", value)
		}
	}
}

func TestRunIsIdempotentAndLeavesPoolOpen(t *testing.T) {
	db := sqlx.MustOpen("sqlite", ":memory:")
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	migration := goose.NewGoMigration(1, &goose.GoFunc{RunTx: func(ctx context.Context, tx *sql.Tx) error {
		return TxExec(ctx, tx, `CREATE TABLE "items" (id INTEGER PRIMARY KEY)`)
	}}, nil)
	for range 2 {
		if _, err := Run(context.Background(), db, "kv", "items", migration); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("borrowed pool was closed: %v", err)
	}
}

func TestRunRejectsUnversionedTable(t *testing.T) {
	db := sqlx.MustOpen("sqlite", ":memory:")
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	migration := goose.NewGoMigration(1, &goose.GoFunc{RunTx: func(ctx context.Context, tx *sql.Tx) error {
		return TxExec(ctx, tx, `CREATE TABLE "items" (id INTEGER PRIMARY KEY)`)
	}}, nil)
	if _, err := Run(context.Background(), db, "kv", "items", migration); err == nil || !strings.Contains(err.Error(), "without migration history") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRunHonorsCanceledContextBeforeDDL(t *testing.T) {
	db := sqlx.MustOpen("sqlite", ":memory:")
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	migration := goose.NewGoMigration(1, &goose.GoFunc{RunTx: func(ctx context.Context, tx *sql.Tx) error {
		return TxExec(ctx, tx, `CREATE TABLE "canceled_items" (id INTEGER PRIMARY KEY)`)
	}}, nil)
	if _, err := Run(ctx, db, "kv", "canceled_items", migration); err == nil {
		t.Fatal("Run() with canceled context succeeded")
	}
	exists, err := TableExists(context.Background(), db, SQLite, "canceled_items")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("canceled Run created the data table")
	}
}

func TestRunSerializesConcurrentSamePoolConstruction(t *testing.T) {
	db := sqlx.MustOpen("sqlite", ":memory:")
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	migration := goose.NewGoMigration(1, &goose.GoFunc{RunTx: func(ctx context.Context, tx *sql.Tx) error {
		return TxExec(ctx, tx, `CREATE TABLE "concurrent_items" (id INTEGER PRIMARY KEY)`)
	}}, nil)
	start := make(chan struct{})
	errorsByCaller := make(chan error, 4)
	var group sync.WaitGroup
	for range 4 {
		group.Go(func() {
			<-start
			_, err := Run(context.Background(), db, "kv", "concurrent_items", migration)
			errorsByCaller <- err
		})
	}
	close(start)
	group.Wait()
	close(errorsByCaller)
	for err := range errorsByCaller {
		if err != nil {
			t.Fatalf("concurrent Run() error = %v", err)
		}
	}
}

func TestRunAcrossSeparateSQLitePoolsHasNoPartialDDL(t *testing.T) {
	dsn := "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "concurrent.sqlite")) +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	open := func() *sqlx.DB {
		db := sqlx.MustOpen("sqlite", dsn)
		db.SetMaxOpenConns(1)
		if err := db.Ping(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		return db
	}
	dbs := []*sqlx.DB{open(), open(), open(), open()}
	migration := goose.NewGoMigration(1, &goose.GoFunc{RunTx: func(ctx context.Context, tx *sql.Tx) error {
		return TxExec(ctx, tx, `CREATE TABLE "separate_items" (id INTEGER PRIMARY KEY)`)
	}}, nil)
	start := make(chan struct{})
	errorsByCaller := make(chan error, len(dbs))
	var group sync.WaitGroup
	for _, db := range dbs {
		group.Go(func() {
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := Run(ctx, db, "kv", "separate_items", migration)
			errorsByCaller <- err
		})
	}
	close(start)
	group.Wait()
	close(errorsByCaller)
	for err := range errorsByCaller {
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("separate-pool Run() error = %v", err)
		}
	}
	namespace, err := Prepare(dbs[0], "kv", "separate_items")
	if err != nil {
		t.Fatal(err)
	}
	dataExists, err := TableExists(context.Background(), dbs[0], SQLite, namespace.Table)
	if err != nil || !dataExists {
		t.Fatalf("data table exists = %v, %v", dataExists, err)
	}
	versionExists, err := TableExists(context.Background(), dbs[0], SQLite, namespace.VersionTable)
	if err != nil || !versionExists {
		t.Fatalf("version table exists = %v, %v", versionExists, err)
	}
	if _, err := Run(context.Background(), dbs[0], "kv", "separate_items", migration); err != nil {
		t.Fatalf("Run() after concurrent construction = %v", err)
	}
}
