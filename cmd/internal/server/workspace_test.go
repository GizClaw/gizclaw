package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/logging"
	"github.com/GizClaw/gizclaw-go/cmd/internal/storage"
	"github.com/GizClaw/gizclaw-go/cmd/internal/stores"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/goccy/go-yaml"
)

func validWorkspaceConfigData(t *testing.T, mutate func(*ConfigFile)) []byte {
	t.Helper()
	runtime := validLayeredConfig(".")
	cfg := ConfigFile{
		Listen:   runtime.Listen,
		Endpoint: runtime.Endpoint,
		Storage:  runtime.Storage,
		Stores:   runtime.Stores,
		Services: runtime.Services,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	var identity *IdentityConfig
	if !cfg.Identity.PrivateKey.IsZero() {
		identity = &cfg.Identity
	}
	raw := struct {
		Identity       *IdentityConfig           `yaml:"identity,omitempty"`
		Listen         string                    `yaml:"listen,omitempty"`
		Endpoint       string                    `yaml:"endpoint,omitempty"`
		ServeToClients bool                      `yaml:"serve-to-clients,omitempty"`
		AdminPublicKey giznet.PublicKey          `yaml:"admin-public-key,omitempty"`
		Storage        map[string]storage.Config `yaml:"storage"`
		Stores         map[string]stores.Config  `yaml:"stores"`
		Services       *ServicesConfig           `yaml:"services"`
	}{
		Identity: identity, Listen: cfg.Listen, Endpoint: cfg.Endpoint,
		ServeToClients: cfg.ServeToClients, AdminPublicKey: cfg.AdminPublicKey,
		Storage: cfg.Storage, Stores: cfg.Stores, Services: cfg.Services,
	}
	data, err := yaml.MarshalWithOptions(raw, yaml.OmitEmpty())
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}
	return data
}

func TestPrepareWorkspaceConfigLoadsWorkspaceConfig(t *testing.T) {
	workspace := t.TempDir()
	serverKP := testKeyPair(t, 0xcd)
	adminKP := testKeyPair(t, 0xab)
	data := validWorkspaceConfigData(t, func(cfg *ConfigFile) {
		cfg.Identity.PrivateKey = serverKP.Private
		cfg.Listen = "127.0.0.1:39001"
		cfg.Endpoint = "127.0.0.1:39001"
		cfg.AdminPublicKey = adminKP.Public
		cfg.Storage["local-files"] = storage.Config{Kind: storage.KindFilesystemDir, Dir: "."}
		cfg.Storage["gameplay-db"] = storage.Config{Kind: storage.KindSQLite, Dir: "data/gameplay.sqlite"}
	})
	if err := os.WriteFile(filepath.Join(workspace, workspaceConfigFile), data, 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	cfg, err := prepareWorkspaceConfig(workspace)
	if err != nil {
		t.Fatalf("prepareWorkspaceConfig error = %v", err)
	}
	if cfg.KeyPair == nil {
		t.Fatal("KeyPair should not be nil")
	}
	if cfg.KeyPair.Public != serverKP.Public {
		t.Fatalf("KeyPair.Public = %v, want %v", cfg.KeyPair.Public, serverKP.Public)
	}
	if cfg.Listen != "127.0.0.1:39001" {
		t.Fatalf("Listen = %q", cfg.Listen)
	}
	if cfg.Endpoint != "127.0.0.1:39001" {
		t.Fatalf("Endpoint = %q", cfg.Endpoint)
	}
	adminKey := adminKP.Public
	if cfg.AdminPublicKey != adminKey {
		t.Fatalf("AdminPublicKey = %v", cfg.AdminPublicKey)
	}
	if got := cfg.Storage["local-files"].Dir; got != workspace {
		t.Fatalf("local-files dir = %q", got)
	}
	if got := cfg.Storage["gameplay-db"].Dir; got != filepath.Join(workspace, "data", "gameplay.sqlite") {
		t.Fatalf("gameplay db dir = %q", got)
	}
}

func TestServeContextServerInfoReportsTCPICE(t *testing.T) {
	addr := localTCPUDPAddr(t)
	workspace := t.TempDir()
	data := validWorkspaceConfigData(t, func(cfg *ConfigFile) {
		cfg.Listen = addr
		cfg.Endpoint = addr
		cfg.ServeToClients = true
	})
	if err := os.WriteFile(filepath.Join(workspace, workspaceConfigFile), data, 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- ServeContext(ctx, workspace, ServeOptions{Force: true})
	}()
	t.Cleanup(func() {
		cancel()
		if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("ServeContext shutdown error = %v", err)
		}
	})

	info := waitForServerInfo(t, "http://"+addr+"/server-info")
	if !info.Ice.Udp || !info.Ice.Tcp {
		t.Fatalf("server-info ice = %+v, want udp=true tcp=true", info.Ice)
	}
	if info.Endpoint != addr {
		t.Fatalf("server-info endpoint = %q, want %q", info.Endpoint, addr)
	}
}

func TestServeContextDefaultKeepsTCPMuxAndRequiresPrivateAuth(t *testing.T) {
	addr := localTCPUDPAddr(t)
	workspace := t.TempDir()
	data := validWorkspaceConfigData(t, func(cfg *ConfigFile) {
		cfg.Listen = addr
		cfg.Endpoint = addr
	})
	if err := os.WriteFile(filepath.Join(workspace, workspaceConfigFile), data, 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- ServeContext(ctx, workspace, ServeOptions{Force: true})
	}()
	t.Cleanup(func() {
		cancel()
		if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("ServeContext shutdown error = %v", err)
		}
	})

	var lastErr error
	var lastStatus int
	client := http.Client{Timeout: 200 * time.Millisecond}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://" + addr + "/server-info")
		if err != nil {
			lastErr = err
			time.Sleep(50 * time.Millisecond)
			continue
		}
		lastStatus = resp.StatusCode
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server-info status = %d err = %v, want 401 over active TCP mux when serve-to-clients is disabled", lastStatus, lastErr)
}

func localTCPUDPAddr(t *testing.T) string {
	t.Helper()
	for range 10 {
		tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("Listen tcp error = %v", err)
		}
		addr := tcpListener.Addr().String()
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			tcpListener.Close()
			t.Fatalf("SplitHostPort error = %v", err)
		}
		udpConn, err := net.ListenPacket("udp", net.JoinHostPort(host, port))
		if err == nil {
			udpConn.Close()
			tcpListener.Close()
			return addr
		}
		tcpListener.Close()
	}
	t.Fatal("could not find an available TCP/UDP localhost port")
	return ""
}

func waitForServerInfo(t *testing.T, url string) apitypes.ServerInfo {
	t.Helper()
	client := http.Client{Timeout: 200 * time.Millisecond}
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err != nil {
			lastErr = err
			time.Sleep(50 * time.Millisecond)
			continue
		}
		var info apitypes.ServerInfo
		decodeErr := json.NewDecoder(resp.Body).Decode(&info)
		closeErr := resp.Body.Close()
		if resp.StatusCode == http.StatusOK && decodeErr == nil && closeErr == nil {
			return info
		}
		if decodeErr != nil {
			lastErr = decodeErr
		} else if closeErr != nil {
			lastErr = closeErr
		} else {
			lastErr = fmt.Errorf("status %s", resp.Status)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server-info was not ready: %v", lastErr)
	return apitypes.ServerInfo{}
}

func TestPrepareWorkspaceConfigUsesDefaultPorts(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, workspaceConfigFile)
	data := validWorkspaceConfigData(t, func(cfg *ConfigFile) {
		cfg.Listen = ""
		cfg.Endpoint = ""
	})
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	cfg, err := prepareWorkspaceConfig(workspace)
	if err != nil {
		t.Fatalf("prepareWorkspaceConfig error = %v", err)
	}
	defaults := DefaultConfig()
	if cfg.Listen != defaults.Listen {
		t.Fatalf("default listen = %q, want %q", cfg.Listen, defaults.Listen)
	}
	if cfg.Endpoint != defaults.Endpoint {
		t.Fatalf("default endpoint = %q, want %q", cfg.Endpoint, defaults.Endpoint)
	}
	rewritten, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig rewritten error = %v", err)
	}
	if rewritten.Identity.PrivateKey.IsZero() {
		t.Fatal("identity.private-key should be written back to config")
	}
	rewrittenKeyPair, err := giznet.NewKeyPair(rewritten.Identity.PrivateKey)
	if err != nil {
		t.Fatalf("rewritten identity private key error = %v", err)
	}
	if rewrittenKeyPair.Public != cfg.KeyPair.Public {
		t.Fatalf("rewritten public key = %v, want %v", rewrittenKeyPair.Public, cfg.KeyPair.Public)
	}
}

func TestPrepareWorkspaceConfigLoadError(t *testing.T) {
	_, err := prepareWorkspaceConfig(t.TempDir())
	if err == nil {
		t.Fatal("prepareWorkspaceConfig should fail without config.yaml")
	}
}

func TestPrepareWorkspaceConfigResolvesRelativeStoreDirs(t *testing.T) {
	workspace := t.TempDir()
	data := validWorkspaceConfigData(t, func(cfg *ConfigFile) {
		cfg.Storage["local-files"] = storage.Config{Kind: storage.KindFilesystemDir, Dir: "."}
		cfg.Storage["gameplay-db"] = storage.Config{Kind: storage.KindSQLite, Dir: "data/fixture.sqlite"}
	})
	if err := os.WriteFile(filepath.Join(workspace, workspaceConfigFile), data, 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	cfg, err := prepareWorkspaceConfig(workspace)
	if err != nil {
		t.Fatalf("prepareWorkspaceConfig error = %v", err)
	}
	if got := cfg.Storage["local-files"].Dir; got != workspace {
		t.Fatalf("asset dir = %q", got)
	}
	if got := cfg.Storage["gameplay-db"].Dir; got != filepath.Join(workspace, "data", "fixture.sqlite") {
		t.Fatalf("gameplay-db dir = %q", got)
	}
	if got := cfg.Stores["workspace-assets"].Prefix; got != "workspaces" {
		t.Fatalf("workspace-assets prefix = %q", got)
	}
}

func TestPrepareWorkspaceConfigIdentityError(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, workspaceConfigFile), []byte(`
identity:
  private-key: not-a-key
`), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	_, err := prepareWorkspaceConfig(workspace)
	if err == nil || !strings.Contains(err.Error(), "invalid key text") {
		t.Fatalf("prepareWorkspaceConfig identity error = %v", err)
	}
}

func TestWriteWorkspaceIdentityReadError(t *testing.T) {
	err := writeWorkspaceIdentity(filepath.Join(t.TempDir(), "missing.yaml"), testKeyPair(t, 0xde).Private)
	if err == nil {
		t.Fatal("writeWorkspaceIdentity should fail for missing config")
	}
}

func TestResolveWorkspaceStorageConfigsPreservesAbsoluteDirs(t *testing.T) {
	root := t.TempDir()
	absoluteDir := filepath.Join(t.TempDir(), "files")

	gotStorage := resolveWorkspaceStorageConfigs(root, map[string]storage.Config{
		"fw": {
			Kind: storage.KindFilesystemDir,
			Dir:  absoluteDir,
		},
	})
	if gotStorage["fw"].Dir != absoluteDir {
		t.Fatalf("fw storage dir = %q, want %q", gotStorage["fw"].Dir, absoluteDir)
	}
}

func TestServeRejectsDirectRun(t *testing.T) {
	err := Serve(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "direct serve is disabled") || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("Serve(direct) err = %v", err)
	}
}

func TestForceServeReturnsWorkspaceLoadError(t *testing.T) {
	err := ServeContext(context.Background(), t.TempDir(), ServeOptions{Force: true})
	if err == nil || !strings.Contains(err.Error(), "load config") {
		t.Fatalf("force serve err = %v, want workspace load error", err)
	}
}

func TestServeContextUsesBootstrapLoggerOnStoreStartupFailure(t *testing.T) {
	workspace := t.TempDir()
	data := validWorkspaceConfigData(t, func(cfg *ConfigFile) {
		cfg.Listen = "127.0.0.1:0"
		cfg.Endpoint = "127.0.0.1:9820"
		cfg.Storage["memory"] = storage.Config{Kind: storage.KindBadger}
		cfg.Services.SystemLog = &logging.Config{Level: "debug"}
	})
	if err := os.WriteFile(filepath.Join(workspace, workspaceConfigFile), data, 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	installed := false
	restoreLoggingInstaller(t, func(cfg logging.Config, _ ...logging.StoreResolver) (func() error, error) {
		installed = true
		if cfg.Level != "debug" {
			t.Fatalf("log config = %+v, want debug", cfg)
		}
		return func() error { return nil }, nil
	})

	err := ServeContext(context.Background(), workspace, ServeOptions{Force: true})
	if err == nil {
		t.Fatal("ServeContext should fail when New cannot build stores")
	}
	if installed {
		t.Fatal("configured logger was installed before stores were available")
	}
}

func TestServeContextClosesLoggerOnShutdown(t *testing.T) {
	workspace := t.TempDir()
	data := validWorkspaceConfigData(t, func(cfg *ConfigFile) {
		cfg.Listen = "127.0.0.1:0"
		cfg.Endpoint = "127.0.0.1:9820"
	})
	if err := os.WriteFile(filepath.Join(workspace, workspaceConfigFile), data, 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	installed := make(chan struct{})
	closed := make(chan struct{})
	restoreLoggingInstaller(t, func(logging.Config, ...logging.StoreResolver) (func() error, error) {
		close(installed)
		return func() error {
			close(closed)
			return nil
		}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- ServeContext(ctx, workspace, ServeOptions{Force: true})
	}()
	select {
	case <-installed:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("logger was not installed")
	}
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("ServeContext shutdown error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ServeContext did not return after cancellation")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("logger cleanup was not called on shutdown")
	}
}

func restoreLoggingInstaller(t *testing.T, fn func(logging.Config, ...logging.StoreResolver) (func() error, error)) {
	t.Helper()
	old := installConfiguredLogger
	installConfiguredLogger = fn
	t.Cleanup(func() { installConfiguredLogger = old })
}

func TestServeReturnsServerBuildError(t *testing.T) {
	workspace := t.TempDir()
	data := validWorkspaceConfigData(t, func(cfg *ConfigFile) {
		cfg.Storage["memory"] = storage.Config{Kind: storage.KindBadger}
	})
	if err := os.WriteFile(filepath.Join(workspace, workspaceConfigFile), data, 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	err := ServeContext(context.Background(), workspace, ServeOptions{Force: true})
	if err == nil {
		t.Fatal("service-managed serve should fail when New cannot build stores")
	}
}

func TestServeContextClosesStoresWhenPIDAcquireFails(t *testing.T) {
	workspace := t.TempDir()
	data := validWorkspaceConfigData(t, func(cfg *ConfigFile) {
		cfg.Storage["memory"] = storage.Config{Kind: storage.KindBadger, Dir: "data/kv"}
		cfg.Storage["local-files"] = storage.Config{Kind: storage.KindFilesystemDir, Dir: "."}
	})
	if err := os.WriteFile(filepath.Join(workspace, workspaceConfigFile), data, 0o644); err != nil {
		t.Fatalf("WriteFile config error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, workspacePIDFile), fmt.Appendf(nil, "%d\n", os.Getpid()), 0o644); err != nil {
		t.Fatalf("WriteFile pid error = %v", err)
	}

	err := ServeContext(context.Background(), workspace, ServeOptions{Force: true})
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("ServeContext() err = %v", err)
	}

	reopened, err := storage.New(map[string]storage.Config{
		"memory": {Kind: storage.KindBadger, Dir: filepath.Join(workspace, "data", "kv")},
	})
	if err != nil {
		t.Fatalf("storage should be closed after PID error, reopen: %v", err)
	}
	defer reopened.Close()
}

func TestHandleExistingWorkspacePIDRejectsStaleWithoutForce(t *testing.T) {
	workspace := t.TempDir()
	pidPath := filepath.Join(workspace, workspacePIDFile)
	if err := os.WriteFile(pidPath, []byte("999999\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	err := handleExistingWorkspacePID(pidPath, false)
	if err == nil || !strings.Contains(err.Error(), "stale pid file") {
		t.Fatalf("handleExistingWorkspacePID() err = %v", err)
	}
}

func TestHandleExistingWorkspacePIDForceRemovesStale(t *testing.T) {
	workspace := t.TempDir()
	pidPath := filepath.Join(workspace, workspacePIDFile)
	if err := os.WriteFile(pidPath, []byte("999999\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	if err := handleExistingWorkspacePID(pidPath, true); err != nil {
		t.Fatalf("handleExistingWorkspacePID(force) error = %v", err)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("pid file should be removed, stat err = %v", err)
	}
}

func TestAcquireWorkspacePIDRejectsRunningPID(t *testing.T) {
	workspace := t.TempDir()
	pidPath := filepath.Join(workspace, workspacePIDFile)
	if err := os.WriteFile(pidPath, fmt.Appendf(nil, "%d\n", os.Getpid()), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	_, err := acquireWorkspacePID(workspace, false)
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("acquireWorkspacePID() err = %v", err)
	}
}

func TestHandleExistingWorkspacePIDForceRemovesUnreadablePID(t *testing.T) {
	workspace := t.TempDir()
	pidPath := filepath.Join(workspace, workspacePIDFile)
	if err := os.WriteFile(pidPath, []byte("not-a-pid\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	if err := handleExistingWorkspacePID(pidPath, true); err != nil {
		t.Fatalf("handleExistingWorkspacePID(force invalid) error = %v", err)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("pid file should be removed, stat err = %v", err)
	}
}

func TestReadWorkspacePIDRejectsInvalidPID(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), workspacePIDFile)
	if err := os.WriteFile(pidPath, []byte("0\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	if _, err := readWorkspacePID(pidPath); err == nil {
		t.Fatal("readWorkspacePID invalid pid error = nil")
	}
}

func TestProcessRunningAndWaitForProcessExitForMissingPID(t *testing.T) {
	if processRunning(0) {
		t.Fatal("processRunning(0) = true")
	}
	if err := waitForProcessExit(999999, time.Millisecond); err != nil {
		t.Fatalf("waitForProcessExit(missing) error = %v", err)
	}
}

func TestAcquireWorkspacePIDWritesAndRemovesCurrentPID(t *testing.T) {
	workspace := t.TempDir()

	release, err := acquireWorkspacePID(workspace, false)
	if err != nil {
		t.Fatalf("acquireWorkspacePID error = %v", err)
	}

	pidPath := filepath.Join(workspace, workspacePIDFile)
	pid, err := readWorkspacePID(pidPath)
	if err != nil {
		t.Fatalf("readWorkspacePID error = %v", err)
	}
	if pid != os.Getpid() {
		t.Fatalf("pid = %d, want %d", pid, os.Getpid())
	}

	release()
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("pid file should be removed, stat err = %v", err)
	}
}
