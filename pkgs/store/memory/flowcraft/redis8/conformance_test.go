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
