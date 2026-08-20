package giztest

import (
	"fmt"
	"io"
)

func emitOutput(w io.Writer, vars *variables, name string) (map[string]any, error) {
	item, ok := vars.values[name]
	if !ok || item.data == nil {
		return nil, fmt.Errorf("output variable %q unavailable", name)
	}
	if item.spec.Secret {
		return nil, fmt.Errorf("secret variable %q cannot be emitted", name)
	}
	if item.spec.Type == "audio" || item.spec.Type == "binary" || item.spec.Type == "object" {
		return nil, fmt.Errorf("variable %q type %s cannot be emitted", name, item.spec.Type)
	}
	fmt.Fprintf(w, "%s=%v\n", name, item.data)
	return map[string]any{"variable": name, "type": item.spec.Type}, nil
}
