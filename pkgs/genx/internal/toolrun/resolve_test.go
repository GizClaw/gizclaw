package toolrun

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/google/jsonschema-go/jsonschema"
)

type definitionInvoker struct {
	definitions []genx.ToolDefinition
	err         error
}

func (invoker *definitionInvoker) ResolveTools(context.Context) ([]genx.ToolDefinition, error) {
	return invoker.definitions, invoker.err
}

func (*definitionInvoker) InvokeTool(
	context.Context,
	string,
	json.RawMessage,
) (json.RawMessage, error) {
	return nil, errors.New("InvokeTool must not be used")
}

func TestResolveToolsValidatesAndClonesDefinitions(t *testing.T) {
	schema := &jsonschema.Schema{
		Type:       "object",
		Properties: map[string]*jsonschema.Schema{"value": {Type: "string"}},
	}
	invoker := &definitionInvoker{definitions: []genx.ToolDefinition{{
		Name: " lookup ", Description: " description ", Argument: schema,
	}}}
	first, err := ResolveTools(t.Context(), invoker)
	if err != nil {
		t.Fatalf("ResolveTools() error = %v", err)
	}
	if len(first) != 1 || first[0].Name != "lookup" ||
		first[0].Description != "description" {
		t.Fatalf("definitions = %#v", first)
	}
	first[0].Name = "mutated"
	first[0].Argument.Properties["value"].Type = "integer"
	second, err := ResolveTools(t.Context(), invoker)
	if err != nil {
		t.Fatalf("second ResolveTools() error = %v", err)
	}
	if second[0].Name != "lookup" ||
		second[0].Argument.Properties["value"].Type != "string" ||
		schema.Properties["value"].Type != "string" {
		t.Fatalf("definition mutation leaked: second=%#v source=%#v", second[0], schema)
	}
}

func TestResolveToolsAcceptsNilInputs(t *testing.T) {
	definitions, err := ResolveTools(nil, nil)
	if err != nil || definitions != nil {
		t.Fatalf("ResolveTools(nil, nil) = %#v, %v", definitions, err)
	}
	definitions, err = ResolveTools(nil, &definitionInvoker{})
	if err != nil || len(definitions) != 0 {
		t.Fatalf("ResolveTools(nil, empty) = %#v, %v", definitions, err)
	}
}

func TestResolveToolsRejectsInvalidDefinitions(t *testing.T) {
	schema := &jsonschema.Schema{Type: "object"}
	tests := []struct {
		name        string
		definitions []genx.ToolDefinition
		want        string
	}{
		{name: "blank name", definitions: []genx.ToolDefinition{{Argument: schema}}, want: "has no name"},
		{name: "nil schema", definitions: []genx.ToolDefinition{{Name: "lookup"}}, want: "has no argument schema"},
		{
			name: "duplicate normalized name",
			definitions: []genx.ToolDefinition{
				{Name: "lookup", Argument: schema},
				{Name: " lookup ", Argument: schema},
			},
			want: "duplicate tool name",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ResolveTools(t.Context(), &definitionInvoker{definitions: test.definitions})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ResolveTools() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestResolveToolsPropagatesErrorAndCancellation(t *testing.T) {
	resolveErr := errors.New("resolve failed")
	if _, err := ResolveTools(
		t.Context(),
		&definitionInvoker{err: resolveErr},
	); !errors.Is(err, resolveErr) {
		t.Fatalf("ResolveTools() error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := ResolveTools(ctx, &definitionInvoker{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveTools(canceled) error = %v", err)
	}
}
