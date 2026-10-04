package graphstate

import (
	"errors"
	"reflect"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestGraphStateReloadAndConflict(t *testing.T) {
	db := testDB(t)
	first, err := OpenScope(t.Context(), db, "owner", "workspace", "agent")
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]any{
		"score": int64(9007199254740993), "status": "playing", "active": true,
		"object":    map[string]any{"scene": "library", "large": int64(9007199254740993), "whole/float~": float64(7)},
		"list":      []any{"badge", int64(-9007199254740993), map[string]any{"nested": int64(9007199254740993), "whole": float64(2)}},
		"messages":  []*schema.Message{schema.UserMessage("hello")},
		"documents": []*schema.Document{{ID: "fact", Content: "retained"}}, "blob": []byte{0, 1, 255},
	}
	committed, err := first.CompareAndSwap(t.Context(), "context", "", values)
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenScope(t.Context(), db, "owner", "workspace", "agent")
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := second.Load(t.Context(), "context")
	if err != nil || !reflect.DeepEqual(reloaded, committed) || !reflect.DeepEqual(reloaded.Fields, values) {
		t.Fatalf("reload = %#v, %v", reloaded, err)
	}
	if _, err := second.CompareAndSwap(t.Context(), "context", "", values); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale initial write = %v", err)
	}
	values["status"] = "finished"
	replacement, err := second.CompareAndSwap(t.Context(), "context", reloaded.Version, values)
	if err != nil || replacement.Version == reloaded.Version {
		t.Fatalf("replacement = %#v, %v", replacement, err)
	}
	if _, err := first.CompareAndSwap(t.Context(), "context", committed.Version, values); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale version = %v", err)
	}
	if err := RetireWorkspace(t.Context(), db, "owner", "workspace"); err != nil {
		t.Fatal(err)
	}
	if _, err := second.CompareAndSwap(t.Context(), "context", replacement.Version, values); !errors.Is(err, ErrRetired) {
		t.Fatalf("retired write = %v", err)
	}
}

func TestLegacyJSONCheckpointPreservesNestedInteger(t *testing.T) {
	snapshot, err := decodeSnapshot([]byte(`{"version":"legacy","fields":{"object":{"kind":"json","value":{"nested":[9007199254740993,-9007199254740993]}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"nested": []any{int64(9007199254740993), int64(-9007199254740993)}}
	if !reflect.DeepEqual(snapshot.Fields["object"], want) {
		t.Fatalf("legacy JSON reload = %#v", snapshot.Fields["object"])
	}
}
