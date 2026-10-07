package giztest

import (
	"math"
	"testing"
)

func TestHTTPQueryPreservesNumericCheckpoint(t *testing.T) {
	vars, err := NewVariables(map[string]VariableSpec{"checkpoint": {Direction: "input", Type: "number", Value: float64(1791331200000)}})
	if err != nil {
		t.Fatal(err)
	}
	path, err := ResolveHTTPQuery("/sync?timestamp=0&keep=yes", map[string]any{"timestamp": "${checkpoint}", "enabled": true, "label": "a b&c"}, vars)
	if err != nil || path != "/sync?enabled=true&keep=yes&label=a+b%26c&timestamp=1791331200000" {
		t.Fatalf("query = %q, %v", path, err)
	}
}

func TestHTTPQueryRejectsNonScalarsAndNonfiniteNumbers(t *testing.T) {
	vars, err := NewVariables(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{nil, []any{1}, map[string]any{}, math.NaN(), math.Inf(1)} {
		if _, err := ResolveHTTPQuery("/sync", map[string]any{"value": value}, vars); err == nil {
			t.Fatalf("query accepted %T", value)
		}
	}
}
