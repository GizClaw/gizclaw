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
	"strings"
	"sync"
)

const EnvExecutable = "GIZCLAW_DESKTOP_SERVER_EXECUTABLE"

type Status struct {
	State string   `json:"state"`
	PID   int      `json:"pid,omitempty"`
	Logs  []string `json:"logs,omitempty"`
	Error string   `json:"error,omitempty"`
}

type process struct {
	cmd   *exec.Cmd
	logs  []string
	err   string
	done  chan struct{}
	state string
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
	if current := m.processes[podID]; current != nil && current.state == "running" {
		status := snapshot(current)
		m.mu.Unlock()
		return status, nil
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
	p := &process{cmd: cmd, done: make(chan struct{}), state: "running"}
	m.processes[podID] = p
	m.mu.Unlock()
	go m.capture(p, stdout)
	go m.capture(p, stderr)
	go func() {
		err := cmd.Wait()
		m.mu.Lock()
		if err != nil && p.state != "stopping" {
			p.err = err.Error()
			p.state = "failed"
		} else {
			p.state = "stopped"
		}
		close(p.done)
		m.mu.Unlock()
	}()
	return m.Status(podID), nil
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
		if err := p.cmd.Process.Signal(os.Interrupt); err != nil {
			_ = p.cmd.Process.Kill()
		}
	}
	done := p.done
	m.mu.Unlock()
	select {
	case <-done:
	case <-ctx.Done():
		_ = p.cmd.Process.Kill()
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
			if err := p.cmd.Process.Signal(os.Interrupt); err != nil {
				_ = p.cmd.Process.Kill()
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
			_ = p.cmd.Process.Kill()
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
	if p.cmd != nil && p.cmd.Process != nil && p.state == "running" {
		status.PID = p.cmd.Process.Pid
	}
	return status
}
