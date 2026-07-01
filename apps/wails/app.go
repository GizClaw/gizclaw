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

func (a *App) SelectContext(name string) (bridge.ContextSummary, error) {
	if a == nil || a.bridge == nil || a.bridge.Context == nil {
		return bridge.ContextSummary{}, fmt.Errorf("desktop app: context bridge is not configured")
	}
	return a.bridge.Context.SelectContext(context.Background(), name)
}

func (a *App) CreateContext(req bridge.CreateContextRequest) (bridge.ContextSummary, error) {
	if a == nil || a.bridge == nil || a.bridge.Context == nil {
		return bridge.ContextSummary{}, fmt.Errorf("desktop app: context bridge is not configured")
	}
	return a.bridge.Context.CreateContext(context.Background(), req)
}

func (a *App) InjectedRuntime() (bridge.RuntimeContext, error) {
	if a == nil || a.bridge == nil {
		return bridge.RuntimeContext{}, fmt.Errorf("desktop app: bridge is not configured")
	}
	return a.bridge.InjectedRuntime(context.Background())
}

func (a *App) ListViews() ([]bridge.DesktopView, error) {
	if a == nil || a.bridge == nil {
		return nil, fmt.Errorf("desktop app: bridge is not configured")
	}
	return a.bridge.ListViews(context.Background())
}

func (a *App) GetViewSession() (bridge.ViewSession, error) {
	if a == nil || a.bridge == nil {
		return bridge.ViewSession{}, fmt.Errorf("desktop app: bridge is not configured")
	}
	return a.bridge.GetViewSession(context.Background())
}

func (a *App) StartViewSession(req bridge.StartViewSessionRequest) (bridge.ViewSession, error) {
	if a == nil || a.bridge == nil {
		return bridge.ViewSession{}, fmt.Errorf("desktop app: bridge is not configured")
	}
	return a.bridge.StartViewSession(context.Background(), req)
}

func (a *App) EndViewSession() (bridge.ViewSession, error) {
	if a == nil || a.bridge == nil {
		return bridge.ViewSession{}, fmt.Errorf("desktop app: bridge is not configured")
	}
	return a.bridge.EndViewSession(context.Background())
}
