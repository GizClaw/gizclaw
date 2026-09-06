package logstore

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

func TestSQLLogQueryFiltersBeforePageLimit(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		store, _ := newSQLiteLog(t)
		testSQLLogQueryPushdown(t, store)
	})
	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv("GIZCLAW_TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("set GIZCLAW_TEST_POSTGRES_DSN for PostgreSQL integration")
		}
		db, err := sqlx.ConnectContext(t.Context(), "postgres", dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		store, err := NewSQLStoreWithDB(db, "log_pushdown_"+strings.ToLower(rand.Text()))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = store.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := db.ExecContext(ctx, "DROP TABLE "+store.quoted); err != nil {
				t.Error(err)
			}
		})
		testSQLLogQueryPushdown(t, store)
	})
}

func testSQLLogQueryPushdown(t *testing.T, store *SQLStore) {
	t.Helper()
	start := time.UnixMilli(1000).UTC()
	records := make([]Record, 1000)
	for i := range records {
		records[i] = Record{ID: fmt.Sprintf("row-%04d", i), Stream: "events", Kind: "message", Time: start.Add(time.Duration(i) * time.Microsecond), Message: "NEEDLE %_", Attributes: map[string]string{"request.id": "other"}}
		if i >= 990 {
			records[i].Message = "needle %_"
			records[i].Attributes["request.id"] = "target"
		}
	}
	if _, err := store.Append(t.Context(), records); err != nil {
		t.Fatal(err)
	}
	for _, matcher := range []AttributeMatcher{
		{Name: "request.id", Op: MatchEqual, Value: "target"},
		{Name: "request.id", Op: MatchNotEqual, Value: "other"},
		{Name: "request.id", Op: MatchExists},
		{Name: "missing", Op: MatchNotExists},
	} {
		query := Query{Start: start, End: start.Add(time.Second), Limit: 1, Order: OrderAsc, Text: "needle %_", Matchers: []AttributeMatcher{matcher}}
		// Execute the generated SQL directly: the database must return only the
		// two matching rows needed to establish this page and its continuation.
		statement, args, err := store.buildQuery(normalizeSQLQuery(query), nil, time.Now().UnixNano(), query.Limit+1)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := store.db.QueryContext(t.Context(), store.db.Rebind(statement), args...)
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for rows.Next() {
			count++
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil || count != 2 {
			t.Fatalf("SQL returned %d rows: %v", count, err)
		}
		page, err := store.Query(t.Context(), query)
		if err != nil || len(page.Records) != 1 || page.Records[0].ID != "row-0990" || !page.HasNext {
			t.Fatalf("filtered page: %+v, %v", page, err)
		}
		query.Cursor = page.NextCursor
		page, err = store.Query(t.Context(), query)
		if err != nil || len(page.Records) != 1 || page.Records[0].ID != "row-0991" {
			t.Fatalf("continuation: %+v, %v", page, err)
		}
	}
	query := Query{Start: start, End: start.Add(time.Second), Limit: 1, Order: OrderDesc, Matchers: []AttributeMatcher{{Name: "missing", Op: MatchNotEqual, Value: "other"}}}
	page, err := store.Query(t.Context(), query)
	if err != nil || len(page.Records) != 0 {
		t.Fatalf("missing attribute must not satisfy !=: %+v, %v", page, err)
	}
}
