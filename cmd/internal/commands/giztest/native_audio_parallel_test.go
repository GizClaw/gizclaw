package giztestcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// Exercise Factory -> AudioDock -> multimodal Eino -> OpenAI-compatible HTTP
// together with an independent ASR Transformer and the actual Giztest runner.
func TestParallelNativeAudioGiztest(t *testing.T) {
	var replyCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		replyCalls.Add(1)
		if !bytes.Contains(body, []byte(`"input_audio"`)) {
			t.Error("model request did not contain native audio")
		}
		if bytes.Contains(body, []byte(`"role":"system"`)) {
			t.Error("audio-only business request gained a system prompt")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"native-fixture\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"八。\"}}]}\n\ndata: {\"id\":\"native-fixture\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	client := openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("local-fixture"), option.WithMaxRetries(0))
	generator := &genx.OpenAIGenerator{Client: &client, Model: "audio-model", PromptRole: genx.PromptRoleSystem}
	packets := voiceTonePackets(t, 440)
	recognition := map[string]string{}
	for _, packet := range packets {
		recognition[string(packet)] = "三加五等于几"
	}
	resources := nativeParallelResources{}
	builder := nativeParallelBuilder{voiceFixtureProvider: &voiceFixtureProvider{recognize: recognition}, generator: generator}
	service := peergenx.New(peergenx.Service{Models: resources, Voices: resources, Credentials: resources, ProviderTenants: resources, Builder: builder})
	workflow := []byte(`{"graph":{"name":"native-parallel","state":{"fields":[{"name":"answer","type":"string","merge":"replace"}]},"nodes":[{"id":"reply","type":"chat_model","inputs":{"messages":{"from":"input.messages"}},"outputs":{"text":"answer"},"model":"audio-llm"}],"edges":[{"from":"start","to":"reply"},{"from":"reply","to":"end"}],"outputs":[{"node":"reply","field":"answer","name":"assistant","mime_type":"text/plain","primary":true}]}}`)
	agent := newVoiceFixtureAgent(t, "eino", workflow, service, apitypes.WorkspaceInputModePushToTalk, "fixture-asr")
	defer agent.(io.Closer).Close()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 128)
	defer input.Stream().Close()
	output, err := agent.Transform(ctx, input.Stream())
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	var ogg bytes.Buffer
	if err := codecconv.OpusPacketsToOgg(&ogg, 16000, 1, packets); err != nil {
		t.Fatal(err)
	}
	driver := &voiceFixtureDriver{driver: newDriver(true, nil), stream: &voiceFixtureStream{input: input, output: output}, audio: map[string][]byte{"recording": ogg.Bytes()}}
	document := `# User Story:
# As a native audio workflow caller,
# I want reply and transcription without a transcription prompt,
# So that AudioDock exposes independent recognition alongside the model reply.
version: gizclaw.test/v1alpha1
name: native-parallel
timeout: 10s
clients:
  peer:
    identity: ephemeral
    connection: webrtc
    access_point: http://fixture.invalid
variables: {}
steps:
  - id: audio-turn
    client: peer
    timeout: 8s
    peer_stream:
      mode: push-to-talk
      input: recording
      pacing: 1ms
      require_text: true
      require_audio: false
    expect:
      /transcript:
        equals: 三加五等于几
      /reply:
        equals: 八。
`
	path := filepath.Join(t.TempDir(), "native-parallel.giztest.yaml")
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := giztest.LoadDocument(path, driver)
	if err != nil {
		t.Fatal(err)
	}
	report := giztest.Run(ctx, []*giztest.Document{doc}, giztest.Options{Driver: driver, Parallel: 1, Out: io.Discard})
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Tasks) != 1 {
		t.Fatalf("tasks=%d", len(report.Tasks))
	}
	if report.Tasks[0].Status != "passed" || replyCalls.Load() != 1 {
		t.Fatalf("model calls=%d report=%s", replyCalls.Load(), encoded)
	}
}

type nativeParallelBuilder struct {
	*voiceFixtureProvider
	generator genx.Generator
}

func (b nativeParallelBuilder) BuildGenerator(context.Context, peergenx.GeneratorConfig) (genx.Generator, error) {
	return b.generator, nil
}

type nativeParallelResources struct{ voiceFixtureResources }

func (r nativeParallelResources) GetModel(ctx context.Context, request adminhttp.GetModelRequestObject) (adminhttp.GetModelResponseObject, error) {
	if request.Id == "fixture-asr" {
		return r.voiceFixtureResources.GetModel(ctx, request)
	}
	var data apitypes.ModelProviderData
	if err := data.FromVolcTenantModelProviderData(apitypes.VolcTenantModelProviderData{ApiMode: "chat_completions", UpstreamModel: new("audio-model"), SupportTextOnly: new(false)}); err != nil {
		return nil, err
	}
	return adminhttp.GetModel200JSONResponse(apitypes.Model{Id: request.Id, Kind: apitypes.ModelKindLlm, Provider: apitypes.ModelProvider{Kind: apitypes.ModelProviderKindVolcTenant, Id: "volc-main"}, ProviderData: data}), nil
}
