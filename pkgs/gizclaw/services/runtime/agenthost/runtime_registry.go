package agenthost

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
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
	key := runtimeKey(ctx, workspaceName, spec)
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

func runtimeKey(ctx context.Context, workspaceName string, spec Spec) string {
	key := workspaceName
	// Owned workspaces deliberately share their owner's runtime across callers.
	// Admin-created workspaces without an owner depend on the caller's
	// RuntimeProfile snapshot and must not reuse another caller's model/voice
	// bindings.
	workspaceOwner := ""
	if spec.Workspace.OwnerPublicKey != nil {
		workspaceOwner = strings.TrimSpace(*spec.Workspace.OwnerPublicKey)
	}
	systemWorkspace := spec.Workspace.System != nil && *spec.Workspace.System
	if workspaceOwner == "" || systemWorkspace {
		fingerprint := spec.runtimeAccessFingerprint
		if fingerprint == "" {
			fingerprint = resourceAccessFingerprint(ctx)
		}
		key += "#profile=" + fingerprint
	}
	if spec.Toolkit == nil {
		return key
	}
	return key + "#toolkit-caller=" + spec.Toolkit.BuildRequest.CallerPublicKey
}

func resourceAccessFingerprint(ctx context.Context) string {
	access, ok := resourceAccessFromContext(ctx)
	if !ok {
		return "none"
	}
	values := []string{"owner=" + access.ownerPublicKey, "profile=" + access.profileFingerprint}
	for alias, resourceID := range access.profileToolBindings {
		values = append(values, "tool="+alias+"="+resourceID)
	}
	for alias, resourceID := range access.profileWorkflowBindings {
		values = append(values, "workflow="+alias+"="+resourceID)
	}
	sort.Strings(values)
	digest := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return fmt.Sprintf("%x", digest[:16])
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
