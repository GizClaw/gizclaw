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

	"github.com/GizClaw/flowcraft/sdk/embedding"
	"github.com/GizClaw/flowcraft/sdk/llm"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func BenchmarkRegistryResolveSameBindingContention(b *testing.B) {
	request := managedTestRequest(b)
	physical, err := openSharedBackend(b.Context(), request)
	if err != nil {
		b.Fatal(err)
	}
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
	request := managedTestRequest(t)
	physical, err := openSharedBackend(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
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
	request := managedTestRequest(t)
	physical, err := openSharedBackend(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
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
	request := managedTestRequest(t)
	registry := NewRegistry()
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
	request := managedTestRequest(t)
	registry := NewRegistry()
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
	request := managedTestRequest(t)
	firstKey, err := registryKey(request)
	if err != nil {
		t.Fatal(err)
	}
	connection := apitypes.RuntimeProfileMemoryConnection{}
	if err := connection.FromRuntimeProfileFlowcraftObjectStoreConnection(
		apitypes.RuntimeProfileFlowcraftObjectStoreConnection{
			Type:      apitypes.RuntimeProfileFlowcraftObjectStoreConnectionTypeFlowcraftObjectStore,
			Directory: t.TempDir(),
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
	t.Cleanup(func() { _ = registry.Close() })
	request := managedTestRequest(t)

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

func TestRegistrySharesPhysicalBackendAcrossWorkspaceScopedStores(t *testing.T) {
	t.Parallel()
	registry := NewRegistry()
	t.Cleanup(func() { _ = registry.Close() })

	firstRequest := managedTestRequest(t)
	firstRequest.WorkspaceID = "workspace-a"
	first, err := registry.Resolve(t.Context(), firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Closer.Close()
	if _, err := first.Store.Observe(t.Context(), memory.Observation{
		Text: "Mochi likes salmon.",
	}); err != nil {
		t.Fatal(err)
	}

	secondRequest := firstRequest
	secondRequest.WorkspaceID = "workspace-b"
	second, err := registry.Resolve(t.Context(), secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Closer.Close()
	if len(registry.entries) != 1 {
		t.Fatalf("physical backends = %d, want 1", len(registry.entries))
	}
	key, err := registryKey(firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	shared := registry.entries[key].backend.(*sharedFlowcraftBackend)
	if shared.temporal == nil || shared.index == nil {
		t.Fatal("registry entry does not share the canonical backend and BBH index")
	}
	recalled, err := second.Store.Recall(t.Context(), memory.Query{Text: "salmon", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(recalled.Matches) != 0 {
		t.Fatalf("workspace-b recalled %d workspace-a facts", len(recalled.Matches))
	}
	if _, err := second.Store.Recall(t.Context(), memory.Query{
		Scope: memory.Scope{AppID: "workspace-a"}, Text: "salmon", Limit: 5,
	}); err == nil {
		t.Fatal("Workspace-bound Store accepted a conflicting AppID")
	}
}

func TestRegistryPersistsDirectFactsWithExtractionConfigured(t *testing.T) {
	t.Parallel()
	registry := NewRegistry()
	t.Cleanup(func() { _ = registry.Close() })
	request := managedTestRequest(t)
	request.Layout.Spec.Flowcraft.Extraction.Model = "extract"
	request.ModelLoader = registryTestModelLoader{}

	result, err := registry.Resolve(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Closer.Close()
	const text = "The workspace code is GIZCLAWMEMORY123."
	if _, err := result.Store.Observe(t.Context(), memory.Observation{
		Facts: []memory.FactCandidate{{Text: text}},
	}); err != nil {
		t.Fatal(err)
	}
	recalled, err := result.Store.Recall(t.Context(), memory.Query{
		Text:  "GIZCLAWMEMORY123",
		Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recalled.Matches) != 1 || recalled.Matches[0].Fact.Text != text {
		t.Fatalf("Recall() matches = %#v, want direct fact %q", recalled.Matches, text)
	}
}

func TestRegistryRebuildsDerivedIndexWhileOldLogicalStoreIsLive(t *testing.T) {
	t.Parallel()
	registry := NewRegistry()
	t.Cleanup(func() { _ = registry.Close() })
	request := managedTestRequest(t)

	first, err := registry.Resolve(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Closer.Close()
	if _, err := first.Store.Observe(t.Context(), memory.Observation{
		Text: "Mochi likes salmon.",
	}); err != nil {
		t.Fatal(err)
	}

	request.Layout.Spec.Flowcraft.Bbh.SearchOverfetch = new(9)
	reloaded, err := registry.Resolve(t.Context(), request)
	if err != nil {
		t.Fatalf("Resolve(policy reload) error = %v", err)
	}
	defer reloaded.Closer.Close()
	recalled, err := reloaded.Store.Recall(t.Context(), memory.Query{Text: "salmon", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(recalled.Matches) == 0 {
		t.Fatal("policy reload lost canonical Workspace facts")
	}
	recalled, err = first.Store.Recall(t.Context(), memory.Query{Text: "salmon", Limit: 5})
	if err != nil {
		t.Fatalf("old logical Store after index swap: %v", err)
	}
	if len(recalled.Matches) == 0 {
		t.Fatal("old logical Store did not adopt the atomically rebuilt index")
	}
}

type registryTestModelLoader struct{}

func (registryTestModelLoader) LoadLLM(context.Context, string) (llm.LLM, error) {
	return registryTestLLM{}, nil
}

func (registryTestModelLoader) LoadEmbedder(context.Context, string) (embedding.Embedder, error) {
	return nil, errors.New("unexpected embedder load")
}

type registryTestLLM struct{}

func (registryTestLLM) Generate(context.Context, []llm.Message, ...llm.GenerateOption) (llm.Message, llm.TokenUsage, error) {
	return llm.NewTextMessage(llm.RoleAssistant, `{"facts":[]}`), llm.TokenUsage{}, nil
}

func (registryTestLLM) GenerateStream(context.Context, []llm.Message, ...llm.GenerateOption) (llm.StreamMessage, error) {
	return nil, errors.New("unexpected streaming extraction")
}

func managedTestRequest(t testing.TB) Request {
	t.Helper()
	connection := apitypes.RuntimeProfileMemoryConnection{}
	if err := connection.FromRuntimeProfileFlowcraftBBHConnection(
		apitypes.RuntimeProfileFlowcraftBBHConnection{
			Type: apitypes.RuntimeProfileFlowcraftBBHConnectionTypeFlowcraftBbh,
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
