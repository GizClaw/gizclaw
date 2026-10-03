package gizclaw

import (
	"context"
	"database/sql"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/memorystore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestPeerMemoryCleanupWithoutProfile(t *testing.T) {
	cleanup := peerMemoryCleanup{
		ProfileForOwner: func(context.Context, string) (apitypes.RuntimeProfile, error) {
			return apitypes.RuntimeProfile{}, sql.ErrNoRows
		},
		Layouts: peerMemoryLayoutGetter{}, Stores: memorystore.NewRegistry(),
	}
	if err := cleanup.PurgePeerMemory(t.Context(), "owner-a"); err != nil {
		t.Fatal(err)
	}
	if absent, err := cleanup.PeerMemoryAbsent(t.Context(), "owner-a"); err != nil || !absent {
		t.Fatalf("PeerMemoryAbsent = %v, %v", absent, err)
	}
}

type peerMemoryLayoutGetter struct{ layout apitypes.MemoryLayout }

func (g peerMemoryLayoutGetter) GetMemoryLayout(_ context.Context, request adminhttp.GetMemoryLayoutRequestObject) (adminhttp.GetMemoryLayoutResponseObject, error) {
	if request.Id != g.layout.Id {
		return adminhttp.GetMemoryLayout404JSONResponse{}, nil
	}
	return adminhttp.GetMemoryLayout200JSONResponse(g.layout), nil
}

func TestPeerMemoryCleanupPurgesOnlySharedScope(t *testing.T) {
	spec := objectStoreMemorySpec(t)
	peerScope := apitypes.FlowcraftMemoryLayoutPolicyScopePeer
	spec.MemoryLayout.Spec.Flowcraft.Scope = &peerScope
	bindings := map[string]apitypes.RuntimeProfileMemoryBinding{spec.MemoryName: *spec.MemoryBinding}
	profile := apitypes.RuntimeProfile{
		Id: spec.MemoryProfileID, Revision: spec.MemoryProfileRevision,
		Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{Memories: &bindings}},
	}
	stores := memorystore.NewRegistry()
	t.Cleanup(func() { _ = stores.Close() })
	cleanup := peerMemoryCleanup{
		ProfileForOwner: func(context.Context, string) (apitypes.RuntimeProfile, error) { return profile, nil },
		Layouts:         peerMemoryLayoutGetter{layout: *spec.MemoryLayout}, Stores: stores, ServerRoot: t.TempDir(),
	}
	requests, err := cleanup.requests(t.Context(), "owner-a")
	if err != nil || len(requests) != 1 {
		t.Fatalf("shared binding requests = %d, %v", len(requests), err)
	}
	shared, err := stores.Resolve(t.Context(), requests[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := shared.Store.Observe(t.Context(), memory.Observation{ID: "shared", Facts: []memory.FactCandidate{{Text: "owner likes tea"}}}); err != nil {
		t.Fatal(err)
	}
	if err := shared.Closer.Close(); err != nil {
		t.Fatal(err)
	}
	privateRequest := requests[0]
	privateRequest.WorkspaceID = "workspace-a"
	workspaceScope := apitypes.FlowcraftMemoryLayoutPolicyScopeWorkspace
	privateRequest.Layout.Spec.Flowcraft.Scope = &workspaceScope
	private, err := stores.Resolve(t.Context(), privateRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := private.Store.Observe(t.Context(), memory.Observation{ID: "private", Facts: []memory.FactCandidate{{Text: "workspace secret"}}}); err != nil {
		t.Fatal(err)
	}
	if err := private.Closer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cleanup.PurgePeerMemory(t.Context(), "owner-a"); err != nil {
		t.Fatal(err)
	}
	if absent, err := cleanup.PeerMemoryAbsent(t.Context(), "owner-a"); err != nil || !absent {
		t.Fatalf("PeerMemoryAbsent = %v, %v", absent, err)
	}
	if empty, err := stores.WorkspaceMemoryEmpty(t.Context(), privateRequest); err != nil || empty {
		t.Fatalf("private memory was removed: empty=%v, error=%v", empty, err)
	}
}
