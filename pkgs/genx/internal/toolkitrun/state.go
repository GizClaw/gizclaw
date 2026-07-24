// Package toolkitrun owns invocation-local ToolCall identity and limits for
// GenX Transformers.
package toolkitrun

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

var (
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
	toolkit *genx.Toolkit
	max     int

	mu    sync.Mutex
	count int
	seen  map[string]struct{}
}

// New creates invocation-local state. A zero maximum uses
// genx.DefaultMaxToolCalls.
func New(toolkit *genx.Toolkit, maximum int) *State {
	if toolkit == nil {
		return nil
	}
	if maximum == 0 {
		maximum = genx.DefaultMaxToolCalls
	}
	return &State{
		toolkit: toolkit,
		max:     maximum,
		seen:    make(map[string]struct{}),
	}
}

// Invoke reserves the call ID and budget before invoking the shared Toolkit.
func (s *State) Invoke(ctx context.Context, call genx.ToolCall) (genx.ToolResult, error) {
	if s == nil || s.toolkit == nil {
		return genx.ToolResult{}, fmt.Errorf("%w: Toolkit is not configured", genx.ErrInvalidToolkit)
	}
	call.ID = strings.TrimSpace(call.ID)
	if call.ID == "" {
		return genx.ToolResult{}, fmt.Errorf("%w: call ID is required", genx.ErrInvalidToolkit)
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
	return s.toolkit.Invoke(ctx, call)
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
