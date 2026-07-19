package agenthost

import (
	"context"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

var _ genx.Transformer = (*Host)(nil)

type Host struct {
	Resolver        Resolver
	Registry        *Registry
	Transformers    *TransformerRegistry
	Coordinator     Coordinator
	RuntimeRegistry *RuntimeRegistry
}

func New(resolver Resolver) *Host {
	return &Host{
		Resolver:        resolver,
		Registry:        NewRegistry(),
		Transformers:    NewTransformerRegistry(),
		Coordinator:     NewMemoryCoordinator(),
		RuntimeRegistry: NewRuntimeRegistry(),
	}
}

func (h *Host) RegisterTransformer(transformerType string, factory TransformerFactory) error {
	registry := h.transformerRegistry()
	if registry == nil {
		return fmt.Errorf("agenthost: transformer registry is required")
	}
	return registry.Register(transformerType, factory)
}

func (h *Host) Register(agentType string, factory Factory) error {
	registry := h.registry()
	if registry == nil {
		return fmt.Errorf("agenthost: registry is required")
	}
	return registry.Register(agentType, factory)
}

func (h *Host) Transform(ctx context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	if h == nil {
		return nil, fmt.Errorf("agenthost: host is nil")
	}
	if input == nil {
		return nil, fmt.Errorf("agenthost: input stream is required")
	}
	agent, release, err := h.OpenAgent(ctx, pattern)
	if err != nil {
		return nil, err
	}
	output, err := agent.Transform(ctx, pattern, input)
	if err != nil {
		release()
		return nil, err
	}
	if output == nil {
		release()
		return nil, fmt.Errorf("agenthost: agent returned nil stream")
	}
	return &leaseStream{Stream: output, release: release}, nil
}

func (h *Host) OpenAgent(ctx context.Context, pattern string) (Agent, func(), error) {
	if h == nil {
		return nil, nil, fmt.Errorf("agenthost: host is nil")
	}
	if h.Resolver == nil {
		return nil, nil, fmt.Errorf("agenthost: resolver is required")
	}

	spec, err := h.Resolver.Resolve(ctx, pattern)
	if err != nil {
		return nil, nil, err
	}
	workspaceName := string(spec.Workspace.Name)
	if workspaceName == "" {
		return nil, nil, fmt.Errorf("agenthost: resolved workspace name is required")
	}
	return h.runtimeRegistry().Acquire(ctx, h, workspaceName, spec)
}

func (h *Host) openWorkspaceAgent(ctx context.Context, workspaceName string, spec Spec) (Agent, func(), error) {
	coordinator := h.coordinator()
	if coordinator == nil {
		return nil, nil, fmt.Errorf("agenthost: coordinator is required")
	}
	lease, err := coordinator.Acquire(ctx, workspaceName)
	if err != nil {
		return nil, nil, err
	}

	release := func() {
		_ = lease.Release(context.Background())
	}
	agent, err := h.constructRuntime(ctx, spec)
	if err != nil {
		release()
		return nil, nil, err
	}
	if agent == nil {
		release()
		return nil, nil, fmt.Errorf("agenthost: factory %q returned nil agent", spec.AgentType)
	}
	agent = wrapHistoryAgent(agent, spec.Runtime.History)
	return agent, release, nil
}

func (h *Host) constructRuntime(ctx context.Context, spec Spec) (Agent, error) {
	if factory, ok := h.registry().Get(spec.AgentType); ok {
		return factory.NewAgent(ctx, spec)
	}
	if factory, ok := h.transformerRegistry().Get(spec.AgentType); ok {
		transformer, err := factory.NewTransformer(ctx, spec)
		if err != nil {
			return nil, err
		}
		if transformer == nil {
			return nil, fmt.Errorf("agenthost: transformer factory %q returned nil transformer", spec.AgentType)
		}
		return NewTransformerAgent(transformer), nil
	}
	return nil, fmt.Errorf("agenthost: runtime factory not found for %q", spec.AgentType)
}

func (h *Host) registry() *Registry {
	if h == nil {
		return nil
	}
	if h.Registry == nil {
		h.Registry = NewRegistry()
	}
	return h.Registry
}

func (h *Host) transformerRegistry() *TransformerRegistry {
	if h == nil {
		return nil
	}
	if h.Transformers == nil {
		h.Transformers = NewTransformerRegistry()
	}
	return h.Transformers
}

func (h *Host) coordinator() Coordinator {
	if h == nil {
		return nil
	}
	if h.Coordinator == nil {
		h.Coordinator = NewMemoryCoordinator()
	}
	return h.Coordinator
}

func (h *Host) runtimeRegistry() *RuntimeRegistry {
	if h == nil {
		return nil
	}
	if h.RuntimeRegistry == nil {
		h.RuntimeRegistry = NewRuntimeRegistry()
	}
	return h.RuntimeRegistry
}

// WorkspaceRuntimes returns the runtime registry shared by peer-scoped host
// views that should attach to the same workspace agent instances.
func (h *Host) WorkspaceRuntimes() *RuntimeRegistry {
	return h.runtimeRegistry()
}
