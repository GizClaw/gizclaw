package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GizClaw/gizclaw-go/apps/wails/internal/appconfig"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/bridge"
)

func TestNewAppUsesConfiguredHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv(appconfig.EnvConfigHome, root)
	app, err := NewApp()
	if err != nil {
		t.Fatal(err)
	}
	if app == nil || app.bridge == nil || app.bridge.Paths.ConfigRoot != root {
		t.Fatalf("NewApp() = %#v", app)
	}
}

func TestBootstrapKeepsMalformedPodVisible(t *testing.T) {
	paths := appconfig.NewPaths(t.TempDir())
	app, err := NewAppWithPaths(paths)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.CreatePod(bridge.PodInput{Version: 1, ID: "healthy", Name: "Healthy", LocalServer: &bridge.LocalServerInput{Port: 19083}}); err != nil {
		t.Fatal(err)
	}
	badDir := filepath.Join(paths.PodsDir, "broken")
	if err := os.MkdirAll(badDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDir, appconfig.PodManifestFile), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := app.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if state.Locale == "" || len(state.Pods) != 2 || state.Pods[0].Valid == state.Pods[1].Valid {
		t.Fatalf("Bootstrap() = %+v", state)
	}
}

func TestAppPodFacadeNeverReturnsPrivateKeys(t *testing.T) {
	app, err := NewAppWithPaths(appconfig.NewPaths(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	admin := appconfigTestKey(t, 0x41)
	client := appconfigTestKey(t, 0x42)
	created, err := app.CreatePod(bridge.PodInput{
		Version:          1,
		ID:               "local-lab",
		Name:             "Local Lab",
		LocalServer:      &bridge.LocalServerInput{Port: 19082, AdminPrivateKey: &admin},
		ClientPrivateKey: &client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Local == nil || !created.Local.AdminConfigured || !created.PlayConfigured {
		t.Fatalf("created = %+v", created)
	}
	bootstrap, err := app.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if len(bootstrap.Pods) != 1 || bootstrap.Pods[0].ID != "local-lab" {
		t.Fatalf("bootstrap = %+v", bootstrap)
	}
}

func TestAppFacadeRequiresConfiguredBridge(t *testing.T) {
	var app *App
	if _, err := app.Bootstrap(); err == nil {
		t.Fatal("Bootstrap error = nil")
	}
	if _, err := app.ListPods(); err == nil {
		t.Fatal("ListPods error = nil")
	}
	if _, err := app.CreatePod(bridge.PodInput{}); err == nil {
		t.Fatal("CreatePod error = nil")
	}
	if _, err := app.OpenPlay("missing"); err == nil {
		t.Fatal("OpenPlay error = nil")
	}
	if err := app.RevealPod("missing"); err == nil {
		t.Fatal("RevealPod error = nil")
	}
}

func TestFileURLForWindowsPath(t *testing.T) {
	if got := fileURLForOS(`C:\Users\gizclaw\pod`, "windows"); got != "file:///C:/Users/gizclaw/pod" {
		t.Fatalf("fileURLForOS() = %q", got)
	}
}

func appconfigTestKey(t *testing.T, fill byte) string {
	t.Helper()
	var key [32]byte
	for i := range key {
		key[i] = fill
	}
	return testKeyString(t, key)
}
