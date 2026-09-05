//go:build store_e2e

package store_e2e_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	stores "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

func TestPostgreSQLStorageConnector(t *testing.T) {
	dsn := requiredEnvironment(t, "GIZCLAW_TEST_POSTGRES_DSN")
	registry, err := storage.New(map[string]storage.Config{
		"postgres": storage.PostgreSQLConfig{DSN: dsn},
	})
	if err != nil {
		t.Fatalf("storage.New() error = %v", err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	db, err := registry.SQL("postgres")
	if err != nil {
		t.Fatalf("SQL(postgres) error = %v", err)
	}
	if got := db.DriverName(); got != "postgres" {
		t.Fatalf("DriverName() = %q, want postgres", got)
	}
	if got := db.Rebind("SELECT ?"); got != "SELECT $1" {
		t.Fatalf("Rebind() = %q, want SELECT $1", got)
	}
	if again, err := registry.SQL("postgres"); err != nil || again != db {
		t.Fatalf("second SQL(postgres) = %p, %v; want %p", again, err, db)
	}
	if err := registry.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := db.PingContext(context.Background()); err == nil {
		t.Fatal("closed physical registry left PostgreSQL pool open")
	}
	if err := registry.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestPostgreSQLRootRegistry(t *testing.T) {
	dsn := requiredEnvironment(t, "GIZCLAW_TEST_POSTGRES_DSN")
	physical, err := storage.New(map[string]storage.Config{
		"database": storage.PostgreSQLConfig{DSN: dsn},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	db, err := physical.SQL("database")
	if err != nil {
		t.Fatal(err)
	}
	tables := map[string]string{
		"kv":      uniqueTable("root_kv"),
		"metrics": uniqueTable("root_metrics"),
		"logs":    uniqueTable("root_logs"),
		"history": uniqueTable("root_history"),
	}
	cleanupPostgreSQLTables(t, db, tables["kv"], tables["metrics"], tables["logs"], tables["history"])
	registry, err := stores.New(map[string]stores.Config{
		"kv":       {Kind: stores.KindKeyValue, Storage: "database", Prefix: tables["kv"]},
		"kv_alias": {Kind: stores.KindKeyValue, Storage: "database", Prefix: tables["kv"]},
		"metrics":  {Kind: stores.KindMetrics, Storage: "database", Table: tables["metrics"]},
		"logs":     {Kind: stores.KindLogImmutable, Storage: "database", Table: tables["logs"]},
		"history":  {Kind: stores.KindLogMutable, Storage: "database", Table: tables["history"]},
		"raw":      {Kind: stores.KindSQL, Storage: "database"},
	}, physical)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	primary, err := registry.KV("kv")
	if err != nil {
		t.Fatal(err)
	}
	alias, err := registry.KV("kv_alias")
	if err != nil {
		t.Fatal(err)
	}
	if err := primary.Set(context.Background(), kv.Key{"key"}, []byte("value")); err != nil {
		t.Fatal(err)
	}
	if got, err := alias.Get(context.Background(), kv.Key{"key"}); err != nil || string(got) != "value" {
		t.Fatalf("alias Get() = %q, %v", got, err)
	}
	raw, err := registry.SQL("raw")
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Close(); err != nil {
		t.Fatal(err)
	}
	if err := raw.Ping(); err != nil {
		t.Fatalf("logical Close() closed physical pool: %v", err)
	}
}

func TestPostgreSQLKV(t *testing.T) {
	db := openPostgreSQL(t)
	table := uniqueTable("kv")
	cleanupPostgreSQLTables(t, db, table)
	storeInstances := make([]*kv.SQL, 2)
	constructorErrors := make([]error, len(storeInstances))
	constructorStart := make(chan struct{})
	var constructors sync.WaitGroup
	for index := range storeInstances {
		constructors.Go(func() {
			<-constructorStart
			storeInstances[index], constructorErrors[index] = kv.NewSQLWithDB(db, table, nil)
		})
	}
	close(constructorStart)
	constructors.Wait()
	for index, err := range constructorErrors {
		if err != nil {
			t.Fatalf("constructor %d: %v", index, err)
		}
	}
	first, second := storeInstances[0], storeInstances[1]
	t.Cleanup(func() {
		_ = first.Close()
		_ = second.Close()
	})
	ctx := context.Background()
	if err := first.Set(ctx, kv.Key{"empty-upsert"}, nil); err != nil {
		t.Fatal(err)
	}
	assertPostgreSQLZeroLengthValue(t, db, table, kv.Key{"empty-upsert"})
	if _, created, err := first.CreateIfAbsent(ctx, kv.Entry{Key: kv.Key{"empty-guard"}, Value: []byte{}}, nil); err != nil || !created {
		t.Fatalf("CreateIfAbsent(empty guard) = _, %v, %v", created, err)
	}
	assertPostgreSQLZeroLengthValue(t, db, table, kv.Key{"empty-guard"})
	related := kv.Key{"by-provider", "openai", "credential"}
	if _, created, err := first.CreateIfAbsent(ctx,
		kv.Entry{Key: kv.Key{"by-id", "credential"}, Value: []byte("record")},
		[]kv.Entry{{Key: related, Value: []byte{}}},
	); err != nil || !created {
		t.Fatalf("CreateIfAbsent(related empty) = _, %v, %v", created, err)
	}
	assertPostgreSQLZeroLengthValue(t, db, table, related)
	sequentialGuard := kv.Entry{Key: kv.Key{"sequential"}, Value: []byte("winner")}
	if _, created, err := first.CreateIfAbsent(ctx, sequentialGuard, nil); err != nil || !created {
		t.Fatalf("first sequential CreateIfAbsent() = _, %v, %v", created, err)
	}
	if existing, created, err := second.CreateIfAbsent(ctx,
		kv.Entry{Key: sequentialGuard.Key, Value: []byte("loser")}, nil,
	); err != nil || created || string(existing) != "winner" {
		t.Fatalf("second sequential CreateIfAbsent() = %q, %v, %v", existing, created, err)
	}

	guard := kv.Key{"concurrent"}
	type result struct {
		existing []byte
		created  bool
		err      error
		value    string
	}
	results := make([]result, 2)
	start := make(chan struct{})
	var creators sync.WaitGroup
	for index, store := range []*kv.SQL{first, second} {
		creators.Go(func() {
			<-start
			results[index].value = fmt.Sprintf("caller-%d", index)
			results[index].existing, results[index].created, results[index].err = store.CreateIfAbsent(
				ctx, kv.Entry{Key: guard, Value: []byte(results[index].value)}, nil,
			)
		})
	}
	close(start)
	creators.Wait()
	createdCount := 0
	winner := ""
	for _, result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.created {
			createdCount++
			winner = result.value
		}
	}
	if createdCount != 1 {
		t.Fatalf("concurrent creators = %d, want 1", createdCount)
	}
	for _, result := range results {
		if !result.created && string(result.existing) != winner {
			t.Fatalf("loser existing = %q, want %q", result.existing, winner)
		}
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("logical Close() closed borrowed pool: %v", err)
	}
}

func TestPostgreSQLMetrics(t *testing.T) {
	db := openPostgreSQL(t)
	table := uniqueTable("metrics")
	cleanupPostgreSQLTables(t, db, table)
	store, err := metrics.NewSQLStoreWithDB(db, table)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Unix(100, 123).UTC()
	if err := store.Append(context.Background(), []metrics.Sample{
		{Name: "cpu", Labels: map[string]string{"host": "one"}, Timestamp: now, Value: 1},
		{Name: "cpu", Labels: map[string]string{"host": "one"}, Timestamp: now, Value: 2},
	}); err != nil {
		t.Fatal(err)
	}
	latest, err := store.Latest(context.Background(), metrics.LatestQuery{
		Selector: metrics.Selector{Name: "cpu"}, At: now, Lookback: time.Second,
	})
	if err != nil || len(latest) != 1 || latest[0].Points[0].Value != 2 {
		t.Fatalf("Latest() = %+v, %v", latest, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("logical Close() closed borrowed pool: %v", err)
	}
}

func TestPostgreSQLLog(t *testing.T) {
	db := openPostgreSQL(t)
	table := uniqueTable("logs")
	cleanupPostgreSQLTables(t, db, table)
	store, err := logstore.NewSQLStoreWithDB(db, table)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.UnixMilli(1000).UTC()
	first := logstore.Record{ID: "one", Stream: "events", Kind: "created", Time: now}
	second := logstore.Record{ID: "two", Stream: "events", Kind: "created", Time: now.Add(time.Millisecond)}
	if _, err := store.Append(context.Background(), []logstore.Record{first, second}); err != nil {
		t.Fatal(err)
	}
	first.Message = "updated"
	if err := store.Replace(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), first.Key())
	if err != nil || got.Message != "updated" || got.Key() != first.Key() {
		t.Fatalf("Get() = %+v, %v", got, err)
	}
	page, err := store.Query(context.Background(), logstore.Query{
		Start: now, End: now.Add(time.Second), Limit: 10, Order: logstore.OrderAsc,
	})
	if err != nil || len(page.Records) != 2 || page.Records[0].Message != "updated" {
		t.Fatalf("Query() = %+v, %v", page, err)
	}

	sqliteDB := sqlx.MustOpen("sqlite", ":memory:")
	sqliteDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqliteDB.Close() })
	sqliteStore, err := logstore.NewSQLStoreWithDB(sqliteDB, "cross_driver_logs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqliteStore.Close() })
	if _, err := sqliteStore.Append(context.Background(), []logstore.Record{first, second}); err != nil {
		t.Fatal(err)
	}
	query := logstore.Query{Start: now, End: now.Add(time.Second), Limit: 1, Order: logstore.OrderAsc}
	sqlitePage, err := sqliteStore.Query(context.Background(), query)
	if err != nil || !sqlitePage.HasNext || sqlitePage.Records[0].ID != "one" {
		t.Fatalf("SQLite first page = %+v, %v", sqlitePage, err)
	}
	query.Cursor = sqlitePage.NextCursor
	query.Limit = 10
	postgresPage, err := store.Query(context.Background(), query)
	if err != nil || len(postgresPage.Records) != 1 || postgresPage.Records[0].ID != "two" {
		t.Fatalf("PostgreSQL continuation = %+v, %v", postgresPage, err)
	}
	if err := store.Delete(context.Background(), first.Key()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(context.Background(), first.Key()); !errors.Is(err, logstore.ErrNotFound) {
		t.Fatalf("Get(deleted) error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("logical Close() closed borrowed pool: %v", err)
	}
}

func TestPostgreSQLLogTTLUsesDailyPartitions(t *testing.T) {
	db := openPostgreSQL(t)
	table := uniqueTable("logs_ttl")
	keysTable := table + "_keys"
	cleanupPostgreSQLTables(t, db, table, keysTable)
	const ttl = 30 * 24 * time.Hour
	const constructors = 4
	stores := make(chan *logstore.SQLStore, constructors)
	constructorErrors := make(chan error, constructors)
	start := make(chan struct{})
	var constructorsDone sync.WaitGroup
	for range constructors {
		constructorsDone.Add(1)
		go func() {
			defer constructorsDone.Done()
			<-start
			store, err := logstore.NewSQLStoreWithDBAndTTL(db, table, ttl)
			if err != nil {
				constructorErrors <- err
				return
			}
			stores <- store
		}()
	}
	close(start)
	constructorsDone.Wait()
	close(stores)
	close(constructorErrors)
	for err := range constructorErrors {
		t.Fatalf("concurrent NewSQLStoreWithDBAndTTL() error = %v", err)
	}
	var store *logstore.SQLStore
	for candidate := range stores {
		if store == nil {
			store = candidate
		} else {
			_ = candidate.Close()
		}
	}
	if store == nil {
		t.Fatal("concurrent constructors returned no Store")
	}
	t.Cleanup(func() { _ = store.Close() })

	var relationKind string
	if err := db.Get(&relationKind, `
		SELECT class.relkind
		FROM pg_class class
		JOIN pg_namespace backend ON backend.oid = class.relnamespace
		WHERE backend.nspname = current_schema() AND class.relname = $1`, table); err != nil {
		t.Fatal(err)
	}
	if relationKind != "p" {
		t.Fatalf("parent relkind = %q, want partitioned table", relationKind)
	}
	var partitionCount int
	if err := db.Get(&partitionCount, `
		SELECT COUNT(*)
		FROM pg_inherits inheritance
		JOIN pg_class parent ON parent.oid = inheritance.inhparent
		JOIN pg_namespace backend ON backend.oid = parent.relnamespace
		WHERE backend.nspname = current_schema() AND parent.relname = $1`, table); err != nil {
		t.Fatal(err)
	}
	if partitionCount != 2 {
		t.Fatalf("initial partition count = %d, want expiry day and following day", partitionCount)
	}

	record := logstore.Record{
		ID: "one", Stream: "history", Kind: "message",
		Time: time.UnixMilli(1000).UTC(),
	}
	beforeAppend := time.Now().UTC()
	if _, err := store.Append(context.Background(), []logstore.Record{record}); err != nil {
		t.Fatal(err)
	}
	afterAppend := time.Now().UTC()
	var expiresAt int64
	var childTable string
	if err := db.QueryRow(
		`SELECT expires_at_unix_nano, tableoid::regclass::text FROM "`+table+`" WHERE stream = $1 AND id = $2`,
		record.Stream,
		record.ID,
	).Scan(&expiresAt, &childTable); err != nil {
		t.Fatal(err)
	}
	if expiresAt < beforeAppend.Add(ttl).UnixNano() || expiresAt > afterAppend.Add(ttl).UnixNano() {
		t.Fatalf("expires_at_unix_nano = %d, want write-time TTL in [%d, %d]", expiresAt, beforeAppend.Add(ttl).UnixNano(), afterAppend.Add(ttl).UnixNano())
	}
	wantChild := table + "_p" + time.Unix(0, expiresAt).UTC().Format("20060102")
	if childTable != wantChild {
		t.Fatalf("record partition = %q, want %q", childTable, wantChild)
	}
	if _, err := store.Append(context.Background(), []logstore.Record{record}); err == nil {
		t.Fatal("duplicate Append succeeded across partitioned key registry")
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	yesterday := today.Add(-24 * time.Hour)
	stalePartition := table + "_p" + yesterday.Format("20060102")
	if _, err := db.Exec(fmt.Sprintf(
		`CREATE TABLE "%s" PARTITION OF "%s" FOR VALUES FROM (%d) TO (%d)`,
		stalePartition,
		table,
		yesterday.UnixNano(),
		today.UnixNano(),
	)); err != nil {
		t.Fatal(err)
	}
	staleExpiry := yesterday.Add(time.Hour).UnixNano()
	if _, err := db.Exec(
		`INSERT INTO "`+keysTable+`" (stream, id, expires_at_unix_nano) VALUES ($1, $2, $3)`,
		"history",
		"stale",
		staleExpiry,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO "`+table+`" (stream, id, timestamp_unix_nano, expires_at_unix_nano, kind, severity, message, attributes_json, payload_json) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		"history", "stale", yesterday.UnixNano(), staleExpiry, "message", "", "", "{}", []byte{},
	); err != nil {
		t.Fatal(err)
	}
	live := record
	live.ID = "two"
	if _, err := store.Append(context.Background(), []logstore.Record{live}); err != nil {
		t.Fatal(err)
	}
	var staleExists bool
	if err := db.Get(&staleExists, `SELECT to_regclass(current_schema() || '.' || $1) IS NOT NULL`, stalePartition); err != nil {
		t.Fatal(err)
	}
	if staleExists {
		t.Fatalf("expired daily partition %q still exists", stalePartition)
	}
	var staleKeyCount int
	if err := db.Get(&staleKeyCount, `SELECT COUNT(*) FROM "`+keysTable+`" WHERE stream = $1 AND id = $2`, "history", "stale"); err != nil {
		t.Fatal(err)
	}
	if staleKeyCount != 0 {
		t.Fatalf("expired partition key count = %d, want 0", staleKeyCount)
	}
}

func TestPostgreSQLLogTTLStartsAfterPartitionMaintenance(t *testing.T) {
	db := openPostgreSQL(t)
	table := uniqueTable("logs_ttl_lock")
	keysTable := table + "_keys"
	cleanupPostgreSQLTables(t, db, table, keysTable)
	const ttl = 30 * 24 * time.Hour
	store, err := logstore.NewSQLStoreWithDBAndTTL(db, table, ttl)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	lockTx, err := db.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer lockTx.Rollback()
	var lockResult any
	if err := lockTx.QueryRow(
		"SELECT pg_advisory_xact_lock(hashtext(current_schema()::text), hashtext($1))",
		table,
	).Scan(&lockResult); err != nil {
		t.Fatal(err)
	}

	record := logstore.Record{
		ID: "delayed", Stream: "history", Kind: "message",
		Time: time.UnixMilli(1000).UTC(),
	}
	appendResult := make(chan error, 1)
	go func() {
		_, err := store.Append(context.Background(), []logstore.Record{record})
		appendResult <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting int
		if err := db.Get(&waiting, `
			SELECT COUNT(*)
			FROM pg_stat_activity
			WHERE datname = current_database()
			  AND wait_event_type = 'Lock'
			  AND lower(wait_event) = 'advisory'
			  AND query LIKE '%pg_advisory_xact_lock%'`); err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Append did not wait for PostgreSQL partition maintenance lock")
		}
		time.Sleep(10 * time.Millisecond)
	}

	maintenanceReleased := time.Now().UTC()
	if err := lockTx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-appendResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Append did not complete after PostgreSQL partition maintenance lock release")
	}
	var expiresAt int64
	if err := db.Get(
		&expiresAt,
		`SELECT expires_at_unix_nano FROM "`+table+`" WHERE stream = $1 AND id = $2`,
		record.Stream,
		record.ID,
	); err != nil {
		t.Fatal(err)
	}
	if expiresAt < maintenanceReleased.Add(ttl).UnixNano() {
		t.Fatalf(
			"expires_at_unix_nano = %d, want at least lock release plus TTL %d",
			expiresAt,
			maintenanceReleased.Add(ttl).UnixNano(),
		)
	}
}

func openPostgreSQL(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("postgres", requiredEnvironment(t, "GIZCLAW_TEST_POSTGRES_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		t.Fatalf("PingContext() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func assertPostgreSQLZeroLengthValue(t *testing.T, db *sqlx.DB, table string, key kv.Key) {
	t.Helper()
	var isNull bool
	var length int
	query := `SELECT value IS NULL, octet_length(value) FROM "` + table + `" WHERE encoded_key = $1`
	if err := db.QueryRow(query, []byte(key.String())).Scan(&isNull, &length); err != nil {
		t.Fatalf("inspect zero-length value: %v", err)
	}
	if isNull || length != 0 {
		t.Fatalf("stored value: is_null=%v length=%d, want false and 0", isNull, length)
	}
}

func cleanupPostgreSQLTables(t *testing.T, db *sqlx.DB, tables ...string) {
	t.Helper()
	t.Cleanup(func() {
		for _, table := range tables {
			_, _ = db.Exec(`DROP TABLE IF EXISTS "` + table + `" CASCADE`)
		}
	})
}

func requiredEnvironment(t *testing.T, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		t.Fatalf("%s is required", name)
	}
	return value
}

func uniqueTable(prefix string) string {
	return fmt.Sprintf("gzc_e2e_%s_%d", prefix, time.Now().UnixNano())
}

// An unrelated device must keep making progress while a writer is blocked on
// one device. Observe the database wait before asserting independent progress.
func TestPostgreSQLKVIndependentKeyProgress(t *testing.T) {
	db := openPostgreSQL(t)
	table := uniqueTable("kv_progress")
	cleanupPostgreSQLTables(t, db, table)
	first, err := kv.NewSQLWithDB(db, table, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := kv.NewSQLWithDB(db, table, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := first.Set(ctx, kv.Key{"held"}, []byte("old")); err != nil {
		t.Fatal(err)
	}
	blocker, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	if _, err := blocker.ExecContext(ctx, `UPDATE "`+table+`" SET value = value`); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- first.Set(ctx, kv.Key{"held"}, []byte("new")) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting int
		err := db.GetContext(ctx, &waiting, `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock' AND position($1 in query) > 0`, table)
		if err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("writer did not reach a database lock wait")
		}
		time.Sleep(10 * time.Millisecond)
	}
	independent, independentCancel := context.WithTimeout(ctx, 2*time.Second)
	defer independentCancel()
	if err := second.Set(independent, kv.Key{"other"}, []byte("ok")); err != nil {
		t.Fatalf("unrelated key blocked: %v", err)
	}
	// The ordinary writer holds the same-key coordination while waiting for the
	// row, so a compare cannot read the old guard and commit a stale mutation.
	compared := make(chan error, 1)
	go func() {
		matched, err := second.CompareAndMutate(ctx, kv.Key{"held"}, []byte("old"), []kv.Entry{{Key: kv.Key{"stale"}, Value: []byte("bad")}}, nil)
		if err == nil && matched {
			err = errors.New("compare committed against stale guard")
		}
		compared <- err
	}()
	if err := blocker.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-compared; err != nil {
		t.Fatal(err)
	}
	if _, err := second.Get(ctx, kv.Key{"stale"}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("stale side effect: %v", err)
	}
}

func TestPostgreSQLKVCompareAndMutateConcurrentWinner(t *testing.T) {
	db := openPostgreSQL(t)
	table := uniqueTable("kv_compare")
	cleanupPostgreSQLTables(t, db, table)
	store, err := kv.NewSQLWithDB(db, table, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	guard := kv.Key{"guard"}
	if err := store.Set(ctx, guard, []byte("initial")); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan bool, 16)
	failures := make(chan error, 16)
	for i := range 16 {
		go func() {
			<-start
			matched, err := store.CompareAndMutate(ctx, guard, []byte("initial"), []kv.Entry{{Key: guard, Value: []byte(fmt.Sprint(i))}}, nil)
			failures <- err
			results <- matched
		}()
	}
	close(start)
	winners := 0
	for range 16 {
		if err := <-failures; err != nil {
			t.Fatal(err)
		}
		if <-results {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("compare winners = %d, want 1", winners)
	}
}
