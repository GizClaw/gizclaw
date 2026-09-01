package memorystore

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	flowrecall "github.com/GizClaw/flowcraft/memory/recall"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
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

func TestSharedFlowcraftRebuildReplaysConcurrentObserveIntoPublishedIndex(t *testing.T) {
	request := bbhTestRequest(t)
	opened, err := openSharedFlowcraft(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	backend := opened.(*sharedFlowcraftBackend)
	t.Cleanup(func() { _ = backend.Close() })

	first, firstCloser, err := backend.NewStore(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = firstCloser.Close() })
	firstStore, err := memory.BindApp(first.Store, request.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := firstStore.Observe(t.Context(), memory.Observation{
		ID: "seed", Facts: []memory.FactCandidate{{Text: "seed fact"}},
	}); err != nil {
		t.Fatal(err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	backend.temporal = &blockingTemporalList{
		TemporalStore:   backend.temporal,
		ScopeEnumerator: backend.temporal.(flowrecall.ScopeEnumerator),
		entered:         entered,
		release:         release,
	}
	changed := request
	changed.Layout.Spec.Flowcraft.Bbh = &apitypes.FlowcraftMemoryBBHPolicy{SearchOverfetch: new(2)}
	type storeResult struct {
		result Result
		closer io.Closer
		err    error
	}
	rebuilt := make(chan storeResult, 1)
	go func() {
		result, closer, err := backend.NewStore(t.Context(), changed)
		rebuilt <- storeResult{result: result, closer: closer, err: err}
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("rebuild did not capture the canonical snapshot")
	}

	observed := make(chan error, 1)
	go func() {
		_, err := firstStore.Observe(t.Context(), memory.Observation{
			ID: "concurrent", Facts: []memory.FactCandidate{{Text: "concurrent platypus fact"}},
		})
		observed <- err
	}()
	select {
	case err := <-observed:
		t.Fatalf("Observe() completed before rebuild publication: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-observed; err != nil {
		t.Fatal(err)
	}
	constructed := <-rebuilt
	if constructed.err != nil {
		t.Fatal(constructed.err)
	}
	t.Cleanup(func() { _ = constructed.closer.Close() })
	rebuiltStore, err := memory.BindApp(constructed.result.Store, changed.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	recalled, err := rebuiltStore.Recall(t.Context(), memory.Query{
		Text: "platypus", Limit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recalled.Matches) != 1 || recalled.Matches[0].Fact.Text != "concurrent platypus fact" {
		t.Fatalf("Recall() matches = %#v, want concurrently observed fact", recalled.Matches)
	}
}

type blockingTemporalList struct {
	flowrecall.TemporalStore
	flowrecall.ScopeEnumerator
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (store *blockingTemporalList) List(
	ctx context.Context,
	scope flowrecall.Scope,
	query flowrecall.ListQuery,
) ([]flowrecall.TemporalFact, error) {
	facts, err := store.TemporalStore.List(ctx, scope, query)
	store.once.Do(func() { close(store.entered) })
	select {
	case <-store.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return facts, err
}

func TestOpenSharedFlowcraftRejectsMismatchedCanonicalLayoutID(t *testing.T) {
	request := objectStoreTestRequest(t)
	request.Layout.Id = "different-layout-id"

	_, err := openSharedFlowcraft(t.Context(), request)
	if err == nil || !strings.Contains(err.Error(), `layout id "different-layout-id" does not match binding layout_id "layout-id"`) {
		t.Fatalf("openSharedFlowcraft() error = %v", err)
	}
}
