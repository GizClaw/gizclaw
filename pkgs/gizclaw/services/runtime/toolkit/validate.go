package toolkit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

type objectInputSchema struct {
	Type       any                        `json:"type"`
	Required   []string                   `json:"required"`
	Properties map[string]json.RawMessage `json:"properties"`
}

func NormalizeTool(tool Tool) (Tool, error) {
	tool.ID = strings.TrimSpace(tool.ID)
	if tool.ID == "" {
		return Tool{}, fmt.Errorf("%w: id is required", ErrInvalidTool)
	}
	if strings.Contains(tool.ID, ":") {
		return Tool{}, fmt.Errorf("%w: id must not contain ':'", ErrInvalidTool)
	}
	switch tool.Source {
	case ToolSourceBuiltin, ToolSourceDevice, ToolSourceAdmin:
	default:
		return Tool{}, fmt.Errorf("%w: unsupported source %q", ErrInvalidTool, tool.Source)
	}
	if len(tool.InputSchema) == 0 {
		return Tool{}, fmt.Errorf("%w: input_schema is required", ErrInvalidTool)
	}
	if err := validateInputSchema(tool.InputSchema); err != nil {
		return Tool{}, err
	}
	if len(tool.OutputSchema) > 0 && !json.Valid(tool.OutputSchema) {
		return Tool{}, fmt.Errorf("%w: output_schema must be valid JSON", ErrInvalidTool)
	}
	if len(tool.Metadata) > 0 && !json.Valid(tool.Metadata) {
		return Tool{}, fmt.Errorf("%w: metadata must be valid JSON", ErrInvalidTool)
	}
	if err := validateExecutor(tool); err != nil {
		return Tool{}, err
	}
	if err := validateTriggers(tool.Triggers); err != nil {
		return Tool{}, err
	}
	return cloneTool(tool), nil
}

func validateInputSchema(raw json.RawMessage) error {
	var schema objectInputSchema
	if err := json.Unmarshal(raw, &schema); err != nil {
		return fmt.Errorf("%w: input_schema must be valid JSON object", ErrInvalidTool)
	}
	if schema.Properties == nil {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
			return fmt.Errorf("%w: input_schema must be a JSON object", ErrInvalidTool)
		}
	}
	if !schemaTypeIncludesObject(schema.Type) {
		return fmt.Errorf("%w: input_schema type must be object", ErrInvalidTool)
	}
	return nil
}

func validateToolArgs(tool Tool, args json.RawMessage) error {
	var schema objectInputSchema
	if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
		return fmt.Errorf("%w: input_schema must be valid JSON object", ErrInvalidTool)
	}
	args = normalizeToolArgs(args)
	var values map[string]json.RawMessage
	if err := json.Unmarshal(args, &values); err != nil || values == nil {
		return fmt.Errorf("%w: tool arguments must be a JSON object", ErrInvalidTool)
	}
	for _, name := range schema.Required {
		if strings.TrimSpace(name) == "" {
			continue
		}
		if _, ok := values[name]; !ok {
			return fmt.Errorf("%w: tool argument %q is required", ErrInvalidTool, name)
		}
	}
	for name, propertySchema := range schema.Properties {
		value, ok := values[name]
		if !ok {
			continue
		}
		if err := validateJSONValueType(name, value, propertySchema); err != nil {
			return err
		}
	}
	return nil
}

func normalizeToolArgs(args json.RawMessage) json.RawMessage {
	if len(args) == 0 {
		return json.RawMessage(`{}`)
	}
	return args
}

func schemaTypeIncludesObject(value any) bool {
	return schemaTypeMatches(value, "object")
}

func schemaTypeMatches(value any, want string) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return typed == want
	case []any:
		for _, item := range typed {
			if item, ok := item.(string); ok && item == want {
				return true
			}
		}
	}
	return false
}

func validateJSONValueType(name string, value json.RawMessage, propertySchema json.RawMessage) error {
	var schema struct {
		Type any `json:"type"`
	}
	if err := json.Unmarshal(propertySchema, &schema); err != nil {
		return fmt.Errorf("%w: property schema %q must be valid JSON", ErrInvalidTool, name)
	}
	if schema.Type == nil {
		return nil
	}
	var decoded any
	dec := json.NewDecoder(bytes.NewReader(value))
	dec.UseNumber()
	if err := dec.Decode(&decoded); err != nil {
		return fmt.Errorf("%w: tool argument %q must be valid JSON", ErrInvalidTool, name)
	}
	types := schemaTypes(schema.Type)
	if len(types) == 0 {
		return nil
	}
	known := false
	for _, typ := range types {
		if !knownJSONSchemaType(typ) {
			continue
		}
		known = true
		if jsonValueMatchesType(decoded, typ) {
			return nil
		}
	}
	if !known {
		return nil
	}
	return fmt.Errorf("%w: tool argument %q does not match input_schema type", ErrInvalidTool, name)
}

func schemaTypes(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return []string{typed}
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if item, ok := item.(string); ok {
				out = append(out, item)
			}
		}
		return out
	}
	return nil
}

func knownJSONSchemaType(typ string) bool {
	switch typ {
	case "string", "boolean", "number", "integer", "object", "array", "null":
		return true
	default:
		return false
	}
}

func jsonValueMatchesType(decoded any, typ string) bool {
	switch typ {
	case "string":
		_, ok := decoded.(string)
		return ok
	case "boolean":
		_, ok := decoded.(bool)
		return ok
	case "number":
		_, ok := decoded.(json.Number)
		return ok
	case "integer":
		number, ok := decoded.(json.Number)
		if !ok {
			return false
		}
		f, err := number.Float64()
		return err == nil && math.Trunc(f) == f
	case "object":
		value, ok := decoded.(map[string]any)
		return ok && value != nil
	case "array":
		_, ok := decoded.([]any)
		return ok
	case "null":
		return decoded == nil
	default:
		return true
	}
}

func validateExecutor(tool Tool) error {
	switch tool.Executor.Kind {
	case ToolExecutorKindBuiltin:
		if trimPtr(tool.Executor.Name) == "" {
			return fmt.Errorf("%w: builtin executor name is required", ErrInvalidTool)
		}
	case ToolExecutorKindDeviceRPC:
		if trimPtr(tool.Executor.Method) == "" {
			return fmt.Errorf("%w: device_rpc executor method is required", ErrInvalidTool)
		}
		if trimPtr(tool.OwnerPeer) == "" && trimPtr(tool.Executor.PeerID) == "" {
			return fmt.Errorf("%w: device_rpc executor owner_peer or peer_id is required", ErrInvalidTool)
		}
	default:
		return fmt.Errorf("%w: unsupported executor kind %q", ErrInvalidTool, tool.Executor.Kind)
	}
	return nil
}

func validateTriggers(triggers []ToolTrigger) error {
	for i, trigger := range triggers {
		if strings.TrimSpace(trigger.Name) == "" {
			return fmt.Errorf("%w: triggers[%d].name is required", ErrInvalidTool, i)
		}
		if len(trigger.Metadata) > 0 && !json.Valid(trigger.Metadata) {
			return fmt.Errorf("%w: triggers[%d].metadata must be valid JSON", ErrInvalidTool, i)
		}
		for j, example := range trigger.Examples {
			if strings.TrimSpace(example.Input) == "" {
				return fmt.Errorf("%w: triggers[%d].examples[%d].input is required", ErrInvalidTool, i, j)
			}
			if len(example.Args) > 0 && !json.Valid(example.Args) {
				return fmt.Errorf("%w: triggers[%d].examples[%d].args must be valid JSON", ErrInvalidTool, i, j)
			}
		}
	}
	return nil
}

func trimPtr(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
