package logstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/volcengine/volc-sdk-golang/service/tls"
)

func TestVolcConfigsHaveNoSerializationTags(t *testing.T) {
	for _, typeOf := range []reflect.Type{reflect.TypeFor[VolcConfig](), reflect.TypeFor[VolcConnectorConfig]()} {
		for field := range typeOf.Fields() {
			if field.Tag.Get("yaml") != "" || field.Tag.Get("json") != "" || field.Tag.Get("mapstructure") != "" {
				t.Fatalf("%s.%s has serialization tag %q", typeOf.Name(), field.Name, field.Tag)
			}
		}
	}
}

type fakeVolcClient struct {
	index    *tls.DescribeIndexResponse
	response *tls.SearchLogsResponse
	request  *tls.SearchLogsRequest
	err      error
	searches int
	puts     int
	failAt   int
	logs     []tls.Log
	release  chan struct{}
}

func (f *fakeVolcClient) DescribeIndex(*tls.DescribeIndexRequest) (*tls.DescribeIndexResponse, error) {
	return f.index, f.err
}

func (f *fakeVolcClient) SearchLogsV2(request *tls.SearchLogsRequest) (*tls.SearchLogsResponse, error) {
	f.searches++
	f.request = request
	return f.response, f.err
}

func (f *fakeVolcClient) PutLogsV2(request *tls.PutLogsV2Request) (*tls.CommonResponse, error) {
	f.puts++
	if f.release != nil {
		<-f.release
	}
	if f.err != nil && (f.failAt == 0 || f.puts == f.failAt) {
		return nil, f.err
	}
	f.logs = append(f.logs, request.Logs...)
	return &tls.CommonResponse{}, nil
}

func TestVolcOperationErrorsHideProviderText(t *testing.T) {
	providerErr := errors.New("provider body contains access-secret")
	client := &fakeVolcClient{err: providerErr}
	if err := validateVolcIndex(client, "topic"); !errors.Is(err, providerErr) || strings.Contains(err.Error(), "access-secret") {
		t.Fatalf("index error = %v", err)
	}
	store := &VolcStore{topicID: "topic", client: client}
	if _, err := store.Append(context.Background(), []Record{validRecord()}); !errors.Is(err, providerErr) || strings.Contains(err.Error(), "access-secret") {
		t.Fatalf("append error = %v", err)
	}
	if _, err := store.Query(context.Background(), validQuery()); !errors.Is(err, providerErr) || strings.Contains(err.Error(), "access-secret") {
		t.Fatalf("query error = %v", err)
	}
}

func TestVolcQueryRejectsInvalidPayload(t *testing.T) {
	client := &fakeVolcClient{response: &tls.SearchLogsResponse{Status: "complete", ListOver: true, Logs: []map[string]any{{"payload": "{"}}}}
	store := &VolcStore{topicID: "topic", client: client}
	if _, err := store.Query(context.Background(), validQuery()); err == nil {
		t.Fatal("invalid payload was accepted")
	}
}

func TestVolcQueryPreservesStringAttributeLeaves(t *testing.T) {
	client := &fakeVolcClient{response: &tls.SearchLogsResponse{Status: "complete", ListOver: true, Logs: []map[string]any{{
		"attributes": `{"literal":"null","nested":{"value":"true"}}`,
	}}}}
	store := &VolcStore{topicID: "topic", client: client}
	page, err := store.Query(context.Background(), validQuery())
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if got := page.Records[0].Attributes; got["literal"] != "null" || got["nested.value"] != "true" {
		t.Fatalf("attributes = %+v", got)
	}
	client.response.Logs[0]["attributes"] = "{"
	if _, err := store.Query(context.Background(), validQuery()); err == nil {
		t.Fatal("invalid attributes JSON was accepted")
	}
}

func TestVolcQueryPreservesFixedFieldWhitespace(t *testing.T) {
	client := &fakeVolcClient{response: &tls.SearchLogsResponse{Status: "complete", ListOver: true, Logs: []map[string]any{{
		"id": " id ", "stream": " system ", "kind": " log ", "level": " WARN ", "msg": " message ",
	}}}}
	store := &VolcStore{topicID: "topic", client: client}
	page, err := store.Query(context.Background(), validQuery())
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	got := page.Records[0]
	if got.ID != " id " || got.Stream != " system " || got.Kind != " log " || got.Severity != " WARN " || got.Message != " message " {
		t.Fatalf("record = %+v", got)
	}
}

func compatibleVolcIndex() *tls.DescribeIndexResponse {
	values := []tls.KeyValueInfo{
		{Key: "id", Value: tls.Value{ValueType: "text", CaseSensitive: true}},
		{Key: "stream", Value: tls.Value{ValueType: "text", CaseSensitive: true}},
		{Key: "kind", Value: tls.Value{ValueType: "text", CaseSensitive: true}},
		{Key: "level", Value: tls.Value{ValueType: "text", CaseSensitive: true}},
		{Key: "msg", Value: tls.Value{ValueType: "text", Delimiter: volcMessageDelim, CaseSensitive: true, IncludeChinese: true}},
		{Key: "attributes", Value: tls.Value{ValueType: "json", CaseSensitive: true, IndexAll: true}},
	}
	return &tls.DescribeIndexResponse{KeyValue: &values, EnablePhraseIndex: true}
}

func TestVolcConnectorScopesTopicsAndBorrowsClient(t *testing.T) {
	client := &fakeVolcClient{index: compatibleVolcIndex()}
	connector := &VolcConnector{
		client: client,
	}
	first, err := connector.Store("topic-one")
	if err != nil {
		t.Fatal(err)
	}
	second, err := connector.Store("topic-two")
	if err != nil {
		t.Fatal(err)
	}
	if first.topicID == second.topicID || first.client != second.client {
		t.Fatalf("topic stores = %+v, %+v", first, second)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := connector.Close(); err != nil {
		t.Fatal(err)
	}
	if err := connector.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateVolcIndex(t *testing.T) {
	if err := validateVolcIndex(&fakeVolcClient{index: compatibleVolcIndex()}, "topic"); err != nil {
		t.Fatalf("validateVolcIndex() error = %v", err)
	}
	bad := compatibleVolcIndex()
	bad.EnableAutoIndex = true
	if err := validateVolcIndex(&fakeVolcClient{index: bad}, "topic"); err == nil {
		t.Fatal("auto index was accepted")
	}
}

func TestValidateVolcIndexAcceptsMessageDelimiters(t *testing.T) {
	tests := map[string]string{
		"logical whitespace": " \t\r\n",
		"Volc escaped text":  " \\t\\r\\n",
	}
	for name, delimiter := range tests {
		t.Run(name, func(t *testing.T) {
			index := compatibleVolcIndex()
			(*index.KeyValue)[4].Value.Delimiter = delimiter
			if err := validateVolcIndex(&fakeVolcClient{index: index}, "topic"); err != nil {
				t.Fatalf("validateVolcIndex() error = %v", err)
			}
		})
	}
}

func TestValidateVolcIndexRejectsMessageDelimiters(t *testing.T) {
	tests := map[string]string{
		"comma":              ",",
		"missing newline":    " \t\r",
		"reordered":          " \r\t\n",
		"partial escape":     " \\t\\r",
		"extra byte":         " \\t\\r\\n ",
		"double backslashes": " \\\\t\\\\r\\\\n",
		"arbitrary escape":   " \\x09\\r\\n",
	}
	for name, delimiter := range tests {
		t.Run(name, func(t *testing.T) {
			index := compatibleVolcIndex()
			(*index.KeyValue)[4].Value.Delimiter = delimiter
			if err := validateVolcIndex(&fakeVolcClient{index: index}, "topic"); err == nil {
				t.Fatal("incompatible delimiter was accepted")
			}
		})
	}
}

func TestValidateVolcIndexRejectsIncompatibleSchemas(t *testing.T) {
	tests := map[string]func(*tls.DescribeIndexResponse){
		"full text":        func(index *tls.DescribeIndexResponse) { index.FullText = &tls.FullTextInfo{} },
		"automatic":        func(index *tls.DescribeIndexResponse) { index.EnableAutoIndex = true },
		"phrase":           func(index *tls.DescribeIndexResponse) { index.EnablePhraseIndex = false },
		"missing id":       func(index *tls.DescribeIndexResponse) { (*index.KeyValue)[0].Key = "other" },
		"attributes index": func(index *tls.DescribeIndexResponse) { (*index.KeyValue)[5].Value.IndexAll = false },
		"payload index": func(index *tls.DescribeIndexResponse) {
			*index.KeyValue = append(*index.KeyValue, tls.KeyValueInfo{Key: "payload", Value: tls.Value{ValueType: "json"}})
		},
	}
	for name, edit := range tests {
		t.Run(name, func(t *testing.T) {
			index := compatibleVolcIndex()
			edit(index)
			if err := validateVolcIndex(&fakeVolcClient{index: index}, "topic"); err == nil {
				t.Fatal("incompatible index was accepted")
			}
		})
	}
	if err := validateVolcIndex(&fakeVolcClient{}, "topic"); err == nil {
		t.Fatal("missing index was accepted")
	}
}

func TestVolcAppendValidatesWholeBatchBeforeSendingAndReportsPartialFailure(t *testing.T) {
	client := &fakeVolcClient{}
	store := &VolcStore{topicID: "topic", client: client}
	invalid := validRecord()
	invalid.Payload = []byte("{")
	if _, err := store.Append(context.Background(), []Record{validRecord(), invalid}); err == nil {
		t.Fatal("invalid batch was accepted")
	}
	if client.puts != 0 {
		t.Fatalf("PutLogsV2 calls = %d before complete validation", client.puts)
	}

	wantErr := errors.New("send failed")
	client.err = wantErr
	keys, err := store.Append(context.Background(), []Record{validRecord()})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Append() error = %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("Append() keys = %+v, want no accepted records", keys)
	}
	if client.puts != 1 || len(client.logs) != 0 {
		t.Fatalf("calls = %d, accepted = %d", client.puts, len(client.logs))
	}

	client.puts, client.logs, client.failAt = 0, nil, 2
	records := make([]Record, volcMaxBatchRecords+1)
	for index := range records {
		records[index] = validRecord()
		records[index].ID = fmt.Sprintf("id-%d", index)
	}
	keys, err = store.Append(context.Background(), records)
	if !errors.Is(err, wantErr) {
		t.Fatalf("second batch error = %v", err)
	}
	if len(keys) == 0 || len(keys) != len(client.logs) {
		t.Fatalf("accepted prefix = %d, committed records = %d", len(keys), len(client.logs))
	}
}

func TestVolcAppendAndQuery(t *testing.T) {
	client := &fakeVolcClient{response: &tls.SearchLogsResponse{
		Status: "complete", ListOver: false, Context: "next", Logs: []map[string]any{{
			"id": "id", "stream": "system", "kind": "log", "level": "WARN", "msg": "hello", "__time__": int64(1000),
			"attributes": map[string]any{"request": map[string]any{"method": "GET"}},
		}},
	}}
	store := &VolcStore{topicID: "topic", client: client}
	record := validRecord()
	keys, err := store.Append(context.Background(), []Record{record})
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if len(keys) != 1 || keys[0] != record.Key() {
		t.Fatalf("Append() keys = %+v, want %+v", keys, []RecordKey{record.Key()})
	}
	if len(client.logs) != 1 || len(client.logs[0].Contents) < 6 {
		t.Fatalf("appended logs = %+v", client.logs)
	}
	contents := map[string]string{}
	for _, content := range client.logs[0].Contents {
		contents[content.Key] = content.Value
	}
	if contents["id"] != record.ID || contents["stream"] != record.Stream || contents["payload"] != string(record.Payload) || contents["attributes"] != `{"request":{"method":"GET"}}` {
		t.Fatalf("contents = %+v", contents)
	}
	query := validQuery()
	query.Streams, query.Kinds = []string{"system"}, []string{"log"}
	query.Severities = []string{"WARN"}
	query.Text = `hello "world"`
	query.Matchers = []AttributeMatcher{{Name: "request.method", Op: MatchNotEqual, Value: "POST"}}
	page, err := store.Query(context.Background(), query)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if !page.HasNext || page.NextCursor == "" || page.Records[0].Attributes["request.method"] != "GET" {
		t.Fatalf("page = %+v", page)
	}
	if strings.Contains(client.request.Query, "SELECT") || !strings.Contains(client.request.Query, `msg:#"hello \"world\""`) || !strings.Contains(client.request.Query, `__source__:"gizclaw"`) {
		t.Fatalf("provider query = %q", client.request.Query)
	}

	client.response = &tls.SearchLogsResponse{Status: "complete", ListOver: true}
	query.Cursor = page.NextCursor
	query.Limit = 1
	if _, err := store.Query(context.Background(), query); err != nil {
		t.Fatalf("continuation with changed limit error = %v", err)
	}
	query.Text = "changed"
	if _, err := store.Query(context.Background(), query); !errors.Is(err, ErrCursorMismatch) {
		t.Fatalf("changed query error = %v, want cursor mismatch", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	if _, err := store.Append(context.Background(), []Record{validRecord()}); err == nil {
		t.Fatal("append after close succeeded")
	}
	if _, err := store.Query(context.Background(), validQuery()); err == nil {
		t.Fatal("query after close succeeded")
	}
}

func TestVolcAppendTruncatesOversizedRecordBeforeProducer(t *testing.T) {
	client := &fakeVolcClient{}
	store := &VolcStore{topicID: "topic", client: client, maxLogBytes: 1024}
	record := validRecord()
	record.Message = strings.Repeat("message", 1024)
	record.Attributes["large"] = strings.Repeat("attribute", 1024)
	payload, err := json.Marshal(map[string]any{
		"version": 1,
		"message": map[string]any{
			"role":    "user",
			"content": strings.Repeat("payload", 1024),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	record.Payload = payload
	keys, err := store.Append(context.Background(), []Record{record})
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0] != record.Key() {
		t.Fatalf("keys = %+v", keys)
	}
	if len(client.logs) != 1 {
		t.Fatalf("appended logs = %d", len(client.logs))
	}
	contents := make(map[string]string)
	for _, content := range client.logs[0].Contents {
		contents[content.Key] = content.Value
	}
	var truncated struct {
		Version int `json:"version"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(contents["payload"]), &truncated); err != nil {
		t.Fatalf("payload = %q: %v", contents["payload"], err)
	}
	if truncated.Version != 1 || truncated.Message.Role != "user" || len(truncated.Message.Content) >= 7*1024 {
		t.Fatalf("payload = %q", contents["payload"])
	}
	if contents["id"] != record.ID || contents["stream"] != record.Stream {
		t.Fatalf("identity fields = %+v", contents)
	}
}

func TestVolcLogUsesProducerTimestampUnits(t *testing.T) {
	record := validRecord()
	record.Time = time.Unix(1_700_000_000, 123_456_789).UTC()
	item, err := volcLog(record)
	if err != nil {
		t.Fatalf("volcLog() error = %v", err)
	}
	if item.Time != record.Time.Unix() {
		t.Fatalf("Time = %d, want Unix seconds %d", item.Time, record.Time.Unix())
	}
	if item.GetTimeNs() != uint32(record.Time.Nanosecond()) {
		t.Fatalf("TimeNs = %d, want nanosecond fraction %d", item.GetTimeNs(), record.Time.Nanosecond())
	}
}

func TestBuildVolcQueryTranslatesAllStructuredOperators(t *testing.T) {
	query := normalizedVolcQuery(Query{
		Streams: []string{"event", "chat"}, Kinds: []string{"created"}, Severities: []string{"WARN"}, Text: `failed "request"`,
		Matchers: []AttributeMatcher{
			{Name: "http-method", Op: MatchEqual, Value: "1"},
			{Name: "b", Op: MatchNotEqual, Value: "2"},
			{Name: "c", Op: MatchExists},
			{Name: "d", Op: MatchNotExists},
		},
		Start: time.UnixMilli(1), End: time.UnixMilli(2), Order: OrderDesc,
	})
	expression := buildVolcQuery(query)
	for _, fragment := range []string{
		`(stream:"chat" OR stream:"event")`, `kind:"created"`, `level:"WARN"`, `msg:#"failed \"request\""`,
		`attributes.http-method:"1"`, `(attributes.b:* AND NOT attributes.b:"2")`, `attributes.c:*`, `NOT attributes.d:*`,
	} {
		if !strings.Contains(expression, fragment) {
			t.Errorf("query %q does not contain %q", expression, fragment)
		}
	}
	if strings.Contains(expression, "SELECT") {
		t.Fatalf("query contains analysis pipeline: %q", expression)
	}
}

func TestVolcQueryRejectsInvalidProviderPages(t *testing.T) {
	analysis := &tls.AnalysisResult{}
	tests := map[string]*tls.SearchLogsResponse{
		"nil":              nil,
		"incomplete":       {Status: "incomplete", ListOver: true},
		"analysis flag":    {Status: "complete", Analysis: true, ListOver: true},
		"analysis result":  {Status: "complete", AnalysisResult: analysis, ListOver: true},
		"missing context":  {Status: "complete", ListOver: false},
		"too many records": {Status: "complete", ListOver: true, Logs: []map[string]any{{}, {}}},
	}
	for name, response := range tests {
		t.Run(name, func(t *testing.T) {
			client := &fakeVolcClient{response: response}
			store := &VolcStore{topicID: "topic", client: client}
			query := validQuery()
			if name == "too many records" {
				query.Limit = 1
			}
			if _, err := store.Query(context.Background(), query); err == nil {
				t.Fatal("invalid provider page was accepted")
			}
		})
	}
}

func TestVolcQueryNormalizesLegacyRecordAndNanoseconds(t *testing.T) {
	timeMS := int64(1_700_000_000_123)
	client := &fakeVolcClient{response: &tls.SearchLogsResponse{Status: "complete", ListOver: true, Logs: []map[string]any{{
		"__time__": timeMS, "__time_ns__": int64(456_789), "__source__": "gizclaw", "__path__": "slog",
		"level": "WARN", "msg": "legacy", "id": "legacy-id", "stream": "user-stream", "kind": "user-kind",
		"payload": "legacy-payload", "request.method": "GET", "attributes": "legacy-value",
	}}}}
	store := &VolcStore{topicID: "topic", client: client}
	query := validQuery()
	query.Streams, query.Kinds = []string{"system"}, []string{"log"}
	page, err := store.Query(context.Background(), query)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	record := page.Records[0]
	if record.Stream != "system" || record.Kind != "log" || record.ID != "" || record.Time.UnixNano() != time.UnixMilli(timeMS).UnixNano()+456_789 {
		t.Fatalf("record = %+v", record)
	}
	if len(record.Payload) != 0 {
		t.Fatalf("payload = %s", record.Payload)
	}
	if record.Attributes["source"] != "gizclaw" || record.Attributes["path"] != "slog" || record.Attributes["id"] != "legacy-id" || record.Attributes["stream"] != "user-stream" || record.Attributes["kind"] != "user-kind" || record.Attributes["payload"] != "legacy-payload" || record.Attributes["request.method"] != "GET" || record.Attributes["attributes"] != "legacy-value" {
		t.Fatalf("attributes = %+v", record.Attributes)
	}
}

func TestVolcRecordTimeNormalizesProviderUnits(t *testing.T) {
	tests := map[string]struct {
		raw  map[string]any
		want time.Time
	}{
		"seconds with fraction": {
			raw:  map[string]any{"__time__": int64(1_700_000_000), "__time_ns__": int64(123_456_789)},
			want: time.Unix(1_700_000_000, 123_456_789).UTC(),
		},
		"milliseconds with remainder": {
			raw:  map[string]any{"__time__": int64(1_700_000_000_123), "__time_ns__": int64(456_789)},
			want: time.UnixMilli(1_700_000_000_123).Add(456_789 * time.Nanosecond).UTC(),
		},
		"nanoseconds": {
			raw:  map[string]any{"__time__": int64(1_700_000_000_123_456_789)},
			want: time.Unix(0, 1_700_000_000_123_456_789).UTC(),
		},
		"absolute nanosecond field": {
			raw:  map[string]any{"__time__": int64(1_700_000_000), "__time_ns__": int64(1_700_000_000_123_456_789)},
			want: time.Unix(0, 1_700_000_000_123_456_789).UTC(),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := volcRecordTime(test.raw); !got.Equal(test.want) {
				t.Fatalf("volcRecordTime() = %v, want %v", got, test.want)
			}
		})
	}
}

type blockingVolcClient struct{ release chan struct{} }

func (*blockingVolcClient) DescribeIndex(*tls.DescribeIndexRequest) (*tls.DescribeIndexResponse, error) {
	return nil, nil
}

func (c *blockingVolcClient) SearchLogsV2(*tls.SearchLogsRequest) (*tls.SearchLogsResponse, error) {
	<-c.release
	return &tls.SearchLogsResponse{Status: "complete", ListOver: true}, nil
}

func (c *blockingVolcClient) PutLogsV2(*tls.PutLogsV2Request) (*tls.CommonResponse, error) {
	<-c.release
	return &tls.CommonResponse{}, nil
}

func TestVolcQueryHonorsEarlierDeadline(t *testing.T) {
	client := &blockingVolcClient{release: make(chan struct{})}
	t.Cleanup(func() { close(client.release) })
	store := &VolcStore{topicID: "topic", client: client}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := store.Query(ctx, validQuery()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Query() error = %v", err)
	}
}

func TestVolcAppendHonorsEarlierDeadline(t *testing.T) {
	client := &fakeVolcClient{release: make(chan struct{})}
	t.Cleanup(func() { close(client.release) })
	store := &VolcStore{topicID: "topic", client: client}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := store.Append(ctx, []Record{validRecord()}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Append() error = %v", err)
	}
}

func TestVolcQueryCancellation(t *testing.T) {
	client := &fakeVolcClient{response: &tls.SearchLogsResponse{Status: "complete", ListOver: true}}
	store := &VolcStore{topicID: "topic", client: client}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Query(ctx, validQuery()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Query() error = %v, want canceled", err)
	}
}

func TestVolcCursorIgnoresExistenceMatcherValue(t *testing.T) {
	client := &fakeVolcClient{response: &tls.SearchLogsResponse{Status: "complete", ListOver: false, Context: "next"}}
	store := &VolcStore{topicID: "topic", client: client}
	query := validQuery()
	query.Matchers = []AttributeMatcher{{Name: "request.id", Op: MatchExists, Value: "ignored"}}
	page, err := store.Query(context.Background(), query)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	client.response = &tls.SearchLogsResponse{Status: "complete", ListOver: true}
	query.Cursor = page.NextCursor
	query.Matchers[0].Value = "also-ignored"
	if _, err := store.Query(context.Background(), query); err != nil {
		t.Fatalf("continuation error = %v", err)
	}
}

func TestVolcCursorSizeIsBoundedInBothDirections(t *testing.T) {
	if _, err := encodeVolcCursor(volcCursor{Version: 1, Context: strings.Repeat("x", volcCursorLimit)}); err == nil {
		t.Fatalf("encode error = %v", err)
	}
	if _, err := decodeVolcCursor(strings.Repeat("x", volcCursorLimit+1)); !errors.Is(err, ErrCursorMismatch) {
		t.Fatalf("decode error = %v", err)
	}
}
