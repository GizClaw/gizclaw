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
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	root := t.TempDir()
	store := &contextstore.Store{Root: filepath.Join(root, "contexts")}
	if err := store.CreateWithOptions("local", "127.0.0.1:9820", contextstore.CreateOptions{
		Description:     "Local context",
		ServerPublicKey: serverKey.Public.String(),
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

	runtime, err := bridge.SelectContext(context.Background(), "local")
	if err != nil {
		t.Fatalf("SelectContext() error = %v", err)
	}
	if runtime.Context == nil || !runtime.Context.Current || runtime.Context.Endpoint != "127.0.0.1:9820" {
		t.Fatalf("RuntimeContext.Context = %+v", runtime.Context)
	}
	if runtime.SignalingURL != "http://127.0.0.1:9820/webrtc/v1/offer" {
		t.Fatalf("SignalingURL = %q", runtime.SignalingURL)
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
	if state.SelectedContext != "local" {
		t.Fatalf("SelectedContext = %q", state.SelectedContext)
	}
}

func TestContextBridgeCreateContext(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	root := t.TempDir()
	paths := appconfig.NewPaths(root)
	bridge := NewContextBridge(&contextstore.Store{Root: paths.ContextDir}, appconfig.StateStore{File: paths.StateFile})

	runtime, err := bridge.CreateContext(context.Background(), CreateContextRequest{
		Description:     "Dev server",
		Endpoint:        "127.0.0.1:9820",
		Name:            "dev",
		ServerPublicKey: serverKey.Public.String(),
	})
	if err != nil {
		t.Fatalf("CreateContext() error = %v", err)
	}
	if runtime.Context == nil || runtime.Context.Name != "dev" || runtime.Context.Description != "Dev server" {
		t.Fatalf("RuntimeContext = %+v", runtime.Context)
	}
}

func TestAppBridgeBootstrap(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	root := t.TempDir()
	paths := appconfig.NewPaths(root)
	store := &contextstore.Store{Root: paths.ContextDir}
	if err := store.Create("local", "127.0.0.1:9820", serverKey.Public.String()); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := store.Use("local"); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	state := appconfig.StateStore{File: paths.StateFile}
	if err := state.Save(appconfig.State{SelectedView: "admin"}); err != nil {
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
	if len(bootstrap.Contexts) != 1 || bootstrap.Runtime.Context == nil {
		t.Fatalf("Bootstrap() = %+v", bootstrap)
	}
	if bootstrap.State.SelectedView != "admin" {
		t.Fatalf("SelectedView = %q", bootstrap.State.SelectedView)
	}

	next, err := app.SetSelectedView(context.Background(), "play")
	if err != nil {
		t.Fatalf("SetSelectedView() error = %v", err)
	}
	if next.SelectedView != "play" {
		t.Fatalf("SelectedView = %q", next.SelectedView)
	}
}
