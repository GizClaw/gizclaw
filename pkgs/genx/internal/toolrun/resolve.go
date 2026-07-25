package toolrun

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/google/jsonschema-go/jsonschema"
)

// ResolveTools validates and clones declarations from one ToolInvoker. The
// returned definitions may be mutated by provider adapters.
func ResolveTools(ctx context.Context, invoker genx.ToolInvoker) ([]genx.ToolDefinition, error) {
	if invoker == nil {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("genx: resolve tools: %w", err)
	}
	definitions, err := invoker.ResolveTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("genx: resolve tools: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("genx: resolve tools: %w", err)
	}
	result := make([]genx.ToolDefinition, 0, len(definitions))
	seen := make(map[string]struct{}, len(definitions))
	for index, definition := range definitions {
		name := strings.TrimSpace(definition.Name)
		if name == "" {
			return nil, fmt.Errorf("genx: resolved tool %d has no name", index)
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("genx: resolved duplicate tool name %q", name)
		}
		if definition.Argument == nil {
			return nil, fmt.Errorf("genx: resolved tool %q has no argument schema", name)
		}
		schemaJSON, err := json.Marshal(definition.Argument)
		if err != nil {
			return nil, fmt.Errorf("genx: encode resolved tool %q schema: %w", name, err)
		}
		var schemaClone jsonschema.Schema
		if err := json.Unmarshal(schemaJSON, &schemaClone); err != nil {
			return nil, fmt.Errorf("genx: clone resolved tool %q schema: %w", name, err)
		}
		if _, err := schemaClone.Resolve(nil); err != nil {
			return nil, fmt.Errorf("genx: resolve tool %q schema: %w", name, err)
		}
		seen[name] = struct{}{}
		result = append(result, genx.ToolDefinition{
			Name:        name,
			Description: strings.TrimSpace(definition.Description),
			Argument:    &schemaClone,
		})
	}
	return result, nil
}
