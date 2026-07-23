package pet

import (
	"context"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
)

const Type = "pet"

// ContextProvider resolves the adopted pet attached to a Workspace. The
// provider is invoked for every turn so Gameplay attribute changes are visible
// without recreating the agent.
type ContextProvider interface {
	ResolvePetContext(context.Context, string) (apitypes.Pet, apitypes.PetDef, error)
}

// Factory is the Pet domain wrapper. It injects transient Pet context and
// delegates construction to the same registered non-Pet driver factory used by
// ordinary Workflows.
type Factory struct {
	Pets      ContextProvider
	Factories *agenthost.Registry
}

func (f Factory) NewAgent(ctx context.Context, spec agenthost.Spec) (agenthost.Agent, error) {
	if spec.Workflow.Spec.Pet == nil {
		return nil, fmt.Errorf("pet: workflow spec.pet is required")
	}
	if f.Pets == nil {
		return nil, fmt.Errorf("pet: gameplay context provider is required")
	}
	if f.Factories == nil {
		return nil, fmt.Errorf("pet: nested workflow factories are required")
	}
	workspaceName := strings.TrimSpace(spec.Workspace.Name)
	if workspaceName == "" {
		return nil, fmt.Errorf("pet: workspace name is required")
	}
	if _, _, err := f.Pets.ResolvePetContext(ctx, workspaceName); err != nil {
		return nil, fmt.Errorf("pet: resolve workspace %q: %w", workspaceName, err)
	}

	nested := *spec.Workflow.Spec.Pet
	driver := strings.TrimSpace(string(nested.Driver))
	factory, ok := f.Factories.Get(driver)
	if !ok {
		return nil, fmt.Errorf("pet: nested workflow factory not found for %q", driver)
	}
	spec.Workflow.Spec = apitypes.WorkflowSpec{
		Driver:         apitypes.WorkflowDriver(nested.Driver),
		Toolkit:        nested.Toolkit,
		Flowcraft:      nested.Flowcraft,
		DoubaoRealtime: nested.DoubaoRealtime,
		AstTranslate:   nested.AstTranslate,
		Chatroom:       nested.Chatroom,
	}
	spec.AgentType = driver
	spec.Workspace.Parameters = nil
	provideInputs := func(turnCtx context.Context) (map[string]any, error) {
		pet, petDef, err := f.Pets.ResolvePetContext(turnCtx, workspaceName)
		if err != nil {
			return nil, fmt.Errorf("resolve workspace %q: %w", workspaceName, err)
		}
		return turnInputs(pet, petDef), nil
	}
	spec.BoardInputs = func(turnCtx context.Context) (map[string]any, error) {
		if inputs, ok := agenthost.BoardInputsFromContext(turnCtx); ok {
			return inputs, nil
		}
		return provideInputs(turnCtx)
	}
	agent, err := factory.NewAgent(ctx, spec)
	if err != nil {
		return nil, err
	}
	return agenthost.NewBoardInputsAgent(agent, provideInputs), nil
}
