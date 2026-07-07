package main

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/apps/wails/internal/appconfig"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/bridge"
)

func TestNewAppUsesConfiguredHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv(appconfig.EnvConfigHome, root)

	app, err := NewApp()
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}
	if app == nil || app.bridge == nil || app.bridge.Paths.ConfigRoot != root {
		t.Fatalf("NewApp() = %#v", app)
	}
}

func TestAppExposesContextRuntimeWithoutServerAccess(t *testing.T) {
	app, err := NewAppWithPaths(appconfig.NewPaths(t.TempDir()))
	if err != nil {
		t.Fatalf("NewAppWithPaths() error = %v", err)
	}
	created, err := app.CreateContext(bridge.CreateContextRequest{
		Description: "Local e2e",
		Endpoint:    "127.0.0.1:9820",
		Name:        "local",
	})
	if err != nil {
		t.Fatalf("CreateContext() error = %v", err)
	}
	if created.Name != "local" {
		t.Fatalf("created = %+v", created)
	}
	bootstrap, err := app.Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if len(bootstrap.Contexts) != 1 || len(bootstrap.Views) != 2 || bootstrap.Session.Active {
		t.Fatalf("bootstrap = %+v", bootstrap)
	}
	session, err := app.StartViewSession(bridge.StartViewSessionRequest{ContextName: "local", View: "admin"})
	if err != nil {
		t.Fatalf("StartViewSession() error = %v", err)
	}
	if !session.Active || session.ContextName != "local" || session.View != "admin" {
		t.Fatalf("session = %+v", session)
	}
	runtime, err := app.InjectedRuntime()
	if err != nil {
		t.Fatalf("InjectedRuntime() error = %v", err)
	}
	if runtime.Context == nil || runtime.Context.Endpoint != "127.0.0.1:9820" || runtime.PrivateKeyBase64 == "" {
		t.Fatalf("runtime = %+v", runtime)
	}
	ended, err := app.EndViewSession()
	if err != nil {
		t.Fatalf("EndViewSession() error = %v", err)
	}
	if ended.Active {
		t.Fatalf("ended = %+v", ended)
	}
	if _, err := app.InjectedRuntime(); err == nil {
		t.Fatalf("InjectedRuntime() after EndViewSession() error = nil")
	}

	contexts, err := app.ListContexts()
	if err != nil {
		t.Fatalf("ListContexts() error = %v", err)
	}
	if len(contexts) != 1 || contexts[0].Name != "local" {
		t.Fatalf("ListContexts() = %+v", contexts)
	}
	selected, err := app.SelectContext("local")
	if err != nil {
		t.Fatalf("SelectContext() error = %v", err)
	}
	if !selected.Current {
		t.Fatalf("SelectContext() = %+v", selected)
	}
	views, err := app.ListViews()
	if err != nil {
		t.Fatalf("ListViews() error = %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("ListViews() = %+v", views)
	}
	currentSession, err := app.GetViewSession()
	if err != nil {
		t.Fatalf("GetViewSession() error = %v", err)
	}
	if currentSession.Active {
		t.Fatalf("GetViewSession() = %+v", currentSession)
	}
}

func TestAppFacadeRequiresConfiguredBridge(t *testing.T) {
	var nilApp *App
	if _, err := nilApp.Bootstrap(); err == nil {
		t.Fatal("nil Bootstrap() error = nil")
	}
	if _, err := nilApp.ListContexts(); err == nil {
		t.Fatal("nil ListContexts() error = nil")
	}
	if _, err := nilApp.SelectContext("local"); err == nil {
		t.Fatal("nil SelectContext() error = nil")
	}
	if _, err := nilApp.CreateContext(bridge.CreateContextRequest{}); err == nil {
		t.Fatal("nil CreateContext() error = nil")
	}
	if _, err := nilApp.InjectedRuntime(); err == nil {
		t.Fatal("nil InjectedRuntime() error = nil")
	}
	if _, err := nilApp.ListViews(); err == nil {
		t.Fatal("nil ListViews() error = nil")
	}
	if _, err := nilApp.GetViewSession(); err == nil {
		t.Fatal("nil GetViewSession() error = nil")
	}
	if _, err := nilApp.StartViewSession(bridge.StartViewSessionRequest{}); err == nil {
		t.Fatal("nil StartViewSession() error = nil")
	}
	if _, err := nilApp.EndViewSession(); err == nil {
		t.Fatal("nil EndViewSession() error = nil")
	}
}
