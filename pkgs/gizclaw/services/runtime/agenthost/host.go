package agenthost

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

var _ genx.TransformerMux = (*Host)(nil)

type Host struct {
	Resolver        Resolver
	Registry        *Registry
	Coordinator     Coordinator
	RuntimeRegistry *RuntimeRegistry
}

func New(resolver Resolver) *Host {
	return &Host{
		Resolver:        resolver,
		Registry:        NewRegistry(),
		Coordinator:     NewMemoryCoordinator(),
		RuntimeRegistry: NewRuntimeRegistry(),
	}
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
	output, err := agent.Transform(ctx, input)
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

	releaseLease := func() { _ = lease.Release(context.Background()) }
	factory, ok := h.registry().Get(spec.AgentType)
	if !ok {
		releaseLease()
		return nil, nil, fmt.Errorf("agenthost: agent factory not found for %q", spec.AgentType)
	}
	agent, err := factory.NewAgent(ctx, spec)
	if err != nil {
		releaseLease()
		return nil, nil, err
	}
	if agent == nil {
		releaseLease()
		return nil, nil, fmt.Errorf("agenthost: factory %q returned nil agent", spec.AgentType)
	}
	var closer io.Closer
	if candidate, ok := agent.(io.Closer); ok {
		closer = candidate
	}
	agent = wrapHistoryAgent(agent, spec.Runtime.History)
	var once sync.Once
	release := func() {
		once.Do(func() {
			if closer != nil {
				_ = closer.Close()
			}
			releaseLease()
		})
	}
	return agent, release, nil
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
