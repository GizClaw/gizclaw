package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
)

// Factory constructs a workflow transformer from a resolved agent spec.
type Factory interface {
	NewAgent(context.Context, Spec) (genx.Transformer, error)
}

// FactoryFunc adapts a function to Factory.
type FactoryFunc func(context.Context, Spec) (genx.Transformer, error)

func (f FactoryFunc) NewAgent(ctx context.Context, spec Spec) (genx.Transformer, error) {
	return f(ctx, spec)
}

// Registry stores workflow factories keyed by workflow type.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]Factory)}
}

func (r *Registry) Register(workflowType string, factory Factory) error {
	workflowType = normalizeWorkflowType(workflowType)
	if workflowType == "" {
		return fmt.Errorf("agent: workflow type is required")
	}
	if factory == nil {
		return fmt.Errorf("agent: factory is required for %q", workflowType)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.factories == nil {
		r.factories = make(map[string]Factory)
	}
	if _, exists := r.factories[workflowType]; exists {
		return fmt.Errorf("agent: factory already registered for %q", workflowType)
	}
	r.factories[workflowType] = factory
	return nil
}

func (r *Registry) Get(workflowType string) (Factory, bool) {
	if r == nil {
		return nil, false
	}
	workflowType = normalizeWorkflowType(workflowType)
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, ok := r.factories[workflowType]
	return factory, ok
}

func normalizeWorkflowType(workflowType string) string {
	return strings.TrimSpace(workflowType)
}
