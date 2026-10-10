//go:build gizclaw_provider_e2e

package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/einoconfig"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	stores "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/adminresource"
	"sigs.k8s.io/yaml"
)

// This opt-in test reads the original Deploy/Raids files without rewriting
// their Graphs or prompts, and makes real billed Lite/Realtime/ASR/TTS/Memory requests.
func TestRuntimeProfileAudioGiztests(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Minute)
	defer cancel()
	cfg := validLayeredConfig(t.TempDir())
	cfg.Stores["usage"] = stores.Config{Kind: stores.KindSQL, Storage: "business-db"}
	cfg.Services.PeerUsage = &SingleStoreConfig{Store: "usage"}
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

	root := os.Getenv("GIZCLAW_TEST_AUDIO_DEPLOY_ROOT")
	evidence := os.Getenv("GIZCLAW_TEST_AUDIO_EVIDENCE")
	memoryEndpoint := os.Getenv("GIZCLAW_TEST_MEM0_ENDPOINT")
	if root == "" || evidence == "" || memoryEndpoint == "" {
		t.Fatal("explicit Deploy root, evidence and local Mem0 endpoint required")
	}
	if err := os.MkdirAll(evidence, 0700); err != nil {
		t.Fatal(err)
	}
	load := func(path string) apitypes.Resource {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		data, err := yaml.YAMLToJSON(raw)
		if err != nil {
			t.Fatal(err)
		}
		var resource apitypes.Resource
		if err := json.Unmarshal(data, &resource); err != nil {
			t.Fatal(err)
		}
		return resource
	}
	apply := func(resource apitypes.Resource) {
		t.Helper()
		if _, err := adminresource.NewClient(api, nil).ApplyResource(ctx, resource); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON := func(name string, value any) {
		t.Helper()
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Error(err)
			return
		}
		if err := os.WriteFile(filepath.Join(evidence, name), append(data, '\n'), 0600); err != nil {
			t.Error(err)
		}
	}
	var receiptsMu sync.Mutex
	var receipts []legacyAudioReceipt
	ark := legacyAudioForwarder(t, "https://ark.cn-beijing.volces.com", func(r legacyAudioReceipt) { receiptsMu.Lock(); receipts = append(receipts, r); receiptsMu.Unlock() })
	defer ark.Close()
	var memoryMu sync.Mutex
	var memoryReceipts []legacyAudioReceipt
	memoryProxy := legacyAudioForwarder(t, memoryEndpoint, func(r legacyAudioReceipt) {
		memoryMu.Lock()
		memoryReceipts = append(memoryReceipts, r)
		memoryMu.Unlock()
	})
	defer memoryProxy.Close()
	defer func() {
		ark.Close()
		memoryProxy.Close()
		writeJSON("ark-requests.json", receipts)
		writeJSON("memory-requests.json", memoryReceipts)
	}()
	var credential apitypes.CredentialBody
	searchKey := os.Getenv("GIZCLAW_E2E_DOUBAO_SEARCH_API_KEY")
	appID, speechKey, arkKey := os.Getenv("GIZCLAW_E2E_DOUBAO_APP_ID"), os.Getenv("GIZCLAW_E2E_DOUBAO_API_KEY"), os.Getenv("GIZCLAW_E2E_VOLC_ARK_API_KEY")
	if appID == "" || speechKey == "" || arkKey == "" || searchKey == "" {
		t.Fatal("real speech, search and Ark credentials required")
	}
	if err := credential.FromVolcCredentialBody(apitypes.VolcCredentialBody{SpeechAppId: &appID, SpeechApiKey: &speechKey, ArkApiKey: &arkKey, SearchApiKey: &searchKey}); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateCredential(ctx, admin, adminhttp.CredentialUpsert{Id: "volc-live", Provider: "volc", Body: credential}); err != nil {
		t.Fatal(err)
	}
	tenant, err := api.CreateVolcTenantWithResponse(ctx, adminhttp.VolcTenantUpsert{Id: "volc-cn-beijing", CredentialId: "volc-live", Endpoint: new(ark.URL + "/api/v3"), Region: new("cn-beijing"), ResourceIds: new([]string{"seed-tts-1.0", "seed-tts-2.0", "seed-icl-1.0", "seed-icl-2.0", "volc.bigasr.sauc.duration"})})
	if err != nil || tenant == nil || tenant.JSON200 == nil {
		t.Fatal("create real speech/Ark tenant", err)
	}
	apply(load("gizclaw/catalogs/raids/models/doubao-seed-2-1-lite-audio.yaml"))
	apply(load("gizclaw/catalogs/raids/models/volc-bigasr-sauc.yaml"))
	apply(load("gizclaw/catalogs/overrides/models/doubao-realtime-dialog-sc.yaml"))
	resource := load("gizclaw/products/h106-tiga/runtime-profiles/h106-tiga.yaml")
	original, err := resource.AsRuntimeProfileResource()
	if err != nil {
		t.Fatal(err)
	}
	profile := adminhttp.RuntimeProfileUpsert{Id: original.Metadata.Id, Spec: original.Spec}
	profile.Spec.Workflows = apitypes.RuntimeProfileWorkflows{}
	models, voices, memories := map[string]apitypes.RuntimeProfileBinding{}, map[string]apitypes.RuntimeProfileBinding{}, map[string]apitypes.RuntimeProfileMemoryBinding{}
	profile.Spec.Resources = apitypes.RuntimeProfileResources{Models: &models, Voices: &voices, Memories: &memories}
	cases := []struct{ name, alias, workflow, document string }{
		{"adventure", "adventure.castle-mystery-multi-role", "gizclaw/catalogs/raids/workflows/adventure-castle-mystery/eino.multi-role.yaml", "tests/giztest/deployment/h106-tiga/raids/adventure.castle-mystery-multi-role.giztest.yaml"},
		{"guess", "guess.animals", "gizclaw/catalogs/raids/workflows/guess-animals/eino.yaml", "tests/giztest/deployment/h106-tiga/learn/guess.animals.giztest.yaml"},
		{"topic", "topic-utiga-03006", "gizclaw/catalogs/overrides/workflows/utiga-03006-tactical-manual/eino.yaml", "tests/giztest/deployment/h106-tiga/assistants/topic-utiga-03006.giztest.yaml"},
		{"realtime-native", "chat", "gizclaw/catalogs/overrides/workflows/h106-tiga-chat/conversation.yaml", "tests/giztest/deployment/h106-tiga/assistants/chat.giztest.yaml"},
	}
	models["asr"] = (*original.Spec.Resources.Models)["asr"]
	hashes := map[string]string{}
	token := ""
	for _, tc := range cases {
		raw, err := os.ReadFile(filepath.Join(root, tc.workflow))
		if err != nil {
			t.Fatal(err)
		}
		hashes[tc.workflow] = fmt.Sprintf("%x", sha256.Sum256(raw))
		workflowResource := load(tc.workflow)
		workflow, err := workflowResource.AsWorkflowResource()
		if err != nil {
			t.Fatal(err)
		}
		apply(workflowResource)
		binding, ok := original.Spec.Workflows[tc.alias]
		if !ok {
			t.Fatalf("Profile does not bind %s", tc.alias)
		}
		binding.PttAsrModel = nil
		binding.RealtimeAsrModel = new("asr")
		profile.Spec.Workflows[tc.alias] = binding
		if workflow.Spec.Eino != nil {
			if err := einoconfig.VisitModelAliases("", workflow.Spec.Eino.Graph, func(_ string, alias string, _ apitypes.ModelKind) error {
				binding := (*original.Spec.Resources.Models)[alias]
				binding.ResourceId = "doubao-seed-2-1-lite-audio"
				models[alias] = binding
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			adapter := workflow.Spec.Eino.VoiceAdapter
			voices[*adapter.DefaultVoice] = (*original.Spec.Resources.Voices)[*adapter.DefaultVoice]
			if adapter.SpeakerVoices != nil {
				for _, alias := range *adapter.SpeakerVoices {
					voices[alias] = (*original.Spec.Resources.Voices)[alias]
				}
			}
			if adapter.NodeVoices != nil {
				for _, alias := range *adapter.NodeVoices {
					voices[alias] = (*original.Spec.Resources.Voices)[alias]
				}
			}
		} else if realtime := workflow.Spec.DoubaoRealtime; realtime != nil {
			models[realtime.Model] = (*original.Spec.Resources.Models)[realtime.Model]
			voices[*realtime.Audio.Output.Voice] = (*original.Spec.Resources.Voices)[*realtime.Audio.Output.Voice]
			voices[realtime.Tts.Voice] = (*original.Spec.Resources.Voices)[realtime.Tts.Voice]
		} else {
			t.Fatalf("unsupported original Workflow driver: %s", workflow.Spec.Driver)
		}
		if workflow.Spec.Memory != nil {
			alias := *workflow.Spec.Memory
			binding := (*original.Spec.Resources.Memories)[alias]
			apply(load("gizclaw/catalogs/raids/memory-layouts/" + binding.LayoutId + ".yaml"))
			binding.Driver = apitypes.RuntimeProfileMemoryDriverMem0
			if err := binding.Connection.FromRuntimeProfileMem0SelfHostedConnection(apitypes.RuntimeProfileMem0SelfHostedConnection{Type: apitypes.RuntimeProfileMem0SelfHostedConnectionTypeMem0SelfHosted, Endpoint: memoryProxy.URL, ApiKey: new(os.Getenv("GIZCLAW_E2E_MEM0_API_KEY"))}); err != nil {
				t.Fatal(err)
			}
			memories[alias] = binding
		}
		raw, err = os.ReadFile(filepath.Join(root, tc.document))
		if err != nil {
			t.Fatal(err)
		}
		hashes[tc.document] = fmt.Sprintf("%x", sha256.Sum256(raw))
		var document struct {
			Variables map[string]struct {
				Value string `json:"value"`
			} `json:"variables"`
		}
		data, err := yaml.YAMLToJSON(raw)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &document); err != nil {
			t.Fatal(err)
		}
		nextToken := document.Variables["registration_token"].Value
		if token != "" && token != nextToken {
			t.Fatal("documents use different registration tokens")
		}
		token = nextToken
	}
	for _, region := range []string{"cn", "global"} {
		prefix := "GIZCLAW_E2E_MINIMAX_" + strings.ToUpper(region)
		key := os.Getenv(prefix + "_API_KEY")
		group := os.Getenv(prefix + "_GROUP_ID")
		if key == "" || group == "" {
			t.Fatal("real MiniMax credentials required for original Voice bindings")
		}
		var body apitypes.CredentialBody
		if err := body.FromMiniMaxCredentialBody(apitypes.MiniMaxCredentialBody{ApiKey: &key}); err != nil {
			t.Fatal(err)
		}
		id := "minimax-" + region
		if _, err := adminapi.CreateCredential(ctx, admin, adminhttp.CredentialUpsert{Id: id, Provider: "minimax", Body: body}); err != nil {
			t.Fatal(err)
		}
		baseURL := "https://api.minimax.io"
		if region == "cn" {
			baseURL = "https://api.minimaxi.com"
		}
		tenant, err := api.CreateMiniMaxTenantWithResponse(ctx, adminhttp.MiniMaxTenantUpsert{Id: id, CredentialId: id, GroupId: &group, BaseUrl: &baseURL})
		if err != nil || tenant == nil || tenant.JSON200 == nil {
			t.Fatal("create real MiniMax tenant", err)
		}
	}
	seen := map[string]bool{}
	for _, binding := range voices {
		if seen[binding.ResourceId] {
			continue
		}
		seen[binding.ResourceId] = true
		parts := strings.Split(binding.ResourceId, ":")
		if len(parts) != 3 {
			t.Fatalf("unexpected original Voice identity: %s", binding.ResourceId)
		}
		filename := parts[2] + ".yaml"
		voicePath := "gizclaw/catalogs/overrides/voices/" + filename
		if _, err := os.Stat(filepath.Join(root, voicePath)); os.IsNotExist(err) {
			voicePath = "gizclaw/catalogs/raids/voices/" + parts[1] + "/" + url.PathEscape(parts[2]) + ".yaml"
		}
		apply(load(voicePath))
	}
	if _, err := adminapi.CreateRuntimeProfile(ctx, admin, profile); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateRegistrationToken(ctx, admin, adminhttp.RegistrationTokenUpsert{Id: "local-live", Token: token, RuntimeProfileId: profile.Id}); err != nil {
		t.Fatal(err)
	}
	writeJSON("source-hashes.json", hashes)
	t.Setenv("GIZCLAW_TEST_ENDPOINT", httpServer.URL)
	t.Setenv("GIZCLAW_TEST_MEMORY_BACKEND", "mem0")
	runDocument := func(name, path string) bool {
		t.Helper()
		executed := false
		t.Run(name, func(t *testing.T) {
			executed = true
			command := giztestcmd.NewCmd()
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetErr(&output)
			command.SetArgs([]string{"run", "--parallel", "1", "--output", filepath.Join(evidence, name+".json"), path})
			err := command.ExecuteContext(ctx)
			if writeErr := os.WriteFile(filepath.Join(evidence, name+".log"), output.Bytes(), 0600); writeErr != nil {
				t.Fatal(writeErr)
			}
			t.Log(output.String())
			if err != nil {
				t.Error(err)
			}
		})
		return executed
	}
	nativeRuns := 0
	for _, tc := range cases {
		if runDocument(tc.name, filepath.Join(root, tc.document)) && tc.name != "realtime-native" {
			nativeRuns++
		}
	}
	if err := srv.Manager().PeerUsage.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	var usage []struct {
		Model    string `db:"model_id" json:"model"`
		Quantity int64  `db:"quantity" json:"quantity"`
	}
	if err := srv.PeerUsageDB.SelectContext(ctx, &usage, "SELECT model_id,SUM(quantity) AS quantity FROM peer_model_usage_hourly GROUP BY model_id"); err != nil {
		t.Fatal(err)
	}
	writeJSON("provider-usage.json", usage)
	liteUsage := int64(0)
	for _, row := range usage {
		if strings.Contains(row.Model, "lite") {
			liteUsage += row.Quantity
		}
		if strings.Contains(row.Model, "bigasr") || strings.Contains(row.Model, "sauc") {
			t.Errorf("independent ASR was billed: %s", row.Model)
		}
	}
	if nativeRuns > 0 && liteUsage <= 0 {
		t.Error("no real Lite usage was recorded")
	}
	receiptsMu.Lock()
	observed := slices.Clone(receipts)
	receiptsMu.Unlock()
	transcripts, replies := 0, 0
	for _, r := range observed {
		if r.Status != http.StatusOK {
			t.Errorf("Ark request failed: status=%d", r.Status)
		}
		if r.Transcript && r.Audio {
			transcripts++
		} else if !r.Audio {
			replies++
		}
	}
	if transcripts < 2*nativeRuns || replies < 2*nativeRuns {
		t.Errorf("expected native transcription and text reply for %d audio turns: transcripts=%d replies=%d", 2*nativeRuns, transcripts, replies)
	}
	// Select external ASR for the same unmodified realtime Workflow, which
	// supports text input. Every lane registers a new Peer with this revision.
	binding := profile.Spec.Workflows["chat"]
	binding.PttAsrModel = new("asr")
	profile.Spec.Workflows["chat"] = binding
	if _, err := adminapi.PutRuntimeProfile(ctx, admin, profile.Id, profile); err != nil {
		t.Fatal(err)
	}
	chatDocument := filepath.Join(root, cases[len(cases)-1].document)
	externalRuns := 0
	if runDocument("realtime-external-ptt", chatDocument) {
		externalRuns++
	}
	raw, err := os.ReadFile(chatDocument)
	if err != nil {
		t.Fatal(err)
	}
	// Only the test input mode changes; the original Workflow and prompts do not.
	realtimeDocument := bytes.ReplaceAll(raw, []byte("WORKSPACE_INPUT_MODE_PUSH_TO_TALK"), []byte("WORKSPACE_INPUT_MODE_REALTIME"))
	realtimeDocument = bytes.ReplaceAll(realtimeDocument, []byte("mode: push-to-talk"), []byte("mode: realtime"))
	path := filepath.Join(evidence, "realtime-external.giztest.yaml")
	if err := os.WriteFile(path, realtimeDocument, 0600); err != nil {
		t.Fatal(err)
	}
	if runDocument("realtime-external-continuous", path) {
		externalRuns++
	}
	data, err := yaml.YAMLToJSON(realtimeDocument)
	if err != nil {
		t.Fatal(err)
	}
	var interruptedDocument map[string]any
	if err := json.Unmarshal(data, &interruptedDocument); err != nil {
		t.Fatal(err)
	}
	interruptedDocument["name"] = "realtime-external-interrupt-recovery"
	steps, ok := interruptedDocument["steps"].([]any)
	if !ok {
		t.Fatal("original chat document has no steps")
	}
	changed := false
	for _, rawStep := range steps {
		step, ok := rawStep.(map[string]any)
		if !ok || step["id"] != "conversation_turn_1" {
			continue
		}
		stream, ok := step["peer_stream"].(map[string]any)
		if !ok {
			t.Fatal("original first chat turn has no peer_stream")
		}
		stream["interrupt_after"] = "250ms"
		step["expect"] = map[string]any{
			"/events":      map[string]any{"non_empty": true},
			"/interrupted": map[string]any{"equals": true},
			"/audio_eos":   map[string]any{"equals": true},
		}
		changed = true
	}
	if !changed {
		t.Fatal("original first chat turn was not found")
	}
	data, err = json.Marshal(interruptedDocument)
	if err != nil {
		t.Fatal(err)
	}
	data = append([]byte("# User Story:\n# As a device using Profile-selected ASR,\n# I want to interrupt a realtime reply and complete the next audio turn,\n# So that cancellation preserves the conversation and cleanup.\n"), data...)
	path = filepath.Join(evidence, "realtime-interrupt.giztest.yaml")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if runDocument("realtime-external-interrupt-recovery", path) {
		externalRuns++
	}
	if err := srv.Manager().PeerUsage.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	usage = nil
	if err := srv.PeerUsageDB.SelectContext(ctx, &usage, "SELECT model_id,SUM(quantity) AS quantity FROM peer_model_usage_hourly GROUP BY model_id"); err != nil {
		t.Fatal(err)
	}
	writeJSON("external-provider-usage.json", usage)
	independentASR := int64(0)
	for _, row := range usage {
		if strings.Contains(row.Model, "bigasr") || strings.Contains(row.Model, "sauc") {
			independentASR += row.Quantity
		}
	}
	if externalRuns > 0 && independentASR <= 0 {
		t.Error("external ASR lanes recorded no independent ASR usage")
	}
}

type legacyAudioReceipt struct {
	Path       string `json:"path"`
	Model      string `json:"model,omitempty"`
	Transcript bool   `json:"transcript_request,omitempty"`
	Audio      bool   `json:"contains_audio,omitempty"`
	User       string `json:"user_text,omitempty"`
	Query      string `json:"memory_query,omitempty"`
	Status     int    `json:"status"`
}

type legacyFlushWriter struct{ http.ResponseWriter }

func (w legacyFlushWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
	return n, err
}

func legacyAudioForwarder(t *testing.T, target string, record func(legacyAudioReceipt)) *httptest.Server {
	t.Helper()
	client := &http.Client{Timeout: 60 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
		if err != nil {
			http.Error(w, "read observer request", 500)
			return
		}
		receipt := legacyAudioReceipt{Path: r.URL.Path}
		var body struct {
			Model    string `json:"model"`
			Query    string `json:"query"`
			Messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"messages"`
		}
		if json.Unmarshal(raw, &body) == nil {
			receipt.Model, receipt.Query = body.Model, body.Query
			for _, message := range body.Messages {
				if text, ok := message.Content.(string); ok {
					if message.Role == "system" && strings.HasPrefix(text, "Transcribe only the audio") {
						receipt.Transcript = true
					}
					if message.Role == "user" {
						receipt.User = text
					}
				}
				if parts, ok := message.Content.([]any); ok {
					for _, p := range parts {
						if part, ok := p.(map[string]any); ok && part["type"] == "input_audio" {
							receipt.Audio = true
						}
					}
				}
			}
		}
		defer func() { record(receipt) }()
		request, err := http.NewRequestWithContext(r.Context(), r.Method, strings.TrimSuffix(target, "/")+r.URL.RequestURI(), bytes.NewReader(raw))
		if err != nil {
			http.Error(w, "observer request", 500)
			return
		}
		request.Header = r.Header.Clone()
		response, err := client.Do(request)
		if err != nil {
			http.Error(w, "upstream request", 502)
			return
		}
		defer response.Body.Close()
		receipt.Status = response.StatusCode
		w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
		w.WriteHeader(response.StatusCode)
		_, _ = io.Copy(legacyFlushWriter{w}, response.Body)
	}))
}
