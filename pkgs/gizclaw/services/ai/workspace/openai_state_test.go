package workspace

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"
)

func TestOpenAIStateStoreReloadAndHistoryCorrelation(t *testing.T) {
	objects := newTestObjectStore(t)
	store := NewObjectRuntimeStore(objects)
	ctx := context.Background()
	runtime, err := store.PrepareWorkspace(ctx, "openai-demo")
	if err != nil {
		t.Fatal(err)
	}
	created := time.Unix(1_700_000_000, 0).UTC()
	conversation := OpenAIConversation{ID: "conv_demo", Metadata: map[string]string{"workflow_name": "story"}, CreatedAt: created}
	if err := runtime.OpenAI.PutConversation(ctx, conversation); err != nil {
		t.Fatal(err)
	}
	entry, err := runtime.History.Append(ctx, AppendHistoryRequest{Type: "gear", GearID: "msg_demo", Name: "user", Text: "authoritative transcript", CreatedAt: created})
	if err != nil {
		t.Fatal(err)
	}
	item := OpenAIItem{ID: "msg_demo", HistoryID: entry.ID, Role: "user", Status: "completed", CreatedAt: created}
	if err := runtime.OpenAI.PutItem(ctx, item); err != nil {
		t.Fatal(err)
	}
	response := OpenAIResponse{ID: "resp_demo", ConversationID: conversation.ID, WorkspaceID: "openai-demo", Model: "story", Status: "completed", InputItemIDs: []string{item.ID}, CreatedAt: created, CompletedAt: &created}
	if err := runtime.OpenAI.PutResponse(ctx, response); err != nil {
		t.Fatal(err)
	}

	reloaded, err := store.GetWorkspaceRuntime(ctx, "openai-demo")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := reloaded.OpenAI.Conversation(ctx); err != nil || got.ID != conversation.ID {
		t.Fatalf("Conversation() = %#v, %v", got, err)
	}
	items, err := reloaded.OpenAI.Items(ctx)
	if err != nil || len(items) != 1 || items[0].HistoryID != entry.ID {
		t.Fatalf("Items() = %#v, %v", items, err)
	}
	if got, err := reloaded.OpenAI.Response(ctx, response.ID); err != nil || got.Status != "completed" {
		t.Fatalf("Response() = %#v, %v", got, err)
	}
	reader, err := objects.Get(reloaded.OpenAI.Prefix + "/items/" + item.ID + ".json")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "authoritative transcript") {
		t.Fatal("OpenAI item duplicated History transcript text")
	}
}

func TestOpenAIStateStoreOrdersItemsByDurableSequence(t *testing.T) {
	objects := newTestObjectStore(t)
	runtime, err := NewObjectRuntimeStore(objects).PrepareWorkspace(t.Context(), "openai-order")
	if err != nil {
		t.Fatal(err)
	}
	created := time.Unix(1_700_000_000, 0).UTC()
	for _, item := range []OpenAIItem{
		{ID: "msg_second", HistoryID: "history-second", Sequence: 1, CreatedAt: created},
		{ID: "msg_first", HistoryID: "history-first", Sequence: 0, CreatedAt: created},
	} {
		if err := runtime.OpenAI.PutItem(t.Context(), item); err != nil {
			t.Fatal(err)
		}
	}
	items, err := runtime.OpenAI.Items(t.Context())
	if err != nil || len(items) != 2 || items[0].ID != "msg_first" || items[1].ID != "msg_second" {
		t.Fatalf("Items() = %#v, %v", items, err)
	}
}

func TestOpenAIStateStoreCorruptionAndRuntimeCleanup(t *testing.T) {
	objects := newTestObjectStore(t)
	store := NewObjectRuntimeStore(objects)
	ctx := context.Background()
	runtime, err := store.PrepareWorkspace(ctx, "openai-corrupt")
	if err != nil {
		t.Fatal(err)
	}
	if err := objects.Put(runtime.OpenAI.Prefix+"/conversation.json", strings.NewReader("{not-json")); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.OpenAI.Conversation(ctx); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("Conversation() corruption error = %v", err)
	}
	if err := store.DeleteWorkspaceRuntime(ctx, "openai-corrupt"); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.OpenAI.Conversation(ctx); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Conversation() after cleanup error = %v", err)
	}
}
