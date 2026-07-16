//go:build integration

package logstore

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestClickHouseStoreIntegration(t *testing.T) {
	dsn := os.Getenv("CLICKHOUSE_TEST_DSN")
	if dsn == "" {
		t.Skip("CLICKHOUSE_TEST_DSN is not set")
	}
	store, err := NewClickHouseStore(ClickHouseConfig{
		DSN:      dsn,
		Database: os.Getenv("CLICKHOUSE_TEST_DATABASE"),
		Table:    "gizclaw_logstore_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, "TRUNCATE TABLE "+store.qualified); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = store.db.ExecContext(context.Background(), "TRUNCATE TABLE "+store.qualified)
	})

	base := time.Unix(1_700_000_000, 123_000_000).UTC()
	records := []Record{
		{
			ID: "a", Time: base, Stream: "history", Kind: "message",
			Attributes: map[string]string{"workspace": "one"}, Payload: []byte("{\"value\":1}"),
		},
		{
			ID: "b", Time: base, Stream: "history", Kind: "message",
			Attributes: map[string]string{"workspace": "one"}, Payload: []byte("{\"value\":2}"),
		},
		{
			ID: "c", Time: base.Add(time.Millisecond), Stream: "history", Kind: "message",
			Attributes: map[string]string{"workspace": "two"}, Payload: []byte("{\"value\":3}"),
		},
	}
	keys, err := store.Append(ctx, records)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != len(records) || keys[0] != records[0].Key() || keys[2] != records[2].Key() {
		t.Fatalf("keys = %+v", keys)
	}

	query := Query{
		Streams: []string{"history"},
		Kinds:   []string{"message"},
		Matchers: []AttributeMatcher{
			{Name: "workspace", Op: MatchEqual, Value: "one"},
		},
		Start: base.Truncate(time.Millisecond),
		End:   base.Add(time.Second).Truncate(time.Millisecond),
		Limit: 1,
		Order: OrderAsc,
	}
	first, err := store.Query(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Records) != 1 || first.Records[0].ID != "a" || !first.HasNext {
		t.Fatalf("first page = %+v", first)
	}
	query.Cursor = first.NextCursor
	second, err := store.Query(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Records) != 1 || second.Records[0].ID != "b" || second.HasNext {
		t.Fatalf("second page = %+v", second)
	}
	query.Cursor = ""
	query.Order = OrderDesc
	latest, err := store.Query(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(latest.Records) != 1 || latest.Records[0].ID != "b" || !latest.HasNext {
		t.Fatalf("latest page = %+v", latest)
	}
	query.Cursor = latest.NextCursor
	oldest, err := store.Query(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(oldest.Records) != 1 || oldest.Records[0].ID != "a" || oldest.HasNext {
		t.Fatalf("oldest page = %+v", oldest)
	}

	replacement := records[0]
	replacement.Payload = []byte("{\"value\":10}")
	if err := store.Replace(ctx, replacement); err != nil {
		t.Fatal(err)
	}
	if err := store.Replace(ctx, Record{
		ID: "missing", Time: base, Stream: "history", Kind: "message",
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing replace error = %v", err)
	}
	query.Cursor = ""
	query.Limit = 10
	query.Order = OrderAsc
	page, err := store.Query(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if string(page.Records[0].Payload) != string(replacement.Payload) {
		t.Fatalf("replacement payload = %s", page.Records[0].Payload)
	}
	if err := store.Delete(ctx, records[1].Key()); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, records[1].Key()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete error = %v", err)
	}
	page, err = store.Query(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 1 || page.Records[0].ID != "a" {
		t.Fatalf("page after delete = %+v", page)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.Query(canceled, query); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled query error = %v", err)
	}
}
