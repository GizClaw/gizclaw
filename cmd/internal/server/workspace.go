package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/logging"
	"github.com/GizClaw/gizclaw-go/cmd/internal/storage"
	"github.com/GizClaw/gizclaw-go/cmd/internal/stores"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

const workspaceConfigFile = "config.yaml"
const workspacePIDFile = "serve.pid"

var installConfiguredLogger = logging.InstallDefault

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
		KeyPair: keyPair,
	}, fileCfg)
	if err != nil {
		return Config{}, err
	}
	cfg.Storage = resolveWorkspaceStorageConfigs(root, cfg.Storage)
	cfg.Stores = resolveWorkspaceStoreConfigs(root, cfg.Stores)
	return prepareConfig(cfg)
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

func prepareWorkspaceMigrationConfig(workspace string) (Config, error) {
	root, err := resolveWorkspaceRoot(workspace)
	if err != nil {
		return Config{}, err
	}
	fileCfg, err := LoadConfig(filepath.Join(root, workspaceConfigFile))
	if err != nil {
		return Config{}, fmt.Errorf("server: load config: %w", err)
	}
	cfg, err := mergeFileConfig(Config{}, fileCfg)
	if err != nil {
		return Config{}, err
	}
	cfg.Storage = resolveWorkspaceStorageConfigs(root, cfg.Storage)
	cfg.Stores = resolveWorkspaceStoreConfigs(root, cfg.Stores)
	return cfg, nil
}

func resolveWorkspaceStorageConfigs(root string, cfgs map[string]storage.Config) map[string]storage.Config {
	if len(cfgs) == 0 {
		return nil
	}

	resolved := make(map[string]storage.Config, len(cfgs))
	for name, cfg := range cfgs {
		cfg.Dir = resolveWorkspaceDir(root, cfg.Dir)
		if cfg.Badger != nil {
			cfg.Badger.Dir = resolveWorkspaceDir(root, cfg.Badger.Dir)
		}
		if cfg.FS != nil {
			cfg.FS.Dir = resolveWorkspaceDir(root, cfg.FS.Dir)
		}
		if cfg.SQLite != nil {
			cfg.SQLite.Dir = resolveWorkspaceDir(root, cfg.SQLite.Dir)
		}
		resolved[name] = cfg
	}
	return resolved
}

func resolveWorkspaceDir(root, dir string) string {
	if dir == "" || filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(root, dir)
}

func resolveWorkspaceStoreConfigs(root string, cfgs map[string]stores.Config) map[string]stores.Config {
	if len(cfgs) == 0 {
		return nil
	}

	resolved := make(map[string]stores.Config, len(cfgs))
	for name, cfg := range cfgs {
		if cfg.Dir != "" && !filepath.IsAbs(cfg.Dir) {
			cfg.Dir = filepath.Join(root, cfg.Dir)
		}
		if cfg.Flowcraft != nil {
			flowcraft := *cfg.Flowcraft
			if expanded, ok := expandEnvIfAllNonEmpty(flowcraft.Dir); ok {
				flowcraft.Dir = resolveWorkspaceDir(root, expanded)
			}
			cfg.Flowcraft = &flowcraft
		}
		resolved[name] = cfg
	}
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
	storeRegistry, err := newStoreRegistryContext(ctx, cfg)
	if err != nil {
		return fmt.Errorf("server: stores: %w", err)
	}
	defer func() {
		err = errors.Join(err, storeRegistry.Close())
	}()
	closeLogger, err := installConfiguredLogger(cfg.SystemLog, storeRegistry)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, closeLogger())
	}()
	if storeExists(cfg, defaultACLStore) {
		migrator, err := newMigratorWithStores(cfg, storeRegistry, false)
		if err != nil {
			return err
		}
		if err := migrator.Migrate(ctx); err != nil {
			_ = migrator.Close()
			return err
		}
		if err := migrator.Close(); err != nil {
			return err
		}
	}

	publicListener, err := net.Listen("tcp", cfg.PublicAPIListenAddr())
	if err != nil {
		return fmt.Errorf("server: listen tcp mux: %w", err)
	}
	publicMux := newPublicTCPMux(publicListener)
	iceTCPListener := publicMux.ICETCPListener()
	defer publicMux.Close()
	srv, err := newWithOptions(cfg, newServerOptions{ICETCPListener: iceTCPListener, Stores: storeRegistry})
	if err != nil {
		return err
	}
	defer srv.Close()
	publicHTTP := &http.Server{Handler: srv}
	releasePID, err := acquireWorkspacePID(root, opts.Force)
	if err != nil {
		return err
	}
	defer releasePID()

	if err := srv.Listen(); err != nil {
		return err
	}
	errCh := make(chan error, 2)
	gizServer := srv.Server
	go func() {
		errCh <- gizServer.Serve()
	}()
	go func() {
		err := publicHTTP.Serve(publicMux.HTTPListener())
		if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
			err = nil
		}
		errCh <- err
	}()

	select {
	case err := <-errCh:
		_ = publicHTTP.Shutdown(context.Background())
		_ = srv.Close()
		return err
	case <-ctx.Done():
		shutdownErr := publicHTTP.Shutdown(context.Background())
		closeErr := srv.Close()
		err1 := <-errCh
		err2 := <-errCh
		return errors.Join(shutdownErr, closeErr, err1, err2)
	}
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
