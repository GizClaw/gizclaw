package eino

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/cloudwego/eino/schema"
)

func TestPredicateOperators(t *testing.T) {
	t.Parallel()
	state := map[string]any{
		"text":   "alpha beta",
		"list":   []any{"alpha", int64(3)},
		"object": map[string]any{"present": true},
		"count":  int64(3),
	}
	tests := []struct {
		name      string
		predicate Predicate
		want      bool
		wantErr   bool
	}{
		{name: "all", predicate: Predicate{All: []Predicate{
			{Field: "text", Op: PredicateContains, Value: "alpha"},
			{Field: "count", Op: PredicateGreaterEqual, Value: 3},
		}}, want: true},
		{name: "any", predicate: Predicate{Any: []Predicate{
			{Field: "missing", Op: PredicateExists},
			{Field: "count", Op: PredicateEqual, Value: float64(3)},
		}}, want: true},
		{name: "not", predicate: Predicate{Not: &Predicate{
			Field: "text", Op: PredicateEqual, Value: "other",
		}}, want: true},
		{name: "not exists", predicate: Predicate{Field: "missing", Op: PredicateNotExists}, want: true},
		{name: "not equal", predicate: Predicate{Field: "count", Op: PredicateNotEqual, Value: 4}, want: true},
		{name: "list contains", predicate: Predicate{Field: "list", Op: PredicateContains, Value: 3}, want: true},
		{name: "object contains", predicate: Predicate{Field: "object", Op: PredicateContains, Value: "present"}, want: true},
		{name: "not contains", predicate: Predicate{Field: "text", Op: PredicateNotContains, Value: "gamma"}, want: true},
		{name: "less", predicate: Predicate{Field: "count", Op: PredicateLess, Value: 4}, want: true},
		{name: "less equal", predicate: Predicate{Field: "count", Op: PredicateLessEqual, Value: 3}, want: true},
		{name: "greater", predicate: Predicate{Field: "count", Op: PredicateGreater, Value: 2}, want: true},
		{name: "missing comparison", predicate: Predicate{Field: "missing", Op: PredicateEqual, Value: "x"}},
		{name: "bad contains", predicate: Predicate{Field: "count", Op: PredicateContains, Value: 3}, wantErr: true},
		{name: "bad number", predicate: Predicate{Field: "text", Op: PredicateLess, Value: 3}, wantErr: true},
		{name: "unsupported", predicate: Predicate{Field: "text", Op: "unsupported"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := evaluatePredicate(test.predicate, state)
			if (err != nil) != test.wantErr {
				t.Fatalf("evaluatePredicate() error = %v, wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("evaluatePredicate() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestTransformOperationsAndBounds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		config  TransformNode
		inputs  map[string]any
		want    map[string]any
		wantErr string
	}{
		{
			name: "select", config: TransformNode{Operation: TransformSelect},
			inputs: map[string]any{"value": "selected"}, want: map[string]any{"value": "selected"},
		},
		{
			name: "concat", config: TransformNode{
				Operation: TransformConcatText, Order: []string{"left", "right"}, Separator: "|",
			},
			inputs: map[string]any{"left": "a", "right": "b"}, want: map[string]any{"text": "a|b"},
		},
		{
			name: "decode", config: TransformNode{
				Operation: TransformDecodeJSON, MaxInputBytes: 128, MaxOutputBytes: 128,
			},
			inputs: map[string]any{"text": `{"count":3,"nested":[1.5]}`},
			want: map[string]any{"object": map[string]any{
				"count": int64(3), "nested": []any{float64(1.5)},
			}},
		},
		{
			name: "messages", config: TransformNode{
				Operation: TransformBuildMessages,
				Messages: []TransformMessage{
					{Role: PromptSystem, Text: "system"},
					{Role: PromptUser, Input: "user"},
					{Role: PromptAssistant, Text: "assistant"},
				},
			},
			inputs: map[string]any{"user": "hello"},
			want: map[string]any{"messages": []*schema.Message{
				schema.SystemMessage("system"),
				schema.UserMessage("hello"),
				schema.AssistantMessage("assistant", nil),
			}},
		},
		{name: "select missing", config: TransformNode{Operation: TransformSelect}, wantErr: "requires value"},
		{name: "concat missing order", config: TransformNode{Operation: TransformConcatText}, wantErr: "requires Order"},
		{
			name: "concat wrong type", config: TransformNode{
				Operation: TransformConcatText, Order: []string{"value"},
			},
			inputs: map[string]any{"value": 1}, wantErr: "not text",
		},
		{
			name: "concat bound", config: TransformNode{
				Operation: TransformConcatText, Order: []string{"value"}, MaxOutputBytes: 1,
			},
			inputs: map[string]any{"value": "too long"}, wantErr: "exceeds",
		},
		{
			name: "decode wrong type", config: TransformNode{Operation: TransformDecodeJSON},
			inputs: map[string]any{"text": 1}, wantErr: "requires text",
		},
		{
			name: "decode input bound", config: TransformNode{
				Operation: TransformDecodeJSON, MaxInputBytes: 1,
			},
			inputs: map[string]any{"text": `{}`}, wantErr: "input exceeds",
		},
		{
			name: "decode duplicate", config: TransformNode{Operation: TransformDecodeJSON},
			inputs: map[string]any{"text": `{"a":1,"a":2}`}, wantErr: "duplicate key",
		},
		{
			name: "decode non-object", config: TransformNode{Operation: TransformDecodeJSON},
			inputs: map[string]any{"text": `[]`}, wantErr: "requires an object",
		},
		{
			name: "decode trailing", config: TransformNode{Operation: TransformDecodeJSON},
			inputs: map[string]any{"text": `{} {}`}, wantErr: "trailing",
		},
		{
			name: "decode output bound", config: TransformNode{
				Operation: TransformDecodeJSON, MaxOutputBytes: 1,
			},
			inputs: map[string]any{"text": `{"a":1}`}, wantErr: "output exceeds",
		},
		{
			name: "messages missing", config: TransformNode{Operation: TransformBuildMessages},
			wantErr: "requires Messages",
		},
		{
			name: "messages ambiguous", config: TransformNode{
				Operation: TransformBuildMessages,
				Messages:  []TransformMessage{{Role: PromptUser, Input: "value", Text: "literal"}},
			},
			wantErr: "exactly one",
		},
		{
			name: "messages wrong input", config: TransformNode{
				Operation: TransformBuildMessages,
				Messages:  []TransformMessage{{Role: PromptUser, Input: "value"}},
			},
			inputs: map[string]any{"value": 1}, wantErr: "not text",
		},
		{
			name: "messages bad role", config: TransformNode{
				Operation: TransformBuildMessages,
				Messages:  []TransformMessage{{Role: "tool", Text: "value"}},
			},
			wantErr: "unsupported Prompt role",
		},
		{name: "unsupported", config: TransformNode{Operation: "unsupported"}, wantErr: "unsupported"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := runTransform(test.config, test.inputs)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("runTransform() error = %v, want containing %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("runTransform() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("runTransform() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestRunStateTypesMergesAndCopies(t *testing.T) {
	t.Parallel()
	fields := map[string]StateField{
		"text":      {Name: "text", Type: StateString, Merge: MergeAppend, Required: true},
		"list":      {Name: "list", Type: StateList, Merge: MergeAppend},
		"messages":  {Name: "messages", Type: StateMessages, Merge: MergeAppend},
		"documents": {Name: "documents", Type: StateDocuments, Merge: MergeAppend},
		"object":    {Name: "object", Type: StateObject, Merge: MergeObject},
		"integer":   {Name: "integer", Type: StateInteger, Merge: MergeReplace},
		"number":    {Name: "number", Type: StateNumber, Merge: MergeReplace},
		"boolean":   {Name: "boolean", Type: StateBoolean, Merge: MergeReplace},
		"blob":      {Name: "blob", Type: StateBlob, Merge: MergeReplace},
	}
	state, err := newRunState(fields, graphInput{}, nil, nil)
	if err != nil {
		t.Fatalf("newRunState() error = %v", err)
	}
	updates := []struct {
		name  string
		value any
	}{
		{name: "text", value: "a"}, {name: "text", value: "b"},
		{name: "list", value: []any{int64(1)}}, {name: "list", value: []any{"two"}},
		{name: "messages", value: []*schema.Message{schema.UserMessage("one")}},
		{name: "messages", value: []*schema.Message{schema.AssistantMessage("two", nil)}},
		{name: "documents", value: []*schema.Document{{ID: "one", Content: "1"}}},
		{name: "documents", value: []*schema.Document{{ID: "two", Content: "2"}}},
		{name: "object", value: map[string]any{"a": int64(1), "same": "old"}},
		{name: "object", value: map[string]any{"b": true, "same": "new"}},
		{name: "integer", value: uint64(9_007_199_254_740_993)},
		{name: "number", value: float32(1.5)},
		{name: "boolean", value: true},
		{name: "blob", value: []byte{1, 2, 3}},
	}
	for _, update := range updates {
		if err := state.set(update.name, update.value); err != nil {
			t.Fatalf("set(%q) error = %v", update.name, err)
		}
	}
	if err := state.required(); err != nil {
		t.Fatalf("required() error = %v", err)
	}
	want := map[string]any{
		"text": "ab", "list": []any{int64(1), "two"},
		"messages":  []*schema.Message{schema.UserMessage("one"), schema.AssistantMessage("two", nil)},
		"documents": []*schema.Document{{ID: "one", Content: "1"}, {ID: "two", Content: "2"}},
		"object":    map[string]any{"a": int64(1), "b": true, "same": "new"},
		"integer":   int64(9_007_199_254_740_993), "number": float64(1.5),
		"boolean": true, "blob": []byte{1, 2, 3},
	}
	got, err := state.snapshot()
	if err != nil {
		t.Fatalf("snapshot() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot() = %#v, want %#v", got, want)
	}
	got["object"].(map[string]any)["a"] = "mutated"
	again, err := state.snapshot()
	if err != nil {
		t.Fatalf("second snapshot() error = %v", err)
	}
	if again["object"].(map[string]any)["a"] != int64(1) {
		t.Fatal("snapshot mutation leaked into run State")
	}

	if err := state.set("integer", uint64(math.MaxUint64)); err == nil {
		t.Fatal("set() accepted overflowing uint64")
	}
	if err := state.set("number", math.Inf(1)); err == nil {
		t.Fatal("set() accepted non-finite number")
	}
	if err := state.set("missing", "value"); err == nil {
		t.Fatal("set() accepted undeclared field")
	}
	cyclic := map[string]any{}
	cyclic["self"] = cyclic
	if err := state.set("object", cyclic); err == nil || !strings.Contains(err.Error(), "nesting exceeds") {
		t.Fatalf("set() cyclic object error = %v", err)
	}
}

func TestRunStateOwnsInputParts(t *testing.T) {
	t.Parallel()
	source := &genx.Blob{MIMEType: "image/png", Data: []byte{1, 2, 3}}
	state, err := newRunState(nil, graphInput{Parts: []any{source}}, nil, nil)
	if err != nil {
		t.Fatalf("newRunState() error = %v", err)
	}
	source.Data[0] = 9
	first, err := state.binding(Binding{From: "input.parts"})
	if err != nil {
		t.Fatalf("binding() error = %v", err)
	}
	firstBlob := first.([]any)[0].(*genx.Blob)
	if firstBlob.Data[0] != 1 {
		t.Fatalf("owned input changed to %v", firstBlob.Data)
	}
	firstBlob.Data[0] = 8
	second, err := state.binding(Binding{From: "input.parts"})
	if err != nil {
		t.Fatalf("second binding() error = %v", err)
	}
	if got := second.([]any)[0].(*genx.Blob).Data[0]; got != 1 {
		t.Fatalf("binding mutation leaked into State: %d", got)
	}
}
