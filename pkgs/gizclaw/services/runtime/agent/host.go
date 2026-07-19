package agent

import (
	"context"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

var _ genx.TransformerMux = (*Host)(nil)

// Host resolves a workspace pattern and loads the workflow transformer for one
// agent run. It does not own any per-workspace singleton state.
type Host struct {
	Resolver Resolver
	Registry *Registry
}

func NewHost(resolver Resolver) *Host {
	return &Host{
		Resolver: resolver,
		Registry: NewRegistry(),
	}
}

func (h *Host) Register(workflowType string, factory Factory) error {
	registry := h.registry()
	if registry == nil {
		return fmt.Errorf("agent: registry is required")
	}
	return registry.Register(workflowType, factory)
}

func (h *Host) Transform(ctx context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	if h == nil {
		return nil, fmt.Errorf("agent: host is nil")
	}
	if input == nil {
		return nil, fmt.Errorf("agent: input stream is required")
	}
	if h.Resolver == nil {
		return nil, fmt.Errorf("agent: resolver is required")
	}

	spec, err := h.Resolver.Resolve(ctx, pattern)
	if err != nil {
		return nil, err
	}
	factory, ok := h.registry().Get(spec.WorkflowType)
	if !ok {
		return nil, fmt.Errorf("agent: workflow factory not found for %q", spec.WorkflowType)
	}
	transformer, err := factory.NewAgent(ctx, spec)
	if err != nil {
		return nil, err
	}
	if transformer == nil {
		return nil, fmt.Errorf("agent: factory %q returned nil agent", spec.WorkflowType)
	}
	output, err := transformer.Transform(ctx, input)
	if err != nil {
		return nil, err
	}
	if output == nil {
		return nil, fmt.Errorf("agent: workflow %q returned nil stream", spec.WorkflowType)
	}
	return output, nil
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
