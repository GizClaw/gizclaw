package metrics

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

func TestPostgreSQLMetricsIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("GIZCLAW_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("GIZCLAW_TEST_POSTGRES_DSN is not set")
	}
	db, err := sqlx.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	table := fmt.Sprintf("gzc_metrics_sql_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = db.Exec(`DROP TABLE IF EXISTS "` + table + `"`)
	})
	store, err := NewSQLStoreWithDB(db, table)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Unix(100, 123).UTC()
	if err := store.Append(context.Background(), []Sample{
		{Name: "cpu", Labels: map[string]string{"host": "one"}, Timestamp: now, Value: 1},
		{Name: "cpu", Labels: map[string]string{"host": "one"}, Timestamp: now, Value: 2},
	}); err != nil {
		t.Fatal(err)
	}
	latest, err := store.Latest(context.Background(), LatestQuery{Selector: Selector{Name: "cpu"}, At: now, Lookback: time.Second})
	if err != nil || len(latest) != 1 || latest[0].Points[0].Value != 2 {
		t.Fatalf("Latest() = %+v, %v", latest, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("Close closed borrowed PostgreSQL pool: %v", err)
	}
}
