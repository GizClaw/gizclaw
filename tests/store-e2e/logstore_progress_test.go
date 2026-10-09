//go:build store_e2e

package store_e2e_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/jmoiron/sqlx"
)

func TestPostgreSQLLogIndependentRecordProgress(t *testing.T) {
	for _, ttl := range []time.Duration{0, 30 * 24 * time.Hour} {
		t.Run(ttl.String(), func(t *testing.T) {
			db := openPostgreSQL(t)
			table := uniqueTable("log_progress")
			cleanupPostgreSQLTables(t, db, table, table+"_keys")
			first, err := logstore.NewSQLStoreWithDBAndTTL(db, table, ttl)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = first.Close() })
			second, err := logstore.NewSQLStoreWithDBAndTTL(db, table, ttl)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = second.Close() })
			tx, err := db.BeginTxx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			var blocker int
			if err := tx.GetContext(t.Context(), &blocker, "SELECT pg_backend_pid()"); err != nil {
				t.Fatal(err)
			}
			if ttl > 0 {
				_, err = tx.ExecContext(t.Context(), `INSERT INTO "`+table+`_keys" (stream,id,expires_at_unix_nano) VALUES ('held-stream','held-id',$1)`, time.Now().Add(ttl).UnixNano())
			} else {
				_, err = tx.ExecContext(t.Context(), `INSERT INTO "`+table+`" (stream,id,timestamp_unix_nano,kind,severity,message,attributes_json,payload_json) VALUES ('held-stream','held-id',1,'message','','','{}',$1)`, []byte{})
			}
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				defer close(done)
				_, err := first.Append(ctx, []logstore.Record{{Stream: "held-stream", ID: "held-id", Kind: "message", Time: time.Now().UTC()}})
				done <- err
			}()
			defer func() { _ = tx.Rollback(); <-done }()
			waitForPostgresBlocker(t, ctx, db, blocker)
			// The same key remains blocked. Different IDs in the same stream,
			// and the same ID in a different stream, must commit independently.
			freeCtx, freeCancel := context.WithTimeout(ctx, 2*time.Second)
			defer freeCancel()
			records := []logstore.Record{
				{Stream: "held-stream", ID: "free-id", Kind: "message", Time: time.Now().UTC()},
				{Stream: "free-stream", ID: "held-id", Kind: "message", Time: time.Now().UTC()},
			}
			keys, err := second.Append(freeCtx, records)
			if err != nil {
				t.Fatalf("independent keys blocked behind held key: %v", err)
			}
			for index, key := range keys {
				if key != records[index].Key() {
					t.Fatalf("accepted keys lost input ordering: %v", keys)
				}
			}
			select {
			case err := <-done:
				t.Fatalf("same record key did not remain blocked: %v", err)
			default:
			}
			if err := tx.Rollback(); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func waitForPostgresBlocker(t *testing.T, ctx context.Context, db *sqlx.DB, blocker int) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err := db.GetContext(ctx, &waiting, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND $1=ANY(pg_blocking_pids(pid)))`, blocker); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(fmt.Errorf("no database waiter on blocker %d: %w", blocker, ctx.Err()))
		case <-ticker.C:
		}
	}
}
