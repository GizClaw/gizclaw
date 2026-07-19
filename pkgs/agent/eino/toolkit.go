package eino

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	einojsonschema "github.com/eino-contrib/jsonschema"
)

type nativeTool struct {
	declaration commonagent.Tool
	toolkit     commonagent.Toolkit
	history     *conversationHistory
	timeout     time.Duration
}

func nativeTools(toolkit commonagent.Toolkit, h *conversationHistory, timeout time.Duration) ([]tool.BaseTool, error) {
	declarations := toolkit.Tools()
	tools := make([]tool.BaseTool, 0, len(declarations))
	names := make(map[string]struct{}, len(declarations))
	for _, declaration := range declarations {
		if declaration.Name == "" {
			return nil, fmt.Errorf("agent/eino: tool name is required")
		}
		if _, exists := names[declaration.Name]; exists {
			return nil, fmt.Errorf("agent/eino: duplicate tool name %q", declaration.Name)
		}
		names[declaration.Name] = struct{}{}
		tools = append(tools, &nativeTool{
			declaration: declaration,
			toolkit:     toolkit,
			history:     h,
			timeout:     timeout,
		})
	}
	return tools, nil
}

func (t *nativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	info := &schema.ToolInfo{Name: t.declaration.Name, Desc: t.declaration.Description}
	if t.declaration.InputSchema == nil {
		return info, nil
	}
	data, err := json.Marshal(t.declaration.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("agent/eino: encode schema for tool %q: %w", t.declaration.Name, err)
	}
	var converted einojsonschema.Schema
	if err := json.Unmarshal(data, &converted); err != nil {
		return nil, fmt.Errorf("agent/eino: convert schema for tool %q: %w", t.declaration.Name, err)
	}
	info.ParamsOneOf = schema.NewParamsOneOfByJSONSchema(&converted)
	return info, nil
}

func (t *nativeTool) InvokableRun(ctx context.Context, arguments string, _ ...tool.Option) (string, error) {
	return t.run(ctx, compose.GetToolCallID(ctx), arguments)
}

func (t *nativeTool) run(ctx context.Context, callID, arguments string) (string, error) {
	call := commonagent.ToolCall{
		ID:        callID,
		Name:      t.declaration.Name,
		Arguments: json.RawMessage(arguments),
	}
	if err := consumeToolBudget(ctx, callID); err != nil {
		return "", err
	}
	callMessage := schema.AssistantMessage("", []schema.ToolCall{{
		ID: callID,
		Function: schema.FunctionCall{
			Name:      call.Name,
			Arguments: arguments,
		},
	}})
	results, err := commonagent.InvokeToolCalls(ctx, t.toolkit, []commonagent.ToolCall{call}, commonagent.ToolLoopConfig{
		MaxCalls: 1,
		Timeout:  t.timeout,
	})
	if err != nil {
		if t.history != nil && context.Cause(ctx) == nil {
			failure := commonagent.ErrorToolResult(callID, "tool_invocation_failed", err.Error())
			if historyErr := t.history.appendToolExchange(context.WithoutCancel(ctx), callMessage, schema.ToolMessage(string(failure.Content), callID, schema.WithToolName(call.Name))); historyErr != nil {
				return "", errors.Join(err, fmt.Errorf("agent/eino: append failed tool result: %w", historyErr))
			}
		}
		return "", err
	}
	result := results[0]
	if t.history != nil {
		if err := t.history.appendToolExchange(ctx, callMessage, schema.ToolMessage(string(result.Content), callID, schema.WithToolName(call.Name))); err != nil {
			return "", err
		}
	}
	return string(result.Content), nil
}

type toolBudgetKey struct{}

type toolBudget struct {
	maximum int64
	used    atomic.Int64
	mu      sync.Mutex
	seen    map[string]struct{}
}

func withToolBudget(ctx context.Context, maximum int) context.Context {
	return context.WithValue(ctx, toolBudgetKey{}, &toolBudget{maximum: int64(maximum), seen: make(map[string]struct{})})
}

func consumeToolBudget(ctx context.Context, callID string) error {
	budget, _ := ctx.Value(toolBudgetKey{}).(*toolBudget)
	if budget == nil {
		return nil
	}
	budget.mu.Lock()
	if _, duplicate := budget.seen[callID]; duplicate {
		budget.mu.Unlock()
		return fmt.Errorf("%w: duplicate call ID %q", commonagent.ErrInvalidToolCall, callID)
	}
	budget.seen[callID] = struct{}{}
	budget.mu.Unlock()
	if budget.maximum <= 0 {
		return nil
	}
	if used := budget.used.Add(1); used > budget.maximum {
		return fmt.Errorf("%w: got %d, maximum %d", commonagent.ErrToolCallLimit, used, budget.maximum)
	}
	return nil
}

var _ tool.InvokableTool = (*nativeTool)(nil)
