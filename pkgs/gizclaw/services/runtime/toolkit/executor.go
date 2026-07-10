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
	mu                 sync.RWMutex
	executors          map[string]Executor
	device             Executor
	deviceAvailability AvailabilityChecker
}

func (r *ExecutorRegistry) RegisterDevice(executor Executor, availability AvailabilityChecker) error {
	if r == nil {
		return ErrNotConfigured
	}
	if executor == nil || availability == nil {
		return fmt.Errorf("%w: device executor and availability are required", ErrInvalidTool)
	}
	r.mu.Lock()
	r.device = executor
	r.deviceAvailability = availability
	r.mu.Unlock()
	return nil
}

func NewExecutorRegistry() *ExecutorRegistry {
	return &ExecutorRegistry{executors: make(map[string]Executor)}
}

func (r *ExecutorRegistry) Register(name string, executor Executor) error {
	if r == nil {
		return ErrNotConfigured
	}
	name = strings.TrimSpace(name)
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
	r.mu.RLock()
	var executor Executor
	var name string
	switch call.Tool.Executor.Kind {
	case ToolExecutorKindBuiltin:
		name = trimPtr(call.Tool.Executor.Name)
		executor = r.executors[name]
	case ToolExecutorKindDeviceRPC:
		name = call.Tool.ID
		executor = r.device
	}
	r.mu.RUnlock()
	if name == "" || executor == nil {
		return Result{}, fmt.Errorf("%w: %s", ErrExecutorNotFound, name)
	}
	result, err := executor.Invoke(ctx, call)
	if err != nil {
		return Result{}, err
	}
	result.Data = cloneRaw(result.Data)
	return result, nil
}

func (r *ExecutorRegistry) ToolAvailable(ctx context.Context, tool Tool) (bool, error) {
	if r == nil {
		return false, nil
	}
	switch tool.Executor.Kind {
	case ToolExecutorKindBuiltin:
		return r.Has(trimPtr(tool.Executor.Name)), nil
	case ToolExecutorKindDeviceRPC:
		r.mu.RLock()
		availability := r.deviceAvailability
		executor := r.device
		r.mu.RUnlock()
		if availability == nil || executor == nil {
			return false, nil
		}
		return availability.ToolAvailable(ctx, tool)
	default:
		return false, nil
	}
}
