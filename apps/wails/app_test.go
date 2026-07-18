package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/apps/wails/internal/appconfig"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/bridge"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/localserver"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/webui"
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

func TestNewAppRecoversLocalServerFromWorkspacePID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test helper is a POSIX shell script")
	}
	paths := appconfig.NewPaths(t.TempDir())
	seed, err := NewAppWithPaths(paths)
	if err != nil {
		t.Fatal(err)
	}
	seed.bridge.Bootstrapper = nil
	created, err := seed.CreatePod(bridge.PodInput{Version: 1, Name: "Recovered", LocalServer: &bridge.LocalServerInput{}})
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "gizclaw")
	script := "#!/bin/sh\ntrap 'exit 0' INT TERM\nwhile :; do sleep 1; done\n"
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	seed.bridge.Local.Executable = executable
	workspace := filepath.Join(paths.PodsDir, created.ID, "workspace")
	started, err := seed.bridge.Local.Start(created.ID, workspace)
	if err != nil {
		t.Fatal(err)
	}
	startLocalServerInfo(t, seed.bridge.Store, created.ID, created.Local.Port)

	restarted, err := NewAppWithPaths(paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		restarted.bridge.Local.Shutdown(ctx)
	})
	recovered := restarted.bridge.Local.Status(created.ID)
	if recovered.State != "running" || recovered.PID != started.PID {
		t.Fatalf("recovered process = %+v, want PID %d", recovered, started.PID)
	}
}

func TestNewAppStopsServerBeforeCleaningInterruptedPod(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test helper is a POSIX shell script")
	}
	paths := appconfig.NewPaths(t.TempDir())
	seed, err := NewAppWithPaths(paths)
	if err != nil {
		t.Fatal(err)
	}
	seed.bridge.Bootstrapper = nil
	created, err := seed.CreatePod(bridge.PodInput{Version: 1, Name: "Interrupted", LocalServer: &bridge.LocalServerInput{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := seed.bridge.Store.MarkInitializing(created.ID); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "gizclaw")
	script := "#!/bin/sh\ntrap 'exit 0' INT TERM\nwhile :; do sleep 1; done\n"
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	seed.bridge.Local.Executable = executable
	if _, err := seed.bridge.Local.Start(created.ID, filepath.Join(paths.PodsDir, created.ID, "workspace")); err != nil {
		t.Fatal(err)
	}
	startLocalServerInfo(t, seed.bridge.Store, created.ID, created.Local.Port)

	if _, err := NewAppWithPaths(paths); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(paths.PodsDir, created.ID)); !os.IsNotExist(err) {
		t.Fatalf("interrupted Pod directory error = %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for seed.bridge.Local.Status(created.ID).State == "running" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if status := seed.bridge.Local.Status(created.ID); status.State == "running" || status.PID != 0 {
		t.Fatalf("interrupted local server = %+v", status)
	}
}

func startLocalServerInfo(t *testing.T, store appconfig.Store, id string, port int) {
	t.Helper()
	publicKey, err := store.LocalServerPublicKey(id)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"endpoint":       fmt.Sprintf("127.0.0.1:%d", port),
			"protocol":       "gizclaw-webrtc",
			"public_key":     publicKey,
			"server_time":    time.Now().Unix(),
			"signaling_path": "/webrtc",
		})
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
}

func TestBootstrapKeepsMalformedPodVisible(t *testing.T) {
	paths := appconfig.NewPaths(t.TempDir())
	app, err := NewAppWithPaths(paths)
	if err != nil {
		t.Fatal(err)
	}
	app.bridge.Bootstrapper = nil
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
	app.bridge.Bootstrapper = nil
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
	if _, err := app.GetBootstrapEnvironment(); err == nil {
		t.Fatal("GetBootstrapEnvironment error = nil")
	}
	if _, err := app.UpdateBootstrapEnvironment(bridge.BootstrapEnvironmentUpdate{}); err == nil {
		t.Fatal("UpdateBootstrapEnvironment error = nil")
	}
	if _, err := app.OpenPlay("missing"); err == nil {
		t.Fatal("OpenPlay error = nil")
	}
	if err := app.RevealPod("missing"); err == nil {
		t.Fatal("RevealPod error = nil")
	}
}

func TestBootstrapEnvironmentFacadeReturnsEditableDotenvContent(t *testing.T) {
	app, err := NewAppWithPaths(appconfig.NewPaths(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	name := app.bridge.Catalog.Requirements[0].Name
	const secret = "must-not-cross-the-bridge"
	content := name + "=" + secret + "\n"
	state, err := app.UpdateBootstrapEnvironment(bridge.BootstrapEnvironmentUpdate{Content: content})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), secret) || state.Content != content || state.Variables[0].Value != secret {
		t.Fatalf("bootstrap state did not return editable content: %s", data)
	}
	if _, err := app.UpdateBootstrapEnvironment(bridge.BootstrapEnvironmentUpdate{Content: "UNKNOWN_PROVIDER_TOKEN=value\n"}); err == nil {
		t.Fatal("unknown bootstrap environment name was accepted")
	}
	if _, err := app.UpdateBootstrapEnvironment(bridge.BootstrapEnvironmentUpdate{Content: "NOT AN ASSIGNMENT\n"}); err == nil {
		t.Fatal("malformed bootstrap environment content was accepted")
	}
}

func TestFileURLForWindowsPath(t *testing.T) {
	if got := fileURLForOS(`C:\Users\gizclaw\pod`, "windows"); got != "file:///C:/Users/gizclaw/pod" {
		t.Fatalf("fileURLForOS() = %q", got)
	}
}

func TestQuitStopsLocalServerBeforeRuntimeExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test helper is a POSIX shell script")
	}
	dir := t.TempDir()
	executable := filepath.Join(dir, "gizclaw")
	script := "#!/bin/sh\ntrap 'exit 0' INT TERM\nwhile :; do sleep 1; done\n"
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	local := localserver.New()
	local.Executable = executable
	app := &App{bridge: &bridge.PodBridge{
		Local: local,
		WebUI: webui.New(os.DirFS(dir)),
	}}
	if _, err := local.Start("local-lab", filepath.Join(dir, "workspace")); err != nil {
		t.Fatal(err)
	}

	app.quit()
	if status := local.Status("local-lab"); status.State != "stopped" || status.PID != 0 {
		t.Fatalf("local server after quit() = %+v", status)
	}
	if !app.quitting {
		t.Fatal("quit() did not mark the app as quitting")
	}

	// Wails calls OnShutdown after runtime.Quit. The second cleanup must be safe.
	app.shutdown(context.Background())
}

func appconfigTestKey(t *testing.T, fill byte) string {
	t.Helper()
	var key [32]byte
	for i := range key {
		key[i] = fill
	}
	return testKeyString(t, key)
}
