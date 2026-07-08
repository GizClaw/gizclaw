package volclog

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/volcengine/volc-sdk-golang/service/tls"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
)

type searchLogsClient interface {
	SearchLogsV2(request *tls.SearchLogsRequest) (*tls.SearchLogsResponse, error)
}

type QueryService struct {
	client  searchLogsClient
	topicID string
}

type queryCursor struct {
	V           int    `json:"v"`
	Filter      string `json:"filter"`
	StartTimeMs int64  `json:"start_time_ms"`
	EndTimeMs   int64  `json:"end_time_ms"`
	Order       string `json:"order"`
	Context     string `json:"context"`
}

func NewQueryService(config Config) (*QueryService, error) {
	config, err := validateConfig(config)
	if err != nil {
		return nil, err
	}
	client := tls.NewClient(config.Endpoint, config.AccessKeyID, config.AccessKeySecret, "", config.Region)
	return NewQueryServiceWithClient(config.TopicID, client), nil
}

func NewQueryServiceWithClient(topicID string, client searchLogsClient) *QueryService {
	return &QueryService{
		client:  client,
		topicID: strings.TrimSpace(topicID),
	}
}

func (s *QueryService) StreamServerLogs(ctx context.Context, req gizclaw.ServerLogStreamRequest, emit func(gizclaw.ServerLogEntry) error) (gizclaw.ServerLogStreamEnd, error) {
	if s == nil || s.client == nil || s.topicID == "" {
		return gizclaw.ServerLogStreamEnd{}, gizclaw.LogQueryNotConfigured()
	}
	if emit == nil {
		emit = func(gizclaw.ServerLogEntry) error { return nil }
	}
	query, providerContext, err := prepareQuery(req)
	if err != nil {
		return gizclaw.ServerLogStreamEnd{}, err
	}
	remaining := req.Limit
	if remaining <= 0 {
		remaining = 100
	}
	if remaining > 1000 {
		remaining = 1000
	}

	var emitted int32
	var hasNext bool
	var nextContext string
	for remaining > 0 {
		if err := ctx.Err(); err != nil {
			return gizclaw.ServerLogStreamEnd{}, err
		}
		callLimit := remaining
		if callLimit > math.MaxInt32 {
			callLimit = math.MaxInt32
		}
		resp, err := s.client.SearchLogsV2(&tls.SearchLogsRequest{
			TopicID:   s.topicID,
			Query:     query.Filter,
			StartTime: query.StartTimeMs,
			EndTime:   query.EndTimeMs,
			Limit:     callLimit,
			Context:   providerContext,
			Sort:      query.Order,
		})
		if err != nil {
			return gizclaw.ServerLogStreamEnd{}, gizclaw.ServerLogBackendError(err)
		}
		if resp == nil {
			return gizclaw.ServerLogStreamEnd{}, gizclaw.ServerLogBackendError(errors.New("empty Volc TLS response"))
		}
		for _, raw := range resp.Logs {
			if remaining <= 0 {
				break
			}
			if err := ctx.Err(); err != nil {
				return gizclaw.ServerLogStreamEnd{}, err
			}
			if err := emit(volcLogEntry(raw)); err != nil {
				return gizclaw.ServerLogStreamEnd{}, err
			}
			emitted++
			remaining--
		}
		nextContext = strings.TrimSpace(resp.Context)
		hasNext = !resp.ListOver && nextContext != ""
		if !hasNext || remaining <= 0 {
			break
		}
		if nextContext == providerContext {
			return gizclaw.ServerLogStreamEnd{}, gizclaw.ServerLogBackendError(errors.New("Volc TLS pagination context did not advance"))
		}
		providerContext = nextContext
	}

	end := gizclaw.ServerLogStreamEnd{Count: emitted, HasNext: hasNext}
	if hasNext {
		cursor, err := encodeQueryCursor(queryCursor{
			V:           1,
			Filter:      query.Filter,
			StartTimeMs: query.StartTimeMs,
			EndTimeMs:   query.EndTimeMs,
			Order:       query.Order,
			Context:     nextContext,
		})
		if err != nil {
			return gizclaw.ServerLogStreamEnd{}, gizclaw.ServerLogBackendError(err)
		}
		end.NextCursor = &cursor
	}
	return end, nil
}

func prepareQuery(req gizclaw.ServerLogStreamRequest) (queryCursor, string, error) {
	filter := strings.TrimSpace(req.Filter)
	if filter == "" {
		filter = "*"
	}
	order := string(req.Order)
	if order == "" {
		order = string(gizclaw.ServerLogOrderAsc)
	}
	switch order {
	case string(gizclaw.ServerLogOrderAsc), string(gizclaw.ServerLogOrderDesc):
	default:
		return queryCursor{}, "", gizclaw.InvalidServerLogQuery("INVALID_LOG_ORDER", "order must be asc or desc")
	}
	if req.Cursor == "" {
		if req.StartTimeMs <= 0 {
			return queryCursor{}, "", gizclaw.InvalidServerLogQuery("INVALID_LOG_TIME_RANGE", "start_time_ms is required")
		}
		if req.EndTimeMs <= req.StartTimeMs {
			return queryCursor{}, "", gizclaw.InvalidServerLogQuery("INVALID_LOG_TIME_RANGE", "end_time_ms must be greater than start_time_ms")
		}
		return queryCursor{Filter: filter, StartTimeMs: req.StartTimeMs, EndTimeMs: req.EndTimeMs, Order: order}, "", nil
	}

	cursor, err := decodeQueryCursor(req.Cursor)
	if err != nil {
		return queryCursor{}, "", err
	}
	if req.FilterSet && filter != cursor.Filter {
		return queryCursor{}, "", gizclaw.InvalidServerLogQuery("LOG_CURSOR_MISMATCH", "filter does not match cursor query")
	}
	if req.StartTimeSet && req.StartTimeMs != cursor.StartTimeMs {
		return queryCursor{}, "", gizclaw.InvalidServerLogQuery("LOG_CURSOR_MISMATCH", "start_time_ms does not match cursor query")
	}
	if req.EndTimeSet && req.EndTimeMs != cursor.EndTimeMs {
		return queryCursor{}, "", gizclaw.InvalidServerLogQuery("LOG_CURSOR_MISMATCH", "end_time_ms does not match cursor query")
	}
	if req.OrderSet && order != cursor.Order {
		return queryCursor{}, "", gizclaw.InvalidServerLogQuery("LOG_CURSOR_MISMATCH", "order does not match cursor query")
	}
	return cursor, cursor.Context, nil
}

func encodeQueryCursor(cursor queryCursor) (string, error) {
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeQueryCursor(value string) (queryCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return queryCursor{}, gizclaw.InvalidServerLogQuery("INVALID_LOG_CURSOR", "cursor is malformed")
	}
	var cursor queryCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return queryCursor{}, gizclaw.InvalidServerLogQuery("INVALID_LOG_CURSOR", "cursor is malformed")
	}
	if cursor.V != 1 || cursor.Context == "" || cursor.Filter == "" || cursor.StartTimeMs <= 0 || cursor.EndTimeMs <= cursor.StartTimeMs {
		return queryCursor{}, gizclaw.InvalidServerLogQuery("INVALID_LOG_CURSOR", "cursor is invalid")
	}
	if cursor.Order != string(gizclaw.ServerLogOrderAsc) && cursor.Order != string(gizclaw.ServerLogOrderDesc) {
		return queryCursor{}, gizclaw.InvalidServerLogQuery("INVALID_LOG_CURSOR", "cursor order is invalid")
	}
	return cursor, nil
}

func volcLogEntry(raw map[string]interface{}) gizclaw.ServerLogEntry {
	fields := make(map[string]string, len(raw))
	for key, value := range raw {
		if reservedLogField(key) {
			continue
		}
		fields[key] = logFieldString(value)
	}
	timeMs := firstInt64(raw, "time_ms", "__time__", "_time_", "Time", "time")
	var timeNs *int64
	if ns, ok := firstInt64OK(raw, "time_ns", "__time_ns__", "_time_ns_"); ok {
		timeNs = &ns
	}
	level := strings.ToUpper(firstString(raw, "level"))
	if level == "" {
		level = "INFO"
	}
	message := firstString(raw, "msg", "message")
	return gizclaw.ServerLogEntry{
		TimeMs:  timeMs,
		TimeNs:  timeNs,
		Level:   level,
		Message: message,
		Source:  firstStringDefault(raw, "gizclaw", "__source__", "source"),
		Path:    firstStringDefault(raw, "slog", "__path__", "__filename__", "path"),
		Fields:  fields,
	}
}

func reservedLogField(key string) bool {
	switch key {
	case "time_ms", "__time__", "_time_", "Time", "time", "time_ns", "__time_ns__", "_time_ns_", "level", "msg", "message", "__source__", "source", "__path__", "__filename__", "path":
		return true
	default:
		return false
	}
}

func firstString(raw map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := raw[key]; ok {
			if s := strings.TrimSpace(logFieldString(value)); s != "" {
				return s
			}
		}
	}
	return ""
}

func firstStringDefault(raw map[string]interface{}, fallback string, keys ...string) string {
	if value := firstString(raw, keys...); value != "" {
		return value
	}
	return fallback
}

func firstInt64(raw map[string]interface{}, keys ...string) int64 {
	value, _ := firstInt64OK(raw, keys...)
	return value
}

func firstInt64OK(raw map[string]interface{}, keys ...string) (int64, bool) {
	for _, key := range keys {
		if value, ok := raw[key]; ok {
			if n, ok := int64Value(value); ok {
				return n, true
			}
		}
	}
	return 0, false
}

func int64Value(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case float64:
		return int64(v), true
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}

func logFieldString(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case json.Number:
		return v.String()
	default:
		data, err := json.Marshal(v)
		if err == nil && string(data) != "null" {
			if len(data) >= 2 && data[0] == '"' && data[len(data)-1] == '"' {
				var s string
				if json.Unmarshal(data, &s) == nil {
					return s
				}
			}
			return string(data)
		}
		return fmt.Sprint(v)
	}
}

func validateConfig(config Config) (Config, error) {
	config.Endpoint = strings.TrimSpace(config.Endpoint)
	config.Region = strings.TrimSpace(config.Region)
	config.TopicID = strings.TrimSpace(config.TopicID)
	config.AccessKeyID = strings.TrimSpace(config.AccessKeyID)
	config.AccessKeySecret = strings.TrimSpace(config.AccessKeySecret)
	if config.Endpoint == "" {
		return Config{}, errors.New("volclog: endpoint is required")
	}
	if config.Region == "" {
		return Config{}, errors.New("volclog: region is required")
	}
	if config.TopicID == "" {
		return Config{}, errors.New("volclog: topic id is required")
	}
	if config.AccessKeyID == "" {
		return Config{}, errors.New("volclog: access key id is required")
	}
	if config.AccessKeySecret == "" {
		return Config{}, errors.New("volclog: access key secret is required")
	}
	return config, nil
}
