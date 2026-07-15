package bridge

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/apps/wails/internal/appconfig"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/endpointhealth"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/localserver"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/webui"
)

type PodBridge struct {
	Paths  appconfig.Paths
	Store  appconfig.Store
	Health *endpointhealth.Prober
	Local  *localserver.Manager
	WebUI  *webui.Manager

	refreshMu sync.Mutex
	refreshes map[string]*podRefresh
}

type podRefresh struct{ cancel context.CancelFunc }

type BootstrapState struct {
	Pods []PodSummary `json:"pods"`
}

type PodSummary struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Description    string         `json:"description,omitempty"`
	Mode           string         `json:"mode"`
	Valid          bool           `json:"valid"`
	Error          string         `json:"error,omitempty"`
	PlayConfigured bool           `json:"play_configured"`
	Local          *LocalSummary  `json:"local,omitempty"`
	Remote         *RemoteSummary `json:"remote,omitempty"`
}

type LocalSummary struct {
	Port            int                   `json:"port"`
	LANAddresses    []string              `json:"lan_addresses"`
	AdminConfigured bool                  `json:"admin_configured"`
	Process         localserver.Status    `json:"process"`
	Health          endpointhealth.Result `json:"health"`
}

type RemoteSummary struct {
	AccessPoint endpointhealth.Result `json:"access_point"`
	Servers     []ServerSummary       `json:"servers"`
}

type ServerSummary struct {
	ID              string                `json:"id"`
	Name            string                `json:"name"`
	Endpoint        string                `json:"endpoint"`
	AdminConfigured bool                  `json:"admin_configured"`
	Health          endpointhealth.Result `json:"health"`
}

type PodInput struct {
	Version           int                 `json:"version"`
	ID                string              `json:"id"`
	Name              string              `json:"name"`
	Description       string              `json:"description,omitempty"`
	LocalServer       *LocalServerInput   `json:"local_server,omitempty"`
	RemoteServers     []RemoteServerInput `json:"remote_servers,omitempty"`
	RemoteAccessPoint string              `json:"remote_access_point,omitempty"`
	ClientPrivateKey  *string             `json:"client_private_key,omitempty"`
}

type LocalServerInput struct {
	Port            int     `json:"port"`
	AdminPrivateKey *string `json:"admin_private_key,omitempty"`
}

type RemoteServerInput struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Endpoint        string  `json:"endpoint"`
	AdminPrivateKey *string `json:"admin_private_key,omitempty"`
}

func (b *PodBridge) Bootstrap(ctx context.Context) (BootstrapState, error) {
	pods, err := b.ListPods(ctx)
	if err != nil {
		return BootstrapState{}, err
	}
	return BootstrapState{Pods: pods}, nil
}

func (b *PodBridge) ListPods(context.Context) ([]PodSummary, error) {
	entries, err := b.Store.Entries()
	if err != nil {
		return nil, err
	}
	out := make([]PodSummary, 0, len(entries))
	for _, entry := range entries {
		if entry.Err != nil {
			out = append(out, PodSummary{ID: entry.ID, Name: entry.ID, Mode: "invalid", Error: entry.Err.Error()})
			continue
		}
		out = append(out, b.summary(entry.Pod))
	}
	return out, nil
}

func (b *PodBridge) GetPod(_ context.Context, id string) (PodSummary, error) {
	pod, err := b.Store.Load(id)
	if err != nil {
		return PodSummary{}, err
	}
	return b.summary(pod), nil
}

func (b *PodBridge) RevealPath(id string) (string, error) { return b.Store.PodDir(id) }

func (b *PodBridge) CreatePod(_ context.Context, input PodInput) (PodSummary, error) {
	pod, err := b.inputToPod(input, nil)
	if err != nil {
		return PodSummary{}, err
	}
	if pod.LocalServer != nil {
		if pod.LocalServer.Port < 0 || pod.LocalServer.Port > 65535 {
			return PodSummary{}, fmt.Errorf("local_server.port must be between 0 and 65535 when creating a Pod")
		}
		usedPorts, usedErr := b.localPodPorts()
		if usedErr != nil {
			return PodSummary{}, usedErr
		}
		switch pod.LocalServer.Port {
		case 0, appconfig.DefaultPort:
			preferred := appconfig.DefaultPort
			if usedPorts[preferred] {
				preferred = 0
			}
			pod.LocalServer.Port, err = appconfig.FindAvailablePort(preferred)
			if err != nil {
				return PodSummary{}, err
			}
			for usedPorts[pod.LocalServer.Port] {
				pod.LocalServer.Port, err = appconfig.FindAvailablePort(0)
				if err != nil {
					return PodSummary{}, err
				}
			}
		default:
			if usedPorts[pod.LocalServer.Port] {
				return PodSummary{}, fmt.Errorf("desktop bridge: local server port %d is already assigned to another Pod", pod.LocalServer.Port)
			}
			if listenErr := appconfig.CheckPortAvailable(pod.LocalServer.Port); listenErr != nil {
				return PodSummary{}, fmt.Errorf("desktop bridge: local server port %d is already in use", pod.LocalServer.Port)
			}
		}
	}
	if err := pod.Validate(); err != nil {
		return PodSummary{}, err
	}
	dir := filepath.Join(b.Paths.PodsDir, pod.ID)
	if err := os.Mkdir(dir, 0o700); err != nil {
		if os.IsExist(err) {
			return PodSummary{}, fmt.Errorf("desktop bridge: pod %q already exists", pod.ID)
		}
		return PodSummary{}, fmt.Errorf("desktop bridge: reserve pod %q: %w", pod.ID, err)
	}
	if err := b.Store.Save(pod); err != nil {
		if cleanupErr := os.RemoveAll(dir); cleanupErr != nil {
			return PodSummary{}, fmt.Errorf("%w; cleanup new pod: %v", err, cleanupErr)
		}
		return PodSummary{}, err
	}
	return b.summary(pod), nil
}

func (b *PodBridge) localPodPorts() (map[int]bool, error) {
	entries, err := b.Store.Entries()
	if err != nil {
		return nil, err
	}
	ports := map[int]bool{}
	for _, entry := range entries {
		if entry.Err == nil && entry.Pod.LocalServer != nil {
			ports[entry.Pod.LocalServer.Port] = true
		}
	}
	return ports, nil
}

func (b *PodBridge) UpdatePod(ctx context.Context, input PodInput) (PodSummary, error) {
	existing, err := b.Store.Load(input.ID)
	if err != nil {
		return PodSummary{}, err
	}
	pod, err := b.inputToPod(input, &existing)
	if err != nil {
		return PodSummary{}, err
	}
	processRunning := b.Local.Status(pod.ID).State == "running"
	if existing.LocalServer != nil && pod.LocalServer == nil && processRunning {
		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if _, err := b.Local.Stop(stopCtx, pod.ID); err != nil {
			return PodSummary{}, err
		}
		processRunning = false
	}
	portChanged := pod.LocalServer != nil && (existing.LocalServer == nil || existing.LocalServer.Port != pod.LocalServer.Port)
	if portChanged {
		if processRunning {
			return PodSummary{}, fmt.Errorf("desktop bridge: stop the local server before changing its port")
		}
		usedPorts, usedErr := b.localPodPorts()
		if usedErr != nil {
			return PodSummary{}, usedErr
		}
		if existing.LocalServer != nil {
			delete(usedPorts, existing.LocalServer.Port)
		}
		if usedPorts[pod.LocalServer.Port] {
			return PodSummary{}, fmt.Errorf("desktop bridge: local server port %d is already assigned to another Pod", pod.LocalServer.Port)
		}
		if listenErr := appconfig.CheckPortAvailable(pod.LocalServer.Port); listenErr != nil {
			return PodSummary{}, fmt.Errorf("desktop bridge: local server port %d is already in use", pod.LocalServer.Port)
		}
	}
	if err := b.Store.Save(pod); err != nil {
		return PodSummary{}, err
	}
	credentialsChanged := existing.ClientPrivateKey != pod.ClientPrivateKey || (existing.LocalServer != nil && pod.LocalServer != nil && existing.LocalServer.AdminPrivateKey != pod.LocalServer.AdminPrivateKey)
	if processRunning && credentialsChanged {
		restartCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if _, err := b.Local.Restart(restartCtx, pod.ID, filepath.Join(b.Paths.PodsDir, pod.ID, "workspace")); err != nil {
			return PodSummary{}, fmt.Errorf("desktop bridge: configuration saved but local server restart failed: %w", err)
		}
	}
	b.WebUI.ClosePod(pod.ID)
	return b.summary(pod), nil
}

func (b *PodBridge) DeletePod(ctx context.Context, id string) error {
	stopCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, _ = b.Local.Stop(stopCtx, id)
	b.WebUI.ClosePod(id)
	return b.Store.Delete(id)
}

func (b *PodBridge) RefreshHealth(ctx context.Context, id string) (PodSummary, error) {
	pod, err := b.Store.Load(id)
	if err != nil {
		return PodSummary{}, err
	}
	endpoints := make([]string, 0, len(pod.RemoteServers)+1)
	if pod.LocalServer != nil {
		if b.Local.Status(id).State == "running" {
			endpoints = append(endpoints, fmt.Sprintf("127.0.0.1:%d", pod.LocalServer.Port))
		}
	} else {
		for _, server := range pod.RemoteServers {
			endpoints = append(endpoints, server.Endpoint)
		}
		endpoints = append(endpoints, pod.RemoteAccessPoint)
	}
	probeCtx, cancel := context.WithCancel(ctx)
	refresh := &podRefresh{cancel: cancel}
	b.refreshMu.Lock()
	if b.refreshes == nil {
		b.refreshes = map[string]*podRefresh{}
	}
	if previous := b.refreshes[id]; previous != nil {
		previous.cancel()
	}
	b.refreshes[id] = refresh
	b.refreshMu.Unlock()
	defer func() {
		cancel()
		b.refreshMu.Lock()
		if b.refreshes[id] == refresh {
			delete(b.refreshes, id)
		}
		b.refreshMu.Unlock()
	}()
	b.Health.ProbeAll(probeCtx, endpoints)
	return b.summary(pod), nil
}

func (b *PodBridge) StartLocal(_ context.Context, id string) (PodSummary, error) {
	pod, err := b.Store.Load(id)
	if err != nil {
		return PodSummary{}, err
	}
	if pod.LocalServer == nil {
		return PodSummary{}, fmt.Errorf("desktop bridge: pod %q is remote", id)
	}
	if b.Local.Status(id).State != "running" {
		if listenErr := appconfig.CheckPortAvailable(pod.LocalServer.Port); listenErr != nil {
			return PodSummary{}, fmt.Errorf("desktop bridge: local server port %d is already in use", pod.LocalServer.Port)
		}
	}
	if _, err := b.Local.Start(id, filepath.Join(b.Paths.PodsDir, id, "workspace")); err != nil {
		return PodSummary{}, err
	}
	return b.summary(pod), nil
}

func (b *PodBridge) StopLocal(ctx context.Context, id string) (PodSummary, error) {
	pod, err := b.Store.Load(id)
	if err != nil {
		return PodSummary{}, err
	}
	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := b.Local.Stop(stopCtx, id); err != nil {
		return PodSummary{}, err
	}
	return b.summary(pod), nil
}

func (b *PodBridge) RestartLocal(ctx context.Context, id string) (PodSummary, error) {
	pod, err := b.Store.Load(id)
	if err != nil {
		return PodSummary{}, err
	}
	if pod.LocalServer == nil {
		return PodSummary{}, fmt.Errorf("desktop bridge: pod %q is remote", id)
	}
	restartCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := b.Local.Stop(restartCtx, id); err != nil {
		return PodSummary{}, err
	}
	if listenErr := appconfig.CheckPortAvailable(pod.LocalServer.Port); listenErr != nil {
		return PodSummary{}, fmt.Errorf("desktop bridge: local server port %d is already in use", pod.LocalServer.Port)
	}
	if _, err := b.Local.Start(id, filepath.Join(b.Paths.PodsDir, id, "workspace")); err != nil {
		return PodSummary{}, err
	}
	return b.summary(pod), nil
}

func (b *PodBridge) AdminURL(_ context.Context, podID, serverID string) (string, error) {
	pod, err := b.Store.Load(podID)
	if err != nil {
		return "", err
	}
	name, endpoint, privateKey := "", "", ""
	if pod.LocalServer != nil {
		name = pod.Name
		endpoint = fmt.Sprintf("127.0.0.1:%d", pod.LocalServer.Port)
		privateKey = pod.LocalServer.AdminPrivateKey
	} else {
		for _, server := range pod.RemoteServers {
			if server.ID == serverID {
				name, endpoint, privateKey = server.Name, server.Endpoint, server.AdminPrivateKey
				break
			}
		}
	}
	if privateKey == "" {
		return "", fmt.Errorf("desktop bridge: Admin is not configured for this server")
	}
	runtime, err := webui.RuntimeFromPrivateKey(name, pod.Description, endpoint, privateKey)
	if err != nil {
		return "", err
	}
	if pod.LocalServer != nil {
		runtime.AdminServerID = "local"
		runtime.AdminServers = []webui.AdminServerRuntime{{ID: "local", Name: pod.Name, Context: runtime.Context, PrivateKeyBase64: runtime.PrivateKeyBase64}}
	} else {
		runtime.AdminServerID = serverID
		for _, server := range pod.RemoteServers {
			if server.AdminPrivateKey == "" {
				continue
			}
			option, optionErr := webui.RuntimeFromPrivateKey(server.Name, pod.Description, server.Endpoint, server.AdminPrivateKey)
			if optionErr != nil {
				return "", optionErr
			}
			runtime.AdminServers = append(runtime.AdminServers, webui.AdminServerRuntime{ID: server.ID, Name: server.Name, Context: option.Context, PrivateKeyBase64: option.PrivateKeyBase64})
		}
	}
	return b.WebUI.LaunchURL(podID, "admin", runtime)
}

func (b *PodBridge) PlayURL(_ context.Context, podID string) (string, error) {
	pod, err := b.Store.Load(podID)
	if err != nil {
		return "", err
	}
	if pod.ClientPrivateKey == "" {
		return "", fmt.Errorf("desktop bridge: Play is not configured for this pod")
	}
	endpoint := pod.RemoteAccessPoint
	if pod.LocalServer != nil {
		endpoint = fmt.Sprintf("127.0.0.1:%d", pod.LocalServer.Port)
	}
	runtime, err := webui.RuntimeFromPrivateKey(pod.Name, pod.Description, endpoint, pod.ClientPrivateKey)
	if err != nil {
		return "", err
	}
	return b.WebUI.LaunchURL(podID, "play", runtime)
}

func (b *PodBridge) summary(pod appconfig.Pod) PodSummary {
	summary := PodSummary{ID: pod.ID, Name: pod.Name, Description: pod.Description, PlayConfigured: pod.ClientPrivateKey != "", Valid: true}
	if pod.LocalServer != nil {
		endpoint := fmt.Sprintf("127.0.0.1:%d", pod.LocalServer.Port)
		summary.Mode = "local"
		summary.Local = &LocalSummary{Port: pod.LocalServer.Port, LANAddresses: lanAddresses(pod.LocalServer.Port), AdminConfigured: pod.LocalServer.AdminPrivateKey != "", Process: b.Local.Status(pod.ID), Health: b.Health.Get(endpoint)}
		return summary
	}
	summary.Mode = "remote"
	remote := &RemoteSummary{AccessPoint: b.Health.Get(pod.RemoteAccessPoint), Servers: make([]ServerSummary, 0, len(pod.RemoteServers))}
	for _, server := range pod.RemoteServers {
		remote.Servers = append(remote.Servers, ServerSummary{ID: server.ID, Name: server.Name, Endpoint: server.Endpoint, AdminConfigured: server.AdminPrivateKey != "", Health: b.Health.Get(server.Endpoint)})
	}
	summary.Remote = remote
	return summary
}

func lanAddresses(port int) []string {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(addresses))
	for _, address := range addresses {
		ip, _, err := net.ParseCIDR(address.String())
		if err != nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() {
			continue
		}
		value := net.JoinHostPort(ip.String(), fmt.Sprint(port))
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func (b *PodBridge) inputToPod(input PodInput, existing *appconfig.Pod) (appconfig.Pod, error) {
	pod := appconfig.Pod{Version: input.Version, ID: strings.TrimSpace(input.ID), Name: strings.TrimSpace(input.Name), Description: strings.TrimSpace(input.Description), RemoteAccessPoint: strings.TrimSpace(input.RemoteAccessPoint)}
	if pod.Version == 0 {
		pod.Version = appconfig.PodVersion
	}
	if input.LocalServer != nil {
		key := secretValue(input.LocalServer.AdminPrivateKey, "")
		if existing != nil && existing.LocalServer != nil {
			key = secretValue(input.LocalServer.AdminPrivateKey, existing.LocalServer.AdminPrivateKey)
		}
		pod.LocalServer = &appconfig.LocalServer{Port: input.LocalServer.Port, AdminPrivateKey: key}
	}
	for _, server := range input.RemoteServers {
		oldKey := ""
		if existing != nil {
			for _, current := range existing.RemoteServers {
				if current.ID == server.ID {
					oldKey = current.AdminPrivateKey
				}
			}
		}
		pod.RemoteServers = append(pod.RemoteServers, appconfig.RemoteServer{ID: strings.TrimSpace(server.ID), Name: strings.TrimSpace(server.Name), Endpoint: strings.TrimSpace(server.Endpoint), AdminPrivateKey: secretValue(server.AdminPrivateKey, oldKey)})
	}
	oldClient := ""
	if existing != nil {
		oldClient = existing.ClientPrivateKey
	}
	pod.ClientPrivateKey = secretValue(input.ClientPrivateKey, oldClient)
	return pod, nil
}

func secretValue(input *string, existing string) string {
	if input == nil {
		return existing
	}
	return strings.TrimSpace(*input)
}
