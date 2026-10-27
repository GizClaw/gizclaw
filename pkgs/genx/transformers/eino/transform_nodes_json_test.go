package eino

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeJSONObjectBoundsNestingBeforeDescending(t *testing.T) {
	const standardJSONNestingLimit = 10000
	for _, kind := range []string{"arrays", "objects"} {
		t.Run(kind, func(t *testing.T) {
			makeJSON := func(depth int) string {
				if kind == "arrays" {
					return `{"value":` + strings.Repeat("[", depth-1) + `0` + strings.Repeat("]", depth-1) + `}`
				}
				return strings.Repeat(`{"value":`, depth) + `0` + strings.Repeat("}", depth)
			}
			boundary := makeJSON(standardJSONNestingLimit)
			if !json.Valid([]byte(boundary)) {
				t.Fatal("boundary fixture disagrees with encoding/json")
			}
			if _, err := decodeJSONObject(boundary); err != nil {
				t.Fatalf("valid encoding/json depth boundary was rejected: %v", err)
			}
			for _, depth := range []int{standardJSONNestingLimit + 1, 100000} {
				if _, err := decodeJSONObject(makeJSON(depth)); err == nil || !strings.Contains(err.Error(), "nesting exceeds maximum depth") {
					t.Fatalf("excessive depth %d did not fail before descent: %v", depth, err)
				}
			}
		})
	}
}

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
