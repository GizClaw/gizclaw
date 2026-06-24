package agenthost

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
)

// Factory constructs an agent runtime from a resolved workspace spec.
type Factory interface {
	NewAgent(context.Context, Spec) (Agent, error)
}

// FactoryFunc adapts a function to Factory.
type FactoryFunc func(context.Context, Spec) (genx.Transformer, error)

func (f FactoryFunc) NewAgent(ctx context.Context, spec Spec) (Agent, error) {
	transformer, err := f(ctx, spec)
	if err != nil {
		return nil, err
	}
	return asAgent(transformer), nil
}

// Registry stores agent factories keyed by agent type.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]Factory)}
}

func (r *Registry) Register(agentType string, factory Factory) error {
	agentType = normalizeAgentType(agentType)
	if agentType == "" {
		return fmt.Errorf("agenthost: agent type is required")
	}
	if factory == nil {
		return fmt.Errorf("agenthost: factory is required for %q", agentType)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.factories == nil {
		r.factories = make(map[string]Factory)
	}
	if _, exists := r.factories[agentType]; exists {
		return fmt.Errorf("agenthost: factory already registered for %q", agentType)
	}
	r.factories[agentType] = factory
	return nil
}

func (r *Registry) Get(agentType string) (Factory, bool) {
	if r == nil {
		return nil, false
	}
	agentType = normalizeAgentType(agentType)
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, ok := r.factories[agentType]
	return factory, ok
}

func normalizeAgentType(agentType string) string {
	return strings.TrimSpace(agentType)
}
