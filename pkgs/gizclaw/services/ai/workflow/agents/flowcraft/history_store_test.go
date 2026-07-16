package flowcraft

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	flowmodel "github.com/GizClaw/flowcraft/sdk/model"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

type memoryHistoryLogStore struct {
	mu sync.Mutex

	records []logstore.Record

	appendErr    error
	queryErr     error
	badKeys      bool
	replaceCalls int
	failReplace  int
	queryCalls   int
}

func (store *memoryHistoryLogStore) Append(
	ctx context.Context,
	records []logstore.Record,
) ([]logstore.RecordKey, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if store.appendErr != nil {
		return nil, store.appendErr
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	keys := make([]logstore.RecordKey, len(records))
	for index, record := range records {
		store.records = append(store.records, cloneHistoryRecord(record))
		keys[index] = record.Key()
	}
	if store.badKeys && len(keys) > 0 {
		keys[0].ID = "wrong"
	}
	return keys, nil
}

func (store *memoryHistoryLogStore) Query(
	ctx context.Context,
	query logstore.Query,
) (logstore.Page, error) {
	if err := ctx.Err(); err != nil {
		return logstore.Page{}, err
	}
	if store.queryErr != nil {
		return logstore.Page{}, store.queryErr
	}
	if err := logstore.ValidateQuery(query); err != nil {
		return logstore.Page{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.queryCalls++
	var records []logstore.Record
	for _, record := range store.records {
		if record.Time.Before(query.Start) || !record.Time.Before(query.End) {
			continue
		}
		if !historySelectorContains(query.Streams, record.Stream) ||
			!historySelectorContains(query.Kinds, record.Kind) ||
			!historySelectorContains(query.Severities, record.Severity) {
			continue
		}
		matched := true
		for _, matcher := range query.Matchers {
			value, exists := record.Attributes[matcher.Name]
			switch matcher.Op {
			case logstore.MatchEqual:
				matched = matched && exists && value == matcher.Value
			case logstore.MatchNotEqual:
				matched = matched && (!exists || value != matcher.Value)
			case logstore.MatchExists:
				matched = matched && exists
			case logstore.MatchNotExists:
				matched = matched && !exists
			}
		}
		if matched {
			records = append(records, cloneHistoryRecord(record))
		}
	}
	sort.Slice(records, func(left, right int) bool {
		if records[left].Time.Equal(records[right].Time) {
			return records[left].ID < records[right].ID
		}
		return records[left].Time.Before(records[right].Time)
	})
	if query.Order == logstore.OrderDesc {
		slices.Reverse(records)
	}
	offset := 0
	if query.Cursor != "" {
		var err error
		offset, err = strconv.Atoi(query.Cursor)
		if err != nil {
			return logstore.Page{}, err
		}
	}
	if offset > len(records) {
		offset = len(records)
	}
	end := min(offset+query.Limit, len(records))
	page := logstore.Page{Records: records[offset:end]}
	if end < len(records) {
		page.HasNext = true
		page.NextCursor = strconv.Itoa(end)
	}
	return page, nil
}

func (store *memoryHistoryLogStore) Replace(ctx context.Context, record logstore.Record) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.replaceCalls++
	if store.failReplace > 0 && store.replaceCalls == store.failReplace {
		return errors.New("replace failed")
	}
	for index := range store.records {
		if store.records[index].Key() == record.Key() {
			store.records[index] = cloneHistoryRecord(record)
			return nil
		}
	}
	return logstore.ErrNotFound
}

func (store *memoryHistoryLogStore) Delete(ctx context.Context, key logstore.RecordKey) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	for index := range store.records {
		if store.records[index].Key() == key {
			store.records = append(store.records[:index], store.records[index+1:]...)
			return nil
		}
	}
	return logstore.ErrNotFound
}

func (*memoryHistoryLogStore) Close() error { return nil }

func (store *memoryHistoryLogStore) snapshot() []logstore.Record {
	store.mu.Lock()
	defer store.mu.Unlock()
	records := make([]logstore.Record, len(store.records))
	for index, record := range store.records {
		records[index] = cloneHistoryRecord(record)
	}
	return records
}

func historySelectorContains(values []string, value string) bool {
	return len(values) == 0 || slices.Contains(values, value)
}

func cloneHistoryRecord(record logstore.Record) logstore.Record {
	clone := record
	clone.Attributes = make(map[string]string, len(record.Attributes))
	maps.Copy(clone.Attributes, record.Attributes)
	clone.Payload = append([]byte(nil), record.Payload...)
	return clone
}

func TestHistoryStoreAppendReadRecentAndWorkspaceIsolation(t *testing.T) {
	backend := &memoryHistoryLogStore{}
	first, err := NewHistoryStore(backend, "workspace-a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewHistoryStore(backend, "workspace-b")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 100).UTC()
	first.now = func() time.Time { return now }
	second.now = func() time.Time { return now.Add(time.Second) }
	messages := historyTestMessages()
	if err := first.AppendMessages(context.Background(), "conversation", messages); err != nil {
		t.Fatal(err)
	}
	if err := second.AppendMessages(context.Background(), "conversation", messages[:1]); err != nil {
		t.Fatal(err)
	}
	got, err := first.GetMessages(context.Background(), "conversation")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, messages) {
		t.Fatalf("GetMessages() = %#v, want %#v", got, messages)
	}
	recent, err := first.GetRecentMessages(context.Background(), "conversation", 2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(recent, messages[len(messages)-2:]) {
		t.Fatalf("GetRecentMessages() = %#v", recent)
	}
	isolated, err := second.GetMessages(context.Background(), "conversation")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(isolated, messages[:1]) {
		t.Fatalf("workspace-b messages = %#v", isolated)
	}
	for _, record := range backend.snapshot() {
		if record.Message != "" || record.Kind != flowcraftHistoryKind || record.Stream != flowcraftHistoryStream {
			t.Fatalf("record = %+v", record)
		}
		if record.Attributes["schema_version"] != "1" {
			t.Fatalf("attributes = %+v", record.Attributes)
		}
	}
}

func TestHistoryStoreSaveDeleteAndAppendAfterDelete(t *testing.T) {
	backend := &memoryHistoryLogStore{}
	store, err := NewHistoryStore(backend, "workspace")
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }
	initial := historyTestMessages()
	if err := store.AppendMessages(context.Background(), "conversation", initial); err != nil {
		t.Fatal(err)
	}
	before := backend.snapshot()
	shorter := []flowmodel.Message{
		flowmodel.NewTextMessage(flowmodel.RoleSystem, "replacement"),
		flowmodel.NewTextMessage(flowmodel.RoleUser, "second"),
	}
	if err := store.SaveMessages(context.Background(), "conversation", shorter); err != nil {
		t.Fatal(err)
	}
	after := backend.snapshot()
	if len(after) != len(shorter) {
		t.Fatalf("records after shorter save = %d", len(after))
	}
	for index := range after {
		if after[index].Key() != before[index].Key() || !after[index].Time.Equal(before[index].Time) {
			t.Fatalf("record identity changed at %d: before=%+v after=%+v", index, before[index], after[index])
		}
	}
	longer := append(slices.Clone(shorter), flowmodel.NewTextMessage(flowmodel.RoleAssistant, "third"))
	if err := store.SaveMessages(context.Background(), "conversation", longer); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetMessages(context.Background(), "conversation")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, longer) {
		t.Fatalf("messages after longer save = %#v", got)
	}
	if err := store.DeleteMessages(context.Background(), "conversation"); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetMessages(context.Background(), "conversation")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("messages after delete = %#v", got)
	}
	if err := store.AppendMessages(context.Background(), "conversation", initial[:1]); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetMessages(context.Background(), "conversation")
	if err != nil || !reflect.DeepEqual(got, initial[:1]) {
		t.Fatalf("append after delete = %#v, %v", got, err)
	}
	if err := store.SaveMessages(context.Background(), "conversation", nil); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetMessages(context.Background(), "conversation")
	if err != nil || len(got) != 0 {
		t.Fatalf("empty save = %#v, %v", got, err)
	}
}

func TestHistoryStorePaginatesAndRetriesPartialSave(t *testing.T) {
	backend := &memoryHistoryLogStore{}
	store, err := NewHistoryStore(backend, "workspace")
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }
	messages := make([]flowmodel.Message, logstore.MaxLimit+1)
	for index := range messages {
		messages[index] = flowmodel.NewTextMessage(flowmodel.RoleUser, strconv.Itoa(index))
	}
	if err := store.AppendMessages(context.Background(), "conversation", messages); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetMessages(context.Background(), "conversation")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, messages) {
		t.Fatalf("paged messages count = %d", len(got))
	}
	recent, err := store.GetRecentMessages(context.Background(), "conversation", 3)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(recent, messages[len(messages)-3:]) {
		t.Fatalf("recent = %#v", recent)
	}

	replacement := []flowmodel.Message{
		flowmodel.NewTextMessage(flowmodel.RoleUser, "a"),
		flowmodel.NewTextMessage(flowmodel.RoleAssistant, "b"),
		flowmodel.NewTextMessage(flowmodel.RoleUser, "c"),
	}
	backend.failReplace = 2
	if err := store.SaveMessages(context.Background(), "conversation", replacement); err == nil {
		t.Fatal("partial replacement unexpectedly succeeded")
	}
	backend.failReplace = 0
	backend.replaceCalls = 0
	if err := store.SaveMessages(context.Background(), "conversation", replacement); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetMessages(context.Background(), "conversation")
	if err != nil || !reflect.DeepEqual(got, replacement) {
		t.Fatalf("retry result = %#v, %v", got, err)
	}
}

func TestHistoryStoreRejectsInvalidScopeKeysAndPayloads(t *testing.T) {
	if _, err := NewHistoryStore(nil, "workspace"); err == nil {
		t.Fatal("nil backend accepted")
	}
	backend := &memoryHistoryLogStore{}
	if _, err := NewHistoryStore(backend, " "); err == nil {
		t.Fatal("empty workspace accepted")
	}
	store, err := NewHistoryStore(backend, "workspace")
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }
	if _, err := store.GetMessages(context.Background(), " "); err == nil {
		t.Fatal("empty conversation accepted")
	}
	backend.badKeys = true
	if err := store.AppendMessages(
		context.Background(),
		"conversation",
		[]flowmodel.Message{flowmodel.NewTextMessage(flowmodel.RoleUser, "x")},
	); err == nil {
		t.Fatal("mismatched append key accepted")
	}
	backend.badKeys = false
	backend.records = nil
	record, err := store.encodeMessage(
		"conversation",
		flowmodel.NewTextMessage(flowmodel.RoleUser, "x"),
		"id",
		time.Unix(1_700_000_000, 0).UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}
	record.Attributes["schema_version"] = "2"
	record.Payload = []byte("{\"version\":2,\"message\":{\"role\":\"user\",\"parts\":[]}}")
	backend.records = append(backend.records, record)
	if _, err := store.GetMessages(context.Background(), "conversation"); err == nil {
		t.Fatal("unknown schema version accepted")
	}
	backend.records = append(backend.records[:0], record, record)
	record.Attributes["schema_version"] = "1"
	record.Payload = []byte("{\"version\":1,\"message\":{\"role\":\"user\",\"parts\":[]}}")
	backend.records[0] = record
	backend.records[1] = record
	if _, err := store.GetMessages(context.Background(), "conversation"); err == nil {
		t.Fatal("duplicate record key accepted")
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.GetMessages(cancelled, "conversation"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled query error = %v", err)
	}
}

func historyTestMessages() []flowmodel.Message {
	return []flowmodel.Message{
		flowmodel.NewTextMessage(flowmodel.RoleUser, "hello"),
		flowmodel.NewToolCallMessage([]flowmodel.ToolCall{
			{ID: "call-1", Name: "weather", Arguments: "{\"city\":\"Shanghai\"}"},
		}),
		flowmodel.NewToolResultMessage([]flowmodel.ToolResult{
			{ToolCallID: "call-1", Content: "sunny"},
		}),
		{
			Role: flowmodel.RoleAssistant,
			Parts: []flowmodel.Part{
				{
					Type: flowmodel.PartData,
					Data: &flowmodel.DataRef{
						MimeType: "application/claw+xml",
						Value: map[string]any{
							"speaker": "cat",
							"node":    "answer",
						},
					},
				},
				{Type: flowmodel.PartText, Text: "done"},
			},
		},
	}
}

var _ logstore.MutableStore = (*memoryHistoryLogStore)(nil)
