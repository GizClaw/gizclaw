//go:build store_e2e

package store_e2e_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	stores "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/jmoiron/sqlx"
)

func TestClickHouseRootRegistry(t *testing.T) {
	physical, db := openClickHouse(t)
	tables := []string{uniqueTable("metrics"), uniqueTable("immutable"), uniqueTable("mutable")}
	cleanupClickHouseTables(t, db, tables...)
	registry, err := stores.New(map[string]stores.Config{
		"metrics": {Kind: stores.KindMetrics, Storage: "analytics", Table: tables[0]},
		"audit":   {Kind: stores.KindLogImmutable, Storage: "analytics", Table: tables[1]},
		"history": {Kind: stores.KindLogMutable, Storage: "analytics", Table: tables[2]},
	}, physical)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	if _, err := registry.Metrics("metrics"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Log("audit"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.MutableLog("audit"); err == nil {
		t.Fatal("immutable Log declaration exposed mutable access")
	}
	if _, err := registry.MutableLog("history"); err != nil {
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

func TestClickHouseLog(t *testing.T) {
	_, db := openClickHouse(t)
	table := uniqueTable("logs")
	cleanupClickHouseTables(t, db, table)
	store, err := logstore.NewClickHouseStore(logstore.ClickHouseConfig{
		DSN: requiredEnvironment(t, "GIZCLAW_TEST_CLICKHOUSE_DSN"), Table: table,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 123_000_000).UTC()
	records := []logstore.Record{
		{ID: "a", Time: base, Stream: "history", Kind: "message", Attributes: map[string]string{"workspace": "one"}, Payload: []byte(`{"value":1}`)},
		{ID: "b", Time: base, Stream: "history", Kind: "message", Attributes: map[string]string{"workspace": "one"}, Payload: []byte(`{"value":2}`)},
		{ID: "c", Time: base.Add(time.Millisecond), Stream: "history", Kind: "message", Attributes: map[string]string{"workspace": "two"}, Payload: []byte(`{"value":3}`)},
		{ID: "a", Time: base, Stream: "other-history", Kind: "message", Payload: []byte(`{"value":4}`)},
	}
	keys, err := store.Append(ctx, records)
	if err != nil || len(keys) != len(records) || keys[0] != records[0].Key() {
		t.Fatalf("Append() = %+v, %v", keys, err)
	}
	got, err := store.Get(ctx, records[0].Key())
	if err != nil || got.Key() != records[0].Key() || string(got.Payload) != string(records[0].Payload) {
		t.Fatalf("Get() = %+v, %v", got, err)
	}
	if _, err := store.Get(ctx, logstore.RecordKey{Stream: "history", ID: "missing"}); !errors.Is(err, logstore.ErrNotFound) {
		t.Fatalf("missing Get() error = %v", err)
	}
	notEqual, err := store.Query(ctx, logstore.Query{
		Streams:  []string{"history", "other-history"},
		Matchers: []logstore.AttributeMatcher{{Name: "workspace", Op: logstore.MatchNotEqual, Value: "one"}},
		Start:    base.Truncate(time.Millisecond),
		End:      base.Add(time.Second).Truncate(time.Millisecond),
		Limit:    10,
		Order:    logstore.OrderAsc,
	})
	if err != nil || len(notEqual.Records) != 1 || notEqual.Records[0].Key() != records[2].Key() {
		t.Fatalf("not-equal Query() = %+v, %v", notEqual, err)
	}
	query := logstore.Query{
		Streams: []string{"history"}, Kinds: []string{"message"},
		Matchers: []logstore.AttributeMatcher{{Name: "workspace", Op: logstore.MatchEqual, Value: "one"}},
		Start:    base.Truncate(time.Millisecond), End: base.Add(time.Second).Truncate(time.Millisecond), Limit: 1, Order: logstore.OrderAsc,
	}
	first, err := store.Query(ctx, query)
	if err != nil || len(first.Records) != 1 || first.Records[0].ID != "a" || !first.HasNext {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	query.Cursor = first.NextCursor
	second, err := store.Query(ctx, query)
	if err != nil || len(second.Records) != 1 || second.Records[0].ID != "b" || second.HasNext {
		t.Fatalf("second page = %+v, %v", second, err)
	}
	query.Cursor = ""
	query.Order = logstore.OrderDesc
	latest, err := store.Query(ctx, query)
	if err != nil || len(latest.Records) != 1 || latest.Records[0].ID != "b" || !latest.HasNext {
		t.Fatalf("latest page = %+v, %v", latest, err)
	}
	query.Cursor = latest.NextCursor
	oldest, err := store.Query(ctx, query)
	if err != nil || len(oldest.Records) != 1 || oldest.Records[0].ID != "a" || oldest.HasNext {
		t.Fatalf("oldest page = %+v, %v", oldest, err)
	}
	replacement := records[0]
	replacement.Payload = []byte(`{"value":10}`)
	if err := store.Replace(ctx, replacement); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Get(ctx, replacement.Key()); err != nil || string(got.Payload) != string(replacement.Payload) {
		t.Fatalf("Get(replaced) = %+v, %v", got, err)
	}
	if err := store.Replace(ctx, logstore.Record{ID: "missing", Time: base, Stream: "history", Kind: "message"}); !errors.Is(err, logstore.ErrNotFound) {
		t.Fatalf("missing Replace() error = %v", err)
	}
	query.Cursor = ""
	query.Limit = 10
	query.Order = logstore.OrderAsc
	replaced, err := store.Query(ctx, query)
	if err != nil || len(replaced.Records) == 0 || string(replaced.Records[0].Payload) != string(replacement.Payload) {
		t.Fatalf("replacement Query() = %+v, %v", replaced, err)
	}
	if err := store.Delete(ctx, records[1].Key()); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, records[1].Key()); !errors.Is(err, logstore.ErrNotFound) {
		t.Fatalf("second Delete() error = %v", err)
	}
	crossStream := logstore.Query{
		Streams: []string{"history", "other-history"},
		Start:   base.Truncate(time.Millisecond),
		End:     base.Add(time.Second).Truncate(time.Millisecond),
		Limit:   1,
		Order:   logstore.OrderAsc,
	}
	var crossStreamKeys []logstore.RecordKey
	for {
		page, err := store.Query(ctx, crossStream)
		if err != nil {
			t.Fatal(err)
		}
		for _, record := range page.Records {
			crossStreamKeys = append(crossStreamKeys, record.Key())
		}
		if !page.HasNext {
			break
		}
		crossStream.Cursor = page.NextCursor
	}
	if len(crossStreamKeys) != 3 || crossStreamKeys[0] != records[0].Key() ||
		crossStreamKeys[1] != records[3].Key() || crossStreamKeys[2] != records[2].Key() {
		t.Fatalf("cross-stream keys = %+v", crossStreamKeys)
	}
	if _, err := store.Append(ctx, []logstore.Record{records[0]}); err == nil {
		t.Fatal("duplicate Append() succeeded")
	}
	concurrent := logstore.Record{ID: "concurrent", Time: base.Add(2 * time.Millisecond), Stream: "history", Kind: "message"}
	start := make(chan struct{})
	errorsByCall := make([]error, 2)
	var wait sync.WaitGroup
	for index := range errorsByCall {
		wait.Go(func() {
			<-start
			_, errorsByCall[index] = store.Append(ctx, []logstore.Record{concurrent})
		})
	}
	close(start)
	wait.Wait()
	successes := 0
	for _, err := range errorsByCall {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent Append() errors = %+v", errorsByCall)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.Query(canceled, query); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Query() error = %v", err)
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
