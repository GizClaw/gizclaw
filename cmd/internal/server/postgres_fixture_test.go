package server

import (
	"context"
	"crypto/rand"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

func isolatedPostgresDSN(t *testing.T, dsn string) string {
	t.Helper()
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		t.Fatal("PostgreSQL test URL required")
	}
	db, err := sqlx.ConnectContext(t.Context(), "postgres", dsn)
	if err != nil {
		t.Fatal("cannot connect PostgreSQL fixture")
	}
	schema := "server_test_" + strings.ToLower(rand.Text())
	if _, err := db.ExecContext(t.Context(), "CREATE SCHEMA "+pq.QuoteIdentifier(schema)); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		defer db.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = db.ExecContext(ctx, "DELETE FROM partman.part_config WHERE split_part(parent_table,'.',1)=$1", schema)
		if _, err := db.ExecContext(ctx, "DROP SCHEMA "+pq.QuoteIdentifier(schema)+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
