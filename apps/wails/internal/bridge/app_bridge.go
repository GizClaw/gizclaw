package bridge

import (
	"context"
	"fmt"

	"github.com/GizClaw/gizclaw-go/apps/wails/internal/appconfig"
)

type AppBridge struct {
	Paths   appconfig.Paths
	State   appconfig.StateStore
	Context *ContextBridge
}

type BootstrapState struct {
	Contexts []ContextSummary `json:"contexts"`
	Paths    appconfig.Paths  `json:"paths"`
	Runtime  RuntimeContext   `json:"runtime"`
	State    appconfig.State  `json:"state"`
}

func (b *AppBridge) Bootstrap(ctx context.Context) (BootstrapState, error) {
	if b == nil || b.Context == nil {
		return BootstrapState{}, fmt.Errorf("desktop bridge: context bridge is not configured")
	}
	state, err := b.State.Load()
	if err != nil {
		return BootstrapState{}, err
	}
	contexts, err := b.Context.ListContexts(ctx)
	if err != nil {
		return BootstrapState{}, err
	}
	runtime, err := b.Context.RuntimeContext(ctx)
	if err != nil {
		return BootstrapState{}, err
	}
	return BootstrapState{
		Contexts: contexts,
		Paths:    b.Paths,
		Runtime:  runtime,
		State:    state,
	}, nil
}

func (b *AppBridge) SetSelectedView(_ context.Context, view string) (appconfig.State, error) {
	if b == nil {
		return appconfig.State{}, fmt.Errorf("desktop bridge: app bridge is not configured")
	}
	state, err := b.State.Load()
	if err != nil {
		return appconfig.State{}, err
	}
	state.SelectedView = appconfig.NormalizeView(view)
	if err := b.State.Save(state); err != nil {
		return appconfig.State{}, err
	}
	return state, nil
}
