//go:build store_e2e

package store_e2e_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	stores "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/jmoiron/sqlx"
)

func TestClickHouseRootRegistry(t *testing.T) {
	physical, db := openClickHouse(t)
	tables := []string{uniqueTable("metrics")}
	cleanupClickHouseTables(t, db, tables...)
	registry, err := stores.New(map[string]stores.Config{
		"metrics": {Kind: stores.KindMetrics, Storage: "analytics", Table: tables[0]},
	}, physical)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	if _, err := registry.Metrics("metrics"); err != nil {
		t.Fatal(err)
	}
	if err := registry.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("logical Close() closed shared pool: %v", err)
	}
	for _, table := range tables {
		if _, err := db.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+table); err != nil {
			t.Fatalf("drop %s: %v", table, err)
		}
	}
	if err := physical.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.PingContext(context.Background()); err == nil {
		t.Fatal("physical Close() left ClickHouse pool open")
	}
}

func TestClickHouseMetrics(t *testing.T) {
	_, db := openClickHouse(t)
	table := uniqueTable("metrics")
	badTable := uniqueTable("metrics_bad")
	cleanupClickHouseTables(t, db, table, badTable)
	store, err := metrics.NewClickHouseStore(metrics.ClickHouseConfig{
		DSN: requiredEnvironment(t, "GIZCLAW_TEST_CLICKHOUSE_DSN"), Table: table,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()
	selector := metrics.Selector{Name: "m", Matchers: []metrics.LabelMatcher{{Name: "peer", Op: metrics.MatchEqual, Value: "a"}}}
	if err := store.Append(ctx, []metrics.Sample{
		{Name: "m", Labels: map[string]string{"peer": "a"}, Timestamp: base, Value: 1},
		{Name: "m", Labels: map[string]string{"peer": "a"}, Timestamp: base.Add(time.Minute), Value: 3},
	}); err != nil {
		t.Fatal(err)
	}
	latest, err := store.Latest(ctx, metrics.LatestQuery{Selector: selector, At: base.Add(time.Minute), Lookback: 2 * time.Minute})
	if err != nil || len(latest) != 1 || latest[0].Points[0].Value != 3 {
		t.Fatalf("Latest() = %+v, %v", latest, err)
	}
	ranged, err := store.Range(ctx, metrics.RangeQuery{Selector: selector, Start: base, End: base.Add(90 * time.Second), Step: time.Minute})
	want := []metrics.Point{{Timestamp: base, Value: 1}, {Timestamp: base.Add(time.Minute), Value: 3}, {Timestamp: base.Add(90 * time.Second), Value: 3}}
	if err != nil || len(ranged) != 1 || !reflect.DeepEqual(ranged[0].Points, want) {
		t.Fatalf("Range() = %+v, %v; want %+v", ranged, err, want)
	}
	for _, operation := range []metrics.Aggregation{metrics.AggregationAvg, metrics.AggregationCount} {
		aggregated, err := store.Aggregate(ctx, metrics.AggregateQuery{
			Selector: selector, Start: base, End: base.Add(time.Minute), Bucket: time.Minute, Operation: operation,
		})
		if err != nil || len(aggregated) != 1 || aggregated[0].Points[0].Value != 2 {
			t.Fatalf("Aggregate(%s) = %+v, %v", operation, aggregated, err)
		}
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE "+badTable+" (metric String) ENGINE=MergeTree ORDER BY metric"); err != nil {
		t.Fatal(err)
	}
	if bad, err := metrics.NewClickHouseStore(metrics.ClickHouseConfig{
		DSN: requiredEnvironment(t, "GIZCLAW_TEST_CLICKHOUSE_DSN"), Table: badTable,
	}); err == nil {
		_ = bad.Close()
		t.Fatal("incompatible schema was accepted")
	}
}

func openClickHouse(t *testing.T) (*storage.Storage, *sqlx.DB) {
	t.Helper()
	physical, err := storage.New(map[string]storage.Config{
		"analytics": storage.ClickHouseConfig{DSN: requiredEnvironment(t, "GIZCLAW_TEST_CLICKHOUSE_DSN")},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	db, err := physical.SQL("analytics")
	if err != nil {
		t.Fatal(err)
	}
	return physical, db
}

func cleanupClickHouseTables(t *testing.T, db *sqlx.DB, tables ...string) {
	t.Helper()
	t.Cleanup(func() {
		for _, table := range tables {
			_, _ = db.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+table)
		}
	})
}
