package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	store "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
)

const workspaceConfigFile = "config.yaml"
const workspacePIDFile = "serve.pid"

var installConfiguredLogger = gizlog.InstallDefault

type ServeOptions struct {
	Force bool
}

func resolveWorkspaceRoot(workspace string) (string, error) {
	root, err := filepath.Abs(workspace)
	if err != nil {
		return "", fmt.Errorf("server: resolve workspace %q: %w", workspace, err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("server: create workspace %q: %w", root, err)
	}
	return root, nil
}

func prepareWorkspaceConfig(workspace string) (Config, error) {
	root, err := resolveWorkspaceRoot(workspace)
	if err != nil {
		return Config{}, err
	}
	configPath := filepath.Join(root, workspaceConfigFile)
	fileCfg, err := LoadConfig(configPath)
	if err != nil {
		return Config{}, fmt.Errorf("server: load config: %w", err)
	}
	keyPair, fileCfg, err := resolveWorkspaceIdentity(configPath, fileCfg)
	if err != nil {
		return Config{}, fmt.Errorf("server: identity: %w", err)
	}

	cfg, err := mergeFileConfig(Config{
		KeyPair:       keyPair,
		WorkspaceRoot: root,
	}, fileCfg)
	if err != nil {
		return Config{}, err
	}
	cfg.Storage = resolveWorkspaceStorageConfigs(root, cfg.Storage)
	cfg.Stores = resolveWorkspaceStoreConfigs(root, cfg.Stores)
	cfg.HTTP = resolveWorkspaceHTTPConfig(root, cfg.HTTP)
	return prepareConfig(cfg)
}

func resolveWorkspaceHTTPConfig(root string, cfg HTTPConfig) HTTPConfig {
	for index := range cfg.Listeners {
		certFile := os.ExpandEnv(cfg.Listeners[index].TLS.CertFile)
		keyFile := os.ExpandEnv(cfg.Listeners[index].TLS.KeyFile)
		cfg.Listeners[index].TLS.CertFile = resolveWorkspaceDir(root, certFile)
		cfg.Listeners[index].TLS.KeyFile = resolveWorkspaceDir(root, keyFile)
	}
	return cfg
}

func resolveWorkspaceIdentity(configPath string, fileCfg ConfigFile) (*giznet.KeyPair, ConfigFile, error) {
	if !fileCfg.Identity.PrivateKey.IsZero() {
		keyPair, err := giznet.NewKeyPair(fileCfg.Identity.PrivateKey)
		if err != nil {
			return nil, ConfigFile{}, fmt.Errorf("load identity.private-key: %w", err)
		}
		return keyPair, fileCfg, nil
	}

	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		return nil, ConfigFile{}, fmt.Errorf("generate: %w", err)
	}
	if err := writeWorkspaceIdentity(configPath, keyPair.Private); err != nil {
		return nil, ConfigFile{}, fmt.Errorf("write config: %w", err)
	}
	fileCfg.Identity.PrivateKey = keyPair.Private
	return keyPair, fileCfg, nil
}

func writeWorkspaceIdentity(configPath string, privateKey giznet.Key) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	prefix := fmt.Sprintf("identity:\n  private-key: %s\n", privateKey.String())
	return os.WriteFile(configPath, []byte(prefix+string(data)), 0o600)
}

func resolveWorkspaceStorageConfigs(root string, cfgs map[string]storage.Config) map[string]storage.Config {
	if len(cfgs) == 0 {
		return nil
	}

	resolved := make(map[string]storage.Config, len(cfgs))
	for name, cfg := range cfgs {
		switch cfg := cfg.(type) {
		case storage.BadgerConfig:
			cfg.Dir = resolveWorkspaceDir(root, cfg.Dir)
			resolved[name] = cfg
		case *storage.BadgerConfig:
			if cfg == nil {
				resolved[name] = cfg
				continue
			}
			copy := *cfg
			copy.Dir = resolveWorkspaceDir(root, copy.Dir)
			resolved[name] = copy
		case storage.FilesystemDirConfig:
			cfg.Dir = resolveWorkspaceDir(root, cfg.Dir)
			resolved[name] = cfg
		case *storage.FilesystemDirConfig:
			if cfg == nil {
				resolved[name] = cfg
				continue
			}
			copy := *cfg
			copy.Dir = resolveWorkspaceDir(root, copy.Dir)
			resolved[name] = copy
		case storage.SQLiteConfig:
			cfg.Dir = resolveWorkspaceDir(root, cfg.Dir)
			resolved[name] = cfg
		case *storage.SQLiteConfig:
			if cfg == nil {
				resolved[name] = cfg
				continue
			}
			copy := *cfg
			copy.Dir = resolveWorkspaceDir(root, copy.Dir)
			resolved[name] = copy
		case storage.GCSConfig:
			cfg.CredentialsFile = resolveWorkspaceDir(root, cfg.CredentialsFile)
			resolved[name] = cfg
		case *storage.GCSConfig:
			if cfg == nil {
				resolved[name] = cfg
				continue
			}
			copy := *cfg
			copy.CredentialsFile = resolveWorkspaceDir(root, copy.CredentialsFile)
			resolved[name] = copy
		default:
			resolved[name] = cfg
		}
	}
	return resolved
}

func resolveWorkspaceDir(root, dir string) string {
	if dir == "" || filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(root, dir)
}

func resolveWorkspaceStoreConfigs(root string, cfgs map[string]store.Config) map[string]store.Config {
	if len(cfgs) == 0 {
		return nil
	}

	resolved := make(map[string]store.Config, len(cfgs))
	maps.Copy(resolved, cfgs)
	return resolved
}

func expandEnvIfAllNonEmpty(value string) (string, bool) {
	valid := true
	expanded := os.Expand(value, func(name string) string {
		result, ok := os.LookupEnv(name)
		if !ok || result == "" {
			valid = false
		}
		return result
	})
	return expanded, valid
}

func Serve(workspace string) error {
	return ServeWithOptions(workspace, ServeOptions{})
}

func ServeContext(ctx context.Context, workspace string, opts ServeOptions) (err error) {
	root, err := resolveWorkspaceRoot(workspace)
	if err != nil {
		return err
	}
	if !opts.Force {
		return fmt.Errorf("server: direct serve is disabled; start the server through service with 'gizclaw service install %s' and 'gizclaw service start', or pass --force for explicit foreground local serve", root)
	}
	cfg, err := prepareWorkspaceConfig(workspace)
	if err != nil {
		return err
	}
	storeRegistry, physicalStorage, err := newStoreRegistry(cfg)
	if err != nil {
		return fmt.Errorf("server: stores: %w", err)
	}
	defer func() {
		err = errors.Join(err, storeRegistry.Close())
		err = errors.Join(err, physicalStorage.Close())
	}()
	closeLogger, err := installConfiguredLogger(cfg.systemLogConfig(), storeRegistry)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, closeLogger())
	}()
	releasePID, err := acquireWorkspacePID(root, opts.Force)
	if err != nil {
		return err
	}
	defer releasePID()
	profilingStore, err := resolveProfilingStore(storeRegistry, cfg.Profiling)
	if err != nil {
		return err
	}
	if cfg.Profiling.Enabled {
		profiler, err := newProcessProfiler(profilingStore, profilingOptions{})
		if err != nil {
			return err
		}
		if err := profiler.baseline(); err != nil {
			return fmt.Errorf("server: profiling baseline: %w", err)
		}
		profiler.start(ctx)
		defer profiler.stop()
	}
	publicListener, err := net.Listen("tcp", cfg.PublicAPIListenAddr())
	if err != nil {
		return fmt.Errorf("server: listen public transport: %w", err)
	}
	preparedHTTP, iceTCPListener, closeIngress, err := prepareServerHTTPListeners(cfg, publicListener)
	if err != nil {
		_ = publicListener.Close()
		return err
	}
	defer closeIngress()
	srv, err := newWithOptions(cfg, newServerOptions{ICETCPListener: iceTCPListener, Stores: storeRegistry})
	if err != nil {
		return err
	}
	defer srv.Close()
	for index := range preparedHTTP {
		preparedHTTP[index].server.Handler = srv
	}
	if err := srv.Listen(); err != nil {
		return err
	}
	errCh := make(chan error, len(preparedHTTP)+1)
	gizServer := srv.Server
	go func() {
		errCh <- gizServer.Serve()
	}()
	for _, item := range preparedHTTP {
		go func(item serverHTTPListener) {
			err := item.server.Serve(item.listener)
			if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
				err = nil
			}
			errCh <- err
		}(item)
	}

	select {
	case err := <-errCh:
		errCh <- err
		return shutdownServerIngress(preparedHTTP, srv, errCh)
	case <-ctx.Done():
		return shutdownServerIngress(preparedHTTP, srv, errCh)
	}
}

type serverHTTPListener struct {
	server   *http.Server
	listener net.Listener
}

func prepareServerHTTPListeners(cfg Config, publicListener net.Listener) ([]serverHTTPListener, net.Listener, func() error, error) {
	primary := cfg.HTTP.Listeners[0]
	primaryTLS, err := primary.TLS.tlsConfig("http.listeners[0].tls")
	if err != nil {
		return nil, nil, nil, err
	}
	var mux *publicTCPMux
	var iceTCPListener net.Listener
	var primaryListener net.Listener
	if primaryTLS == nil {
		mux = newPublicTCPMux(publicListener)
		primaryListener = mux.HTTPListener()
		iceTCPListener = mux.ICETCPListener()
	} else {
		primaryListener = tls.NewListener(publicListener, primaryTLS)
	}
	prepared := []serverHTTPListener{{server: &http.Server{}, listener: primaryListener}}
	for index := 1; index < len(cfg.HTTP.Listeners); index++ {
		listenerCfg := cfg.HTTP.Listeners[index]
		listener, listenErr := net.Listen("tcp", listenerCfg.Listen)
		if listenErr != nil {
			for _, item := range prepared[1:] {
				_ = item.listener.Close()
			}
			if mux != nil {
				_ = mux.Close()
			}
			return nil, nil, nil, fmt.Errorf("server: listen http.listeners[%d]: %w", index, listenErr)
		}
		tlsConfig, tlsErr := listenerCfg.TLS.tlsConfig(fmt.Sprintf("http.listeners[%d].tls", index))
		if tlsErr != nil {
			_ = listener.Close()
			for _, item := range prepared[1:] {
				_ = item.listener.Close()
			}
			if mux != nil {
				_ = mux.Close()
			}
			return nil, nil, nil, tlsErr
		}
		if tlsConfig != nil {
			listener = tls.NewListener(listener, tlsConfig)
		}
		prepared = append(prepared, serverHTTPListener{server: &http.Server{}, listener: listener})
	}
	closeIngress := func() error {
		var errs []error
		if mux != nil {
			errs = append(errs, mux.Close())
		} else {
			errs = append(errs, publicListener.Close())
		}
		for _, item := range prepared[1:] {
			errs = append(errs, item.listener.Close())
		}
		return errors.Join(errs...)
	}
	return prepared, iceTCPListener, closeIngress, nil
}

func shutdownServerIngress(httpListeners []serverHTTPListener, srv *CmdServer, errCh <-chan error) error {
	var errs []error
	for _, item := range httpListeners {
		errs = append(errs, item.server.Shutdown(context.Background()))
	}
	errs = append(errs, srv.Close())
	for range len(httpListeners) + 1 {
		errs = append(errs, <-errCh)
	}
	return errors.Join(errs...)
}

func ServeWithOptions(workspace string, opts ServeOptions) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return ServeContext(ctx, workspace, opts)
}

func acquireWorkspacePID(root string, force bool) (func(), error) {
	pidPath := filepath.Join(root, workspacePIDFile)
	if err := handleExistingWorkspacePID(pidPath, force); err != nil {
		return nil, err
	}
	pid := os.Getpid()
	file, err := os.OpenFile(pidPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("server: %s already exists", pidPath)
		}
		return nil, fmt.Errorf("server: create %s: %w", pidPath, err)
	}
	if _, err := fmt.Fprintf(file, "%d\n", pid); err != nil {
		file.Close()
		_ = os.Remove(pidPath)
		return nil, fmt.Errorf("server: write %s: %w", pidPath, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(pidPath)
		return nil, fmt.Errorf("server: close %s: %w", pidPath, err)
	}

	return func() {
		currentPID, err := readWorkspacePID(pidPath)
		if err == nil && currentPID == pid {
			_ = os.Remove(pidPath)
		}
	}, nil
}

func handleExistingWorkspacePID(pidPath string, force bool) error {
	pid, err := readWorkspacePID(pidPath)
	if err == nil {
		if processRunning(pid) {
			return fmt.Errorf("server: already running with pid %d", pid)
		} else if !force {
			return fmt.Errorf("server: stale pid file %s exists (use -f to replace)", pidPath)
		}
		if err := os.Remove(pidPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("server: remove %s: %w", pidPath, err)
		}
		return nil
	}
	if os.IsNotExist(err) {
		return nil
	}
	if !force {
		return fmt.Errorf("server: read %s: %w", pidPath, err)
	}
	if removeErr := os.Remove(pidPath); removeErr != nil && !os.IsNotExist(removeErr) {
		return fmt.Errorf("server: remove %s: %w", pidPath, removeErr)
	}
	return nil
}

func readWorkspacePID(pidPath string) (int, error) {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("invalid pid in %s", pidPath)
	}
	return pid, nil
}

func processRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil || strings.Contains(err.Error(), "operation not permitted")
}

func terminateProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("server: find pid %d: %w", pid, err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil && !strings.Contains(err.Error(), "process already finished") {
		return fmt.Errorf("server: terminate pid %d: %w", pid, err)
	}
	return nil
}

func waitForProcessExit(pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processRunning(pid) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("server: pid %d did not exit after %s", pid, timeout)
}
