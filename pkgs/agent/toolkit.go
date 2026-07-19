package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
)

var (
	// ErrInvalidToolCall indicates that a provider call cannot be matched with
	// a valid result and therefore cannot safely resume the model turn.
	ErrInvalidToolCall = errors.New("agent: invalid tool call")
	// ErrToolCallLimit indicates that a turn exceeded its configured call limit.
	ErrToolCallLimit = errors.New("agent: tool call limit exceeded")
)

// Tool is the provider-neutral declaration exposed to an Agent model.
type Tool struct {
	ID          string
	Name        string
	Description string
	InputSchema *jsonschema.Schema
}

// ToolCall is one provider-requested invocation. ID is the provider call ID and
// must be preserved in the matching ToolResult.
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// ToolResult is the result returned to the model for one ToolCall. Business
// failures use IsError with valid JSON Content; protocol and lifecycle failures
// are returned as Go errors from Toolkit.Invoke.
type ToolResult struct {
	ID      string
	Content json.RawMessage
	IsError bool
}

// Toolkit is the immutable, executable tool view supplied when an Agent is
// constructed. Implementations must defensively own values returned by Tools.
type Toolkit interface {
	Tools() []Tool
	Invoke(context.Context, ToolCall) (ToolResult, error)
}

// ToolkitFunc adapts functions to Toolkit.
type ToolkitFunc struct {
	List       func() []Tool
	InvokeFunc func(context.Context, ToolCall) (ToolResult, error)
}

// EmptyToolkit returns an executable Toolkit with no declarations. It is used
// when an Agent supports tools but a particular runtime exposes none.
func EmptyToolkit() Toolkit {
	return ToolkitFunc{
		List: func() []Tool { return nil },
		InvokeFunc: func(context.Context, ToolCall) (ToolResult, error) {
			return ToolResult{}, fmt.Errorf("agent: no tools are configured")
		},
	}
}

func (t ToolkitFunc) Tools() []Tool {
	if t.List == nil {
		return nil
	}
	return cloneTools(t.List())
}

func (t ToolkitFunc) Invoke(ctx context.Context, call ToolCall) (ToolResult, error) {
	if t.InvokeFunc == nil {
		return ToolResult{}, fmt.Errorf("agent: toolkit invoke is not configured")
	}
	return t.InvokeFunc(ctx, cloneToolCall(call))
}

// ToolLoopConfig controls automatic invocation for one model step.
type ToolLoopConfig struct {
	MaxCalls int
	Timeout  time.Duration
}

// InvokeToolCalls executes calls strictly in slice order. It never retries or
// starts a later call before the prior call has returned a valid result.
func InvokeToolCalls(ctx context.Context, toolkit Toolkit, calls []ToolCall, cfg ToolLoopConfig) ([]ToolResult, error) {
	if toolkit == nil {
		return nil, fmt.Errorf("agent: toolkit is required")
	}
	if cfg.MaxCalls > 0 && len(calls) > cfg.MaxCalls {
		return nil, fmt.Errorf("%w: got %d, maximum %d", ErrToolCallLimit, len(calls), cfg.MaxCalls)
	}
	seen := make(map[string]struct{}, len(calls))
	for i, call := range calls {
		if err := validateToolCall(call); err != nil {
			return nil, fmt.Errorf("tool call %d: %w", i, err)
		}
		if _, duplicate := seen[call.ID]; duplicate {
			return nil, fmt.Errorf("tool call %d: %w: duplicate call ID %q", i, ErrInvalidToolCall, call.ID)
		}
		seen[call.ID] = struct{}{}
	}
	results := make([]ToolResult, 0, len(calls))
	for i, call := range calls {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("before tool call %d: %w", i, err)
		}
		callCtx := ctx
		cancel := func() {}
		if cfg.Timeout > 0 {
			callCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		}
		result, err := toolkit.Invoke(callCtx, call)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("invoke tool %q (%s): %w", call.Name, call.ID, err)
		}
		if err := validateToolResult(call, result); err != nil {
			return nil, fmt.Errorf("tool result %d: %w", i, err)
		}
		result.Content = slices.Clone(result.Content)
		results = append(results, result)
	}
	return results, nil
}

// ErrorToolResult creates a structured business-error result that can be sent
// back to a model without pretending the Tool succeeded.
func ErrorToolResult(callID, code, message string) ToolResult {
	content, err := json.Marshal(struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{Error: struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: strings.TrimSpace(code), Message: message}})
	if err != nil {
		panic(err)
	}
	return ToolResult{ID: callID, Content: content, IsError: true}
}

func validateToolCall(call ToolCall) error {
	if strings.TrimSpace(call.ID) == "" {
		return fmt.Errorf("%w: call ID is required", ErrInvalidToolCall)
	}
	if strings.TrimSpace(call.Name) == "" {
		return fmt.Errorf("%w: tool name is required", ErrInvalidToolCall)
	}
	if len(call.Arguments) == 0 || !json.Valid(call.Arguments) {
		return fmt.Errorf("%w: arguments must be valid JSON", ErrInvalidToolCall)
	}
	return nil
}

func validateToolResult(call ToolCall, result ToolResult) error {
	if result.ID != call.ID {
		return fmt.Errorf("%w: result ID %q does not match call ID %q", ErrInvalidToolCall, result.ID, call.ID)
	}
	if len(result.Content) == 0 || !json.Valid(result.Content) {
		return fmt.Errorf("%w: result content must be valid JSON", ErrInvalidToolCall)
	}
	return nil
}

func cloneTools(in []Tool) []Tool {
	out := make([]Tool, len(in))
	for i := range in {
		out[i] = in[i]
		if in[i].InputSchema != nil {
			out[i].InputSchema = in[i].InputSchema.CloneSchemas()
		}
	}
	return out
}

func cloneToolCall(call ToolCall) ToolCall {
	call.Arguments = slices.Clone(call.Arguments)
	return call
}
