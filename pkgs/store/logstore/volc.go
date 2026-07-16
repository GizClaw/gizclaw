package logstore

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/volcengine/volc-sdk-golang/service/tls"
	"github.com/volcengine/volc-sdk-golang/service/tls/common"
	"github.com/volcengine/volc-sdk-golang/service/tls/pb"
	"github.com/volcengine/volc-sdk-golang/service/tls/producer"
)

const (
	volcQueryTimeout          = 30 * time.Second
	volcCursorLimit           = 16 * 1024
	volcMessageDelim          = " \t\r\n"
	volcSecondsUpperBound     = int64(10_000_000_000)
	volcNanosecondsLowerBound = int64(1_000_000_000_000_000)
	volcDefaultMaxLogBytes    = 512 * 1024
)

// VolcConfig configures a Volc TLS log store. The topic and compatible index
// must be provisioned by the operator before the process starts.
type VolcConfig struct {
	Endpoint        string `yaml:"endpoint"`
	Region          string `yaml:"region"`
	TopicID         string `yaml:"topic_id"`
	AccessKeyID     string `yaml:"access_key_id"`
	AccessKeySecret string `yaml:"access_key_secret"`
}

type volcProducer interface {
	SendLog(shardHash, topic, source, filename string, log *pb.Log, callback producer.CallBack) error
	Start()
	Close()
	ForceClose()
}

type volcClient interface {
	DescribeIndex(*tls.DescribeIndexRequest) (*tls.DescribeIndexResponse, error)
	SearchLogsV2(*tls.SearchLogsRequest) (*tls.SearchLogsResponse, error)
}

// VolcStore persists records in one externally provisioned Volc TLS topic.
type VolcStore struct {
	topicID     string
	client      volcClient
	writer      volcProducer
	close       sync.Once
	closed      atomic.Bool
	maxLogBytes int
}

// NewVolcStore validates the topic index without mutating it and starts the
// producer used by the store.
func NewVolcStore(config VolcConfig) (*VolcStore, error) {
	config, err := validateVolcConfig(config)
	if err != nil {
		return nil, err
	}
	client := tls.NewClient(config.Endpoint, config.AccessKeyID, config.AccessKeySecret, "", config.Region)
	client.SetTimeout(volcQueryTimeout)
	retryPolicy := client.GetRetryPolicy()
	retryPolicy.TotalTimeout = volcQueryTimeout
	client.SetRetryPolicy(retryPolicy)
	if err := validateVolcIndex(client, config.TopicID); err != nil {
		return nil, err
	}
	producerConfig := producer.GetDefaultProducerConfig()
	producerConfig.ClientConfig = common.ClientConfig{
		Endpoint:        config.Endpoint,
		AccessKeyID:     config.AccessKeyID,
		AccessKeySecret: config.AccessKeySecret,
		Region:          config.Region,
	}
	producerConfig.EnableNanosecond = true
	producerConfig.MaxBlockSec = int(volcQueryTimeout / time.Second)
	writer := producer.NewProducer(producerConfig)
	writer.Start()
	return &VolcStore{
		topicID:     config.TopicID,
		client:      client,
		writer:      writer,
		maxLogBytes: int(producerConfig.MaxBatchSize),
	}, nil
}

// Append validates the complete batch before accepting records into the Volc producer.
func (s *VolcStore) Append(ctx context.Context, records []Record) ([]RecordKey, error) {
	if len(records) == 0 {
		return []RecordKey{}, nil
	}
	if s == nil || s.writer == nil || s.topicID == "" {
		return nil, errors.New("logstore: volc store is not initialized")
	}
	if s.closed.Load() {
		return nil, errors.New("logstore: volc store is closed")
	}
	for _, record := range records {
		if err := ValidateRecord(record); err != nil {
			return nil, err
		}
	}
	keys := make([]RecordKey, 0, len(records))
	for index, record := range records {
		if err := ctx.Err(); err != nil {
			return keys, err
		}
		item, err := s.volcRecordForAppend(record)
		if err != nil {
			return keys, fmt.Errorf("logstore: encode Volc record %d: %w", index, err)
		}
		callCtx, cancel := cappedContext(ctx, volcQueryTimeout)
		err = callVolcSend(callCtx, s.writer, s.topicID, item)
		cancel()
		if err != nil {
			return keys, newVolcOperationError(fmt.Sprintf("append record %d", index), err)
		}
		keys = append(keys, record.Key())
	}
	return keys, nil
}

func (s *VolcStore) volcRecordForAppend(record Record) (*pb.Log, error) {
	limit := s.maxLogBytes
	if limit <= 0 {
		limit = volcDefaultMaxLogBytes
	}
	truncated := cloneRecord(record)
	payloadExhausted := false
	for {
		item, err := volcLog(truncated)
		if err != nil {
			return nil, err
		}
		if producer.GetLogSize(item) <= limit {
			return item, nil
		}
		switch {
		case truncated.Message != "":
			truncated.Message = truncateVolcString(truncated.Message)
		case truncateLargestVolcAttribute(truncated.Attributes):
		case truncated.Severity != "":
			truncated.Severity = truncateVolcString(truncated.Severity)
		case len(truncated.Payload) != 0 && !payloadExhausted:
			payload, changed, err := truncateVolcJSONPayload(truncated.Payload)
			if err != nil {
				return nil, err
			}
			if changed {
				truncated.Payload = payload
				continue
			}
			payloadExhausted = true
		default:
			return nil, fmt.Errorf("logstore: Volc record exceeds %d bytes after truncation", limit)
		}
	}
}

type volcJSONPathElement struct {
	key   string
	index int
	array bool
}

func truncateVolcJSONPayload(payload json.RawMessage) (json.RawMessage, bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false, fmt.Errorf("decode payload for truncation: %w", err)
	}
	compact, err := json.Marshal(value)
	if err != nil {
		return nil, false, fmt.Errorf("encode payload for truncation: %w", err)
	}
	if len(compact) < len(payload) {
		return compact, true, nil
	}
	path, largest := largestVolcJSONString(value)
	if largest == 0 {
		return payload, false, nil
	}
	if len(path) == 0 {
		truncated, err := json.Marshal(truncateVolcString(value.(string)))
		if err != nil {
			return nil, false, fmt.Errorf("encode truncated payload: %w", err)
		}
		return truncated, true, nil
	}
	setVolcJSONString(value, path, truncateVolcString(stringAtVolcJSONPath(value, path)))
	truncated, err := json.Marshal(value)
	if err != nil {
		return nil, false, fmt.Errorf("encode truncated payload: %w", err)
	}
	return truncated, true, nil
}

func largestVolcJSONString(value any) ([]volcJSONPathElement, int) {
	type node struct {
		value any
		path  []volcJSONPathElement
	}
	stack := []node{{value: value}}
	var selected []volcJSONPathElement
	selectedBytes := 0
	for len(stack) > 0 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		switch typed := current.value.(type) {
		case string:
			if len(typed) > selectedBytes {
				selected = current.path
				selectedBytes = len(typed)
			}
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			slices.Sort(keys)
			slices.Reverse(keys)
			for _, key := range keys {
				path := append(append([]volcJSONPathElement(nil), current.path...), volcJSONPathElement{key: key})
				stack = append(stack, node{value: typed[key], path: path})
			}
		case []any:
			for index := len(typed) - 1; index >= 0; index-- {
				path := append(append([]volcJSONPathElement(nil), current.path...), volcJSONPathElement{index: index, array: true})
				stack = append(stack, node{value: typed[index], path: path})
			}
		}
	}
	return selected, selectedBytes
}

func stringAtVolcJSONPath(value any, path []volcJSONPathElement) string {
	for _, element := range path {
		if element.array {
			value = value.([]any)[element.index]
		} else {
			value = value.(map[string]any)[element.key]
		}
	}
	return value.(string)
}

func setVolcJSONString(value any, path []volcJSONPathElement, replacement string) {
	for _, element := range path[:len(path)-1] {
		if element.array {
			value = value.([]any)[element.index]
		} else {
			value = value.(map[string]any)[element.key]
		}
	}
	last := path[len(path)-1]
	if last.array {
		value.([]any)[last.index] = replacement
		return
	}
	value.(map[string]any)[last.key] = replacement
}

func truncateVolcString(value string) string {
	if len(value) <= 1 {
		return ""
	}
	end := len(value) / 2
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}

func truncateLargestVolcAttribute(attributes map[string]string) bool {
	var selected string
	for name, value := range attributes {
		if selected == "" || len(value) > len(attributes[selected]) || len(value) == len(attributes[selected]) && name < selected {
			selected = name
		}
	}
	if selected == "" {
		return false
	}
	if attributes[selected] == "" {
		delete(attributes, selected)
		return true
	}
	attributes[selected] = truncateVolcString(attributes[selected])
	return true
}

// Query returns one provider-context-backed ordered page.
func (s *VolcStore) Query(ctx context.Context, query Query) (Page, error) {
	if err := ValidateQuery(query); err != nil {
		return Page{}, err
	}
	if err := ctx.Err(); err != nil {
		return Page{}, err
	}
	if s == nil || s.client == nil || s.topicID == "" {
		return Page{}, errors.New("logstore: volc store is not initialized")
	}
	if s.closed.Load() {
		return Page{}, errors.New("logstore: volc store is closed")
	}
	bound := normalizedVolcQuery(query)
	providerContext := ""
	if query.Cursor != "" {
		cursor, err := decodeVolcCursor(query.Cursor)
		if err != nil {
			return Page{}, err
		}
		if !equalVolcQuery(cursor.Query, bound) {
			return Page{}, fmt.Errorf("%w: query fields changed", ErrCursorMismatch)
		}
		providerContext = cursor.Context
	}
	expression := buildVolcQuery(bound)
	callCtx, cancel := cappedContext(ctx, volcQueryTimeout)
	defer cancel()
	response, err := callVolcSearch(callCtx, s.client, &tls.SearchLogsRequest{
		TopicID:   s.topicID,
		Query:     expression,
		StartTime: query.Start.UnixMilli(),
		EndTime:   query.End.UnixMilli(),
		Limit:     query.Limit,
		Context:   providerContext,
		Sort:      string(query.Order),
	})
	if err != nil {
		return Page{}, newVolcOperationError("query logs", err)
	}
	if response == nil {
		return Page{}, errors.New("logstore: query Volc logs returned an empty response")
	}
	if response.Status != "complete" {
		return Page{}, fmt.Errorf("logstore: query Volc logs returned status %q", response.Status)
	}
	if response.Analysis || response.AnalysisResult != nil {
		return Page{}, errors.New("logstore: query Volc logs unexpectedly returned an analysis response")
	}
	page := Page{Records: make([]Record, 0, len(response.Logs))}
	for index, raw := range response.Logs {
		record, err := recordFromVolc(raw)
		if err != nil {
			return Page{}, fmt.Errorf("logstore: normalize Volc record %d: %w", index, err)
		}
		page.Records = append(page.Records, record)
	}
	contextValue := strings.TrimSpace(response.Context)
	if !response.ListOver {
		if contextValue == "" {
			return Page{}, errors.New("logstore: query Volc logs omitted the continuation context")
		}
		if contextValue == providerContext {
			return Page{}, errors.New("logstore: query Volc logs returned a non-advancing context")
		}
		cursor, err := encodeVolcCursor(volcCursor{Version: 1, Query: bound, Context: contextValue})
		if err != nil {
			return Page{}, fmt.Errorf("logstore: encode Volc cursor: %w", err)
		}
		page.HasNext = true
		page.NextCursor = cursor
	}
	if err := ValidatePage(page, query.Limit); err != nil {
		return Page{}, err
	}
	return page, nil
}

// Close flushes and closes the Volc producer once.
func (s *VolcStore) Close() error {
	if s == nil {
		return nil
	}
	s.close.Do(func() {
		s.closed.Store(true)
		if s.writer != nil {
			s.writer.Close()
		}
	})
	return nil
}

func validateVolcConfig(config VolcConfig) (VolcConfig, error) {
	config.Endpoint = strings.TrimSpace(config.Endpoint)
	config.Region = strings.TrimSpace(config.Region)
	config.TopicID = strings.TrimSpace(config.TopicID)
	config.AccessKeyID = strings.TrimSpace(config.AccessKeyID)
	config.AccessKeySecret = strings.TrimSpace(config.AccessKeySecret)
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "endpoint", value: config.Endpoint},
		{name: "region", value: config.Region},
		{name: "topic_id", value: config.TopicID},
		{name: "access_key_id", value: config.AccessKeyID},
		{name: "access_key_secret", value: config.AccessKeySecret},
	} {
		if field.value == "" {
			return VolcConfig{}, fmt.Errorf("logstore: volc %s is required", field.name)
		}
	}
	return config, nil
}

func validateVolcIndex(client volcClient, topicID string) error {
	response, err := client.DescribeIndex(&tls.DescribeIndexRequest{TopicID: topicID})
	if err != nil {
		return newVolcOperationError("describe topic index", err)
	}
	if response == nil || response.KeyValue == nil {
		return errors.New("logstore: Volc topic requires a key-value index")
	}
	if response.FullText != nil {
		return errors.New("logstore: Volc topic full-text index must be disabled")
	}
	if response.EnableAutoIndex {
		return errors.New("logstore: Volc topic automatic index updates must be disabled")
	}
	if !response.EnablePhraseIndex {
		return errors.New("logstore: Volc topic phrase index must be enabled")
	}
	fields := make(map[string]tls.Value, len(*response.KeyValue))
	for _, item := range *response.KeyValue {
		fields[item.Key] = item.Value
	}
	if _, exists := fields["payload"]; exists {
		return errors.New("logstore: Volc topic payload field must remain unindexed")
	}
	if response.UserInnerKeyValue != nil {
		for _, item := range *response.UserInnerKeyValue {
			if item.Key == "payload" || strings.HasPrefix(item.Key, "payload.") {
				return errors.New("logstore: Volc topic payload field must remain unindexed")
			}
		}
	}
	for _, name := range []string{"id", "stream", "kind", "level"} {
		if err := validateVolcIndexValue(name, fields[name], tls.Value{ValueType: "text", CaseSensitive: true}); err != nil {
			return err
		}
	}
	if err := validateVolcIndexValue("msg", fields["msg"], tls.Value{ValueType: "text", Delimiter: volcMessageDelim, CaseSensitive: true, IncludeChinese: true}); err != nil {
		return err
	}
	if err := validateVolcIndexValue("attributes", fields["attributes"], tls.Value{ValueType: "json", CaseSensitive: true, IndexAll: true}); err != nil {
		return err
	}
	return nil
}

func validateVolcIndexValue(name string, got, want tls.Value) error {
	if got.ValueType != want.ValueType || got.Delimiter != want.Delimiter || got.CaseSensitive != want.CaseSensitive || got.IncludeChinese != want.IncludeChinese || got.SQLFlag || got.IndexAll != want.IndexAll {
		return fmt.Errorf("logstore: Volc topic index field %q is incompatible", name)
	}
	return nil
}

func volcLog(record Record) (*pb.Log, error) {
	attributes, err := nestedAttributes(record.Attributes)
	if err != nil {
		return nil, err
	}
	contents := []*pb.LogContent{
		{Key: "id", Value: record.ID},
		{Key: "stream", Value: record.Stream},
		{Key: "kind", Value: record.Kind},
		{Key: "level", Value: record.Severity},
		{Key: "msg", Value: record.Message},
		{Key: "attributes", Value: string(attributes)},
	}
	if len(record.Payload) != 0 {
		contents = append(contents, &pb.LogContent{Key: "payload", Value: string(record.Payload)})
	}
	item := &pb.Log{Time: record.Time.Unix(), Contents: contents}
	item.OptionalTimeNs = &pb.Log_TimeNs{TimeNs: uint32(record.Time.Nanosecond())}
	return item, nil
}

func nestedAttributes(attributes map[string]string) ([]byte, error) {
	root := make(map[string]any)
	for path, value := range attributes {
		parts := strings.Split(path, ".")
		current := root
		for _, part := range parts[:len(parts)-1] {
			next, ok := current[part].(map[string]any)
			if !ok {
				next = make(map[string]any)
				current[part] = next
			}
			current = next
		}
		current[parts[len(parts)-1]] = value
	}
	return json.Marshal(root)
}

type volcBoundQuery struct {
	Streams    []string           `json:"streams,omitempty"`
	Kinds      []string           `json:"kinds,omitempty"`
	Severities []string           `json:"severities,omitempty"`
	Matchers   []AttributeMatcher `json:"matchers,omitempty"`
	Text       string             `json:"text,omitempty"`
	StartMS    int64              `json:"start_ms"`
	EndMS      int64              `json:"end_ms"`
	Order      Order              `json:"order"`
}

type volcCursor struct {
	Version int            `json:"v"`
	Query   volcBoundQuery `json:"query"`
	Context string         `json:"context"`
}

func normalizedVolcQuery(query Query) volcBoundQuery {
	bound := volcBoundQuery{
		Streams: append([]string(nil), query.Streams...), Kinds: append([]string(nil), query.Kinds...),
		Severities: append([]string(nil), query.Severities...), Matchers: append([]AttributeMatcher(nil), query.Matchers...),
		Text: query.Text, StartMS: query.Start.UnixMilli(), EndMS: query.End.UnixMilli(), Order: query.Order,
	}
	for _, values := range [][]string{bound.Streams, bound.Kinds, bound.Severities} {
		sort.Strings(values)
	}
	for index := range bound.Matchers {
		if bound.Matchers[index].Op == MatchExists || bound.Matchers[index].Op == MatchNotExists {
			bound.Matchers[index].Value = ""
		}
	}
	sort.Slice(bound.Matchers, func(i, j int) bool {
		left, right := bound.Matchers[i], bound.Matchers[j]
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		if left.Op != right.Op {
			return left.Op < right.Op
		}
		return left.Value < right.Value
	})
	return bound
}

func equalVolcQuery(left, right volcBoundQuery) bool {
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	return string(a) == string(b)
}

func encodeVolcCursor(cursor volcCursor) (string, error) {
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(data)
	if len(encoded) > volcCursorLimit {
		return "", errors.New("logstore: Volc cursor is too large")
	}
	return encoded, nil
}

func decodeVolcCursor(value string) (volcCursor, error) {
	if len(value) > volcCursorLimit {
		return volcCursor{}, fmt.Errorf("%w: cursor is too large", ErrCursorMismatch)
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil || len(data) > volcCursorLimit {
		return volcCursor{}, fmt.Errorf("%w: cursor is malformed", ErrCursorMismatch)
	}
	var cursor volcCursor
	if err := json.Unmarshal(data, &cursor); err != nil || cursor.Version != 1 || strings.TrimSpace(cursor.Context) == "" {
		return volcCursor{}, fmt.Errorf("%w: cursor is invalid", ErrCursorMismatch)
	}
	return cursor, nil
}

func buildVolcQuery(query volcBoundQuery) string {
	modern := buildVolcBranch(query, false)
	if len(query.Streams) == 1 && query.Streams[0] == "system" && len(query.Kinds) == 1 && query.Kinds[0] == "log" {
		legacy := buildVolcBranch(query, true)
		return "(" + modern + ") OR (" + legacy + ")"
	}
	return modern
}

func buildVolcBranch(query volcBoundQuery, legacy bool) string {
	clauses := make([]string, 0, 8+len(query.Matchers))
	if legacy {
		clauses = append(clauses, `__source__:"gizclaw"`, `__path__:"slog"`)
	} else {
		clauses = appendVolcSet(clauses, "stream", query.Streams)
		clauses = appendVolcSet(clauses, "kind", query.Kinds)
	}
	clauses = appendVolcSet(clauses, "level", query.Severities)
	if query.Text != "" {
		clauses = append(clauses, "msg:#"+volcQuoted(query.Text))
	}
	for _, matcher := range query.Matchers {
		field := "attributes." + matcher.Name
		if legacy {
			switch matcher.Name {
			case "source":
				field = "__source__"
			case "path":
				field = "__path__"
			default:
				field = matcher.Name
			}
		}
		switch matcher.Op {
		case MatchEqual:
			clauses = append(clauses, field+":"+volcQuoted(matcher.Value))
		case MatchNotEqual:
			clauses = append(clauses, "("+field+":* AND NOT "+field+":"+volcQuoted(matcher.Value)+")")
		case MatchExists:
			clauses = append(clauses, field+":*")
		case MatchNotExists:
			clauses = append(clauses, "NOT "+field+":*")
		}
	}
	if len(clauses) == 0 {
		return "*"
	}
	return strings.Join(clauses, " AND ")
}

func appendVolcSet(clauses []string, field string, values []string) []string {
	if len(values) == 0 {
		return clauses
	}
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = append(items, field+":"+volcQuoted(value))
	}
	if len(items) == 1 {
		return append(clauses, items[0])
	}
	return append(clauses, "("+strings.Join(items, " OR ")+")")
}

func volcQuoted(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func callVolcSearch(ctx context.Context, client volcClient, request *tls.SearchLogsRequest) (*tls.SearchLogsResponse, error) {
	type result struct {
		response *tls.SearchLogsResponse
		err      error
	}
	done := make(chan result, 1)
	go func() {
		response, err := client.SearchLogsV2(request)
		done <- result{response: response, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case item := <-done:
		return item.response, item.err
	}
}

func callVolcSend(ctx context.Context, writer volcProducer, topicID string, item *pb.Log) error {
	done := make(chan error, 1)
	go func() {
		done <- writer.SendLog("", topicID, "gizclaw", "logstore", item, nil)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func cappedContext(ctx context.Context, limit time.Duration) (context.Context, context.CancelFunc) {
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= limit {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, limit)
}

func recordFromVolc(raw map[string]any) (Record, error) {
	recordTime := volcRecordTime(raw)
	legacySource := firstVolcString(raw, "__source__")
	legacyPath := firstVolcString(raw, "__path__", "__filename__")
	legacy := legacySource == "gizclaw" && legacyPath == "slog"
	record := Record{
		ID: firstVolcRawString(raw, "id"), Time: recordTime, Stream: firstVolcRawString(raw, "stream"),
		Kind: firstVolcRawString(raw, "kind"), Severity: firstVolcRawString(raw, "level"), Message: firstVolcRawString(raw, "msg", "message"),
		Attributes: make(map[string]string),
	}
	if legacy {
		record.ID, record.Stream, record.Kind = "", "system", "log"
	}
	if value, exists := raw["attributes"]; exists && !legacy {
		decoded, err := decodeVolcAttributes(value)
		if err != nil {
			return Record{}, err
		}
		flattenVolcAttributes(record.Attributes, "", decoded)
		if err := validateAttributes(record.Attributes); err != nil {
			return Record{}, fmt.Errorf("attributes: %w", err)
		}
	} else if legacy {
		keys := make([]string, 0, len(raw))
		for key := range raw {
			if !reservedLegacyVolcField(key) && ValidateAttributeName(key) == nil {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			setVolcAttribute(record.Attributes, key, volcString(raw[key]))
		}
		if legacySource != "" {
			setVolcAttribute(record.Attributes, "source", legacySource)
		}
		if legacyPath != "" {
			setVolcAttribute(record.Attributes, "path", legacyPath)
		}
	}
	if value, exists := raw["payload"]; exists && !legacy {
		payload := []byte(volcString(value))
		if !json.Valid(payload) {
			return Record{}, errors.New("payload is not valid JSON")
		}
		record.Payload = append(json.RawMessage(nil), payload...)
	}
	return record, nil
}

func volcRecordTime(raw map[string]any) time.Time {
	value, _ := firstVolcInt(raw, "__time__", "_time_", "Time", "time", "time_ms")
	nanoseconds, hasNanoseconds := firstVolcInt(raw, "__time_ns__", "_time_ns_", "time_ns")
	if hasNanoseconds && nanoseconds >= volcNanosecondsLowerBound {
		return time.Unix(0, nanoseconds).UTC()
	}
	if value > 0 && value < volcSecondsUpperBound {
		if hasNanoseconds && nanoseconds >= 0 && nanoseconds < int64(time.Second) {
			return time.Unix(value, nanoseconds).UTC()
		}
		return time.Unix(value, 0).UTC()
	}
	if value >= volcNanosecondsLowerBound {
		return time.Unix(0, value).UTC()
	}
	recordTime := time.UnixMilli(value).UTC()
	if hasNanoseconds && nanoseconds >= 0 && nanoseconds < int64(time.Millisecond) {
		return recordTime.Add(time.Duration(nanoseconds))
	}
	if hasNanoseconds && nanoseconds >= 0 && nanoseconds < int64(time.Second) {
		return time.Unix(recordTime.Unix(), nanoseconds).UTC()
	}
	return recordTime
}

type volcOperationError struct {
	operation string
	err       error
}

func newVolcOperationError(operation string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("logstore: Volc %s: %w", operation, err)
	}
	return &volcOperationError{operation: operation, err: err}
}

func (e *volcOperationError) Error() string {
	return "logstore: Volc " + e.operation + " failed"
}

func (e *volcOperationError) Unwrap() error { return e.err }

func decodeVolcAttributes(value any) (map[string]any, error) {
	if text, ok := value.(string); ok {
		var decoded any
		if err := json.Unmarshal([]byte(text), &decoded); err != nil {
			return nil, errors.New("attributes is not valid JSON")
		}
		value = decoded
	}
	if values, ok := value.(map[string]string); ok {
		attributes := make(map[string]any, len(values))
		for key, item := range values {
			attributes[key] = item
		}
		return attributes, nil
	}
	attributes, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("attributes is not a JSON object")
	}
	return attributes, nil
}

func flattenVolcAttributes(out map[string]string, prefix string, value any) {
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			flattenVolcAttributes(out, path, child)
		}
	case map[string]string:
		for key, child := range item {
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			out[path] = child
		}
	default:
		if prefix != "" {
			out[prefix] = volcString(item)
		}
	}
}

func setVolcAttribute(attributes map[string]string, key, value string) {
	for existing := range attributes {
		if existing == key || strings.HasPrefix(existing, key+".") || strings.HasPrefix(key, existing+".") {
			delete(attributes, existing)
		}
	}
	attributes[key] = value
}

func reservedLegacyVolcField(key string) bool {
	switch key {
	case "level", "msg", "message", "__time__", "_time_", "Time", "time", "__time_ns__", "_time_ns_", "__source__", "source", "__path__", "__filename__", "path":
		return true
	default:
		return false
	}
}

func firstVolcString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, exists := raw[key]; exists {
			if result := strings.TrimSpace(volcString(value)); result != "" {
				return result
			}
		}
	}
	return ""
}

func firstVolcRawString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, exists := raw[key]; exists {
			return volcString(value)
		}
	}
	return ""
}

func firstVolcInt(raw map[string]any, keys ...string) (int64, bool) {
	for _, key := range keys {
		value, exists := raw[key]
		if !exists {
			continue
		}
		switch item := value.(type) {
		case int64:
			return item, true
		case int:
			return int64(item), true
		case float64:
			return int64(item), true
		case json.Number:
			result, err := item.Int64()
			return result, err == nil
		case string:
			result, err := strconv.ParseInt(strings.TrimSpace(item), 10, 64)
			return result, err == nil
		}
	}
	return 0, false
}

func volcString(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	if number, ok := value.(json.Number); ok {
		return number.String()
	}
	data, err := json.Marshal(value)
	if err == nil && string(data) != "null" {
		return string(data)
	}
	return fmt.Sprint(value)
}
