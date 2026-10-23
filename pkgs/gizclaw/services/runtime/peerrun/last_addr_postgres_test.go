package peerrun

import (
	"context"
	"crypto/rand"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

func TestPostgreSQLLastAddrPersistenceAndMigration(t *testing.T) {
	dsn := os.Getenv("GIZCLAW_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("GIZCLAW_TEST_POSTGRES_DSN not configured")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		t.Fatal("test PostgreSQL URL required")
	}
	admin, err := sqlx.ConnectContext(t.Context(), "postgres", dsn)
	if err != nil {
		t.Fatal("cannot connect PostgreSQL test database")
	}
	t.Cleanup(func() { _ = admin.Close() })
	schema := "peerrun_" + strings.ToLower(rand.Text())
	if _, err := admin.ExecContext(t.Context(), "CREATE SCHEMA "+pq.QuoteIdentifier(schema)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(ctx, "DROP SCHEMA "+pq.QuoteIdentifier(schema)+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err := sqlx.ConnectContext(t.Context(), "postgres", parsed.String())
	if err != nil {
		t.Fatal("cannot connect isolated test schema")
	}
	t.Cleanup(func() { _ = db.Close() })
	s := &Server{DB: db}
	if err := s.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `ALTER TABLE peer_runs DROP COLUMN last_addr`); err != nil {
		t.Fatal(err)
	}
	key := testPublicKey(t)
	if err := s.SetDebugMode(t.Context(), key, "readonly"); err != nil {
		t.Fatal(err)
	}
	if err := s.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetLastAddr(t.Context(), key); err != nil || got != "" {
		t.Fatalf("legacy address = %q, %v", got, err)
	}
	if err := s.RecordLastAddr(t.Context(), key, "198.51.100.1:1000"); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordLastAddr(t.Context(), key, "[2001:db8::2]:2000"); err != nil {
		t.Fatal(err)
	}
	if err := s.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	reopened := &Server{DB: db}
	if got, err := reopened.GetLastAddr(t.Context(), key); err != nil || got != "[2001:db8::2]:2000" {
		t.Fatalf("persisted replacement address = %q, %v", got, err)
	}
	if got, err := reopened.GetDebugMode(t.Context(), key); err != nil || got != "readonly" {
		t.Fatalf("address write changed debug mode = %q, %v", got, err)
	}
}
