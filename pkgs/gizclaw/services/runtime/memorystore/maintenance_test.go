package memorystore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestRegistryPurgeWorkspaceSharesLiveBackend(t *testing.T) {
	t.Parallel()
	registry := NewRegistry()
	t.Cleanup(func() { _ = registry.Close() })
	request := remoteTestRequest(t)
	runtimes := make(map[string]Result)
	for _, workspaceID := range []string{"workspace-a", "workspace-b"} {
		workspaceRequest := request
		workspaceRequest.WorkspaceID = workspaceID
		result, err := registry.Resolve(t.Context(), workspaceRequest)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = result.Closer.Close() })
		if _, err := result.Store.Observe(t.Context(), memory.Observation{
			ID: "fact", Facts: []memory.FactCandidate{{Text: "GIZCLAWMAINTAIN " + workspaceID}},
		}); err != nil {
			t.Fatal(err)
		}
		runtimes[workspaceID] = result
	}

	purged := request
	purged.WorkspaceID = "workspace-a"
	if empty, err := registry.WorkspaceMemoryEmpty(t.Context(), purged); err != nil || empty {
		t.Fatalf("WorkspaceMemoryEmpty() before purge = %v, %v; want false", empty, err)
	}
	if err := registry.PurgeWorkspace(t.Context(), purged); err != nil {
		t.Fatalf("PurgeWorkspace() error = %v", err)
	}
	if empty, err := registry.WorkspaceMemoryEmpty(t.Context(), purged); err != nil || !empty {
		t.Fatalf("WorkspaceMemoryEmpty() after purge = %v, %v; want true", empty, err)
	}
	kept := request
	kept.WorkspaceID = "workspace-b"
	if empty, err := registry.WorkspaceMemoryEmpty(t.Context(), kept); err != nil || empty {
		t.Fatalf("WorkspaceMemoryEmpty(kept) = %v, %v; want false", empty, err)
	}
	if len(registry.entries) != 1 {
		t.Fatalf("physical backends = %d, want the one shared by runtime and maintenance", len(registry.entries))
	}
	for workspaceID, want := range map[string]int{"workspace-a": 0, "workspace-b": 1} {
		recalled, err := runtimes[workspaceID].Store.Recall(t.Context(), memory.Query{Text: "GIZCLAWMAINTAIN", Limit: 5})
		if err != nil {
			t.Fatal(err)
		}
		if len(recalled.Matches) != want {
			t.Fatalf("%s recalled %d facts after purge, want %d", workspaceID, len(recalled.Matches), want)
		}
	}
}

func TestRegistryMaintenanceRequiresRegistryAndWorkspace(t *testing.T) {
	t.Parallel()
	request := remoteTestRequest(t)
	var registry *Registry
	if err := registry.PurgeWorkspace(t.Context(), request); err == nil || !strings.Contains(err.Error(), "registry is required") {
		t.Fatalf("nil registry PurgeWorkspace() error = %v", err)
	}
	request.WorkspaceID = ".."
	if _, err := NewRegistry().WorkspaceMemoryEmpty(t.Context(), request); err == nil {
		t.Fatal("WorkspaceMemoryEmpty() accepted an invalid Workspace ID")
	}
	if _, err := os.Stat(filepath.Join(request.ServerRoot, "data")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid Workspace maintenance touched the server root: %v", err)
	}
}
