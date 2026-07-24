package eino

import (
	"encoding/json"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/cloudwego/eino/schema"
	einojsonschema "github.com/eino-contrib/jsonschema"
)

func einoToolInfos(toolkit *genx.Toolkit) ([]*schema.ToolInfo, error) {
	if toolkit == nil {
		return nil, nil
	}
	result := make([]*schema.ToolInfo, 0)
	for tool := range toolkit.Tools() {
		encoded, err := json.Marshal(tool.Argument)
		if err != nil {
			return nil, fmt.Errorf("eino: encode Toolkit tool %q schema: %w", tool.Name, err)
		}
		var params einojsonschema.Schema
		if err := json.Unmarshal(encoded, &params); err != nil {
			return nil, fmt.Errorf("eino: convert Toolkit tool %q schema: %w", tool.Name, err)
		}
		result = append(result, &schema.ToolInfo{
			Name:        tool.Name,
			Desc:        tool.Description,
			ParamsOneOf: schema.NewParamsOneOfByJSONSchema(&params),
		})
	}
	return result, nil
}
