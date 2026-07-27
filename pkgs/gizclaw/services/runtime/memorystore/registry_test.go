package memorystore

import (
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

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

func TestRegistrySharesPhysicalBackendAcrossWorkspaceScopedStores(t *testing.T) {
	t.Parallel()
	registry := NewRegistry()
	t.Cleanup(func() { _ = registry.Close() })

	firstRequest := managedTestRequest(t)
	firstRequest.WorkspaceName = "workspace-a"
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
	secondRequest.WorkspaceName = "workspace-b"
	second, err := registry.Resolve(t.Context(), secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Closer.Close()
	if len(registry.entries) != 1 {
		t.Fatalf("physical backends = %d, want 1", len(registry.entries))
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

func managedTestRequest(t *testing.T) Request {
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
		WorkspaceName:   "workspace",
		ProfileName:     "default",
		ProfileRevision: "revision",
		BindingName:     "pet-memory",
		ServerRoot:      t.TempDir(),
		Layout: apitypes.MemoryLayout{
			Name: "pet-memory",
			Spec: apitypes.MemoryLayoutSpec{Flowcraft: testFlowcraftPolicy()},
		},
		Binding: apitypes.RuntimeProfileMemoryBinding{
			LayoutId:   "pet-memory",
			Driver:     apitypes.RuntimeProfileMemoryDriverFlowcraft,
			Connection: connection,
		},
	}
}
