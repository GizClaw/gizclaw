package agenthost

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
)

var ErrWorkspaceBusy = errors.New("agenthost: workspace already has a running agent")

type Coordinator interface {
	Acquire(context.Context, string) (Lease, error)
}

type Lease interface {
	Workspace() string
	Token() string
	Release(context.Context) error
}

type MemoryCoordinator struct {
	mu     sync.Mutex
	next   atomic.Uint64
	leases map[string]*memoryLease
}

func NewMemoryCoordinator() *MemoryCoordinator {
	return &MemoryCoordinator{leases: make(map[string]*memoryLease)}
}

func (c *MemoryCoordinator) Acquire(ctx context.Context, workspace string) (Lease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if workspace == "" {
		return nil, fmt.Errorf("agenthost: workspace is required")
	}
	if c == nil {
		return nil, fmt.Errorf("agenthost: coordinator is nil")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.leases == nil {
		c.leases = make(map[string]*memoryLease)
	}
	if _, exists := c.leases[workspace]; exists {
		return nil, ErrWorkspaceBusy
	}
	lease := &memoryLease{
		coordinator: c,
		workspace:   workspace,
		token:       strconv.FormatUint(c.next.Add(1), 10),
	}
	c.leases[workspace] = lease
	return lease, nil
}

type memoryLease struct {
	coordinator *MemoryCoordinator
	workspace   string
	token       string
	once        sync.Once
}

func (l *memoryLease) Workspace() string {
	if l == nil {
		return ""
	}
	return l.workspace
}

func (l *memoryLease) Token() string {
	if l == nil {
		return ""
	}
	return l.token
}

func (l *memoryLease) Release(context.Context) error {
	if l == nil || l.coordinator == nil {
		return nil
	}
	l.once.Do(func() {
		l.coordinator.mu.Lock()
		defer l.coordinator.mu.Unlock()
		if current := l.coordinator.leases[l.workspace]; current == l {
			delete(l.coordinator.leases, l.workspace)
		}
	})
	return nil
}
