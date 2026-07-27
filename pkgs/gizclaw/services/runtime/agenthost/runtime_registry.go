package agenthost

import (
	"context"
	"fmt"
	"sync"
)

// RuntimeRegistry keeps one current Agent generation per Workspace and hands
// out reference-counted stream attachments. A reload can publish a replacement
// generation while attachments to the previous generation drain.
type RuntimeRegistry struct {
	mu       sync.Mutex
	runtimes map[string]*workspaceRuntime
}

func NewRuntimeRegistry() *RuntimeRegistry {
	return &RuntimeRegistry{runtimes: make(map[string]*workspaceRuntime)}
}

type workspaceRuntime struct {
	current          *agentGeneration
	refs             int
	releaseWorkspace func()
}

type agentGeneration struct {
	agent   Agent
	release func()
	refs    int
	retired bool
	closed  bool
}

type preparedAgentReplacement struct {
	mu sync.Mutex

	registry     *RuntimeRegistry
	key          string
	workspace    *workspaceRuntime
	previous     *agentGeneration
	next         *agentGeneration
	newWorkspace bool

	committed bool
	released  bool
	lease     func()
}

func (replacement *preparedAgentReplacement) Agent() Agent {
	if replacement == nil || replacement.next == nil {
		return nil
	}
	return replacement.next.agent
}

func (replacement *preparedAgentReplacement) Commit() error {
	if replacement == nil {
		return fmt.Errorf("agenthost: prepared Workspace generation is required")
	}
	replacement.mu.Lock()
	if replacement.released {
		replacement.mu.Unlock()
		return fmt.Errorf("agenthost: prepared Workspace generation was released")
	}
	if replacement.committed {
		replacement.mu.Unlock()
		return nil
	}

	registry := replacement.registry
	registry.mu.Lock()
	if replacement.newWorkspace {
		if registry.runtimes[replacement.key] != nil {
			registry.mu.Unlock()
			replacement.mu.Unlock()
			replacement.Release()
			return fmt.Errorf("agenthost: Workspace generation changed before replacement commit")
		}
		registry.runtimes[replacement.key] = replacement.workspace
	} else if registry.runtimes[replacement.key] != replacement.workspace ||
		replacement.workspace.current != replacement.previous {
		registry.mu.Unlock()
		replacement.mu.Unlock()
		replacement.Release()
		return fmt.Errorf("agenthost: Workspace generation changed before replacement commit")
	} else {
		replacement.workspace.current = replacement.next
	}

	var releasePrevious func()
	if replacement.previous != nil {
		replacement.previous.retired = true
		if replacement.previous.refs == 0 {
			replacement.previous.closed = true
			releasePrevious = replacement.previous.release
		}
	}
	_, lease, err := registry.attachLocked(replacement.key, replacement.workspace, replacement.next)
	if err != nil {
		if replacement.newWorkspace {
			delete(registry.runtimes, replacement.key)
		} else {
			replacement.workspace.current = replacement.previous
			if replacement.previous != nil {
				replacement.previous.retired = false
				replacement.previous.closed = false
			}
		}
		registry.mu.Unlock()
		replacement.mu.Unlock()
		replacement.Release()
		return err
	}
	replacement.lease = lease
	replacement.committed = true
	registry.mu.Unlock()
	replacement.mu.Unlock()
	if releasePrevious != nil {
		releasePrevious()
	}
	return nil
}

func (replacement *preparedAgentReplacement) Release() {
	if replacement == nil {
		return
	}
	replacement.mu.Lock()
	if replacement.released {
		replacement.mu.Unlock()
		return
	}
	replacement.released = true
	lease := replacement.lease
	var releaseCandidate func()
	var releaseWorkspace func()
	if !replacement.committed {
		if replacement.next != nil {
			releaseCandidate = replacement.next.release
		}
		if replacement.newWorkspace && replacement.workspace != nil {
			releaseWorkspace = replacement.workspace.releaseWorkspace
		}
	}
	replacement.mu.Unlock()
	if lease != nil {
		lease()
	}
	if releaseCandidate != nil {
		releaseCandidate()
	}
	if releaseWorkspace != nil {
		releaseWorkspace()
	}
}

func (r *RuntimeRegistry) Acquire(ctx context.Context, host *Host, workspaceName string, spec Spec) (Agent, func(), error) {
	if r == nil {
		r = NewRuntimeRegistry()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runtimes == nil {
		r.runtimes = make(map[string]*workspaceRuntime)
	}
	key := runtimeKey(workspaceName)
	if current := r.runtimes[key]; current != nil && current.current != nil {
		return r.attachLocked(key, current, current.current)
	}
	current, err := r.constructWorkspaceLocked(ctx, host, workspaceName, spec)
	if err != nil {
		return nil, nil, err
	}
	r.runtimes[key] = current
	return r.attachLocked(key, current, current.current)
}

// PrepareReplacement constructs a complete candidate generation without
// publishing it. Commit atomically changes the registry after the caller has
// prepared the stream and persisted the run activation; Release aborts an
// uncommitted candidate or releases a committed attachment.
func (r *RuntimeRegistry) PrepareReplacement(ctx context.Context, host *Host, workspaceName string, spec Spec) (*preparedAgentReplacement, error) {
	if r == nil {
		r = NewRuntimeRegistry()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runtimes == nil {
		r.runtimes = make(map[string]*workspaceRuntime)
	}
	key := runtimeKey(workspaceName)
	current := r.runtimes[key]
	if current == nil {
		constructed, err := r.constructWorkspaceLocked(ctx, host, workspaceName, spec)
		if err != nil {
			return nil, err
		}
		return &preparedAgentReplacement{
			registry: r, key: key, workspace: constructed,
			next: constructed.current, newWorkspace: true,
		}, nil
	}
	agent, release, err := host.newWorkspaceAgent(ctx, spec)
	if err != nil {
		return nil, err
	}
	next := &agentGeneration{agent: agent, release: release}
	return &preparedAgentReplacement{
		registry: r, key: key, workspace: current,
		previous: current.current, next: next,
	}, nil
}

func (r *RuntimeRegistry) constructWorkspaceLocked(ctx context.Context, host *Host, workspaceName string, spec Spec) (*workspaceRuntime, error) {
	if host == nil {
		return nil, fmt.Errorf("agenthost: host is required")
	}
	coordinator := host.coordinator()
	if coordinator == nil {
		return nil, fmt.Errorf("agenthost: coordinator is required")
	}
	lease, err := coordinator.Acquire(ctx, workspaceName)
	if err != nil {
		return nil, err
	}
	releaseWorkspace := func() { _ = lease.Release(context.Background()) }
	agent, release, err := host.newWorkspaceAgent(ctx, spec)
	if err != nil {
		releaseWorkspace()
		return nil, err
	}
	return &workspaceRuntime{
		current:          &agentGeneration{agent: agent, release: release},
		releaseWorkspace: releaseWorkspace,
	}, nil
}

func (r *RuntimeRegistry) attachLocked(key string, current *workspaceRuntime, generation *agentGeneration) (Agent, func(), error) {
	if current == nil || generation == nil || generation.agent == nil || generation.closed {
		return nil, nil, fmt.Errorf("agenthost: Workspace generation is unavailable")
	}
	current.refs++
	generation.refs++
	return generation.agent, r.releaseFunc(key, current, generation), nil
}

func runtimeKey(workspaceName string) string {
	return workspaceName
}

func (r *RuntimeRegistry) releaseFunc(key string, current *workspaceRuntime, generation *agentGeneration) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			r.release(key, current, generation)
		})
	}
}

func (r *RuntimeRegistry) release(key string, current *workspaceRuntime, generation *agentGeneration) {
	if r == nil || current == nil || generation == nil {
		return
	}
	var releases []func()
	r.mu.Lock()
	if r.runtimes[key] == current {
		if generation.refs > 0 {
			generation.refs--
		}
		if current.refs > 0 {
			current.refs--
		}
		if generation.retired && generation.refs == 0 && !generation.closed {
			generation.closed = true
			releases = append(releases, generation.release)
		}
		if current.refs == 0 {
			delete(r.runtimes, key)
			if current.current != nil && !current.current.closed {
				current.current.closed = true
				releases = append(releases, current.current.release)
			}
			releases = append(releases, current.releaseWorkspace)
		}
	}
	r.mu.Unlock()
	for _, release := range releases {
		if release != nil {
			release()
		}
	}
}
