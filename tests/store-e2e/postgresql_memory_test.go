//go:build store_e2e

package store_e2e_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/flowcraft/memory/recall"
	flowpostgres "github.com/GizClaw/flowcraft/memory/recall/store/postgres"
	"github.com/GizClaw/flowcraft/memory/retrieval"
	retrievalpostgres "github.com/GizClaw/flowcraft/memory/retrieval/postgres"
	"github.com/GizClaw/flowcraft/sdk/embedding"
	"github.com/GizClaw/flowcraft/sdk/llm"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/memorystore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

// TestPostgreSQLFlowcraftMemoryPurge runs Workspace deletion's maintenance
// purge against a flowcraft_postgresql binding: canonical facts, the retrieval
// index, and a queued extraction job of the purged Workspace are removed while
// another Workspace on the same binding keeps its memory.
func TestPostgreSQLFlowcraftMemoryPurge(t *testing.T) {
	dsn := requiredEnvironment(t, "GIZCLAW_TEST_POSTGRES_DSN")
	ctx := t.Context()
	// Product Workspace IDs are 16 hex characters. flowcraft_postgresql's
	// retrieval namespace ("recall_<id>__global") is limited to 48 bytes.
	purged, kept := randomWorkspaceID(t), randomWorkspaceID(t)
	registry := memorystore.NewRegistry()
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		for _, workspaceID := range []string{purged, kept} {
			if err := registry.PurgeWorkspace(cleanupCtx, postgresMemoryRequest(t, dsn, workspaceID)); err != nil {
				t.Errorf("cleanup purge %s: %v", workspaceID, err)
			}
		}
		if err := registry.Close(); err != nil {
			t.Error(err)
		}
	})

	for _, workspaceID := range []string{purged, kept} {
		result, err := registry.Resolve(ctx, postgresMemoryRequest(t, dsn, workspaceID))
		if err != nil {
			t.Fatalf("Resolve(%s) error = %v", workspaceID, err)
		}
		_, observeErr := result.Store.Observe(ctx, memory.Observation{
			ID: "direct-fact", Facts: []memory.FactCandidate{{Text: "GIZCLAWPGPURGE pet fact of " + workspaceID}},
		})
		var pending memory.ObserveResult
		if observeErr == nil && workspaceID == purged {
			pending, observeErr = result.Store.Observe(ctx, memory.Observation{
				ID: "extraction", Turns: []memory.Turn{{ID: "turn", Role: memory.RoleUser, Text: "My cat likes salmon."}},
			})
		}
		if err := errors.Join(observeErr, result.Closer.Close()); err != nil {
			t.Fatalf("Observe(%s) error = %v", workspaceID, err)
		}
		if workspaceID == purged && (pending.Operation == nil || pending.Operation.Status != memory.OperationPending) {
			t.Fatalf("extraction Observe() = %+v, want a queued job", pending)
		}
	}

	backend, err := flowpostgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	index, err := retrievalpostgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	if jobs := queuedJobs(t, backend, purged); jobs == 0 {
		t.Fatal("extraction job was not queued before purge")
	}

	purgedRequest := postgresMemoryRequest(t, dsn, purged)
	if empty, err := registry.WorkspaceMemoryEmpty(ctx, purgedRequest); err != nil || empty {
		t.Fatalf("WorkspaceMemoryEmpty(purged) before purge = %v, %v; want false", empty, err)
	}
	if err := registry.PurgeWorkspace(ctx, purgedRequest); err != nil {
		t.Fatalf("PurgeWorkspace() error = %v", err)
	}
	if empty, err := registry.WorkspaceMemoryEmpty(ctx, purgedRequest); err != nil || !empty {
		t.Fatalf("WorkspaceMemoryEmpty(purged) after purge = %v, %v; want true", empty, err)
	}
	if empty, err := registry.WorkspaceMemoryEmpty(ctx, postgresMemoryRequest(t, dsn, kept)); err != nil || empty {
		t.Fatalf("WorkspaceMemoryEmpty(kept) = %v, %v; want false", empty, err)
	}
	if jobs := queuedJobs(t, backend, purged); jobs != 0 {
		t.Fatalf("queued extraction jobs after purge = %d, want 0", jobs)
	}
	for workspaceID, want := range map[string]bool{purged: false, kept: true} {
		docs, err := index.List(ctx, recall.NamespaceFor(recall.Scope{RuntimeID: workspaceID}), retrieval.ListRequest{PageSize: 10})
		if err != nil {
			t.Fatal(err)
		}
		if got := len(docs.Items) > 0; got != want {
			t.Fatalf("retrieval docs for %s present = %v, want %v", workspaceID, got, want)
		}
	}
}

func randomWorkspaceID(t *testing.T) string {
	t.Helper()
	var id [8]byte
	if _, err := rand.Read(id[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(id[:])
}

func postgresMemoryRequest(t *testing.T, dsn, workspaceID string) memorystore.Request {
	t.Helper()
	connection := apitypes.RuntimeProfileMemoryConnection{}
	if err := connection.FromRuntimeProfileFlowcraftPostgreSQLConnection(apitypes.RuntimeProfileFlowcraftPostgreSQLConnection{
		Type: apitypes.RuntimeProfileFlowcraftPostgreSQLConnectionTypeFlowcraftPostgresql, Dsn: dsn,
	}); err != nil {
		t.Fatal(err)
	}
	return memorystore.Request{
		WorkspaceID: workspaceID, ProfileID: "store-e2e-profile", ProfileRevision: "revision", BindingName: "pet-memory",
		Layout: apitypes.MemoryLayout{Id: "store-e2e-layout", Spec: apitypes.MemoryLayoutSpec{Flowcraft: apitypes.FlowcraftMemoryLayoutPolicy{
			Extraction: apitypes.FlowcraftMemoryExtractionPolicy{Model: "extract", Mode: apitypes.FlowcraftMemoryExtractionPolicyModeSinglePass},
			Write: apitypes.FlowcraftMemoryWritePolicy{
				Mode: apitypes.FlowcraftMemoryWritePolicyModeAsyncSemantic, Tier: apitypes.FlowcraftMemoryWritePolicyTierGeneral,
			},
		}}},
		Binding: apitypes.RuntimeProfileMemoryBinding{
			LayoutId: "store-e2e-layout", Driver: apitypes.RuntimeProfileMemoryDriverFlowcraft, Connection: connection,
		},
		ModelLoader: queuedExtractionLoader{},
	}
}

func queuedJobs(t *testing.T, backend *flowpostgres.Backend, workspaceID string) int {
	t.Helper()
	stats, err := backend.AsyncSemanticQueue().Stats(t.Context(), recall.AsyncSemanticStatsFilter{Scope: recall.Scope{RuntimeID: workspaceID}})
	if err != nil {
		t.Fatalf("queue Stats(%s) error = %v", workspaceID, err)
	}
	return stats.Pending + stats.Leased + stats.Failed + stats.DeadLetter + stats.Completed
}

// queuedExtractionLoader satisfies the async extraction policy. The test never
// processes the queued job, so the model is never called.
type queuedExtractionLoader struct{}

func (queuedExtractionLoader) LoadLLM(context.Context, string) (llm.LLM, error) {
	return queuedExtractionModel{}, nil
}

func (queuedExtractionLoader) LoadEmbedder(context.Context, string) (embedding.Embedder, error) {
	return nil, errors.New("store-e2e flowcraft purge configures no embedder")
}

type queuedExtractionModel struct{}

func (queuedExtractionModel) Generate(context.Context, []llm.Message, ...llm.GenerateOption) (llm.Message, llm.TokenUsage, error) {
	return llm.NewTextMessage(llm.RoleAssistant, `{"facts":[]}`), llm.TokenUsage{}, nil
}

func (queuedExtractionModel) GenerateStream(context.Context, []llm.Message, ...llm.GenerateOption) (llm.StreamMessage, error) {
	return nil, errors.New("store-e2e flowcraft purge does not stream")
}
