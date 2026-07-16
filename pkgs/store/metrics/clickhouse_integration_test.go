//go:build integration

package metrics

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestClickHouseStoreIntegration(t *testing.T) {
	dsn := os.Getenv("CLICKHOUSE_TEST_DSN")
	if dsn == "" {
		t.Skip("CLICKHOUSE_TEST_DSN is not set")
	}
	store, err := NewClickHouseStore(ClickHouseConfig{DSN: dsn, Table: "gizclaw_metrics_test"})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, "TRUNCATE TABLE gizclaw_metrics_test"); err != nil {
		t.Fatal(err)
	}
	base := time.Unix(1_700_000_000, 0).UTC()
	selector := Selector{Name: "m", Matchers: []LabelMatcher{{Name: "peer", Op: MatchEqual, Value: "a"}}}
	if err := store.Append(ctx, []Sample{{Name: "m", Labels: map[string]string{"peer": "a"}, Timestamp: base, Value: 1}, {Name: "m", Labels: map[string]string{"peer": "a"}, Timestamp: base.Add(time.Minute), Value: 3}}); err != nil {
		t.Fatal(err)
	}
	latest, err := store.Latest(ctx, LatestQuery{Selector: selector, At: base.Add(time.Minute), Lookback: 2 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if len(latest) != 1 || latest[0].Points[0].Value != 3 {
		t.Fatalf("latest=%+v", latest)
	}
	ranged, err := store.Range(ctx, RangeQuery{Selector: selector, Start: base, End: base.Add(90 * time.Second), Step: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if want := []Point{{Timestamp: base, Value: 1}, {Timestamp: base.Add(time.Minute), Value: 3}, {Timestamp: base.Add(90 * time.Second), Value: 3}}; len(ranged) != 1 || !pointsEqual(ranged[0].Points, want) {
		t.Fatalf("range=%+v want=%+v", ranged, want)
	}
	agg, err := store.Aggregate(ctx, AggregateQuery{Selector: selector, Start: base, End: base.Add(time.Minute), Bucket: time.Minute, Operation: AggregationAvg})
	if err != nil {
		t.Fatal(err)
	}
	if len(agg) != 1 || agg[0].Points[0].Value != 2 {
		t.Fatalf("aggregate=%+v", agg)
	}
	counted, err := store.Aggregate(ctx, AggregateQuery{Selector: selector, Start: base, End: base.Add(time.Minute), Bucket: time.Minute, Operation: AggregationCount})
	if err != nil {
		t.Fatal(err)
	}
	if len(counted) != 1 || counted[0].Points[0].Value != 2 {
		t.Fatalf("count aggregate=%+v", counted)
	}
	if _, err := store.db.ExecContext(ctx, "DROP TABLE IF EXISTS gizclaw_metrics_bad"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, "CREATE TABLE gizclaw_metrics_bad (metric String) ENGINE=MergeTree ORDER BY metric"); err != nil {
		t.Fatal(err)
	}
	if bad, err := NewClickHouseStore(ClickHouseConfig{DSN: dsn, Table: "gizclaw_metrics_bad"}); err == nil {
		bad.Close()
		t.Fatal("expected incompatible schema error")
	}
}
