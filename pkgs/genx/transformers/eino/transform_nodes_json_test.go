package eino

import (
	"strings"
	"testing"
)

func TestDecodeJSONObjectRejectsRecursiveDuplicateKeys(t *testing.T) {
	for _, text := range []string{
		`{"intent":{"mode":"invalid","mode":"research"}}`,
		`{"steps":[{"mode":"invalid","mode":"research"}]}`,
		`{"steps":[[{"mode":"invalid","mode":"research"}]]}`,
		`{"intent":{"mode":"invalid","\u006dode":"research"}}`,
	} {
		if _, err := decodeJSONObject(text); err == nil || !strings.Contains(err.Error(), "duplicate key") {
			t.Fatalf("ambiguous JSON was accepted: %s; error=%v", text, err)
		}
	}
	object, err := decodeJSONObject(`{"steps":[{"mode":"plot"},{"mode":"research"}],"empty":[],"nothing":null,"number":2}`)
	if err != nil {
		t.Fatal(err)
	}
	steps := object["steps"].([]any)
	if len(steps) != 2 || steps[0].(map[string]any)["mode"] != "plot" || steps[1].(map[string]any)["mode"] != "research" || object["number"] != int64(2) || object["nothing"] != nil || object["empty"] == nil {
		t.Fatalf("unambiguous nested values changed: %#v", object)
	}
}
