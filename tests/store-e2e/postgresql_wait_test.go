//go:build store_e2e

package store_e2e_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

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
