//go:build gizclaw_locomo_e2e

package locomo_e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"os"
	"strconv"

	"testing"
	"time"

	memorymem0 "github.com/GizClaw/gizclaw-go/pkgs/store/memory/mem0"
	mem0sdk "github.com/GizClaw/gizclaw-go/sdk/go/mem0"
)

func TestLoCoMoMem0SelfHostedPGVector(t *testing.T) {
	settings := requireLiveSettings(t, liveNeeds{embedding: true})
	settings.observationGranularity = envOr("GIZCLAW_LOCOMO_E2E_OBSERVATION_GRANULARITY", "turn")
	settings.topK = envInt(t, "GIZCLAW_LOCOMO_E2E_TOP_K", 50)
	settings.minF1 = envRatio(t, "GIZCLAW_LOCOMO_E2E_MIN_F1", 0.20)
	endpoint := os.Getenv("GIZCLAW_LOCOMO_E2E_MEM0_PGVECTOR_URL")
	if err := validateRequired(map[string]string{
		"GIZCLAW_LOCOMO_E2E_MEM0_PGVECTOR_URL": endpoint,
	}, "GIZCLAW_LOCOMO_E2E_MEM0_PGVECTOR_URL"); err != nil {
		t.Fatal(err)
	}
	health := requirePGVectorHealth(t, endpoint)
	if health.LlmModel != settings.extractionModel {
		t.Fatalf("Mem0 extraction model = %q, configured test model = %q", health.LlmModel, settings.extractionModel)
	}
	if health.EmbeddingModel != settings.embeddingModel || health.EmbeddingDimensions != settings.embeddingDims {
		t.Fatalf("Mem0 embedding model/dimensions differ from the configured test: %+v", health)
	}
	if health.EmbeddingProtocol != envOr("GIZCLAW_LOCOMO_E2E_MEM0_EMBEDDING_PROTOCOL", "openai") {
		t.Fatalf("Mem0 embedding protocol differs from the configured test: %+v", health)
	}
	instructions := envOr("GIZCLAW_LOCOMO_E2E_MEM0_CUSTOM_INSTRUCTIONS", locomoExtractionInstructions)
	policyHash := sha256.Sum256([]byte(instructions))
	policyFingerprint := hex.EncodeToString(policyHash[:])
	store, err := memorymem0.New(memorymem0.Config{Endpoint: endpoint, Flavor: memorymem0.SelfHosted, CustomInstructions: instructions})
	if err != nil {
		t.Fatal(err)
	}
	profile := "mem0_self_hosted_pgvector"
	fingerprint := configFingerprint(profile, endpoint, health.SdkVersion, "pgvector-pg17",
		settings.modelProvider, health.LlmProvider, health.LlmModel, health.LlmThinking,
		settings.embeddingModel, strconv.Itoa(settings.embeddingDims), strconv.Itoa(health.LlmMaxTokens))
	if health.LlmServiceTier != "" {
		fingerprint = configFingerprint(fingerprint, health.LlmServiceTier)
	}
	if health.EmbeddingProtocol != "openai" {
		fingerprint = configFingerprint(fingerprint, health.EmbeddingProtocol, health.EmbeddingPolicyFingerprint)
	}
	models := reportModels{
		Extraction: health.LlmModel, ExtractionProvider: health.LlmProvider, ExtractionThinking: health.LlmThinking,
		ExtractionServiceTier:       health.LlmServiceTier,
		SDKVersion:                  health.SdkVersion,
		ExtractionPolicyFingerprint: policyFingerprint,
		Embedding:                   settings.embeddingModel, ExtractionMaxTokens: health.LlmMaxTokens,
	}
	if health.EmbeddingProtocol != "openai" {
		models.EmbeddingProtocol = health.EmbeddingProtocol
		models.EmbeddingPolicyFingerprint = health.EmbeddingPolicyFingerprint
	}
	runLiveProfile(t, settings, profile, fingerprint, models, store, nil)
}

const locomoExtractionInstructions = "This is a conversation between two named people, not a user talking to an AI. " +
	"Extract supported facts about both named speakers from new messages. " +
	"Preserve names, ownership, negation, event dates, specific activities and preferences. " +
	"Resolve relative dates using the conversation time supplied with each message, not today's date. " +
	"Distinguish when a statement was made from when the described event happened. " +
	"Treat shared-image captions as descriptions of the image, not unsupported facts about the speaker. " +
	"Ignore greetings and filler. Do not invent details or copy existing memories."

func requirePGVectorHealth(t *testing.T, endpoint string) mem0sdk.HealthStatus {
	t.Helper()
	return requireSelfHostedHealth(t, endpoint, "pgvector")
}

func requireSelfHostedHealth(t *testing.T, endpoint, backend string) mem0sdk.HealthStatus {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	client, err := mem0sdk.NewClientWithResponses(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.HealthWithResponse(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode() != 200 || response.JSON200 == nil {
		t.Fatalf("Mem0 health = HTTP %d; require typed ready health", response.StatusCode())
	}
	health := *response.JSON200
	if health.Status != "ready" || health.VectorStore != backend {
		t.Fatalf("Mem0 health = %+v; require ready %s", health, backend)
	}
	if health.LlmMaxTokens <= 0 || health.LlmModel == "" || health.LlmProvider == "" {
		t.Fatal("Mem0 health must report actual extraction model/provider/budget")
	}
	if health.SdkVersion != "2.2.1" {
		t.Fatalf("Mem0 LoCoMo requires pinned mem0ai 2.2.1, got %q", health.SdkVersion)
	}
	if health.EmbeddingProtocol == "ark_multimodal" && len(health.EmbeddingPolicyFingerprint) != 64 {
		t.Fatal("Mem0 health must report the actual Ark query/corpus embedding policy fingerprint")
	}
	return health
}
