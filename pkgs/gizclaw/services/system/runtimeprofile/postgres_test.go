package runtimeprofile

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

func TestPostgreSQLRuntimeProfileAppConfig(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("GIZCLAW_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("GIZCLAW_TEST_POSTGRES_DSN is not set")
	}
	t.Run("existing table", func(t *testing.T) {
		open := openProfilePostgresSchema(t, dsn)
		db := open()
		defer db.Close()
		testRuntimeProfileSQLAddsAppConfigToExistingTable(t, db)
	})
	t.Run("persistence", func(t *testing.T) {
		testRuntimeProfileSQLAppConfigPersistence(t, openProfilePostgresSchema(t, dsn))
	})
}

// Each subtest owns a schema; reconnects reuse it without touching other tests.
func openProfilePostgresSchema(t *testing.T, dsn string) func() *sqlx.DB {
	t.Helper()
	admin, err := sqlx.ConnectContext(t.Context(), "postgres", dsn)
	if err != nil {
		t.Fatal("connect to PostgreSQL test database")
	}
	t.Cleanup(func() { _ = admin.Close() })
	schema := "app_config_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.ExecContext(t.Context(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})
	return func() *sqlx.DB {
		db, err := sqlx.Open("postgres", dsn)
		if err != nil {
			t.Fatal("open PostgreSQL test database")
		}
		db.SetMaxOpenConns(1)
		if _, err := db.ExecContext(t.Context(), "SET search_path TO "+schema); err != nil {
			_ = db.Close()
			t.Fatalf("select test schema: %v", err)
		}
		return db
	}
}
