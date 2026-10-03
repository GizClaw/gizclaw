//go:build gizclaw_provider_e2e

package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/adminapi"
	giztestcmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/giztest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

type doubaoTierReceipt struct {
	Model         string   `json:"model"`
	RequestedTier string   `json:"requested_tier"`
	ResponseTiers []string `json:"response_tiers"`
	HTTPStatus    int      `json:"http_status"`
	ErrorCode     string   `json:"error_code,omitempty"`
	FirstTextMS   int64    `json:"first_text_ms"`
	DurationMS    int64    `json:"duration_ms"`
	FinishReason  string   `json:"finish_reason"`
	OutputTokens  int      `json:"output_tokens"`
	ReadError     bool     `json:"read_error"`
}

// This opt-in Giztest makes real, billed Ark requests. The forwarding observer
// retains only tier/status/timing metadata, never headers, prompts, or replies.
func TestDoubaoServiceTierGiztest(t *testing.T) {
	apiKey := os.Getenv("GIZCLAW_E2E_VOLC_ARK_API_KEY")
	if apiKey == "" {
		t.Fatal("GIZCLAW_E2E_VOLC_ARK_API_KEY is required for this live Giztest")
	}
	modelName := os.Getenv("GIZCLAW_E2E_DOUBAO_FAST_MODEL")
	if modelName == "" {
		modelName = "doubao-seed-2-0-mini-260428"
	}
	reportDir := os.Getenv("GIZCLAW_E2E_SERVICE_TIER_REPORT_DIR")
	if reportDir == "" {
		reportDir = t.TempDir()
	}
	if err := os.MkdirAll(reportDir, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	var mu sync.Mutex
	var receipts []doubaoTierReceipt
	upstream := &http.Client{Timeout: 45 * time.Second}
	defer upstream.CloseIdleConnections()
	observer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v3/chat/completions" {
			http.NotFound(w, r)
			return
		}
		started := time.Now()
		receipt := doubaoTierReceipt{FirstTextMS: -1}
		defer func() {
			receipt.DurationMS = time.Since(started).Milliseconds()
			mu.Lock()
			receipts = append(receipts, receipt)
			mu.Unlock()
		}()
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var request struct {
			Model       string `json:"model"`
			ServiceTier string `json:"service_tier"`
		}
		if err != nil || json.Unmarshal(body, &request) != nil {
			http.Error(w, "invalid observer request", http.StatusBadRequest)
			return
		}
		receipt.Model, receipt.RequestedTier = request.Model, request.ServiceTier
		forward, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://ark.cn-beijing.volces.com/api/v3/chat/completions", bytes.NewReader(body))
		if err != nil {
			http.Error(w, "create upstream request", http.StatusBadGateway)
			return
		}
		forward.Header = r.Header.Clone()
		response, err := upstream.Do(forward)
		if err != nil {
			http.Error(w, "upstream transport failed", http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		receipt.HTTPStatus = response.StatusCode
		w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
		w.WriteHeader(response.StatusCode)
		if response.StatusCode != http.StatusOK {
			payload, err := io.ReadAll(io.LimitReader(response.Body, 16384))
			receipt.ReadError = err != nil
			var failure struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if json.Unmarshal(payload, &failure) == nil {
				receipt.ErrorCode = failure.Error.Code
				t.Logf("Ark rejection: HTTP %d, code=%s, message=%s", response.StatusCode, failure.Error.Code, strings.ReplaceAll(failure.Error.Message, apiKey, "[redacted]"))
			} else {
				t.Logf("Ark rejection: HTTP %d, content_type=%s", response.StatusCode, response.Header.Get("Content-Type"))
			}
			_, _ = w.Write(payload)
			return
		}
		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		for scanner.Scan() {
			line := scanner.Text()
			if payload, ok := strings.CutPrefix(line, "data: "); ok && payload != "[DONE]" {
				var chunk struct {
					ServiceTier string `json:"service_tier"`
					Choices     []struct {
						Delta struct {
							Content string `json:"content"`
						} `json:"delta"`
						FinishReason string `json:"finish_reason"`
					} `json:"choices"`
					Usage struct {
						CompletionTokens int `json:"completion_tokens"`
					} `json:"usage"`
				}
				if json.Unmarshal([]byte(payload), &chunk) == nil {
					if chunk.ServiceTier != "" && !slices.Contains(receipt.ResponseTiers, chunk.ServiceTier) {
						receipt.ResponseTiers = append(receipt.ResponseTiers, chunk.ServiceTier)
					}
					for _, choice := range chunk.Choices {
						if choice.Delta.Content != "" && receipt.FirstTextMS < 0 {
							receipt.FirstTextMS = time.Since(started).Milliseconds()
						}
						if choice.FinishReason != "" {
							receipt.FinishReason = choice.FinishReason
						}
					}
					if chunk.Usage.CompletionTokens != 0 {
						receipt.OutputTokens = chunk.Usage.CompletionTokens
					}
				}
			}
			if _, err := io.WriteString(w, line+"\n"); err != nil {
				receipt.ReadError = true
				return
			}
			w.(http.Flusher).Flush()
		}
		receipt.ReadError = scanner.Err() != nil
	}))
	defer observer.Close()

	cfg := validLayeredConfig(t.TempDir())
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	cfg.AdminPublicKey = key.Public
	srv, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(srv)
	defer httpServer.Close()
	srv.PublicEndpoint = strings.TrimPrefix(httpServer.URL, "http://")
	srv.PeerListenerFactories = []gizclaw.PeerListenerFactory{func(opts gizclaw.PeerListenerOptions) (giznet.Listener, error) {
		listener, err := (&gizwebrtc.ListenConfig{SecurityPolicy: opts.SecurityPolicy, PeerEventHandler: opts.PeerEventHandler}).Listen(opts.KeyPair)
		if err == nil {
			srv.WebRTCSignalingHandler = listener.SignalingHandler()
		}
		return listener, err
	}}
	if err := srv.Listen(); err != nil {
		_ = srv.Close()
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- srv.Serve() }()
	defer func() {
		if err := srv.Close(); err != nil {
			t.Error(err)
		}
		select {
		case err := <-served:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Server did not stop")
		}
	}()
	admin := &gizcli.Client{KeyPair: key, DialTransport: func(key *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
		return gizwebrtc.Dial(ctx, key, srv.PublicKey(), gizwebrtc.DialConfig{SignalingURL: httpServer.URL + gizwebrtc.SignalingPath, SecurityPolicy: policy})
	}}
	defer admin.Close()
	if err := admin.Dial(srv.PublicKey(), httpServer.URL); err != nil {
		t.Fatal(err)
	}
	go func() { _ = admin.Serve() }()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatal(err)
	}
	var credentialBody apitypes.CredentialBody
	if err := credentialBody.FromVolcCredentialBody(apitypes.VolcCredentialBody{ArkApiKey: &apiKey}); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateCredential(ctx, admin, adminhttp.CredentialUpsert{Id: "volc-fast", Provider: "volc", Body: credentialBody}); err != nil {
		t.Fatal(err)
	}
	tenant, err := api.CreateVolcTenantWithResponse(ctx, adminhttp.VolcTenantUpsert{Id: "volc-fast", CredentialId: "volc-fast", Endpoint: new(observer.URL + "/api/v3")})
	if err != nil {
		t.Fatal(err)
	}
	if tenant.JSON200 == nil {
		t.Fatalf("create Volc tenant: HTTP %d", tenant.StatusCode())
	}
	var data apitypes.ModelProviderData
	if err := data.FromVolcTenantModelProviderData(apitypes.VolcTenantModelProviderData{
		ApiMode: apitypes.VolcTenantModelProviderDataApiModeChatCompletions, UpstreamModel: &modelName,
		ServiceTier:     new(apitypes.VolcTenantModelProviderDataServiceTierFast),
		SupportTextOnly: new(true), UseSystemRole: new(true), ThinkingParam: new("thinking.type"), DefaultThinkingLevel: new("disabled"),
	}); err != nil {
		t.Fatal(err)
	}
	model, err := api.CreateModelWithResponse(ctx, adminhttp.ModelUpsert{Id: "doubao-fast", Kind: apitypes.ModelKindLlm, Source: apitypes.ModelSourceManual, Provider: apitypes.ModelProvider{Kind: apitypes.ModelProviderKindVolcTenant, Id: "volc-fast"}, ProviderData: data})
	if err != nil {
		t.Fatal(err)
	}
	if model.JSON200 == nil {
		t.Fatalf("create Model: HTTP %d", model.StatusCode())
	}
	var graph apitypes.FlowcraftWorkflowSpec
	if err := json.Unmarshal([]byte(`{"graph":{"name":"Doubao fast acceptance","entry":"answer","nodes":[{"id":"answer","type":"llm","publish":true,"config":{"model":"llm","max_tokens":32,"system_prompt":"按用户要求简短回答，不输出其他内容。"}}],"edges":[{"from":"answer","to":"__end__"}]},"conversation":{}}`), &graph); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateWorkflow(ctx, admin, apitypes.Workflow{Id: "doubao-fast", Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverFlowcraft, Flowcraft: &graph}}); err != nil {
		t.Fatal(err)
	}
	binding := apitypes.RuntimeProfileBinding{ResourceId: "doubao-fast", I18n: map[string]apitypes.RuntimeProfileI18nText{
		"en": {DisplayName: "Doubao Fast"}, "zh-CN": {DisplayName: "豆包低延迟"},
	}}
	profile := adminhttp.RuntimeProfileUpsert{Id: "doubao-fast", Spec: apitypes.RuntimeProfileSpec{
		Workflows: apitypes.RuntimeProfileWorkflows{"doubao-fast": {ResourceId: binding.ResourceId, I18n: binding.I18n}},
		Resources: apitypes.RuntimeProfileResources{Models: new(map[string]apitypes.RuntimeProfileBinding{"llm": binding})}, Quota: apitypes.RuntimeProfileQuota{Endpoint: "http://quota.example.test/v1/quota",

			ApiKey: "test-quota-key",
		},
	}}
	if _, err := adminapi.CreateRuntimeProfile(ctx, admin, profile); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateRegistrationToken(ctx, admin, adminhttp.RegistrationTokenUpsert{Id: "doubao-fast", Token: "doubao-fast-giztest", RuntimeProfileId: profile.Id}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIZCLAW_TEST_ENDPOINT", httpServer.URL)
	t.Setenv("GIZCLAW_TEST_REGISTRATION_TOKEN", "doubao-fast-giztest")
	command := giztestcmd.NewCmd()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"run", "--parallel", "1", "--output", filepath.Join(reportDir, "giztest.json"), filepath.Join("..", "..", "..", "tests", "gizclaw-e2e", "testdata", "doubao-service-tier", "fast.giztest.yaml")})
	runErr := command.ExecuteContext(ctx)
	// Closing the observer drains handlers before the receipts are inspected.
	observer.Close()
	mu.Lock()
	observed := slices.Clone(receipts)
	mu.Unlock()
	evidence, err := json.MarshalIndent(observed, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reportDir, "ark-tiers.json"), append(evidence, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Log(output.String())
	t.Logf("Ark tier receipts: %s", evidence)
	if runErr != nil {
		t.Fatalf("real Giztest failed: %v", runErr)
	}
	if len(observed) != 3 {
		t.Fatalf("Ark requests = %d, want 3", len(observed))
	}
	for i, receipt := range observed {
		if receipt.Model != modelName || receipt.RequestedTier != "fast" || receipt.HTTPStatus != http.StatusOK || receipt.ReadError || receipt.FirstTextMS < 0 || receipt.FinishReason != "stop" {
			t.Errorf("turn %d: incomplete fast request: %+v", i+1, receipt)
		}
		if !slices.Contains(receipt.ResponseTiers, "fast") || slices.Contains(receipt.ResponseTiers, "default") {
			t.Errorf("turn %d: upstream did not confirm fast execution: %v", i+1, receipt.ResponseTiers)
		}
	}
}
