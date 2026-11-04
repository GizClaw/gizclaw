//go:build store_e2e

package store_e2e_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	memorymem0 "github.com/GizClaw/gizclaw-go/pkgs/store/memory/mem0"
	memoryvolc "github.com/GizClaw/gizclaw-go/pkgs/store/memory/volc"
)

const (
	memoryPurgeDeadline = 5 * time.Minute
	memoryPurgePoll     = 5 * time.Second
	memoryPurgeSettle   = 15 * time.Second
)

// TestMemoryScopePurge verifies the Workspace deletion purge contract against
// a live remote memory provider: Workspace A's direct Fact and its pending
// extraction job are purged, the purge is re-run until verification reports
// empty (as the deletion handler retries memory_residual), A stays empty after
// the extraction job reaches a terminal state, and Workspace B is untouched.
func TestMemoryScopePurge(t *testing.T) {
	provider := strings.TrimSpace(os.Getenv("GIZCLAW_MEMORY_PROVIDER"))
	if provider == "" {
		t.Skip("GIZCLAW_MEMORY_PROVIDER is not set")
	}
	store := openMemoryPurgeStore(t, provider)
	run := fmt.Sprintf("gizclaw-e2e-purge-%d", time.Now().UnixNano())
	purged := bindMemoryWorkspace(t, store, run+"-a")
	kept := bindMemoryWorkspace(t, store, run+"-b")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), memoryPurgeDeadline)
		defer cancel()
		for _, workspace := range []memory.Store{purged, kept} {
			if _, err := purgeUntilEmpty(ctx, workspace); err != nil {
				t.Errorf("cleanup purge: %v", err)
			}
		}
	})

	ctx, cancel := context.WithTimeout(t.Context(), 3*memoryPurgeDeadline)
	defer cancel()
	for name, workspace := range map[string]memory.Store{"purged": purged, "kept": kept} {
		observed, err := workspace.Observe(ctx, memory.Observation{
			ID:    "direct-fact",
			Facts: []memory.FactCandidate{{Text: "The pet in the " + name + " Workspace is named Mochi."}},
		})
		if err != nil {
			t.Fatalf("Observe(%s direct Fact) error = %v", name, err)
		}
		if len(observed.Facts) == 0 && observed.Operation == nil {
			t.Fatalf("Observe(%s direct Fact) returned neither a Fact nor an operation", name)
		}
	}
	extraction, err := purged.Observe(ctx, memory.Observation{
		ID: "extraction",
		Turns: []memory.Turn{{
			ID: "turn-1", Role: memory.RoleUser,
			Text: "Remember that my cat Mochi loves salmon and sleeps on the blue sofa.",
		}},
	})
	if err != nil {
		t.Fatalf("Observe(extraction) error = %v", err)
	}
	if empty, err := memory.ScopeEmpty(ctx, purged, memory.Scope{}); err != nil || empty {
		t.Fatalf("ScopeEmpty(purged) before purge = %v, %v; want the direct Fact", empty, err)
	}

	// Purge while the extraction job may still be running, as a Workspace
	// deletion does after quiescing a runtime whose writes were accepted.
	if err := memory.PurgeScope(ctx, purged, memory.Scope{}); err != nil {
		t.Fatalf("PurgeScope() error = %v", err)
	}
	if extraction.Operation != nil {
		waitMemoryOperation(t, ctx, purged, extraction.Operation.ID)
	}
	rounds, err := purgeUntilEmpty(ctx, purged)
	if err != nil {
		t.Fatalf("purge until empty: %v", err)
	}
	t.Logf("%s: Workspace memory verified empty after %d purge round(s)", provider, rounds)

	settle := time.NewTimer(memoryPurgeSettle)
	select {
	case <-settle.C:
	case <-ctx.Done():
		settle.Stop()
		t.Fatal(ctx.Err())
	}
	if empty, err := memory.ScopeEmpty(ctx, purged, memory.Scope{}); err != nil || !empty {
		t.Fatalf("ScopeEmpty(purged) after settling = %v, %v; want no late materialization", empty, err)
	}
	if empty, err := memory.ScopeEmpty(ctx, kept, memory.Scope{}); err != nil || empty {
		t.Fatalf("ScopeEmpty(kept) = %v, %v; purge removed another Workspace's memory", empty, err)
	}
}

func openMemoryPurgeStore(t *testing.T, provider string) memory.Store {
	t.Helper()
	switch provider {
	case "volc-mem0":
		store, err := memoryvolc.Open(t.Context(), memoryvolc.Config{Mem0: memorymem0.Config{
			Endpoint: requiredEnvironment(t, "GIZCLAW_VOLC_MEM0_ENDPOINT"),
			APIKey:   requiredEnvironment(t, "GIZCLAW_VOLC_MEM0_API_KEY"),
		}})
		if err != nil {
			t.Fatal(err)
		}
		return store
	case "mem0-platform":
		store, err := memorymem0.New(memorymem0.Config{
			Endpoint: strings.TrimSpace(os.Getenv("GIZCLAW_MEM0_ENDPOINT")),
			APIKey:   requiredEnvironment(t, "GIZCLAW_MEM0_API_KEY"),
			Flavor:   memorymem0.Platform,
		})
		if err != nil {
			t.Fatal(err)
		}
		return store
	case "mem0-self-hosted":
		store, err := memorymem0.New(memorymem0.Config{
			Endpoint: requiredEnvironment(t, "GIZCLAW_MEM0_SELF_HOSTED_URL"),
			APIKey:   strings.TrimSpace(os.Getenv("GIZCLAW_MEM0_SELF_HOSTED_API_KEY")),
			Flavor:   memorymem0.SelfHosted,
		})
		if err != nil {
			t.Fatal(err)
		}
		return store
	default:
		t.Fatalf("GIZCLAW_MEMORY_PROVIDER must be volc-mem0, mem0-platform, or mem0-self-hosted, got %q", provider)
		return nil
	}
}

func bindMemoryWorkspace(t *testing.T, store memory.Store, workspaceID string) memory.Store {
	t.Helper()
	bound, err := memory.BindApp(store, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	return bound
}

// waitMemoryOperation waits until the provider reports the extraction job
// terminal. A job whose Facts were purged before it finished can report an
// error; only a job that never finishes fails the test.
func waitMemoryOperation(t *testing.T, ctx context.Context, store memory.Store, operationID string) {
	t.Helper()
	waiter, ok := store.(memory.OperationWaiter)
	if !ok {
		t.Fatal("provider returned an operation but cannot wait for it")
	}
	waitCtx, cancel := context.WithTimeout(ctx, memoryPurgeDeadline)
	defer cancel()
	result, err := waiter.Wait(waitCtx, memory.OperationRequest{ID: operationID})
	if waitCtx.Err() != nil {
		t.Fatalf("extraction job did not finish within %s", memoryPurgeDeadline)
	}
	status := "error"
	if result.Operation != nil {
		status = string(result.Operation.Status)
	}
	t.Logf("extraction job finished: status=%s facts=%d wait_error=%v", status, len(result.Facts), err != nil)
}

// purgeUntilEmpty repeats purge and verification the way Workspace deletion
// retries memory_residual, returning the number of purge rounds.
func purgeUntilEmpty(ctx context.Context, store memory.Store) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, memoryPurgeDeadline)
	defer cancel()
	for round := 1; ; round++ {
		if err := memory.PurgeScope(ctx, store, memory.Scope{}); err != nil {
			return round, fmt.Errorf("round %d purge: %w", round, err)
		}
		empty, err := memory.ScopeEmpty(ctx, store, memory.Scope{})
		if err != nil {
			return round, fmt.Errorf("round %d verify: %w", round, err)
		}
		if empty {
			return round, nil
		}
		timer := time.NewTimer(memoryPurgePoll)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return round, fmt.Errorf("memory still present after %d purge round(s): %w", round, ctx.Err())
		}
	}
}
