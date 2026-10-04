package graphstate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
)

// Integral floats need a type hint because encoding/json writes 7.0 as 7.
// Integers are decoded losslessly, including checkpoints without these hints.
func jsonStateFloatPaths(value any) []string {
	var paths []string
	var visit func(any, string)
	visit = func(value any, path string) {
		switch value := value.(type) {
		case float64:
			if math.Trunc(value) == value {
				paths = append(paths, path)
			}
		case map[string]any:
			for key, child := range value {
				visit(child, path+"/"+jsonStatePointerKey(key))
			}
		case []any:
			for index, child := range value {
				visit(child, fmt.Sprintf("%s/%d", path, index))
			}
		}
	}
	visit(value, "")
	slices.Sort(paths)
	return paths
}

func jsonStatePointerKey(key string) string {
	return strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}

func decodeJSONState(data []byte, floatPaths []string) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	floats := make(map[string]bool, len(floatPaths))
	for _, path := range floatPaths {
		floats[path] = true
	}
	var restore func(any, string) (any, error)
	restore = func(value any, path string) (any, error) {
		switch value := value.(type) {
		case json.Number:
			if !floats[path] && !strings.ContainsAny(string(value), ".eE") {
				return value.Int64()
			}
			return value.Float64()
		case map[string]any:
			for key, child := range value {
				restored, err := restore(child, path+"/"+jsonStatePointerKey(key))
				if err != nil {
					return nil, err
				}
				value[key] = restored
			}
		case []any:
			for index, child := range value {
				restored, err := restore(child, fmt.Sprintf("%s/%d", path, index))
				if err != nil {
					return nil, err
				}
				value[index] = restored
			}
		}
		return value, nil
	}
	return restore(value, "")
}
