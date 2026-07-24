package main

import (
	"context"
	"fmt"
	"io/fs"
	"os/exec"
	goruntime "runtime"
	"sync"
	"time"

	appmessages "github.com/GizClaw/gizclaw-go/apps/wails/i18n"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/appconfig"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/bridge"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/endpointhealth"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/localserver"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/tray"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/webui"
	desktopresources "github.com/GizClaw/gizclaw-go/apps/wails/resources"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	bridge       *bridge.PodBridge
	ctx          context.Context
	tray         *tray.Manager
	mu           sync.RWMutex
	quitting     bool
	shutdownOnce sync.Once
	messages     appmessages.Catalog
}

func NewApp() (*App, error) {
	paths, err := appconfig.DefaultPaths()
	if err != nil {
		return nil, err
	}
	dist, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		return nil, fmt.Errorf("desktop app: frontend assets: %w", err)
	}
	return NewAppWithPathsAndAssets(paths, dist)
}

func NewAppWithPaths(paths appconfig.Paths) (*App, error) {
	dist, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		return nil, err
	}
	return NewAppWithPathsAndAssets(paths, dist)
}

func NewAppWithPathsAndAssets(paths appconfig.Paths, assets fs.FS) (*App, error) {
	if err := paths.Ensure(); err != nil {
		return nil, err
	}
	store := appconfig.Store{Paths: paths}
	local := localserver.New()
	health := endpointhealth.New()
	recovery := &bridge.PodBridge{Paths: paths, Store: store, Health: health, Local: local}
	if err := stopInterruptedLocalServers(context.Background(), store, recovery); err != nil {
		return nil, err
	}
	if err := store.CleanupIncomplete(); err != nil {
		return nil, err
	}
	catalogFS, err := desktopresources.LocalServer()
	if err != nil {
		return nil, fmt.Errorf("desktop app: local server resources: %w", err)
	}
	resolver, err := localserver.NewRaidsResolver(catalogFS, paths.RaidsCacheDir)
	if err != nil {
		return nil, fmt.Errorf("desktop app: local Raids assets: %w", err)
	}
	messages := appmessages.System()
	app := &App{messages: messages, bridge: &bridge.PodBridge{
		Paths:                paths,
		Store:                store,
		BootstrapEnvironment: appconfig.BootstrapEnvironmentStore{Path: paths.BootstrapEnvFile},
		CatalogResolver:      resolver,
		Health:               health,
		Local:                local,
		WebUI:                webui.New(assets),
	}}
	app.bridge.Bootstrapper = &localserver.Bootstrapper{Resolver: resolver, Executable: local.ExecutablePath}
	if err := app.bridge.RecoverLocalServers(context.Background()); err != nil {
		return nil, fmt.Errorf("desktop app: recover local servers: %w", err)
	}
	app.tray = tray.New(
		tray.Callbacks{OpenWindow: app.openWindow, OpenPod: app.openPod, Quit: app.quit},
		tray.Labels{OpenWindow: messages.Text("openWindow"), Quit: messages.Text("quit")},
	)
	return app, nil
}

func stopInterruptedLocalServers(ctx context.Context, store appconfig.Store, recovery *bridge.PodBridge) error {
	entries, err := store.Entries()
	if err != nil {
		return fmt.Errorf("desktop app: inspect interrupted local servers: %w", err)
	}
	for _, entry := range entries {
		if entry.Err != nil || entry.Pod.LocalServer == nil {
			continue
		}
		initialization, err := store.Initialization(entry.Pod.ID)
		if err != nil {
			return fmt.Errorf("desktop app: inspect interrupted local server %q: %w", entry.Pod.ID, err)
		}
		if initialization == nil || initialization.State != "initializing" {
			continue
		}
		status, err := recovery.RecoverLocalServer(ctx, entry.Pod.ID)
		if err != nil {
			return fmt.Errorf("desktop app: verify interrupted local server %q before cleanup: %w", entry.Pod.ID, err)
		}
		if status.State != "running" {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err = recovery.Local.Stop(ctx, entry.Pod.ID)
		cancel()
		if err != nil {
			return fmt.Errorf("desktop app: stop interrupted local server %q: %w", entry.Pod.ID, err)
		}
	}
	return nil
}

func (a *App) startup(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	a.mu.Unlock()
	a.syncTray(true)
}

func (a *App) shutdown(context.Context) {
	if a == nil || a.bridge == nil {
		return
	}
	a.shutdownOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		a.bridge.ShutdownInitializations(ctx)
		a.bridge.Local.Shutdown(ctx)
		a.bridge.WebUI.Shutdown()
		if a.tray != nil {
			a.tray.Stop()
		}
	})
}

func (a *App) beforeClose(ctx context.Context) bool {
	a.mu.RLock()
	quitting := a.quitting
	a.mu.RUnlock()
	if quitting {
		return false
	}
	runtime.WindowHide(ctx)
	return true
}

func (a *App) openWindow() {
	ctx := a.runtimeContext()
	if ctx == nil {
		return
	}
	runtime.WindowShow(ctx)
	runtime.WindowUnminimise(ctx)
}

func (a *App) openPod(id string) {
	a.openWindow()
	if ctx := a.runtimeContext(); ctx != nil {
		runtime.EventsEmit(ctx, "desktop:open-pod", id)
	}
}

func (a *App) quit() {
	a.mu.Lock()
	a.quitting = true
	a.mu.Unlock()
	a.shutdown(context.Background())
	if ctx := a.runtimeContext(); ctx != nil {
		runtime.Quit(ctx)
	}
}

func (a *App) runtimeContext() context.Context {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.ctx
}

func (a *App) syncTray(start bool) {
	if a.tray == nil || a.bridge == nil {
		return
	}
	pods, err := a.bridge.ListPods(context.Background())
	if err != nil {
		return
	}
	localItems := make([]tray.Pod, 0, len(pods))
	remoteItems := make([]tray.Pod, 0, len(pods))
	invalidItems := make([]tray.Pod, 0, len(pods))
	for _, pod := range pods {
		if !pod.Valid {
			invalidItems = append(invalidItems, tray.Pod{ID: pod.ID, Label: pod.Name, Section: a.messages.Text("invalid")})
		} else if pod.Remote != nil {
			remoteItems = append(remoteItems, tray.Pod{
				ID:      pod.ID,
				Label:   fmt.Sprintf("%s · %d %s", pod.Name, len(pod.Remote.Servers), a.messages.Text("servers")),
				Section: a.messages.Text("remote"),
			})
		} else {
			localItems = append(localItems, tray.Pod{ID: pod.ID, Label: pod.Name, Section: a.messages.Text("local")})
		}
	}
	items := append(localItems, remoteItems...)
	items = append(items, invalidItems...)
	if start {
		a.tray.Start(items)
	} else {
		a.tray.Update(items)
	}
}

func (a *App) Bootstrap() (bridge.BootstrapState, error) {
	if a == nil || a.bridge == nil {
		return bridge.BootstrapState{}, fmt.Errorf("desktop app: bridge is not configured")
	}
	state, err := a.bridge.Bootstrap(context.Background())
	if err != nil {
		return bridge.BootstrapState{}, err
	}
	state.Locale = a.messages.Locale()
	return state, nil
}

func (a *App) ListPods() ([]bridge.PodSummary, error) {
	if a == nil || a.bridge == nil {
		return nil, fmt.Errorf("desktop app: bridge is not configured")
	}
	return a.bridge.ListPods(context.Background())
}

func (a *App) GetPod(id string) (bridge.PodSummary, error) {
	if a == nil || a.bridge == nil {
		return bridge.PodSummary{}, fmt.Errorf("desktop app: bridge is not configured")
	}
	return a.bridge.GetPod(context.Background(), id)
}

func (a *App) CreatePod(input bridge.PodInput) (bridge.PodSummary, error) {
	if a == nil || a.bridge == nil {
		return bridge.PodSummary{}, fmt.Errorf("desktop app: bridge is not configured")
	}
	pod, err := a.bridge.CreatePod(context.Background(), input)
	if err == nil {
		a.syncTray(false)
	}
	return pod, err
}

func (a *App) GetBootstrapEnvironment() (bridge.BootstrapEnvironmentState, error) {
	if a == nil || a.bridge == nil {
		return bridge.BootstrapEnvironmentState{}, fmt.Errorf("desktop app: bridge is not configured")
	}
	return a.bridge.GetBootstrapEnvironment(context.Background())
}

func (a *App) UpdateBootstrapEnvironment(update bridge.BootstrapEnvironmentUpdate) (bridge.BootstrapEnvironmentState, error) {
	if a == nil || a.bridge == nil {
		return bridge.BootstrapEnvironmentState{}, fmt.Errorf("desktop app: bridge is not configured")
	}
	return a.bridge.UpdateBootstrapEnvironment(context.Background(), update)
}

func (a *App) UpdatePod(input bridge.PodInput) (bridge.PodSummary, error) {
	if a == nil || a.bridge == nil {
		return bridge.PodSummary{}, fmt.Errorf("desktop app: bridge is not configured")
	}
	pod, err := a.bridge.UpdatePod(context.Background(), input)
	if err == nil {
		a.syncTray(false)
	}
	return pod, err
}

func (a *App) DeletePod(id string) error {
	if a == nil || a.bridge == nil {
		return fmt.Errorf("desktop app: bridge is not configured")
	}
	err := a.bridge.DeletePod(context.Background(), id)
	if err == nil {
		a.syncTray(false)
	}
	return err
}

func (a *App) RefreshPodHealth(id string) (bridge.PodSummary, error) {
	if a == nil || a.bridge == nil {
		return bridge.PodSummary{}, fmt.Errorf("desktop app: bridge is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return a.bridge.RefreshHealth(ctx, id)
}

func (a *App) RevealPod(id string) error {
	if a == nil || a.bridge == nil {
		return fmt.Errorf("desktop app: bridge is not configured")
	}
	path, err := a.bridge.RevealPath(id)
	if err != nil {
		return err
	}
	if ctx := a.runtimeContext(); ctx != nil {
		return revealPath(path)
	}
	return nil
}

func revealPath(path string) error {
	name, args := revealCommandForOS(path, goruntime.GOOS)
	return exec.Command(name, args...).Start()
}

func revealCommandForOS(path, goos string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{path}
	case "windows":
		return "explorer", []string{path}
	default:
		return "xdg-open", []string{path}
	}
}

func (a *App) StartLocalServer(id string) (bridge.PodSummary, error) {
	if a == nil || a.bridge == nil {
		return bridge.PodSummary{}, fmt.Errorf("desktop app: bridge is not configured")
	}
	return a.bridge.StartLocal(context.Background(), id)
}

func (a *App) StopLocalServer(id string) (bridge.PodSummary, error) {
	if a == nil || a.bridge == nil {
		return bridge.PodSummary{}, fmt.Errorf("desktop app: bridge is not configured")
	}
	return a.bridge.StopLocal(context.Background(), id)
}

func (a *App) RestartLocalServer(id string) (bridge.PodSummary, error) {
	if a == nil || a.bridge == nil {
		return bridge.PodSummary{}, fmt.Errorf("desktop app: bridge is not configured")
	}
	return a.bridge.RestartLocal(context.Background(), id)
}

func (a *App) OpenAdmin(podID, serverID string) (string, error) {
	if a == nil || a.bridge == nil {
		return "", fmt.Errorf("desktop app: bridge is not configured")
	}
	return a.bridge.AdminURL(context.Background(), podID, serverID)
}

func (a *App) OpenPlay(podID string) (string, error) {
	if a == nil || a.bridge == nil {
		return "", fmt.Errorf("desktop app: bridge is not configured")
	}
	return a.bridge.PlayURL(context.Background(), podID)
}
