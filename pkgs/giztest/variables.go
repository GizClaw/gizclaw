package giztest

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

type value struct {
	data any
	spec VariableSpec
}

// Variables holds one task's variable values. Input variables are populated
// at construction; each output variable is assigned exactly once by the step
// that produces it.
type Variables struct{ values map[string]value }

func (v *Variables) release() {
	if v == nil {
		return
	}
	for name, item := range v.values {
		if data, ok := item.data.([]byte); ok {
			clear(data)
		}
		item.data = nil
		v.values[name] = item
	}
}

func (v *Variables) redactions(names []string) []string {
	if v == nil {
		return nil
	}
	explicit := make(map[string]struct{}, len(names))
	for _, name := range names {
		explicit[name] = struct{}{}
	}
	var redactions []string
	for name, item := range v.values {
		_, requested := explicit[name]
		if !item.spec.Secret && !requested {
			continue
		}
		if text, ok := item.data.(string); ok && text != "" {
			redactions = append(redactions, text)
		}
	}
	sort.Slice(redactions, func(i, j int) bool {
		return len(redactions[i]) > len(redactions[j])
	})
	return redactions
}

// NewVariables materializes the document's declared variables, reading
// environment inputs and generating requested values. Output variables start
// unassigned.
func NewVariables(specs map[string]VariableSpec) (*Variables, error) {
	v := &Variables{values: make(map[string]value, len(specs))}
	for name, spec := range specs {
		if spec.Direction == "output" {
			v.values[name] = value{spec: spec}
			continue
		}
		var data any
		switch {
		case spec.Env != "":
			text, ok := os.LookupEnv(spec.Env)
			if !ok {
				return nil, fmt.Errorf("input variable %s requires environment %s", name, spec.Env)
			}
			data = text
			if spec.Type == "audio" || spec.Type == "binary" {
				decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
				if err != nil {
					return nil, fmt.Errorf("input variable %s: environment %s must hold standard base64 %s bytes: %w", name, spec.Env, spec.Type, err)
				}
				data = decoded
			}
		case spec.Generate != "":
			generated, err := generateValue(spec.Generate)
			if err != nil {
				return nil, fmt.Errorf("variable %s: %w", name, err)
			}
			data = generated
		default:
			data = spec.Value
		}
		if err := CheckValueType(spec, data); err != nil {
			return nil, fmt.Errorf("variable %s: %w", name, err)
		}
		v.values[name] = value{data: data, spec: spec}
	}
	return v, nil
}

func generateValue(kind string) (string, error) {
	size := 16
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	if kind == "uuid" {
		b[6] = b[6]&0x0f | 0x40
		b[8] = b[8]&0x3f | 0x80
		s := hex.EncodeToString(b)
		return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:], nil
	}
	return "g" + hex.EncodeToString(b), nil
}

func CheckValueType(spec VariableSpec, data any) error {
	if data == nil && spec.Direction == "output" {
		return nil
	}
	switch spec.Type {
	case "string":
		if _, ok := data.(string); !ok {
			return fmt.Errorf("want string")
		}
	case "boolean":
		if _, ok := data.(bool); !ok {
			return fmt.Errorf("want boolean")
		}
	case "integer":
		if !isInteger(data) {
			return fmt.Errorf("want integer")
		}
	case "number":
		if !isNumber(data) {
			return fmt.Errorf("want number")
		}
	case "object":
		if _, ok := data.(map[string]any); !ok {
			return fmt.Errorf("want object")
		}
	case "audio", "binary":
		if _, ok := data.([]byte); !ok {
			return fmt.Errorf("want in-memory bytes")
		}
	default:
		return fmt.Errorf("unsupported type %q", spec.Type)
	}
	if b, ok := data.([]byte); ok && spec.MaxBytes > 0 && len(b) > spec.MaxBytes {
		return fmt.Errorf("value exceeds max_bytes")
	}
	return nil
}

func isNumber(v any) bool {
	switch v.(type) {
	case int, int64, float64, json.Number:
		return true
	}
	return false
}
func isInteger(v any) bool {
	switch x := v.(type) {
	case int, int64:
		return true
	case float64:
		return x == float64(int64(x))
	case json.Number:
		_, err := x.Int64()
		return err == nil
	}
	return false
}

func (v *Variables) assign(name string, data any) error {
	current, ok := v.values[name]
	if !ok {
		return fmt.Errorf("unknown variable %q", name)
	}
	if current.spec.Direction != "output" {
		return fmt.Errorf("variable %q is not output", name)
	}
	if current.data != nil {
		return fmt.Errorf("variable %q already assigned", name)
	}
	if err := CheckValueType(current.spec, data); err != nil {
		return err
	}
	current.data = data
	v.values[name] = current
	return nil
}

// Resolve substitutes ${name} references in input, recursing through slices
// and maps. A string that is exactly one reference resolves to the variable's
// typed value; a reference embedded in a longer string requires a string
// variable.
func (v *Variables) Resolve(input any) (any, error) {
	switch x := input.(type) {
	case string:
		if m := ReferencePattern.FindStringSubmatch(x); m != nil {
			item, ok := v.values[m[1]]
			if !ok || item.data == nil {
				return nil, fmt.Errorf("variable %q unavailable", m[1])
			}
			return item.data, nil
		}
		result := x
		for _, m := range regexpReferences(x) {
			item, ok := v.values[m]
			if !ok || item.data == nil {
				return nil, fmt.Errorf("variable %q unavailable", m)
			}
			scalar, ok := item.data.(string)
			if !ok {
				return nil, fmt.Errorf("embedded variable %q must be string", m)
			}
			result = strings.ReplaceAll(result, "${"+m+"}", scalar)
		}
		return result, nil
	case []any:
		out := make([]any, len(x))
		for i := range x {
			value, err := v.Resolve(x[i])
			if err != nil {
				return nil, err
			}
			out[i] = value
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, raw := range x {
			value, err := v.Resolve(raw)
			if err != nil {
				return nil, err
			}
			out[k] = value
		}
		return out, nil
	default:
		return input, nil
	}
}

// ReferencedSpec reports the spec of the variable input references, when
// input is exactly one ${name} reference.
func (v *Variables) ReferencedSpec(input any) (VariableSpec, bool) {
	text, ok := input.(string)
	if !ok {
		return VariableSpec{}, false
	}
	match := ReferencePattern.FindStringSubmatch(text)
	if match == nil {
		return VariableSpec{}, false
	}
	item, ok := v.values[match[1]]
	return item.spec, ok
}

// Value reports the current value of one variable. An output variable that no
// step has produced yet reports nil.
func (v *Variables) Value(name string) (any, bool) {
	item, ok := v.values[name]
	return item.data, ok
}

// Spec reports the declared spec of one variable.
func (v *Variables) Spec(name string) (VariableSpec, bool) {
	item, ok := v.values[name]
	return item.spec, ok
}

func regexpReferences(s string) []string {
	matches := AllReferencesPattern.FindAllStringSubmatch(s, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

var AllReferencesPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
