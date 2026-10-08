package apitypes

import (
	"encoding/json"
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/jsonschema-go/jsonschema"
)

// InnerToolSchema projects device arguments from the embedded HTTP contracts.
// Target identity belongs to the binding, never to the model's arguments.
func InnerToolSchema(source, name string, fields []string) (jsonschema.Schema, error) {
	validator := clientToolValidator
	if source == "mhs" {
		validator = mhsWriteValidator
	}
	validator.once.Do(func() { validator.schema, validator.err = validator.load() })
	if validator.err != nil {
		return jsonschema.Schema{}, validator.err
	}
	if validator.schema == nil {
		return jsonschema.Schema{}, fmt.Errorf("missing device Tool contract")
	}
	var schema *openapi3.Schema
	switch source {
	case "client_tool":
		for _, ref := range validator.schema.OneOf {
			target := ref.Value.Properties["tool"].Value
			if len(target.Enum) == 1 && target.Enum[0] == name {
				schema = ref.Value.Properties["args"].Value
				break
			}
		}
	case "mhs":
		if len(fields) == 0 {
			return jsonschema.Schema{}, fmt.Errorf("MHS write fields must be explicit")
		}
		for _, ref := range validator.schema.OneOf {
			target := ref.Value.Properties["hwd"].Value
			if len(target.Enum) == 1 && target.Enum[0] == name {
				schema = ref.Value.Properties["value"].Value
				break
			}
		}
	default:
		return jsonschema.Schema{}, fmt.Errorf("unknown inner Tool source %q", source)
	}
	if schema == nil {
		return jsonschema.Schema{}, fmt.Errorf("no %s argument schema for %q", source, name)
	}
	value, err := inlineToolSchema(schema, 0)
	if err != nil {
		return jsonschema.Schema{}, err
	}
	if source == "mhs" {
		properties := value["properties"].(map[string]any)
		selected := make(map[string]any, len(fields))
		for _, field := range fields {
			property, ok := properties[field]
			if !ok {
				return jsonschema.Schema{}, fmt.Errorf("%s has no writable field %q", name, field)
			}
			if _, duplicate := selected[field]; duplicate {
				return jsonschema.Schema{}, fmt.Errorf("duplicate write field %q", field)
			}
			selected[field] = property
		}
		value["properties"] = selected
		value["minProperties"] = 1
	}
	value["additionalProperties"] = false
	data, err := json.Marshal(value)
	if err != nil {
		return jsonschema.Schema{}, err
	}
	var result jsonschema.Schema
	err = json.Unmarshal(data, &result)
	return result, err
}

// inlineToolSchema follows the loader's resolved references. All device input
// contracts are finite; a depth bound rejects unexpected recursive contracts.
func inlineToolSchema(schema *openapi3.Schema, depth int) (map[string]any, error) {
	if schema == nil || depth > 32 {
		return nil, fmt.Errorf("invalid or recursive Tool argument schema")
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	if schema.Properties != nil {
		properties := make(map[string]any, len(schema.Properties))
		for name, ref := range schema.Properties {
			value, err := inlineToolSchema(ref.Value, depth+1)
			if err != nil {
				return nil, err
			}
			properties[name] = value
		}
		result["properties"] = properties
	}
	if schema.Items != nil {
		value, err := inlineToolSchema(schema.Items.Value, depth+1)
		if err != nil {
			return nil, err
		}
		result["items"] = value
	}
	for _, composition := range []struct {
		name string
		refs openapi3.SchemaRefs
	}{{"oneOf", schema.OneOf}, {"anyOf", schema.AnyOf}, {"allOf", schema.AllOf}} {
		if len(composition.refs) == 0 {
			continue
		}
		values := make([]any, 0, len(composition.refs))
		for _, ref := range composition.refs {
			value, err := inlineToolSchema(ref.Value, depth+1)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		result[composition.name] = values
	}
	return result, nil
}
