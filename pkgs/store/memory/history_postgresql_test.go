package memory

import (
	"crypto/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

func TestPostgreSQLHistoryColumnsAndPartmanRetention(t *testing.T) {
	dsn := os.Getenv("GIZCLAW_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set GIZCLAW_TEST_POSTGRES_DSN for pg_partman integration")
	}
	db, err := sqlx.ConnectContext(t.Context(), "postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	schema := "history_" + strings.ToLower(rand.Text())
	if _, err := db.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("SET search_path TO " + schema + ",public"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM partman.part_config WHERE parent_table=$1", schema+".messages")
		_, _ = db.Exec("DROP SCHEMA " + schema + " CASCADE")
	})
	store, err := NewSQLHistoryStore(t.Context(), db, "messages", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	expires := now.Add(time.Hour)
	entry := HistoryEntry{ConversationID: "a", ID: "one", Role: RoleUser, Text: "hello", CreatedAt: now, ExpiresAt: &expires, Assets: []HistoryAsset{{Name: "audio/one.ogg", MIMEType: "audio/ogg", Bytes: 42}}}
	if err := store.Append(t.Context(), []HistoryEntry{entry}); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(t.Context(), entry.Key())
	if err != nil || got.Text != "hello" || len(got.Assets) != 1 || !got.ExpiresAt.Equal(expires) {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	var epoch, interval, retention string
	var keep bool
	if err := db.QueryRow("SELECT epoch,partition_interval,retention,retention_keep_table FROM partman.part_config WHERE parent_table=$1", schema+".messages").Scan(&epoch, &interval, &retention, &keep); err != nil {
		t.Fatal(err)
	}
	if epoch != "nanoseconds" || interval != "1 day" || retention != "0 seconds" || keep {
		t.Fatalf("partman=%s %s %s keep=%t", epoch, interval, retention, keep)
	}
	// Expired partitions and identity/asset rows are physically reclaimed even
	// without another Append. Use an explicit old day to test partition removal.
	old := now.Add(-72 * time.Hour)
	oldEntry := entry
	oldEntry.ID = "old"
	oldEntry.CreatedAt = old.Add(-time.Hour)
	oldEntry.ExpiresAt = &old
	if err := store.Append(t.Context(), []HistoryEntry{oldEntry}); err != nil {
		t.Fatal(err)
	}
	if err := store.Maintain(t.Context()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.Get(&count, "SELECT count(*) FROM messages WHERE id='old'"); err != nil || count != 0 {
		t.Fatalf("expired rows=%d err=%v", count, err)
	}
	if err := db.Get(&count, "SELECT count(*) FROM "+store.keys.Quoted()+" WHERE id='old'"); err != nil || count != 0 {
		t.Fatalf("expired identities=%d err=%v", count, err)
	}
	if err := db.Get(&count, "SELECT count(*) FROM "+store.assets.Quoted()+" WHERE id='old'"); err != nil || count != 0 {
		t.Fatalf("expired assets=%d err=%v", count, err)
	}
	if _, err := store.Get(t.Context(), entry.Key()); err != nil {
		t.Fatalf("live history lost: %v", err)
	}
}
