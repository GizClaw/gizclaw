package eino

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/toolrun"
	"github.com/cloudwego/eino/schema"
	einojsonschema "github.com/eino-contrib/jsonschema"
)

func einoToolInfos(ctx context.Context, invoker genx.ToolInvoker) ([]*schema.ToolInfo, error) {
	if invoker == nil {
		return nil, nil
	}
	definitions, err := toolrun.ResolveTools(ctx, invoker)
	if err != nil {
		return nil, fmt.Errorf("eino: %w", err)
	}
	result := make([]*schema.ToolInfo, 0, len(definitions))
	for _, definition := range definitions {
		encoded, err := json.Marshal(definition.Argument)
		if err != nil {
			return nil, fmt.Errorf("eino: encode tool %q schema: %w", definition.Name, err)
		}
		var params einojsonschema.Schema
		if err := json.Unmarshal(encoded, &params); err != nil {
			return nil, fmt.Errorf("eino: convert tool %q schema: %w", definition.Name, err)
		}
		result = append(result, &schema.ToolInfo{
			Name:        definition.Name,
			Desc:        definition.Description,
			ParamsOneOf: schema.NewParamsOneOfByJSONSchema(&params),
		})
	}
	return result, nil
}
