//go:build store_e2e

package store_e2e_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

func postgresPool(t *testing.T, config storage.PostgreSQLConfig) *sqlx.DB {
	t.Helper()
	config.DSN = requiredEnvironment(t, "GIZCLAW_TEST_POSTGRES_DSN")
	registry, err := storage.New(map[string]storage.Config{"pool": config})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := registry.Close(); err != nil {
			t.Error(err)
		}
	})
	db, err := registry.SQL("pool")
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// Holding a whole burst prevents a one-session sequential loop from hiding
// connection churn. Later bursts must reuse the same real PostgreSQL backends.
func TestPostgreSQLPoolReusesBursts(t *testing.T) {
	db := postgresPool(t, storage.PostgreSQLConfig{})
	if stats := db.Stats(); stats.OpenConnections != 1 {
		t.Fatalf("pool must open sessions lazily: %+v", stats)
	}
	burst := 16
	if capacity := db.Stats().MaxOpenConnections; capacity > 0 && capacity < burst {
		burst = capacity
	}
	backends := make(map[int]bool)
	for round := range 4 {
		var connections []*sql.Conn
		for range burst {
			conn, err := db.Conn(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			connections = append(connections, conn)
			t.Cleanup(func() { _ = conn.Close() })
			var pid int
			if err := conn.QueryRowContext(t.Context(), "SELECT pg_backend_pid()").Scan(&pid); err != nil {
				t.Fatal(err)
			}
			if round == 0 {
				backends[pid] = true
			} else if !backends[pid] {
				t.Errorf("burst %d opened a new backend %d", round, pid)
			}
		}
		for _, conn := range connections {
			if err := conn.Close(); err != nil {
				t.Fatal(err)
			}
		}
		if stats := db.Stats(); stats.InUse != 0 || stats.Idle != burst || stats.MaxIdleClosed != 0 {
			t.Fatalf("burst %d was not retained/returned: %+v", round, stats)
		}
	}
	if db.Stats().MaxOpenConnections <= 0 {
		t.Fatal("default pool has no connection bound")
	}
}

func TestPostgreSQLPoolBoundsWaitingAndReclaimsIdle(t *testing.T) {
	db := postgresPool(t, storage.PostgreSQLConfig{MaxOpenConns: 4, ConnMaxIdleTime: 100 * time.Millisecond})
	var connections []*sql.Conn
	for range 4 {
		conn, err := db.Conn(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		connections = append(connections, conn)
		t.Cleanup(func() { _ = conn.Close() })
	}
	waitCtx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	conn, err := db.Conn(waitCtx)
	cancel()
	if conn != nil {
		_ = conn.Close()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("excess acquisition: %v", err)
	}
	for _, conn := range connections {
		if err := conn.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if stats := db.Stats(); stats.MaxOpenConnections != 4 || stats.InUse != 0 || stats.Idle != 4 || stats.WaitCount == 0 || stats.WaitDuration == 0 {
		t.Fatalf("pool limit/release: %+v", stats)
	}
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for db.Stats().OpenConnections != 0 {
		select {
		case <-deadline.C:
			t.Fatalf("idle sessions retained: %+v", db.Stats())
		case <-tick.C:
		}
	}
	if stats := db.Stats(); stats.MaxIdleTimeClosed != 4 {
		t.Fatalf("idle expiry: %+v", stats)
	}
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestPostgreSQLPoolReturnsRowsTransactionsAndCanceledQueries(t *testing.T) {
	db := postgresPool(t, storage.PostgreSQLConfig{MaxOpenConns: 4})
	for range 4 {
		rows, err := db.QueryContext(t.Context(), "SELECT n FROM generate_series(1,100) n")
		if err != nil {
			t.Fatal(err)
		}
		if !rows.Next() {
			t.Fatal("missing result")
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		tx, err := db.BeginTx(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(t.Context(), "SELECT 1"); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
	}
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
			defer cancel()
			var unused any
			err := db.QueryRowContext(ctx, "SELECT pg_sleep(10)").Scan(&unused)
			pqError, ok := errors.AsType[*pq.Error](err)
			// pq may return PostgreSQL's cancellation response before sql.Rows
			// publishes ctx.Err. Both must correspond to this actual deadline.
			if !errors.Is(err, context.DeadlineExceeded) && !(ctx.Err() == context.DeadlineExceeded && ok && pqError.Code == "57014") {
				t.Errorf("query cancellation: %v", err)
			}
		})
	}
	workers.Wait()
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if stats := db.Stats(); stats.InUse != 0 || stats.OpenConnections > 4 {
		t.Fatalf("rows/transaction/cancellation leaked sessions: %+v", stats)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if stats := db.Stats(); stats.OpenConnections != 0 {
		t.Fatalf("closed pool retained sessions: %+v", stats)
	}
}
