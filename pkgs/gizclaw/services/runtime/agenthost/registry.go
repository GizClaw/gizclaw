package agenthost

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
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

// TransformerFactory constructs an ordinary stream Transformer. It is kept
// separate from Factory so non-reasoning workflows are not registered as AI
// Agents merely because they share the outer workspace stream host.
type TransformerFactory interface {
	NewTransformer(context.Context, Spec) (genx.Transformer, error)
}

// TransformerFactoryFunc adapts a function to TransformerFactory.
type TransformerFactoryFunc func(context.Context, Spec) (genx.Transformer, error)

func (f TransformerFactoryFunc) NewTransformer(ctx context.Context, spec Spec) (genx.Transformer, error) {
	return f(ctx, spec)
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

// TransformerRegistry stores ordinary Transformer factories separately from
// the AI Agent registry.
type TransformerRegistry struct {
	mu        sync.RWMutex
	factories map[string]TransformerFactory
}

func NewTransformerRegistry() *TransformerRegistry {
	return &TransformerRegistry{factories: make(map[string]TransformerFactory)}
}

func (r *TransformerRegistry) Register(transformerType string, factory TransformerFactory) error {
	transformerType = normalizeAgentType(transformerType)
	if transformerType == "" {
		return fmt.Errorf("agenthost: transformer type is required")
	}
	if factory == nil {
		return fmt.Errorf("agenthost: transformer factory is required for %q", transformerType)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.factories == nil {
		r.factories = make(map[string]TransformerFactory)
	}
	if _, exists := r.factories[transformerType]; exists {
		return fmt.Errorf("agenthost: transformer factory already registered for %q", transformerType)
	}
	r.factories[transformerType] = factory
	return nil
}

func (r *TransformerRegistry) Get(transformerType string) (TransformerFactory, bool) {
	if r == nil {
		return nil, false
	}
	transformerType = normalizeAgentType(transformerType)
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, ok := r.factories[transformerType]
	return factory, ok
}
