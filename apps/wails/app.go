package main

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"sync"
	"time"

	appmessages "github.com/GizClaw/gizclaw-go/apps/wails/i18n"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/appconfig"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/bridge"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/endpointhealth"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/localserver"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/tray"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/webui"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	bridge   *bridge.PodBridge
	ctx      context.Context
	tray     *tray.Manager
	mu       sync.RWMutex
	quitting bool
	messages appmessages.Catalog
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
	messages := appmessages.System()
	app := &App{messages: messages, bridge: &bridge.PodBridge{
		Paths:  paths,
		Store:  appconfig.Store{Paths: paths},
		Health: endpointhealth.New(),
		Local:  localserver.New(),
		WebUI:  webui.New(assets),
	}}
	app.tray = tray.New(
		tray.Callbacks{OpenWindow: app.openWindow, OpenPod: app.openPod, Quit: app.quit},
		tray.Labels{OpenWindow: messages.Text("openWindow"), OpenPod: messages.Text("openPod"), Quit: messages.Text("quit")},
	)
	return app, nil
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a.bridge.Local.Shutdown(ctx)
	a.bridge.WebUI.Shutdown()
	if a.tray != nil {
		a.tray.Stop()
	}
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
	items := make([]tray.Pod, 0, len(pods))
	for _, pod := range pods {
		label := fmt.Sprintf("%s · %s", pod.Name, a.messages.Text("local"))
		if !pod.Valid {
			label = fmt.Sprintf("%s · %s", pod.Name, a.messages.Text("invalid"))
		} else if pod.Remote != nil {
			label = fmt.Sprintf("%s · %s · %d %s", pod.Name, a.messages.Text("remote"), len(pod.Remote.Servers), a.messages.Text("servers"))
		}
		items = append(items, tray.Pod{ID: pod.ID, Label: label})
	}
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
	return a.bridge.Bootstrap(context.Background())
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
		runtime.BrowserOpenURL(ctx, (&url.URL{Scheme: "file", Path: path}).String())
	}
	return nil
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

func (a *App) OpenAdmin(podID, serverID string) error {
	if a == nil || a.bridge == nil {
		return fmt.Errorf("desktop app: bridge is not configured")
	}
	url, err := a.bridge.AdminURL(context.Background(), podID, serverID)
	if err != nil {
		return err
	}
	if ctx := a.runtimeContext(); ctx != nil {
		runtime.BrowserOpenURL(ctx, url)
	}
	return nil
}

func (a *App) OpenPlay(podID string) error {
	if a == nil || a.bridge == nil {
		return fmt.Errorf("desktop app: bridge is not configured")
	}
	url, err := a.bridge.PlayURL(context.Background(), podID)
	if err != nil {
		return err
	}
	if ctx := a.runtimeContext(); ctx != nil {
		runtime.BrowserOpenURL(ctx, url)
	}
	return nil
}
