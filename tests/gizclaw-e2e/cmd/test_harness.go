//go:build gizclaw_e2e

package clitest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/gizcli"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/goccy/go-yaml"
)

const (
	fixtureListenAddrToken = "__LISTEN_ADDR__"
	serverStopTimeout      = 5 * time.Second
	readyTimeout           = 30 * time.Second
	probeTimeout           = time.Second
	pollInterval           = 20 * time.Millisecond
)

var (
	buildBinaryOnce sync.Once
	buildBinaryPath string
	buildBinaryErr  error
)

type Harness struct {
	t testing.TB

	RepoRoot string
	StoryDir string

	SandboxDir      string
	HomeDir         string
	XDGConfigHome   string
	ServerWorkspace string
	LogsDir         string

	BinaryPath      string
	ServerAddr      string
	ServerPublicKey string
	ServerLogPath   string

	lastFixtureName string
	serverRuns      int
	serverCmd       *exec.Cmd
	serverLog       *os.File
	serverWaitCh    chan error
	extraProcesses  []*managedProcess
	contextAliases  map[string]contextAlias
}

type Result struct {
	Args   []string
	Stdout string
	Stderr string
	Err    error
}

type cliContextConfig struct {
	Server struct {
		Endpoint  string `yaml:"endpoint"`
		PublicKey string `yaml:"public-key"`
	} `yaml:"server"`
}

type contextAlias struct {
	ConfigHome string
	Name       string
	Dir        string
}

type serverWorkspaceConfig struct {
	Endpoint string `yaml:"endpoint"`
}

type managedProcess struct {
	name    string
	cmd     *exec.Cmd
	log     *os.File
	waitCh  chan error
	logPath string
}

func (r Result) MustSucceed(t testing.TB) {
	t.Helper()
	if r.Err == nil {
		return
	}
	t.Fatalf("command %q failed: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(r.Args, " "), r.Err, r.Stdout, r.Stderr)
}

func NewHarness(t testing.TB, story string) *Harness {
	t.Helper()

	return NewHarnessForRoot(t, "tests/gizclaw-e2e/cmd", story)
}

func NewSetupHarness(t testing.TB, story string) *Harness {
	t.Helper()

	h := NewHarness(t, story)
	h.UseSetupServer()
	return h
}

func NewHarnessForRoot(t testing.TB, storyRoot, story string) *Harness {
	t.Helper()

	return NewPersistentHarnessForRoot(t, storyRoot, story, "")
}

func NewPersistentHarnessForRoot(t testing.TB, storyRoot, story, sandboxDir string) *Harness {
	t.Helper()

	repoRoot := mustRepoRoot(t)
	if sandboxDir == "" {
		sandboxDir = t.TempDir()
	}
	homeDir := filepath.Join(sandboxDir, "home")
	xdgConfigHome := filepath.Join(sandboxDir, "xdg-config")
	serverWorkspace := filepath.Join(sandboxDir, "server-workspace")
	logsDir := filepath.Join(sandboxDir, "logs")
	for _, dir := range []string{homeDir, xdgConfigHome, serverWorkspace, logsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", dir, err)
		}
	}

	h := &Harness{
		t:               t,
		RepoRoot:        repoRoot,
		StoryDir:        filepath.Join(repoRoot, storyRoot, story),
		SandboxDir:      sandboxDir,
		HomeDir:         homeDir,
		XDGConfigHome:   xdgConfigHome,
		ServerWorkspace: serverWorkspace,
		LogsDir:         logsDir,
		BinaryPath:      mustBuildCLI(t, repoRoot),
	}
	t.Cleanup(func() { h.StopAllProcesses() })
	return h
}

func (h *Harness) UseSetupServer() {
	h.t.Helper()

	workspaceDir := filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "server-workspace")
	configPath := filepath.Join(workspaceDir, "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		h.t.Fatalf("read setup server config %q: %v", configPath, err)
	}
	var cfg serverWorkspaceConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		h.t.Fatalf("parse setup server config %q: %v", configPath, err)
	}
	keyPair, err := loadIdentity(filepath.Join(workspaceDir, "identity.key"))
	if err != nil {
		h.t.Fatalf("load setup server identity: %v", err)
	}
	serverAddr := serverWorkspaceEndpoint(cfg)
	if strings.TrimSpace(serverAddr) == "" {
		h.t.Fatalf("setup server config %q has empty server address", configPath)
	}

	h.ServerWorkspace = workspaceDir
	h.ServerAddr = serverAddr
	h.ServerPublicKey = keyPair.Public.String()
	h.applySetupContextServer()
	h.waitForSetupServerReady()
}

func (h *Harness) applySetupContextServer() {
	h.t.Helper()

	setupConfigHome := strings.TrimSpace(os.Getenv("GIZCLAW_E2E_CONFIG_HOME"))
	setupContext := strings.TrimSpace(os.Getenv("GIZCLAW_E2E_ADMIN_CONTEXT"))
	if setupConfigHome == "" && setupContext == "" {
		return
	}
	if setupConfigHome == "" {
		setupConfigHome = e2eConfigHome(h.RepoRoot)
	}
	if setupContext == "" {
		setupContext = "admin"
	}

	configPath := filepath.Join(setupConfigHome, "gizclaw", setupContext, "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		h.t.Fatalf("read setup context config %q: %v", configPath, err)
	}
	var cfg cliContextConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		h.t.Fatalf("parse setup context config %q: %v", configPath, err)
	}
	if strings.TrimSpace(cliContextDialAddr(cfg)) == "" {
		h.t.Fatalf("setup context config %q has empty server address", configPath)
	}
	if strings.TrimSpace(cfg.Server.PublicKey) == "" {
		h.t.Fatalf("setup context config %q has empty server public key", configPath)
	}

	h.ServerAddr = cliContextDialAddr(cfg)
	h.ServerPublicKey = strings.TrimSpace(cfg.Server.PublicKey)
}

func (h *Harness) StartServerFromFixture(fixtureName string) {
	h.t.Helper()

	if h.ServerAddr == "" {
		h.ServerAddr = allocateUDPAddr(h.t)
	}
	h.lastFixtureName = fixtureName
	h.PrepareServerWorkspaceFromFixture(fixtureName)
	h.MigrateServerWorkspace()
	h.startServerProcess()
}

func (h *Harness) PrepareServerWorkspaceFromFixture(fixtureName string) {
	h.t.Helper()

	h.lastFixtureName = fixtureName
	h.renderServerFixture(fixtureName, map[string]string{
		fixtureListenAddrToken: h.ServerAddr,
	})
}

func (h *Harness) RestartServer() {
	h.t.Helper()

	h.StopServer()
	h.startServerProcess()
}

func (h *Harness) MigrateServerWorkspace() {
	h.t.Helper()

	result := h.RunCLI("migrate", "--workspace", h.ServerWorkspace)
	result.MustSucceed(h.t)
}

func (h *Harness) StopServer() {
	h.t.Helper()
	h.stopServer()
}

func (h *Harness) StopAllProcesses() {
	h.t.Helper()
	for i := len(h.extraProcesses) - 1; i >= 0; i-- {
		h.extraProcesses[i].stop(h.t)
	}
	h.extraProcesses = nil
	h.stopServer()
}

func (h *Harness) startServerProcess() {
	h.t.Helper()

	h.serverRuns++
	logPath := filepath.Join(h.LogsDir, fmt.Sprintf("server-%02d.log", h.serverRuns))
	logFile, err := os.Create(logPath)
	if err != nil {
		h.t.Fatalf("create server log: %v", err)
	}

	cmd := exec.Command(h.BinaryPath, "serve", "--force", h.ServerWorkspace)
	cmd.Dir = h.RepoRoot
	cmd.Env = h.baseEnv()
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		h.t.Fatalf("start server: %v", err)
	}

	h.serverCmd = cmd
	h.serverLog = logFile
	h.ServerLogPath = logPath
	waitCh := make(chan error, 1)
	h.serverWaitCh = waitCh
	go func() {
		waitCh <- cmd.Wait()
		close(waitCh)
	}()

	h.waitForServerIdentity()
	h.waitForServerReady()
}

func (h *Harness) CreateContext(name string) Result {
	h.t.Helper()
	return h.CreateContextWith(name, h.ServerAddr, h.ServerPublicKey)
}

func (h *Harness) CreateContextWith(name, serverAddr, serverPublicKey string) Result {
	h.t.Helper()
	args := []string{
		"context", "create", name,
		"--server", serverAddr,
		"--public-key", serverPublicKey,
	}
	h.t.Logf("create context %s endpoint=%s", name, serverAddr)
	return h.RunCLI(args...)
}

func (h *Harness) EnsureContext(name string) Result {
	h.t.Helper()

	contextDir := filepath.Join(h.contextRoot(), name)
	identityPath := filepath.Join(contextDir, "identity.key")
	if _, err := os.Stat(identityPath); os.IsNotExist(err) {
		result := h.CreateContext(name)
		if result.Err != nil {
			return result
		}
	} else if err != nil {
		return Result{Args: []string{"ensure-context", name}, Err: err, Stderr: err.Error()}
	}

	cfg := cliContextConfig{}
	cfg.Server.Endpoint = h.ServerAddr
	cfg.Server.PublicKey = h.ServerPublicKey
	h.t.Logf("ensure context %s endpoint=%s", name, cfg.Server.Endpoint)
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return Result{Args: []string{"ensure-context", name}, Err: err, Stderr: err.Error()}
	}
	if err := os.WriteFile(filepath.Join(contextDir, "config.yaml"), data, 0o600); err != nil {
		return Result{Args: []string{"ensure-context", name}, Err: err, Stderr: err.Error()}
	}
	return h.UseContext(name)
}

func (h *Harness) InstallFixedAdminContext(name string) Result {
	h.t.Helper()

	contextDir := h.contextDir(name)
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		return Result{Args: []string{"install-fixed-admin-context", name}, Err: err, Stderr: err.Error()}
	}
	fixtureIdentity := filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "identities", "admin", "identity.key")
	identityData, err := os.ReadFile(fixtureIdentity)
	if err != nil {
		return Result{Args: []string{"install-fixed-admin-context", name}, Err: err, Stderr: err.Error()}
	}
	if len(identityData) != giznet.KeySize {
		err := fmt.Errorf("fixed admin identity has %d bytes, want %d", len(identityData), giznet.KeySize)
		return Result{Args: []string{"install-fixed-admin-context", name}, Err: err, Stderr: err.Error()}
	}
	if err := os.WriteFile(filepath.Join(contextDir, "identity.key"), identityData, 0o600); err != nil {
		return Result{Args: []string{"install-fixed-admin-context", name}, Err: err, Stderr: err.Error()}
	}

	cfg := cliContextConfig{}
	cfg.Server.Endpoint = h.ServerAddr
	cfg.Server.PublicKey = h.ServerPublicKey
	h.t.Logf("install fixed admin context %s endpoint=%s", name, cfg.Server.Endpoint)
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return Result{Args: []string{"install-fixed-admin-context", name}, Err: err, Stderr: err.Error()}
	}
	if err := os.WriteFile(filepath.Join(contextDir, "config.yaml"), data, 0o600); err != nil {
		return Result{Args: []string{"install-fixed-admin-context", name}, Err: err, Stderr: err.Error()}
	}
	return h.UseContext(name)
}

func (h *Harness) RegisterContext(name string, extraArgs ...string) Result {
	h.t.Helper()

	info, err := h.deviceInfoFromArgs(name, extraArgs...)
	if err != nil {
		return Result{Args: append([]string{"register-context", name}, extraArgs...), Err: err, Stderr: err.Error()}
	}
	c, err := h.connectClientFromContext(name)
	if err != nil {
		return Result{Args: []string{"register-context", name}, Err: err, Stderr: err.Error()}
	}
	defer c.Close()
	rpcReq, err := convertHarnessAPIType[rpcapi.ServerPutInfoRequest](info)
	if err != nil {
		return Result{Args: []string{"register-context", name}, Err: err, Stderr: err.Error()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), readyTimeout)
	defer cancel()
	resp, err := c.PutServerInfo(ctx, "server.info.put", rpcReq)
	if err != nil {
		return Result{Args: []string{"register-context", name}, Err: err, Stderr: err.Error()}
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return Result{Args: []string{"register-context", name}, Err: err, Stderr: err.Error()}
	}
	return Result{Args: append([]string{"register-context", name}, extraArgs...), Stdout: string(data)}
}

func (h *Harness) deviceInfoFromArgs(_ string, extraArgs ...string) (apitypes.DeviceInfo, error) {
	device := apitypes.DeviceInfo{
		Hardware: &apitypes.HardwareInfo{},
	}
	for i := 0; i < len(extraArgs); i++ {
		flag := extraArgs[i]
		if !strings.HasPrefix(flag, "--") {
			return apitypes.DeviceInfo{}, fmt.Errorf("unexpected register arg %q", flag)
		}
		if i+1 >= len(extraArgs) {
			return apitypes.DeviceInfo{}, fmt.Errorf("missing value for %s", flag)
		}
		value := extraArgs[i+1]
		i++
		switch flag {
		case "--name":
			device.Name = &value
		case "--sn":
			device.Sn = &value
		case "--manufacturer":
			device.Hardware.Manufacturer = &value
		case "--model":
			device.Hardware.Model = &value
		case "--hardware-revision":
			device.Hardware.HardwareRevision = &value
		default:
			return apitypes.DeviceInfo{}, fmt.Errorf("unsupported register arg %q", flag)
		}
	}
	return device, nil
}

func convertHarnessAPIType[T any](value any) (T, error) {
	var out T
	data, err := json.Marshal(value)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (h *Harness) WaitForPing(contextName string) {
	h.t.Helper()

	if _, err := h.RunCLIUntilSuccess("connect", "ping", "--context", contextName); err != nil {
		h.t.Fatalf("context %q did not become ping-ready: %v", contextName, err)
	}
}

func (h *Harness) UseContext(name string) Result {
	h.t.Helper()
	return h.RunCLI("context", "use", name)
}

func (h *Harness) ListContexts() Result {
	h.t.Helper()
	return h.RunCLI("context", "list")
}

func (h *Harness) ContextPublicKey(name string) string {
	h.t.Helper()

	keyPair := h.ContextKeyPair(name)
	return keyPair.Public.String()
}

func (h *Harness) ContextKeyPair(name string) *giznet.KeyPair {
	h.t.Helper()

	keyPair, err := loadIdentity(filepath.Join(h.contextDir(name), "identity.key"))
	if err != nil {
		h.t.Fatalf("load context %q identity: %v", name, err)
	}
	return keyPair
}

func (h *Harness) PublicHTTPURL() string {
	return "http://" + h.ServerAddr
}

func (h *Harness) PublicHTTPLogin(name string) publiclogin.LoginResponse {
	h.t.Helper()

	var serverPublicKey giznet.PublicKey
	if err := serverPublicKey.UnmarshalText([]byte(h.ServerPublicKey)); err != nil {
		h.t.Fatalf("parse server public key: %v", err)
	}
	assertion, err := publiclogin.NewLoginAssertion(h.ContextKeyPair(name), serverPublicKey, time.Minute)
	if err != nil {
		h.t.Fatalf("create login assertion: %v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, h.PublicHTTPURL()+"/api/public/login", nil)
	if err != nil {
		h.t.Fatalf("create login request: %v", err)
	}
	req.Header.Set("X-Public-Key", h.ContextPublicKey(name))
	req.Header.Set("Authorization", "Bearer "+assertion)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatalf("public http login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		h.t.Fatalf("public http login status = %d body=%s", resp.StatusCode, string(body))
	}
	var result publiclogin.LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		h.t.Fatalf("decode public http login response: %v", err)
	}
	return result
}

func (h *Harness) ConnectClientFromContext(name string) *gizcli.Client {
	h.t.Helper()

	client, err := h.connectClientFromContext(name)
	if err != nil {
		h.t.Fatalf("connect client from context %q: %v", name, err)
	}
	return client
}

func (h *Harness) SetContextAlias(alias, configHome, contextName string) {
	h.t.Helper()
	alias = strings.TrimSpace(alias)
	configHome = strings.TrimSpace(configHome)
	contextName = strings.TrimSpace(contextName)
	if alias == "" {
		h.t.Fatal("context alias is required")
	}
	if configHome == "" {
		h.t.Fatalf("context alias %q config home is required", alias)
	}
	if contextName == "" {
		h.t.Fatalf("context alias %q context name is required", alias)
	}
	if h.contextAliases == nil {
		h.contextAliases = make(map[string]contextAlias)
	}
	h.contextAliases[alias] = contextAlias{ConfigHome: configHome, Name: contextName}
	configPath := filepath.Join(configHome, "gizclaw", contextName, "config.yaml")
	if data, err := os.ReadFile(configPath); err == nil {
		var cfg cliContextConfig
		if err := yaml.Unmarshal(data, &cfg); err == nil {
			h.t.Logf("context alias %s -> %s/%s endpoint=%s", alias, configHome, contextName, cliContextDialAddr(cfg))
		}
	}
}

func (h *Harness) SetContextDirAlias(alias, contextDir string) {
	h.t.Helper()
	alias = strings.TrimSpace(alias)
	contextDir = strings.TrimSpace(contextDir)
	if alias == "" {
		h.t.Fatal("context alias is required")
	}
	if contextDir == "" {
		h.t.Fatalf("context alias %q dir is required", alias)
	}
	if h.contextAliases == nil {
		h.contextAliases = make(map[string]contextAlias)
	}
	h.contextAliases[alias] = contextAlias{Dir: contextDir}
	configPath := filepath.Join(contextDir, "config.yaml")
	if data, err := os.ReadFile(configPath); err == nil {
		var cfg cliContextConfig
		if err := yaml.Unmarshal(data, &cfg); err == nil {
			h.t.Logf("context alias %s -> %s endpoint=%s", alias, contextDir, cliContextDialAddr(cfg))
		}
	}
}

func (h *Harness) RunCLI(args ...string) Result {
	h.t.Helper()

	cmd := exec.Command(h.BinaryPath, args...)
	cmd.Dir = h.SandboxDir
	cmd.Env = h.baseEnv()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return Result{
		Args:   append([]string(nil), args...),
		Stdout: stdout.String(),
		Stderr: stderr.String(),
		Err:    err,
	}
}

func (h *Harness) connectClientFromContext(name string) (*gizcli.Client, error) {
	contextDir := h.contextDir(name)
	data, err := os.ReadFile(filepath.Join(contextDir, "config.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read context config: %w", err)
	}

	var cfg cliContextConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse context config: %w", err)
	}

	keyPair, err := loadIdentity(filepath.Join(contextDir, "identity.key"))
	if err != nil {
		return nil, fmt.Errorf("load context identity: %w", err)
	}

	serverPublicKey := h.serverPublicKeyFromContextConfig(cfg)
	h.t.Logf("connect context %s endpoint=%s", name, cliContextDialAddr(cfg))

	client := &gizcli.Client{
		KeyPair:       keyPair,
		DialTransport: e2eDialTransport(cfg),
	}
	if err := client.Dial(serverPublicKey, cliContextDialAddr(cfg)); err != nil {
		_ = client.Close()
		return nil, err
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Serve()
	}()

	deadline := time.Now().Add(readyTimeout)
	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			if err != nil {
				_ = client.Close()
				return nil, err
			}
			_ = client.Close()
			return nil, fmt.Errorf("client stopped before ready")
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
		err := probeServerPublicReady(ctx, client)
		cancel()
		if err == nil {
			return client, nil
		}

		time.Sleep(10 * time.Millisecond)
	}

	_ = client.Close()
	return nil, fmt.Errorf("timeout waiting for client readiness")
}

func (h *Harness) RunCLIUntilSuccess(args ...string) (Result, error) {
	var last Result
	if err := waitUntil(readyTimeout, func() error {
		last = h.RunCLI(args...)
		if last.Err != nil {
			return fmt.Errorf("command %q failed: %w\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), last.Err, last.Stdout, last.Stderr)
		}
		return nil
	}); err != nil {
		return last, fmt.Errorf("command %q did not succeed before timeout: %w", strings.Join(args, " "), err)
	}
	return last, nil
}

func (h *Harness) waitForServerIdentity() {
	h.t.Helper()

	identityPath := filepath.Join(h.ServerWorkspace, "identity.key")
	if err := waitUntil(readyTimeout, func() error {
		if err := h.serverProcessError(); err != nil {
			return err
		}
		keyPair, err := loadIdentity(identityPath)
		if err != nil {
			return err
		}
		h.ServerPublicKey = keyPair.Public.String()
		return nil
	}); err != nil {
		h.t.Fatalf("server identity not ready: %v\nserver log: %s", err, h.ServerLogPath)
	}
}

func (h *Harness) waitForServerReady() {
	h.t.Helper()

	var serverPublicKey giznet.PublicKey
	if err := serverPublicKey.UnmarshalText([]byte(h.ServerPublicKey)); err != nil {
		h.t.Fatalf("parse server public key: %v", err)
	}

	if err := waitUntil(readyTimeout, func() error {
		if err := h.serverProcessError(); err != nil {
			return err
		}

		keyPair, err := giznet.GenerateKeyPair()
		if err != nil {
			return err
		}
		cfg := cliContextConfig{}
		cfg.Server.Endpoint = h.ServerAddr
		client := &gizcli.Client{KeyPair: keyPair, DialTransport: e2eDialTransport(cfg)}
		if err := client.Dial(serverPublicKey, h.ServerAddr); err != nil {
			_ = client.Close()
			return err
		}
		errCh := make(chan error, 1)
		go func() {
			errCh <- client.Serve()
		}()
		defer client.Close()

		for range 20 {
			select {
			case err := <-errCh:
				if err != nil {
					return err
				}
				return fmt.Errorf("client stopped before ready")
			default:
			}

			ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
			err = probeServerPublicReady(ctx, client)
			cancel()
			if err == nil {
				return nil
			}
			time.Sleep(50 * time.Millisecond)
		}
		return fmt.Errorf("server public probe did not become ready")
	}); err != nil {
		h.t.Fatalf("server did not become ready: %v\nserver log: %s", err, h.ServerLogPath)
	}
}

func (h *Harness) waitForSetupServerReady() {
	h.t.Helper()

	setupConfigHome := e2eConfigHome(h.RepoRoot)
	setupContext := setupAdminContextName()
	if err := h.probeSetupServer(setupConfigHome, setupContext, 2*time.Second); err != nil {
		startScript := filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "setup", "start-server.sh")
		cmd := exec.Command(startScript)
		cmd.Dir = h.RepoRoot
		output, startErr := cmd.CombinedOutput()
		if startErr != nil {
			h.t.Fatalf("start setup server: %v\n%s", startErr, string(output))
		}
	}
	if err := waitUntil(readyTimeout, func() error {
		return h.probeSetupServer(setupConfigHome, setupContext, 2*time.Second)
	}); err != nil {
		h.t.Fatalf("setup server did not become ready: %v\nrun tests/gizclaw-e2e/setup/start-server.sh before setup-driven cmd tests", err)
	}
}

func e2eConfigHome(repoRoot string) string {
	if value := strings.TrimSpace(os.Getenv("GIZCLAW_E2E_CONFIG_HOME")); value != "" {
		if !filepath.IsAbs(value) {
			return filepath.Join(repoRoot, value)
		}
		return value
	}
	return filepath.Join(repoRoot, "tests", "gizclaw-e2e", "testdata", "cmd-config-home")
}

func setupAdminContextName() string {
	if value := strings.TrimSpace(os.Getenv("GIZCLAW_E2E_ADMIN_CONTEXT")); value != "" {
		return value
	}
	return "admin"
}

func (h *Harness) probeSetupServer(setupConfigHome, setupContext string, timeout time.Duration) error {
	h.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, h.BinaryPath, "connect", "ping", "--context", setupContext)
	cmd.Dir = h.RepoRoot
	cmd.Env = append(os.Environ(),
		"HOME="+h.HomeDir,
		"XDG_CONFIG_HOME="+setupConfigHome,
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("setup ping failed: %w\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	return nil
}

func (h *Harness) renderServerFixture(fixtureName string, replacements map[string]string) {
	h.t.Helper()

	var data []byte
	if fixtureName == "server_config.yaml" {
		fixturePath := filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "server-workspace", "config.yaml")
		var err error
		data, err = os.ReadFile(fixturePath)
		if err != nil {
			h.t.Fatalf("read shared server fixture %q: %v", fixturePath, err)
		}
	} else {
		fixturePath := filepath.Join(h.StoryDir, fixtureName)
		var err error
		data, err = os.ReadFile(fixturePath)
		if err != nil {
			h.t.Fatalf("read fixture %q: %v", fixturePath, err)
		}
	}

	rendered := string(data)
	for old, newValue := range replacements {
		rendered = strings.ReplaceAll(rendered, old, newValue)
	}
	if listenAddr := replacements[fixtureListenAddrToken]; listenAddr != "" {
		h.ServerAddr = listenAddr
		rendered = strings.ReplaceAll(rendered, "endpoint: 127.0.0.1:9820", "endpoint: "+listenAddr)
		rendered = strings.ReplaceAll(rendered, `endpoint: "127.0.0.1:9820"`, fmt.Sprintf(`endpoint: "%s"`, listenAddr))
	}

	targetPath := filepath.Join(h.ServerWorkspace, "config.yaml")
	if err := os.WriteFile(targetPath, []byte(rendered), 0o644); err != nil {
		h.t.Fatalf("write rendered config %q: %v", targetPath, err)
	}
}

func (h *Harness) baseEnv() []string {
	env := os.Environ()
	env = append(env,
		"HOME="+h.HomeDir,
		"XDG_CONFIG_HOME="+h.XDGConfigHome,
	)
	return env
}

func (h *Harness) contextRoot() string {
	return filepath.Join(h.XDGConfigHome, "gizclaw")
}

func (h *Harness) contextDir(name string) string {
	if alias, ok := h.contextAliases[name]; ok {
		if alias.Dir != "" {
			return alias.Dir
		}
		return filepath.Join(alias.ConfigHome, "gizclaw", alias.Name)
	}
	return filepath.Join(h.contextRoot(), name)
}

func (h *Harness) serverPublicKeyFromContextConfig(cfg cliContextConfig) giznet.PublicKey {
	h.t.Helper()
	var serverPublicKey giznet.PublicKey
	if err := serverPublicKey.UnmarshalText([]byte(strings.TrimSpace(cfg.Server.PublicKey))); err != nil {
		h.t.Fatalf("parse context server public key: %v", err)
	}
	if serverPublicKey.IsZero() {
		h.t.Fatal("context server public key is zero")
	}
	return serverPublicKey
}

func e2eDialTransport(cfg cliContextConfig) gizcli.DialTransportFunc {
	return func(key *giznet.KeyPair, serverPK giznet.PublicKey, serverAddr string, securityPolicy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
		if strings.TrimSpace(cfg.Server.Endpoint) == "" {
			cfg.Server.Endpoint = serverAddr
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return gizwebrtc.Dial(ctx, key, serverPK, gizwebrtc.DialConfig{
			SignalingURL:   cliContextSignalingURL(cfg),
			SecurityPolicy: securityPolicy,
		})
	}
}

func cliContextDialAddr(cfg cliContextConfig) string {
	return strings.TrimSpace(cfg.Server.Endpoint)
}

func cliContextSignalingURL(cfg cliContextConfig) string {
	return "http://" + cliContextDialAddr(cfg) + gizwebrtc.SignalingPath
}

func serverWorkspaceEndpoint(cfg serverWorkspaceConfig) string {
	return strings.TrimSpace(cfg.Endpoint)
}

func (h *Harness) serverProcessError() error {
	if h.serverWaitCh == nil {
		return nil
	}
	select {
	case err, ok := <-h.serverWaitCh:
		h.serverWaitCh = nil
		if !ok {
			return fmt.Errorf("server exited before readiness")
		}
		if err != nil {
			return fmt.Errorf("server exited early: %w", err)
		}
		return fmt.Errorf("server exited before readiness")
	default:
		return nil
	}
}

func (h *Harness) stopServer() {
	if h.serverCmd == nil {
		return
	}

	defer func() {
		if failed, ok := h.t.(interface{ Failed() bool }); ok && failed.Failed() && h.ServerLogPath != "" {
			if data, err := os.ReadFile(h.ServerLogPath); err == nil && len(data) > 0 {
				h.t.Logf("CLI integration server log contents:\n%s", string(data))
			}
		}
		if h.serverLog != nil {
			_ = h.serverLog.Close()
		}
		if failed, ok := h.t.(interface{ Failed() bool }); ok && failed.Failed() {
			h.t.Logf("CLI integration server log: %s", h.ServerLogPath)
		}
		h.serverCmd = nil
		h.serverLog = nil
		h.serverWaitCh = nil
	}()

	if h.serverCmd.Process == nil {
		return
	}

	_ = h.serverCmd.Process.Signal(os.Interrupt)

	if h.serverWaitCh != nil {
		select {
		case <-h.serverWaitCh:
		case <-time.After(serverStopTimeout):
			_ = h.serverCmd.Process.Kill()
			<-h.serverWaitCh
		}
	}
}

func (p *managedProcess) stop(t testing.TB) {
	t.Helper()
	defer func() {
		if p.log != nil {
			_ = p.log.Close()
		}
	}()
	if p.cmd == nil || p.cmd.Process == nil {
		return
	}
	if p.cmd.ProcessState != nil && p.cmd.ProcessState.Exited() {
		return
	}
	_ = p.cmd.Process.Signal(os.Interrupt)
	select {
	case <-p.waitCh:
	case <-time.After(serverStopTimeout):
		_ = p.cmd.Process.Kill()
		<-p.waitCh
	}
}

func (p *managedProcess) errorIfExited() error {
	select {
	case err, ok := <-p.waitCh:
		if !ok {
			return fmt.Errorf("%s exited before readiness", p.name)
		}
		if err != nil {
			return fmt.Errorf("%s exited early: %w", p.name, err)
		}
		return fmt.Errorf("%s exited before readiness", p.name)
	default:
		return nil
	}
}

func allocateUDPAddr(t testing.TB) string {
	t.Helper()
	for range 20 {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("allocateUDPAddr tcp: %v", err)
		}
		port := l.Addr().(*net.TCPAddr).Port
		pc, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			_ = pc.Close()
			_ = l.Close()
			return fmt.Sprintf("127.0.0.1:%d", port)
		}
		_ = l.Close()
	}
	t.Fatalf("allocateUDPAddr: could not find a TCP/UDP-free port")
	return ""
}

func waitUntil(timeout time.Duration, check func() error) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := check(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(pollInterval)
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("condition not satisfied before timeout")
}

func mustRepoRoot(t testing.TB) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve tests/gizclaw-e2e/cmd harness path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func mustBuildCLI(t testing.TB, repoRoot string) string {
	t.Helper()

	buildBinaryOnce.Do(func() {
		binaryPath := filepath.Join(repoRoot, "tests", "gizclaw-e2e", "testdata", "bin", "gizclaw")
		if info, err := os.Stat(binaryPath); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			buildBinaryPath = binaryPath
			return
		}
		if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
			buildBinaryErr = err
			return
		}

		cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/gizclaw")
		cmd.Dir = repoRoot
		output, err := cmd.CombinedOutput()
		if err != nil {
			buildBinaryErr = fmt.Errorf("build gizclaw CLI: %w\n%s", err, string(output))
			return
		}
		buildBinaryPath = binaryPath
	})

	if buildBinaryErr != nil {
		t.Fatalf("build CLI binary: %v", buildBinaryErr)
	}
	return buildBinaryPath
}

func loadIdentity(path string) (*giznet.KeyPair, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) != giznet.KeySize {
		return nil, fmt.Errorf("invalid key file: got %d bytes, want %d", len(data), giznet.KeySize)
	}
	var key giznet.Key
	copy(key[:], data)
	return giznet.NewKeyPair(key)
}

func probeServerPublicReady(ctx context.Context, client *gizcli.Client) error {
	api, err := client.ServerPublicClient()
	if err != nil {
		return err
	}
	resp, err := api.GetServerInfoWithResponse(ctx)
	if err != nil {
		return err
	}
	if resp.JSON200 == nil {
		if resp.StatusCode() != 0 {
			return fmt.Errorf("unexpected server info status %d", resp.StatusCode())
		}
		return fmt.Errorf("missing server info response body")
	}
	var _ apitypes.ServerInfo = *resp.JSON200
	return nil
}
