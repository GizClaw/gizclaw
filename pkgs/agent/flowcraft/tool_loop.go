package flowcraft

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	flowmodel "github.com/GizClaw/flowcraft/sdk/model"
	flowtool "github.com/GizClaw/flowcraft/sdk/tool"
	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
)

type toolSequencer struct {
	mu     sync.Mutex
	queue  []string
	seen   map[string]struct{}
	notify chan struct{}
}

func newToolSequencer() *toolSequencer {
	return &toolSequencer{seen: make(map[string]struct{}), notify: make(chan struct{})}
}

func (s *toolSequencer) record(id string) error {
	if s == nil || id == "" {
		return fmt.Errorf("agent/flowcraft: ToolCall ID is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.seen[id]; exists {
		return fmt.Errorf("%w: duplicate call ID %q", commonagent.ErrInvalidToolCall, id)
	}
	s.seen[id] = struct{}{}
	s.queue = append(s.queue, id)
	s.signalLocked()
	return nil
}

func (s *toolSequencer) acquire(ctx context.Context, id string) (func(), error) {
	for {
		s.mu.Lock()
		if len(s.queue) > 0 && s.queue[0] == id {
			s.mu.Unlock()
			return func() {
				s.mu.Lock()
				if len(s.queue) > 0 && s.queue[0] == id {
					s.queue = s.queue[1:]
					s.signalLocked()
				}
				s.mu.Unlock()
			}, nil
		}
		notify := s.notify
		s.mu.Unlock()
		select {
		case <-notify:
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		}
	}
}

func (s *toolSequencer) signalLocked() {
	close(s.notify)
	s.notify = make(chan struct{})
}

type turnStateKey struct{}

type turnState struct {
	maximum int64
	used    atomic.Int64
	abort   context.CancelCauseFunc
}

func withTurnState(ctx context.Context, maximum int, abort context.CancelCauseFunc) context.Context {
	return context.WithValue(ctx, turnStateKey{}, &turnState{maximum: int64(maximum), abort: abort})
}

func buildToolRegistry(toolkit commonagent.Toolkit, timeout time.Duration, h *conversationHistory) (*flowtool.Registry, []string, error) {
	registry := flowtool.NewRegistry()
	declarations := toolkit.Tools()
	names := make([]string, 0, len(declarations))
	seen := make(map[string]struct{}, len(declarations))
	for _, declaration := range declarations {
		if declaration.Name == "" {
			return nil, nil, fmt.Errorf("agent/flowcraft: tool name is required")
		}
		if _, exists := seen[declaration.Name]; exists {
			return nil, nil, fmt.Errorf("agent/flowcraft: duplicate tool name %q", declaration.Name)
		}
		seen[declaration.Name] = struct{}{}
		schema := map[string]any{}
		if declaration.InputSchema != nil {
			data, err := json.Marshal(declaration.InputSchema)
			if err != nil {
				return nil, nil, fmt.Errorf("agent/flowcraft: encode schema for %q: %w", declaration.Name, err)
			}
			if err := json.Unmarshal(data, &schema); err != nil {
				return nil, nil, fmt.Errorf("agent/flowcraft: decode schema for %q: %w", declaration.Name, err)
			}
		}
		registry.Register(flowtool.FuncTool(flowmodel.ToolDefinition{
			Name: declaration.Name, Description: declaration.Description, InputSchema: schema,
		}, func(context.Context, string) (string, error) {
			return "", fmt.Errorf("agent/flowcraft: toolkit middleware was bypassed")
		}))
		names = append(names, declaration.Name)
	}
	registry.Use(func(flowtool.Dispatch) flowtool.Dispatch {
		return func(ctx context.Context, call flowmodel.ToolCall) flowmodel.ToolResult {
			sequencer, _ := ctx.Value(sequencerKey{}).(*toolSequencer)
			if sequencer == nil {
				return flowmodel.ToolResult{ToolCallID: call.ID, Content: `{"error":{"code":"protocol","message":"missing tool sequencer"}}`, IsError: true}
			}
			release, err := sequencer.acquire(ctx, call.ID)
			if err != nil {
				return flowmodel.ToolResult{ToolCallID: call.ID, Content: string(commonagent.ErrorToolResult(call.ID, "interrupted", err.Error()).Content), IsError: true}
			}
			defer release()
			state, _ := ctx.Value(turnStateKey{}).(*turnState)
			if state != nil && state.maximum > 0 && state.used.Add(1) > state.maximum {
				err = fmt.Errorf("%w: maximum %d", commonagent.ErrToolCallLimit, state.maximum)
				state.abort(err)
				return flowmodel.ToolResult{ToolCallID: call.ID, Content: string(commonagent.ErrorToolResult(call.ID, "tool_call_limit", err.Error()).Content), IsError: true}
			}
			results, invokeErr := commonagent.InvokeToolCalls(ctx, toolkit, []commonagent.ToolCall{{
				ID: call.ID, Name: call.Name, Arguments: json.RawMessage(call.Arguments),
			}}, commonagent.ToolLoopConfig{MaxCalls: 1, Timeout: timeout})
			if invokeErr != nil {
				interrupted := context.Cause(ctx) != nil
				if state != nil {
					state.abort(invokeErr)
				}
				failure := commonagent.ErrorToolResult(call.ID, "invoke_failed", invokeErr.Error())
				flowFailure := flowmodel.ToolResult{ToolCallID: call.ID, Content: string(failure.Content), IsError: true}
				if !interrupted {
					if err := h.append(context.WithoutCancel(ctx), []flowmodel.Message{
						flowmodel.NewToolCallMessage([]flowmodel.ToolCall{call}),
						flowmodel.NewToolResultMessage([]flowmodel.ToolResult{flowFailure}),
					}, false); err != nil {
						return flowmodel.ToolResult{ToolCallID: call.ID, Content: string(commonagent.ErrorToolResult(call.ID, "history_failed", err.Error()).Content), IsError: true}
					}
				}
				return flowFailure
			}
			result := results[0]
			flowResult := flowmodel.ToolResult{ToolCallID: result.ID, Content: string(result.Content), IsError: result.IsError}
			if err := h.append(ctx, []flowmodel.Message{
				flowmodel.NewToolCallMessage([]flowmodel.ToolCall{call}),
				flowmodel.NewToolResultMessage([]flowmodel.ToolResult{flowResult}),
			}, false); err != nil {
				if state != nil {
					state.abort(err)
				}
				failure := commonagent.ErrorToolResult(call.ID, "history_failed", err.Error())
				return flowmodel.ToolResult{ToolCallID: call.ID, Content: string(failure.Content), IsError: true}
			}
			return flowResult
		}
	})
	return registry, slices.Clone(names), nil
}

type sequencerKey struct{}

func withSequencer(ctx context.Context, sequencer *toolSequencer) context.Context {
	return context.WithValue(ctx, sequencerKey{}, sequencer)
}
