//go:build store_e2e

package store_e2e_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/memorystore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

// TestVolcMemoryLayoutScopeRouting exercises the product Registry against a
// real Volc Mem0 project. The project ID and data-plane key must refer to the
// same dedicated test project; no provider value is printed or committed.
func TestVolcMemoryLayoutScopeRouting(t *testing.T) {
	if strings.TrimSpace(os.Getenv("GIZCLAW_MEMORY_PROVIDER")) != "volc-mem0" {
		t.Skip("set GIZCLAW_MEMORY_PROVIDER=volc-mem0 for this live test")
	}
	connection := apitypes.RuntimeProfileMemoryConnection{}
	if err := connection.FromRuntimeProfileVolcMem0Connection(apitypes.RuntimeProfileVolcMem0Connection{
		Type:            apitypes.RuntimeProfileVolcMem0ConnectionTypeVolcMem0,
		MemoryProjectId: requiredEnvironment(t, "GIZCLAW_VOLC_MEM0_PROJECT_ID"),
		Endpoint:        requiredEnvironment(t, "GIZCLAW_VOLC_MEM0_ENDPOINT"),
		ApiKey:          requiredEnvironment(t, "GIZCLAW_VOLC_MEM0_API_KEY"),
	}); err != nil {
		t.Fatal(err)
	}
	sharedScope := apitypes.VolcMem0MemoryLayoutPolicyScopePeer
	privateScope := apitypes.VolcMem0MemoryLayoutPolicyScopeWorkspace
	run := fmt.Sprintf("scope-%x", time.Now().UnixNano())
	t.Logf("Volc memory scope run: %s", run)
	request := memorystore.Request{
		WorkspaceID: "ws-a-" + run, OwnerPublicKey: "owner-" + run,
		ProfileID: "profile-" + run, BindingName: "memory-a",
		Layout: apitypes.MemoryLayout{Id: "scope-layout", Spec: apitypes.MemoryLayoutSpec{
			VolcMem0: apitypes.VolcMem0MemoryLayoutPolicy{Scope: &sharedScope},
		}},
		Binding: apitypes.RuntimeProfileMemoryBinding{
			LayoutId: "scope-layout", Driver: apitypes.RuntimeProfileMemoryDriverVolcMem0,
			Connection: connection,
		},
	}
	registry := memorystore.NewRegistry()
	t.Cleanup(func() { _ = registry.Close() })
	sharedA := request
	sharedB := request
	sharedB.WorkspaceID = "ws-b-" + run
	sharedB.BindingName = "memory-b"
	foreign := request
	foreign.WorkspaceID = "ws-c-" + run
	foreign.OwnerPublicKey = "other-" + run
	private := request
	private.WorkspaceID = "ws-d-" + run
	private.Layout.Spec.VolcMem0.Scope = &privateScope
	requests := []memorystore.Request{sharedA, sharedB, foreign, private}
	results := make([]memorystore.Result, len(requests))
	for i, item := range requests {
		result, err := registry.Resolve(t.Context(), item)
		if err != nil {
			t.Fatalf("Resolve(%d): %v", i, err)
		}
		results[i] = result
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), memoryPurgeDeadline)
		defer cancel()
		for _, item := range []memorystore.Request{sharedA, foreign, private} {
			if err := purgeRegistryScopeUntilEmpty(ctx, registry, item); err != nil {
				t.Errorf("cleanup memory scope: %v", err)
			}
		}
		for _, result := range results {
			if result.Closer != nil {
				_ = result.Closer.Close()
			}
		}
	})
	marker := "GizClaw shared scope " + run
	if _, err := results[0].Store.Observe(t.Context(), memory.Observation{
		ID: "shared-fact", Facts: []memory.FactCandidate{{Text: marker}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := recallMarkerEventually(t.Context(), results[1].Store, marker, true); err != nil {
		t.Fatalf("second Workspace could not recall shared Peer fact: %v", err)
	}
	for _, index := range []int{2, 3} {
		if err := recallMarkerEventually(t.Context(), results[index].Store, marker, false); err != nil {
			t.Fatalf("scope %d unexpectedly recalled shared Peer fact: %v", index, err)
		}
	}
	foreignMarker := "GizClaw other Peer scope " + run
	if _, err := results[2].Store.Observe(t.Context(), memory.Observation{
		ID: "foreign-fact", Facts: []memory.FactCandidate{{Text: foreignMarker}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := recallMarkerEventually(t.Context(), results[2].Store, foreignMarker, true); err != nil {
		t.Fatalf("other Peer could not recall its fact: %v", err)
	}
	privateMarker := "GizClaw private scope " + run
	if _, err := results[3].Store.Observe(t.Context(), memory.Observation{
		ID: "private-fact", Facts: []memory.FactCandidate{{Text: privateMarker}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := recallMarkerEventually(t.Context(), results[3].Store, privateMarker, true); err != nil {
		t.Fatalf("private Workspace could not recall its fact: %v", err)
	}
	if err := recallMarkerEventually(t.Context(), results[1].Store, privateMarker, false); err != nil {
		t.Fatalf("Peer scope recalled private fact: %v", err)
	}
	if err := purgeRegistryScopeUntilEmpty(t.Context(), registry, sharedA); err != nil {
		t.Fatalf("purge shared Peer scope: %v", err)
	}
	if empty, err := registry.WorkspaceMemoryEmpty(t.Context(), sharedB); err != nil || !empty {
		t.Fatalf("second Workspace still sees deleted Peer memory: empty=%v, error=%v", empty, err)
	}
	if err := recallMarkerEventually(t.Context(), results[2].Store, foreignMarker, true); err != nil {
		t.Fatalf("other Peer memory changed after purge: %v", err)
	}
	if err := recallMarkerEventually(t.Context(), results[3].Store, privateMarker, true); err != nil {
		t.Fatalf("private Workspace memory changed after purge: %v", err)
	}
}

func recallMarkerEventually(ctx context.Context, store memory.Store, marker string, want bool) error {
	deadline := time.NewTimer(memoryPurgeDeadline)
	defer deadline.Stop()
	ticker := time.NewTicker(memoryPurgePoll)
	defer ticker.Stop()
	for {
		result, err := store.Recall(ctx, memory.Query{Text: marker, Limit: 10})
		if err != nil {
			return err
		}
		found := false
		for _, match := range result.Matches {
			if strings.Contains(match.Fact.Text, marker) {
				found = true
			}
		}
		if found == want {
			return nil
		}
		if !want {
			return fmt.Errorf("unexpected exact marker %q", marker)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("marker was not recalled within %s", memoryPurgeDeadline)
		case <-ticker.C:
		}
	}
}

func purgeRegistryScopeUntilEmpty(ctx context.Context, registry *memorystore.Registry, request memorystore.Request) error {
	for {
		if err := registry.PurgeWorkspace(ctx, request); err != nil {
			return err
		}
		empty, err := registry.WorkspaceMemoryEmpty(ctx, request)
		if err != nil || empty {
			return err
		}
		timer := time.NewTimer(memoryPurgePoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
