package workspace

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

func workspacePostgresTestDB(t *testing.T, dsn string) *sqlx.DB {
	t.Helper()
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		t.Fatal("GIZCLAW_TEST_POSTGRES_DSN must be a PostgreSQL URL")
	}
	admin, err := sqlx.ConnectContext(t.Context(), "postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	schema := "workspace_" + strings.ToLower(rand.Text())
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
		t.Fatal(err)
	}
	db.SetMaxOpenConns(8)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := Initialize(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestWorkspaceSQLPostgresIdentityAndConditionalMutation(t *testing.T) {
	dsn := os.Getenv("GIZCLAW_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set GIZCLAW_TEST_POSTGRES_DSN for PostgreSQL integration")
	}
	testWorkspaceSQLIdentityAndConditionalMutation(t, workspacePostgresTestDB(t, dsn))
}

func TestWorkspaceSQLPostgresActivityRetirement(t *testing.T) {
	dsn := os.Getenv("GIZCLAW_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set GIZCLAW_TEST_POSTGRES_DSN for PostgreSQL integration")
	}
	testWorkspaceActivityRetirement(t, workspacePostgresTestDB(t, dsn))
}
