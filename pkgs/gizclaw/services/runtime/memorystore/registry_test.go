package memorystore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func BenchmarkRegistryResolveSameBindingContention(b *testing.B) {
	request := supportedFlowcraftTestRequest(b)
	physical := &testSharedBackend{}
	const constructorDelay = time.Millisecond
	backend := &delayedRegistryBackend{sharedBackend: physical, delay: constructorDelay}
	registry := NewRegistry()
	registry.open = func(context.Context, Request) (sharedBackend, error) {
		return backend, nil
	}
	anchor, err := registry.Resolve(b.Context(), request)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		_ = anchor.Closer.Close()
		_ = registry.Close()
	})

	for _, benchmark := range []struct {
		name     string
		parallel bool
	}{
		{name: "serial-8"},
		{name: "parallel-8", parallel: true},
	} {
		b.Run(benchmark.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ReportMetric(float64(constructorDelay), "constructor-delay-ns")
			b.ResetTimer()
			for b.Loop() {
				if err := resolveStoreBatch(b.Context(), registry, request, 8, benchmark.parallel); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*8), "ns/store")
		})
	}
}

type delayedRegistryBackend struct {
	sharedBackend
	delay time.Duration
}

func (backend *delayedRegistryBackend) NewStore(
	ctx context.Context,
	request Request,
) (Result, io.Closer, error) {
	timer := time.NewTimer(backend.delay)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		return Result{}, nil, ctx.Err()
	}
	return backend.sharedBackend.NewStore(ctx, request)
}

func resolveStoreBatch(
	ctx context.Context,
	registry *Registry,
	request Request,
	count int,
	parallel bool,
) error {
	resolve := func(index int) error {
		workspaceRequest := request
		workspaceRequest.WorkspaceID = fmt.Sprintf("benchmark-workspace-%d", index)
		result, err := registry.Resolve(ctx, workspaceRequest)
		if err != nil {
			return err
		}
		return result.Closer.Close()
	}
	if !parallel {
		for index := range count {
			if err := resolve(index); err != nil {
				return err
			}
		}
		return nil
	}

	errs := make([]error, count)
	var wait sync.WaitGroup
	for index := range count {
		wait.Go(func() { errs[index] = resolve(index) })
	}
	wait.Wait()
	return errors.Join(errs...)
}

func TestRegistrySameBindingLogicalStoresConstructConcurrently(t *testing.T) {
	request := supportedFlowcraftTestRequest(t)
	physical := &testSharedBackend{}
	entered := make(chan string, 2)
	release := make(chan struct{})
	backend := &blockingRegistryBackend{
		sharedBackend: physical,
		entered:       entered,
		release:       release,
	}
	registry := NewRegistry()
	var opens atomic.Int32
	registry.open = func(context.Context, Request) (sharedBackend, error) {
		opens.Add(1)
		return backend, nil
	}
	t.Cleanup(func() { _ = registry.Close() })

	type resolved struct {
		result Result
		err    error
	}
	results := make(chan resolved, 2)
	resolve := func(workspace string) {
		workspaceRequest := request
		workspaceRequest.WorkspaceID = workspace
		result, err := registry.Resolve(t.Context(), workspaceRequest)
		results <- resolved{result: result, err: err}
	}
	go resolve("workspace-a")
	wantEntered(t, entered, "workspace-a")
	go resolve("workspace-b")
	wantEntered(t, entered, "workspace-b")
	close(release)

	for range 2 {
		resolved := <-results
		if resolved.err != nil {
			t.Fatal(resolved.err)
		}
		if err := resolved.result.Closer.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if got := opens.Load(); got != 1 {
		t.Fatalf("physical backend opens = %d, want 1", got)
	}
	if got := backend.closes.Load(); got != 1 {
		t.Fatalf("physical backend closes = %d, want 1", got)
	}
}

func TestRegistryCloseWaitsForInFlightLogicalStore(t *testing.T) {
	request := supportedFlowcraftTestRequest(t)
	physical := &testSharedBackend{}
	entered := make(chan string, 1)
	release := make(chan struct{})
	backend := &blockingRegistryBackend{
		sharedBackend: physical,
		entered:       entered,
		release:       release,
		logicalClosed: make(chan struct{}),
	}
	registry := NewRegistry()
	registry.open = func(context.Context, Request) (sharedBackend, error) {
		return backend, nil
	}
	resolved := make(chan error, 1)
	go func() {
		_, err := registry.Resolve(t.Context(), request)
		resolved <- err
	}()
	wantEntered(t, entered, request.WorkspaceID)

	closed := make(chan error, 1)
	go func() { closed <- registry.Close() }()
	waitForRegistryClosing(t, registry)
	select {
	case err := <-closed:
		t.Fatalf("Registry.Close() returned before the in-flight constructor left: %v", err)
	default:
	}
	close(release)
	if err := <-resolved; !errors.Is(err, errRegistryEntryClosed) {
		t.Fatalf("Resolve() error = %v, want %v", err, errRegistryEntryClosed)
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if got := backend.closes.Load(); got != 1 {
		t.Fatalf("physical backend closes = %d, want 1", got)
	}
	if backend.physicalClosedFirst.Load() {
		t.Fatal("physical backend closed before the rejected logical Store")
	}
}

func waitForRegistryClosing(t *testing.T, registry *Registry) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		registry.mu.Lock()
		closing := len(registry.entries) == 0
		registry.mu.Unlock()
		if closing {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("Registry.Close() did not detach the active entry")
}

func wantEntered(t *testing.T, entered <-chan string, want string) {
	t.Helper()
	select {
	case got := <-entered:
		if got != want {
			t.Fatalf("constructor entered for %q, want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("constructor for %q did not enter while the other Workspace remained blocked", want)
	}
}

type blockingRegistryBackend struct {
	sharedBackend
	entered             chan<- string
	release             <-chan struct{}
	logicalClosed       chan struct{}
	closes              atomic.Int32
	physicalClosedFirst atomic.Bool
}

func (backend *blockingRegistryBackend) NewStore(
	ctx context.Context,
	request Request,
) (Result, io.Closer, error) {
	select {
	case backend.entered <- request.WorkspaceID:
	case <-ctx.Done():
		return Result{}, nil, ctx.Err()
	}
	select {
	case <-backend.release:
	case <-ctx.Done():
		return Result{}, nil, ctx.Err()
	}
	result, closer, err := backend.sharedBackend.NewStore(ctx, request)
	if err == nil && backend.logicalClosed != nil {
		closer = &signalingCloser{Closer: closer, closed: backend.logicalClosed}
	}
	return result, closer, err
}

func (backend *blockingRegistryBackend) Close() error {
	if backend.logicalClosed != nil {
		select {
		case <-backend.logicalClosed:
		default:
			backend.physicalClosedFirst.Store(true)
		}
	}
	backend.closes.Add(1)
	return backend.sharedBackend.Close()
}

type signalingCloser struct {
	io.Closer
	closed chan struct{}
	once   sync.Once
}

func (closer *signalingCloser) Close() error {
	err := closer.Closer.Close()
	closer.once.Do(func() { close(closer.closed) })
	return err
}

func TestRegistrySharesBindingUntilFinalRelease(t *testing.T) {
	t.Parallel()
	request := supportedFlowcraftTestRequest(t)
	registry := NewRegistry()
	registry.open = func(context.Context, Request) (sharedBackend, error) {
		return &testSharedBackend{}, nil
	}
	t.Cleanup(func() { _ = registry.Close() })

	first, err := registry.Resolve(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := registry.Resolve(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Store == second.Store {
		t.Fatal("identical binding reused one logical Store instead of creating Workspace views")
	}
	key, err := registryKey(request)
	if err != nil {
		t.Fatal(err)
	}
	shared := registry.entries[key].backend
	if err := first.Closer.Close(); err != nil {
		t.Fatal(err)
	}
	third, err := registry.Resolve(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if registry.entries[key].backend != shared {
		t.Fatal("physical backend closed before the final lease")
	}
	if err := second.Closer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := third.Closer.Close(); err != nil {
		t.Fatal(err)
	}
	if len(registry.entries) != 0 {
		t.Fatalf("registry entries after final release = %d, want 0", len(registry.entries))
	}

	reopened, err := registry.Resolve(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if registry.entries[key].backend == shared {
		t.Fatal("physical backend was not reconstructed after the final release")
	}
	if err := reopened.Closer.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryConcurrentResolveConstructsOneStore(t *testing.T) {
	t.Parallel()
	request := supportedFlowcraftTestRequest(t)
	registry := NewRegistry()
	registry.open = func(context.Context, Request) (sharedBackend, error) {
		return &testSharedBackend{}, nil
	}
	t.Cleanup(func() { _ = registry.Close() })

	const count = 24
	results := make([]Result, count)
	errs := make([]error, count)
	var wait sync.WaitGroup
	for index := range count {
		wait.Go(func() {
			results[index], errs[index] = registry.Resolve(t.Context(), request)
		})
	}
	wait.Wait()
	for index, err := range errs {
		if err != nil {
			t.Fatalf("Resolve(%d) error = %v", index, err)
		}
		if index > 0 && results[index].Store == results[0].Store {
			t.Fatalf("Resolve(%d) reused one logical Store", index)
		}
	}
	if len(registry.entries) != 1 {
		t.Fatalf("concurrent Resolve constructed %d physical backends, want 1", len(registry.entries))
	}
	for _, result := range results {
		if err := result.Closer.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRegistryIdentityIncludesPhysicalConnection(t *testing.T) {
	t.Parallel()
	request := supportedFlowcraftTestRequest(t)
	firstKey, err := registryKey(request)
	if err != nil {
		t.Fatal(err)
	}
	connection := apitypes.RuntimeProfileMemoryConnection{}
	if err := connection.FromRuntimeProfileFlowcraftRedis8Connection(
		apitypes.RuntimeProfileFlowcraftRedis8Connection{
			Type: apitypes.RuntimeProfileFlowcraftRedis8ConnectionTypeFlowcraftRedis8,
			Url:  "redis://other.example:6379/0",
		},
	); err != nil {
		t.Fatal(err)
	}
	request.Binding.Connection = connection
	secondKey, err := registryKey(request)
	if err != nil {
		t.Fatal(err)
	}
	if firstKey == secondKey {
		t.Fatal("physical connection did not change registry identity")
	}
}

func TestRegistryValidatesLayoutIDForEveryResolve(t *testing.T) {
	t.Parallel()
	registry := NewRegistry()
	registry.open = func(context.Context, Request) (sharedBackend, error) {
		return &testSharedBackend{}, nil
	}
	t.Cleanup(func() { _ = registry.Close() })
	request := supportedFlowcraftTestRequest(t)

	first, err := registry.Resolve(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Closer.Close() })

	request.Layout.Id = "different-layout-id"
	if _, err := registry.Resolve(t.Context(), request); err == nil ||
		!strings.Contains(err.Error(), `layout id "different-layout-id" does not match binding layout_id "layout-id"`) {
		t.Fatalf("Resolve() error = %v", err)
	}
}

func supportedFlowcraftTestRequest(t testing.TB) Request {
	t.Helper()
	connection := apitypes.RuntimeProfileMemoryConnection{}
	if err := connection.FromRuntimeProfileFlowcraftRedis8Connection(
		apitypes.RuntimeProfileFlowcraftRedis8Connection{
			Type: apitypes.RuntimeProfileFlowcraftRedis8ConnectionTypeFlowcraftRedis8,
			Url:  "redis://redis.example:6379/0",
		},
	); err != nil {
		t.Fatal(err)
	}
	return Request{
		WorkspaceID:     "workspace",
		ProfileID:       "profile-id",
		ProfileRevision: "revision",
		BindingName:     "pet-memory",
		ServerRoot:      t.TempDir(),
		Layout: apitypes.MemoryLayout{
			Id:   "layout-id",
			Spec: apitypes.MemoryLayoutSpec{Flowcraft: testFlowcraftPolicy()},
		},
		Binding: apitypes.RuntimeProfileMemoryBinding{
			LayoutId:   "layout-id",
			Driver:     apitypes.RuntimeProfileMemoryDriverFlowcraft,
			Connection: connection,
		},
	}
}

type testSharedBackend struct{ marker byte }

func (*testSharedBackend) NewStore(context.Context, Request) (Result, io.Closer, error) {
	return Result{
		Store:  &testMemoryStore{},
		Driver: string(apitypes.RuntimeProfileMemoryDriverFlowcraft),
	}, testLogicalCloser{}, nil
}

func (*testSharedBackend) Close() error { return nil }

type testLogicalCloser struct{}

func (testLogicalCloser) Close() error { return nil }

type testMemoryStore struct{}

func (*testMemoryStore) Observe(context.Context, memory.Observation) (memory.ObserveResult, error) {
	return memory.ObserveResult{}, nil
}

func (*testMemoryStore) Recall(context.Context, memory.Query) (memory.RecallResult, error) {
	return memory.RecallResult{}, nil
}

func (*testMemoryStore) Update(_ context.Context, request memory.UpdateRequest) (memory.Fact, error) {
	return memory.Fact{ID: request.ID}, nil
}

func (*testMemoryStore) Delete(context.Context, memory.DeleteRequest) error { return nil }
