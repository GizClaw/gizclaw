package gizclaw

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/jmoiron/sqlx"
)

type blockingHistoryMaintenance struct {
	entered chan context.Context
}

func (m *blockingHistoryMaintenance) Maintain(ctx context.Context) error {
	m.entered <- ctx
	<-ctx.Done()
	return ctx.Err()
}

func TestHistoryRetentionCancelsBeforeClose(t *testing.T) {
	maintenance := &blockingHistoryMaintenance{entered: make(chan context.Context, 2)}
	r := &historyRetention{interval: time.Millisecond, stores: []historyMaintainer{maintenance}}
	r.start(t.Context())
	r.start(t.Context())
	var attempt context.Context
	select {
	case attempt = <-maintenance.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("maintenance did not start")
	}
	done := make(chan struct{})
	go func() { r.close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not cancel and join blocked maintenance")
	}
	if attempt.Err() != context.Canceled {
		t.Fatal("maintenance context was not canceled")
	}
	r.close()
	select {
	case <-maintenance.entered:
		t.Fatal("duplicate worker or maintenance after Close")
	default:
	}
}

func TestPostgreSQLHistoryRetentionWithoutWrites(t *testing.T) {
	dsn := os.Getenv("GIZCLAW_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("GIZCLAW_TEST_POSTGRES_DSN is required")
	}
	db, err := sqlx.ConnectContext(t.Context(), "postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	table := fmt.Sprintf("gzc_idle_history_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = db.Exec(`DROP TABLE IF EXISTS "` + table + `" CASCADE`)
		_, _ = db.Exec(`DROP TABLE IF EXISTS "` + table + `_keys"`)
	})
	store, err := memory.NewSQLHistoryStore(t.Context(), db, table, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().UTC().Truncate(24 * time.Hour).Add(-23 * time.Hour)
	if err := store.Append(t.Context(), []memory.HistoryEntry{{ConversationID: "idle", ID: "expired", Role: memory.RoleUser, Text: "expired", CreatedAt: expiry.Add(-time.Hour), ExpiresAt: &expiry}}); err != nil {
		t.Fatal(err)
	}
	r := newHistoryRetention(store)
	r.interval = 10 * time.Millisecond
	r.start(t.Context())
	defer r.close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var rows int
		if err := db.GetContext(ctx, &rows, `SELECT count(*) FROM "`+table+`" WHERE id='expired'`); err != nil {
			t.Fatal(err)
		}
		if rows == 0 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("idle history partition was not reclaimed")
		case <-ticker.C:
		}
	}
	if _, err := store.Get(t.Context(), memory.HistoryKey{ConversationID: "idle", ID: "expired"}); err == nil {
		t.Fatal("expired history remained readable")
	}
}
