package tray

import "sync"

type Pod struct {
	ID      string
	Label   string
	Section string
}

type Callbacks struct {
	OpenWindow func()
	OpenPod    func(string)
	Quit       func()
}

type Labels struct {
	OpenWindow string
	Quit       string
}

type Manager struct {
	mu      sync.Mutex
	backend platformBackend
}

type platformBackend interface {
	Start([]Pod)
	Update([]Pod)
	Stop()
}

func New(callbacks Callbacks, labels Labels) *Manager {
	return &Manager{backend: newPlatformBackend(callbacks, labels)}
}

func (m *Manager) Start(pods []Pod) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.backend.Start(append([]Pod(nil), pods...))
}

func (m *Manager) Update(pods []Pod) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.backend.Update(append([]Pod(nil), pods...))
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.backend.Stop()
}
