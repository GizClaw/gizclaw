// Package dashscoperealtime implements the Tool-capable Qwen3.5 Realtime Agent.
package dashscoperealtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/GizClaw/dashscope-realtime-go"
	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers"
)

var _ commonagent.Agent = (*Agent)(nil)

// Agent owns DashScope dialogue turns and automatic Toolkit continuation.
type Agent struct {
	config Config
	tools  []dashscope.FunctionTool
}

// New constructs a Tool-capable DashScope Realtime Agent.
func New(config Config) (*Agent, error) {
	if config.Transformer == nil {
		return nil, fmt.Errorf("agent/dashscoperealtime: transformer is required")
	}
	config.Pattern = strings.TrimSpace(config.Pattern)
	if config.Pattern == "" {
		return nil, fmt.Errorf("agent/dashscoperealtime: pattern is required")
	}
	config.Model = strings.TrimSpace(config.Model)
	if config.Model != "" && !SupportsFunctionCalls(config.Model) {
		return nil, fmt.Errorf("agent/dashscoperealtime: model %q is not a supported Qwen3.5 realtime function-call model", config.Model)
	}
	if config.Toolkit == nil {
		return nil, fmt.Errorf("agent/dashscoperealtime: toolkit is required")
	}
	if config.MaxToolCalls < 0 {
		return nil, fmt.Errorf("agent/dashscoperealtime: max tool calls cannot be negative")
	}
	if config.ToolTimeout < 0 {
		return nil, fmt.Errorf("agent/dashscoperealtime: tool timeout cannot be negative")
	}
	tools, err := providerTools(config.Toolkit.Tools())
	if err != nil {
		return nil, err
	}
	return &Agent{config: config, tools: tools}, nil
}

// Transform starts one bidirectional Agent session using the configured model,
// Toolkit, and provider transformer.
func (a *Agent) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	if a == nil {
		return nil, fmt.Errorf("agent/dashscoperealtime: agent is nil")
	}
	runtime := transformers.DashScopeRealtimeCtxOptions{
		Model:        a.config.Model,
		Tools:        append([]dashscope.FunctionTool(nil), a.tools...),
		MaxToolCalls: a.config.MaxToolCalls,
		FunctionCallHandler: func(callCtx context.Context, calls []transformers.DashScopeRealtimeFunctionCall) ([]transformers.DashScopeRealtimeFunctionCallOutput, error) {
			return a.invoke(callCtx, calls)
		},
	}
	return a.config.Transformer.Transform(
		transformers.WithDashScopeRealtimeCtxOptions(ctx, runtime),
		a.config.Pattern,
		input,
	)
}

func (a *Agent) invoke(ctx context.Context, calls []transformers.DashScopeRealtimeFunctionCall) ([]transformers.DashScopeRealtimeFunctionCallOutput, error) {
	commonCalls := make([]commonagent.ToolCall, 0, len(calls))
	for _, call := range calls {
		commonCalls = append(commonCalls, commonagent.ToolCall{
			ID:        call.CallID,
			Name:      call.Name,
			Arguments: []byte(call.Arguments),
		})
	}
	results, err := commonagent.InvokeToolCalls(ctx, a.config.Toolkit, commonCalls, commonagent.ToolLoopConfig{
		MaxCalls: a.config.MaxToolCalls,
		Timeout:  a.config.ToolTimeout,
	})
	if err != nil {
		return nil, err
	}
	outputs := make([]transformers.DashScopeRealtimeFunctionCallOutput, 0, len(results))
	for _, result := range results {
		outputs = append(outputs, transformers.DashScopeRealtimeFunctionCallOutput{CallID: result.ID, Output: string(result.Content)})
	}
	return outputs, nil
}

// SupportsFunctionCalls reports whether model can drive the automatic Toolkit
// continuation contract owned by Agent.
func SupportsFunctionCalls(model string) bool {
	switch strings.TrimSpace(model) {
	case dashscope.ModelQwen35OmniPlusRealtime,
		dashscope.ModelQwen35OmniPlusRealtime20260315,
		dashscope.ModelQwen35OmniFlashRealtime,
		dashscope.ModelQwen35OmniFlashRealtime20260315:
		return true
	default:
		return false
	}
}
