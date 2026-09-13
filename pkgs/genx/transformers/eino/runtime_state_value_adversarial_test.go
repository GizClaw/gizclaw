package eino

import (
	"context"
	"errors"
	"math"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
	"go.starlark.net/starlark"
)

func TestStateValueNormalizationCoversEveryDeclaredType(t *testing.T) {
	t.Parallel()
	message := schema.UserMessage("hello")
	document := &schema.Document{
		ID: "document", Content: "body", MetaData: map[string]any{"nested": "value"},
	}
	tests := []struct {
		name      string
		stateType StateType
		value     any
		wantErr   string
	}{
		{name: "string", stateType: StateString, value: "value"},
		{name: "boolean", stateType: StateBoolean, value: true},
		{name: "integer", stateType: StateInteger, value: int32(7)},
		{name: "number", stateType: StateNumber, value: float32(1.5)},
		{name: "object", stateType: StateObject, value: map[string]any{"key": []any{"value"}}},
		{name: "list", stateType: StateList, value: []any{map[string]any{"key": "value"}}},
		{name: "messages", stateType: StateMessages, value: []*schema.Message{message, nil}},
		{name: "documents", stateType: StateDocuments, value: []*schema.Document{document, nil}},
		{name: "blob", stateType: StateBlob, value: []byte{1, 2, 3}},
		{name: "nil", stateType: StateString, wantErr: "nil is not"},
		{name: "wrong string", stateType: StateString, value: 1, wantErr: "expected string"},
		{name: "wrong boolean", stateType: StateBoolean, value: "true", wantErr: "expected boolean"},
		{name: "wrong object", stateType: StateObject, value: []any{}, wantErr: "expected object"},
		{name: "wrong list", stateType: StateList, value: map[string]any{}, wantErr: "expected list"},
		{name: "wrong messages", stateType: StateMessages, value: []any{}, wantErr: "expected messages"},
		{name: "wrong documents", stateType: StateDocuments, value: []any{}, wantErr: "expected documents"},
		{name: "wrong blob", stateType: StateBlob, value: "data", wantErr: "expected blob"},
		{name: "unknown type", stateType: "unknown", value: "value", wantErr: "unsupported type"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeStateValue(test.value, test.stateType)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("normalizeStateValue() error = %v, want containing %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeStateValue() error = %v", err)
			}
			if !valueMatchesStateType(test.value, test.stateType) {
				t.Fatalf("valueMatchesStateType(%T, %q) = false", test.value, test.stateType)
			}
			switch test.stateType {
			case StateObject:
				got.(map[string]any)["key"].([]any)[0] = "changed"
				if test.value.(map[string]any)["key"].([]any)[0] != "value" {
					t.Fatal("object normalization did not take ownership")
				}
			case StateList:
				got.([]any)[0].(map[string]any)["key"] = "changed"
				if test.value.([]any)[0].(map[string]any)["key"] != "value" {
					t.Fatal("list normalization did not take ownership")
				}
			case StateDocuments:
				got.([]*schema.Document)[0].MetaData["nested"] = "changed"
				if document.MetaData["nested"] != "value" {
					t.Fatal("document normalization did not take ownership")
				}
			case StateBlob:
				got.([]byte)[0] = 9
				if test.value.([]byte)[0] != 1 {
					t.Fatal("blob normalization did not take ownership")
				}
			}
		})
	}
}

func TestIntegerAndNumberNormalizationAdversarialBoundaries(t *testing.T) {
	t.Parallel()
	integers := []any{
		int(1), int8(2), int16(3), int32(4), int64(5),
		uint(6), uint8(7), uint16(8), uint32(9), uint64(10), float64(11),
	}
	for _, value := range integers {
		got, err := normalizeInteger(value)
		if err != nil || got <= 0 {
			t.Fatalf("normalizeInteger(%T(%v)) = %d, %v", value, value, got, err)
		}
	}
	for _, test := range []struct {
		value any
		want  string
	}{
		{value: float64(1.25), want: "integral"},
		{value: uint64(math.MaxUint64), want: "overflows"},
		{value: "1", want: "expected integer"},
	} {
		if _, err := normalizeInteger(test.value); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("normalizeInteger(%v) error = %v, want containing %q", test.value, err, test.want)
		}
	}
	numbers := []any{
		int(-1), int8(-2), int16(-3), int32(-4), int64(-5),
		uint(1), uint8(2), uint16(3), uint32(4), uint64(5), float32(1.25), float64(2.5),
	}
	for _, value := range numbers {
		if _, err := normalizeNumber(value); err != nil {
			t.Fatalf("normalizeNumber(%T(%v)) error = %v", value, value, err)
		}
	}
	for _, value := range []any{math.NaN(), math.Inf(1), "number"} {
		if _, err := normalizeNumber(value); err == nil {
			t.Fatalf("normalizeNumber(%v) succeeded", value)
		}
	}
}

func TestStateCloneAndMergeAdversarialBoundaries(t *testing.T) {
	t.Parallel()
	messages := []*schema.Message{schema.UserMessage("one")}
	documents := []*schema.Document{{ID: "one", Content: "body"}}
	for _, value := range []any{
		nil, true, "text", int(1), int8(1), int16(1), int32(1), int64(1),
		uint(1), uint8(1), uint16(1), uint32(1), uint64(1), float32(1), float64(1),
		[]any{"item"}, map[string]any{"key": "value"}, messages, documents, []byte{1},
	} {
		if _, err := cloneValue(value); err != nil {
			t.Fatalf("cloneValue(%T) error = %v", value, err)
		}
	}
	if _, err := cloneValue(make(chan int)); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("cloneValue(unsupported) error = %v", err)
	}
	if cloned, err := cloneMap(nil); err != nil || cloned != nil {
		t.Fatalf("cloneMap(nil) = %#v, %v", cloned, err)
	}
	deep := any("leaf")
	for range maxStateValueDepth + 2 {
		deep = []any{deep}
	}
	if _, err := cloneValue(deep); err == nil || !strings.Contains(err.Error(), "nesting exceeds") {
		t.Fatalf("cloneValue(deep) error = %v", err)
	}

	mergeTests := []struct {
		field   StateField
		current any
		next    any
		want    any
	}{
		{
			field:   StateField{Type: StateString, Merge: MergeAppend},
			current: "a", next: "b", want: "ab",
		},
		{
			field:   StateField{Type: StateList, Merge: MergeAppend},
			current: []any{"a"}, next: []any{"b"}, want: []any{"a", "b"},
		},
		{
			field:   StateField{Type: StateMessages, Merge: MergeAppend},
			current: []*schema.Message{schema.UserMessage("a")},
			next:    []*schema.Message{schema.AssistantMessage("b", nil)},
			want:    []*schema.Message{schema.UserMessage("a"), schema.AssistantMessage("b", nil)},
		},
		{
			field:   StateField{Type: StateDocuments, Merge: MergeAppend},
			current: []*schema.Document{{ID: "a"}}, next: []*schema.Document{{ID: "b"}},
			want: []*schema.Document{{ID: "a"}, {ID: "b"}},
		},
		{
			field:   StateField{Type: StateObject, Merge: MergeObject},
			current: map[string]any{"a": "old", "b": "keep"},
			next:    map[string]any{"a": "new"},
			want:    map[string]any{"a": "new", "b": "keep"},
		},
		{
			field:   StateField{Type: StateInteger, Merge: MergeReplace},
			current: int64(1), next: int64(2), want: int64(2),
		},
	}
	for _, test := range mergeTests {
		got, err := mergeStateValue(test.current, test.next, test.field)
		if err != nil || !reflect.DeepEqual(got, test.want) {
			t.Fatalf("mergeStateValue(%q) = %#v, %v, want %#v", test.field.Type, got, err, test.want)
		}
	}
}

func TestRunStateAdversarialOwnershipBindingsAndEmission(t *testing.T) {
	t.Parallel()
	fields := map[string]StateField{
		"required": {Name: "required", Type: StateString, Merge: MergeReplace, Required: true},
		"answer":   {Name: "answer", Type: StateString, Merge: MergeReplace},
	}
	if _, err := newRunState(fields, graphInput{}, map[string]any{"answer": 1}, nil); err == nil {
		t.Fatal("newRunState(invalid initial value) succeeded")
	}
	message := &schema.Message{
		Role: schema.User, Content: "message",
		Extra: map[string]any{"unmarshalable": make(chan int)},
	}
	input := graphInput{
		Text:     "text",
		Messages: []*schema.Message{message},
		Parts:    []any{"opaque", []byte{1}},
		History:  []*schema.Message{schema.AssistantMessage("history", nil)},
		Memory:   "memory",
	}
	state, err := newRunState(fields, input, map[string]any{"answer": "initial"}, nil)
	if err != nil {
		t.Fatalf("newRunState() error = %v", err)
	}
	cloned, err := state.clone(nil)
	if err != nil {
		t.Fatalf("clone() error = %v", err)
	}
	if cloned.input.Text != "text" || len(cloned.input.Messages) != 1 ||
		len(cloned.input.Parts) != 2 || cloned.input.Memory != "memory" {
		t.Fatalf("clone input = %#v", cloned.input)
	}
	message.Content = "mutated"
	if cloned.input.Messages[0].Content != "message" {
		t.Fatal("cloneMessages fallback leaked source mutation")
	}
	for source, want := range map[string]any{
		"input.text":      "text",
		"memory.recalled": "memory",
	} {
		got, err := state.binding(Binding{From: source})
		if err != nil || got != want {
			t.Fatalf("binding(%q) = %#v, %v, want %#v", source, got, err, want)
		}
	}
	for _, source := range []string{"input.messages", "input.parts", "history.messages", "answer"} {
		if _, err := state.binding(Binding{From: source}); err != nil {
			t.Fatalf("binding(%q) error = %v", source, err)
		}
	}
	if _, err := state.binding(Binding{From: "missing"}); err == nil {
		t.Fatal("binding(missing) succeeded")
	}
	if _, err := state.nodeInputs(map[string]Binding{"missing": {From: "missing"}}); err == nil {
		t.Fatal("nodeInputs(missing) succeeded")
	}
	if err := state.required(); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("required() error = %v", err)
	}
	if err := state.emit(OutputDefinition{Name: "unused"}, "value"); err != nil {
		t.Fatalf("emit(nil emitter) error = %v", err)
	}

	emitErr := errors.New("emit failed")
	state.emitter = failingOutputEmitter{err: emitErr}
	node := NodeDefinition{
		ID: "node", Outputs: map[string]string{"text": "answer"},
	}
	output := OutputDefinition{
		Node: "node", Field: "answer", Name: "assistant",
		MIMEType: "text/plain", Primary: true,
	}
	if err := state.writeNodeOutputs(node, map[string]any{"missing": "value"}, nil, nil); err == nil {
		t.Fatal("writeNodeOutputs(unknown port) succeeded")
	}
	if err := state.writeNodeOutputs(node, map[string]any{}, nil, nil); err == nil {
		t.Fatal("writeNodeOutputs(missing port) succeeded")
	}
	if err := state.writeNodeOutputs(node, map[string]any{"text": "value"}, []OutputDefinition{output}, nil); err == nil ||
		!strings.Contains(err.Error(), emitErr.Error()) {
		t.Fatalf("writeNodeOutputs(emitter) error = %v", err)
	}
	state.emitter = nil
	if err := state.writeNodeOutputs(
		node,
		map[string]any{"text": "value"},
		[]OutputDefinition{output},
		map[string]bool{"answer": true},
	); err != nil {
		t.Fatalf("writeNodeOutputs(streamed) error = %v", err)
	}
}

func TestScriptValueConversionAdversarialMatrix(t *testing.T) {
	t.Parallel()
	for _, value := range []any{
		nil, true, "text", int(1), int64(2), float64(3.5), []byte{1, 2},
		[]any{"item"}, map[string]any{"key": "value"},
		[]*schema.Message{schema.UserMessage("hello")},
		[]*schema.Document{{ID: "id", Content: "content", MetaData: map[string]any{"key": "value"}}},
	} {
		converted, err := toStarlark(value)
		if err != nil {
			t.Fatalf("toStarlark(%T) error = %v", value, err)
		}
		if _, err := fromStarlark(converted); err != nil {
			t.Fatalf("fromStarlark(toStarlark(%T)) error = %v", value, err)
		}
	}
	for _, value := range []any{
		math.NaN(), math.Inf(1), make(chan int), []any{make(chan int)},
		map[string]any{"bad": make(chan int)},
	} {
		if _, err := toStarlark(value); err == nil {
			t.Fatalf("toStarlark(%T) succeeded", value)
		}
	}
	tooLarge := starlark.MakeBigInt(newBigInt(t, "9223372036854775808"))
	if _, err := fromStarlark(tooLarge); err == nil || !strings.Contains(err.Error(), "overflows") {
		t.Fatalf("fromStarlark(overflow) error = %v", err)
	}
	if _, err := fromStarlark(starlark.Float(math.Inf(1))); err == nil {
		t.Fatal("fromStarlark(infinite) succeeded")
	}
	dictionary := starlark.NewDict(1)
	if err := dictionary.SetKey(starlark.MakeInt(1), starlark.String("value")); err != nil {
		t.Fatalf("SetKey() error = %v", err)
	}
	if _, err := fromStarlark(dictionary); err == nil || !strings.Contains(err.Error(), "key must be text") {
		t.Fatalf("fromStarlark(non-text key) error = %v", err)
	}
	set := starlark.NewSet(1)
	if err := set.Insert(starlark.String("value")); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	if _, err := fromStarlark(set); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("fromStarlark(set) error = %v", err)
	}
	listWithSet := starlark.NewList([]starlark.Value{set})
	if _, err := fromStarlark(listWithSet); err == nil {
		t.Fatal("fromStarlark(list containing set) succeeded")
	}
	if _, err := fromStarlark(starlark.Tuple{set}); err == nil {
		t.Fatal("fromStarlark(tuple containing set) succeeded")
	}
	dictWithSet := starlark.NewDict(1)
	if err := dictWithSet.SetKey(starlark.String("bad"), set); err != nil {
		t.Fatalf("SetKey(set value) error = %v", err)
	}
	if _, err := fromStarlark(dictWithSet); err == nil {
		t.Fatal("fromStarlark(dictionary containing set) succeeded")
	}

	outputTests := []struct {
		name      string
		value     any
		stateType StateType
		wantErr   string
	}{
		{name: "blob", value: "AQI=", stateType: StateBlob},
		{name: "blob type", value: 1, stateType: StateBlob, wantErr: "base64 text"},
		{name: "blob encoding", value: "!", stateType: StateBlob, wantErr: "decode base64"},
		{name: "messages type", value: "bad", stateType: StateMessages, wantErr: "must be a list"},
		{name: "message item", value: []any{"bad"}, stateType: StateMessages, wantErr: "must be an object"},
		{
			name: "message shape", value: []any{map[string]any{"role": "user"}},
			stateType: StateMessages, wantErr: "role and content",
		},
		{
			name: "message role", value: []any{map[string]any{"role": "tool", "content": "x"}},
			stateType: StateMessages, wantErr: "unsupported role",
		},
		{
			name: "messages", value: []any{map[string]any{"role": "assistant", "content": "ok"}},
			stateType: StateMessages,
		},
		{name: "documents type", value: "bad", stateType: StateDocuments, wantErr: "must be a list"},
		{name: "document item", value: []any{"bad"}, stateType: StateDocuments, wantErr: "must be an object"},
		{
			name: "document fields", value: []any{map[string]any{"id": "id"}},
			stateType: StateDocuments, wantErr: "requires id and content",
		},
		{
			name: "document metadata", value: []any{map[string]any{
				"id": "id", "content": "body", "metadata": "bad",
			}},
			stateType: StateDocuments, wantErr: "metadata must be an object",
		},
		{
			name: "document unknown", value: []any{map[string]any{
				"id": "id", "content": "body", "extra": true,
			}},
			stateType: StateDocuments, wantErr: "unknown field",
		},
		{
			name: "documents", value: []any{map[string]any{
				"id": "id", "content": "body", "metadata": map[string]any{"key": "value"},
			}},
			stateType: StateDocuments,
		},
	}
	for _, test := range outputTests {
		t.Run(test.name, func(t *testing.T) {
			_, err := convertScriptOutput(test.value, test.stateType)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("convertScriptOutput() error = %v, want containing %q", err, test.wantErr)
				}
			} else if err != nil {
				t.Fatalf("convertScriptOutput() error = %v", err)
			}
		})
	}
}

func TestCompiledScriptRejectsAdversarialOutputContracts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		source  string
		outputs map[string]StateType
		wantErr string
	}{
		{
			name:    "non dictionary",
			source:  "def run(input):\n  return 1\n",
			outputs: map[string]StateType{"out": StateString},
			wantErr: "must be a dictionary",
		},
		{
			name:    "undeclared output",
			source:  "def run(input):\n  return {\"out\": \"ok\", \"extra\": True}\n",
			outputs: map[string]StateType{"out": StateString},
			wantErr: "undeclared output",
		},
		{
			name:    "omitted output",
			source:  "def run(input):\n  return {}\n",
			outputs: map[string]StateType{"out": StateString},
			wantErr: "omitted output",
		},
		{
			name:    "wrong declared type",
			source:  "def run(input):\n  return {\"out\": []}\n",
			outputs: map[string]StateType{"out": StateString},
			wantErr: "expected string",
		},
		{
			name:    "runtime failure",
			source:  "def run(input):\n  return {\"out\": 1 // 0}\n",
			outputs: map[string]StateType{"out": StateInteger},
			wantErr: "run Script",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			script, err := compileScript(context.Background(), ScriptNode{
				Source: test.source,
				Limits: ScriptLimits{
					Timeout:           time.Second,
					MaxExecutionSteps: 1_000,
					MaxInputBytes:     1 << 10,
					MaxOutputBytes:    1 << 10,
				},
			})
			if err != nil {
				t.Fatalf("compileScript() error = %v", err)
			}
			if _, err := script.run(context.Background(), map[string]any{}, test.outputs); err == nil ||
				!strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("run() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}

	tuple := starlark.Tuple{
		starlark.String("first"),
		starlark.Tuple{starlark.MakeInt(2), starlark.Bool(true)},
	}
	converted, err := fromStarlark(tuple)
	if err != nil {
		t.Fatalf("fromStarlark(tuple) error = %v", err)
	}
	if !reflect.DeepEqual(converted, []any{"first", []any{int64(2), true}}) {
		t.Fatalf("fromStarlark(tuple) = %#v", converted)
	}

	if _, err := compileScript(context.Background(), ScriptNode{
		Source: "run = 1\n",
		Limits: ScriptLimits{
			Timeout: time.Second, MaxExecutionSteps: 1_000,
		},
	}); err == nil || !strings.Contains(err.Error(), "not callable") {
		t.Fatalf("compileScript(non-callable) error = %v", err)
	}
}

func newBigInt(t *testing.T, value string) *big.Int {
	t.Helper()
	result, ok := new(big.Int).SetString(value, 10)
	if !ok {
		t.Fatalf("SetString(%q) failed", value)
	}
	return result
}

type failingOutputEmitter struct {
	err error
}

func (emitter failingOutputEmitter) Emit(OutputDefinition, any) error {
	return emitter.err
}
