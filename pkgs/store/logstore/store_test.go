package logstore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func validRecord() Record {
	return Record{ID: "id", Time: time.UnixMilli(1000).UTC(), Stream: "event", Kind: "created", Attributes: map[string]string{"request.method": "GET"}, Payload: json.RawMessage(`{"ok":true}`)}
}

func validQuery() Query {
	return Query{Streams: []string{"event"}, Start: time.UnixMilli(1000).UTC(), End: time.UnixMilli(2000).UTC(), Limit: 100, Order: OrderAsc}
}

func TestValidateRecord(t *testing.T) {
	if err := ValidateRecord(validRecord()); err != nil {
		t.Fatalf("ValidateRecord() error = %v", err)
	}
	for _, edit := range []func(*Record){
		func(r *Record) { r.ID = "" }, func(r *Record) { r.Time = time.Time{} },
		func(r *Record) { r.Stream = "" }, func(r *Record) { r.Kind = "" },
		func(r *Record) { r.Payload = json.RawMessage(`{`) },
		func(r *Record) { r.Attributes = map[string]string{"request": "x", "request.method": "GET"} },
		func(r *Record) { r.Attributes = map[string]string{"bad..name": "x"} },
	} {
		record := validRecord()
		edit(&record)
		if err := ValidateRecord(record); err == nil {
			t.Fatalf("ValidateRecord(%+v) error = nil", record)
		}
	}
}

func TestRecordKeyAndEmptyAppendContract(t *testing.T) {
	record := validRecord()
	if got, want := record.Key(), (RecordKey{Stream: record.Stream, ID: record.ID}); got != want {
		t.Fatalf("Record.Key() = %+v, want %+v", got, want)
	}
	if err := ValidateRecordKey(record.Key()); err != nil {
		t.Fatalf("ValidateRecordKey() error = %v", err)
	}
	for _, key := range []RecordKey{{ID: "id"}, {Stream: "stream"}} {
		if err := ValidateRecordKey(key); err == nil {
			t.Fatalf("ValidateRecordKey(%+v) error = nil", key)
		}
	}
	keys, err := (&VolcStore{}).Append(context.Background(), nil)
	if err != nil || len(keys) != 0 {
		t.Fatalf("empty Append() = %+v, %v", keys, err)
	}
	var immutable ImmutableStore = &VolcStore{}
	if _, mutable := immutable.(MutableStore); mutable {
		t.Fatal("VolcStore unexpectedly satisfies MutableStore")
	}
}

func TestValidateAttributeNameContract(t *testing.T) {
	for _, name := range []string{"a", "_request", "request.http-method", strings.Repeat("a", MaxAttributeNameBytes)} {
		if err := ValidateAttributeName(name); err != nil {
			t.Errorf("ValidateAttributeName(%q) error = %v", name, err)
		}
	}
	for _, name := range []string{"", ".a", "a.", "a..b", "1a", "a b", "中文", strings.Repeat("a", MaxAttributeNameBytes+1)} {
		if err := ValidateAttributeName(name); err == nil {
			t.Errorf("ValidateAttributeName(%q) error = nil", name)
		}
	}
}

func TestValidateQuery(t *testing.T) {
	if err := ValidateQuery(validQuery()); err != nil {
		t.Fatalf("ValidateQuery() error = %v", err)
	}
	tests := []Query{
		{},
		func() Query { q := validQuery(); q.Start = q.Start.Add(time.Nanosecond); return q }(),
		func() Query { q := validQuery(); q.End = q.Start; return q }(),
		func() Query { q := validQuery(); q.Limit = MaxLimit + 1; return q }(),
		func() Query { q := validQuery(); q.Order = "newest"; return q }(),
		func() Query { q := validQuery(); q.Text = string([]byte{0xff}); return q }(),
		func() Query {
			q := validQuery()
			q.Matchers = []AttributeMatcher{{Name: "x", Op: MatchEqual}}
			return q
		}(),
		func() Query {
			q := validQuery()
			q.Matchers = []AttributeMatcher{{Name: "x", Op: "contains", Value: "y"}}
			return q
		}(),
	}
	for _, query := range tests {
		if err := ValidateQuery(query); !errors.Is(err, ErrInvalidQuery) {
			t.Fatalf("ValidateQuery(%+v) error = %v, want ErrInvalidQuery", query, err)
		}
	}
}

func TestValidatePage(t *testing.T) {
	if err := ValidatePage(Page{Records: []Record{{}, {}}}, 1); err == nil {
		t.Fatal("oversized page was accepted")
	}
	if err := ValidatePage(Page{HasNext: true}, 1); err == nil {
		t.Fatal("missing next cursor was accepted")
	}
	if err := ValidatePage(Page{NextCursor: "cursor"}, 1); err == nil {
		t.Fatal("final page cursor was accepted")
	}
}
