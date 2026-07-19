package dashscoperealtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/GizClaw/dashscope-realtime-go"
	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
)

func providerTools(tools []commonagent.Tool) ([]dashscope.FunctionTool, error) {
	out := make([]dashscope.FunctionTool, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			return nil, fmt.Errorf("agent/dashscoperealtime: tool %q has no provider name", tool.ID)
		}
		parameters, err := providerSchema(tool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("agent/dashscoperealtime: tool %q schema: %w", name, err)
		}
		out = append(out, dashscope.FunctionTool{
			Type: dashscope.ToolTypeFunction,
			Function: dashscope.FunctionDefinition{
				Name:        name,
				Description: tool.Description,
				Parameters:  parameters,
			},
		})
	}
	return out, nil
}

func providerSchema(schema any) (*dashscope.JSONSchema, error) {
	if schema == nil {
		return nil, nil
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var out dashscope.JSONSchema
	if err := decoder.Decode(&out); err != nil {
		return nil, fmt.Errorf("unsupported JSON Schema: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("invalid JSON Schema document")
	}
	return &out, nil
}
