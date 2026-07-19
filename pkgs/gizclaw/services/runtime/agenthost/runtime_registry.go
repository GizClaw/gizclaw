package agenthost

import (
	"context"
	"sync"
)

// RuntimeRegistry keeps one live agent/lease per workspace and hands out
// reference-counted attachments for per-gear Transform calls.
type RuntimeRegistry struct {
	mu       sync.Mutex
	runtimes map[string]*workspaceRuntime
}

func NewRuntimeRegistry() *RuntimeRegistry {
	return &RuntimeRegistry{runtimes: make(map[string]*workspaceRuntime)}
}

type workspaceRuntime struct {
	agent   Agent
	release func()
	refs    int
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
	key := runtimeKey(workspaceName, spec)
	if current := r.runtimes[key]; current != nil {
		current.refs++
		return current.agent, r.releaseFunc(key, current), nil
	}
	agent, release, err := host.openWorkspaceAgent(ctx, workspaceName, spec)
	if err != nil {
		return nil, nil, err
	}
	current := &workspaceRuntime{agent: agent, release: release, refs: 1}
	r.runtimes[key] = current
	return agent, r.releaseFunc(key, current), nil
}

func runtimeKey(workspaceName string, spec Spec) string {
	if spec.Toolkit == nil {
		return workspaceName
	}
	return workspaceName + "#toolkit-owner=" + spec.Toolkit.BuildRequest.OwnerPublicKey
}

func (r *RuntimeRegistry) releaseFunc(key string, current *workspaceRuntime) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			r.release(key, current)
		})
	}
}

func (r *RuntimeRegistry) release(key string, current *workspaceRuntime) {
	if r == nil || current == nil {
		return
	}
	var release func()
	r.mu.Lock()
	if r.runtimes[key] == current {
		current.refs--
		if current.refs <= 0 {
			delete(r.runtimes, key)
			release = current.release
		}
	}
	r.mu.Unlock()
	if release != nil {
		release()
	}
}
