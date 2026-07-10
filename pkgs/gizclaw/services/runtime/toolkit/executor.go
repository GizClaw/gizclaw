package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

type Call struct {
	ID        string
	Tool      Tool
	Args      json.RawMessage
	SubjectID string
}

type Result struct {
	Data json.RawMessage
}

type Executor interface {
	Invoke(context.Context, Call) (Result, error)
}

type ExecutorFunc func(context.Context, Call) (Result, error)

func (f ExecutorFunc) Invoke(ctx context.Context, call Call) (Result, error) {
	return f(ctx, call)
}

type ExecutorRegistry struct {
	mu        sync.RWMutex
	executors map[string]Executor
}

func NewExecutorRegistry() *ExecutorRegistry {
	return &ExecutorRegistry{executors: make(map[string]Executor)}
}

func (r *ExecutorRegistry) Register(name string, executor Executor) error {
	if r == nil {
		return ErrNotConfigured
	}
	if name == "" {
		return fmt.Errorf("%w: executor name is required", ErrInvalidTool)
	}
	if executor == nil {
		return fmt.Errorf("%w: executor is required", ErrInvalidTool)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.executors == nil {
		r.executors = make(map[string]Executor)
	}
	r.executors[name] = executor
	return nil
}

func (r *ExecutorRegistry) Has(name string) bool {
	if r == nil {
		return false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	r.mu.RLock()
	executor := r.executors[name]
	r.mu.RUnlock()
	return executor != nil
}

func (r *ExecutorRegistry) Invoke(ctx context.Context, call Call) (Result, error) {
	if r == nil {
		return Result{}, ErrNotConfigured
	}
	name := trimPtr(call.Tool.Executor.Name)
	if call.Tool.Executor.Kind != ToolExecutorKindBuiltin || name == "" {
		return Result{}, fmt.Errorf("%w: %s", ErrExecutorNotFound, call.Tool.ID)
	}
	r.mu.RLock()
	executor := r.executors[name]
	r.mu.RUnlock()
	if executor == nil {
		return Result{}, fmt.Errorf("%w: %s", ErrExecutorNotFound, name)
	}
	result, err := executor.Invoke(ctx, call)
	if err != nil {
		return Result{}, err
	}
	result.Data = cloneRaw(result.Data)
	return result, nil
}
