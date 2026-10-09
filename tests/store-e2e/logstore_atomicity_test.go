//go:build store_e2e

package store_e2e_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

func TestPostgreSQLLogConflictingBatchesRemainAtomic(t *testing.T) {
	for _, ttl := range []time.Duration{0, 30 * 24 * time.Hour} {
		t.Run(ttl.String(), func(t *testing.T) {
			db := openPostgreSQL(t)
			table := uniqueTable("log_atomic")
			cleanupPostgreSQLTables(t, db, table, table+"_keys")
			store, err := logstore.NewSQLStoreWithDBAndTTL(db, table, ttl)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			const writers = 16
			start := make(chan struct{})
			errs := make([]error, writers)
			var workers sync.WaitGroup
			for writer := range writers {
				workers.Go(func() {
					<-start
					records := []logstore.Record{
						{Stream: "shared", ID: "one", Kind: "message", Time: time.Now().UTC()},
						{Stream: "shared", ID: "two", Kind: "message", Time: time.Now().UTC()},
						{Stream: "shared", ID: fmt.Sprintf("unique-%d", writer), Kind: "message", Time: time.Now().UTC()},
					}
					if writer%2 == 0 {
						records[0], records[1] = records[1], records[0]
					}
					keys, err := store.Append(t.Context(), records)
					errs[writer] = err
					if err == nil {
						if len(keys) != len(records) {
							t.Error("accepted batch length changed")
						}
						for index, key := range keys {
							if key != records[index].Key() {
								t.Error("accepted batch key order changed")
							}
						}
					} else if len(keys) != 0 {
						t.Error("failed transaction returned accepted keys")
					}
				})
			}
			close(start)
			workers.Wait()
			winners := 0
			for _, err := range errs {
				if err == nil {
					winners++
					continue
				}
				var pgErr *pq.Error
				if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
					t.Fatalf("conflicting batch returned %v, want unique violation", err)
				}
			}
			if winners != 1 {
				t.Fatalf("successful conflicting batches = %d, want 1", winners)
			}
			for _, target := range []string{table, table + "_keys"} {
				if target != table && ttl == 0 {
					continue
				}
				var rows int
				if err := db.Get(&rows, `SELECT COUNT(*) FROM "`+target+`"`); err != nil {
					t.Fatal(err)
				}
				if rows != 3 {
					t.Fatalf("%s contains %d rows, want one complete batch", target, rows)
				}
			}
		})
	}
}

func TestPostgreSQLLogConcurrentMissingPartitions(t *testing.T) {
	db := openPostgreSQL(t)
	table := uniqueTable("log_missing")
	cleanupPostgreSQLTables(t, db, table, table+"_keys")
	const ttl = 30 * 24 * time.Hour
	store, err := logstore.NewSQLStoreWithDBAndTTL(db, table, ttl)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	day := time.Now().UTC().Add(ttl).Truncate(24 * time.Hour)
	for _, at := range []time.Time{day, day.AddDate(0, 0, 1)} {
		if _, err := db.Exec(`DROP TABLE "` + table + `_p` + at.Format("20060102") + `"`); err != nil {
			t.Fatal(err)
		}
	}
	const writers = 16
	start := make(chan struct{})
	errs := make([]error, writers)
	var workers sync.WaitGroup
	for writer := range writers {
		workers.Go(func() {
			<-start
			_, errs[writer] = store.Append(t.Context(), []logstore.Record{{Stream: fmt.Sprintf("workspace-%d", writer), ID: "same-id", Kind: "message", Time: time.Now().UTC()}})
		})
	}
	close(start)
	workers.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{table, table + "_keys"} {
		var rows int
		if err := db.Get(&rows, `SELECT COUNT(*) FROM "`+target+`"`); err != nil {
			t.Fatal(err)
		}
		if rows != writers {
			t.Fatalf("%s contains %d rows, want %d", target, rows, writers)
		}
	}
	var children int
	if err := db.Get(&children, `SELECT COUNT(*) FROM pg_inherits WHERE inhparent=('"' || current_schema() || '"."' || $1 || '"')::regclass`, table); err != nil {
		t.Fatal(err)
	}
	if children != 2 {
		t.Fatalf("created %d partitions, want required and following UTC day", children)
	}
}

func seedExpiredPostgresLog(t *testing.T, db *sqlx.DB, table, stream, id string) string {
	t.Helper()
	today := time.Now().UTC().Truncate(24 * time.Hour)
	yesterday := today.AddDate(0, 0, -1)
	child := table + "_p" + yesterday.Format("20060102")
	if _, err := db.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS "%s" PARTITION OF "%s" FOR VALUES FROM (%d) TO (%d)`, child, table, yesterday.UnixNano(), today.UnixNano())); err != nil {
		t.Fatal(err)
	}
	expiration := yesterday.Add(time.Hour).UnixNano()
	if _, err := db.Exec(`INSERT INTO "`+table+`_keys" (stream,id,expires_at_unix_nano) VALUES ($1,$2,$3)`, stream, id, expiration); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO "`+table+`" (stream,id,timestamp_unix_nano,expires_at_unix_nano,kind,severity,message,attributes_json,payload_json) VALUES ($1,$2,$3,$4,'message','','','{}',$5)`, stream, id, yesterday.UnixNano(), expiration, []byte{}); err != nil {
		t.Fatal(err)
	}
	return child
}

func TestPostgreSQLLogMaintenanceWithExpiredKeyReuse(t *testing.T) {
	db := openPostgreSQL(t)
	table := uniqueTable("log_reuse")
	cleanupPostgreSQLTables(t, db, table, table+"_keys")
	store, err := logstore.NewSQLStoreWithDBAndTTL(db, table, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	child := seedExpiredPostgresLog(t, db, table, "workspace", "reused")
	if _, err := store.Get(t.Context(), logstore.RecordKey{Stream: "workspace", ID: "reused"}); !errors.Is(err, logstore.ErrNotFound) {
		t.Fatalf("expired record returned %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var blocker int
	if err := tx.GetContext(ctx, &blocker, "SELECT pg_backend_pid()"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE "`+table+`_keys" SET expires_at_unix_nano=expires_at_unix_nano WHERE stream='workspace' AND id='reused'`); err != nil {
		t.Fatal(err)
	}
	appended := make(chan error, 1)
	go func() {
		defer close(appended)
		_, err := store.Append(ctx, []logstore.Record{{Stream: "workspace", ID: "reused", Kind: "message", Message: "new", Time: time.Now().UTC()}})
		appended <- err
	}()
	defer func() { _ = tx.Rollback(); <-appended }()
	waitForPostgresBlocker(t, ctx, db, blocker)
	var writerPID int
	if err := db.GetContext(ctx, &writerPID, `SELECT pid FROM pg_stat_activity WHERE datname=current_database() AND $1=ANY(pg_blocking_pids(pid)) LIMIT 1`, blocker); err != nil {
		t.Fatal(err)
	}
	maintained := make(chan error, 1)
	go func() { defer close(maintained); maintained <- store.Maintain(ctx) }()
	defer func() { cancel(); <-maintained }()
	// Maintenance must wait at the parent, before touching registry keys.
	waitForPostgresBlocker(t, ctx, db, writerPID)
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := <-appended; err != nil {
		t.Fatal(err)
	}
	if err := <-maintained; err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, logstore.RecordKey{Stream: "workspace", ID: "reused"})
	if err != nil || got.Message != "new" {
		t.Fatalf("Get(reused) = %+v, %v", got, err)
	}
	var exists bool
	if err := db.Get(&exists, `SELECT to_regclass(current_schema() || '.' || $1) IS NOT NULL`, child); err != nil || exists {
		t.Fatalf("expired child remains: %v, %v", exists, err)
	}
	for _, target := range []string{table, table + "_keys"} {
		var rows int
		if err := db.Get(&rows, `SELECT COUNT(*) FROM "`+target+`"`); err != nil || rows != 1 {
			t.Fatalf("%s row count = %d, %v; want reused identity only", target, rows, err)
		}
	}
}

func TestPostgreSQLLogConcurrentMutations(t *testing.T) {
	db := openPostgreSQL(t)
	table := uniqueTable("log_mutations")
	cleanupPostgreSQLTables(t, db, table, table+"_keys")
	store, err := logstore.NewSQLStoreWithDBAndTTL(db, table, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	record := logstore.Record{Stream: "workspace", ID: "one", Kind: "message", Time: time.Now().UTC()}
	if _, err := store.Append(t.Context(), []logstore.Record{record}); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for worker := range 12 {
		workers.Go(func() {
			for range 8 {
				var err error
				switch worker % 3 {
				case 0:
					_, err = store.Append(t.Context(), []logstore.Record{record})
				case 1:
					err = store.Replace(t.Context(), record)
				case 2:
					err = store.Delete(t.Context(), record.Key())
				}
				var pgErr *pq.Error
				if err != nil && !errors.Is(err, logstore.ErrNotFound) && (!errors.As(err, &pgErr) || pgErr.Code != "23505") {
					t.Error(err)
				}
			}
		})
	}
	workers.Wait()
	var rows, keys int
	if err := db.Get(&rows, `SELECT COUNT(*) FROM "`+table+`"`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&keys, `SELECT COUNT(*) FROM "`+table+`_keys"`); err != nil {
		t.Fatal(err)
	}
	if rows != keys || rows > 1 {
		t.Fatalf("concurrent mutations left rows=%d keys=%d", rows, keys)
	}
}

func TestPostgreSQLLogExpirationAcrossUTCDays(t *testing.T) {
	db := openPostgreSQL(t)
	table := uniqueTable("log_days")
	cleanupPostgreSQLTables(t, db, table, table+"_keys")
	today := time.Now().UTC().Truncate(24 * time.Hour)
	firstTTL := today.AddDate(0, 0, 2).Add(23 * time.Hour).Sub(time.Now().UTC())
	for index, ttl := range []time.Duration{firstTTL, firstTTL + 2*time.Hour} {
		store, err := logstore.NewSQLStoreWithDBAndTTL(db, table, ttl)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Close() })
		record := logstore.Record{Stream: "workspace", ID: fmt.Sprintf("day-%d", index), Kind: "message", Time: time.Now().In(time.FixedZone("UTC+8", 8*3600))}
		if _, err := store.Append(context.Background(), []logstore.Record{record}); err != nil {
			t.Fatal(err)
		}
		var actual string
		if err := db.Get(&actual, `SELECT tableoid::regclass::text FROM "`+table+`" WHERE id=$1`, record.ID); err != nil {
			t.Fatal(err)
		}
		want := table + "_p" + today.AddDate(0, 0, 2+index).Format("20060102")
		if actual != want {
			t.Fatalf("expiry across UTC midnight: %s, want %s", actual, want)
		}
	}
}
