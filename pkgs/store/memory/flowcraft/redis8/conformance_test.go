package redis8

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/GizClaw/flowcraft/memory/recall"
	"github.com/GizClaw/flowcraft/memory/recall/recalltest"
	"github.com/GizClaw/flowcraft/memory/retrieval"
	"github.com/GizClaw/flowcraft/memory/retrieval/contract"
	"github.com/redis/go-redis/v9"
)

func redis8TestClient(t testing.TB) *redis.Client {
	t.Helper()
	url := os.Getenv("FLOWCRAFT_REDIS8_URL")
	if url == "" {
		t.Skip("FLOWCRAFT_REDIS8_URL is not set")
	}
	options, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(options)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func freshBackend(t testing.TB) *Backend {
	t.Helper()
	client := redis8TestClient(t)
	prefix := fmt.Sprintf("gizclaw:test:flowcraft:redis8:%d", time.Now().UnixNano())
	backend, err := OpenBackend(context.Background(), client, prefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		keys, _ := client.Keys(context.Background(), prefix+"*").Result()
		if len(keys) > 0 {
			_ = client.Del(context.Background(), keys...).Err()
		}
		_ = client.FTDropIndex(context.Background(), backend.index.indexName).Err()
	})
	return backend
}

func TestRecallConformance(t *testing.T) {
	recalltest.RunTemporalStoreSuite(t, func(t testing.TB) recall.TemporalStore { return freshBackend(t).TemporalStore() })
	recalltest.RunScopeEnumeratorSuite(t, func(t testing.TB) (recall.TemporalStore, recall.ScopeEnumerator) {
		store := freshBackend(t).TemporalStore()
		return store, store.(recall.ScopeEnumerator)
	})
	recalltest.RunEvidenceStoreSuite(t, func(t testing.TB) recall.EvidenceStore { return freshBackend(t).EvidenceStore() })
	recalltest.RunAsyncSemanticQueueSuite(t, func(t testing.TB) recall.AsyncSemanticQueue { return freshBackend(t).AsyncSemanticQueue() })
	recalltest.RunSideEffectOutboxSuite(t, func(t testing.TB) recall.SideEffectOutbox { return freshBackend(t).SideEffectOutbox() })
}

func TestRetrievalConformance(t *testing.T) {
	contract.Run(t, func(t *testing.T) (retrieval.Index, func()) {
		index := freshBackend(t).RetrievalIndex()
		return index, func() {}
	})
}

func TestSearchUsesRedisNativeHybridAndFilter(t *testing.T) {
	backend := freshBackend(t)
	index := backend.RetrievalIndex()
	ctx := context.Background()
	const namespace = "native-hybrid"
	if err := index.Upsert(ctx, namespace, []retrieval.Doc{
		{ID: "keep", Content: "likes tea", Vector: []float32{1, 0}, Metadata: map[string]any{"kind": "preference", "entities": []any{"tea", "user"}}, Timestamp: time.Now()},
		{ID: "drop-filter", Content: "likes tea", Vector: []float32{1, 0}, Metadata: map[string]any{"kind": "episode", "entities": []any{"tea"}}, Timestamp: time.Now()},
		{ID: "drop-evidence", Content: "unrelated", Vector: []float32{0, 1}, Metadata: map[string]any{"kind": "preference", "entities": []any{"tea"}}, Timestamp: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}

	// The legacy client-side path enumerates this set before searching. Removing
	// it proves this request is answered by FT.HYBRID and Redis-side filters.
	if err := backend.client.Del(ctx, backend.index.namespaceKey(namespace)).Err(); err != nil {
		t.Fatal(err)
	}
	response, err := index.Search(ctx, namespace, retrieval.SearchRequest{
		QueryText:   "tea",
		QueryVector: []float32{1, 0},
		Filter: retrieval.Filter{
			Eq:          map[string]any{"kind": "preference"},
			ContainsAny: map[string][]any{"entities": {"tea"}},
		},
		TopK: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Hits) != 1 || response.Hits[0].Doc.ID != "keep" {
		t.Fatalf("Search() hits = %#v, want only keep", response.Hits)
	}
	if response.Hits[0].Scores["bm25"] <= 0 || response.Hits[0].Scores["cos"] <= 0 {
		t.Fatalf("Search() scores = %#v, want positive Redis text and vector lanes", response.Hits[0].Scores)
	}
}

func TestScopeEnumeratorPreservesCrossAgentHardPartition(t *testing.T) {
	store := freshBackend(t).TemporalStore()
	enumerator := store.(recall.ScopeEnumerator)
	ctx := context.Background()
	observedAt := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	if err := store.Append(ctx, []recall.TemporalFact{
		{ID: "agent-a-fact", Scope: recall.Scope{RuntimeID: "runtime", UserID: "user", AgentID: "agent-a"}, Kind: recall.FactPreference, Content: "a", ObservedAt: observedAt},
		{ID: "agent-b-fact", Scope: recall.Scope{RuntimeID: "runtime", UserID: "user", AgentID: "agent-b"}, Kind: recall.FactPreference, Content: "b", ObservedAt: observedAt.Add(time.Second)},
	}); err != nil {
		t.Fatal(err)
	}

	scopes, err := enumerator.ListScopes(ctx, recall.ScopeListQuery{RuntimeID: "runtime"})
	if err != nil {
		t.Fatal(err)
	}
	if len(scopes) != 1 || scopes[0].RuntimeID != "runtime" || scopes[0].UserID != "user" || scopes[0].AgentID != "" {
		t.Fatalf("ListScopes() = %#v, want one canonical runtime/user hard partition", scopes)
	}
	facts, err := store.List(ctx, scopes[0], recall.ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 || facts[0].ID != "agent-a-fact" || facts[1].ID != "agent-b-fact" {
		t.Fatalf("List(canonical scope) = %#v, want facts from both agent metadata values", facts)
	}
}
