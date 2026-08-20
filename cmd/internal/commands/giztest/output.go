package giztest

import (
	"fmt"
	"io"
	"unicode/utf8"
)

const maxOutputTextBytes = 4096

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
	text := fmt.Sprint(item.data)
	truncated := len(text) > maxOutputTextBytes
	if truncated {
		text = text[:maxOutputTextBytes]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
	}
	fmt.Fprintf(w, "%s=%s\n", name, text)
	return map[string]any{"variable": name, "type": item.spec.Type, "bytes": len(text), "truncated": truncated}, nil
}
