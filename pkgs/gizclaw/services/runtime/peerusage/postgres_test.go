package peerusage

import (
	"context"
	"crypto/rand"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

func TestPostgreSQLDailyPartitionsAndIdempotentWriters(t *testing.T) {
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
	schema := "peerusage_" + strings.ToLower(rand.Text())
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
	dsn = parsed.String()
	db, err := sqlx.ConnectContext(t.Context(), "postgres", dsn)
	if err != nil {
		t.Fatal("cannot connect isolated test schema")
	}
	defer db.Close()
	s, err := NewStore(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	hour := time.Now().UTC().Truncate(time.Hour)
	peer := giznet.PublicKey{9}
	item := Snapshot{Peer: peer, ModelID: "m", Hour: hour, WriterID: "one", Quantity: 11}
	if err := s.Write(t.Context(), []Snapshot{item, item}); err != nil {
		t.Fatal(err)
	}
	other, err := sqlx.ConnectContext(t.Context(), "postgres", dsn)
	if err != nil {
		t.Fatal("cannot connect second test writer")
	}
	defer other.Close()
	s2, err := NewStore(t.Context(), other)
	if err != nil {
		t.Fatal(err)
	}
	item.WriterID = "two"
	item.Quantity = 7
	if err := s2.Write(t.Context(), []Snapshot{item}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Query(t.Context(), peer, "m", hour, hour.Add(time.Hour))
	if err != nil || len(rows) != 1 || rows[0].Quantity != 18 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	var children int
	if err := db.GetContext(t.Context(), &children, "SELECT count(*) FROM pg_inherits JOIN pg_class p ON p.oid=inhparent WHERE p.relname=$1 AND p.relnamespace=current_schema()::regnamespace", TableName); err != nil {
		t.Fatal(err)
	}
	if children < 2 {
		t.Fatalf("children=%d", children)
	}
	now := hour.Add(Retention + 48*time.Hour)
	s.Now = func() time.Time { return now }
	if err := s.Maintain(t.Context()); err != nil {
		t.Fatal(err)
	}
	var oldExists bool
	name := TableName + "_p" + hour.Format("20060102")
	if err := db.GetContext(t.Context(), &oldExists, "SELECT EXISTS(SELECT 1 FROM pg_class WHERE relname=$1 AND relnamespace=current_schema()::regnamespace)", name); err != nil {
		t.Fatal(err)
	}
	if oldExists {
		t.Fatal("expired partition retained")
	}
	rows, err = s.Query(t.Context(), peer, "", hour, now.Add(time.Hour))
	if err != nil || len(rows) != 0 {
		t.Fatalf("expired rows=%+v err=%v", rows, err)
	}
}
