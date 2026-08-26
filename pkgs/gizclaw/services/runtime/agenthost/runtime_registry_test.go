package agenthost

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestRuntimeRegistryConstructsIndependentWorkspacesConcurrently(t *testing.T) {
	t.Parallel()
	resolver := mutableWorkspaceResolver{idByPattern: map[string]string{
		"first":  "workspace-a",
		"second": "workspace-b",
	}}
	host := New(resolver)
	entered := make(chan string, 2)
	releaseFactories := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseFactories) }) }
	t.Cleanup(release)
	if err := host.Register("shared", agentFactoryFunc(func(ctx context.Context, spec Spec) (Agent, error) {
		entered <- spec.Workspace.Id
		select {
		case <-releaseFactories:
			return &pointerTestAgent{Agent: NewTransformerAgent(passthroughTransformer{})}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})); err != nil {
		t.Fatal(err)
	}

	type openResult struct {
		agent   Agent
		release func()
		err     error
	}
	results := make(chan openResult, 2)
	for _, pattern := range []string{"first", "second"} {
		go func() {
			agent, releaseAgent, err := host.OpenAgent(t.Context(), pattern)
			results <- openResult{agent: agent, release: releaseAgent, err: err}
		}()
	}

	seen := map[string]bool{}
	for range 2 {
		select {
		case workspaceID := <-entered:
			seen[workspaceID] = true
		case <-time.After(time.Second):
			t.Fatal("independent Workspace constructors did not enter concurrently")
		}
	}
	if !seen["workspace-a"] || !seen["workspace-b"] {
		t.Fatalf("entered Workspace constructors = %#v", seen)
	}
	release()
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.agent == nil || result.release == nil {
			t.Fatal("OpenAgent returned an incomplete attachment")
		}
		result.release()
	}
}

func TestRuntimeRegistryCanceledWaiterDoesNotCancelWorkspaceConstruction(t *testing.T) {
	t.Parallel()
	spec := Spec{Workspace: apitypes.Workspace{Id: "workspace-a", Name: "demo"}, AgentType: "shared"}
	host := New(fakeResolver{spec: spec})
	entered := make(chan struct{})
	releaseFactory := make(chan struct{})
	var calls atomic.Int32
	if err := host.Register("shared", agentFactoryFunc(func(ctx context.Context, _ Spec) (Agent, error) {
		calls.Add(1)
		close(entered)
		select {
		case <-releaseFactory:
			return &pointerTestAgent{Agent: NewTransformerAgent(passthroughTransformer{})}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})); err != nil {
		t.Fatal(err)
	}

	type openResult struct {
		release func()
		err     error
	}
	ownerResult := make(chan openResult, 1)
	go func() {
		_, release, err := host.OpenAgent(t.Context(), "demo")
		ownerResult <- openResult{release: release, err: err}
	}()
	<-entered

	waiterCtx, cancelWaiter := context.WithCancel(t.Context())
	cancelWaiter()
	if _, _, err := host.OpenAgent(waiterCtx, "demo"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter error = %v, want %v", err, context.Canceled)
	}
	close(releaseFactory)
	owner := <-ownerResult
	if owner.err != nil {
		t.Fatal(owner.err)
	}
	owner.release()
	if got := calls.Load(); got != 1 {
		t.Fatalf("factory call count = %d, want 1", got)
	}
}

func TestRuntimeRegistryPropagatesQuiesceToConstructionWaiters(t *testing.T) {
	t.Parallel()
	spec := Spec{Workspace: apitypes.Workspace{Id: "workspace-a", Name: "demo"}, AgentType: "shared"}
	host := New(fakeResolver{spec: spec})
	entered := make(chan struct{})
	releaseFactory := make(chan struct{})
	var calls atomic.Int32
	if err := host.Register("shared", agentFactoryFunc(func(ctx context.Context, _ Spec) (Agent, error) {
		calls.Add(1)
		close(entered)
		select {
		case <-releaseFactory:
			return &pointerTestAgent{Agent: NewTransformerAgent(passthroughTransformer{})}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})); err != nil {
		t.Fatal(err)
	}

	ownerResult := make(chan error, 1)
	go func() {
		_, _, err := host.OpenAgent(t.Context(), "demo")
		ownerResult <- err
	}()
	<-entered
	registry := host.runtimeRegistry()
	registry.mu.Lock()
	construction := registry.constructions["workspace-a"]
	registry.mu.Unlock()
	if construction == nil {
		t.Fatal("workspace construction was not registered")
	}
	waiterResult := make(chan error, 1)
	go func() {
		waiterResult <- registry.waitForWorkspaceConstruction(t.Context(), construction)
	}()
	if err := host.QuiesceWorkspace(t.Context(), "workspace-a"); err != nil {
		t.Fatal(err)
	}
	close(releaseFactory)
	if err := <-ownerResult; !errors.Is(err, errWorkspaceQuiesced) {
		t.Fatalf("OpenAgent() error = %v, want %v", err, errWorkspaceQuiesced)
	}
	if err := <-waiterResult; !errors.Is(err, errWorkspaceQuiesced) {
		t.Fatalf("waitForWorkspaceConstruction() error = %v, want %v", err, errWorkspaceQuiesced)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("factory call count = %d, want 1", got)
	}
}

func TestRuntimeRegistrySerializesPreparedReplacementsUntilCommit(t *testing.T) {
	t.Parallel()
	spec := Spec{Workspace: apitypes.Workspace{Id: "workspace-a", Name: "demo"}, AgentType: "shared"}
	host := New(fakeResolver{spec: spec})
	entered := make(chan int, 2)
	allowSecond := make(chan struct{})
	allowThird := make(chan struct{})
	var calls atomic.Int32
	if err := host.Register("shared", agentFactoryFunc(func(ctx context.Context, _ Spec) (Agent, error) {
		call := int(calls.Add(1))
		switch call {
		case 2:
			entered <- call
			select {
			case <-allowSecond:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		case 3:
			entered <- call
			select {
			case <-allowThird:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return &pointerTestAgent{Agent: NewTransformerAgent(passthroughTransformer{})}, nil
	})); err != nil {
		t.Fatal(err)
	}

	_, releaseInitial, err := host.OpenAgent(t.Context(), "demo")
	if err != nil {
		t.Fatal(err)
	}

	firstResult := make(chan *preparedAgentReplacement, 1)
	firstErr := make(chan error, 1)
	go func() {
		replacement, prepareErr := host.PrepareReloadAgent(t.Context(), "demo")
		firstResult <- replacement
		firstErr <- prepareErr
	}()
	if call := <-entered; call != 2 {
		t.Fatalf("first replacement factory call = %d, want 2", call)
	}
	close(allowSecond)
	first := <-firstResult
	if err := <-firstErr; err != nil {
		t.Fatal(err)
	}

	secondStarted := make(chan struct{})
	secondResult := make(chan *preparedAgentReplacement, 1)
	secondErr := make(chan error, 1)
	go func() {
		close(secondStarted)
		replacement, prepareErr := host.PrepareReloadAgent(t.Context(), "demo")
		secondResult <- replacement
		secondErr <- prepareErr
	}()
	<-secondStarted
	enteredBeforeCommit := false
	select {
	case call := <-entered:
		if call != 3 {
			t.Fatalf("second replacement factory call = %d, want 3", call)
		}
		enteredBeforeCommit = true
		close(allowThird)
	case <-time.After(100 * time.Millisecond):
	}

	if err := first.Commit(); err != nil {
		t.Fatal(err)
	}
	if !enteredBeforeCommit {
		select {
		case call := <-entered:
			if call != 3 {
				t.Fatalf("second replacement factory call = %d, want 3", call)
			}
		case <-time.After(time.Second):
			t.Fatal("second replacement factory did not enter after commit")
		}
		close(allowThird)
	}
	second := <-secondResult
	if err := <-secondErr; err != nil {
		t.Fatal(err)
	}
	second.Release()
	first.Release()
	releaseInitial()
	if enteredBeforeCommit {
		t.Fatal("replacement factory entered before the prior candidate committed")
	}
}

func TestRuntimeRegistryQuiesceRejectsUncommittedWorkspace(t *testing.T) {
	t.Parallel()
	spec := Spec{Workspace: apitypes.Workspace{Id: "workspace-a", Name: "demo"}, AgentType: "shared"}
	host := New(fakeResolver{spec: spec})
	var closed atomic.Int32
	if err := host.Register("shared", agentFactoryFunc(func(context.Context, Spec) (Agent, error) {
		return &closeTrackingAgent{
			Agent: NewTransformerAgent(passthroughTransformer{}),
			close: func() {
				closed.Add(1)
			},
		}, nil
	})); err != nil {
		t.Fatal(err)
	}

	replacement, err := host.PrepareReloadAgent(t.Context(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := host.QuiesceWorkspace(t.Context(), "workspace-a"); err != nil {
		t.Fatal(err)
	}
	if err := replacement.Commit(); !errors.Is(err, errWorkspaceQuiesced) {
		t.Fatalf("Commit() error = %v, want %v", err, errWorkspaceQuiesced)
	}
	if got := closed.Load(); got != 1 {
		t.Fatalf("candidate close count = %d, want 1", got)
	}
	lease, err := host.coordinator().Acquire(t.Context(), "workspace-a")
	if err != nil {
		t.Fatalf("coordinator lease remained after rejected candidate: %v", err)
	}
	if err := lease.Release(t.Context()); err != nil {
		t.Fatal(err)
	}
}
