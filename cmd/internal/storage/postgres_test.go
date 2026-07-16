package storage

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestPostgresSQLStorage(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("GIZCLAW_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("GIZCLAW_TEST_POSTGRES_DSN is not set")
	}

	registry, err := New(map[string]Config{
		"postgres": {Kind: KindSQL, Postgres: &SQLConfig{DSN: dsn}},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = registry.Close() })

	db, err := registry.SQL("postgres")
	if err != nil {
		t.Fatalf("SQL(postgres) error = %v", err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("PingContext() error = %v", err)
	}
	if got := db.DriverName(); got != "postgres" {
		t.Fatalf("DriverName() = %q, want postgres", got)
	}
	if got := db.Rebind("SELECT ?"); got != "SELECT $1" {
		t.Fatalf("Rebind() = %q, want %q", got, "SELECT $1")
	}
	if again, err := registry.SQL("postgres"); err != nil || again != db {
		t.Fatalf("SQL(postgres) second lookup = %p, %v; want %p", again, err, db)
	}
	if err := registry.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := db.PingContext(context.Background()); err == nil {
		t.Fatal("PingContext() after Close() error = nil")
	}
	if err := registry.Close(); err != nil {
		t.Fatalf("Close() second call error = %v", err)
	}
}
