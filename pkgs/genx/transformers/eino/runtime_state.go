package eino

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/cloudwego/eino/schema"
)

const maxStateValueDepth = 64

type graphInput struct {
	Text     string
	Messages []*schema.Message
	Parts    []any
	History  []*schema.Message
	Memory   string
}

type outputEmitter interface {
	Emit(OutputDefinition, any) error
}

type runState struct {
	mu      sync.RWMutex
	fields  map[string]StateField
	values  map[string]any
	input   graphInput
	emitter outputEmitter
}

func newRunState(fields map[string]StateField, input graphInput, initial map[string]any, emitter outputEmitter) (*runState, error) {
	state := &runState{
		fields: maps.Clone(fields), values: make(map[string]any, len(fields)),
		input: cloneGraphInput(input), emitter: emitter,
	}
	for name, value := range initial {
		if _, ok := fields[name]; !ok {
			continue
		}
		if err := state.set(name, value); err != nil {
			return nil, err
		}
	}
	return state, nil
}

func (state *runState) clone(emitter outputEmitter) (*runState, error) {
	state.mu.RLock()
	defer state.mu.RUnlock()
	values, err := cloneMap(state.values)
	if err != nil {
		return nil, err
	}
	return &runState{
		fields: maps.Clone(state.fields), values: values, input: cloneGraphInput(state.input),
		emitter: emitter,
	}, nil
}

func cloneGraphInput(input graphInput) graphInput {
	result := input
	result.Messages = cloneMessages(input.Messages)
	result.History = cloneMessages(input.History)
	result.Parts = cloneParts(input.Parts)
	return result
}

func cloneParts(parts []any) []any {
	result := make([]any, len(parts))
	for index, part := range parts {
		switch typed := part.(type) {
		case *genx.Blob:
			if typed != nil {
				result[index] = &genx.Blob{
					MIMEType: typed.MIMEType,
					Data:     slices.Clone(typed.Data),
				}
			}
		default:
			result[index] = typed
		}
	}
	return result
}

func cloneMessages(messages []*schema.Message) []*schema.Message {
	data, err := json.Marshal(messages)
	if err == nil {
		var result []*schema.Message
		if json.Unmarshal(data, &result) == nil {
			return result
		}
	}
	result := make([]*schema.Message, len(messages))
	for index, message := range messages {
		if message != nil {
			copyMessage := *message
			copyMessage.ToolCalls = slices.Clone(message.ToolCalls)
			copyMessage.MultiContent = slices.Clone(message.MultiContent)
			copyMessage.UserInputMultiContent = slices.Clone(message.UserInputMultiContent)
			copyMessage.AssistantGenMultiContent = slices.Clone(message.AssistantGenMultiContent)
			copyMessage.Extra = maps.Clone(message.Extra)
			result[index] = &copyMessage
		}
	}
	return result
}

func (state *runState) binding(binding Binding) (any, error) {
	source := strings.TrimSpace(binding.From)
	state.mu.RLock()
	defer state.mu.RUnlock()
	switch source {
	case "input.text":
		return state.input.Text, nil
	case "input.messages":
		return cloneMessages(state.input.Messages), nil
	case "input.parts":
		return cloneParts(state.input.Parts), nil
	case "history.messages":
		return cloneMessages(state.input.History), nil
	case "memory.recalled":
		return state.input.Memory, nil
	}
	value, ok := state.values[source]
	if !ok {
		return nil, fmt.Errorf("eino: binding %q has no value", source)
	}
	return cloneValue(value)
}

func (state *runState) nodeInputs(bindings map[string]Binding) (map[string]any, error) {
	result := make(map[string]any, len(bindings))
	for port, binding := range bindings {
		value, err := state.binding(binding)
		if err != nil {
			return nil, fmt.Errorf("input %q: %w", port, err)
		}
		result[port] = value
	}
	return result, nil
}

func (state *runState) set(name string, value any) error {
	field, ok := state.fields[name]
	if !ok {
		return fmt.Errorf("eino: State field %q is not declared", name)
	}
	owned, err := normalizeStateValue(value, field.Type)
	if err != nil {
		return fmt.Errorf("eino: State field %q: %w", name, err)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	current, exists := state.values[name]
	if !exists || field.Merge == MergeReplace {
		state.values[name] = owned
		return nil
	}
	merged, err := mergeStateValue(current, owned, field)
	if err != nil {
		return fmt.Errorf("eino: State field %q: %w", name, err)
	}
	state.values[name] = merged
	return nil
}

func (state *runState) writeNodeOutputs(node NodeDefinition, outputs map[string]any, published []OutputDefinition, streamed map[string]bool) error {
	for port, field := range node.Outputs {
		value, ok := outputs[port]
		if !ok {
			return fmt.Errorf("eino: node %q did not produce port %q", node.ID, port)
		}
		if err := state.set(field, value); err != nil {
			return err
		}
	}
	for _, output := range published {
		if streamed[output.Field] {
			continue
		}
		value, err := state.value(output.Field)
		if err != nil {
			return err
		}
		if state.emitter != nil {
			if err := state.emitter.Emit(output, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func (state *runState) emit(output OutputDefinition, value any) error {
	if state.emitter == nil {
		return nil
	}
	return state.emitter.Emit(output, value)
}

func (state *runState) value(name string) (any, error) {
	state.mu.RLock()
	defer state.mu.RUnlock()
	value, ok := state.values[name]
	if !ok {
		return nil, fmt.Errorf("eino: State field %q has no value", name)
	}
	return cloneValue(value)
}

func (state *runState) snapshot() (map[string]any, error) {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return cloneMap(state.values)
}

func (state *runState) required() error {
	state.mu.RLock()
	defer state.mu.RUnlock()
	for name, field := range state.fields {
		if field.Required {
			if _, ok := state.values[name]; !ok {
				return fmt.Errorf("eino: required State field %q has no value", name)
			}
		}
	}
	return nil
}

func normalizeStateValue(value any, stateType StateType) (any, error) {
	if value == nil {
		return nil, fmt.Errorf("nil is not a valid %s value", stateType)
	}
	switch stateType {
	case StateString:
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("expected string, got %T", value)
		}
		return text, nil
	case StateBoolean:
		boolean, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("expected boolean, got %T", value)
		}
		return boolean, nil
	case StateInteger:
		return normalizeInteger(value)
	case StateNumber:
		return normalizeNumber(value)
	case StateObject:
		object, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected object, got %T", value)
		}
		return cloneMap(object)
	case StateList:
		list, ok := value.([]any)
		if !ok {
			return nil, fmt.Errorf("expected list, got %T", value)
		}
		return cloneSlice(list)
	case StateMessages:
		messages, ok := value.([]*schema.Message)
		if !ok {
			return nil, fmt.Errorf("expected messages, got %T", value)
		}
		return cloneMessages(messages), nil
	case StateDocuments:
		documents, ok := value.([]*schema.Document)
		if !ok {
			return nil, fmt.Errorf("expected documents, got %T", value)
		}
		result := make([]*schema.Document, len(documents))
		for index, document := range documents {
			if document != nil {
				copyDocument := *document
				copyDocument.MetaData = maps.Clone(document.MetaData)
				result[index] = &copyDocument
			}
		}
		return result, nil
	case StateBlob:
		blob, ok := value.([]byte)
		if !ok {
			return nil, fmt.Errorf("expected blob, got %T", value)
		}
		return slices.Clone(blob), nil
	default:
		return nil, fmt.Errorf("unsupported type %q", stateType)
	}
}

func normalizeInteger(value any) (int64, error) {
	switch number := value.(type) {
	case int:
		return int64(number), nil
	case int8:
		return int64(number), nil
	case int16:
		return int64(number), nil
	case int32:
		return int64(number), nil
	case int64:
		return number, nil
	case uint:
		return checkedUint64(uint64(number))
	case uint8:
		return int64(number), nil
	case uint16:
		return int64(number), nil
	case uint32:
		return int64(number), nil
	case uint64:
		return checkedUint64(number)
	case float64:
		if number != float64(int64(number)) {
			return 0, fmt.Errorf("expected integral number")
		}
		return int64(number), nil
	default:
		return 0, fmt.Errorf("expected integer, got %T", value)
	}
}

func checkedUint64(value uint64) (int64, error) {
	converted := int64(value)
	if converted < 0 {
		return 0, fmt.Errorf("integer overflows int64")
	}
	return converted, nil
}

func normalizeNumber(value any) (float64, error) {
	reflectValue := reflect.ValueOf(value)
	switch reflectValue.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(reflectValue.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(reflectValue.Uint()), nil
	case reflect.Float32, reflect.Float64:
		result := reflectValue.Convert(reflect.TypeFor[float64]()).Float()
		if result != result || result > 1.7976931348623157e308 || result < -1.7976931348623157e308 {
			return 0, fmt.Errorf("number must be finite")
		}
		return result, nil
	default:
		return 0, fmt.Errorf("expected number, got %T", value)
	}
}

func mergeStateValue(current, next any, field StateField) (any, error) {
	switch field.Merge {
	case MergeAppend:
		switch field.Type {
		case StateString:
			return current.(string) + next.(string), nil
		case StateList:
			return append(current.([]any), next.([]any)...), nil
		case StateMessages:
			return append(current.([]*schema.Message), next.([]*schema.Message)...), nil
		case StateDocuments:
			return append(current.([]*schema.Document), next.([]*schema.Document)...), nil
		}
	case MergeObject:
		result, err := cloneMap(current.(map[string]any))
		if err != nil {
			return nil, err
		}
		maps.Copy(result, next.(map[string]any))
		return result, nil
	}
	return next, nil
}

func cloneValue(value any) (any, error) {
	return cloneValueAtDepth(value, 0)
}

func cloneValueAtDepth(value any, depth int) (any, error) {
	if depth > maxStateValueDepth {
		return nil, fmt.Errorf("copy value: nesting exceeds %d", maxStateValueDepth)
	}
	switch typed := value.(type) {
	case nil, bool, string,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return typed, nil
	case []any:
		return cloneSliceAtDepth(typed, depth)
	case map[string]any:
		return cloneMapAtDepth(typed, depth)
	case []*schema.Message:
		return cloneMessages(typed), nil
	case []*schema.Document:
		return normalizeStateValue(typed, StateDocuments)
	case []byte:
		return slices.Clone(typed), nil
	default:
		return nil, fmt.Errorf("copy value: unsupported type %T", value)
	}
}

func cloneMap(source map[string]any) (map[string]any, error) {
	return cloneMapAtDepth(source, 0)
}

func cloneMapAtDepth(source map[string]any, depth int) (map[string]any, error) {
	if depth > maxStateValueDepth {
		return nil, fmt.Errorf("nesting exceeds %d", maxStateValueDepth)
	}
	if source == nil {
		return nil, nil
	}
	result := make(map[string]any, len(source))
	for key, value := range source {
		cloned, err := cloneValueAtDepth(value, depth+1)
		if err != nil {
			return nil, fmt.Errorf("copy %q: %w", key, err)
		}
		result[key] = cloned
	}
	return result, nil
}

func cloneSlice(source []any) ([]any, error) {
	return cloneSliceAtDepth(source, 0)
}

func cloneSliceAtDepth(source []any, depth int) ([]any, error) {
	if depth > maxStateValueDepth {
		return nil, fmt.Errorf("nesting exceeds %d", maxStateValueDepth)
	}
	result := make([]any, len(source))
	for index, value := range source {
		cloned, err := cloneValueAtDepth(value, depth+1)
		if err != nil {
			return nil, fmt.Errorf("copy item %d: %w", index, err)
		}
		result[index] = cloned
	}
	return result, nil
}
