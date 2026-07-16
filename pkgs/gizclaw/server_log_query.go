package gizclaw

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

const (
	defaultServerLogStreamLimit = 100
	maxServerLogStreamLimit     = 1000
	maxServerLogFilterBytes     = 4096
	maxServerLogFilterClauses   = 32
	maxServerLogFilterValue     = 1024
	maxServerLogCursorBytes     = 32 * 1024
)

type ServerLogOrder string

const (
	ServerLogOrderAsc  ServerLogOrder = "asc"
	ServerLogOrderDesc ServerLogOrder = "desc"
)

type ServerLogStreamRequest struct {
	Filter       string
	FilterSet    bool
	StartTimeMs  int64
	StartTimeSet bool
	EndTimeMs    int64
	EndTimeSet   bool
	Limit        int
	Order        ServerLogOrder
	OrderSet     bool
	Cursor       string
}

type ServerLogQueryService interface {
	StreamServerLogs(ctx context.Context, req ServerLogStreamRequest, emit func(apitypes.ServerLogEntry) error) (apitypes.ServerLogStreamEnd, error)
}

// NewServerLogQueryService adapts a backend-neutral Querier to the Admin system-log stream.
func NewServerLogQueryService(querier logstore.Querier) ServerLogQueryService {
	if querier == nil {
		return nil
	}
	return &serverLogStoreQuery{querier: querier}
}

type serverLogStoreQuery struct {
	querier logstore.Querier
}

type adminLogQuery struct {
	Severities []string                    `json:"severities,omitempty"`
	Matchers   []logstore.AttributeMatcher `json:"matchers,omitempty"`
	Text       string                      `json:"text,omitempty"`
	StartMS    int64                       `json:"start_ms"`
	EndMS      int64                       `json:"end_ms"`
	Order      logstore.Order              `json:"order"`
}

type adminLogCursor struct {
	Version     int           `json:"v"`
	Query       adminLogQuery `json:"query"`
	StoreCursor string        `json:"store_cursor"`
}

func (s *serverLogStoreQuery) StreamServerLogs(ctx context.Context, req ServerLogStreamRequest, emit func(apitypes.ServerLogEntry) error) (apitypes.ServerLogStreamEnd, error) {
	query, err := prepareAdminLogQuery(req)
	if err != nil {
		return apitypes.ServerLogStreamEnd{}, err
	}
	page, err := s.querier.Query(ctx, query)
	if err != nil {
		switch {
		case errors.Is(err, logstore.ErrCursorMismatch):
			return apitypes.ServerLogStreamEnd{}, InvalidServerLogQuery("LOG_CURSOR_MISMATCH", err.Error())
		case errors.Is(err, logstore.ErrInvalidQuery):
			return apitypes.ServerLogStreamEnd{}, InvalidServerLogQuery("INVALID_LOG_QUERY", err.Error())
		default:
			return apitypes.ServerLogStreamEnd{}, ServerLogBackendError(err)
		}
	}
	if err := logstore.ValidatePage(page, query.Limit); err != nil {
		return apitypes.ServerLogStreamEnd{}, ServerLogBackendError(err)
	}
	if emit == nil {
		emit = func(apitypes.ServerLogEntry) error { return nil }
	}
	for _, record := range page.Records {
		if record.Stream != "system" || record.Kind != "log" {
			return apitypes.ServerLogStreamEnd{}, ServerLogBackendError(errors.New("log store returned a record outside the system log scope"))
		}
		if err := ctx.Err(); err != nil {
			return apitypes.ServerLogStreamEnd{}, err
		}
		if err := emit(serverLogEntry(record)); err != nil {
			return apitypes.ServerLogStreamEnd{}, err
		}
	}
	end := apitypes.ServerLogStreamEnd{Count: int32(len(page.Records)), HasNext: page.HasNext}
	if page.HasNext {
		cursor, err := encodeAdminLogCursor(adminLogCursor{Version: 1, Query: adminQueryFromStore(query), StoreCursor: page.NextCursor})
		if err != nil {
			return apitypes.ServerLogStreamEnd{}, ServerLogBackendError(err)
		}
		end.NextCursor = &cursor
	}
	return end, nil
}

func prepareAdminLogQuery(req ServerLogStreamRequest) (logstore.Query, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = defaultServerLogStreamLimit
	}
	if limit > maxServerLogStreamLimit {
		limit = maxServerLogStreamLimit
	}
	if req.Cursor == "" {
		if req.StartTimeMs <= 0 {
			return logstore.Query{}, InvalidServerLogQuery("INVALID_LOG_QUERY", "start_time_ms is required")
		}
		filter, err := parseServerLogFilter(req.Filter)
		if err != nil {
			return logstore.Query{}, err
		}
		order := logstore.Order(req.Order)
		if order == "" {
			order = logstore.OrderAsc
		}
		query := logstore.Query{
			Streams: []string{"system"}, Kinds: []string{"log"}, Severities: filter.Severities,
			Matchers: filter.Matchers, Text: filter.Text, Start: time.UnixMilli(req.StartTimeMs).UTC(),
			End: time.UnixMilli(req.EndTimeMs).UTC(), Limit: limit, Order: order,
		}
		if err := logstore.ValidateQuery(query); err != nil {
			return logstore.Query{}, InvalidServerLogQuery("INVALID_LOG_QUERY", err.Error())
		}
		return query, nil
	}
	cursor, err := decodeAdminLogCursor(req.Cursor)
	if err != nil {
		return logstore.Query{}, err
	}
	if req.FilterSet {
		filter, err := parseServerLogFilter(req.Filter)
		if err != nil {
			return logstore.Query{}, err
		}
		if !slices.Equal(filter.Severities, cursor.Query.Severities) || filter.Text != cursor.Query.Text || !slices.Equal(filter.Matchers, cursor.Query.Matchers) {
			return logstore.Query{}, InvalidServerLogQuery("LOG_CURSOR_MISMATCH", "filter does not match cursor query")
		}
	}
	if req.StartTimeSet && req.StartTimeMs != cursor.Query.StartMS {
		return logstore.Query{}, InvalidServerLogQuery("LOG_CURSOR_MISMATCH", "start_time_ms does not match cursor query")
	}
	if req.EndTimeSet && req.EndTimeMs != cursor.Query.EndMS {
		return logstore.Query{}, InvalidServerLogQuery("LOG_CURSOR_MISMATCH", "end_time_ms does not match cursor query")
	}
	if req.OrderSet && logstore.Order(req.Order) != cursor.Query.Order {
		return logstore.Query{}, InvalidServerLogQuery("LOG_CURSOR_MISMATCH", "order does not match cursor query")
	}
	query := logstore.Query{
		Streams: []string{"system"}, Kinds: []string{"log"}, Severities: append([]string(nil), cursor.Query.Severities...),
		Matchers: append([]logstore.AttributeMatcher(nil), cursor.Query.Matchers...), Text: cursor.Query.Text,
		Start: time.UnixMilli(cursor.Query.StartMS).UTC(), End: time.UnixMilli(cursor.Query.EndMS).UTC(),
		Limit: limit, Order: cursor.Query.Order, Cursor: cursor.StoreCursor,
	}
	if err := logstore.ValidateQuery(query); err != nil {
		return logstore.Query{}, InvalidServerLogQuery("INVALID_LOG_CURSOR", "cursor query is invalid")
	}
	return query, nil
}

func adminQueryFromStore(query logstore.Query) adminLogQuery {
	return adminLogQuery{
		Severities: append([]string(nil), query.Severities...), Matchers: append([]logstore.AttributeMatcher(nil), query.Matchers...),
		Text: query.Text, StartMS: query.Start.UnixMilli(), EndMS: query.End.UnixMilli(), Order: query.Order,
	}
}

type parsedServerLogFilter struct {
	Severities []string
	Matchers   []logstore.AttributeMatcher
	Text       string
}

func parseServerLogFilter(value string) (parsedServerLogFilter, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "*" {
		return parsedServerLogFilter{}, nil
	}
	if len(value) > maxServerLogFilterBytes || !utf8.ValidString(value) {
		return parsedServerLogFilter{}, InvalidServerLogQuery("INVALID_LOG_QUERY", "filter must be valid UTF-8 and at most 4096 bytes")
	}
	clauses, err := splitServerLogClauses(value)
	if err != nil {
		return parsedServerLogFilter{}, err
	}
	if len(clauses) > maxServerLogFilterClauses {
		return parsedServerLogFilter{}, InvalidServerLogQuery("INVALID_LOG_QUERY", "filter contains more than 32 clauses")
	}
	result := parsedServerLogFilter{}
	levelSet, textSet := false, false
	for _, clause := range clauses {
		if clause == "*" {
			return parsedServerLogFilter{}, InvalidServerLogQuery("INVALID_LOG_QUERY", "* must be the complete filter")
		}
		field, op, raw, err := parseServerLogClause(clause)
		if err != nil {
			return parsedServerLogFilter{}, err
		}
		decoded, err := decodeServerLogValue(raw)
		if err != nil {
			return parsedServerLogFilter{}, err
		}
		switch field {
		case "level":
			if op != logstore.MatchEqual || raw == "*" || levelSet {
				return parsedServerLogFilter{}, InvalidServerLogQuery("INVALID_LOG_QUERY", "level supports one equality clause")
			}
			levelSet = true
			result.Severities = []string{decoded}
		case "text":
			if op != logstore.MatchEqual || raw == "*" || textSet {
				return parsedServerLogFilter{}, InvalidServerLogQuery("INVALID_LOG_QUERY", "text supports one equality clause")
			}
			textSet = true
			result.Text = decoded
		case "message", "stream", "kind", "__source__", "__path__", "__filename__", "__time__", "_time_", "time_ms", "__time_ns__", "_time_ns_", "time_ns":
			return parsedServerLogFilter{}, InvalidServerLogQuery("INVALID_LOG_QUERY", field+" is reserved")
		default:
			if err := logstore.ValidateAttributeName(field); err != nil {
				return parsedServerLogFilter{}, InvalidServerLogQuery("INVALID_LOG_QUERY", err.Error())
			}
			result.Matchers = append(result.Matchers, logstore.AttributeMatcher{Name: field, Op: op, Value: decoded})
		}
	}
	sort.Slice(result.Matchers, func(i, j int) bool {
		left, right := result.Matchers[i], result.Matchers[j]
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		if left.Op != right.Op {
			return left.Op < right.Op
		}
		return left.Value < right.Value
	})
	return result, nil
}

func splitServerLogClauses(value string) ([]string, error) {
	var clauses []string
	start, quoted, escaped := 0, false, false
	for index := 0; index < len(value); index++ {
		char := value[index]
		if quoted {
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
			} else if char == '"' {
				quoted = false
			}
			continue
		}
		if char == '"' {
			quoted = true
			continue
		}
		if !asciiSpace(char) {
			continue
		}
		spaceStart := index
		for index < len(value) && asciiSpace(value[index]) {
			index++
		}
		if index+3 <= len(value) && value[index:index+3] == "AND" && index+3 < len(value) && asciiSpace(value[index+3]) {
			clause := strings.TrimSpace(value[start:spaceStart])
			if clause == "" {
				return nil, InvalidServerLogQuery("INVALID_LOG_QUERY", "filter contains an empty clause")
			}
			clauses = append(clauses, clause)
			index += 3
			for index < len(value) && asciiSpace(value[index]) {
				index++
			}
			start = index
			index--
		}
	}
	if quoted || escaped {
		return nil, InvalidServerLogQuery("INVALID_LOG_QUERY", "filter contains an unterminated string")
	}
	last := strings.TrimSpace(value[start:])
	if last == "" {
		return nil, InvalidServerLogQuery("INVALID_LOG_QUERY", "filter contains an empty clause")
	}
	return append(clauses, last), nil
}

func parseServerLogClause(clause string) (string, logstore.MatchOp, string, error) {
	if strings.HasPrefix(clause, "-") && strings.HasSuffix(clause, ":*") {
		return strings.TrimSpace(clause[1 : len(clause)-2]), logstore.MatchNotExists, "*", nil
	}
	if field, raw, found := strings.Cut(clause, "!="); found {
		return strings.TrimSpace(field), logstore.MatchNotEqual, strings.TrimSpace(raw), nil
	}
	field, raw, found := strings.Cut(clause, ":")
	if !found {
		return "", "", "", InvalidServerLogQuery("INVALID_LOG_QUERY", "filter clause is missing an operator")
	}
	field, raw = strings.TrimSpace(field), strings.TrimSpace(raw)
	op := logstore.MatchEqual
	if raw == "*" {
		op = logstore.MatchExists
	}
	return field, op, raw, nil
}

func decodeServerLogValue(raw string) (string, error) {
	if raw == "*" {
		return "", nil
	}
	if raw == "" {
		return "", InvalidServerLogQuery("INVALID_LOG_QUERY", "filter value is empty")
	}
	var value string
	if raw[0] == '"' {
		if json.Unmarshal([]byte(raw), &value) != nil {
			return "", InvalidServerLogQuery("INVALID_LOG_QUERY", "filter string is malformed")
		}
	} else {
		for index := range len(raw) {
			if asciiSpace(raw[index]) || raw[index] == '"' || raw[index] == '\\' || raw[index] == '*' {
				return "", InvalidServerLogQuery("INVALID_LOG_QUERY", "filter token contains an unsupported character")
			}
		}
		value = raw
	}
	if value == "" || len(value) > maxServerLogFilterValue || !utf8.ValidString(value) {
		return "", InvalidServerLogQuery("INVALID_LOG_QUERY", "filter value must be valid UTF-8, non-empty, and at most 1024 bytes")
	}
	if strings.Contains(value, "*") {
		return "", InvalidServerLogQuery("INVALID_LOG_QUERY", "filter values do not support wildcards")
	}
	return value, nil
}

func asciiSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n'
}

func encodeAdminLogCursor(cursor adminLogCursor) (string, error) {
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(data)
	if len(encoded) > maxServerLogCursorBytes {
		return "", errors.New("admin log cursor is too large")
	}
	return encoded, nil
}

func decodeAdminLogCursor(value string) (adminLogCursor, error) {
	if len(value) > maxServerLogCursorBytes {
		return adminLogCursor{}, InvalidServerLogQuery("INVALID_LOG_CURSOR", "cursor is too large")
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil || len(data) > maxServerLogCursorBytes {
		return adminLogCursor{}, InvalidServerLogQuery("INVALID_LOG_CURSOR", "cursor is malformed")
	}
	var cursor adminLogCursor
	if json.Unmarshal(data, &cursor) != nil || cursor.Version != 1 || cursor.StoreCursor == "" || cursor.Query.StartMS <= 0 || cursor.Query.EndMS <= cursor.Query.StartMS || cursor.Query.Order != logstore.OrderAsc && cursor.Query.Order != logstore.OrderDesc {
		return adminLogCursor{}, InvalidServerLogQuery("INVALID_LOG_CURSOR", "cursor is invalid")
	}
	return cursor, nil
}

func serverLogEntry(record logstore.Record) apitypes.ServerLogEntry {
	fields := make(map[string]string, len(record.Attributes))
	for key, value := range record.Attributes {
		if key != "source" && key != "path" {
			fields[key] = value
		}
	}
	level := strings.ToUpper(record.Severity)
	if level == "" {
		level = "INFO"
	}
	var timeNS *string
	if record.Time.Nanosecond()%int(time.Millisecond) != 0 {
		value := strconv.FormatInt(record.Time.UnixNano(), 10)
		timeNS = &value
	}
	source, path := record.Attributes["source"], record.Attributes["path"]
	if source == "" {
		source = "gizclaw"
	}
	if path == "" {
		path = "slog"
	}
	return apitypes.ServerLogEntry{TimeMs: record.Time.UnixMilli(), TimeNs: timeNS, Level: level, Message: record.Message, Source: source, Path: path, Fields: fields}
}

type ServerLogQueryError struct {
	StatusCode int
	Code       string
	Message    string
	Err        error
}

func (e *ServerLogQueryError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *ServerLogQueryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func InvalidServerLogQuery(code, message string) *ServerLogQueryError {
	return &ServerLogQueryError{StatusCode: http.StatusBadRequest, Code: code, Message: message}
}

func LogQueryNotConfigured() *ServerLogQueryError {
	return &ServerLogQueryError{StatusCode: http.StatusNotImplemented, Code: "LOG_QUERY_NOT_CONFIGURED", Message: "server log query backend is not configured"}
}

func ServerLogBackendError(err error) *ServerLogQueryError {
	if err == nil {
		return nil
	}
	return &ServerLogQueryError{StatusCode: http.StatusBadGateway, Code: "LOG_QUERY_BACKEND_ERROR", Message: "server log query backend failed", Err: err}
}

func serverLogQueryErrorResponse(err error) (int, apitypes.ErrorResponse) {
	var queryErr *ServerLogQueryError
	if errors.As(err, &queryErr) {
		return queryErr.StatusCode, apitypes.NewErrorResponse(queryErr.Code, queryErr.Message)
	}
	return http.StatusBadGateway, apitypes.NewErrorResponse("LOG_QUERY_BACKEND_ERROR", "server log query backend failed")
}
