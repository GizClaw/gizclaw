package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestServerPutGetListDelete(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	store := &Server{
		Store: kv.NewMemory(nil),
		Now: func() time.Time {
			return now
		},
	}
	tool := testBuiltinTool("system.music.play")
	created, err := store.PutTool(ctx, tool)
	if err != nil {
		t.Fatalf("PutTool() error = %v", err)
	}
	if created.CreatedAt != now || created.UpdatedAt != now {
		t.Fatalf("timestamps = %s/%s, want %s", created.CreatedAt, created.UpdatedAt, now)
	}
	created.InputSchema[0] = '['

	got, err := store.GetTool(ctx, tool.ID)
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	if !json.Valid(got.InputSchema) {
		t.Fatalf("stored input schema was mutated: %s", got.InputSchema)
	}

	now = now.Add(time.Minute)
	updated := got
	updated.Version = stringPtr("2")
	syncedAt := now.Add(-time.Second)
	updated.SyncedAt = &syncedAt
	if updated, err = store.PutTool(ctx, updated); err != nil {
		t.Fatalf("PutTool(update) error = %v", err)
	}
	if updated.CreatedAt != got.CreatedAt {
		t.Fatalf("CreatedAt changed on update: got %s want %s", updated.CreatedAt, got.CreatedAt)
	}
	if updated.UpdatedAt != now {
		t.Fatalf("UpdatedAt = %s, want %s", updated.UpdatedAt, now)
	}
	if updated.SyncedAt == nil || !updated.SyncedAt.Equal(syncedAt) {
		t.Fatalf("SyncedAt = %v, want %s", updated.SyncedAt, syncedAt)
	}

	now = now.Add(time.Minute)
	updated.SyncedAt = nil
	if updated, err = store.PutTool(ctx, updated); err != nil {
		t.Fatalf("PutTool(clear sync) error = %v", err)
	}
	if updated.SyncedAt != nil {
		t.Fatalf("SyncedAt after clear = %v, want nil", updated.SyncedAt)
	}

	if _, err := store.PutTool(ctx, testBuiltinTool("system.mode.switch")); err != nil {
		t.Fatalf("PutTool(second) error = %v", err)
	}
	items, err := store.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("ListTools() len = %d, want 2", len(items))
	}

	if err := store.DeleteTool(ctx, tool.ID); err != nil {
		t.Fatalf("DeleteTool() error = %v", err)
	}
	if _, err := store.GetTool(ctx, tool.ID); !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("GetTool(deleted) error = %v, want %v", err, ErrToolNotFound)
	}
}

func TestNormalizeToolValidatesExecutorAndJSON(t *testing.T) {
	tool := testBuiltinTool("system.bad")
	tool.InputSchema = json.RawMessage(`{`)
	if _, err := NormalizeTool(tool); err == nil {
		t.Fatal("NormalizeTool(invalid JSON) error = nil")
	}

	for _, schema := range []json.RawMessage{
		json.RawMessage(`null`),
		json.RawMessage(`[]`),
		json.RawMessage(`"object"`),
		json.RawMessage(`{"type":"string"}`),
	} {
		tool = testBuiltinTool("system.bad")
		tool.InputSchema = schema
		if _, err := NormalizeTool(tool); err == nil {
			t.Fatalf("NormalizeTool(input_schema=%s) error = nil", schema)
		}
	}

	tool = testDeviceTool("peer.peer-a.music.play", "peer-a")
	tool.Executor.Method = nil
	if _, err := NormalizeTool(tool); err == nil {
		t.Fatal("NormalizeTool(device without method) error = nil")
	}

	tool = testBuiltinTool("system.bad")
	tool.Executor.Name = nil
	if _, err := NormalizeTool(tool); err == nil {
		t.Fatal("NormalizeTool(builtin without name) error = nil")
	}
}

func TestNormalizeToolValidatesTriggersAndOptionalJSON(t *testing.T) {
	tool := testBuiltinTool("system.music.play")
	tool.Triggers = []ToolTrigger{{
		Name:        "play",
		Description: stringPtr("play music"),
		Patterns:    []string{"play {query}"},
		Examples: []ToolTriggerExample{{
			Input:  "play song",
			Args:   json.RawMessage(`{"query":"song"}`),
			Output: stringPtr("playing"),
		}},
		Metadata: json.RawMessage(`{"intent":"music"}`),
	}}
	tool.OutputSchema = json.RawMessage(`{"type":"object"}`)
	tool.Metadata = json.RawMessage(`{"category":"media"}`)
	normalized, err := NormalizeTool(tool)
	if err != nil {
		t.Fatalf("NormalizeTool() error = %v", err)
	}
	normalized.Triggers[0].Patterns[0] = "mutated"
	if tool.Triggers[0].Patterns[0] == "mutated" {
		t.Fatal("NormalizeTool() did not clone trigger patterns")
	}

	tool.Triggers[0].Name = ""
	if _, err := NormalizeTool(tool); err == nil {
		t.Fatal("NormalizeTool(trigger without name) error = nil")
	}
	tool.Triggers[0].Name = "play"
	tool.Triggers[0].Examples[0].Args = json.RawMessage(`{`)
	if _, err := NormalizeTool(tool); err == nil {
		t.Fatal("NormalizeTool(trigger example invalid args) error = nil")
	}
	tool.Triggers[0].Examples[0].Args = json.RawMessage(`{"query":"song"}`)
	tool.Triggers[0].Examples[0].Input = ""
	if _, err := NormalizeTool(tool); err == nil {
		t.Fatal("NormalizeTool(trigger example without input) error = nil")
	}
	tool.Triggers[0].Examples[0].Input = "play song"
	tool.Triggers[0].Metadata = json.RawMessage(`{`)
	if _, err := NormalizeTool(tool); err == nil {
		t.Fatal("NormalizeTool(trigger invalid metadata) error = nil")
	}
}

func TestServerInvalidStateAndConfigErrors(t *testing.T) {
	ctx := context.Background()
	if _, err := (&Server{}).ListTools(ctx); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("ListTools(no store) error = %v, want %v", err, ErrNotConfigured)
	}
	store := &Server{Store: kv.NewMemory(nil)}
	if _, err := store.GetTool(ctx, ""); err == nil {
		t.Fatal("GetTool(empty) error = nil")
	}
	if err := store.DeleteTool(ctx, ""); err == nil {
		t.Fatal("DeleteTool(empty) error = nil")
	}
	if err := store.Store.Set(ctx, toolKey("bad-json"), []byte(`{`)); err != nil {
		t.Fatalf("raw Set() error = %v", err)
	}
	if _, err := store.GetTool(ctx, "bad-json"); err == nil {
		t.Fatal("GetTool(bad JSON) error = nil")
	}
	if _, err := store.ListTools(ctx); err == nil {
		t.Fatal("ListTools(bad JSON) error = nil")
	}
}
