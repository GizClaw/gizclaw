package localserver

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	EnvExecutable = "GIZCLAW_DESKTOP_SERVER_EXECUTABLE"
	PIDFile       = "server.pid"
)

// ErrProcessIdentityMismatch marks a definitive verifier result proving that
// a persisted PID does not belong to the expected local Server.
var ErrProcessIdentityMismatch = errors.New("local server: process identity mismatch")

type Status struct {
	State string   `json:"state"`
	PID   int      `json:"pid,omitempty"`
	Logs  []string `json:"logs,omitempty"`
	Error string   `json:"error,omitempty"`
}

// ProcessVerifier confirms that a live persisted PID belongs to the expected
// local Server before Manager is allowed to signal it.
type ProcessVerifier func(pid int) error

type process struct {
	process *os.Process
	pidPath string
	logs    []string
	err     string
	done    chan struct{}
	state   string
}

type Manager struct {
	Executable  string
	MaxLogLines int

	mu        sync.Mutex
	processes map[string]*process
	closing   bool
}

func New() *Manager {
	return &Manager{MaxLogLines: 250, processes: map[string]*process{}}
}

func (m *Manager) Start(podID, workspace string) (Status, error) {
	m.mu.Lock()
	if m.closing {
		m.mu.Unlock()
		return Status{}, errors.New("local server: manager is shutting down")
	}
	if current := m.processes[podID]; current != nil && (current.state == "running" || current.state == "stopping") {
		status := snapshot(current)
		m.mu.Unlock()
		return status, nil
	}
	pidPath := filepath.Join(workspace, PIDFile)
	if _, found, err := readPID(pidPath); err != nil {
		m.mu.Unlock()
		return Status{}, err
	} else if found {
		m.mu.Unlock()
		return Status{}, errors.New("local server: persisted PID must be verified before start")
	}
	executable, err := m.resolveExecutable()
	if err != nil {
		m.mu.Unlock()
		return Status{}, err
	}
	cmd := exec.Command(executable, "serve", "--force", workspace)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		m.mu.Unlock()
		return Status{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		m.mu.Unlock()
		return Status{}, err
	}
	if err := cmd.Start(); err != nil {
		m.mu.Unlock()
		return Status{}, fmt.Errorf("local server: start: %w", err)
	}
	if err := writePID(pidPath, cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		m.mu.Unlock()
		return Status{}, err
	}
	p := &process{process: cmd.Process, pidPath: pidPath, done: make(chan struct{}), state: "running"}
	m.processes[podID] = p
	m.mu.Unlock()
	go m.capture(p, stdout)
	go m.capture(p, stderr)
	go func() {
		err := cmd.Wait()
		m.finish(p, err)
	}()
	return m.Status(podID), nil
}

// Recover attaches the manager to a local Server recorded in its workspace.
// Attached processes are polled because they are no longer children of the
// current Desktop process and therefore cannot be waited on with exec.Cmd.
func (m *Manager) Recover(podID, workspace string, verify ProcessVerifier) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closing {
		return Status{}, errors.New("local server: manager is shutting down")
	}
	if current := m.processes[podID]; current != nil && (current.state == "running" || current.state == "stopping") {
		return snapshot(current), nil
	}
	p, err := m.recoverLocked(podID, workspace, verify)
	if err != nil {
		done := make(chan struct{})
		close(done)
		m.processes[podID] = &process{done: done, state: "failed", err: err.Error()}
		return Status{}, err
	}
	if p == nil {
		delete(m.processes, podID)
		return Status{State: "stopped"}, nil
	}
	return snapshot(p), nil
}

// ExecutablePath returns the same companion binary used to run local Servers.
// Bootstrap operations use it for the matching Admin CLI surface.
func (m *Manager) ExecutablePath() (string, error) {
	return m.resolveExecutable()
}

func (m *Manager) Stop(ctx context.Context, podID string) (Status, error) {
	m.mu.Lock()
	p := m.processes[podID]
	if p == nil || (p.state != "running" && p.state != "stopping") {
		m.mu.Unlock()
		return Status{State: "stopped"}, nil
	}
	if p.state == "running" {
		p.state = "stopping"
		if err := p.process.Signal(os.Interrupt); err != nil {
			_ = p.process.Kill()
		}
	}
	done := p.done
	m.mu.Unlock()
	select {
	case <-done:
	case <-ctx.Done():
		_ = p.process.Kill()
		<-done
	}
	return m.Status(podID), nil
}

func (m *Manager) Restart(ctx context.Context, podID, workspace string) (Status, error) {
	if _, err := m.Stop(ctx, podID); err != nil {
		return Status{}, err
	}
	return m.Start(podID, workspace)
}

func (m *Manager) Status(podID string) Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.processes[podID]; p != nil {
		return snapshot(p)
	}
	return Status{State: "stopped"}
}

func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	m.closing = true
	processes := make([]*process, 0, len(m.processes))
	for _, p := range m.processes {
		if p.state != "running" && p.state != "stopping" {
			continue
		}
		if p.state == "running" {
			p.state = "stopping"
			if err := p.process.Signal(os.Interrupt); err != nil {
				_ = p.process.Kill()
			}
		}
		processes = append(processes, p)
	}
	m.mu.Unlock()

	done := make(chan struct{})
	go func() {
		for _, p := range processes {
			<-p.done
		}
		close(done)
	}()
	select {
	case <-done:
		return
	case <-ctx.Done():
	}
	for _, p := range processes {
		select {
		case <-p.done:
		default:
			_ = p.process.Kill()
		}
	}
	<-done
}

func (m *Manager) capture(p *process, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		m.mu.Lock()
		p.logs = append(p.logs, scanner.Text())
		if len(p.logs) > m.MaxLogLines {
			p.logs = append([]string(nil), p.logs[len(p.logs)-m.MaxLogLines:]...)
		}
		m.mu.Unlock()
	}
}

func (m *Manager) recoverLocked(podID, workspace string, verify ProcessVerifier) (*process, error) {
	pidPath := filepath.Join(workspace, PIDFile)
	pid, found, err := readPID(pidPath)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	osProcess, err := os.FindProcess(pid)
	if err != nil || !processRunning(osProcess) {
		if osProcess != nil {
			_ = osProcess.Release()
		}
		if removeErr := removePIDIfMatches(pidPath, pid); removeErr != nil {
			return nil, removeErr
		}
		return nil, nil
	}
	if verify == nil {
		_ = osProcess.Release()
		return nil, errors.New("local server: persisted PID verifier is required")
	}
	if err := verify(pid); err != nil {
		_ = osProcess.Release()
		if errors.Is(err, ErrProcessIdentityMismatch) {
			if removeErr := removePIDIfMatches(pidPath, pid); removeErr != nil {
				return nil, removeErr
			}
			return nil, nil
		}
		return nil, fmt.Errorf("local server: verify persisted PID %d: %w", pid, err)
	}
	p := &process{process: osProcess, pidPath: pidPath, done: make(chan struct{}), state: "running"}
	m.processes[podID] = p
	go m.monitorAttached(p)
	return p, nil
}

func (m *Manager) monitorAttached(p *process) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		if processRunning(p.process) {
			continue
		}
		m.finish(p, nil)
		_ = p.process.Release()
		return
	}
}

func (m *Manager) finish(p *process, waitErr error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if waitErr != nil && p.state != "stopping" {
		p.err = waitErr.Error()
		p.state = "failed"
	} else {
		p.state = "stopped"
	}
	if err := removePIDIfMatches(p.pidPath, p.process.Pid); err != nil && p.err == "" {
		p.err = err.Error()
		p.state = "failed"
	}
	close(p.done)
}

func readPID(path string) (int, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("local server: inspect PID file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return 0, false, errors.New("local server: PID file must be a regular file")
	}
	if info.Size() > 32 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return 0, false, fmt.Errorf("local server: remove invalid PID file: %w", err)
		}
		return 0, false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false, fmt.Errorf("local server: read PID file: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err == nil && pid > 0 {
		return pid, true, nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return 0, false, fmt.Errorf("local server: remove invalid PID file: %w", err)
	}
	return 0, false, nil
}

func writePID(path string, pid int) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("local server: create PID directory: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("local server: inspect PID directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("local server: PID directory must be a directory")
	}
	tmp, err := os.CreateTemp(dir, ".server.pid-*")
	if err != nil {
		return fmt.Errorf("local server: create PID file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("local server: secure PID file: %w", err)
	}
	if _, err := fmt.Fprintf(tmp, "%d\n", pid); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("local server: write PID file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("local server: sync PID file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("local server: close PID file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("local server: publish PID file: %w", err)
	}
	return nil
}

func removePIDIfMatches(path string, pid int) error {
	if path == "" {
		return nil
	}
	current, found, err := readPID(path)
	if err != nil || !found || current != pid {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("local server: remove PID file: %w", err)
	}
	return nil
}

func (m *Manager) resolveExecutable() (string, error) {
	if m.Executable != "" {
		return m.Executable, nil
	}
	if current, err := os.Executable(); err == nil {
		candidates := []string{
			filepath.Join(filepath.Dir(current), "..", "Resources", "gizclaw"),
			filepath.Join(filepath.Dir(current), "gizclaw"),
		}
		for _, candidate := range candidates {
			if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0 {
				return filepath.Clean(candidate), nil
			}
		}
	}
	if value := strings.TrimSpace(os.Getenv(EnvExecutable)); value != "" {
		return value, nil
	}
	path, err := exec.LookPath("gizclaw")
	if err != nil {
		return "", fmt.Errorf("local server: gizclaw executable not found; set %s", EnvExecutable)
	}
	return path, nil
}

func snapshot(p *process) Status {
	status := Status{State: p.state, Error: p.err, Logs: append([]string(nil), p.logs...)}
	if p.process != nil && (p.state == "running" || p.state == "stopping") {
		status.PID = p.process.Pid
	}
	return status
}
