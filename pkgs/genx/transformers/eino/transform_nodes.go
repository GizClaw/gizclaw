package eino

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/schema"
)

func runTransform(config TransformNode, inputs map[string]any) (map[string]any, error) {
	switch config.Operation {
	case TransformSelect:
		value, ok := inputs["value"]
		if !ok {
			return nil, fmt.Errorf("eino: select requires value")
		}
		return map[string]any{"value": value}, nil
	case TransformConcatText:
		if len(config.Order) == 0 {
			return nil, fmt.Errorf("eino: concat_text requires Order")
		}
		parts := make([]string, len(config.Order))
		for index, name := range config.Order {
			value, ok := inputs[name].(string)
			if !ok {
				return nil, fmt.Errorf("eino: concat_text input %q is not text", name)
			}
			parts[index] = value
		}
		result := strings.Join(parts, config.Separator)
		if config.MaxOutputBytes > 0 && len(result) > config.MaxOutputBytes {
			return nil, fmt.Errorf("eino: concat_text output exceeds %d bytes", config.MaxOutputBytes)
		}
		return map[string]any{"text": result}, nil
	case TransformDecodeJSON:
		text, ok := inputs["text"].(string)
		if !ok {
			return nil, fmt.Errorf("eino: decode_json requires text")
		}
		if config.MaxInputBytes > 0 && len(text) > config.MaxInputBytes {
			return nil, fmt.Errorf("eino: decode_json input exceeds %d bytes", config.MaxInputBytes)
		}
		object, err := decodeJSONObject(text)
		if err != nil {
			return nil, err
		}
		if config.MaxOutputBytes > 0 {
			data, _ := json.Marshal(object)
			if len(data) > config.MaxOutputBytes {
				return nil, fmt.Errorf("eino: decode_json output exceeds %d bytes", config.MaxOutputBytes)
			}
		}
		return map[string]any{"object": object}, nil
	case TransformBuildMessages:
		if len(config.Messages) == 0 {
			return nil, fmt.Errorf("eino: build_messages requires Messages")
		}
		messages := make([]*schema.Message, 0, len(config.Messages))
		for index, item := range config.Messages {
			if (item.Input == "") == (item.Text == "") {
				return nil, fmt.Errorf("eino: build_messages item %d requires exactly one Input or Text", index)
			}
			text := item.Text
			if item.Input != "" {
				var ok bool
				text, ok = inputs[item.Input].(string)
				if !ok {
					return nil, fmt.Errorf("eino: build_messages input %q is not text", item.Input)
				}
			}
			message, err := messageForRole(item.Role, text)
			if err != nil {
				return nil, err
			}
			messages = append(messages, message)
		}
		return map[string]any{"messages": messages}, nil
	default:
		return nil, fmt.Errorf("eino: unsupported Transform operation %q", config.Operation)
	}
}

func decodeJSONObject(text string) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("eino: decode JSON: %w", err)
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return nil, fmt.Errorf("eino: decode_json requires an object")
	}
	result := make(map[string]any)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("eino: decode JSON key: %w", err)
		}
		key := keyToken.(string)
		if _, duplicate := result[key]; duplicate {
			return nil, fmt.Errorf("eino: decode_json duplicate key %q", key)
		}
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("eino: decode JSON value %q: %w", key, err)
		}
		result[key] = normalizeJSONNumbers(value)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("eino: decode JSON object: %w", err)
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("eino: decode_json has trailing token %v", token)
		}
		return nil, fmt.Errorf("eino: decode JSON trailing data: %w", err)
	}
	return result, nil
}

func normalizeJSONNumbers(value any) any {
	switch typed := value.(type) {
	case json.Number:
		if integer, err := typed.Int64(); err == nil {
			return integer
		}
		number, _ := typed.Float64()
		return number
	case []any:
		for index := range typed {
			typed[index] = normalizeJSONNumbers(typed[index])
		}
	case map[string]any:
		for key := range typed {
			typed[key] = normalizeJSONNumbers(typed[key])
		}
	}
	return value
}

func messageForRole(role PromptRole, text string) (*schema.Message, error) {
	switch role {
	case PromptSystem:
		return schema.SystemMessage(text), nil
	case PromptUser:
		return schema.UserMessage(text), nil
	case PromptAssistant:
		return schema.AssistantMessage(text, nil), nil
	default:
		return nil, fmt.Errorf("eino: unsupported Prompt role %q", role)
	}
}

func encodeBounded(value any, limit int) error {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	if err := encoder.Encode(value); err != nil {
		return err
	}
	if limit > 0 && buffer.Len() > limit {
		return fmt.Errorf("encoded value exceeds %d bytes", limit)
	}
	return nil
}
