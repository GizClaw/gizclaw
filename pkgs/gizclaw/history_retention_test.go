package gizclaw

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
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
	store, err := logstore.NewSQLStoreWithDBAndTTL(db, table, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	today := time.Now().UTC().Truncate(24 * time.Hour)
	yesterday := today.AddDate(0, 0, -1)
	child := table + "_p" + yesterday.Format("20060102")
	if _, err := db.Exec(fmt.Sprintf(`CREATE TABLE "%s" PARTITION OF "%s" FOR VALUES FROM (%d) TO (%d)`, child, table, yesterday.UnixNano(), today.UnixNano())); err != nil {
		t.Fatal(err)
	}
	expiry := yesterday.Add(time.Hour).UnixNano()
	if _, err := db.Exec(`INSERT INTO "`+table+`_keys" (stream,id,expires_at_unix_nano) VALUES ('idle','expired',$1)`, expiry); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO "`+table+`" (stream,id,timestamp_unix_nano,expires_at_unix_nano,kind,severity,message,attributes_json,payload_json) VALUES ('idle','expired',1,$1,'message','','','{}',$2)`, expiry, []byte{}); err != nil {
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
		var childExists bool
		if err := db.GetContext(ctx, &childExists, `SELECT to_regclass(current_schema() || '.' || $1) IS NOT NULL`, child); err != nil {
			t.Fatal(err)
		}
		if !childExists {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("idle history partition was not reclaimed")
		case <-ticker.C:
		}
	}
	var keys int
	if err := db.Get(&keys, `SELECT COUNT(*) FROM "`+table+`_keys"`); err != nil || keys != 0 {
		t.Fatalf("idle history keys = %d, %v; want 0", keys, err)
	}
}
