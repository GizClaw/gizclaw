//go:build gizclaw_locomo_e2e

package locomo_e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	memorymem0 "github.com/GizClaw/gizclaw-go/pkgs/store/memory/mem0"
	memoryvolc "github.com/GizClaw/gizclaw-go/pkgs/store/memory/volc"
)

func TestLoCoMoVolcAgentKitCustomInstructions(t *testing.T) {
	settings := requireLiveSettings(t, liveNeeds{})
	settings.cleanupScopes = true
	settings.observationGranularity = envOr("GIZCLAW_LOCOMO_E2E_OBSERVATION_GRANULARITY", "turn")
	settings.topK = envInt(t, "GIZCLAW_LOCOMO_E2E_TOP_K", 50)
	settings.minF1 = envRatio(t, "GIZCLAW_LOCOMO_E2E_MIN_F1", .20)
	instructions := strings.TrimSpace(os.Getenv("GIZCLAW_LOCOMO_E2E_VOLC_CUSTOM_INSTRUCTIONS"))
	if instructions == "" {
		t.Fatal("GIZCLAW_LOCOMO_E2E_VOLC_CUSTOM_INSTRUCTIONS is required for the controlled Volc comparison")
	}
	config, identity := requireVolcConfig(t)
	config.Mem0.HTTPClient = volcInstructionsClient{base: http.DefaultClient, instructions: instructions}
	store, err := memoryvolc.Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	policyHash := sha256.Sum256([]byte(instructions))
	policyFingerprint := hex.EncodeToString(policyHash[:])
	profile := "volc_agentkit_custom_instructions"
	fingerprint := configFingerprint(profile, config.Mem0.Endpoint, identity, policyFingerprint)
	runLiveProfile(t, settings, profile, fingerprint,
		reportModels{ExtractionPolicyFingerprint: policyFingerprint}, &volcContextStore{Store: store}, nil)
}

// volcContextStore sends the same source-speaker/time text that the self-hosted
// adapter sends. The shared Volc production adapter remains provider-native.
type volcContextStore struct {
	*memoryvolc.Store
}

func (s *volcContextStore) Observe(ctx context.Context, observation memorystore.Observation) (memorystore.ObserveResult, error) {
	observation.Turns = slices.Clone(observation.Turns)
	if strings.TrimSpace(observation.Text) != "" {
		observation.Text = comparisonContent(observation.Text, "", observation.ObservedAt)
	}
	for index := range observation.Turns {
		turn := &observation.Turns[index]
		observedAt := turn.ObservedAt
		if observedAt.IsZero() {
			observedAt = observation.ObservedAt
		}
		turn.Text = comparisonContent(turn.Text, turn.Speaker, observedAt)
	}
	return s.Store.Observe(ctx, observation)
}

func comparisonContent(content, speaker string, observedAt time.Time) string {
	if speaker = strings.TrimSpace(speaker); speaker != "" {
		content = speaker + ": " + content
	}
	if !observedAt.IsZero() {
		content = "[Conversation time: " + observedAt.UTC().Format(time.RFC3339) + "]\n" + content
	}
	return content
}

// The override is request-local; no shared project strategy is modified.
type volcInstructionsClient struct {
	base         memorymem0.HTTPClient
	instructions string
}

func (c volcInstructionsClient) Do(request *http.Request) (*http.Response, error) {
	if request.Method != http.MethodPost || strings.TrimRight(request.URL.Path, "/") != "/v1/memories" || request.Body == nil {
		return c.base.Do(request)
	}
	const maximumBody = 1 << 20
	raw, readErr := io.ReadAll(io.LimitReader(request.Body, maximumBody+1))
	if err := errors.Join(readErr, request.Body.Close()); err != nil {
		return nil, fmt.Errorf("read comparison memory request: %w", err)
	}
	if len(raw) > maximumBody {
		return nil, errors.New("comparison memory request exceeds 1 MiB")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode comparison memory request: %w", err)
	}
	if string(payload["infer"]) == "true" {
		instructions, err := json.Marshal(c.instructions)
		if err != nil {
			return nil, err
		}
		payload["custom_instructions"] = instructions
		raw, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encode comparison memory request: %w", err)
		}
	}
	clone := request.Clone(request.Context())
	clone.Body = io.NopCloser(bytes.NewReader(raw))
	clone.ContentLength = int64(len(raw))
	clone.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(raw)), nil }
	return c.base.Do(clone)
}

func purgeBenchmarkScope(ctx context.Context, store memorystore.Store, scope memorystore.Scope, interval time.Duration) error {
	consecutiveEmpty := 0
	for consecutiveEmpty < 3 {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := memorystore.PurgeScope(ctx, store, scope); err != nil {
			return err
		}
		empty, err := memorystore.ScopeEmpty(ctx, store, scope)
		if err != nil {
			return err
		}
		if empty {
			consecutiveEmpty++
		} else {
			consecutiveEmpty = 0
		}
		if consecutiveEmpty == 3 {
			return nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

func TestVolcComparisonPreservesSourceAndInjectsOnlyInferredWrites(t *testing.T) {
	received := make(chan map[string]json.RawMessage, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var payload map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			received <- payload
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":[{"event_id":"fixture-event"}]}`)
	}))
	defer server.Close()
	client := volcInstructionsClient{base: server.Client(), instructions: "preserve names and event dates"}
	store, err := memoryvolc.Open(t.Context(), memoryvolc.Config{Mem0: memorymem0.Config{
		Endpoint: server.URL, APIKey: "fixture", HTTPClient: client,
	}})
	if err != nil {
		t.Fatal(err)
	}
	input := memorystore.Observation{Scope: memorystore.Scope{AppID: "comparison-only"}, ID: "observation",
		ObservedAt: time.Date(2023, 1, 20, 16, 4, 0, 0, time.UTC),
		Turns:      []memorystore.Turn{{Role: memorystore.RoleUser, Speaker: "Alice", Text: "Yesterday I went hiking."}},
	}
	wrapped := &volcContextStore{Store: store}
	if _, err := wrapped.Observe(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	if input.Turns[0].Text != "Yesterday I went hiking." {
		t.Fatal("comparison mutated source input")
	}
	payload := <-received
	if string(payload["custom_instructions"]) != `"preserve names and event dates"` {
		t.Fatalf("missing request-local instructions: %v", payload)
	}
	var messages []struct{ Role, Content, Name string }
	if err := json.Unmarshal(payload["messages"], &messages); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Name != "Alice" || messages[0].Role != "user" ||
		messages[0].Content != "[Conversation time: 2023-01-20T16:04:00Z]\nAlice: Yesterday I went hiking." {
		t.Fatalf("source speaker/time not preserved: %+v", messages)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/v1/memories/", strings.NewReader(`{"infer":false,"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if payload := <-received; payload["custom_instructions"] != nil {
		t.Fatal("instructions were injected into a direct import")
	}
}

type failedComparisonStore struct {
	memorystore.Store
	onObserve func(memorystore.Scope)
}

func (s failedComparisonStore) Observe(_ context.Context, observation memorystore.Observation) (memorystore.ObserveResult, error) {
	s.onObserve(observation.Scope)
	return memorystore.ObserveResult{}, errors.New("provider write failed")
}

func TestVolcComparisonRegistersCleanupBeforeFailedWrite(t *testing.T) {
	var registered memorystore.Scope
	store := failedComparisonStore{onObserve: func(scope memorystore.Scope) {
		if scope != registered || registered.AppID == "" {
			t.Fatal("write began before cleanup registration")
		}
	}}
	_, err := runBenchmark(t.Context(), benchmarkOptions{Profile: "volc", IngestTimeout: time.Second,
		OnScope: func(scope memorystore.Scope) { registered = scope },
	}, store, validOfflineDataset(), nil)
	if err == nil || !strings.Contains(err.Error(), "provider write failed") {
		t.Fatalf("ingestion failure was hidden: %v", err)
	}
}

type cleanupProbeStore struct {
	memorystore.Store
	purged []memorystore.Scope
	empty  []bool
	err    error
}

func (s *cleanupProbeStore) PurgeScope(_ context.Context, scope memorystore.Scope) error {
	s.purged = append(s.purged, scope)
	return s.err
}

func (s *cleanupProbeStore) ScopeEmpty(context.Context, memorystore.Scope) (bool, error) {
	if len(s.empty) == 0 {
		return false, errors.New("unexpected scope probe")
	}
	empty := s.empty[0]
	s.empty = s.empty[1:]
	return empty, nil
}

func TestVolcComparisonCleanupVerifiesOnlyOwnedScope(t *testing.T) {
	store := &cleanupProbeStore{empty: []bool{true, false, true, true, true}}
	scope := memorystore.Scope{AppID: "locomo-owned"}
	if err := purgeBenchmarkScope(t.Context(), store, scope, time.Nanosecond); err != nil {
		t.Fatal(err)
	}
	if len(store.purged) != 5 {
		t.Fatalf("purge attempts=%d, want 5", len(store.purged))
	}
	for _, purged := range store.purged {
		if purged != scope {
			t.Fatal("cleanup widened the owned scope")
		}
	}
	store.err = errors.New("provider rejected purge")
	if err := purgeBenchmarkScope(t.Context(), store, scope, time.Nanosecond); !errors.Is(err, store.err) {
		t.Fatalf("purge failure was hidden: %v", err)
	}
}
