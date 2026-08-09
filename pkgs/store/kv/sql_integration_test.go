package kv

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

func TestPostgreSQLStoreIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("GIZCLAW_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("GIZCLAW_TEST_POSTGRES_DSN is not set")
	}
	db, err := sqlx.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	table := fmt.Sprintf("gzc_kv_sql_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = db.Exec(`DROP TABLE IF EXISTS "` + table + `"`)
	})
	stores := make([]*SQL, 2)
	errorsByConstructor := make([]error, len(stores))
	start := make(chan struct{})
	var constructors sync.WaitGroup
	for index := range stores {
		constructors.Go(func() {
			<-start
			stores[index], errorsByConstructor[index] = NewSQLWithDB(db, table, nil)
		})
	}
	close(start)
	constructors.Wait()
	for index, err := range errorsByConstructor {
		if err != nil {
			t.Fatalf("constructor %d: %v", index, err)
		}
		t.Cleanup(func() { _ = stores[index].Close() })
	}
	first, second := stores[0], stores[1]
	guard := Entry{Key: Key{"guard"}, Value: []byte("winner")}
	if _, created, err := first.CreateIfAbsent(context.Background(), guard, nil); err != nil || !created {
		t.Fatalf("first CreateIfAbsent() = _, %v, %v", created, err)
	}
	if existing, created, err := second.CreateIfAbsent(context.Background(), Entry{Key: guard.Key, Value: []byte("loser")}, nil); err != nil || created || string(existing) != "winner" {
		t.Fatalf("second CreateIfAbsent() = %q, %v, %v", existing, created, err)
	}
	concurrentGuard := Key{"concurrent"}
	type result struct {
		existing []byte
		created  bool
		err      error
		value    string
	}
	results := make([]result, 2)
	start = make(chan struct{})
	var creators sync.WaitGroup
	for index, store := range stores {
		creators.Go(func() {
			<-start
			results[index].value = fmt.Sprintf("caller-%d", index)
			results[index].existing, results[index].created, results[index].err = store.CreateIfAbsent(
				context.Background(), Entry{Key: concurrentGuard, Value: []byte(results[index].value)}, nil,
			)
		})
	}
	close(start)
	creators.Wait()
	createdCount := 0
	winner := ""
	for _, result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.created {
			createdCount++
			winner = result.value
		}
	}
	if createdCount != 1 {
		t.Fatalf("concurrent created count = %d, results = %+v", createdCount, results)
	}
	for _, result := range results {
		if !result.created && string(result.existing) != winner {
			t.Fatalf("loser existing = %q, want winner %q", result.existing, winner)
		}
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("Close closed borrowed PostgreSQL pool: %v", err)
	}
}
