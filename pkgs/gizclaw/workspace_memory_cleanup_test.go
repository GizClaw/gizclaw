package gizclaw

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/memorystore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

type retainedMemoryResolverFunc func(context.Context, string) (agenthost.Spec, error)

func (f retainedMemoryResolverFunc) ResolveRetainedMemoryByID(ctx context.Context, id string) (agenthost.Spec, error) {
	return f(ctx, id)
}

func objectStoreMemorySpec(t *testing.T) agenthost.Spec {
	t.Helper()
	connection := apitypes.RuntimeProfileMemoryConnection{}
	if err := connection.FromRuntimeProfileFlowcraftObjectStoreConnection(apitypes.RuntimeProfileFlowcraftObjectStoreConnection{
		Type:      apitypes.RuntimeProfileFlowcraftObjectStoreConnectionTypeFlowcraftObjectStore,
		Directory: t.TempDir(),
	}); err != nil {
		t.Fatal(err)
	}
	return agenthost.Spec{
		MemoryName: "pet-memory", MemoryProfileID: "profile", MemoryProfileRevision: "revision",
		MemoryLayout: &apitypes.MemoryLayout{Id: "layout", Spec: apitypes.MemoryLayoutSpec{
			Flowcraft: apitypes.FlowcraftMemoryLayoutPolicy{Write: apitypes.FlowcraftMemoryWritePolicy{
				Mode: apitypes.FlowcraftMemoryWritePolicyModeSync, Tier: apitypes.FlowcraftMemoryWritePolicyTierGeneral,
			}},
		}},
		MemoryBinding: &apitypes.RuntimeProfileMemoryBinding{
			LayoutId: "layout", Driver: apitypes.RuntimeProfileMemoryDriverFlowcraft, Connection: connection,
		},
	}
}

func TestWorkspaceMemoryCleanupPurgesCurrentBinding(t *testing.T) {
	t.Parallel()
	spec := objectStoreMemorySpec(t)
	stores := memorystore.NewRegistry()
	t.Cleanup(func() { _ = stores.Close() })
	cleanup := workspaceMemoryCleanup{
		Resolver: retainedMemoryResolverFunc(func(context.Context, string) (agenthost.Spec, error) { return spec, nil }),
		Stores:   stores, ServerRoot: t.TempDir(),
	}
	for _, workspaceID := range []string{"workspace-a", "workspace-b"} {
		request, ok, err := cleanup.request(t.Context(), workspaceID)
		if err != nil || !ok {
			t.Fatalf("request(%s) = %v, %v", workspaceID, ok, err)
		}
		runtime, err := stores.Resolve(t.Context(), request)
		if err != nil {
			t.Fatal(err)
		}
		_, observeErr := runtime.Store.Observe(t.Context(), memory.Observation{
			ID: "fact", Facts: []memory.FactCandidate{{Text: "remembered by " + workspaceID}},
		})
		if err := errors.Join(observeErr, runtime.Closer.Close()); err != nil {
			t.Fatal(err)
		}
	}

	if absent, err := cleanup.WorkspaceMemoryAbsent(t.Context(), "workspace-a"); err != nil || absent {
		t.Fatalf("WorkspaceMemoryAbsent() before purge = %v, %v; want false", absent, err)
	}
	if err := cleanup.PurgeWorkspaceMemory(t.Context(), "workspace-a"); err != nil {
		t.Fatalf("PurgeWorkspaceMemory() error = %v", err)
	}
	if absent, err := cleanup.WorkspaceMemoryAbsent(t.Context(), "workspace-a"); err != nil || !absent {
		t.Fatalf("WorkspaceMemoryAbsent() after purge = %v, %v; want true", absent, err)
	}
	if absent, err := cleanup.WorkspaceMemoryAbsent(t.Context(), "workspace-b"); err != nil || absent {
		t.Fatalf("WorkspaceMemoryAbsent(other Workspace) = %v, %v; want false", absent, err)
	}
}

func TestWorkspaceMemoryCleanupSkipsUnresolvableBindings(t *testing.T) {
	t.Parallel()
	for name, resolve := range map[string]func(context.Context, string) (agenthost.Spec, error){
		"binding not found": func(context.Context, string) (agenthost.Spec, error) {
			return agenthost.Spec{}, errors.Join(agenthost.ErrMemoryBindingNotFound, errors.New("workflow deleted"))
		},
		"no memory": func(context.Context, string) (agenthost.Spec, error) { return agenthost.Spec{}, nil },
	} {
		cleanup := workspaceMemoryCleanup{Resolver: retainedMemoryResolverFunc(resolve), Stores: memorystore.NewRegistry()}
		if err := cleanup.PurgeWorkspaceMemory(t.Context(), "workspace-a"); err != nil {
			t.Fatalf("%s: PurgeWorkspaceMemory() error = %v", name, err)
		}
		if absent, err := cleanup.WorkspaceMemoryAbsent(t.Context(), "workspace-a"); err != nil || !absent {
			t.Fatalf("%s: WorkspaceMemoryAbsent() = %v, %v; want true", name, absent, err)
		}
	}

	transient := errors.New("workflow store unavailable")
	cleanup := workspaceMemoryCleanup{
		Resolver: retainedMemoryResolverFunc(func(context.Context, string) (agenthost.Spec, error) { return agenthost.Spec{}, transient }),
		Stores:   memorystore.NewRegistry(),
	}
	if err := cleanup.PurgeWorkspaceMemory(t.Context(), "workspace-a"); !errors.Is(err, transient) {
		t.Fatalf("PurgeWorkspaceMemory() error = %v, want the transient resolver failure", err)
	}
	if absent, err := cleanup.WorkspaceMemoryAbsent(t.Context(), "workspace-a"); !errors.Is(err, transient) || absent {
		t.Fatalf("WorkspaceMemoryAbsent() = %v, %v; want the transient resolver failure", absent, err)
	}
}
