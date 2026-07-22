package flowcraft

import (
	"context"
	"fmt"
	"testing"

	flowmodel "github.com/GizClaw/flowcraft/sdk/model"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

type recordingHistoryStore struct {
	query   logstore.Query
	records []logstore.Record
}

func (s *recordingHistoryStore) Append(_ context.Context, records []logstore.Record) ([]logstore.RecordKey, error) {
	s.records = append(s.records, records...)
	keys := make([]logstore.RecordKey, len(records))
	for index, record := range records {
		keys[index] = record.Key()
	}
	return keys, nil
}

func (s *recordingHistoryStore) Query(_ context.Context, query logstore.Query) (logstore.Page, error) {
	s.query = query
	return logstore.Page{}, nil
}

func (*recordingHistoryStore) Replace(context.Context, logstore.Record) error { return nil }

func (*recordingHistoryStore) Delete(context.Context, logstore.RecordKey) error { return nil }

func (*recordingHistoryStore) Close() error { return nil }

func TestInvocationLocalHistoryUsesWindow(t *testing.T) {
	t.Parallel()
	history := &conversationHistory{}
	messages := make([]flowmodel.Message, 0, historyWindow+10)
	for index := range historyWindow + 10 {
		messages = append(messages, flowmodel.NewTextMessage(flowmodel.RoleUser, fmt.Sprintf("message-%d", index)))
	}
	if err := history.append(context.Background(), messages, false); err != nil {
		t.Fatalf("append() error = %v", err)
	}
	if len(history.live) != historyWindow {
		t.Fatalf("retained History = %d messages, want %d", len(history.live), historyWindow)
	}
	messages, err := history.load(context.Background())
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if len(messages) != historyWindow || messages[0].Content() != "message-10" {
		t.Fatalf("window = %d messages starting at %q", len(messages), messages[0].Content())
	}
}

func TestPersistentHistoryOmitsEmptyScope(t *testing.T) {
	t.Parallel()
	store := &recordingHistoryStore{}
	history := &conversationHistory{store: store, agentID: "agent", contextID: "context"}

	if _, err := history.load(t.Context()); err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if len(store.query.Matchers) != 2 {
		t.Fatalf("load matchers = %#v, want agent and context only", store.query.Matchers)
	}
	if err := history.append(t.Context(), []flowmodel.Message{flowmodel.NewTextMessage(flowmodel.RoleUser, "hello")}, false); err != nil {
		t.Fatalf("append() error = %v", err)
	}
	if len(store.records) != 1 {
		t.Fatalf("appended records = %d, want 1", len(store.records))
	}
	if _, exists := store.records[0].Attributes["scope"]; exists {
		t.Fatalf("empty scope was persisted: %#v", store.records[0].Attributes)
	}
}

func TestPersistentHistoryUsesNonEmptyScope(t *testing.T) {
	t.Parallel()
	store := &recordingHistoryStore{}
	history := &conversationHistory{store: store, agentID: "agent", contextID: "context", scope: "workspace/agent"}

	if _, err := history.load(t.Context()); err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if len(store.query.Matchers) != 3 || store.query.Matchers[2].Name != "scope" || store.query.Matchers[2].Value != "workspace/agent" {
		t.Fatalf("load matchers = %#v", store.query.Matchers)
	}
	if err := history.append(t.Context(), []flowmodel.Message{flowmodel.NewTextMessage(flowmodel.RoleUser, "hello")}, false); err != nil {
		t.Fatalf("append() error = %v", err)
	}
	if got := store.records[0].Attributes["scope"]; got != "workspace/agent" {
		t.Fatalf("persisted scope = %q", got)
	}
}
