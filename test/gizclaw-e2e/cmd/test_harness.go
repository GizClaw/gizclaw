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

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/gearservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	itest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/testutil"
	"github.com/goccy/go-yaml"
)

const (
	fixtureListenAddrToken = "__LISTEN_ADDR__"
	serviceRunWorkspaceArg = "--service-run-workspace"
	serverStopTimeout      = 5 * time.Second
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
}

type Result struct {
	Args   []string
	Stdout string
	Stderr string
	Err    error
}

type cliContextConfig struct {
	Server struct {
		Address   string `yaml:"address"`
		PublicKey string `yaml:"public-key"`
	} `yaml:"server"`
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

func (h *Harness) StartServerFromFixture(fixtureName string) {
	h.t.Helper()

	if h.ServerAddr == "" {
		h.ServerAddr = itest.AllocateUDPAddr(h.t)
	}
	h.lastFixtureName = fixtureName
	h.PrepareServerWorkspaceFromFixture(fixtureName)
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

	cmd := exec.Command(h.BinaryPath, serviceRunWorkspaceArg, h.ServerWorkspace)
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
	h.serverWaitCh = make(chan error, 1)
	go func() {
		h.serverWaitCh <- cmd.Wait()
		close(h.serverWaitCh)
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
	return h.RunCLI(
		"context", "create", name,
		"--server", serverAddr,
		"--pubkey", serverPublicKey,
	)
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
	cfg.Server.Address = h.ServerAddr
	cfg.Server.PublicKey = h.ServerPublicKey
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return Result{Args: []string{"ensure-context", name}, Err: err, Stderr: err.Error()}
	}
	if err := os.WriteFile(filepath.Join(contextDir, "config.yaml"), data, 0o644); err != nil {
		return Result{Args: []string{"ensure-context", name}, Err: err, Stderr: err.Error()}
	}
	return h.UseContext(name)
}

func (h *Harness) RegisterContext(name string, extraArgs ...string) Result {
	h.t.Helper()

	req, err := h.registrationRequest(name, extraArgs...)
	if err != nil {
		return Result{Args: append([]string{"register-context", name}, extraArgs...), Err: err, Stderr: err.Error()}
	}
	c, err := h.connectClientFromContext(name)
	if err != nil {
		return Result{Args: []string{"register-context", name}, Err: err, Stderr: err.Error()}
	}
	defer c.Close()
	rpcReq, err := convertHarnessAPIType[rpcapi.GearRegisterRequest](req)
	if err != nil {
		return Result{Args: []string{"register-context", name}, Err: err, Stderr: err.Error()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), itest.ReadyTimeout)
	defer cancel()
	resp, err := c.RegisterGear(ctx, "gear.registration.register", rpcReq)
	if err != nil {
		return Result{Args: []string{"register-context", name}, Err: err, Stderr: err.Error()}
	}
	result, err := convertHarnessAPIType[gearservice.RegistrationResult](*resp)
	if err != nil {
		return Result{Args: []string{"register-context", name}, Err: err, Stderr: err.Error()}
	}
	data, err := json.Marshal(result)
	if err != nil {
		return Result{Args: []string{"register-context", name}, Err: err, Stderr: err.Error()}
	}
	return Result{Args: append([]string{"register-context", name}, extraArgs...), Stdout: string(data)}
}

func (h *Harness) registrationRequest(_ string, extraArgs ...string) (gearservice.RegistrationRequest, error) {
	req := gearservice.RegistrationRequest{}
	device := apitypes.DeviceInfo{
		Hardware: &apitypes.HardwareInfo{},
	}
	for i := 0; i < len(extraArgs); i++ {
		flag := extraArgs[i]
		if !strings.HasPrefix(flag, "--") {
			return gearservice.RegistrationRequest{}, fmt.Errorf("unexpected register arg %q", flag)
		}
		if i+1 >= len(extraArgs) {
			return gearservice.RegistrationRequest{}, fmt.Errorf("missing value for %s", flag)
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
		case "--depot":
			device.Hardware.Depot = &value
		case "--firmware-semver":
			device.Hardware.FirmwareSemver = &value
		default:
			return gearservice.RegistrationRequest{}, fmt.Errorf("unsupported register arg %q", flag)
		}
	}
	req.Device = device
	return req, nil
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

	if _, err := h.RunCLIUntilSuccess("ping", "--context", contextName); err != nil {
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

	keyPair, err := loadIdentity(filepath.Join(h.contextRoot(), name, "identity.key"))
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

func (h *Harness) ConnectClientFromContext(name string) *gizclaw.Client {
	h.t.Helper()

	client, err := h.connectClientFromContext(name)
	if err != nil {
		h.t.Fatalf("connect client from context %q: %v", name, err)
	}
	return client
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

func (h *Harness) connectClientFromContext(name string) (*gizclaw.Client, error) {
	contextDir := filepath.Join(h.contextRoot(), name)
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

	var serverPublicKey giznet.PublicKey
	if err := serverPublicKey.UnmarshalText([]byte(cfg.Server.PublicKey)); err != nil {
		return nil, fmt.Errorf("parse server public key: %w", err)
	}

	client := &gizclaw.Client{KeyPair: keyPair}
	if err := client.Dial(serverPublicKey, cfg.Server.Address); err != nil {
		_ = client.Close()
		return nil, err
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Serve()
	}()

	deadline := time.Now().Add(itest.ReadyTimeout)
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

		ctx, cancel := context.WithTimeout(context.Background(), itest.ProbeTimeout)
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
	if err := itest.WaitUntil(itest.ReadyTimeout, func() error {
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
	if err := itest.WaitUntil(itest.ReadyTimeout, func() error {
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

	if err := itest.WaitUntil(itest.ReadyTimeout, func() error {
		if err := h.serverProcessError(); err != nil {
			return err
		}

		keyPair, err := giznet.GenerateKeyPair()
		if err != nil {
			return err
		}
		client := &gizclaw.Client{KeyPair: keyPair}
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

			ctx, cancel := context.WithTimeout(context.Background(), itest.ProbeTimeout)
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

func (h *Harness) renderServerFixture(fixtureName string, replacements map[string]string) {
	h.t.Helper()

	fixturePath := filepath.Join(h.StoryDir, fixtureName)
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		h.t.Fatalf("read fixture %q: %v", fixturePath, err)
	}

	rendered := string(data)
	for old, newValue := range replacements {
		rendered = strings.ReplaceAll(rendered, old, newValue)
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
	if h.serverCmd.ProcessState != nil && h.serverCmd.ProcessState.Exited() {
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
	client := &http.Client{Timeout: itest.ProbeTimeout}
	return itest.WaitUntil(itest.ReadyTimeout, func() error {
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
		outDir, err := os.MkdirTemp("", "gizclaw-cli-bin-*")
		if err != nil {
			buildBinaryErr = err
			return
		}

		binaryPath := filepath.Join(outDir, "gizclaw")
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

func probeServerPublicReady(ctx context.Context, client *gizclaw.Client) error {
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
