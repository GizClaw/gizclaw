package gizclaw

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

type fakeLogQuerier struct {
	queries []logstore.Query
	page    logstore.Page
	err     error
}

func (f *fakeLogQuerier) Query(_ context.Context, query logstore.Query) (logstore.Page, error) {
	f.queries = append(f.queries, query)
	return f.page, f.err
}

func newTestServerLogQueryService(t *testing.T, querier logstore.Querier) ServerLogQueryService {
	t.Helper()
	service, err := NewServerLogQueryService(querier)
	if err != nil {
		t.Fatalf("NewServerLogQueryService() error = %v", err)
	}
	return service
}

func TestParseServerLogFilter(t *testing.T) {
	filter, err := parseServerLogFilter(`level:WARN AND text:"failed request" AND request.method!=POST AND request.id:* AND -trace.id:*`)
	if err != nil {
		t.Fatalf("parseServerLogFilter() error = %v", err)
	}
	if len(filter.Severities) != 1 || filter.Severities[0] != "WARN" || filter.Text != "failed request" || len(filter.Matchers) != 3 {
		t.Fatalf("filter = %+v", filter)
	}
	lowerLevel, err := parseServerLogFilter(`level:warn`)
	if err != nil || len(lowerLevel.Severities) != 1 || lowerLevel.Severities[0] != "WARN" {
		t.Fatalf("lowercase level filter = %+v, %v", lowerLevel, err)
	}
	for _, value := range []string{
		`level!=WARN`, `message:x`, `stream:system`, `text:x OR text:y`, `field:foo*`,
		`level:WARN AND level:ERROR`, `bad..field:x`, `field:"unterminated`,
	} {
		if _, err := parseServerLogFilter(value); err == nil {
			t.Fatalf("parseServerLogFilter(%q) error = nil", value)
		}
	}
	quotedOperators, err := parseServerLogFilter(`text:"retry != nil" AND result:"a!=b" AND status!="a:b"`)
	if err != nil {
		t.Fatalf("parse quoted operators: %v", err)
	}
	if quotedOperators.Text != "retry != nil" || len(quotedOperators.Matchers) != 2 || quotedOperators.Matchers[0].Value != "a!=b" || quotedOperators.Matchers[1].Value != "a:b" {
		t.Fatalf("quoted operator filter = %+v", quotedOperators)
	}
}

func TestParseServerLogFilterBoundsAndEscaping(t *testing.T) {
	accepted := []string{
		"*",
		`text:"quoted AND escaped \"value\""`,
		"field:value\tAND\tother:*\nAND\n-third:*",
		strings.Repeat("a", 128) + ":value",
	}
	for _, value := range accepted {
		if _, err := parseServerLogFilter(value); err != nil {
			t.Errorf("parseServerLogFilter(%q) error = %v", value, err)
		}
	}
	tooMany := make([]string, 33)
	for index := range tooMany {
		tooMany[index] = "field" + string(rune('A'+index%26)) + ":value"
	}
	rejected := []string{
		strings.Repeat("x", maxServerLogFilterBytes+1),
		strings.Join(tooMany, " AND "),
		strings.Repeat("a", 129) + ":value",
		"field:" + strings.Repeat("v", maxServerLogFilterValue+1),
		"field:\\escape", "field:value*", `field:"value*"`, "field:", "*:value", "kind:log", "message!=x", "text:*", "-level:*",
		"__source__:gizclaw", "__path__:slog", "__time__:1", "Time:1", "time:*",
	}
	for _, value := range rejected {
		if _, err := parseServerLogFilter(value); err == nil {
			t.Errorf("parseServerLogFilter(%q) error = nil", value)
		}
	}
}

func TestServerLogStoreQueryCursor(t *testing.T) {
	querier := &fakeLogQuerier{page: logstore.Page{Records: []logstore.Record{{
		ID: "id", Time: time.Unix(10, 123).UTC(), Stream: "system", Kind: "log", Severity: "warn", Message: "message",
		Attributes: map[string]string{"source": "gizclaw", "path": "slog", "request.id": "1"},
	}}, HasNext: true, NextCursor: "provider-secret-context"}}
	service := newTestServerLogQueryService(t, querier)
	end, err := service.StreamServerLogs(context.Background(), ServerLogStreamRequest{
		Filter: "level:warn", StartTimeMs: 1000, EndTimeMs: 2000, Limit: 10, Order: ServerLogOrderDesc,
	}, nil)
	if err != nil {
		t.Fatalf("StreamServerLogs() error = %v", err)
	}
	if end.NextCursor == nil || *end.NextCursor == "provider-secret-context" {
		t.Fatalf("outer cursor = %v", end.NextCursor)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(*end.NextCursor)
	if err != nil || bytes.Contains(decoded, []byte("provider-secret-context")) {
		t.Fatalf("outer cursor exposes provider context: %q, %v", decoded, err)
	}
	query := querier.queries[0]
	if len(query.Streams) != 1 || query.Streams[0] != "system" || len(query.Kinds) != 1 || query.Kinds[0] != "log" || query.Cursor != "" {
		t.Fatalf("first query = %+v", query)
	}
	querier.page = logstore.Page{}
	_, err = service.StreamServerLogs(context.Background(), ServerLogStreamRequest{Cursor: *end.NextCursor, Limit: 1}, nil)
	if err != nil {
		t.Fatalf("continuation error = %v", err)
	}
	if got := querier.queries[1]; got.Cursor != "provider-secret-context" || got.Limit != 1 || got.Start.UnixMilli() != 1000 || got.Order != logstore.OrderDesc {
		t.Fatalf("continuation query = %+v", got)
	}
	_, err = service.StreamServerLogs(context.Background(), ServerLogStreamRequest{Cursor: *end.NextCursor, Filter: "level:error", FilterSet: true}, nil)
	var queryErr *ServerLogQueryError
	if !errors.As(err, &queryErr) || queryErr.Code != "LOG_CURSOR_MISMATCH" {
		t.Fatalf("mismatch error = %v", err)
	}
	_, err = service.StreamServerLogs(context.Background(), ServerLogStreamRequest{Cursor: *end.NextCursor, Filter: "*", FilterSet: true}, nil)
	if !errors.As(err, &queryErr) || queryErr.Code != "LOG_CURSOR_MISMATCH" {
		t.Fatalf("wildcard mismatch error = %v", err)
	}
	other := newTestServerLogQueryService(t, &fakeLogQuerier{})
	_, err = other.StreamServerLogs(context.Background(), ServerLogStreamRequest{Cursor: *end.NextCursor}, nil)
	if !errors.As(err, &queryErr) || queryErr.Code != "INVALID_LOG_CURSOR" {
		t.Fatalf("foreign service cursor error = %v", err)
	}
}

func TestServerLogStoreQueryMapsStoreErrors(t *testing.T) {
	querier := &fakeLogQuerier{err: logstore.ErrInvalidQuery}
	service := newTestServerLogQueryService(t, querier)
	_, err := service.StreamServerLogs(context.Background(), ServerLogStreamRequest{StartTimeMs: 1000, EndTimeMs: 2000}, nil)
	var queryErr *ServerLogQueryError
	if !errors.As(err, &queryErr) || queryErr.Code != "INVALID_LOG_QUERY" {
		t.Fatalf("error = %v", err)
	}
}

func TestServerLogStoreQueryRejectsInvalidOrOutOfScopePage(t *testing.T) {
	for _, page := range []logstore.Page{
		{HasNext: true},
		{Records: []logstore.Record{{Stream: "chat", Kind: "message"}}},
	} {
		querier := &fakeLogQuerier{page: page}
		service := newTestServerLogQueryService(t, querier)
		_, err := service.StreamServerLogs(context.Background(), ServerLogStreamRequest{StartTimeMs: 1000, EndTimeMs: 2000}, nil)
		var queryErr *ServerLogQueryError
		if !errors.As(err, &queryErr) || queryErr.StatusCode != 502 {
			t.Fatalf("page %+v error = %v", page, err)
		}
	}
}

func TestServerLogStoreQueryRejectsNonPositiveStart(t *testing.T) {
	service := newTestServerLogQueryService(t, &fakeLogQuerier{})
	for _, start := range []int64{0, -1} {
		_, err := service.StreamServerLogs(context.Background(), ServerLogStreamRequest{StartTimeMs: start, EndTimeMs: 2000}, nil)
		var queryErr *ServerLogQueryError
		if !errors.As(err, &queryErr) || queryErr.Code != "INVALID_LOG_QUERY" {
			t.Fatalf("start %d error = %v", start, err)
		}
	}
}
