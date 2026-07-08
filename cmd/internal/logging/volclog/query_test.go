package volclog

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/volcengine/volc-sdk-golang/service/tls"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
)

func TestQueryServiceStreamsLogsAndCursor(t *testing.T) {
	client := &fakeSearchClient{
		responses: []*tls.SearchLogsResponse{
			{
				ListOver: false,
				Context:  "provider-next",
				Logs: []map[string]interface{}{
					{
						"__time__":   json.Number("1783403541016"),
						"time_ns":    "1783403541016789000",
						"level":      "error",
						"msg":        "agenthost failed",
						"error":      "boom",
						"request_id": "req-1",
					},
				},
			},
		},
	}
	service := NewQueryServiceWithClient("topic-a", client)
	var entries []gizclaw.ServerLogEntry
	end, err := service.StreamServerLogs(context.Background(), gizclaw.ServerLogStreamRequest{
		Filter:      "level:ERROR",
		StartTimeMs: 1783400000000,
		EndTimeMs:   1783403600000,
		Limit:       1,
		Order:       gizclaw.ServerLogOrderDesc,
	}, func(entry gizclaw.ServerLogEntry) error {
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamServerLogs error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries len = %d", len(entries))
	}
	got := entries[0]
	if got.TimeMs != 1783403541016 || got.TimeNs == nil || *got.TimeNs != 1783403541016789000 {
		t.Fatalf("entry time = %d/%v", got.TimeMs, got.TimeNs)
	}
	if got.Level != "ERROR" || got.Message != "agenthost failed" || got.Source != "gizclaw" || got.Path != "slog" {
		t.Fatalf("entry normalized fields = %#v", got)
	}
	if got.Fields["error"] != "boom" || got.Fields["request_id"] != "req-1" {
		t.Fatalf("entry fields = %#v", got.Fields)
	}
	if !end.HasNext || end.NextCursor == nil || *end.NextCursor == "" || end.Count != 1 {
		t.Fatalf("end = %#v", end)
	}
	if len(client.requests) != 1 {
		t.Fatalf("requests len = %d", len(client.requests))
	}
	req := client.requests[0]
	if req.TopicID != "topic-a" || req.Query != "level:ERROR" || req.Sort != "desc" || req.Limit != 1 {
		t.Fatalf("provider request = %#v", req)
	}
	if strings.Contains(*end.NextCursor, "provider-next") {
		t.Fatalf("cursor exposes provider context: %q", *end.NextCursor)
	}
}

func TestQueryServiceContinuesFromCursorAndValidatesMismatch(t *testing.T) {
	cursor, err := encodeQueryCursor(queryCursor{
		V:           1,
		Filter:      "*",
		StartTimeMs: 1000,
		EndTimeMs:   2000,
		Order:       string(gizclaw.ServerLogOrderAsc),
		Context:     "ctx-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeSearchClient{responses: []*tls.SearchLogsResponse{{ListOver: true}}}
	service := NewQueryServiceWithClient("topic-a", client)
	if _, err := service.StreamServerLogs(context.Background(), gizclaw.ServerLogStreamRequest{
		Filter:       "level:ERROR",
		FilterSet:    true,
		StartTimeMs:  1000,
		StartTimeSet: true,
		EndTimeMs:    2000,
		EndTimeSet:   true,
		Order:        gizclaw.ServerLogOrderAsc,
		OrderSet:     true,
		Limit:        10,
		Cursor:       cursor,
	}, nil); err == nil || !strings.Contains(err.Error(), "filter") {
		t.Fatalf("mismatch error = %v", err)
	}

	if _, err := service.StreamServerLogs(context.Background(), gizclaw.ServerLogStreamRequest{
		Limit:  10,
		Cursor: cursor,
	}, nil); err != nil {
		t.Fatalf("cursor continuation error = %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("requests len = %d", len(client.requests))
	}
	req := client.requests[0]
	if req.Context != "ctx-1" || req.Query != "*" || req.StartTime != 1000 || req.EndTime != 2000 || req.Sort != "asc" {
		t.Fatalf("cursor request = %#v", req)
	}
}

func TestQueryServicePaginationAndLimit(t *testing.T) {
	client := &fakeSearchClient{
		responses: []*tls.SearchLogsResponse{
			{ListOver: false, Context: "ctx-2", Logs: []map[string]interface{}{{"msg": "a"}}},
			{ListOver: true, Logs: []map[string]interface{}{{"msg": "b"}}},
		},
	}
	service := NewQueryServiceWithClient("topic-a", client)
	var messages []string
	end, err := service.StreamServerLogs(context.Background(), gizclaw.ServerLogStreamRequest{
		Filter:      "*",
		StartTimeMs: 1000,
		EndTimeMs:   2000,
		Limit:       2,
		Order:       gizclaw.ServerLogOrderAsc,
	}, func(entry gizclaw.ServerLogEntry) error {
		messages = append(messages, entry.Message)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamServerLogs error = %v", err)
	}
	if strings.Join(messages, ",") != "a,b" {
		t.Fatalf("messages = %v", messages)
	}
	if end.HasNext || end.NextCursor != nil || end.Count != 2 {
		t.Fatalf("end = %#v", end)
	}
	if len(client.requests) != 2 || client.requests[0].Limit != 2 || client.requests[1].Limit != 1 || client.requests[1].Context != "ctx-2" {
		t.Fatalf("requests = %#v", client.requests)
	}
}

func TestQueryServiceCancellationAndProviderErrors(t *testing.T) {
	service := NewQueryServiceWithClient("topic-a", &fakeSearchClient{err: errors.New("denied")})
	if _, err := service.StreamServerLogs(context.Background(), gizclaw.ServerLogStreamRequest{
		Filter:      "*",
		StartTimeMs: 1000,
		EndTimeMs:   2000,
		Limit:       10,
		Order:       gizclaw.ServerLogOrderAsc,
	}, nil); err == nil || !strings.Contains(err.Error(), "backend failed") {
		t.Fatalf("provider err = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service = NewQueryServiceWithClient("topic-a", &fakeSearchClient{})
	if _, err := service.StreamServerLogs(ctx, gizclaw.ServerLogStreamRequest{
		Filter:      "*",
		StartTimeMs: 1000,
		EndTimeMs:   2000,
		Limit:       10,
		Order:       gizclaw.ServerLogOrderAsc,
	}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel err = %v", err)
	}
}

func TestQueryServiceInvalidInputs(t *testing.T) {
	service := NewQueryServiceWithClient("topic-a", &fakeSearchClient{})
	tests := []struct {
		name string
		req  gizclaw.ServerLogStreamRequest
		want string
	}{
		{name: "missing start", req: gizclaw.ServerLogStreamRequest{EndTimeMs: 2, Limit: 1, Order: gizclaw.ServerLogOrderAsc}, want: "start_time_ms"},
		{name: "bad range", req: gizclaw.ServerLogStreamRequest{StartTimeMs: 2, EndTimeMs: 2, Limit: 1, Order: gizclaw.ServerLogOrderAsc}, want: "end_time_ms"},
		{name: "bad order", req: gizclaw.ServerLogStreamRequest{StartTimeMs: 1, EndTimeMs: 2, Limit: 1, Order: "newest"}, want: "order"},
		{name: "bad cursor", req: gizclaw.ServerLogStreamRequest{Limit: 1, Cursor: "not-base64"}, want: "cursor"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := service.StreamServerLogs(context.Background(), tc.req, nil); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestNewQueryServiceValidatesConfig(t *testing.T) {
	base := Config{
		Endpoint:        " https://tls-cn-beijing.volces.com ",
		Region:          " cn-beijing ",
		TopicID:         " topic ",
		AccessKeyID:     " ak ",
		AccessKeySecret: " sk ",
	}
	service, err := NewQueryService(base)
	if err != nil || service == nil {
		t.Fatalf("NewQueryService() service=%v err=%v", service, err)
	}
	if service.topicID != "topic" {
		t.Fatalf("topicID = %q", service.topicID)
	}
	base.TopicID = ""
	if _, err := NewQueryService(base); err == nil || !strings.Contains(err.Error(), "topic") {
		t.Fatalf("NewQueryService missing topic err = %v", err)
	}
}

type fakeSearchClient struct {
	requests  []*tls.SearchLogsRequest
	responses []*tls.SearchLogsResponse
	err       error
}

func (c *fakeSearchClient) SearchLogsV2(request *tls.SearchLogsRequest) (*tls.SearchLogsResponse, error) {
	c.requests = append(c.requests, request)
	if c.err != nil {
		return nil, c.err
	}
	if len(c.responses) == 0 {
		return &tls.SearchLogsResponse{ListOver: true}, nil
	}
	resp := c.responses[0]
	c.responses = c.responses[1:]
	return resp, nil
}
