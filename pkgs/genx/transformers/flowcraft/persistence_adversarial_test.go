package flowcraft

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/flowcraft/sdk/engine"
	flowmodel "github.com/GizClaw/flowcraft/sdk/model"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

func TestBoardStateAdversarialLoadSaveAndCopy(t *testing.T) {
	t.Parallel()
	store := kv.NewMemory(nil)
	if state, err := loadBoardState(t.Context(), store, "missing"); err != nil || state != nil {
		t.Fatalf("loadBoardState(missing) = %#v, %v", state, err)
	}
	if err := store.Set(t.Context(), kv.Key{"broken"}, []byte("{")); err != nil {
		t.Fatalf("Set(broken) error = %v", err)
	}
	if _, err := loadBoardState(t.Context(), store, "broken"); err == nil ||
		!strings.Contains(err.Error(), "decode State") {
		t.Fatalf("loadBoardState(broken) error = %v", err)
	}

	board := engine.NewBoard()
	board.SetVar("durable", map[string]any{"nested": []any{"value"}})
	board.SetVar("tmp_transient", "discard")
	board.SetVar("response.tokens", 10)
	board.SetVar("__private", true)
	if err := saveBoardState(t.Context(), store, "context", board); err != nil {
		t.Fatalf("saveBoardState() error = %v", err)
	}
	loaded, err := loadBoardState(t.Context(), store, "context")
	if err != nil {
		t.Fatalf("loadBoardState() error = %v", err)
	}
	if len(loaded) != 1 || loaded["durable"] == nil {
		t.Fatalf("loaded State = %#v", loaded)
	}

	board.SetVar("bad", make(chan int))
	if err := saveBoardState(t.Context(), store, "bad", board); err == nil ||
		!strings.Contains(err.Error(), "copy Board variables") {
		t.Fatalf("saveBoardState(unserializable) error = %v", err)
	}
	observed, err := observationBoardVariables(board)
	if err != nil {
		t.Fatalf("observationBoardVariables() error = %v", err)
	}
	if _, exists := observed["bad"]; exists {
		t.Fatalf("unserializable observation value escaped: %#v", observed)
	}
	if observed["tmp_transient"] != "discard" {
		t.Fatalf("transient observation = %#v", observed)
	}

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := loadBoardState(cancelled, store, "context"); err == nil ||
		!strings.Contains(err.Error(), "load State") {
		t.Fatalf("loadBoardState(cancelled) error = %v", err)
	}
	if err := saveBoardState(cancelled, store, "context", engine.NewBoard()); err == nil ||
		!strings.Contains(err.Error(), "save State") {
		t.Fatalf("saveBoardState(cancelled) error = %v", err)
	}
}

func TestPersistentHistoryAdversarialOrderingDecodeAndStoreFailures(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	record := func(id string, at time.Time, version int, text string) logstore.Record {
		payload, err := json.Marshal(struct {
			Version int               `json:"version"`
			Message flowmodel.Message `json:"message"`
		}{Version: version, Message: flowmodel.NewTextMessage(flowmodel.RoleUser, text)})
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		return logstore.Record{ID: id, Time: at, Payload: payload}
	}
	store := &adversarialHistoryStore{page: logstore.Page{Records: []logstore.Record{
		record("later", now.Add(time.Second), historyVersion, "second"),
		record("alpha", now, historyVersion, "first-a"),
		record("beta", now, historyVersion, "first-b"),
	}}}
	history := &conversationHistory{store: store, agentID: "agent", contextID: "context"}
	messages, err := history.load(t.Context())
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if len(messages) != 3 ||
		messages[0].Content() != "first-a" ||
		messages[1].Content() != "first-b" ||
		messages[2].Content() != "second" {
		t.Fatalf("ordered History = %#v", messages)
	}

	store.page.Records = []logstore.Record{{ID: "broken", Payload: []byte("{")}}
	if _, err := history.load(t.Context()); err == nil || !strings.Contains(err.Error(), "decode History") {
		t.Fatalf("load(broken) error = %v", err)
	}
	store.page.Records = []logstore.Record{record("future", now, historyVersion+1, "future")}
	if _, err := history.load(t.Context()); err == nil || !strings.Contains(err.Error(), "unsupported History version") {
		t.Fatalf("load(version) error = %v", err)
	}
	store.queryErr = errors.New("query failed")
	if _, err := history.load(t.Context()); err == nil || !strings.Contains(err.Error(), "query failed") {
		t.Fatalf("load(query failure) error = %v", err)
	}

	store.queryErr = nil
	store.page = logstore.Page{}
	store.appendErr = errors.New("append failed")
	if err := history.append(t.Context(), []flowmodel.Message{
		flowmodel.NewTextMessage(flowmodel.RoleAssistant, "partial"),
	}, true); err == nil || !strings.Contains(err.Error(), "append failed") {
		t.Fatalf("append(failure) error = %v", err)
	}
	store.appendErr = nil
	store.accepted = 0
	if err := history.append(t.Context(), []flowmodel.Message{
		flowmodel.NewTextMessage(flowmodel.RoleUser, "hello"),
	}, false); err == nil || !strings.Contains(err.Error(), "accepted 0 of 1") {
		t.Fatalf("append(short acceptance) error = %v", err)
	}
}

type adversarialHistoryStore struct {
	page      logstore.Page
	queryErr  error
	appendErr error
	accepted  int
}

func (store *adversarialHistoryStore) Append(
	_ context.Context,
	records []logstore.Record,
) ([]logstore.RecordKey, error) {
	if store.appendErr != nil {
		return nil, store.appendErr
	}
	keys := make([]logstore.RecordKey, min(store.accepted, len(records)))
	for index := range keys {
		keys[index] = records[index].Key()
	}
	return keys, nil
}

func (store *adversarialHistoryStore) Query(
	context.Context,
	logstore.Query,
) (logstore.Page, error) {
	return store.page, store.queryErr
}

func (*adversarialHistoryStore) Replace(context.Context, logstore.Record) error { return nil }
func (*adversarialHistoryStore) Delete(context.Context, logstore.RecordKey) error {
	return nil
}
func (*adversarialHistoryStore) Close() error { return nil }
