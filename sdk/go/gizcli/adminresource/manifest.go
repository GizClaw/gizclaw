package adminresource

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/goccy/go-yaml"
)

// Format is the encoding of a resource manifest.
type Format string

const (
	FormatJSON Format = "json"
	FormatYAML Format = "yaml"
)

// FormatForPath returns the manifest format selected by a file path. The path
// "-" denotes JSON standard input. Other paths select JSON for .json and YAML
// for .yaml or .yml.
func FormatForPath(path string) (Format, error) {
	if path == "-" {
		return FormatJSON, nil
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return FormatJSON, nil
	case ".yaml", ".yml":
		return FormatYAML, nil
	default:
		return "", &UnsupportedFormatError{Extension: filepath.Ext(path)}
	}
}

// PrepareManifest converts a manifest to JSON ready for decoding.
//
// ${NAME} and ${NAME:-default} references are replaced from the process
// environment; an unset or empty variable without a default fails with
// *MissingEnvError. In JSON, a replacement inside a string literal is escaped
// as a string fragment and a replacement outside a string is inserted as raw
// JSON. In YAML, replacements apply to string scalars. A kind written as
// <Kind>Resource is normalized to <Kind>; any other unknown kind fails with
// *UnknownKindError.
func PrepareManifest(format Format, data []byte) ([]byte, error) {
	switch format {
	case FormatJSON:
		expanded, err := expandJSONEnv(data)
		if err != nil {
			return nil, err
		}
		data = expanded
	case FormatYAML:
		var value any
		if err := yaml.Unmarshal(data, &value); err != nil {
			return nil, err
		}
		expanded, err := expandYAMLValue(value)
		if err != nil {
			return nil, err
		}
		data, err = json.Marshal(expanded)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported resource manifest format %q", format)
	}
	return normalizeKind(data)
}

// DecodeManifest prepares a manifest with PrepareManifest and decodes it.
func DecodeManifest(format Format, data []byte) (apitypes.Resource, error) {
	prepared, err := PrepareManifest(format, data)
	if err != nil {
		return apitypes.Resource{}, err
	}
	return DecodePrepared(prepared)
}

// DecodePrepared decodes JSON returned by PrepareManifest.
func DecodePrepared(data []byte) (apitypes.Resource, error) {
	var resource apitypes.Resource
	if err := json.Unmarshal(data, &resource); err != nil {
		return apitypes.Resource{}, err
	}
	return resource, nil
}

// UnsupportedFormatError reports a manifest path with an unsupported extension.
type UnsupportedFormatError struct {
	Extension string
}

func (e *UnsupportedFormatError) Error() string {
	return fmt.Sprintf("unsupported resource file extension %q; use .json, .yaml, or .yml", e.Extension)
}

// UnknownKindError reports a manifest kind that is neither a Resource kind nor
// a <Kind>Resource alias.
type UnknownKindError struct {
	Kind string
}

func (e *UnknownKindError) Error() string {
	return fmt.Sprintf("unknown resource kind %q", e.Kind)
}

// MissingEnvError reports an environment reference without a value or default.
type MissingEnvError struct {
	Name string
}

func (e *MissingEnvError) Error() string {
	return fmt.Sprintf("environment variable %s is required", e.Name)
}

func normalizeKind(data []byte) ([]byte, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	rawKind, ok := envelope["kind"]
	if !ok {
		return data, nil
	}
	var kind string
	if err := json.Unmarshal(rawKind, &kind); err != nil {
		if _, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
			return data, nil
		}
		return nil, err
	}

	if apitypes.ResourceKind(kind).Valid() {
		return data, nil
	}
	if alias, ok := strings.CutSuffix(kind, "Resource"); ok && apitypes.ResourceKind(alias).Valid() {
		normalized, err := json.Marshal(alias)
		if err != nil {
			return nil, err
		}
		envelope["kind"] = normalized
		return json.Marshal(envelope)
	}
	return nil, &UnknownKindError{Kind: kind}
}

func expandYAMLValue(value any) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			expanded, err := expandYAMLValue(item)
			if err != nil {
				return nil, err
			}
			out[key] = expanded
		}
		return out, nil
	case map[any]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("resource YAML map key must be a string, got %T", key)
			}
			expanded, err := expandYAMLValue(item)
			if err != nil {
				return nil, err
			}
			out[name] = expanded
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			expanded, err := expandYAMLValue(item)
			if err != nil {
				return nil, err
			}
			out[i] = expanded
		}
		return out, nil
	case string:
		return expandEnvString(v)
	default:
		return value, nil
	}
}

var envPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-([^}]*))?\}`)

func expandJSONEnv(data []byte) ([]byte, error) {
	input := string(data)
	expanded, err := expandEnvWith(input, func(replacement string, offset int) string {
		if insideJSONString(input, offset) {
			return escapeJSONStringFragment(replacement)
		}
		return replacement
	})
	if err != nil {
		return nil, err
	}
	return []byte(expanded), nil
}

// HasEnvReference reports whether input contains a ${NAME} or
// ${NAME:-default} reference.
func HasEnvReference(input string) bool {
	return envPattern.MatchString(input)
}

// ExpandEnvString expands ${NAME} and ${NAME:-default} references in one
// string with the rules PrepareManifest applies to string values: an unset or
// empty variable uses its default, and fails with *MissingEnvError when there
// is none.
func ExpandEnvString(input string) (string, error) {
	return expandEnvString(input)
}

func expandEnvString(input string) (string, error) {
	return expandEnvWith(input, func(replacement string, _ int) string {
		return replacement
	})
}

func expandEnvWith(input string, formatReplacement func(string, int) string) (string, error) {
	matches := envPattern.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 {
		return input, nil
	}
	var expanded strings.Builder
	last := 0
	for _, match := range matches {
		expanded.WriteString(input[last:match[0]])
		name := input[match[2]:match[3]]
		replacement := ""
		if value, ok := os.LookupEnv(name); ok && value != "" {
			replacement = value
		} else if match[4] != -1 {
			replacement = input[match[6]:match[7]]
		} else {
			return "", &MissingEnvError{Name: name}
		}
		expanded.WriteString(formatReplacement(replacement, match[0]))
		last = match[1]
	}
	expanded.WriteString(input[last:])
	return expanded.String(), nil
}

func insideJSONString(input string, offset int) bool {
	inString := false
	escaped := false
	for i := range offset {
		switch input[i] {
		case '\\':
			if escaped {
				escaped = false
			} else {
				escaped = true
			}
		case '"':
			if !escaped {
				inString = !inString
			}
			escaped = false
		default:
			escaped = false
		}
	}
	return inString
}

func escapeJSONStringFragment(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	quoted := string(data)
	return quoted[1 : len(quoted)-1]
}
