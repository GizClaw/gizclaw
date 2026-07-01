package main

import (
	"context"
	"fmt"

	"github.com/GizClaw/gizclaw-go/apps/wails/internal/appconfig"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/bridge"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/contextstore"
)

type App struct {
	bridge *bridge.AppBridge
}

func NewApp() (*App, error) {
	paths, err := appconfig.DefaultPaths()
	if err != nil {
		return nil, err
	}
	return NewAppWithPaths(paths)
}

func NewAppWithPaths(paths appconfig.Paths) (*App, error) {
	if err := paths.Ensure(); err != nil {
		return nil, err
	}
	state := appconfig.StateStore{File: paths.StateFile}
	store := &contextstore.Store{Root: paths.ContextDir}
	contextBridge := bridge.NewContextBridge(store, state)
	return &App{
		bridge: &bridge.AppBridge{
			Paths:   paths,
			State:   state,
			Context: contextBridge,
		},
	}, nil
}

func (a *App) Bootstrap() (bridge.BootstrapState, error) {
	if a == nil || a.bridge == nil {
		return bridge.BootstrapState{}, fmt.Errorf("desktop app: bridge is not configured")
	}
	return a.bridge.Bootstrap(context.Background())
}

func (a *App) ListContexts() ([]bridge.ContextSummary, error) {
	if a == nil || a.bridge == nil || a.bridge.Context == nil {
		return nil, fmt.Errorf("desktop app: context bridge is not configured")
	}
	return a.bridge.Context.ListContexts(context.Background())
}

func (a *App) SelectContext(name string) (bridge.RuntimeContext, error) {
	if a == nil || a.bridge == nil || a.bridge.Context == nil {
		return bridge.RuntimeContext{}, fmt.Errorf("desktop app: context bridge is not configured")
	}
	return a.bridge.Context.SelectContext(context.Background(), name)
}

func (a *App) CreateContext(req bridge.CreateContextRequest) (bridge.RuntimeContext, error) {
	if a == nil || a.bridge == nil || a.bridge.Context == nil {
		return bridge.RuntimeContext{}, fmt.Errorf("desktop app: context bridge is not configured")
	}
	return a.bridge.Context.CreateContext(context.Background(), req)
}

func (a *App) RuntimeContext() (bridge.RuntimeContext, error) {
	if a == nil || a.bridge == nil || a.bridge.Context == nil {
		return bridge.RuntimeContext{}, fmt.Errorf("desktop app: context bridge is not configured")
	}
	return a.bridge.Context.RuntimeContext(context.Background())
}

func (a *App) SetSelectedView(view string) (appconfig.State, error) {
	if a == nil || a.bridge == nil {
		return appconfig.State{}, fmt.Errorf("desktop app: bridge is not configured")
	}
	return a.bridge.SetSelectedView(context.Background(), view)
}
