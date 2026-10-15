package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/adminapi"
	giztestcmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/giztest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

type podcastRequestReceipt struct {
	marker     string
	users      int
	assistants int
}

type podcastProtocolFixture struct {
	mu              sync.Mutex
	requests        []podcastRequestReceipt
	modelCancelled  atomic.Int32
	speechCancelled atomic.Int32
}

// All workflow execution, provider adaptation, audio pacing, History and RPC
// use production code. Only the remote model and speech protocols are local,
// deterministic fixtures; no billed provider or credentials are required.
func TestContinuousPodcastGiztest(t *testing.T) {
	fixture := &podcastProtocolFixture{}
	provider := httptest.NewServer(http.HandlerFunc(fixture.serve))
	t.Cleanup(provider.Close)
	withPeerControlServer(t, 8, func(ctx context.Context, _ *gizclaw.Server, admin *gizcli.Client, _, edgeURL string) {
		root := filepath.Join("..", "..", "..", "tests", "gizclaw-e2e", "testdata", "continuous-podcast")
		setupPodcastGiztest(t, ctx, admin, provider.URL, filepath.Join(root, "eino.json"))
		t.Setenv("GIZCLAW_TEST_ENDPOINT", edgeURL)
		t.Setenv("GIZCLAW_TEST_REGISTRATION_TOKEN", "local-peer-sync-test-token")
		reportDir := os.Getenv("GIZCLAW_TEST_PODCAST_GIZTEST_EVIDENCE")
		if reportDir == "" {
			reportDir = t.TempDir()
		}
		if err := os.MkdirAll(reportDir, 0700); err != nil {
			t.Fatal(err)
		}
		reportPath := filepath.Join(reportDir, "podcast.giztest.json")
		replayDir := filepath.Join(reportDir, "out")
		command := giztestcmd.NewCmd()
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetErr(&output)
		command.SetArgs([]string{"run", "--parallel", "1", "--report", reportPath, "--output", replayDir, filepath.Join(root, "continuous.giztest.yaml")})
		runErr := command.ExecuteContext(ctx)
		if err := os.WriteFile(filepath.Join(reportDir, "runner.log"), output.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		data, readErr := os.ReadFile(reportPath)
		if runErr != nil || readErr != nil {
			t.Fatalf("podcast Giztest failed: %v, report: %v\n%s\n%s", runErr, readErr, output.String(), data)
		}
		var report giztest.Report
		if err := json.Unmarshal(data, &report); err != nil {
			t.Fatal(err)
		}
		if report.Status != "passed" || len(report.Tasks) != 1 || len(report.Tasks[0].Steps) != 20 || len(report.Tasks[0].Cleanup) != 3 {
			t.Fatalf("podcast Giztest did not execute every step: %s", data)
		}
		pages, err := filepath.Glob(filepath.Join(replayDir, "index.html"))
		if err != nil || len(pages) != 1 {
			t.Fatalf("missing per-test replay: %v, %v", pages, err)
		}
		var replay struct {
			Report giztest.Report `json:"report"`
			Events []struct {
				Audio string `json:"audio"`
			} `json:"events"`
		}
		replayJSON, err := os.ReadFile(filepath.Join(filepath.Dir(pages[0]), "replay.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(replayJSON, &replay); err != nil {
			t.Fatal(err)
		}
		if len(replay.Report.Tasks) != 1 {
			t.Fatal("replay includes other tests")
		}
		audioClips := 0
		for _, event := range replay.Events {
			if event.Audio == "" {
				continue
			}
			clip, err := os.ReadFile(filepath.Join(filepath.Dir(pages[0]), filepath.FromSlash(event.Audio)))
			if err != nil || len(clip) <= 44 || string(clip[:4]) != "RIFF" {
				t.Fatalf("replay references missing or invalid WAV: %s, %v", event.Audio, err)
			}
			audioClips++
		}
		if audioClips == 0 {
			t.Fatal("real Giztest replay did not save conversation audio")
		}
		t.Logf("per-test HTML with %d local WAV clips: %s", audioClips, pages[0])
		children := 0
		for _, steps := range [][]giztest.StepReport{report.Tasks[0].Steps, report.Tasks[0].Cleanup} {
			for _, step := range steps {
				if step.Status != "passed" {
					t.Fatalf("Giztest step %s was %s", step.ID, step.Status)
				}
				for _, child := range step.Children {
					children++
					if child.Status != "passed" {
						t.Fatalf("Giztest child %s was %s", child.ID, child.Status)
					}
				}
			}
		}
		if children != 6 {
			t.Fatalf("Giztest executed %d parallel children, want 6", children)
		}
		fixture.mu.Lock()
		requests := append([]podcastRequestReceipt(nil), fixture.requests...)
		fixture.mu.Unlock()
		firstEpisode := 0
		for _, request := range requests {
			if request.marker == "Paused" {
				break
			}
			firstEpisode++
			if request.users != 1 || request.assistants > 8 {
				t.Fatalf("continuation added user turns or unbounded context: %+v", request)
			}
		}
		if firstEpisode < 9 {
			t.Fatalf("only %d generations; Graph step budget is 6", firstEpisode)
		}
		if fixture.modelCancelled.Load() < 2 || fixture.speechCancelled.Load() < 2 {
			t.Fatalf("interrupt did not cancel live provider I/O: model=%d TTS=%d", fixture.modelCancelled.Load(), fixture.speechCancelled.Load())
		}
		t.Logf("real Server/Edge/WebRTC: 20 Giztest steps, 6 parallel children and 3 cleanup steps; first episode=%d generations; cancelled model/TTS=%d/%d\n%s", firstEpisode, fixture.modelCancelled.Load(), fixture.speechCancelled.Load(), output.String())
	})
}

func setupPodcastGiztest(t *testing.T, ctx context.Context, admin *gizcli.Client, endpoint, graphPath string) {
	t.Helper()
	var credential apitypes.CredentialBody
	if err := credential.FromMiniMaxCredentialBody(apitypes.MiniMaxCredentialBody{ApiKey: new("local-protocol-fixture")}); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateCredential(ctx, admin, adminhttp.CredentialUpsert{Id: "podcast-fixture", Provider: "minimax", Body: credential}); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateMiniMaxTenant(ctx, admin, adminhttp.MiniMaxTenantUpsert{Id: "podcast-fixture", CredentialId: "podcast-fixture", BaseUrl: &endpoint}); err != nil {
		t.Fatal(err)
	}
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatal(err)
	}
	var modelData apitypes.ModelProviderData
	if err := modelData.FromMiniMaxTenantModelProviderData(apitypes.MiniMaxTenantModelProviderData{ApiMode: "chat_completions", UpstreamModel: "podcast-fixture", SupportTextOnly: new(true), UseSystemRole: new(true)}); err != nil {
		t.Fatal(err)
	}
	model, err := api.CreateModelWithResponse(ctx, adminhttp.ModelUpsert{Id: "podcast-model", Kind: apitypes.ModelKindLlm, Source: apitypes.ModelSourceManual, Provider: apitypes.ModelProvider{Kind: "minimax-tenant", Id: "podcast-fixture"}, ProviderData: modelData})
	if err != nil || model == nil || model.JSON200 == nil {
		t.Fatalf("create podcast Model: %v, %+v", err, model)
	}
	var voiceData apitypes.VoiceProviderData
	if err := voiceData.FromMiniMaxTenantVoiceProviderData(apitypes.MiniMaxTenantVoiceProviderData{VoiceId: new("podcast-host"), Model: new("podcast-speech"), Format: new("ogg_opus")}); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateVoice(ctx, admin, adminhttp.VoiceUpsert{Id: "podcast-voice", Source: apitypes.VoiceSourceManual, Provider: apitypes.VoiceProvider{Kind: "minimax-tenant", Id: "podcast-fixture"}, ProviderData: &voiceData}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(graphPath)
	if err != nil {
		t.Fatal(err)
	}
	var graph apitypes.EinoWorkflowSpec
	if err := json.Unmarshal(data, &graph); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateWorkflow(ctx, admin, apitypes.Workflow{Id: "eino-podcast", Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverEino, Eino: &graph}}); err != nil {
		t.Fatal(err)
	}
	binding := func(id string) apitypes.RuntimeProfileBinding {
		return apitypes.RuntimeProfileBinding{ResourceId: id, I18n: map[string]apitypes.RuntimeProfileI18nText{"en": {DisplayName: "Podcast fixture"}, "zh-CN": {DisplayName: "播客夹具"}}}
	}
	profile := adminhttp.RuntimeProfileUpsert{Id: "peer-sync-giztest", Spec: apitypes.RuntimeProfileSpec{
		Workflows: apitypes.RuntimeProfileWorkflows{"podcast": {ResourceId: "eino-podcast", I18n: binding("eino-podcast").I18n}},
		Resources: apitypes.RuntimeProfileResources{Models: new(map[string]apitypes.RuntimeProfileBinding{"podcast.model": binding("podcast-model")}), Voices: new(map[string]apitypes.RuntimeProfileBinding{"podcast.host": binding("podcast-voice")})},
	}}
	if _, err := adminapi.PutRuntimeProfile(ctx, admin, profile.Id, profile); err != nil {
		t.Fatal(err)
	}
}

func (f *podcastProtocolFixture) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v1/t2a_v2" {
		f.speech(w, r)
		return
	}
	if r.URL.Path != "/v1/chat/completions" {
		http.NotFound(w, r)
		return
	}
	var request struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	receipt := podcastRequestReceipt{}
	for _, message := range request.Messages {
		if marker, ok := strings.CutPrefix(message.Content, "podcast-section "); ok {
			receipt.marker = marker
		}
		if message.Role == "user" {
			receipt.users++
		}
		if message.Role == "assistant" {
			receipt.assistants++
		}
	}
	if receipt.marker == "" {
		http.Error(w, "missing section marker", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.requests = append(f.requests, receipt)
	f.mu.Unlock()
	select {
	case <-time.After(20 * time.Millisecond):
	case <-r.Context().Done():
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	write := func(delta map[string]any, finish any) bool {
		data, _ := json.Marshal(map[string]any{"id": "podcast", "object": "chat.completion.chunk", "created": 1, "model": "podcast-fixture", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return false
		}
		w.(http.Flusher).Flush()
		return true
	}
	if !write(map[string]any{"role": "assistant", "content": receipt.marker + ". "}, nil) {
		return
	}
	if strings.HasPrefix(receipt.marker, "slow-") {
		select {
		case <-r.Context().Done():
			f.modelCancelled.Add(1)
		case <-time.After(15 * time.Second):
		}
		return
	}
	if write(map[string]any{}, "stop") {
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}
}

func (f *podcastProtocolFixture) speech(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Text          string `json:"text"`
		Stream        bool   `json:"stream"`
		StreamOptions struct {
			ExcludeAggregatedAudio bool `json:"exclude_aggregated_audio"`
		} `json:"stream_options"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&request); err != nil || r.Method != http.MethodPost || !request.Stream || !request.StreamOptions.ExcludeAggregatedAudio {
		http.Error(w, "invalid speech request", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	write := func(audio []byte, final bool) error {
		status := 1
		if final {
			status = 2
		}
		payload, err := json.Marshal(map[string]any{"data": map[string]any{"audio": hex.EncodeToString(audio), "status": status}, "base_resp": map[string]any{"status_code": 0, "status_msg": "success"}})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(w, "data: %s\n\n", payload)
		if err != nil {
			return err
		}
		w.(http.Flusher).Flush()
		return nil
	}
	pcm, slow := podcastSpeechPCM(request.Text)
	for offset := 0; offset < len(pcm); offset += 640 {
		if err := write(pcm[offset:offset+640], false); err != nil {
			return
		}
	}
	if slow {
		<-ctx.Done()
		if ctx.Err() != context.DeadlineExceeded {
			f.speechCancelled.Add(1)
		}
		return
	}
	_ = write(nil, true)
}

func podcastSpeechPCM(text string) ([]byte, bool) {
	samples, frequency := 3200, 440.0
	if strings.HasPrefix(text, "space:") {
		frequency = 880
	}
	slow := strings.HasPrefix(text, "slow-")
	if slow {
		samples = 32000
	}
	pcm := make([]byte, samples*2)
	for i := range samples {
		binary.LittleEndian.PutUint16(pcm[2*i:], uint16(int16(12000*math.Sin(2*math.Pi*frequency*float64(i)/16000))))
	}
	return pcm, slow
}
