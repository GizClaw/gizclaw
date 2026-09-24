package memorystore

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestScopeForRequestUsesSelectedImplementation(t *testing.T) {
	peer := apitypes.FlowcraftMemoryLayoutPolicyScopePeer
	workspace := apitypes.Mem0MemoryLayoutPolicyScopeWorkspace
	volcPeer := apitypes.VolcMem0MemoryLayoutPolicyScopePeer
	request := Request{
		WorkspaceID: "workspace-a", OwnerPublicKey: "owner-a",
		Layout: apitypes.MemoryLayout{Spec: apitypes.MemoryLayoutSpec{
			Flowcraft: apitypes.FlowcraftMemoryLayoutPolicy{Scope: &peer},
			Mem0:      apitypes.Mem0MemoryLayoutPolicy{Scope: &workspace},
			VolcMem0:  apitypes.VolcMem0MemoryLayoutPolicy{Scope: &volcPeer},
		}},
	}
	for _, test := range []struct {
		driver apitypes.RuntimeProfileMemoryDriver
		shared bool
	}{
		{apitypes.RuntimeProfileMemoryDriverFlowcraft, true},
		{apitypes.RuntimeProfileMemoryDriverMem0, false},
		{apitypes.RuntimeProfileMemoryDriverVolcMem0, true},
	} {
		request.Binding.Driver = test.driver
		scope, err := ScopeForRequest(request)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(scope.AppID, "peer:") != test.shared {
			t.Fatalf("%s scope = %#v, shared=%v", test.driver, scope, test.shared)
		}
		if !test.shared && scope != (memory.Scope{AppID: request.WorkspaceID}) {
			t.Fatalf("%s scope = %#v", test.driver, scope)
		}
	}
	request.Binding.Driver = apitypes.RuntimeProfileMemoryDriverMem0
	request.Layout.Spec.Mem0.Scope = nil
	scope, err := ScopeForRequest(request)
	if err != nil || scope.AppID != request.WorkspaceID {
		t.Fatalf("omitted scope = %#v, %v", scope, err)
	}
	request.Binding.Driver = apitypes.RuntimeProfileMemoryDriverFlowcraft
	request.OwnerPublicKey = ""
	if _, err := ScopeForRequest(request); err == nil {
		t.Fatal("peer scope accepted without owner")
	}
}

func TestFlowcraftPeerScopeRedis8CrossAlias(t *testing.T) {
	url := strings.TrimSpace(os.Getenv("FLOWCRAFT_REDIS8_URL"))
	if url == "" {
		t.Skip("FLOWCRAFT_REDIS8_URL is required for the live Redis 8 test")
	}
	request := objectStoreTestRequest(t)
	connection := apitypes.RuntimeProfileMemoryConnection{}
	if err := connection.FromRuntimeProfileFlowcraftRedis8Connection(apitypes.RuntimeProfileFlowcraftRedis8Connection{
		Type: apitypes.RuntimeProfileFlowcraftRedis8ConnectionTypeFlowcraftRedis8,
		Url:  url,
	}); err != nil {
		t.Fatal(err)
	}
	request.Binding.Connection = connection
	request.ProfileID = "scope-test-" + time.Now().Format("20060102150405.000000000")
	request.OwnerPublicKey = "owner-a"
	peerScope := apitypes.FlowcraftMemoryLayoutPolicyScopePeer
	request.Layout.Spec.Flowcraft.Scope = &peerScope
	other := request
	other.WorkspaceID = "workspace-b"
	other.BindingName = "second-alias"
	registry := NewRegistry()
	t.Cleanup(func() { _ = registry.Close() })
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := registry.PurgeWorkspace(ctx, request); err != nil {
			t.Errorf("cleanup Redis Peer scope: %v", err)
		}
	})
	first, err := registry.Resolve(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Closer.Close() })
	second, err := registry.Resolve(t.Context(), other)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Closer.Close() })
	if _, err := first.Store.Observe(t.Context(), memory.Observation{
		ID: "redis-shared", Facts: []memory.FactCandidate{{Text: "The owner prefers jasmine tea."}},
	}); err != nil {
		t.Fatal(err)
	}
	hits, err := second.Store.Recall(t.Context(), memory.Query{Text: "jasmine tea", Limit: 5})
	if err != nil || len(hits.Matches) != 1 {
		t.Fatalf("cross-alias Redis recall = %d hits, %v", len(hits.Matches), err)
	}
	if err := registry.PurgeWorkspace(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	if empty, err := registry.WorkspaceMemoryEmpty(t.Context(), other); err != nil || !empty {
		t.Fatalf("cross-alias Redis purge = empty:%v, error:%v", empty, err)
	}
}

func TestRegistryPeerAndWorkspaceScopesAreDistinct(t *testing.T) {
	registry := NewRegistry()
	t.Cleanup(func() { _ = registry.Close() })
	request := objectStoreTestRequest(t)
	peerScope := apitypes.FlowcraftMemoryLayoutPolicyScopePeer
	request.Layout.Spec.Flowcraft.Scope = &peerScope
	request.OwnerPublicKey = "owner-a"
	first, err := registry.Resolve(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Closer.Close() })
	if _, err := first.Store.Observe(t.Context(), memory.Observation{
		ID: "peer-fact", Facts: []memory.FactCandidate{{Text: "owner likes tea"}},
	}); err != nil {
		t.Fatal(err)
	}
	secondRequest := request
	secondRequest.WorkspaceID = "workspace-b"
	second, err := registry.Resolve(t.Context(), secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Closer.Close() })
	shared, err := second.Store.Recall(t.Context(), memory.Query{Text: "tea", Limit: 5})
	if err != nil || len(shared.Matches) != 1 {
		t.Fatalf("second Workspace recall = %d, %v", len(shared.Matches), err)
	}
	privateScope := apitypes.FlowcraftMemoryLayoutPolicyScopeWorkspace
	privateRequest := request
	privateRequest.Layout.Spec.Flowcraft.Scope = &privateScope
	private, err := registry.Resolve(t.Context(), privateRequest)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Closer.Close() })
	isolated, err := private.Store.Recall(t.Context(), memory.Query{Text: "tea", Limit: 5})
	if err != nil || len(isolated.Matches) != 0 {
		t.Fatalf("isolated recall = %d, %v", len(isolated.Matches), err)
	}
	secondRequest.OwnerPublicKey = "owner-b"
	otherPeer, err := registry.Resolve(t.Context(), secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = otherPeer.Closer.Close() })
	foreign, err := otherPeer.Store.Recall(t.Context(), memory.Query{Text: "tea", Limit: 5})
	if err != nil || len(foreign.Matches) != 0 {
		t.Fatalf("other Peer recall = %d, %v", len(foreign.Matches), err)
	}
}

func TestFlowcraftPeerScopeSharesLayoutAcrossBindingAliases(t *testing.T) {
	request := bbhTestRequest(t)
	request.OwnerPublicKey = "owner-a"
	peerScope := apitypes.FlowcraftMemoryLayoutPolicyScopePeer
	request.Layout.Spec.Flowcraft.Scope = &peerScope
	other := request
	other.WorkspaceID = "workspace-b"
	other.BindingName = "second-alias"
	firstKey, err := registryKey(request)
	if err != nil {
		t.Fatal(err)
	}
	secondKey, err := registryKey(other)
	if err != nil {
		t.Fatal(err)
	}
	if firstKey != secondKey || flowcraftRedis8Prefix(request) != flowcraftRedis8Prefix(other) {
		t.Fatal("Peer Flowcraft aliases using one Layout did not share the physical identity")
	}
	registry := NewRegistry()
	t.Cleanup(func() { _ = registry.Close() })
	first, err := registry.Resolve(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Closer.Close() })
	if _, err := first.Store.Observe(t.Context(), memory.Observation{
		ID: "shared-fact", Facts: []memory.FactCandidate{{Text: "The owner prefers jasmine tea."}},
	}); err != nil {
		t.Fatal(err)
	}
	second, err := registry.Resolve(t.Context(), other)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Closer.Close() })
	hits, err := second.Store.Recall(t.Context(), memory.Query{Text: "jasmine tea", Limit: 5})
	if err != nil || len(hits.Matches) != 1 {
		t.Fatalf("cross-alias Peer recall = %d hits, %v", len(hits.Matches), err)
	}
	workspaceScope := apitypes.FlowcraftMemoryLayoutPolicyScopeWorkspace
	other.Layout.Spec.Flowcraft.Scope = &workspaceScope
	privateKey, err := registryKey(other)
	if err != nil {
		t.Fatal(err)
	}
	if privateKey == firstKey || flowcraftRedis8Prefix(other) == flowcraftRedis8Prefix(request) {
		t.Fatal("Workspace Flowcraft binding reused the Peer physical identity")
	}
	// A Workspace alias can have the same spelling as the former Peer storage
	// name. The two scopes must still have disjoint registry, Redis, and BBH
	// identities.
	other.BindingName = "peer-" + request.Layout.Id
	collidingKey, err := registryKey(other)
	if err != nil {
		t.Fatal(err)
	}
	peerDir, err := flowcraftManagedRoot(request)
	if err != nil {
		t.Fatal(err)
	}
	workspaceDir, err := flowcraftManagedRoot(other)
	if err != nil {
		t.Fatal(err)
	}
	if collidingKey == firstKey || flowcraftRedis8Prefix(other) == flowcraftRedis8Prefix(request) || workspaceDir == peerDir {
		t.Fatal("Workspace alias collided with the Peer physical namespace")
	}
	private, err := registry.Resolve(t.Context(), other)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Closer.Close() })
	hits, err = private.Store.Recall(t.Context(), memory.Query{Text: "jasmine tea", Limit: 5})
	if err != nil || len(hits.Matches) != 0 {
		t.Fatalf("colliding Workspace alias recall = %d hits, %v", len(hits.Matches), err)
	}
}
