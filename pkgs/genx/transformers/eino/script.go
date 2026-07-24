package eino

import (
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"strings"

	"github.com/cloudwego/eino/schema"
	"go.starlark.net/starlark"
)

type compiledScript struct {
	config ScriptNode
	entry  starlark.Callable
}

func compileScript(config ScriptNode) (*compiledScript, error) {
	_, program, err := starlark.SourceProgram("eino.star", config.Source, func(string) bool { return false })
	if err != nil {
		return nil, fmt.Errorf("eino: compile Script: %w", err)
	}
	if strings.TrimSpace(config.Entrypoint) == "" {
		config.Entrypoint = "run"
	}
	thread := &starlark.Thread{Name: "eino-script-init"}
	thread.SetMaxExecutionSteps(config.Limits.MaxExecutionSteps)
	globals, err := program.Init(thread, nil)
	if err != nil {
		return nil, fmt.Errorf("eino: initialize Script: %w", err)
	}
	entry, ok := globals[config.Entrypoint]
	if !ok {
		return nil, fmt.Errorf("eino: Script entrypoint %q not found", config.Entrypoint)
	}
	callable, ok := entry.(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("eino: Script entrypoint %q is not callable", config.Entrypoint)
	}
	return &compiledScript{config: config, entry: callable}, nil
}

func (script *compiledScript) run(
	ctx context.Context,
	inputs map[string]any,
	outputs map[string]StateType,
) (map[string]any, error) {
	if err := encodeBounded(inputs, script.config.Limits.MaxInputBytes); err != nil {
		return nil, fmt.Errorf("eino: Script input: %w", err)
	}
	thread := &starlark.Thread{Name: "eino-script"}
	thread.SetMaxExecutionSteps(script.config.Limits.MaxExecutionSteps)
	runCtx, cancel := context.WithTimeout(ctx, script.config.Limits.Timeout)
	defer cancel()
	done := make(chan struct{})
	go func() {
		select {
		case <-runCtx.Done():
			thread.Cancel(context.Cause(runCtx).Error())
		case <-done:
		}
	}()
	input, err := toStarlark(inputs)
	if err != nil {
		close(done)
		return nil, fmt.Errorf("eino: convert Script input: %w", err)
	}
	value, err := starlark.Call(thread, script.entry, starlark.Tuple{input}, nil)
	close(done)
	if err != nil {
		return nil, fmt.Errorf("eino: run Script: %w", err)
	}
	result, err := fromStarlark(value)
	if err != nil {
		return nil, fmt.Errorf("eino: convert Script output: %w", err)
	}
	object, ok := result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("eino: Script output must be a dictionary")
	}
	for key := range object {
		if _, declared := outputs[key]; !declared {
			return nil, fmt.Errorf("eino: Script returned undeclared output %q", key)
		}
	}
	for key := range outputs {
		if _, exists := object[key]; !exists {
			return nil, fmt.Errorf("eino: Script omitted output %q", key)
		}
	}
	for key, stateType := range outputs {
		converted, err := convertScriptOutput(object[key], stateType)
		if err != nil {
			return nil, fmt.Errorf("eino: Script output %q: %w", key, err)
		}
		object[key] = converted
	}
	if err := encodeBounded(object, script.config.Limits.MaxOutputBytes); err != nil {
		return nil, fmt.Errorf("eino: Script output: %w", err)
	}
	return object, nil
}

func convertScriptOutput(value any, stateType StateType) (any, error) {
	switch stateType {
	case StateBlob:
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("binary output must be base64 text")
		}
		decoded, err := base64.StdEncoding.DecodeString(text)
		if err != nil {
			return nil, fmt.Errorf("decode base64 binary: %w", err)
		}
		return decoded, nil
	case StateMessages:
		items, ok := value.([]any)
		if !ok {
			return nil, fmt.Errorf("messages output must be a list")
		}
		messages := make([]*schema.Message, len(items))
		for index, item := range items {
			object, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("message %d must be an object", index)
			}
			role, roleOK := object["role"].(string)
			content, contentOK := object["content"].(string)
			if !roleOK || !contentOK || len(object) != 2 {
				return nil, fmt.Errorf("message %d requires only role and content", index)
			}
			switch schema.RoleType(role) {
			case schema.System, schema.User, schema.Assistant:
				messages[index] = &schema.Message{Role: schema.RoleType(role), Content: content}
			default:
				return nil, fmt.Errorf("message %d has unsupported role %q", index, role)
			}
		}
		return messages, nil
	case StateDocuments:
		items, ok := value.([]any)
		if !ok {
			return nil, fmt.Errorf("documents output must be a list")
		}
		documents := make([]*schema.Document, len(items))
		for index, item := range items {
			object, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("document %d must be an object", index)
			}
			id, idOK := object["id"].(string)
			content, contentOK := object["content"].(string)
			if !idOK || !contentOK {
				return nil, fmt.Errorf("document %d requires id and content", index)
			}
			document := &schema.Document{ID: id, Content: content}
			if metadata, exists := object["metadata"]; exists {
				var metadataOK bool
				document.MetaData, metadataOK = metadata.(map[string]any)
				if !metadataOK {
					return nil, fmt.Errorf("document %d metadata must be an object", index)
				}
			}
			for key := range object {
				if key != "id" && key != "content" && key != "metadata" {
					return nil, fmt.Errorf("document %d has unknown field %q", index, key)
				}
			}
			documents[index] = document
		}
		return documents, nil
	default:
		return normalizeStateValue(value, stateType)
	}
}

func toStarlark(value any) (starlark.Value, error) {
	switch typed := value.(type) {
	case nil:
		return starlark.None, nil
	case bool:
		return starlark.Bool(typed), nil
	case string:
		return starlark.String(typed), nil
	case int:
		return starlark.MakeInt(typed), nil
	case int64:
		return starlark.MakeInt64(typed), nil
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return nil, fmt.Errorf("non-finite number")
		}
		return starlark.Float(typed), nil
	case []byte:
		return starlark.String(base64.StdEncoding.EncodeToString(typed)), nil
	case []any:
		items := make([]starlark.Value, len(typed))
		for index, item := range typed {
			converted, err := toStarlark(item)
			if err != nil {
				return nil, err
			}
			items[index] = converted
		}
		return starlark.NewList(items), nil
	case map[string]any:
		dictionary := starlark.NewDict(len(typed))
		for key, item := range typed {
			converted, err := toStarlark(item)
			if err != nil {
				return nil, err
			}
			if err := dictionary.SetKey(starlark.String(key), converted); err != nil {
				return nil, err
			}
		}
		dictionary.Freeze()
		return dictionary, nil
	case []*schema.Message:
		items := make([]any, len(typed))
		for index, message := range typed {
			items[index] = map[string]any{"role": string(message.Role), "content": message.Content}
		}
		return toStarlark(items)
	case []*schema.Document:
		items := make([]any, len(typed))
		for index, document := range typed {
			items[index] = map[string]any{
				"id": document.ID, "content": document.Content, "metadata": document.MetaData,
			}
		}
		return toStarlark(items)
	default:
		return nil, fmt.Errorf("unsupported value %T", value)
	}
}

func fromStarlark(value starlark.Value) (any, error) {
	switch typed := value.(type) {
	case starlark.NoneType:
		return nil, nil
	case starlark.Bool:
		return bool(typed), nil
	case starlark.String:
		return string(typed), nil
	case starlark.Int:
		integer, ok := typed.Int64()
		if !ok {
			return nil, fmt.Errorf("integer overflows int64")
		}
		return integer, nil
	case starlark.Float:
		number := float64(typed)
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return nil, fmt.Errorf("non-finite number")
		}
		return number, nil
	case *starlark.List:
		result := make([]any, typed.Len())
		for index := range typed.Len() {
			converted, err := fromStarlark(typed.Index(index))
			if err != nil {
				return nil, err
			}
			result[index] = converted
		}
		return result, nil
	case starlark.Tuple:
		result := make([]any, len(typed))
		for index, item := range typed {
			converted, err := fromStarlark(item)
			if err != nil {
				return nil, err
			}
			result[index] = converted
		}
		return result, nil
	case *starlark.Dict:
		result := make(map[string]any, typed.Len())
		for _, item := range typed.Items() {
			key, ok := item[0].(starlark.String)
			if !ok {
				return nil, fmt.Errorf("dictionary key must be text")
			}
			converted, err := fromStarlark(item[1])
			if err != nil {
				return nil, err
			}
			result[string(key)] = converted
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported Starlark value %s", value.Type())
	}
}
