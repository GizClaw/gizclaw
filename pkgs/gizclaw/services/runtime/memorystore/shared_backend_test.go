package memorystore

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	memoryflowcraft "github.com/GizClaw/gizclaw-go/pkgs/store/memory/flowcraft"
)

func TestOpenSharedFlowcraftAcceptsCanonicalLayoutID(t *testing.T) {
	request := objectStoreTestRequest(t)

	backend, err := openSharedFlowcraft(t.Context(), request)
	if err != nil {
		t.Fatalf("openSharedFlowcraft() error = %v", err)
	}
	t.Cleanup(func() { _ = backend.Close() })
}

func TestSharedFlowcraftSameSignatureStoresConstructConcurrently(t *testing.T) {
	request := objectStoreTestRequest(t)
	opened, err := openSharedFlowcraft(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	backend := opened.(*sharedFlowcraftBackend)
	t.Cleanup(func() { _ = backend.Close() })

	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	backend.newStore = func(ctx context.Context, config memoryflowcraft.Config) (*memoryflowcraft.Store, error) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return memoryflowcraft.New(ctx, config)
	}
	type constructed struct {
		result Result
		closer io.Closer
		err    error
	}
	results := make(chan constructed, 2)
	construct := func(workspace string) {
		workspaceRequest := request
		workspaceRequest.WorkspaceID = workspace
		result, closer, err := backend.NewStore(t.Context(), workspaceRequest)
		results <- constructed{result: result, closer: closer, err: err}
	}
	go construct("workspace-a")
	wantFlowcraftConstructor(t, entered)
	go construct("workspace-b")
	wantFlowcraftConstructor(t, entered)
	close(release)

	for range 2 {
		constructed := <-results
		if constructed.err != nil {
			t.Fatal(constructed.err)
		}
		if constructed.result.Store == nil || constructed.closer == nil {
			t.Fatal("logical Flowcraft Store was not constructed")
		}
		if err := constructed.closer.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func wantFlowcraftConstructor(t *testing.T, entered <-chan struct{}) {
	t.Helper()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("same-signature logical Store constructor did not overlap")
	}
}

func TestSharedFlowcraftObjectStoreKeepsConcurrentWorkspacesIsolated(t *testing.T) {
	request := objectStoreTestRequest(t)
	backend, err := openSharedFlowcraft(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	shared := backend.(*sharedFlowcraftBackend)
	if shared.temporal == nil || shared.index == nil {
		t.Fatal("shared Flowcraft backend did not retain canonical and retrieval dependencies")
	}
	const workspaces = 4
	const factsPerWorkspace = 12
	stores := make([]memory.Store, workspaces)
	closers := make([]interface{ Close() error }, workspaces)
	for workspace := range workspaces {
		logicalRequest := request
		logicalRequest.WorkspaceID = fmt.Sprintf("workspace-%d", workspace)
		result, closer, err := backend.NewStore(t.Context(), logicalRequest)
		if err != nil {
			t.Fatal(err)
		}
		stores[workspace], err = memory.BindApp(result.Store, logicalRequest.WorkspaceID)
		if err != nil {
			t.Fatal(err)
		}
		closers[workspace] = closer
	}
	t.Cleanup(func() {
		for _, closer := range closers {
			_ = closer.Close()
		}
	})

	var wait sync.WaitGroup
	errorsByWorkspace := make([]error, workspaces)
	for workspace := range workspaces {
		wait.Go(func() {
			for fact := range factsPerWorkspace {
				_, err := stores[workspace].Observe(t.Context(), memory.Observation{
					ID: fmt.Sprintf("observation-%d-%d", workspace, fact),
					Facts: []memory.FactCandidate{{
						Text: fmt.Sprintf("workspace-%d fact-%d", workspace, fact),
					}},
				})
				if err != nil {
					errorsByWorkspace[workspace] = err
					return
				}
			}
		})
	}
	wait.Wait()
	for workspace, err := range errorsByWorkspace {
		if err != nil {
			t.Fatalf("workspace %d Observe() error = %v", workspace, err)
		}
		recalled, err := stores[workspace].Recall(t.Context(), memory.Query{
			Text: fmt.Sprintf("workspace-%d", workspace), Limit: factsPerWorkspace,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(recalled.Matches) != factsPerWorkspace {
			t.Fatalf("workspace %d matches = %d, want %d", workspace, len(recalled.Matches), factsPerWorkspace)
		}
		for _, match := range recalled.Matches {
			if !strings.Contains(match.Fact.Text, fmt.Sprintf("workspace-%d ", workspace)) {
				t.Fatalf("workspace %d recalled foreign fact %q", workspace, match.Fact.Text)
			}
		}
	}
}

func TestOpenSharedFlowcraftRejectsMismatchedCanonicalLayoutID(t *testing.T) {
	request := objectStoreTestRequest(t)
	request.Layout.Id = "different-layout-id"

	_, err := openSharedFlowcraft(t.Context(), request)
	if err == nil || !strings.Contains(err.Error(), `layout id "different-layout-id" does not match binding layout_id "layout-id"`) {
		t.Fatalf("openSharedFlowcraft() error = %v", err)
	}
}
