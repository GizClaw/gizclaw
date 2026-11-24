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

func einoToolCatalog(ctx context.Context, invoker genx.ToolInvoker, noticeAlias string) ([]*schema.ToolInfo, string, error) {
	if invoker == nil {
		return nil, "", nil
	}
	definitions, err := toolrun.ResolveTools(ctx, invoker)
	if err != nil {
		return nil, "", fmt.Errorf("eino: %w", err)
	}
	result := make([]*schema.ToolInfo, 0, len(definitions))
	var noticeTool string
	for _, definition := range definitions {
		if noticeAlias != "" && definition.Alias == noticeAlias {
			noticeTool = definition.Name
		}
		encoded, err := json.Marshal(definition.Argument)
		if err != nil {
			return nil, "", fmt.Errorf("eino: encode tool %q schema: %w", definition.Name, err)
		}
		var params einojsonschema.Schema
		if err := json.Unmarshal(encoded, &params); err != nil {
			return nil, "", fmt.Errorf("eino: convert tool %q schema: %w", definition.Name, err)
		}
		result = append(result, &schema.ToolInfo{
			Name:        definition.Name,
			Desc:        definition.Description,
			ParamsOneOf: schema.NewParamsOneOfByJSONSchema(&params),
		})
	}
	return result, noticeTool, nil
}
