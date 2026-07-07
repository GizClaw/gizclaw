package bridge

import (
	"context"
	"encoding/base64"
	"path/filepath"
	"testing"

	"github.com/GizClaw/gizclaw-go/apps/wails/internal/appconfig"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/contextstore"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestContextBridgeListSelectAndRuntime(t *testing.T) {
	root := t.TempDir()
	store := &contextstore.Store{Root: filepath.Join(root, "contexts")}
	if err := store.CreateWithOptions("local", "127.0.0.1:9820", contextstore.CreateOptions{
		Description: "Local context",
	}); err != nil {
		t.Fatalf("CreateWithOptions() error = %v", err)
	}
	bridge := NewContextBridge(store, appconfig.StateStore{File: filepath.Join(root, "state.json")})

	contexts, err := bridge.ListContexts(context.Background())
	if err != nil {
		t.Fatalf("ListContexts() error = %v", err)
	}
	if len(contexts) != 1 || contexts[0].Name != "local" || contexts[0].Description != "Local context" {
		t.Fatalf("ListContexts() = %+v", contexts)
	}

	selected, err := bridge.SelectContext(context.Background(), "local")
	if err != nil {
		t.Fatalf("SelectContext() error = %v", err)
	}
	if !selected.Current || selected.Endpoint != "127.0.0.1:9820" {
		t.Fatalf("SelectContext() = %+v", selected)
	}
	runtime, err := bridge.RuntimeContext(context.Background())
	if err != nil {
		t.Fatalf("RuntimeContext() error = %v", err)
	}
	if runtime.Context == nil || runtime.Context.Name != "local" {
		t.Fatalf("RuntimeContext() = %+v", runtime)
	}
	privateKey, err := base64.StdEncoding.DecodeString(runtime.PrivateKeyBase64)
	if err != nil {
		t.Fatalf("PrivateKeyBase64 decode error = %v", err)
	}
	if len(privateKey) != giznet.KeySize {
		t.Fatalf("private key length = %d, want %d", len(privateKey), giznet.KeySize)
	}

	state, err := bridge.State.Load()
	if err != nil {
		t.Fatalf("State.Load() error = %v", err)
	}
	if state.LastContext != "local" {
		t.Fatalf("LastContext = %q", state.LastContext)
	}
}

func TestContextBridgeCreateContext(t *testing.T) {
	root := t.TempDir()
	paths := appconfig.NewPaths(root)
	bridge := NewContextBridge(&contextstore.Store{Root: paths.ContextDir}, appconfig.StateStore{File: paths.StateFile})

	created, err := bridge.CreateContext(context.Background(), CreateContextRequest{
		Description: "Dev server",
		Endpoint:    "127.0.0.1:9820",
		Name:        "dev",
	})
	if err != nil {
		t.Fatalf("CreateContext() error = %v", err)
	}
	if created.Name != "dev" || created.Description != "Dev server" {
		t.Fatalf("CreateContext() = %+v", created)
	}
}

func TestContextBridgeGuards(t *testing.T) {
	var nilBridge *ContextBridge
	if _, err := nilBridge.ListContexts(context.Background()); err == nil {
		t.Fatal("nil ListContexts() error = nil")
	}
	if _, err := nilBridge.SelectContext(context.Background(), "local"); err == nil {
		t.Fatal("nil SelectContext() error = nil")
	}
	if _, err := nilBridge.CreateContext(context.Background(), CreateContextRequest{Name: "local"}); err == nil {
		t.Fatal("nil CreateContext() error = nil")
	}
	if _, err := nilBridge.RuntimeContext(context.Background()); err == nil {
		t.Fatal("nil RuntimeContext() error = nil")
	}

	root := t.TempDir()
	paths := appconfig.NewPaths(root)
	bridge := NewContextBridge(&contextstore.Store{Root: paths.ContextDir}, appconfig.StateStore{File: paths.StateFile})
	if _, err := bridge.SelectContext(context.Background(), " "); err == nil {
		t.Fatal("SelectContext(empty) error = nil")
	}
	if _, err := bridge.CreateContext(context.Background(), CreateContextRequest{Name: " "}); err == nil {
		t.Fatal("CreateContext(empty) error = nil")
	}
	runtime, err := bridge.RuntimeContext(context.Background())
	if err != nil {
		t.Fatalf("RuntimeContext(empty current) error = %v", err)
	}
	if runtime.Context != nil || runtime.PrivateKeyBase64 != "" {
		t.Fatalf("RuntimeContext(empty current) = %+v", runtime)
	}
}

func TestAppBridgeBootstrap(t *testing.T) {
	root := t.TempDir()
	paths := appconfig.NewPaths(root)
	store := &contextstore.Store{Root: paths.ContextDir}
	if err := store.Create("local", "127.0.0.1:9820"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := store.Use("local"); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	state := appconfig.StateStore{File: paths.StateFile}
	if err := state.Save(appconfig.State{LastView: "admin"}); err != nil {
		t.Fatalf("State.Save() error = %v", err)
	}
	app := &AppBridge{
		Paths:   paths,
		State:   state,
		Context: NewContextBridge(store, state),
	}

	bootstrap, err := app.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if len(bootstrap.Contexts) != 1 || len(bootstrap.Views) != 2 || bootstrap.Session.Active {
		t.Fatalf("Bootstrap() = %+v", bootstrap)
	}
	if bootstrap.State.LastView != "admin" {
		t.Fatalf("LastView = %q", bootstrap.State.LastView)
	}

	next, err := app.StartViewSession(context.Background(), StartViewSessionRequest{ContextName: "local", View: "play"})
	if err != nil {
		t.Fatalf("StartViewSession() error = %v", err)
	}
	if !next.Active || next.View != "play" || next.ContextName != "local" {
		t.Fatalf("StartViewSession() = %+v", next)
	}
	runtime, err := app.InjectedRuntime(context.Background())
	if err != nil {
		t.Fatalf("InjectedRuntime() error = %v", err)
	}
	if runtime.Context == nil || runtime.Context.Name != "local" || runtime.PrivateKeyBase64 == "" {
		t.Fatalf("InjectedRuntime() = %+v", runtime)
	}
	ended, err := app.EndViewSession(context.Background())
	if err != nil {
		t.Fatalf("EndViewSession() error = %v", err)
	}
	if ended.Active {
		t.Fatalf("EndViewSession() = %+v", ended)
	}
	if _, err := app.InjectedRuntime(context.Background()); err == nil {
		t.Fatalf("InjectedRuntime() after EndViewSession() error = nil")
	}
}

func TestAppBridgeViewSessionGuards(t *testing.T) {
	var nilBridge *AppBridge
	if _, err := nilBridge.ListViews(context.Background()); err != nil {
		t.Fatalf("nil ListViews() error = %v", err)
	}
	if _, err := nilBridge.StartViewSession(context.Background(), StartViewSessionRequest{}); err == nil {
		t.Fatal("nil StartViewSession() error = nil")
	}
	if _, err := nilBridge.EndViewSession(context.Background()); err == nil {
		t.Fatal("nil EndViewSession() error = nil")
	}
	if _, err := nilBridge.GetViewSession(context.Background()); err == nil {
		t.Fatal("nil GetViewSession() error = nil")
	}
	if _, err := nilBridge.InjectedRuntime(context.Background()); err == nil {
		t.Fatal("nil InjectedRuntime() error = nil")
	}

	root := t.TempDir()
	paths := appconfig.NewPaths(root)
	state := appconfig.StateStore{File: paths.StateFile}
	app := &AppBridge{State: state}
	if _, err := app.Bootstrap(context.Background()); err == nil {
		t.Fatal("Bootstrap(without context bridge) error = nil")
	}
	if _, err := app.StartViewSession(context.Background(), StartViewSessionRequest{ContextName: "local"}); err == nil {
		t.Fatal("StartViewSession(without context bridge) error = nil")
	}
	if _, err := app.InjectedRuntime(context.Background()); err == nil {
		t.Fatal("InjectedRuntime(without context bridge) error = nil")
	}
	if session, err := app.EndViewSession(context.Background()); err != nil || session.Active {
		t.Fatalf("EndViewSession(without active session) = %+v, err = %v", session, err)
	}
	if session, err := app.GetViewSession(context.Background()); err != nil || session.Active {
		t.Fatalf("GetViewSession(empty) = %+v, err = %v", session, err)
	}
}

func TestAppBridgeNormalizesUnknownView(t *testing.T) {
	root := t.TempDir()
	paths := appconfig.NewPaths(root)
	store := &contextstore.Store{Root: paths.ContextDir}
	if err := store.Create("local", "127.0.0.1:9820"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	state := appconfig.StateStore{File: paths.StateFile}
	app := &AppBridge{
		Paths:   paths,
		State:   state,
		Context: NewContextBridge(store, state),
	}
	session, err := app.StartViewSession(context.Background(), StartViewSessionRequest{ContextName: "local", View: "bad-view"})
	if err != nil {
		t.Fatalf("StartViewSession() error = %v", err)
	}
	if session.View != appconfig.DefaultView {
		t.Fatalf("View = %q, want %q", session.View, appconfig.DefaultView)
	}
	if views := ListViews(); len(views) != 2 || views[0].ID != "admin" || views[1].ID != "play" {
		t.Fatalf("ListViews() = %+v", views)
	}
}
