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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/gizcli"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/system/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/giznoise"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/gizwebrtc"
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

	BinaryPath       string
	ServerAddr       string
	ServerPublicPort int
	ServerNoisePort  int
	ServerICEPort    int
	ServerPublicKey  string
	ServerCipherMode string
	ServerLogPath    string

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
		Address       string `yaml:"address,omitempty"`
		Host          string `yaml:"host,omitempty"`
		PublicAPIPort int    `yaml:"public-api-port,omitempty"`
		NoiseUDPPort  int    `yaml:"noise-udp-port,omitempty"`
		ICEPort       int    `yaml:"ice-port,omitempty"`
		PublicKey     string `yaml:"public-key"`
		Transport     string `yaml:"transport,omitempty"`
		CipherMode    string `yaml:"cipher-mode,omitempty"`
	} `yaml:"server"`
}

type contextAlias struct {
	ConfigHome string
	Name       string
}

type serverWorkspaceConfig struct {
	Listen        string `yaml:"listen"`
	Host          string `yaml:"host"`
	PublicAPIPort int    `yaml:"public-api-port"`
	NoiseUDPPort  int    `yaml:"noise-udp-port"`
	ICEPort       int    `yaml:"ice-port"`
	CipherMode    string `yaml:"cipher-mode"`
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

	return NewHarnessForRoot(t, "test/gizclaw-e2e/cmd", story)
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

	workspaceDir := filepath.Join(h.RepoRoot, "test", "gizclaw-e2e", "testdata", "server-workspace")
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
	serverAddr := serverWorkspaceNoiseAddr(cfg)
	if strings.TrimSpace(serverAddr) == "" {
		h.t.Fatalf("setup server config %q has empty server address", configPath)
	}

	h.ServerWorkspace = workspaceDir
	h.ServerAddr = serverAddr
	h.ServerPublicPort = defaultPort(cfg.PublicAPIPort, 9820)
	h.ServerNoisePort = defaultPort(cfg.NoiseUDPPort, h.ServerPublicPort)
	h.ServerICEPort = defaultPort(cfg.ICEPort, 9821)
	h.ServerPublicKey = keyPair.Public.String()
	h.ServerCipherMode = strings.TrimSpace(cfg.CipherMode)
	h.applySetupContextServer()
	h.waitForSetupServerReady()
}

func (h *Harness) applySetupContextServer() {
	h.t.Helper()

	setupConfigHome := strings.TrimSpace(os.Getenv("GIZCLAW_E2E_ADMIN_SETUP_CONFIG_HOME"))
	setupContext := strings.TrimSpace(os.Getenv("GIZCLAW_E2E_ADMIN_SETUP_CONTEXT"))
	if setupConfigHome == "" && setupContext == "" {
		return
	}
	if setupConfigHome == "" {
		setupConfigHome = filepath.Join(h.RepoRoot, "test", "gizclaw-e2e", "testdata", "admin-config-home")
	}
	if setupContext == "" {
		setupContext = "e2e-admin"
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
	if strings.TrimSpace(cliContextNoiseAddr(cfg)) == "" {
		h.t.Fatalf("setup context config %q has empty server address", configPath)
	}
	if strings.TrimSpace(cfg.Server.PublicKey) == "" {
		h.t.Fatalf("setup context config %q has empty server public key", configPath)
	}

	h.ServerAddr = cliContextNoiseAddr(cfg)
	h.ServerPublicPort = cfg.Server.PublicAPIPort
	h.ServerNoisePort = cfg.Server.NoiseUDPPort
	h.ServerICEPort = cfg.Server.ICEPort
	h.ServerPublicKey = strings.TrimSpace(cfg.Server.PublicKey)
	h.ServerCipherMode = strings.TrimSpace(cfg.Server.CipherMode)
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
	transport := h.defaultContextTransport()
	_, portText := splitHostPortForHarness(serverAddr)
	port := parsePortForHarness(h.t, portText)
	publicPort := defaultPort(h.ServerPublicPort, port)
	noisePort := defaultPort(h.ServerNoisePort, port)
	icePort := defaultPort(h.ServerICEPort, 9821)
	args := []string{
		"context", "create", name,
		"--server", serverAddr,
		"--public-key", serverPublicKey,
		"--transport", transport,
		"--public-api-port", strconv.Itoa(publicPort),
		"--noise-udp-port", strconv.Itoa(noisePort),
	}
	if transport == "webrtc" {
		args = append(args, "--ice-port", strconv.Itoa(icePort))
	}
	if h.ServerCipherMode != "" {
		args = append(args, "--cipher-mode", h.ServerCipherMode)
	}
	h.t.Logf("create context %s transport=%s public=%d noise=%d ice=%d", name, transport, publicPort, noisePort, icePort)
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
	host, portText := splitHostPortForHarness(h.ServerAddr)
	port := parsePortForHarness(h.t, portText)
	cfg.Server.Host = host
	cfg.Server.PublicAPIPort = defaultPort(h.ServerPublicPort, port)
	cfg.Server.NoiseUDPPort = defaultPort(h.ServerNoisePort, port)
	cfg.Server.ICEPort = defaultPort(h.ServerICEPort, 9821)
	cfg.Server.PublicKey = h.ServerPublicKey
	cfg.Server.Transport = h.defaultContextTransport()
	cfg.Server.CipherMode = h.ServerCipherMode
	h.t.Logf("ensure context %s transport=%s public=%d noise=%d ice=%d", name, cfg.Server.Transport, cfg.Server.PublicAPIPort, cfg.Server.NoiseUDPPort, cfg.Server.ICEPort)
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return Result{Args: []string{"ensure-context", name}, Err: err, Stderr: err.Error()}
	}
	if err := os.WriteFile(filepath.Join(contextDir, "config.yaml"), data, 0o600); err != nil {
		return Result{Args: []string{"ensure-context", name}, Err: err, Stderr: err.Error()}
	}
	return h.UseContext(name)
}

func (h *Harness) defaultContextTransport() string {
	transport := strings.TrimSpace(os.Getenv("GIZCLAW_E2E_CONNECT_TRANSPORT"))
	if transport == "" {
		transport = strings.TrimSpace(os.Getenv("GIZCLAW_E2E_CLIENT_TRANSPORT"))
	}
	if transport == "" && strings.Contains(strings.ToLower(os.Getenv("GIZCLAW_E2E_CLIENT_CONTEXT")), "webrtc") {
		transport = "webrtc"
	}
	if transport == "" {
		return "noise"
	}
	if transport != "noise" && transport != "webrtc" {
		h.t.Fatalf("unsupported e2e connect transport %q", transport)
	}
	return transport
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

func (h *Harness) ApproveAdminPeer(publicKey string) {
	h.t.Helper()

	h.SetContextAlias("setup-admin", filepath.Join(h.RepoRoot, "test", "gizclaw-e2e", "testdata", "admin-config-home"), "e2e-admin")
	admin := h.ConnectClientFromContext("setup-admin")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		h.t.Fatalf("create setup admin API client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := api.ApprovePeerWithResponse(ctx, publicKey, adminservice.ApproveRequest{Role: apitypes.PeerRoleAdmin})
	if err != nil {
		h.t.Fatalf("approve admin peer %s: %v", publicKey, err)
	}
	if resp.StatusCode() != http.StatusOK {
		h.t.Fatalf("approve admin peer %s status = %d: %s", publicKey, resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
	}
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
			h.t.Logf("context alias %s -> %s/%s transport=%s dial=%s", alias, configHome, contextName, cliContextTransport(cfg), cliContextDialAddr(cfg))
		}
	}
}

func (h *Harness) StartUI(kind, contextName string) string {
	h.t.Helper()

	h.UseContext(contextName).MustSucceed(h.t)

	listenAddr := freeTCPAddr(h.t)
	logPath := filepath.Join(h.LogsDir, fmt.Sprintf("%s-%s-ui.log", kind, safeLogName(contextName)))
	logFile, err := os.Create(logPath)
	if err != nil {
		h.t.Fatalf("create %s UI log: %v", kind, err)
	}

	cmd := exec.Command(h.BinaryPath, kind, "--context", contextName, "--listen", listenAddr)
	cmd.Dir = h.RepoRoot
	cmd.Env = h.baseEnv()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		h.t.Fatalf("start %s UI: %v", kind, err)
	}

	process := &managedProcess{
		name:    kind + "-ui",
		cmd:     cmd,
		log:     logFile,
		waitCh:  make(chan error, 1),
		logPath: logPath,
	}
	go func() {
		process.waitCh <- cmd.Wait()
		close(process.waitCh)
	}()
	h.extraProcesses = append(h.extraProcesses, process)

	url := "http://" + listenAddr
	if err := waitForHTTP(url, process); err != nil {
		h.t.Fatalf("wait for %s UI: %v\nlog: %s", kind, err, logPath)
	}
	return url
}

func safeLogName(value string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-")
	return replacer.Replace(value)
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
	h.t.Logf("connect context %s transport=%s dial=%s", name, cliContextTransport(cfg), cliContextDialAddr(cfg))

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
		host, portText := splitHostPortForHarness(h.ServerAddr)
		port := parsePortForHarness(h.t, portText)
		cfg.Server.Host = host
		cfg.Server.PublicAPIPort = port
		cfg.Server.NoiseUDPPort = port
		cfg.Server.Transport = "noise"
		cfg.Server.CipherMode = h.ServerCipherMode
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

	setupConfigHome := setupAdminConfigHome(h.RepoRoot)
	setupContext := setupAdminContextName()
	if err := h.probeSetupServer(setupConfigHome, setupContext, 2*time.Second); err != nil {
		startScript := filepath.Join(h.RepoRoot, "test", "gizclaw-e2e", "setup", "start-server.sh")
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
		h.t.Fatalf("setup server did not become ready: %v\nrun test/gizclaw-e2e/setup/start-server.sh before setup-driven cmd tests", err)
	}
}

func setupAdminConfigHome(repoRoot string) string {
	if value := strings.TrimSpace(os.Getenv("GIZCLAW_E2E_ADMIN_SETUP_CONFIG_HOME")); value != "" {
		return value
	}
	return filepath.Join(repoRoot, "test", "gizclaw-e2e", "testdata", "admin-config-home")
}

func setupAdminContextName() string {
	if value := strings.TrimSpace(os.Getenv("GIZCLAW_E2E_ADMIN_SETUP_CONTEXT")); value != "" {
		return value
	}
	return "e2e-admin"
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
		fixturePath := filepath.Join(h.RepoRoot, "test", "gizclaw-e2e", "testdata", "server-workspace", "config.yaml")
		var err error
		data, err = os.ReadFile(fixturePath)
		if err != nil {
			h.t.Fatalf("read shared server fixture %q: %v", fixturePath, err)
		}
		h.ServerCipherMode = "chacha_poly"
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
		host, port := splitHostPortForHarness(listenAddr)
		_, icePort := splitHostPortForHarness(allocateUDPAddr(h.t))
		h.ServerPublicPort = parsePortForHarness(h.t, port)
		h.ServerNoisePort = h.ServerPublicPort
		h.ServerICEPort = parsePortForHarness(h.t, icePort)
		rendered = strings.ReplaceAll(rendered, `listen: "127.0.0.1:9820"`, fmt.Sprintf(`listen: "%s"`, listenAddr))
		rendered = strings.ReplaceAll(rendered, "host: 127.0.0.1", "host: "+host)
		rendered = strings.ReplaceAll(rendered, "host: 0.0.0.0", "host: "+host)
		rendered = strings.ReplaceAll(rendered, "public-api-port: 9820", "public-api-port: "+port)
		rendered = strings.ReplaceAll(rendered, "noise-udp-port: 9820", "noise-udp-port: "+port)
		rendered = strings.ReplaceAll(rendered, "ice-port: 9821", "ice-port: "+icePort)
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
		if cliContextTransport(cfg) == "webrtc" {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			return gizwebrtc.Dial(ctx, key, serverPK, gizwebrtc.DialConfig{
				SignalingURL:   cliContextSignalingURL(cfg),
				CipherMode:     gizwebrtc.CipherMode(cfg.Server.CipherMode),
				SecurityPolicy: securityPolicy,
			})
		}
		l, err := (&giznoise.ListenConfig{
			Addr:           ":0",
			CipherMode:     giznoise.CipherMode(cfg.Server.CipherMode),
			SecurityPolicy: securityPolicy,
		}).Listen(key)
		if err != nil {
			return nil, nil, err
		}
		udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
		if err != nil {
			_ = l.Close()
			return nil, nil, err
		}
		conn, err := l.Dial(serverPK, udpAddr)
		if err != nil {
			_ = l.Close()
			return nil, nil, err
		}
		return l, conn, nil
	}
}

func cliContextTransport(cfg cliContextConfig) string {
	transport := strings.TrimSpace(cfg.Server.Transport)
	if transport == "" {
		return "noise"
	}
	return transport
}

func cliContextDialAddr(cfg cliContextConfig) string {
	if cliContextTransport(cfg) == "webrtc" {
		return cliContextPublicAPIAddr(cfg)
	}
	return cliContextNoiseAddr(cfg)
}

func cliContextSignalingURL(cfg cliContextConfig) string {
	return "http://" + cliContextPublicAPIAddr(cfg) + gizwebrtc.SignalingPath
}

func cliContextPublicAPIAddr(cfg cliContextConfig) string {
	host, addressPort := cliContextHostAndAddressPort(cfg)
	port := cfg.Server.PublicAPIPort
	if port == 0 {
		port = addressPort
	}
	if port == 0 {
		port = 9820
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func cliContextNoiseAddr(cfg cliContextConfig) string {
	if strings.TrimSpace(cfg.Server.Address) != "" && cfg.Server.NoiseUDPPort == 0 && cfg.Server.Host == "" {
		return strings.TrimSpace(cfg.Server.Address)
	}
	host, addressPort := cliContextHostAndAddressPort(cfg)
	port := cfg.Server.NoiseUDPPort
	if port == 0 {
		port = addressPort
	}
	if port == 0 {
		port = 9820
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func cliContextHostAndAddressPort(cfg cliContextConfig) (string, int) {
	host := strings.TrimSpace(cfg.Server.Host)
	var port int
	if addr := strings.TrimSpace(cfg.Server.Address); addr != "" {
		addrHost, addrPort, err := net.SplitHostPort(addr)
		if err == nil {
			if host == "" {
				host = addrHost
			}
			port, _ = strconv.Atoi(addrPort)
		} else if host == "" {
			host = addr
		}
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return host, port
}

func serverWorkspaceNoiseAddr(cfg serverWorkspaceConfig) string {
	if strings.TrimSpace(cfg.Listen) != "" {
		return strings.TrimSpace(cfg.Listen)
	}
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	port := cfg.NoiseUDPPort
	if port == 0 {
		port = cfg.PublicAPIPort
	}
	if port == 0 {
		port = 9820
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func splitHostPortForHarness(addr string) (string, string) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return "127.0.0.1", "9820"
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return host, port
}

func parsePortForHarness(t testing.TB, port string) int {
	t.Helper()
	n, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("parse port %q: %v", port, err)
	}
	return n
}

func defaultPort(value, fallback int) int {
	if value != 0 {
		return value
	}
	return fallback
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

func waitForHTTP(url string, process *managedProcess) error {
	client := &http.Client{Timeout: probeTimeout}
	return waitUntil(readyTimeout, func() error {
		if err := process.errorIfExited(); err != nil {
			return err
		}
		resp, err := client.Get(url)
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("GET %s status %d", url, resp.StatusCode)
		}
		return nil
	})
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

func freeTCPAddr(t testing.TB) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate TCP addr: %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	return addr
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
		t.Fatal("resolve test/gizclaw-e2e/cmd harness path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func mustBuildCLI(t testing.TB, repoRoot string) string {
	t.Helper()

	buildBinaryOnce.Do(func() {
		binaryPath := filepath.Join(repoRoot, "test", "gizclaw-e2e", "testdata", "bin", "gizclaw")
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
