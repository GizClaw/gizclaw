package metrics

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/internal/sqlbackend"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func newSQLiteMetrics(t *testing.T) (*SQLStore, *sqlx.DB) {
	t.Helper()
	db := sqlx.MustOpen("sqlite", ":memory:")
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewSQLStoreWithDB(db, "metric_samples")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, db
}

func TestSQLMetricsQueriesAndPrecision(t *testing.T) {
	store, _ := newSQLiteMetrics(t)
	ctx := context.Background()
	start := time.Unix(100, 123).UTC()
	labels := map[string]string{"host": "one"}
	values := []Sample{
		{Name: "cpu", Labels: labels, Timestamp: start, Value: math.Inf(1)},
		{Name: "cpu", Labels: labels, Timestamp: start.Add(time.Nanosecond), Value: 2},
		{Name: "cpu", Labels: labels, Timestamp: start.Add(time.Nanosecond), Value: 3},
	}
	if err := store.Append(ctx, values); err != nil {
		t.Fatal(err)
	}
	latest, err := store.Latest(ctx, LatestQuery{Selector: Selector{Name: "cpu"}, At: start.Add(time.Nanosecond), Lookback: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if len(latest) != 1 || len(latest[0].Points) != 1 || latest[0].Points[0].Value != 3 || !latest[0].Points[0].Timestamp.Equal(start.Add(time.Nanosecond)) {
		t.Fatalf("Latest() = %+v", latest)
	}
	aggregate, err := store.Aggregate(ctx, AggregateQuery{Selector: Selector{Name: "cpu"}, Start: start, End: start.Add(time.Nanosecond), Bucket: time.Second, Operation: AggregationCount})
	if err != nil {
		t.Fatal(err)
	}
	if len(aggregate) != 1 || aggregate[0].Points[0].Value != 3 {
		t.Fatalf("Aggregate() = %+v", aggregate)
	}
}

func TestSQLMetricsAtomicValidationAndClose(t *testing.T) {
	store, db := newSQLiteMetrics(t)
	ctx := context.Background()
	err := store.Append(ctx, []Sample{
		{Name: "valid", Timestamp: time.Now(), Value: 1},
		{Name: "bad name", Timestamp: time.Now(), Value: 2},
	})
	if err == nil {
		t.Fatal("Append accepted invalid batch")
	}
	got, err := store.Latest(ctx, LatestQuery{Selector: Selector{Name: "valid"}, At: time.Now().Add(time.Second), Lookback: time.Minute})
	if err != nil || len(got) != 0 {
		t.Fatalf("Latest after rejected batch = %+v, %v", got, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(ctx, nil); err == nil {
		t.Fatal("closed Store accepted Append")
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("Close closed borrowed pool: %v", err)
	}
}

func TestSQLMetricsSelectorsBoundariesCancellationAndSchema(t *testing.T) {
	store, db := newSQLiteMetrics(t)
	ctx := context.Background()
	start := time.Unix(200, 100).UTC()
	if err := store.Append(ctx, []Sample{
		{Name: "cpu", Labels: nil, Timestamp: start, Value: math.Float64frombits(0x7ff8000000000042)},
		{Name: "cpu", Labels: map[string]string{}, Timestamp: start.Add(time.Nanosecond), Value: math.Inf(-1)},
		{Name: "cpu", Labels: map[string]string{"host": "two"}, Timestamp: start.Add(2 * time.Nanosecond), Value: 4},
	}); err != nil {
		t.Fatal(err)
	}
	latest, err := store.Latest(ctx, LatestQuery{
		Selector: Selector{Name: "cpu", Matchers: []LabelMatcher{{Name: "host", Op: MatchRegexp, Value: ""}}},
		At:       start.Add(time.Nanosecond), Lookback: time.Nanosecond,
	})
	if err != nil || len(latest) != 1 || len(latest[0].Labels) != 0 || !math.IsInf(latest[0].Points[0].Value, -1) {
		t.Fatalf("Latest(empty labels) = %+v, %v", latest, err)
	}
	ranged, err := store.Range(ctx, RangeQuery{Selector: Selector{Name: "cpu"}, Start: start, End: start.Add(2 * time.Nanosecond), Step: time.Nanosecond})
	if err != nil || len(ranged) != 2 || len(ranged[0].Points) != 2 || len(ranged[1].Points) != 1 {
		t.Fatalf("Range() = %+v, %v", ranged, err)
	}
	if bits := math.Float64bits(ranged[0].Points[0].Value); bits != 0x7ff8000000000042 {
		t.Fatalf("Range() NaN bits = %#x", bits)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Latest(canceled, LatestQuery{Selector: Selector{Name: "cpu"}, At: start, Lookback: time.Second}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Latest(canceled) error = %v", err)
	}
	index, err := sqlbackend.IndexName(store.backend, "metric_idx")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP INDEX "` + index + `"`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE INDEX "` + index + `" ON "metric_samples" (metric)`); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSQLStoreWithDB(db, "metric_samples"); err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("NewSQLStoreWithDB() error = %v", err)
	}
}

func TestSQLMetricsRejectsOutOfRangeTime(t *testing.T) {
	store, _ := newSQLiteMetrics(t)
	err := store.Append(context.Background(), []Sample{{
		Name: "cpu", Timestamp: time.Date(2500, 1, 1, 0, 0, 0, 0, time.UTC), Value: 1,
	}})
	if err == nil || !strings.Contains(err.Error(), "outside signed nanosecond range") {
		t.Fatalf("Append() error = %v", err)
	}
}
