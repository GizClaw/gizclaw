package giztest

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strconv"
)

// ResolveHTTPQuery adds scalar query values to a request path. An exact variable
// reference retains its number/bool type until query serialization. Query keys
// replace the same keys already present in path; other existing keys are retained.
func ResolveHTTPQuery(path string, query map[string]any, vars *Variables) (string, error) {
	if len(query) == 0 {
		return path, nil
	}
	parsed, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	values := parsed.Query()
	for name, raw := range query {
		value, err := vars.Resolve(raw)
		if err != nil {
			return "", err
		}
		var encoded string
		switch value := value.(type) {
		case string:
			encoded = value
		case bool:
			encoded = strconv.FormatBool(value)
		case int:
			encoded = strconv.Itoa(value)
		case int64:
			encoded = strconv.FormatInt(value, 10)
		case float64:
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return "", fmt.Errorf("HTTP query %s must be finite", name)
			}
			encoded = strconv.FormatFloat(value, 'f', -1, 64)
		case json.Number:
			encoded = value.String()
		default:
			return "", fmt.Errorf("HTTP query %s must be a string, number, or boolean", name)
		}
		values.Set(name, encoded)
	}
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}
