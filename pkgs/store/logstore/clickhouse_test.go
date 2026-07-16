package logstore

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestClickHouseConfigValidation(t *testing.T) {
	tests := []ClickHouseConfig{
		{},
		{DSN: "clickhouse://localhost", Table: "bad-name"},
		{DSN: "clickhouse://localhost", Database: "bad-name", Table: "logs"},
	}
	for _, config := range tests {
		if store, err := NewClickHouseStore(config); err == nil {
			_ = store.Close()
			t.Fatalf("NewClickHouseStore(%+v) unexpectedly succeeded", config)
		}
	}
}

func TestClickHouseCursorBindsQueryButNotLimit(t *testing.T) {
	query := Query{
		Streams:    []string{"system", "history"},
		Kinds:      []string{"message"},
		Severities: []string{"WARN"},
		Matchers: []AttributeMatcher{
			{Name: "workspace_name", Op: MatchEqual, Value: "a"},
			{Name: "optional", Op: MatchExists, Value: "ignored"},
		},
		Text:  "needle",
		Start: time.UnixMilli(1),
		End:   time.UnixMilli(20),
		Limit: 10,
		Order: OrderDesc,
	}
	bound := normalizeClickHouseQuery(query)
	cursorValue, err := encodeClickHouseCursor(clickHouseCursor{
		Version: 1,
		Query:   bound,
		Position: clickHousePosition{
			TimeUnixNano: time.UnixMilli(10).UnixNano(),
			Stream:       "history",
			ID:           "id",
		},
	})
	if err != nil {
		t.Fatalf("encodeClickHouseCursor() error = %v", err)
	}
	cursor, err := decodeClickHouseCursor(cursorValue)
	if err != nil {
		t.Fatalf("decodeClickHouseCursor() error = %v", err)
	}
	query.Limit = 1
	query.Streams = []string{"history", "system"}
	query.Matchers[1].Value = "changed-but-ignored"
	if got := normalizeClickHouseQuery(query); !equalClickHouseQuery(cursor.Query, got) {
		t.Fatalf("cursor query = %+v, normalized = %+v", cursor.Query, got)
	}
	query.Text = "changed"
	if equalClickHouseQuery(cursor.Query, normalizeClickHouseQuery(query)) {
		t.Fatal("changed query matched cursor")
	}
	if _, err := decodeClickHouseCursor("not-base64"); !errors.Is(err, ErrCursorMismatch) {
		t.Fatalf("decode malformed cursor error = %v", err)
	}
}

func TestClickHouseCursorSupportsUnixEpochPosition(t *testing.T) {
	value, err := encodeClickHouseCursor(clickHouseCursor{
		Version:  1,
		Query:    normalizeClickHouseQuery(Query{Start: time.UnixMilli(0), End: time.UnixMilli(1), Limit: 1, Order: OrderAsc}),
		Position: clickHousePosition{Stream: "history", ID: "epoch"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeClickHouseCursor(value); err != nil {
		t.Fatalf("decode epoch cursor error = %v", err)
	}
}

func TestBuildClickHouseWhereTranslatesSelectorsAndCursor(t *testing.T) {
	bound := normalizeClickHouseQuery(Query{
		Streams:    []string{"b", "a"},
		Kinds:      []string{"message"},
		Severities: []string{"WARN"},
		Matchers: []AttributeMatcher{
			{Name: "equal", Op: MatchEqual, Value: "1"},
			{Name: "different", Op: MatchNotEqual, Value: "2"},
			{Name: "present", Op: MatchExists},
			{Name: "missing", Op: MatchNotExists},
		},
		Text:  "needle",
		Start: time.UnixMilli(1),
		End:   time.UnixMilli(20),
		Limit: 2,
		Order: OrderDesc,
	})
	position := &clickHousePosition{TimeUnixNano: time.UnixMilli(10).UnixNano(), Stream: "history", ID: "id"}
	where, args := buildClickHouseWhere(bound, position)
	for _, fragment := range []string{
		"stream IN (?,?)",
		"kind IN (?)",
		"severity IN (?)",
		"position(message, ?) > 0",
		"attributes[?] = ?",
		"mapContains(attributes, ?) AND attributes[?] != ?",
		"mapContains(attributes, ?)",
		"NOT mapContains(attributes, ?)",
		"timestamp < ?",
		"stream < ?",
		"id < ?",
	} {
		if !strings.Contains(where, fragment) {
			t.Fatalf("where = %q, missing %q", where, fragment)
		}
	}
	if len(args) != 19 {
		t.Fatalf("args = %#v, want 19 values", args)
	}
	if got := args[2:4]; !reflect.DeepEqual(got, []any{"a", "b"}) {
		t.Fatalf("stream args = %#v, want sorted selectors", got)
	}
}

func TestQuoteClickHouseIdentifier(t *testing.T) {
	if got := quoteClickHouseIdentifier("logs"); got != "\"logs\"" {
		t.Fatalf("quoteClickHouseIdentifier() = %q", got)
	}
}
