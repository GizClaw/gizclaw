package bridge

import (
	"context"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/apps/wails/internal/appconfig"
)

type AppBridge struct {
	Paths   appconfig.Paths
	State   appconfig.StateStore
	Context *ContextBridge
}

type BootstrapState struct {
	Contexts []ContextSummary `json:"contexts"`
	State    AppState         `json:"state"`
	Views    []DesktopView    `json:"views"`
	Session  ViewSession      `json:"view_session"`
}

type AppState struct {
	LastContext string `json:"last_context,omitempty"`
	LastView    string `json:"last_view,omitempty"`
}

type DesktopView struct {
	Description string `json:"description,omitempty"`
	ID          string `json:"id"`
	Title       string `json:"title"`
}

type ViewSession struct {
	Active      bool   `json:"active"`
	ContextName string `json:"context_name,omitempty"`
	View        string `json:"view,omitempty"`
}

type StartViewSessionRequest struct {
	ContextName string `json:"context_name"`
	View        string `json:"view"`
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
	session := viewSession(state)
	return BootstrapState{
		Contexts: contexts,
		State: AppState{
			LastContext: state.LastContext,
			LastView:    state.LastView,
		},
		Views:   ListViews(),
		Session: session,
	}, nil
}

func (b *AppBridge) ListViews(context.Context) ([]DesktopView, error) {
	return ListViews(), nil
}

func (b *AppBridge) StartViewSession(_ context.Context, req StartViewSessionRequest) (ViewSession, error) {
	if b == nil {
		return ViewSession{}, fmt.Errorf("desktop bridge: app bridge is not configured")
	}
	if b.Context == nil {
		return ViewSession{}, fmt.Errorf("desktop bridge: context bridge is not configured")
	}
	contextName := strings.TrimSpace(req.ContextName)
	if contextName == "" {
		return ViewSession{}, fmt.Errorf("desktop bridge: context name is required")
	}
	if err := b.Context.Store.Use(contextName); err != nil {
		return ViewSession{}, err
	}
	state, err := b.State.Load()
	if err != nil {
		return ViewSession{}, err
	}
	view := appconfig.NormalizeView(req.View)
	state.LastContext = contextName
	state.LastView = view
	state.SessionActive = true
	state.SessionContext = contextName
	state.SessionView = view
	if err := b.State.Save(state); err != nil {
		return ViewSession{}, err
	}
	return viewSession(state), nil
}

func (b *AppBridge) EndViewSession(context.Context) (ViewSession, error) {
	if b == nil {
		return ViewSession{}, fmt.Errorf("desktop bridge: app bridge is not configured")
	}
	state, err := b.State.Load()
	if err != nil {
		return ViewSession{}, err
	}
	state.SessionActive = false
	state.SessionContext = ""
	state.SessionView = ""
	if err := b.State.Save(state); err != nil {
		return ViewSession{}, err
	}
	return viewSession(state), nil
}

func (b *AppBridge) GetViewSession(context.Context) (ViewSession, error) {
	if b == nil {
		return ViewSession{}, fmt.Errorf("desktop bridge: app bridge is not configured")
	}
	state, err := b.State.Load()
	if err != nil {
		return ViewSession{}, err
	}
	return viewSession(state), nil
}

func (b *AppBridge) InjectedRuntime(ctx context.Context) (RuntimeContext, error) {
	if b == nil {
		return RuntimeContext{}, fmt.Errorf("desktop bridge: app bridge is not configured")
	}
	if b.Context == nil {
		return RuntimeContext{}, fmt.Errorf("desktop bridge: context bridge is not configured")
	}
	if b.Context.Store == nil {
		return RuntimeContext{}, fmt.Errorf("desktop bridge: context store is not configured")
	}
	state, err := b.State.Load()
	if err != nil {
		return RuntimeContext{}, err
	}
	session := viewSession(state)
	if !session.Active || session.ContextName == "" {
		return RuntimeContext{}, fmt.Errorf("desktop bridge: no active view session")
	}
	if err := b.Context.Store.Use(session.ContextName); err != nil {
		return RuntimeContext{}, err
	}
	return b.Context.RuntimeContext(ctx)
}

func ListViews() []DesktopView {
	return []DesktopView{
		{ID: "admin", Title: "Admin", Description: "Manage GizClaw server resources."},
		{ID: "play", Title: "Play", Description: "Use workspaces, chat history, social, and firmware flows."},
	}
}

func viewSession(state appconfig.State) ViewSession {
	if !state.SessionActive {
		return ViewSession{Active: false}
	}
	return ViewSession{
		Active:      true,
		ContextName: state.SessionContext,
		View:        appconfig.NormalizeView(state.SessionView),
	}
}
