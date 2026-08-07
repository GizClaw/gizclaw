package agenthost

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

var errWorkspaceQuiesced = errors.New("agenthost: Workspace runtime quiesced")

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
	generations      map[*agentGeneration]struct{}
	refs             int
	releaseWorkspace func()
}

type agentGeneration struct {
	agent      Agent
	release    func()
	refs       int
	retired    bool
	closed     bool
	registry   *RuntimeRegistry
	transforms map[*runtimeTransform]struct{}
}

type runtimeTransform struct {
	generation *agentGeneration
	cancel     context.CancelCauseFunc
	once       sync.Once
	outputMu   sync.Mutex
	output     genx.Stream
	stopped    bool
}

type quiescingAgent struct {
	Agent
	generation *agentGeneration
}

type quiescingStream struct {
	genx.Stream
	transform *runtimeTransform
}

func (a *quiescingAgent) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	if a == nil || a.generation == nil || a.generation.registry == nil {
		return nil, errWorkspaceQuiesced
	}
	runCtx, cancel := context.WithCancelCause(ctx)
	registry := a.generation.registry
	registry.mu.Lock()
	if a.generation.closed {
		registry.mu.Unlock()
		cancel(errWorkspaceQuiesced)
		return nil, errWorkspaceQuiesced
	}
	transform := &runtimeTransform{generation: a.generation, cancel: cancel}
	a.generation.transforms[transform] = struct{}{}
	registry.mu.Unlock()
	go func() {
		<-runCtx.Done()
		transform.finish(context.Cause(runCtx))
	}()
	output, err := a.Agent.Transform(runCtx, input)
	if err != nil {
		transform.finish(err)
		return nil, err
	}
	if output == nil {
		transform.finish(nil)
		return nil, nil
	}
	if transform.setOutput(output) {
		_ = output.CloseWithError(errWorkspaceQuiesced)
		return nil, errWorkspaceQuiesced
	}
	return &quiescingStream{Stream: output, transform: transform}, nil
}

func (s *quiescingStream) Next() (*genx.MessageChunk, error) {
	chunk, err := s.Stream.Next()
	if err != nil {
		s.transform.finish(err)
	}
	return chunk, err
}

func (s *quiescingStream) Close() error {
	err := s.Stream.Close()
	s.transform.finish(err)
	return err
}

func (s *quiescingStream) CloseWithError(err error) error {
	closeErr := s.Stream.CloseWithError(err)
	s.transform.finish(err)
	return closeErr
}

func (t *runtimeTransform) finish(cause error) {
	if t == nil {
		return
	}
	t.once.Do(func() {
		if t.cancel != nil {
			t.cancel(cause)
		}
		if t.generation == nil || t.generation.registry == nil {
			return
		}
		registry := t.generation.registry
		registry.mu.Lock()
		delete(t.generation.transforms, t)
		registry.mu.Unlock()
	})
}

func (t *runtimeTransform) setOutput(output genx.Stream) bool {
	t.outputMu.Lock()
	defer t.outputMu.Unlock()
	t.output = output
	return t.stopped
}

func (t *runtimeTransform) stop(cause error) {
	if t == nil {
		return
	}
	t.outputMu.Lock()
	t.stopped = true
	output := t.output
	t.outputMu.Unlock()
	t.finish(cause)
	if output != nil {
		_ = output.CloseWithError(cause)
	}
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
		replacement.workspace.generations[replacement.next] = struct{}{}
	}

	var releasePrevious func()
	if replacement.previous != nil {
		replacement.previous.retired = true
		if replacement.previous.refs == 0 {
			replacement.previous.closed = true
			delete(replacement.workspace.generations, replacement.previous)
			releasePrevious = replacement.previous.release
		}
	}
	_, lease, err := registry.attachLocked(replacement.key, replacement.workspace, replacement.next)
	if err != nil {
		if replacement.newWorkspace {
			delete(registry.runtimes, replacement.key)
		} else {
			replacement.workspace.current = replacement.previous
			delete(replacement.workspace.generations, replacement.next)
			if replacement.previous != nil {
				replacement.previous.retired = false
				replacement.previous.closed = false
				replacement.workspace.generations[replacement.previous] = struct{}{}
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
	next := r.newGeneration(agent, release)
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
	generation := r.newGeneration(agent, release)
	return &workspaceRuntime{
		current:          generation,
		generations:      map[*agentGeneration]struct{}{generation: {}},
		releaseWorkspace: releaseWorkspace,
	}, nil
}

func (r *RuntimeRegistry) newGeneration(agent Agent, release func()) *agentGeneration {
	generation := &agentGeneration{
		release: release, registry: r, transforms: make(map[*runtimeTransform]struct{}),
	}
	generation.agent = &quiescingAgent{Agent: agent, generation: generation}
	return generation
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
			delete(current.generations, generation)
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

// Quiesce removes the published generation for one Workspace and closes its
// agent and coordinator lease. Future acquisition must pass through the
// product resolver again, where the durable deletion marker is enforced.
func (r *RuntimeRegistry) Quiesce(workspaceID string) {
	if r == nil {
		return
	}
	var releases []func()
	var transforms []*runtimeTransform
	r.mu.Lock()
	current := r.runtimes[runtimeKey(workspaceID)]
	if current != nil {
		delete(r.runtimes, runtimeKey(workspaceID))
		for generation := range current.generations {
			if generation.closed {
				continue
			}
			generation.closed = true
			generation.retired = true
			for transform := range generation.transforms {
				transforms = append(transforms, transform)
			}
			releases = append(releases, generation.release)
		}
		clear(current.generations)
		releases = append(releases, current.releaseWorkspace)
	}
	r.mu.Unlock()
	for _, transform := range transforms {
		transform.stop(errWorkspaceQuiesced)
	}
	for _, release := range releases {
		if release != nil {
			release()
		}
	}
}
