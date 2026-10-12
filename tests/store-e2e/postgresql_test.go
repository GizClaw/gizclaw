//go:build store_e2e

package store_e2e_test

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
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
		"history":  {Kind: stores.KindSQL, Storage: "database"},
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

// Concurrent CREATE ... IF NOT EXISTS on one new relation can fail with
// catalog conflicts (for example SQLSTATE 42710) unless initialization is
// serialized. Every SQL Store kind must tolerate simultaneous constructors.
func TestPostgreSQLConcurrentSchemaInitialization(t *testing.T) {
	db := openPostgreSQL(t)
	constructors := map[string]func(table string) (io.Closer, error){
		"kv": func(table string) (io.Closer, error) { return kv.NewSQLWithDB(db, table, nil) },
		"metrics": func(table string) (io.Closer, error) {
			return metrics.NewSQLStoreWithDB(db, table)
		},
	}
	const rounds, callers = 5, 8
	for kind, construct := range constructors {
		t.Run(kind, func(t *testing.T) {
			for range rounds {
				table := uniqueTable("init_" + kind)
				cleanupPostgreSQLTables(t, db, table)
				stores := make([]io.Closer, callers)
				errs := make([]error, callers)
				start := make(chan struct{})
				var wg sync.WaitGroup
				for index := range callers {
					wg.Go(func() {
						<-start
						stores[index], errs[index] = construct(table)
					})
				}
				close(start)
				wg.Wait()
				for _, store := range stores {
					if store != nil {
						_ = store.Close()
					}
				}
				if err := errors.Join(errs...); err != nil {
					t.Fatalf("concurrent %s constructors on %q: %v", kind, table, err)
				}
			}
		})
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

func TestPostgreSQLLogBackendIsRetired(t *testing.T) {
	db := openPostgreSQL(t)
	table := uniqueTable("retired_log")
	if _, err := logstore.NewSQLStoreWithDB(db, table); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("PostgreSQL LogStore=%v", err)
	}
	var exists bool
	if err := db.Get(&exists, "SELECT to_regclass(current_schema()||'.'||$1) IS NOT NULL", table); err != nil || exists {
		t.Fatalf("retired driver created table=%t err=%v", exists, err)
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

// A stale expired read must wait for its key owner, preserve a refresh, and
// never serialize an independent device. Successful cleanup removes one key.
func TestPostgreSQLKVExpiredReadCoordination(t *testing.T) {
	db := openPostgreSQL(t)
	table := uniqueTable("kv_expired_read")
	cleanupPostgreSQLTables(t, db, table)
	store, err := kv.NewSQLWithDB(db, table, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	key := kv.Key{"held"}
	if err := store.Set(ctx, key, []byte("old")); err != nil {
		t.Fatal(err)
	}
	var encoded []byte
	if err := db.GetContext(ctx, &encoded, `SELECT encoded_key FROM "`+table+`"`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE "`+table+`" SET expires_at_unix_nano = 0`); err != nil {
		t.Fatal(err)
	}
	blocker, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	digest := sha256.Sum256(append([]byte(table+"\x00"), encoded...))
	if _, err := blocker.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", int64(binary.BigEndian.Uint64(digest[:8]))); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := store.Get(ctx, key); done <- err }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting int
		if err := db.GetContext(ctx, &waiting, `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock' AND query LIKE 'SELECT pg_advisory_xact_lock%'`); err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("expired read bypassed key lock: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("expired read did not wait for key lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	independent, independentCancel := context.WithTimeout(ctx, 2*time.Second)
	defer independentCancel()
	if err := store.Set(independent, kv.Key{"other"}, []byte("ok")); err != nil {
		t.Fatalf("unrelated key blocked: %v", err)
	}
	if _, err := blocker.ExecContext(ctx, `UPDATE "`+table+`" SET value = $1, expires_at_unix_nano = NULL WHERE encoded_key = $2`, []byte("fresh"), encoded); err != nil {
		t.Fatal(err)
	}
	if err := blocker.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("expired read = %v", err)
	}
	if value, err := store.Get(ctx, key); err != nil || string(value) != "fresh" {
		t.Fatalf("refresh lost: %q %v", value, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE "`+table+`" SET expires_at_unix_nano = 0`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, key); !errors.Is(err, kv.ErrNotFound) {
		t.Fatal(err)
	}
	var count int
	if err := db.GetContext(ctx, &count, `SELECT count(*) FROM "`+table+`"`); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("cleanup affected unrelated expired key: remaining=%d", count)
	}
}
