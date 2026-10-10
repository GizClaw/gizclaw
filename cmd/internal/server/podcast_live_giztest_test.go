//go:build gizclaw_provider_e2e

package server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
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
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/einoconfig"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	stores "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"sigs.k8s.io/yaml"
)

type podcastLiveReceipt struct {
	StartedAt time.Time `json:"started_at"`
	Model     string    `json:"model"`
	Text      string    `json:"text"`
	Status    int       `json:"status"`
	Completed bool      `json:"completed"`
	Cancelled bool      `json:"cancelled"`
	LastRole  string    `json:"last_role"`
	Users     int       `json:"users"`
	User      string    `json:"user"`
	ReadError bool      `json:"read_error"`
}

// TestContinuousPodcastLiveGiztest makes billed Ark and Volc TTS requests using
// the supplied original podcast Graph. It never substitutes provider responses.
func TestContinuousPodcastLiveGiztest(t *testing.T) {
	graphPath := os.Getenv("GIZCLAW_TEST_PODCAST_LIVE_WORKFLOW")
	evidence := os.Getenv("GIZCLAW_TEST_PODCAST_LIVE_EVIDENCE")
	if graphPath == "" || evidence == "" {
		t.Fatal("explicit original Workflow and evidence paths are required")
	}
	for _, name := range []string{"GIZCLAW_E2E_VOLC_ARK_API_KEY", "GIZCLAW_E2E_DOUBAO_APP_ID", "GIZCLAW_E2E_DOUBAO_API_KEY"} {
		if os.Getenv(name) == "" {
			t.Fatalf("%s is required", name)
		}
	}
	if err := os.MkdirAll(evidence, 0700); err != nil {
		t.Fatal(err)
	}
	writeJSON := func(name string, value any) {
		t.Helper()
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(evidence, name), append(data, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var mu sync.Mutex
	var receipts []podcastLiveReceipt
	observer := newPodcastLiveObserver(t, func(receipt podcastLiveReceipt) {
		mu.Lock()
		receipts = append(receipts, receipt)
		mu.Unlock()
	})
	t.Cleanup(func() {
		observer.Close()
		mu.Lock()
		copyReceipts := slices.Clone(receipts)
		mu.Unlock()
		writeJSON("model-requests.json", copyReceipts)
	})
	withPeerControlServerTimeout(t, 8, 10*time.Minute, func(ctx context.Context, srv *gizclaw.Server, admin *gizcli.Client, _, edgeURL string) {
		api, err := admin.ServerAdminClient()
		if err != nil {
			t.Fatal(err)
		}
		graph := setupPodcastLiveGiztest(t, ctx, admin, observer.URL, graphPath)
		document := filepath.Join("..", "..", "..", "tests", "gizclaw-e2e", "testdata", "continuous-podcast", "live.giztest.yaml")
		hashes := map[string]string{}
		for _, path := range []string{graphPath, document} {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			hashes[filepath.Base(path)] = fmt.Sprintf("%x", sha256.Sum256(data))
		}
		writeJSON("source-hashes.json", hashes)
		writeJSON("workflow.json", graph)
		workspaceName := fmt.Sprintf("podcast-live-%d", time.Now().UnixNano())
		t.Setenv("GIZCLAW_TEST_ENDPOINT", edgeURL)
		t.Setenv("GIZCLAW_TEST_REGISTRATION_TOKEN", "local-peer-sync-test-token")
		t.Setenv("GIZCLAW_TEST_PODCAST_LIVE_WORKSPACE", workspaceName)
		command := giztestcmd.NewCmd()
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetErr(&output)
		reportPath := filepath.Join(evidence, "podcast-live.giztest.json")
		command.SetArgs([]string{"run", "--parallel", "1", "--output", reportPath, document})
		runErr := command.ExecuteContext(ctx)
		if err := os.WriteFile(filepath.Join(evidence, "runner.log"), output.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		if err := srv.Manager().PeerUsage.Flush(ctx); err != nil {
			t.Fatal(err)
		}
		var usage []struct {
			Model    string `db:"model_id" json:"model_id"`
			Quantity int64  `db:"quantity" json:"quantity"`
		}
		if err := srv.PeerUsageDB.SelectContext(ctx, &usage, "SELECT model_id,SUM(quantity) AS quantity FROM peer_model_usage_hourly GROUP BY model_id"); err != nil {
			t.Fatal(err)
		}
		writeJSON("provider-usage.json", usage)
		workspaces, err := api.ListWorkspacesWithResponse(ctx, &adminhttp.ListWorkspacesParams{Limit: new(int32(200))})
		if err != nil || workspaces == nil || workspaces.JSON200 == nil {
			t.Fatal("list live Workspaces", err)
		}
		var selected *apitypes.Workspace
		for _, workspace := range workspaces.JSON200.Items {
			if workspace.Name == workspaceName {
				selected = &workspace
				break
			}
		}
		if selected == nil {
			t.Fatalf("live Workspace was not created; Giztest: %v\n%s", runErr, output.String())
		}
		defer func() {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
			defer cancel()
			deleted, err := api.DeleteWorkspaceWithResponse(cleanupCtx, selected.Id)
			workspaceDeleted := err == nil && deleted != nil && deleted.StatusCode() == http.StatusOK
			peerDeleted := false
			if selected.OwnerPublicKey != nil {
				deleted, err := api.DeletePeerWithResponse(cleanupCtx, *selected.OwnerPublicKey)
				peerDeleted = err == nil && deleted != nil && deleted.StatusCode() == http.StatusOK
			}
			writeJSON("cleanup.json", map[string]bool{"workspace_deleted": workspaceDeleted, "peer_deleted": peerDeleted})
			if !workspaceDeleted || !peerDeleted {
				t.Error("live Workspace/Peer cleanup failed")
			}
		}()
		history, err := api.ListWorkspaceHistoryWithResponse(ctx, selected.Id, &adminhttp.ListWorkspaceHistoryParams{Limit: new(100), Order: new(adminhttp.ListWorkspaceHistoryParamsOrder("asc"))})
		if err != nil || history == nil || history.JSON200 == nil {
			t.Fatal("read live History", err)
		}
		writeJSON("workspace-history.json", history.JSON200)
		mu.Lock()
		copyReceipts := slices.Clone(receipts)
		mu.Unlock()
		slices.SortFunc(copyReceipts, func(a, b podcastLiveReceipt) int { return a.StartedAt.Compare(b.StartedAt) })
		used := map[int]bool{}
		matched, complete, audioFiles := 0, 0, 0
		for _, entry := range history.JSON200.Items {
			if entry.Type != rpcapi.PeerRunHistoryEntryTypeAgent {
				continue
			}
			text := strings.TrimSpace(entry.Text)
			match := -1
			for i, receipt := range copyReceipts {
				if !used[i] && receipt.Status == http.StatusOK && text != "" && strings.HasPrefix(strings.TrimSpace(receipt.Text), text) {
					match = i
					break
				}
			}
			if match < 0 {
				t.Errorf("History %s does not match exactly one unused real model generation", entry.Id)
				continue
			}
			used[match] = true
			matched++
			if copyReceipts[match].Completed && text == strings.TrimSpace(copyReceipts[match].Text) {
				complete++
			}
			if audioFiles < 2 && entry.ReplayAvailable && len([]rune(text)) >= 80 {
				response, err := api.DownloadWorkspaceHistoryAudio(ctx, selected.Id, entry.Id)
				if err != nil {
					t.Fatal("download live History audio", err)
				}
				data, readErr := io.ReadAll(io.LimitReader(response.Body, 8<<20))
				_ = response.Body.Close()
				if readErr != nil || response.StatusCode != http.StatusOK || !bytes.HasPrefix(data, []byte("OggS")) || len(data) < 1000 {
					t.Fatal("invalid live History audio")
				}
				audioFiles++
				if err := os.WriteFile(filepath.Join(evidence, fmt.Sprintf("history-%02d.ogg", audioFiles)), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
		writeJSON("history-matching.json", map[string]int{"agent_entries": matched, "complete_generations": complete, "audio_files": audioFiles, "real_model_requests": len(copyReceipts)})
		if runErr != nil {
			t.Fatalf("live podcast Giztest failed: %v\n%s", runErr, output.String())
		}
		data, err := os.ReadFile(reportPath)
		if err != nil {
			t.Fatal(err)
		}
		var report giztest.Report
		if err := json.Unmarshal(data, &report); err != nil {
			t.Fatal(err)
		}
		if report.Status != "passed" || len(report.Tasks) != 1 || len(report.Tasks[0].Steps) != 20 || len(report.Tasks[0].Cleanup) != 1 {
			t.Fatal("live Giztest did not execute every step")
		}
		children := 0
		for _, step := range append(report.Tasks[0].Steps, report.Tasks[0].Cleanup...) {
			if step.Status != "passed" {
				t.Fatalf("live Giztest step %s: %s", step.ID, step.Status)
			}
			for _, child := range step.Children {
				children++
				if child.Status != "passed" {
					t.Fatalf("live Giztest child %s: %s", child.ID, child.Status)
				}
			}
		}
		if children != 8 || matched < 6 || complete < 4 || audioFiles != 2 {
			t.Fatalf("incomplete live evidence: children=%d matched=%d complete=%d audio=%d", children, matched, complete, audioFiles)
		}
		modelUsage, speechUsage := int64(0), int64(0)
		for _, row := range usage {
			if row.Model == "doubao-seed-2-1-lite-260915" {
				modelUsage += row.Quantity
			} else if row.Model == "seed-tts-2.0" {
				speechUsage += row.Quantity
			}
		}
		if modelUsage <= 0 || speechUsage <= 0 {
			t.Fatalf("real model/TTS usage is missing: %d/%d", modelUsage, speechUsage)
		}
		for _, receipt := range copyReceipts {
			if receipt.Status == http.StatusOK && receipt.LastRole != "user" {
				t.Error("podcast request does not end in the current real user request")
			}
		}
		t.Logf("real providers: 20 steps, 8 parallel children; %d separate History entries match real generations; model/TTS usage=%d/%d", matched, modelUsage, speechUsage)
	}, func(cfg *Config) {
		cfg.Stores["usage"] = stores.Config{Kind: stores.KindSQL, Storage: "business-db"}
		cfg.Services.PeerUsage = &SingleStoreConfig{Store: "usage"}
	})
}

func setupPodcastLiveGiztest(t *testing.T, ctx context.Context, admin *gizcli.Client, endpoint, graphPath string) apitypes.Workflow {
	t.Helper()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatal(err)
	}
	var credential apitypes.CredentialBody
	if err := credential.FromVolcCredentialBody(apitypes.VolcCredentialBody{ArkApiKey: new(os.Getenv("GIZCLAW_E2E_VOLC_ARK_API_KEY")), SpeechAppId: new(os.Getenv("GIZCLAW_E2E_DOUBAO_APP_ID")), SpeechApiKey: new(os.Getenv("GIZCLAW_E2E_DOUBAO_API_KEY"))}); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateCredential(ctx, admin, adminhttp.CredentialUpsert{Id: "podcast-live", Provider: "volc", Body: credential}); err != nil {
		t.Fatal(err)
	}
	tenant, err := api.CreateVolcTenantWithResponse(ctx, adminhttp.VolcTenantUpsert{Id: "podcast-live", CredentialId: "podcast-live", Endpoint: new(endpoint + "/api/v3"), Region: new("cn-beijing"), ResourceIds: new([]string{"seed-tts-2.0"})})
	if err != nil || tenant == nil || tenant.JSON200 == nil {
		t.Fatal("create live tenant", err)
	}
	var modelData apitypes.ModelProviderData
	if err := modelData.FromVolcTenantModelProviderData(apitypes.VolcTenantModelProviderData{ApiMode: "chat_completions", UpstreamModel: new("doubao-seed-2-1-lite-260915"), ServiceTier: new(apitypes.VolcTenantModelProviderDataServiceTier("fast")), SupportTextOnly: new(true), UseSystemRole: new(true), SupportThinking: new(true), ThinkingParam: new("thinking.type"), ThinkingLevels: new([]string{"enabled", "disabled"}), DefaultThinkingLevel: new("disabled")}); err != nil {
		t.Fatal(err)
	}
	model, err := api.CreateModelWithResponse(ctx, adminhttp.ModelUpsert{Id: "podcast-live-model", Kind: apitypes.ModelKindLlm, Source: apitypes.ModelSourceManual, Provider: apitypes.ModelProvider{Kind: "volc-tenant", Id: "podcast-live"}, ProviderData: modelData})
	if err != nil || model == nil || model.JSON200 == nil {
		t.Fatal("create live model", err)
	}
	var voiceData apitypes.VoiceProviderData
	if err := voiceData.FromVolcTenantVoiceProviderData(apitypes.VolcTenantVoiceProviderData{ResourceId: new("seed-tts-2.0"), VoiceId: new("zh_female_qingxinnvsheng_uranus_bigtts")}); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateVoice(ctx, admin, adminhttp.VoiceUpsert{Id: "podcast-live-voice", Source: apitypes.VoiceSourceManual, Provider: apitypes.VoiceProvider{Kind: "volc-tenant", Id: "podcast-live"}, ProviderData: &voiceData}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(graphPath)
	if err != nil {
		t.Fatal(err)
	}
	var resource apitypes.Resource
	if err := yaml.Unmarshal(raw, &resource); err != nil {
		t.Fatal(err)
	}
	workflow, err := resource.AsWorkflowResource()
	if err != nil || workflow.Spec.Eino == nil || workflow.Spec.Eino.Conversation == nil || workflow.Spec.Eino.Conversation.ContinueFrom == nil || workflow.Spec.Eino.VoiceAdapter == nil || workflow.Spec.Eino.VoiceAdapter.DefaultVoice == nil {
		t.Fatal("original Workflow must enable continuous narration and a default Voice", err)
	}
	original := apitypes.Workflow{Id: workflow.Metadata.Id, Spec: workflow.Spec}
	if _, err := adminapi.CreateWorkflow(ctx, admin, original); err != nil {
		t.Fatal(err)
	}
	binding := func(id string) apitypes.RuntimeProfileBinding {
		return apitypes.RuntimeProfileBinding{ResourceId: id, I18n: map[string]apitypes.RuntimeProfileI18nText{"en": {DisplayName: "Live podcast"}, "zh-CN": {DisplayName: "真实播客测试"}}}
	}
	models := map[string]apitypes.RuntimeProfileBinding{}
	if err := einoconfig.VisitModelAliases("", workflow.Spec.Eino.Graph, func(_ string, alias string, kind apitypes.ModelKind) error {
		if kind != apitypes.ModelKindLlm {
			return fmt.Errorf("unexpected original Graph model kind %s", kind)
		}
		models[alias] = binding("podcast-live-model")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	profile := adminhttp.RuntimeProfileUpsert{Id: "peer-sync-giztest", Spec: apitypes.RuntimeProfileSpec{
		Workflows: apitypes.RuntimeProfileWorkflows{"podcast": {ResourceId: original.Id, I18n: binding(original.Id).I18n}},
		Resources: apitypes.RuntimeProfileResources{Models: &models, Voices: new(map[string]apitypes.RuntimeProfileBinding{*workflow.Spec.Eino.VoiceAdapter.DefaultVoice: binding("podcast-live-voice")})},
	}}
	if _, err := adminapi.PutRuntimeProfile(ctx, admin, profile.Id, profile); err != nil {
		t.Fatal(err)
	}
	return original
}

func newPodcastLiveObserver(t *testing.T, record func(podcastLiveReceipt)) *httptest.Server {
	t.Helper()
	client := &http.Client{Timeout: 45 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receipt := podcastLiveReceipt{StartedAt: time.Now()}
		var text strings.Builder
		defer func() {
			receipt.Text = text.String()
			receipt.Cancelled = r.Context().Err() != nil
			record(receipt)
		}()
		body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
		if err != nil || r.URL.Path != "/api/v3/chat/completions" {
			http.Error(w, "invalid observer request", http.StatusBadRequest)
			return
		}
		var request struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			http.Error(w, "invalid model request", http.StatusBadRequest)
			return
		}
		receipt.Model = request.Model
		for _, message := range request.Messages {
			receipt.LastRole = message.Role
			if message.Role == "user" {
				receipt.Users++
				receipt.User = message.Content
			}
		}
		forward, err := http.NewRequestWithContext(r.Context(), r.Method, "https://ark.cn-beijing.volces.com"+r.URL.RequestURI(), bytes.NewReader(body))
		if err != nil {
			http.Error(w, "create upstream request", http.StatusBadGateway)
			return
		}
		forward.Header = r.Header.Clone()
		response, err := client.Do(forward)
		if err != nil {
			http.Error(w, "upstream transport failed", http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		receipt.Status = response.StatusCode
		w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
		w.WriteHeader(response.StatusCode)
		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		for scanner.Scan() {
			line := scanner.Text()
			if payload, ok := strings.CutPrefix(line, "data: "); ok && payload != "[DONE]" {
				var chunk struct {
					Choices []struct {
						Delta struct {
							Content string `json:"content"`
						} `json:"delta"`
						FinishReason string `json:"finish_reason"`
					} `json:"choices"`
				}
				if json.Unmarshal([]byte(payload), &chunk) == nil {
					for _, choice := range chunk.Choices {
						text.WriteString(choice.Delta.Content)
						receipt.Completed = receipt.Completed || choice.FinishReason == "stop"
					}
				}
			}
			if _, err := io.WriteString(w, line+"\n"); err != nil {
				receipt.ReadError = true
				return
			}
			w.(http.Flusher).Flush()
		}
		if scanner.Err() != nil {
			receipt.ReadError = true
			panic(http.ErrAbortHandler)
		}
	}))
}
