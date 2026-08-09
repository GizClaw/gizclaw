package logstore

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

func TestPostgreSQLLogIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("GIZCLAW_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("GIZCLAW_TEST_POSTGRES_DSN is not set")
	}
	db, err := sqlx.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	table := fmt.Sprintf("gzc_log_sql_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = db.Exec(`DROP TABLE IF EXISTS "` + table + `"`)
	})
	store, err := NewSQLStoreWithDB(db, table)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.UnixMilli(1000).UTC()
	record := Record{ID: "one", Stream: "events", Kind: "created", Time: now}
	if _, err := store.Append(context.Background(), []Record{record}); err != nil {
		t.Fatal(err)
	}
	record.Message = "updated"
	if err := store.Replace(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	second := Record{ID: "two", Stream: "events", Kind: "created", Time: now.Add(time.Millisecond)}
	if _, err := store.Append(context.Background(), []Record{second}); err != nil {
		t.Fatal(err)
	}
	page, err := store.Query(context.Background(), Query{Start: now, End: now.Add(time.Second), Limit: 10, Order: OrderAsc})
	if err != nil || len(page.Records) != 2 || page.Records[0].Message != "updated" {
		t.Fatalf("Query() = %+v, %v", page, err)
	}
	sqliteDB := sqlx.MustOpen("sqlite", ":memory:")
	sqliteDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqliteDB.Close() })
	sqliteStore, err := NewSQLStoreWithDB(sqliteDB, "cross_driver_logs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqliteStore.Close() })
	if _, err := sqliteStore.Append(context.Background(), []Record{record, second}); err != nil {
		t.Fatal(err)
	}
	query := Query{Start: now, End: now.Add(time.Second), Limit: 1, Order: OrderAsc}
	sqlitePage, err := sqliteStore.Query(context.Background(), query)
	if err != nil || !sqlitePage.HasNext || sqlitePage.Records[0].ID != "one" {
		t.Fatalf("SQLite first page = %+v, %v", sqlitePage, err)
	}
	query.Cursor = sqlitePage.NextCursor
	query.Limit = 10
	postgresPage, err := store.Query(context.Background(), query)
	if err != nil || len(postgresPage.Records) != 1 || postgresPage.Records[0].ID != "two" {
		t.Fatalf("PostgreSQL continuation = %+v, %v", postgresPage, err)
	}
	if err := store.Delete(context.Background(), record.Key()); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("Close closed borrowed PostgreSQL pool: %v", err)
	}
}
