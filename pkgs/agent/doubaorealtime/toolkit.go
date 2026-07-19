package doubaorealtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/GizClaw/doubao-speech-go"
	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
)

func providerTools(tools []commonagent.Tool) ([]doubaospeech.RealtimeDuplexFunctionTool, error) {
	out := make([]doubaospeech.RealtimeDuplexFunctionTool, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			return nil, fmt.Errorf("agent/doubaorealtime: tool %q has no provider name", tool.ID)
		}
		parameters, err := providerSchema(tool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("agent/doubaorealtime: tool %q schema: %w", name, err)
		}
		strict := true
		out = append(out, doubaospeech.RealtimeDuplexFunctionTool{
			Type:        "function",
			Name:        name,
			Description: tool.Description,
			Parameters:  parameters,
			Strict:      &strict,
		})
	}
	return out, nil
}

func providerSchema(schema any) (*doubaospeech.RealtimeDuplexJSONSchema, error) {
	if schema == nil {
		return nil, nil
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var out doubaospeech.RealtimeDuplexJSONSchema
	if err := decoder.Decode(&out); err != nil {
		return nil, fmt.Errorf("unsupported JSON Schema: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("invalid JSON Schema document")
	}
	return &out, nil
}
