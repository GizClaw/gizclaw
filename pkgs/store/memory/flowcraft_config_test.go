package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/flowcraft/sdk/embedding"
	"github.com/GizClaw/flowcraft/sdk/llm"
)

type testFlowcraftLoader struct {
	llmNames      []string
	embedderNames []string
	model         llm.LLM
	embedder      embedding.Embedder
}

func (l *testFlowcraftLoader) LoadLLM(_ context.Context, name string) (llm.LLM, error) {
	l.llmNames = append(l.llmNames, name)
	return l.model, nil
}

func (l *testFlowcraftLoader) LoadEmbedder(_ context.Context, name string) (embedding.Embedder, error) {
	l.embedderNames = append(l.embedderNames, name)
	return l.embedder, nil
}

type testLLM struct{ response string }

func (m testLLM) Generate(context.Context, []llm.Message, ...llm.GenerateOption) (llm.Message, llm.TokenUsage, error) {
	return llm.NewTextMessage(llm.RoleAssistant, m.response), llm.TokenUsage{}, nil
}

func (testLLM) GenerateStream(context.Context, []llm.Message, ...llm.GenerateOption) (llm.StreamMessage, error) {
	return nil, errors.New("streaming is not used")
}

type testEmbedder struct{}

func (testEmbedder) Embed(context.Context, string) ([]float32, error) { return []float32{1}, nil }
func (testEmbedder) EmbedBatch(_ context.Context, input []string) ([][]float32, error) {
	output := make([][]float32, len(input))
	for i := range output {
		output[i] = []float32{1}
	}
	return output, nil
}

func TestOpenFlowcraftStoreLoadsConfiguredModels(t *testing.T) {
	t.Parallel()
	loader := &testFlowcraftLoader{model: testLLM{response: `{"facts":[]}`}, embedder: testEmbedder{}}
	store, err := OpenFlowcraftStore(context.Background(), FlowcraftConfig{
		RuntimeID: "app", UserID: "user", ExtractionModel: "extract", EmbeddingModel: "embed", RerankModel: "rerank",
	}, loader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if got := loader.llmNames; len(got) != 2 || got[0] != "extract" || got[1] != "rerank" {
		t.Fatalf("loaded LLM names = %v", got)
	}
	if got := loader.embedderNames; len(got) != 1 || got[0] != "embed" {
		t.Fatalf("loaded embedder names = %v", got)
	}
}

func TestOpenFlowcraftStoreRejectsMissingLoader(t *testing.T) {
	t.Parallel()
	_, err := OpenFlowcraftStore(context.Background(), FlowcraftConfig{RuntimeID: "app", UserID: "user", ExtractionModel: "extract"}, nil)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
}

func TestOpenFlowcraftStoreRejectsNilLoadedModels(t *testing.T) {
	t.Parallel()
	for _, config := range []FlowcraftConfig{
		{RuntimeID: "app", UserID: "user", ExtractionModel: "extract"},
		{RuntimeID: "app", UserID: "user", EmbeddingModel: "embed"},
		{RuntimeID: "app", UserID: "user", RerankModel: "rerank"},
	} {
		if _, err := OpenFlowcraftStore(context.Background(), config, &testFlowcraftLoader{}); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("config %+v error = %v", config, err)
		}
	}
}

func TestOpenFlowcraftStoreAcceptsAdditionalRecallOptions(t *testing.T) {
	t.Parallel()
	store, err := OpenFlowcraftStore(context.Background(), FlowcraftConfig{RuntimeID: "app", UserID: "user"}, nil, WithFlowcraftRecallOptions(nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestFlowcraftConfigValidation(t *testing.T) {
	t.Parallel()
	for name, config := range map[string]FlowcraftConfig{
		"runtime":         {UserID: "user"},
		"user":            {RuntimeID: "app"},
		"mode":            {RuntimeID: "app", UserID: "user", ExtractionMode: "unknown"},
		"timeout":         {RuntimeID: "app", UserID: "user", StageTimeout: -1},
		"async extractor": {RuntimeID: "app", UserID: "user", Async: FlowcraftAsyncConfig{Enabled: true}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := config.validate(); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("validate() error = %v, want ErrInvalidInput", err)
			}
		})
	}
}
