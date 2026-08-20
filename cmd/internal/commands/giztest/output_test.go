package giztest

import (
	"bytes"
	"strings"
	"testing"
)

func TestEmitOutputRejectsSecretAndMedia(t *testing.T) {
	for name, item := range map[string]value{
		"secret": {data: "hidden", spec: VariableSpec{Type: "string", Secret: true}},
		"audio":  {data: []byte{1}, spec: VariableSpec{Type: "audio"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := emitOutput(&bytes.Buffer{}, &variables{values: map[string]value{name: item}}, name); err == nil {
				t.Fatal("unsafe output accepted")
			}
		})
	}
}

func TestEmitOutputWritesOnlyDeclaredScalar(t *testing.T) {
	var output bytes.Buffer
	vars := &variables{values: map[string]value{"status": {data: "ready", spec: VariableSpec{Type: "string"}}}}
	if _, err := emitOutput(&output, vars, "status"); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(output.String()) != "status=ready" {
		t.Fatalf("output = %q", output.String())
	}
}
