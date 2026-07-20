package bridge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
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
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type PodBridge struct {
	Paths                appconfig.Paths
	Store                appconfig.Store
	BootstrapEnvironment appconfig.BootstrapEnvironmentStore
	Catalog              *localserver.Catalog
	Bootstrapper         LocalPodBootstrapper
	WaitLocalReady       func(context.Context, string, int) error
	Health               *endpointhealth.Prober
	Local                *localserver.Manager
	WebUI                *webui.Manager

	mutationMu   sync.Mutex
	contractMu   sync.Mutex
	tokenMu      sync.Mutex
	creating     sync.Map
	initializing sync.Map
	refreshMu    sync.Mutex
	refreshes    map[string]*podRefresh
}

type LocalPodBootstrapper interface {
	Apply(context.Context, string, map[string]string) error
	MigrateRuntimeContract(context.Context, string) error
	RecoverRegistrationToken(context.Context, string, map[string]string) error
}

type podRefresh struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type podInitialization struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type BootstrapState struct {
	Locale               string                    `json:"locale"`
	Pods                 []PodSummary              `json:"pods"`
	BootstrapEnvironment BootstrapEnvironmentState `json:"bootstrap_environment"`
}

type BootstrapEnvironmentState struct {
	Ready     bool                                `json:"ready"`
	Missing   []string                            `json:"missing"`
	Content   string                              `json:"content"`
	Variables []BootstrapEnvironmentVariableState `json:"variables"`
	Error     string                              `json:"error,omitempty"`
}

type BootstrapEnvironmentVariableState struct {
	Name       string `json:"name"`
	Required   bool   `json:"required"`
	Configured bool   `json:"configured"`
	Defaulted  bool   `json:"defaulted"`
	Value      string `json:"value"`
}

type BootstrapEnvironmentUpdate struct {
	Content string `json:"content"`
}

type PodSummary struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	Description       string                 `json:"description,omitempty"`
	Mode              string                 `json:"mode"`
	Valid             bool                   `json:"valid"`
	Error             string                 `json:"error,omitempty"`
	Initialization    *InitializationSummary `json:"initialization,omitempty"`
	PlayConfigured    bool                   `json:"play_configured"`
	PlayPublicKey     string                 `json:"play_public_key,omitempty"`
	RegistrationToken string                 `json:"registration_token,omitempty"`
	Local             *LocalSummary          `json:"local,omitempty"`
	Remote            *RemoteSummary         `json:"remote,omitempty"`
}

type InitializationSummary struct {
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

type LocalSummary struct {
	Port            int                   `json:"port"`
	LANAddresses    []string              `json:"lan_addresses"`
	AdminConfigured bool                  `json:"admin_configured"`
	AdminPublicKey  string                `json:"admin_public_key,omitempty"`
	ServerPublicKey string                `json:"server_public_key,omitempty"`
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
	AdminPublicKey  string                `json:"admin_public_key,omitempty"`
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
	RegistrationToken *string             `json:"registration_token,omitempty"`
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
	environment, _, err := b.bootstrapEnvironmentState()
	if err != nil {
		return BootstrapState{}, err
	}
	return BootstrapState{Pods: pods, BootstrapEnvironment: environment}, nil
}

// RecoverLocalServers attaches process management to local Servers that
// survived a previous Desktop process. Invalid Pod manifests remain visible
// through ListPods and do not prevent recovery of other Pods.
func (b *PodBridge) RecoverLocalServers(ctx context.Context) error {
	entries, err := b.Store.Entries()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Err != nil || entry.Pod.LocalServer == nil {
			continue
		}
		status, err := b.recoverLocalServer(ctx, entry.Pod)
		if err != nil {
			endpoint := fmt.Sprintf("127.0.0.1:%d", entry.Pod.LocalServer.Port)
			b.Health.MarkUnreachable(endpoint, fmt.Sprintf("local server recovery failed: %v", err))
			continue
		}
		if status.State == "running" {
			pod := entry.Pod
			if pod.LocalCatalogVersion < appconfig.LocalCatalogVersion {
				restartCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				_, restartErr := b.Local.Restart(restartCtx, pod.ID, filepath.Join(b.Paths.PodsDir, pod.ID, "workspace"))
				cancel()
				if restartErr != nil {
					endpoint := fmt.Sprintf("127.0.0.1:%d", pod.LocalServer.Port)
					b.Health.MarkUnreachable(endpoint, fmt.Sprintf("local server upgrade restart failed: %v", restartErr))
					continue
				}
			}
			if _, err := b.ensureLocalRuntimeContract(ctx, pod); err != nil {
				endpoint := fmt.Sprintf("127.0.0.1:%d", entry.Pod.LocalServer.Port)
				b.Health.MarkUnreachable(endpoint, fmt.Sprintf("local runtime migration failed: %v", err))
			}
		}
	}
	return nil
}

// RecoverLocalServer verifies and attaches process management to one local
// Server that survived a previous Desktop process.
func (b *PodBridge) RecoverLocalServer(ctx context.Context, id string) (localserver.Status, error) {
	pod, err := b.Store.Load(id)
	if err != nil {
		return localserver.Status{}, err
	}
	if pod.LocalServer == nil {
		return localserver.Status{}, fmt.Errorf("desktop bridge: pod %q is remote", id)
	}
	return b.recoverLocalServer(ctx, pod)
}

func (b *PodBridge) recoverLocalServer(ctx context.Context, pod appconfig.Pod) (localserver.Status, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}
	workspace := filepath.Join(b.Paths.PodsDir, pod.ID, "workspace")
	return b.Local.Recover(pod.ID, workspace, func(pid int) error {
		expectedPublicKey, err := b.Store.LocalServerPublicKey(pod.ID)
		if err != nil {
			return err
		}
		endpoint := fmt.Sprintf("127.0.0.1:%d", pod.LocalServer.Port)
		for {
			result := b.Health.Probe(ctx, endpoint)
			if result.State == endpointhealth.Reachable {
				if result.PublicKey != expectedPublicKey {
					return fmt.Errorf("PID %d server identity does not match Pod %q: %w", pid, pod.ID, localserver.ErrProcessIdentityMismatch)
				}
				return nil
			}
			select {
			case <-ctx.Done():
				return fmt.Errorf("PID %d server-info verification timed out (%s): %w", pid, result.Message, ctx.Err())
			case <-time.After(100 * time.Millisecond):
			}
		}
	})
}

func (b *PodBridge) recoverLocalServerForMutation(ctx context.Context, pod appconfig.Pod) error {
	if pod.LocalServer == nil || b.Local.Status(pod.ID).State == "running" {
		return nil
	}
	if _, err := b.recoverLocalServer(ctx, pod); err != nil {
		return fmt.Errorf("desktop bridge: verify local server before lifecycle operation: %w", err)
	}
	return nil
}

func (b *PodBridge) GetBootstrapEnvironment(context.Context) (BootstrapEnvironmentState, error) {
	state, _, err := b.bootstrapEnvironmentState()
	return state, err
}

func (b *PodBridge) UpdateBootstrapEnvironment(_ context.Context, update BootstrapEnvironmentUpdate) (BootstrapEnvironmentState, error) {
	b.mutationMu.Lock()
	defer b.mutationMu.Unlock()
	values, err := appconfig.ParseBootstrapEnvironment(update.Content)
	if err != nil {
		return BootstrapEnvironmentState{}, err
	}
	allowed := map[string]bool{}
	if b.Catalog == nil {
		return BootstrapEnvironmentState{}, fmt.Errorf("desktop bridge: bootstrap catalog is not configured")
	}
	for _, requirement := range b.Catalog.Requirements {
		allowed[requirement.Name] = true
	}
	for name := range values {
		if !allowed[name] {
			return BootstrapEnvironmentState{}, fmt.Errorf("desktop bridge: bootstrap environment %q is not used by the catalog", name)
		}
	}
	if err := b.BootstrapEnvironment.Replace(update.Content); err != nil {
		return BootstrapEnvironmentState{}, err
	}
	state, _, err := b.bootstrapEnvironmentState()
	return state, err
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
		pod := entry.Pod
		changed, identityErr := ensurePodIdentities(&pod)
		if identityErr != nil {
			return nil, identityErr
		}
		if changed {
			if saveErr := b.Store.Save(pod); saveErr != nil {
				return nil, saveErr
			}
		}
		out = append(out, b.summary(pod))
	}
	return out, nil
}

func (b *PodBridge) GetPod(_ context.Context, id string) (PodSummary, error) {
	pod, err := b.Store.Load(id)
	if err != nil {
		return PodSummary{}, err
	}
	changed, err := ensurePodIdentities(&pod)
	if err != nil {
		return PodSummary{}, err
	}
	if changed {
		if err := b.Store.Save(pod); err != nil {
			return PodSummary{}, err
		}
	}
	return b.summary(pod), nil
}

func (b *PodBridge) RevealPath(id string) (string, error) { return b.Store.PodDir(id) }

func (b *PodBridge) CreatePod(_ context.Context, input PodInput) (PodSummary, error) {
	input.ID = strings.TrimSpace(input.ID)
	if input.ID == "" {
		input.ID = newInternalID("pod")
	}
	if _, loaded := b.creating.LoadOrStore(input.ID, struct{}{}); loaded {
		return PodSummary{}, fmt.Errorf("desktop bridge: Pod %q creation is already in progress", input.ID)
	}
	defer b.creating.Delete(input.ID)
	b.mutationMu.Lock()
	defer b.mutationMu.Unlock()
	var savedEnvironment map[string]string
	if input.LocalServer != nil && b.Bootstrapper != nil {
		state, saved, err := b.bootstrapEnvironmentState()
		if err != nil {
			return PodSummary{}, err
		}
		if !state.Ready {
			if state.Error != "" {
				return PodSummary{}, fmt.Errorf("desktop bridge: configure bootstrap environment: %s", state.Error)
			}
			return PodSummary{}, fmt.Errorf("desktop bridge: configure bootstrap environment: %s", strings.Join(state.Missing, ", "))
		}
		savedEnvironment = saved
	}
	pod, err := b.inputToPod(input, nil)
	if err != nil {
		return PodSummary{}, err
	}
	if _, err := ensurePodIdentities(&pod); err != nil {
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
	initializing := pod.LocalServer != nil && b.Bootstrapper != nil
	if initializing {
		if err := b.Store.MarkInitializing(pod.ID); err != nil {
			_ = os.RemoveAll(dir)
			return PodSummary{}, err
		}
	}
	if err := b.Store.Save(pod); err != nil {
		if cleanupErr := os.RemoveAll(dir); cleanupErr != nil {
			return PodSummary{}, fmt.Errorf("%w; cleanup new pod: %v", err, cleanupErr)
		}
		return PodSummary{}, err
	}
	if initializing {
		b.startLocalInitialization(pod, dir, savedEnvironment)
	}
	return b.summary(pod), nil
}

func (b *PodBridge) startLocalInitialization(pod appconfig.Pod, dir string, savedEnvironment map[string]string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	task := &podInitialization{cancel: cancel, done: make(chan struct{})}
	b.initializing.Store(pod.ID, task)
	go func() {
		defer close(task.done)
		defer cancel()
		defer b.initializing.Delete(pod.ID)
		err := b.initializeLocalPod(ctx, pod, dir, savedEnvironment)
		if err == nil {
			return
		}
		stopCtx, stopCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		_, stopErr := b.Local.Stop(stopCtx, pod.ID)
		stopCancel()
		b.WebUI.ClosePod(pod.ID)
		if errors.Is(ctx.Err(), context.Canceled) {
			return
		}
		if stopErr != nil {
			err = fmt.Errorf("%w; stop local server: %v", err, stopErr)
		}
		_ = b.Store.FailInitialization(pod.ID, err)
	}()
}

func (b *PodBridge) initializeLocalPod(ctx context.Context, pod appconfig.Pod, dir string, savedEnvironment map[string]string) error {
	if _, err := b.Local.Start(pod.ID, filepath.Join(dir, "workspace")); err != nil {
		return err
	}
	if err := b.waitLocalReady(ctx, pod.ID, pod.LocalServer.Port); err != nil {
		return err
	}
	if err := b.Bootstrapper.Apply(ctx, dir, savedEnvironment); err != nil {
		return err
	}
	pod.LocalCatalogVersion = appconfig.LocalCatalogVersion
	if err := b.Store.Save(pod); err != nil {
		return fmt.Errorf("desktop bridge: record local catalog version: %w", err)
	}
	if status := b.Local.Status(pod.ID); status.State != "running" {
		return errors.New("desktop bridge: local server exited during bootstrap")
	}
	return b.Store.CompleteInitialization(pod.ID)
}

func (b *PodBridge) bootstrapEnvironmentState() (BootstrapEnvironmentState, map[string]string, error) {
	if b.Catalog == nil {
		return BootstrapEnvironmentState{Ready: true, Missing: []string{}, Variables: []BootstrapEnvironmentVariableState{}}, map[string]string{}, nil
	}
	content, err := b.BootstrapEnvironment.Content()
	if err != nil {
		return BootstrapEnvironmentState{}, nil, err
	}
	saved, err := appconfig.ParseBootstrapEnvironment(content)
	if err != nil {
		return BootstrapEnvironmentState{
			Content:   content,
			Error:     err.Error(),
			Missing:   []string{},
			Variables: []BootstrapEnvironmentVariableState{},
		}, map[string]string{}, nil
	}
	state := BootstrapEnvironmentState{
		Ready:     true,
		Missing:   []string{},
		Content:   content,
		Variables: make([]BootstrapEnvironmentVariableState, 0, len(b.Catalog.Requirements)),
	}
	for _, requirement := range b.Catalog.Requirements {
		variable := BootstrapEnvironmentVariableState{Name: requirement.Name, Required: requirement.Default == nil, Value: saved[requirement.Name]}
		if saved[requirement.Name] != "" {
			variable.Configured = true
		} else if value, ok := os.LookupEnv(requirement.Name); ok && value != "" {
			variable.Configured = true
		} else if requirement.Default != nil {
			variable.Defaulted = true
		} else {
			state.Ready = false
			state.Missing = append(state.Missing, requirement.Name)
		}
		state.Variables = append(state.Variables, variable)
	}
	return state, saved, nil
}

func (b *PodBridge) waitLocalReady(ctx context.Context, podID string, port int) error {
	if b.WaitLocalReady != nil {
		return b.WaitLocalReady(ctx, podID, port)
	}
	endpoint := fmt.Sprintf("127.0.0.1:%d", port)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		status := b.Local.Status(podID)
		if status.State == "failed" || status.State == "stopped" {
			return fmt.Errorf("desktop bridge: local server exited before bootstrap readiness")
		}
		probeCtx, cancel := context.WithTimeout(ctx, 350*time.Millisecond)
		result := b.Health.Probe(probeCtx, endpoint)
		cancel()
		if result.State == endpointhealth.Reachable {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("desktop bridge: wait for local server readiness: %w", ctx.Err())
		case <-ticker.C:
		}
	}
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
	b.mutationMu.Lock()
	defer b.mutationMu.Unlock()
	if err := b.requireInitializationComplete(input.ID); err != nil {
		return PodSummary{}, err
	}
	existing, err := b.Store.Load(input.ID)
	if err != nil {
		return PodSummary{}, err
	}
	if err := b.recoverLocalServerForMutation(ctx, existing); err != nil {
		return PodSummary{}, err
	}
	if _, err := ensurePodIdentities(&existing); err != nil {
		return PodSummary{}, err
	}
	pod, err := b.inputToPod(input, &existing)
	if err != nil {
		return PodSummary{}, err
	}
	if err := pod.Validate(); err != nil {
		return PodSummary{}, err
	}
	processRunning := b.Local.Status(pod.ID).State == "running"
	if existing.LocalServer != nil && pod.LocalServer == nil && processRunning {
		return PodSummary{}, fmt.Errorf("desktop bridge: stop the local server before changing its mode")
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
	waitCtx, waitCancel := context.WithTimeout(ctx, 5*time.Second)
	defer waitCancel()
	if err := b.cancelInitialization(waitCtx, id); err != nil {
		return err
	}
	if pod, err := b.Store.Load(id); err == nil {
		if err := b.recoverLocalServerForMutation(ctx, pod); err != nil {
			return err
		}
	} else {
		pidPath := filepath.Join(b.Paths.PodsDir, id, "workspace", localserver.PIDFile)
		if _, statErr := os.Lstat(pidPath); statErr == nil {
			return fmt.Errorf("desktop bridge: cannot delete invalid Pod %q while a persisted PID requires verification", id)
		} else if !os.IsNotExist(statErr) {
			return fmt.Errorf("desktop bridge: inspect invalid Pod %q PID: %w", id, statErr)
		}
	}
	stopCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, _ = b.Local.Stop(stopCtx, id)
	b.WebUI.ClosePod(id)
	return b.Store.Delete(id)
}

func (b *PodBridge) cancelInitialization(ctx context.Context, id string) error {
	value, ok := b.initializing.Load(id)
	if !ok {
		return nil
	}
	task := value.(*podInitialization)
	task.cancel()
	select {
	case <-task.done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("desktop bridge: cancel Pod initialization: %w", ctx.Err())
	}
}

// ShutdownInitializations cancels background bootstrap work before Desktop
// stops its local Server processes.
func (b *PodBridge) ShutdownInitializations(ctx context.Context) {
	tasks := make([]*podInitialization, 0)
	b.initializing.Range(func(_, value any) bool {
		task := value.(*podInitialization)
		task.cancel()
		tasks = append(tasks, task)
		return true
	})
	for _, task := range tasks {
		select {
		case <-task.done:
		case <-ctx.Done():
			return
		}
	}
}

func (b *PodBridge) requireInitializationComplete(id string) error {
	initialization, err := b.Store.Initialization(id)
	if err != nil {
		return err
	}
	if initialization == nil {
		return nil
	}
	if initialization.State == "failed" {
		return fmt.Errorf("desktop bridge: Pod initialization failed: %s", initialization.Error)
	}
	return errors.New("desktop bridge: Pod initialization is still in progress")
}

func (b *PodBridge) RefreshHealth(ctx context.Context, id string) (PodSummary, error) {
	pod, err := b.Store.Load(id)
	if err != nil {
		return PodSummary{}, err
	}
	if initialization, initErr := b.Store.Initialization(id); initErr != nil || initialization != nil {
		return b.summary(pod), nil
	}
	endpoints := make([]string, 0, len(pod.RemoteServers)+1)
	if pod.LocalServer != nil {
		if b.Local.Status(id).State == "running" {
			endpoints = append(endpoints, fmt.Sprintf("127.0.0.1:%d", pod.LocalServer.Port))
		} else {
			b.Health.MarkUnreachable(fmt.Sprintf("127.0.0.1:%d", pod.LocalServer.Port), "local server is stopped")
		}
	} else {
		for _, server := range pod.RemoteServers {
			endpoints = append(endpoints, server.Endpoint)
		}
		endpoints = append(endpoints, pod.RemoteAccessPoint)
	}
	probeCtx, cancel := context.WithCancel(ctx)
	refresh := &podRefresh{cancel: cancel, done: make(chan struct{})}
	b.refreshMu.Lock()
	if b.refreshes == nil {
		b.refreshes = map[string]*podRefresh{}
	}
	previous := b.refreshes[id]
	b.refreshes[id] = refresh
	b.refreshMu.Unlock()
	if previous != nil {
		previous.cancel()
		<-previous.done
	}
	defer func() {
		cancel()
		close(refresh.done)
		b.refreshMu.Lock()
		if b.refreshes[id] == refresh {
			delete(b.refreshes, id)
		}
		b.refreshMu.Unlock()
	}()
	b.Health.ProbeAll(probeCtx, endpoints)
	return b.summary(pod), nil
}

func (b *PodBridge) StartLocal(ctx context.Context, id string) (PodSummary, error) {
	if err := b.requireInitializationComplete(id); err != nil {
		return PodSummary{}, err
	}
	pod, err := b.Store.Load(id)
	if err != nil {
		return PodSummary{}, err
	}
	if pod.LocalServer == nil {
		return PodSummary{}, fmt.Errorf("desktop bridge: pod %q is remote", id)
	}
	if err := b.recoverLocalServerForMutation(ctx, pod); err != nil {
		return PodSummary{}, err
	}
	if err := b.Store.Save(pod); err != nil {
		return PodSummary{}, fmt.Errorf("desktop bridge: refresh local workspace: %w", err)
	}
	workspace := filepath.Join(b.Paths.PodsDir, id, "workspace")
	if b.Local.Status(id).State != "running" {
		if listenErr := appconfig.CheckPortAvailable(pod.LocalServer.Port); listenErr != nil {
			return PodSummary{}, fmt.Errorf("desktop bridge: local server port %d is already in use", pod.LocalServer.Port)
		}
	}
	if _, err := b.Local.Start(id, workspace); err != nil {
		return PodSummary{}, err
	}
	pod, err = b.ensureLocalRuntimeContract(ctx, pod)
	if err != nil {
		return PodSummary{}, err
	}
	return b.summary(pod), nil
}

func (b *PodBridge) ensureLocalRuntimeContract(ctx context.Context, pod appconfig.Pod) (appconfig.Pod, error) {
	if pod.LocalServer == nil || pod.LocalCatalogVersion >= appconfig.LocalCatalogVersion {
		return pod, nil
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Minute)
		defer cancel()
	}
	if err := b.waitLocalReady(ctx, pod.ID, pod.LocalServer.Port); err != nil {
		return appconfig.Pod{}, err
	}
	if err := b.migrateLocalRuntimeContract(ctx, pod.ID); err != nil {
		return appconfig.Pod{}, err
	}
	return b.Store.Load(pod.ID)
}

func (b *PodBridge) migrateLocalRuntimeContract(ctx context.Context, id string) error {
	b.contractMu.Lock()
	defer b.contractMu.Unlock()
	pod, err := b.Store.Load(id)
	if err != nil {
		return err
	}
	if pod.LocalServer == nil || pod.LocalCatalogVersion >= appconfig.LocalCatalogVersion {
		return nil
	}
	if b.Bootstrapper == nil {
		return errors.New("desktop bridge: local runtime migration requires a bootstrapper")
	}
	podDir := filepath.Join(b.Paths.PodsDir, id)
	if err := b.Bootstrapper.MigrateRuntimeContract(ctx, podDir); err != nil {
		return fmt.Errorf("desktop bridge: migrate local runtime contract: %w", err)
	}
	pod.LocalCatalogVersion = appconfig.LocalCatalogVersion
	if err := b.Store.Save(pod); err != nil {
		return fmt.Errorf("desktop bridge: record local catalog version: %w", err)
	}
	return nil
}

func (b *PodBridge) StopLocal(ctx context.Context, id string) (PodSummary, error) {
	if err := b.requireInitializationComplete(id); err != nil {
		return PodSummary{}, err
	}
	pod, err := b.Store.Load(id)
	if err != nil {
		return PodSummary{}, err
	}
	if pod.LocalServer == nil {
		return PodSummary{}, fmt.Errorf("desktop bridge: pod %q is remote", id)
	}
	if err := b.recoverLocalServerForMutation(ctx, pod); err != nil {
		return PodSummary{}, err
	}
	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := b.Local.Stop(stopCtx, id); err != nil {
		return PodSummary{}, err
	}
	b.Health.MarkUnreachable(fmt.Sprintf("127.0.0.1:%d", pod.LocalServer.Port), "local server is stopped")
	return b.summary(pod), nil
}

func (b *PodBridge) RestartLocal(ctx context.Context, id string) (PodSummary, error) {
	if err := b.requireInitializationComplete(id); err != nil {
		return PodSummary{}, err
	}
	pod, err := b.Store.Load(id)
	if err != nil {
		return PodSummary{}, err
	}
	if pod.LocalServer == nil {
		return PodSummary{}, fmt.Errorf("desktop bridge: pod %q is remote", id)
	}
	if err := b.recoverLocalServerForMutation(ctx, pod); err != nil {
		return PodSummary{}, err
	}
	if err := b.Store.Save(pod); err != nil {
		return PodSummary{}, fmt.Errorf("desktop bridge: refresh local workspace: %w", err)
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
	pod, err = b.ensureLocalRuntimeContract(ctx, pod)
	if err != nil {
		return PodSummary{}, err
	}
	return b.summary(pod), nil
}

func (b *PodBridge) AdminURL(_ context.Context, podID, serverID string) (string, error) {
	if err := b.requireInitializationComplete(podID); err != nil {
		return "", err
	}
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

func (b *PodBridge) PlayURL(ctx context.Context, podID string) (string, error) {
	if err := b.requireInitializationComplete(podID); err != nil {
		return "", err
	}
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
	if pod.LocalServer != nil {
		podDir := filepath.Join(b.Paths.PodsDir, podID)
		tokenPath := filepath.Join(podDir, "workspace", localserver.RegistrationTokenFile)
		b.tokenMu.Lock()
		token, err := func() ([]byte, error) {
			defer b.tokenMu.Unlock()
			token, err := os.ReadFile(tokenPath)
			if !errors.Is(err, os.ErrNotExist) {
				return token, err
			}
			if b.Bootstrapper == nil {
				return nil, fmt.Errorf("recover local Play registration token: bootstrapper is not configured")
			}
			savedEnvironment, loadErr := b.BootstrapEnvironment.Load()
			if loadErr != nil {
				return nil, fmt.Errorf("load bootstrap environment for local Play registration token: %w", loadErr)
			}
			if recoverErr := b.Bootstrapper.RecoverRegistrationToken(ctx, podDir, savedEnvironment); recoverErr != nil {
				return nil, fmt.Errorf("recover local Play registration token: %w", recoverErr)
			}
			return os.ReadFile(tokenPath)
		}()
		if err != nil {
			return "", fmt.Errorf("desktop bridge: read local Play registration token: %w", err)
		}
		runtime.RegistrationToken = strings.TrimSpace(string(token))
		if runtime.RegistrationToken == "" {
			return "", fmt.Errorf("desktop bridge: local Play registration token is empty")
		}
	} else {
		runtime.RegistrationToken = strings.TrimSpace(pod.RegistrationToken)
		if runtime.RegistrationToken == "" {
			return "", fmt.Errorf("desktop bridge: remote Play RegistrationToken is not configured")
		}
	}
	return b.WebUI.LaunchURL(podID, "play", runtime)
}

func (b *PodBridge) summary(pod appconfig.Pod) PodSummary {
	playConfigured := pod.ClientPrivateKey != ""
	if pod.LocalServer == nil {
		playConfigured = playConfigured && strings.TrimSpace(pod.RegistrationToken) != ""
	}
	summary := PodSummary{ID: pod.ID, Name: pod.Name, Description: pod.Description, PlayConfigured: playConfigured, PlayPublicKey: publicKeyForPrivate(pod.ClientPrivateKey), RegistrationToken: b.shareRegistrationToken(pod), Valid: true}
	if initialization, err := b.Store.Initialization(pod.ID); err != nil {
		summary.Initialization = &InitializationSummary{State: "failed", Error: err.Error()}
	} else if initialization != nil {
		summary.Initialization = &InitializationSummary{State: initialization.State, Error: initialization.Error}
	}
	if pod.LocalServer != nil {
		endpoint := fmt.Sprintf("127.0.0.1:%d", pod.LocalServer.Port)
		summary.Mode = "local"
		serverPublicKey, _ := b.Store.LocalServerPublicKey(pod.ID)
		summary.Local = &LocalSummary{Port: pod.LocalServer.Port, LANAddresses: lanAddresses(pod.LocalServer.Port), AdminConfigured: pod.LocalServer.AdminPrivateKey != "", AdminPublicKey: publicKeyForPrivate(pod.LocalServer.AdminPrivateKey), ServerPublicKey: serverPublicKey, Process: b.Local.Status(pod.ID), Health: b.Health.Get(endpoint)}
		return summary
	}
	summary.Mode = "remote"
	remote := &RemoteSummary{AccessPoint: b.Health.Get(pod.RemoteAccessPoint), Servers: make([]ServerSummary, 0, len(pod.RemoteServers))}
	for _, server := range pod.RemoteServers {
		remote.Servers = append(remote.Servers, ServerSummary{ID: server.ID, Name: server.Name, Endpoint: server.Endpoint, AdminConfigured: server.AdminPrivateKey != "", AdminPublicKey: publicKeyForPrivate(server.AdminPrivateKey), Health: b.Health.Get(server.Endpoint)})
	}
	summary.Remote = remote
	return summary
}

func (b *PodBridge) shareRegistrationToken(pod appconfig.Pod) string {
	if pod.LocalServer == nil {
		return strings.TrimSpace(pod.RegistrationToken)
	}
	data, err := os.ReadFile(filepath.Join(b.Paths.PodsDir, pod.ID, "workspace", localserver.RegistrationTokenFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func ensurePodIdentities(pod *appconfig.Pod) (bool, error) {
	if pod.IdentitiesInitialized {
		return false, nil
	}
	changed := false
	ensure := func(value *string) error {
		if strings.TrimSpace(*value) != "" {
			return nil
		}
		kp, err := giznet.GenerateKeyPair()
		if err != nil {
			return fmt.Errorf("desktop bridge: generate identity: %w", err)
		}
		*value = kp.Private.String()
		changed = true
		return nil
	}
	if err := ensure(&pod.ClientPrivateKey); err != nil {
		return false, err
	}
	if pod.LocalServer != nil {
		if err := ensure(&pod.LocalServer.AdminPrivateKey); err != nil {
			return false, err
		}
	}
	pod.IdentitiesInitialized = true
	changed = true
	return changed, nil
}

func publicKeyForPrivate(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	var private giznet.Key
	if err := private.UnmarshalText([]byte(value)); err != nil {
		return ""
	}
	kp, err := giznet.NewKeyPair(private)
	if err != nil {
		return ""
	}
	return kp.Public.String()
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
	preferred := appconfig.PreferredLANEndpoint(port)
	for i, value := range result {
		if value == preferred {
			copy(result[1:i+1], result[:i])
			result[0] = value
			break
		}
	}
	return result
}

func (b *PodBridge) inputToPod(input PodInput, existing *appconfig.Pod) (appconfig.Pod, error) {
	pod := appconfig.Pod{Version: input.Version, ID: strings.TrimSpace(input.ID), Name: strings.TrimSpace(input.Name), Description: strings.TrimSpace(input.Description), RemoteAccessPoint: strings.TrimSpace(input.RemoteAccessPoint)}
	if existing != nil {
		pod.IdentitiesInitialized = existing.IdentitiesInitialized
		pod.LocalCatalogVersion = existing.LocalCatalogVersion
	}
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
		serverID := strings.TrimSpace(server.ID)
		if serverID == "" {
			serverID = newInternalID("server")
		}
		oldKey := ""
		if existing != nil {
			for _, current := range existing.RemoteServers {
				if current.ID == serverID {
					oldKey = current.AdminPrivateKey
				}
			}
		}
		name := strings.TrimSpace(server.Name)
		if name == "" {
			name = strings.TrimSpace(server.Endpoint)
		}
		pod.RemoteServers = append(pod.RemoteServers, appconfig.RemoteServer{ID: serverID, Name: name, Endpoint: strings.TrimSpace(server.Endpoint), AdminPrivateKey: secretValue(server.AdminPrivateKey, oldKey)})
	}
	oldClient := ""
	oldRegistrationToken := ""
	if existing != nil {
		oldClient = existing.ClientPrivateKey
		oldRegistrationToken = existing.RegistrationToken
	}
	pod.ClientPrivateKey = secretValue(input.ClientPrivateKey, oldClient)
	pod.RegistrationToken = secretValue(input.RegistrationToken, oldRegistrationToken)
	return pod, nil
}

func newInternalID(prefix string) string {
	var value [6]byte
	if _, err := rand.Read(value[:]); err == nil {
		return prefix + "-" + hex.EncodeToString(value[:])
	}
	return fmt.Sprintf("%s-%x", prefix, time.Now().UnixNano())
}

func secretValue(input *string, existing string) string {
	if input == nil {
		return existing
	}
	return strings.TrimSpace(*input)
}
