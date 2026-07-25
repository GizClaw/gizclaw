// Package toolrun owns invocation-local ToolCall identity and limits for
// GenX Transformers.
package toolrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

var (
	// ErrInvalidCall reports malformed provider ToolCall control data.
	ErrInvalidCall = errors.New("genx: invalid ToolCall")
	// ErrInvalidResult reports non-JSON output returned by a ToolInvoker.
	ErrInvalidResult = errors.New("genx: invalid tool result")
	// ErrDuplicateCallID reports a repeated ID within one Transformer
	// invocation.
	ErrDuplicateCallID = errors.New("genx: duplicate ToolCall ID")
	// ErrCallLimit reports exhaustion of the per-invocation ToolCall budget.
	ErrCallLimit = errors.New("genx: ToolCall limit exceeded")
)

type contextKey struct{}

// State tracks ToolCalls within one Transformer invocation. It never holds its
// mutex while executing caller code.
type State struct {
	invoker genx.ToolInvoker
	max     int

	mu    sync.Mutex
	count int
	seen  map[string]struct{}
}

// New creates invocation-local state. A zero maximum uses
// genx.DefaultMaxToolCalls.
func New(invoker genx.ToolInvoker, maximum int) *State {
	if invoker == nil {
		return nil
	}
	if maximum == 0 {
		maximum = genx.DefaultMaxToolCalls
	}
	return &State{
		invoker: invoker,
		max:     maximum,
		seen:    make(map[string]struct{}),
	}
}

// Invoke reserves the call ID and budget before invoking the shared
// ToolInvoker. The provider call ID never crosses the ToolInvoker boundary.
func (s *State) Invoke(ctx context.Context, call genx.ToolCall) (genx.ToolResult, error) {
	if s == nil || s.invoker == nil {
		return genx.ToolResult{}, fmt.Errorf("%w: ToolInvoker is not configured", ErrInvalidCall)
	}
	call.ID = strings.TrimSpace(call.ID)
	if call.ID == "" {
		return genx.ToolResult{}, fmt.Errorf("%w: call ID is required", ErrInvalidCall)
	}
	if call.FuncCall == nil {
		return genx.ToolResult{}, fmt.Errorf("%w: call %q has no function", ErrInvalidCall, call.ID)
	}
	name := strings.TrimSpace(call.FuncCall.Name)
	if name == "" {
		return genx.ToolResult{}, fmt.Errorf("%w: call %q function name is required", ErrInvalidCall, call.ID)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return genx.ToolResult{}, fmt.Errorf("genx: invoke tool %q for call %q: %w", name, call.ID, err)
	}
	s.mu.Lock()
	if _, duplicate := s.seen[call.ID]; duplicate {
		s.mu.Unlock()
		return genx.ToolResult{}, fmt.Errorf("%w: %q", ErrDuplicateCallID, call.ID)
	}
	if s.count >= s.max {
		s.mu.Unlock()
		return genx.ToolResult{}, fmt.Errorf("%w: maximum %d", ErrCallLimit, s.max)
	}
	s.seen[call.ID] = struct{}{}
	s.count++
	s.mu.Unlock()
	result, err := s.invoker.InvokeTool(ctx, name, json.RawMessage(call.FuncCall.Arguments))
	if err != nil {
		return genx.ToolResult{}, fmt.Errorf("genx: invoke tool %q for call %q: %w", name, call.ID, err)
	}
	if err := ctx.Err(); err != nil {
		return genx.ToolResult{}, fmt.Errorf(
			"genx: discard late tool %q result for call %q: %w",
			name,
			call.ID,
			err,
		)
	}
	if len(result) == 0 || !json.Valid(result) {
		return genx.ToolResult{}, fmt.Errorf(
			"%w: tool %q returned invalid JSON for call %q",
			ErrInvalidResult,
			name,
			call.ID,
		)
	}
	return genx.ToolResult{ID: call.ID, Result: string(result)}, nil
}

// WithContext stores state unless the context already carries a state. Nested
// Graphs therefore share their root invocation's call budget and seen IDs.
func WithContext(ctx context.Context, state *State) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if FromContext(ctx) != nil || state == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, state)
}

// FromContext returns invocation-local ToolCall state.
func FromContext(ctx context.Context) *State {
	if ctx == nil {
		return nil
	}
	state, _ := ctx.Value(contextKey{}).(*State)
	return state
}
