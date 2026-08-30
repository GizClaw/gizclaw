package flowcraft

import (
	"context"
	"encoding/json"

	flowagent "github.com/GizClaw/flowcraft/core/agent"
	flowgraph "github.com/GizClaw/flowcraft/core/graph"
)

func testNodeConfig(source map[string]any) json.RawMessage {
	data, err := json.Marshal(source)
	if err != nil {
		panic(err)
	}
	return data
}

func testInferenceConfig(model string) json.RawMessage {
	return testNodeConfig(map[string]any{
		"model":            map[string]any{"id": map[string]any{"provider": genXInferenceProvider, "name": model}},
		"messages_channel": "inference." + model,
		"stream":           true,
	})
}

func testExecutionContext(ctx context.Context, runID string) flowgraph.ExecutionContext {
	return flowgraph.ExecutionContext{Context: flowagent.WithRunInfo(ctx, flowagent.RunInfo{Identity: flowagent.Identity{RunID: runID}})}
}
