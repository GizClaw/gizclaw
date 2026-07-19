package agenthost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
)

// ToolkitContext is the resolved ToolKit runtime available to an agent factory.
type ToolkitContext struct {
	Builder      *toolkit.Builder
	Executors    *toolkit.ExecutorRegistry
	BuildRequest toolkit.BuildRequest
}

func (c *ToolkitContext) BuildToolkit(ctx context.Context) (toolkit.ToolKit, error) {
	if c == nil {
		return toolkit.ToolKit{}, nil
	}
	if c.Builder == nil {
		return toolkit.ToolKit{}, fmt.Errorf("%w: builder is required", toolkit.ErrNotConfigured)
	}
	return c.Builder.Build(ctx, c.requestForContext(ctx))
}

func (c *ToolkitContext) Invoke(ctx context.Context, callID, name string, args json.RawMessage) (toolkit.Result, error) {
	if c == nil || c.Builder == nil || c.Executors == nil {
		return toolkit.Result{}, toolkit.ErrNotConfigured
	}
	return c.Builder.Invoke(ctx, c.Executors, toolkit.InvokeRequest{
		Build:  c.requestForContext(ctx),
		CallID: callID,
		Name:   name,
		Args:   args,
	})
}

// BuildAgentToolkit resolves the product ToolKit into the provider-neutral,
// executable view passed to a common Agent constructor.
func (c *ToolkitContext) BuildAgentToolkit(ctx context.Context) (commonagent.Toolkit, error) {
	resolved, err := c.BuildToolkit(ctx)
	if err != nil {
		return nil, err
	}
	tools := make([]commonagent.Tool, 0, len(resolved.Tools))
	for _, tool := range resolved.Tools {
		name := tool.ID
		if tool.Name != nil && strings.TrimSpace(*tool.Name) != "" {
			name = strings.TrimSpace(*tool.Name)
		}
		description := ""
		if tool.Description != nil {
			description = strings.TrimSpace(*tool.Description)
		}
		tools = append(tools, commonagent.Tool{
			ID:          tool.ID,
			Name:        name,
			Description: description,
			InputSchema: tool.InputSchema.CloneSchemas(),
		})
	}
	return agentToolkit{context: c, tools: tools}, nil
}

type agentToolkit struct {
	context *ToolkitContext
	tools   []commonagent.Tool
}

func (t agentToolkit) Tools() []commonagent.Tool {
	out := make([]commonagent.Tool, len(t.tools))
	for i := range t.tools {
		out[i] = t.tools[i]
		if t.tools[i].InputSchema != nil {
			out[i].InputSchema = t.tools[i].InputSchema.CloneSchemas()
		}
	}
	return out
}

func (t agentToolkit) Invoke(ctx context.Context, call commonagent.ToolCall) (commonagent.ToolResult, error) {
	declared := slices.ContainsFunc(t.tools, func(tool commonagent.Tool) bool { return tool.Name == call.Name })
	if !declared {
		return commonagent.ErrorToolResult(call.ID, "tool_not_found", fmt.Sprintf("tool %q was not declared by this Toolkit", call.Name)), nil
	}
	result, err := t.context.Invoke(ctx, call.ID, call.Name, slices.Clone(call.Arguments))
	if err != nil {
		if cause := ctx.Err(); cause != nil {
			return commonagent.ToolResult{}, cause
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return commonagent.ToolResult{}, err
		}
		return commonagent.ErrorToolResult(call.ID, agentToolErrorCode(err), err.Error()), nil
	}
	if len(result.Data) == 0 || !json.Valid(result.Data) {
		return commonagent.ToolResult{}, fmt.Errorf("agenthost: Tool %q returned invalid JSON", call.Name)
	}
	return commonagent.ToolResult{ID: call.ID, Content: slices.Clone(result.Data)}, nil
}

func agentToolErrorCode(err error) string {
	switch {
	case errors.Is(err, toolkit.ErrToolNotFound):
		return "tool_not_found"
	case errors.Is(err, toolkit.ErrInvalidTool):
		return "invalid_arguments"
	case errors.Is(err, toolkit.ErrExecutorNotFound):
		return "executor_not_found"
	default:
		return "tool_error"
	}
}

func (c *ToolkitContext) requestForContext(ctx context.Context) toolkit.BuildRequest {
	req := c.BuildRequest
	if subject, ok := aclSubjectFromContext(ctx); ok {
		req.Subject = subject
	}
	return req
}
