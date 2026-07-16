package logstore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/volcengine/volc-sdk-golang/service/tls"
	"github.com/volcengine/volc-sdk-golang/service/tls/pb"
	"github.com/volcengine/volc-sdk-golang/service/tls/producer"
)

type fakeVolcClient struct {
	index    *tls.DescribeIndexResponse
	response *tls.SearchLogsResponse
	request  *tls.SearchLogsRequest
	err      error
	searches int
}

func (f *fakeVolcClient) DescribeIndex(*tls.DescribeIndexRequest) (*tls.DescribeIndexResponse, error) {
	return f.index, f.err
}

func (f *fakeVolcClient) SearchLogsV2(request *tls.SearchLogsRequest) (*tls.SearchLogsResponse, error) {
	f.searches++
	f.request = request
	return f.response, f.err
}

func TestVolcOperationErrorsHideProviderText(t *testing.T) {
	providerErr := errors.New("provider body contains access-secret")
	client := &fakeVolcClient{err: providerErr}
	if err := validateVolcIndex(client, "topic"); !errors.Is(err, providerErr) || strings.Contains(err.Error(), "access-secret") {
		t.Fatalf("index error = %v", err)
	}
	store := &VolcStore{topicID: "topic", client: client, writer: &fakeVolcProducer{err: providerErr}}
	if err := store.Append(context.Background(), []Record{validRecord()}); !errors.Is(err, providerErr) || strings.Contains(err.Error(), "access-secret") {
		t.Fatalf("append error = %v", err)
	}
	if _, err := store.Query(context.Background(), validQuery()); !errors.Is(err, providerErr) || strings.Contains(err.Error(), "access-secret") {
		t.Fatalf("query error = %v", err)
	}
}

func TestVolcQueryRejectsInvalidPayload(t *testing.T) {
	client := &fakeVolcClient{response: &tls.SearchLogsResponse{Status: "complete", ListOver: true, Logs: []map[string]any{{"payload": "{"}}}}
	store := &VolcStore{topicID: "topic", client: client, writer: &fakeVolcProducer{}}
	if _, err := store.Query(context.Background(), validQuery()); err == nil {
		t.Fatal("invalid payload was accepted")
	}
}

func TestVolcQueryPreservesStringAttributeLeaves(t *testing.T) {
	client := &fakeVolcClient{response: &tls.SearchLogsResponse{Status: "complete", ListOver: true, Logs: []map[string]any{{
		"attributes": `{"literal":"null","nested":{"value":"true"}}`,
	}}}}
	store := &VolcStore{topicID: "topic", client: client, writer: &fakeVolcProducer{}}
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
	store := &VolcStore{topicID: "topic", client: client, writer: &fakeVolcProducer{}}
	page, err := store.Query(context.Background(), validQuery())
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	got := page.Records[0]
	if got.ID != " id " || got.Stream != " system " || got.Kind != " log " || got.Severity != " WARN " || got.Message != " message " {
		t.Fatalf("record = %+v", got)
	}
}

type fakeVolcProducer struct {
	logs   []*pb.Log
	closed int
	err    error
	calls  int
	failAt int
}

func (f *fakeVolcProducer) SendLog(_, _, _, _ string, item *pb.Log, _ producer.CallBack) error {
	f.calls++
	if f.err != nil && (f.failAt == 0 || f.calls == f.failAt) {
		return f.err
	}
	f.logs = append(f.logs, item)
	return nil
}
func (*fakeVolcProducer) Start()      {}
func (f *fakeVolcProducer) Close()    { f.closed++ }
func (*fakeVolcProducer) ForceClose() {}

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

func TestValidateVolcIndexRejectsIncompatibleSchemas(t *testing.T) {
	tests := map[string]func(*tls.DescribeIndexResponse){
		"full text":  func(index *tls.DescribeIndexResponse) { index.FullText = &tls.FullTextInfo{} },
		"automatic":  func(index *tls.DescribeIndexResponse) { index.EnableAutoIndex = true },
		"phrase":     func(index *tls.DescribeIndexResponse) { index.EnablePhraseIndex = false },
		"missing id": func(index *tls.DescribeIndexResponse) { (*index.KeyValue)[0].Key = "other" },
		"message tokenizer": func(index *tls.DescribeIndexResponse) {
			(*index.KeyValue)[4].Value.Delimiter = ","
		},
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
	writer := &fakeVolcProducer{}
	store := &VolcStore{topicID: "topic", writer: writer}
	invalid := validRecord()
	invalid.Payload = []byte("{")
	if err := store.Append(context.Background(), []Record{validRecord(), invalid}); err == nil {
		t.Fatal("invalid batch was accepted")
	}
	if writer.calls != 0 {
		t.Fatalf("writer calls = %d before complete validation", writer.calls)
	}

	wantErr := errors.New("send failed")
	writer.err, writer.failAt = wantErr, 2
	if err := store.Append(context.Background(), []Record{validRecord(), validRecord()}); !errors.Is(err, wantErr) {
		t.Fatalf("Append() error = %v", err)
	}
	if writer.calls != 2 || len(writer.logs) != 1 {
		t.Fatalf("calls = %d, accepted = %d", writer.calls, len(writer.logs))
	}
}

func TestVolcAppendAndQuery(t *testing.T) {
	writer := &fakeVolcProducer{}
	client := &fakeVolcClient{response: &tls.SearchLogsResponse{
		Status: "complete", ListOver: false, Context: "next", Logs: []map[string]any{{
			"id": "id", "stream": "system", "kind": "log", "level": "WARN", "msg": "hello", "__time__": int64(1000),
			"attributes": map[string]any{"request": map[string]any{"method": "GET"}},
		}},
	}}
	store := &VolcStore{topicID: "topic", client: client, writer: writer}
	record := validRecord()
	if err := store.Append(context.Background(), []Record{record}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if len(writer.logs) != 1 || len(writer.logs[0].Contents) < 6 {
		t.Fatalf("appended logs = %+v", writer.logs)
	}
	contents := map[string]string{}
	for _, content := range writer.logs[0].Contents {
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
	if writer.closed != 1 {
		t.Fatalf("close count = %d", writer.closed)
	}
	if err := store.Append(context.Background(), []Record{validRecord()}); err == nil {
		t.Fatal("append after close succeeded")
	}
	if _, err := store.Query(context.Background(), validQuery()); err == nil {
		t.Fatal("query after close succeeded")
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
			store := &VolcStore{topicID: "topic", client: client, writer: &fakeVolcProducer{}}
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
	store := &VolcStore{topicID: "topic", client: client, writer: &fakeVolcProducer{}}
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

func TestVolcQueryHonorsEarlierDeadline(t *testing.T) {
	client := &blockingVolcClient{release: make(chan struct{})}
	t.Cleanup(func() { close(client.release) })
	store := &VolcStore{topicID: "topic", client: client, writer: &fakeVolcProducer{}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := store.Query(ctx, validQuery()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Query() error = %v", err)
	}
}

type blockingVolcProducer struct{ release chan struct{} }

func (p *blockingVolcProducer) SendLog(string, string, string, string, *pb.Log, producer.CallBack) error {
	<-p.release
	return nil
}
func (*blockingVolcProducer) Start()      {}
func (*blockingVolcProducer) Close()      {}
func (*blockingVolcProducer) ForceClose() {}

func TestVolcAppendHonorsEarlierDeadline(t *testing.T) {
	writer := &blockingVolcProducer{release: make(chan struct{})}
	t.Cleanup(func() { close(writer.release) })
	store := &VolcStore{topicID: "topic", writer: writer}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := store.Append(ctx, []Record{validRecord()}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Append() error = %v", err)
	}
}

func TestVolcQueryCancellation(t *testing.T) {
	client := &fakeVolcClient{response: &tls.SearchLogsResponse{Status: "complete", ListOver: true}}
	store := &VolcStore{topicID: "topic", client: client, writer: &fakeVolcProducer{}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Query(ctx, validQuery()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Query() error = %v, want canceled", err)
	}
}

func TestVolcCursorIgnoresExistenceMatcherValue(t *testing.T) {
	client := &fakeVolcClient{response: &tls.SearchLogsResponse{Status: "complete", ListOver: false, Context: "next"}}
	store := &VolcStore{topicID: "topic", client: client, writer: &fakeVolcProducer{}}
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
