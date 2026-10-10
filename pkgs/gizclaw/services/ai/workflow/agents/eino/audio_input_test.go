package eino

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	genxeino "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/eino"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
)

func TestGenXModelContextSendsUserAudioPartsAsBlobs(t *testing.T) {
	t.Parallel()
	context, err := genXModelContext([]*schema.Message{
		schema.SystemMessage("be brief"),
		schema.UserMessage("earlier"),
		schema.AssistantMessage("reply", nil),
		{Role: schema.User, UserInputMultiContent: testAudioParts("audio/opus", []byte{1}, []byte{2, 3})},
	})
	if err != nil {
		t.Fatalf("genXModelContext() error = %v", err)
	}
	var messages []*genx.Message
	for message := range context.Messages() {
		messages = append(messages, message)
	}
	if len(messages) != 3 {
		t.Fatalf("messages = %d, want 3", len(messages))
	}
	contents, ok := messages[2].Payload.(genx.Contents)
	if !ok || messages[2].Role != genx.RoleUser || len(contents) != 2 {
		t.Fatalf("audio user message = %#v", messages[2])
	}
	for index, want := range [][]byte{{1}, {2, 3}} {
		blob, ok := contents[index].(*genx.Blob)
		if !ok || blob.MIMEType != "audio/opus" || !bytes.Equal(blob.Data, want) {
			t.Fatalf("audio part %d = %#v", index, contents[index])
		}
	}
}

func TestUserAudioBlobsRejectsUnusableParts(t *testing.T) {
	t.Parallel()
	for _, parts := range [][]schema.MessageInputPart{
		{{Type: schema.ChatMessagePartTypeImageURL}},
		{{Type: schema.ChatMessagePartTypeAudioURL, Audio: &schema.MessageInputAudio{}}},
	} {
		if _, err := userAudioBlobs(parts); err == nil {
			t.Fatalf("userAudioBlobs(%#v) accepted unusable audio", parts)
		}
	}
}

func TestGenXChatModelForwardsInputTranscript(t *testing.T) {
	t.Parallel()
	chat := genXChatModel{generator: einoStreamGenerator{chunks: []*genx.MessageChunk{
		genx.NewInputTranscriptChunk("三加五等于几"),
		{Role: genx.RoleModel, Part: genx.Text("八")},
	}}, pattern: "model/audio-llm"}
	reader, err := chat.Stream(t.Context(), []*schema.Message{schema.UserMessage("x")})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var got []*schema.Message
	for {
		message, err := reader.Recv()
		// The adapter forwards the GenX terminal state, which Eino treats as
		// the end of the model stream.
		if errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, message)
	}
	want := genxeino.TranscriptMessage("三加五等于几")
	if len(got) != 2 || got[0].Role != want.Role || got[0].Content != want.Content || !reflect.DeepEqual(got[0].Extra, want.Extra) || got[1].Content != "八" {
		t.Fatalf("messages = %#v", got)
	}
}

type einoStreamGenerator struct {
	chunks []*genx.MessageChunk
}

func (g einoStreamGenerator) GenerateStream(_ context.Context, _ string, mctx genx.ModelContext) (genx.Stream, error) {
	builder := genx.NewGrowableStreamBuilder(mctx, 4)
	if err := builder.Add(g.chunks...); err != nil {
		return nil, err
	}
	if err := builder.Done(genx.Usage{}); err != nil {
		return nil, err
	}
	return builder.Stream(), nil
}

func (einoStreamGenerator) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("not used")
}

func TestFactoryAudioInputPath(t *testing.T) {
	t.Parallel()
	asr, model := apitypes.AudioInputPathAsr, apitypes.AudioInputPathModel
	for _, test := range []struct {
		name                           string
		legacyASR                      bool
		asrModel                       string
		textModel, realtime, textLogic bool
		want                           apitypes.AudioInputPath
		wantErr                        string
	}{
		{name: "native input without graph configuration", want: model},
		{name: "legacy Workflow ASR slot is ignored", legacyASR: true, want: model},
		{name: "Profile selects external ASR", asrModel: "asr", want: asr},
		{name: "external ASR feeds text to an audio-capable Model", asrModel: "asr", legacyASR: true, want: asr},
		{name: "input text is prepared before legacy control scripts", textLogic: true, want: model},
		{name: "text Model with Profile ASR", textModel: true, asrModel: "asr", want: asr},
		{name: "invalid Profile ASR never falls back", asrModel: "audio-llm", wantErr: "is not a transformer"},
		{name: "native Lite cannot segment realtime input", realtime: true, wantErr: "configure RuntimeProfile realtime_asr_model"},
		{name: "Profile realtime ASR", asrModel: "asr", realtime: true, want: asr},
		{name: "text only without ASR", textModel: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			spec := einoFactorySpec(t)
			spec.Workflow.Spec.Eino = audioTranscriptSpec(t)
			if test.textLogic {
				spec.Workflow.Spec.Eino = audioInputControlSpec(t)
			}
			if test.legacyASR {
				alias := "ignored-legacy-slot"
				spec.Workflow.Spec.Eino.VoiceAdapter = &apitypes.VoiceAdapter{AsrModel: &alias}
			}
			spec.ASRModel = test.asrModel
			if test.realtime {
				spec.Workspace.Parameters = einoWorkspaceParameters(t, apitypes.WorkspaceInputModeRealtime)
			}
			builder := &audioPathBuilder{}
			service := peergenx.New(peergenx.Service{
				Models:          audioPathModels{textModel: test.textModel},
				Credentials:     einoTTSResources{},
				ProviderTenants: einoTTSResources{},
				Builder:         builder,
			})
			agent, err := (Factory{GenX: service}).NewAgent(t.Context(), spec)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("NewAgent() error = %v, want containing %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewAgent() error = %v", err)
			}
			if closer, ok := agent.(io.Closer); ok {
				defer closer.Close()
			}
			state, err := agent.Status(t.Context())
			if err != nil {
				t.Fatalf("Status() error = %v", err)
			}
			if test.want == "" {
				if state.AudioInput != nil {
					t.Fatalf("Status().AudioInput = %q, want absent", *state.AudioInput)
				}
				return
			}
			if state.AudioInput == nil || *state.AudioInput != test.want {
				t.Fatalf("Status().AudioInput = %v, want %q", state.AudioInput, test.want)
			}

			chunks := runAudioTurn(t, agent)
			var transcript, reply strings.Builder
			for _, chunk := range chunks {
				text, ok := chunk.Part.(genx.Text)
				if !ok || chunk.Ctrl == nil {
					continue
				}
				switch chunk.Ctrl.Label {
				case "transcript":
					transcript.WriteString(string(text))
				case "assistant":
					reply.WriteString(string(text))
				}
			}
			heard, asrBuilds := builder.snapshot()
			if test.want == model {
				wantHeard := "audio"
				if test.textLogic {
					wantHeard = "model heard"
				}
				if heard != wantHeard || asrBuilds != 0 || transcript.String() != "model heard" || reply.String() != "reply" {
					t.Fatalf("model path: heard %q, ASR builds %d, transcript %q, reply %q", heard, asrBuilds, transcript.String(), reply.String())
				}
				return
			}
			if heard != "asr heard" || asrBuilds != 1 || transcript.String() != "asr heard" || reply.String() != "reply" {
				t.Fatalf("asr path: heard %q, ASR builds %d, transcript %q, reply %q", heard, asrBuilds, transcript.String(), reply.String())
			}
		})
	}
}

func TestAutomaticAudioReceiverDoesNotChangeSourceGraph(t *testing.T) {
	source := genxeino.GraphDefinition{Nodes: []genxeino.NodeDefinition{
		{ID: "prompt", Prompt: &genxeino.PromptNode{}},
		{ID: "answer", ChatModel: &genxeino.ChatModelNode{Model: "audio-llm"}},
	}}
	derived := automaticAudioReceiver(source, "audio-llm")
	if !genxeino.AcceptsAudioInput(genxeino.Config{Graph: derived}) || genxeino.AcceptsAudioInput(genxeino.Config{Graph: source}) {
		t.Fatal("automatic receiver did not preserve the source Graph")
	}
	source.Nodes[0].Inputs = map[string]genxeino.Binding{"text": {From: "input.text"}}
	staged := automaticAudioReceiver(source, "audio-llm")
	if genxeino.AcceptsAudioInput(genxeino.Config{Graph: staged}) {
		t.Fatal("text-dependent Graph must prepare input transcription")
	}
}

// runAudioTurn sends one push-to-talk audio route and returns every output
// chunk of the session.
func runAudioTurn(t testing.TB, agent genx.Transformer) []*genx.MessageChunk {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	input := einoAudioGuardInput()
	output, err := agent.Transform(ctx, input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer output.Close()
	if err := input.Add(
		&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1, 2}}, Ctrl: &genx.StreamCtrl{StreamID: "speech", BeginOfStream: true}},
		&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "speech", EndOfStream: true}},
	); err != nil {
		t.Fatal(err)
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatal(err)
	}
	return einoCollectChunks(t, output)
}

// audioPathModels serves the chat Model of the audio_transcript node and the
// streaming ASR Model.
type audioPathModels struct {
	textModel   bool
	textModels  map[string]bool
	resourceIDs map[string]string
}

func (m audioPathModels) GetModel(_ context.Context, request adminhttp.GetModelRequestObject) (adminhttp.GetModelResponseObject, error) {
	model := apitypes.Model{
		Id: request.Id, Kind: apitypes.ModelKindLlm,
		Provider: apitypes.ModelProvider{Kind: apitypes.ModelProviderKindVolcTenant, Id: "volc-main"},
	}
	data := apitypes.VolcTenantModelProviderData{
		ApiMode: apitypes.VolcTenantModelProviderDataApiModeChatCompletions, SupportTextOnly: &m.textModel,
	}
	if value, ok := m.textModels[request.Id]; ok {
		data.SupportTextOnly = &value
	}
	if id := m.resourceIDs[request.Id]; id != "" {
		model.Id = id
	}
	if request.Id == "asr" {
		model.Kind = apitypes.ModelKindAsr
		data = apitypes.VolcTenantModelProviderData{ApiMode: apitypes.VolcTenantModelProviderDataApiModeAsr}
	}
	if err := model.ProviderData.FromVolcTenantModelProviderData(data); err != nil {
		return nil, err
	}
	return adminhttp.GetModel200JSONResponse(model), nil
}

func TestAutomaticInputModelSelectionUsesBoundCapabilities(t *testing.T) {
	for _, tc := range []struct {
		name      string
		resources audioPathModels
		wantErr   bool
	}{
		{name: "one native and one text Model", resources: audioPathModels{textModels: map[string]bool{"other": true}}},
		{name: "two aliases for the same native Model", resources: audioPathModels{resourceIDs: map[string]string{"audio-llm": "shared", "other": "shared"}}},
		{name: "different native Models are ambiguous", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			public := audioTranscriptSpecWithout(t)
			node, err := public.Graph.Nodes[0].AsEinoChatModelNode()
			if err != nil {
				t.Fatal(err)
			}
			node.Id, node.Model = "other", "other"
			var raw apitypes.EinoNode
			if err := raw.FromEinoChatModelNode(node); err != nil {
				t.Fatal(err)
			}
			public.Graph.Nodes = append(public.Graph.Nodes, raw)
			service := peergenx.New(peergenx.Service{Models: tc.resources, Credentials: einoTTSResources{}, ProviderTenants: einoTTSResources{}})
			alias, capable, err := selectInputModel(t.Context(), service, *public)
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "multiple audio-capable Models") {
					t.Fatalf("selection=%q capable=%v error=%v", alias, capable, err)
				}
				return
			}
			if err != nil || !capable || alias != "audio-llm" {
				t.Fatalf("selection=%q capable=%v error=%v", alias, capable, err)
			}
		})
	}
}

// audioPathBuilder records what the chat Model received and how often the
// ASR stage was built.
type audioPathBuilder struct {
	mu        sync.Mutex
	heard     string
	asrBuilds int
}

func (b *audioPathBuilder) snapshot() (string, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.heard, b.asrBuilds
}

func (b *audioPathBuilder) BuildGenerator(context.Context, peergenx.GeneratorConfig) (genx.Generator, error) {
	return audioPathGenerator{builder: b}, nil
}

func (b *audioPathBuilder) BuildTransformer(_ context.Context, config peergenx.TransformerConfig) (genx.Transformer, error) {
	if config.Model == nil || config.Model.Kind != apitypes.ModelKindAsr {
		return nil, errors.New("unexpected transformer build")
	}
	b.mu.Lock()
	b.asrBuilds++
	b.mu.Unlock()
	return einoTestTransformer(func(_ context.Context, input genx.Stream) (genx.Stream, error) {
		output := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
		go func() {
			defer input.Close()
			for {
				chunk, err := input.Next()
				if err != nil {
					_ = output.Done(genx.Usage{})
					return
				}
				if !chunk.IsEndOfStream() {
					continue
				}
				streamID := einoChunkStreamID(chunk)
				_ = output.Add(
					&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text("asr heard"), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "transcript", BeginOfStream: true}},
					&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "transcript", EndOfStream: true}},
				)
			}
		}()
		return output.Stream(), nil
	}), nil
}

// audioPathGenerator answers like an audio-input chat Model: it reports a
// transcript when the latest user message carries audio.
type audioPathGenerator struct {
	builder *audioPathBuilder
}

func (audioPathGenerator) TranscribeInput(context.Context, string, genx.ModelContext) (string, genx.Usage, error) {
	return "model heard", genx.Usage{}, nil
}

func (g audioPathGenerator) GenerateStream(_ context.Context, _ string, mctx genx.ModelContext) (genx.Stream, error) {
	heard := ""
	for message := range mctx.Messages() {
		if message.Role != genx.RoleUser {
			continue
		}
		contents, _ := message.Payload.(genx.Contents)
		for _, part := range contents {
			switch value := part.(type) {
			case genx.Text:
				heard = string(value)
			case *genx.Blob:
				heard = "audio"
			}
		}
	}
	g.builder.mu.Lock()
	g.builder.heard = heard
	g.builder.mu.Unlock()
	output := genx.NewGrowableStreamBuilder(mctx, 4)
	var chunks []*genx.MessageChunk
	if heard == "audio" {
		chunks = append(chunks, genx.NewInputTranscriptChunk("model heard"))
	}
	chunks = append(chunks, &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("reply")})
	if err := output.Add(chunks...); err != nil {
		return nil, err
	}
	if err := output.Done(genx.Usage{}); err != nil {
		return nil, err
	}
	return output.Stream(), nil
}

func (audioPathGenerator) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("not used")
}

func audioTranscriptSpecWithout(t testing.TB) *apitypes.EinoWorkflowSpec {
	return audioTranscriptSpec(t)
}

func audioInputControlSpec(t testing.TB) *apitypes.EinoWorkflowSpec {
	t.Helper()
	var spec apitypes.EinoWorkflowSpec
	body := `{"graph":{"name":"control-input","state":{"fields":[{"name":"messages","type":"messages","merge":"replace"},{"name":"answer","type":"string","merge":"replace"}]},"nodes":[{"id":"control","type":"script","inputs":{"text":{"from":"input.text"},"messages":{"from":"input.messages"}},"outputs":{"messages":"messages"},"language":"starlark","entrypoint":"run","limits":{"max_execution_steps":10000,"timeout":"1s","max_input_bytes":65536,"max_output_bytes":65536},"source":"def run(input):\n  if input[\"text\"] != \"model heard\":\n    fail(\"empty or wrong transcription\")\n  return {\"messages\": input[\"messages\"]}\n"},{"id":"answer","type":"chat_model","model":"audio-llm","inputs":{"messages":{"from":"messages"}},"outputs":{"text":"answer"}}],"edges":[{"from":"start","to":"control"},{"from":"control","to":"answer"},{"from":"answer","to":"end"}],"outputs":[{"node":"answer","field":"answer","name":"assistant","mime_type":"text/plain","primary":true}]}}`
	if err := json.Unmarshal([]byte(body), &spec); err != nil {
		t.Fatal(err)
	}
	return &spec
}

func audioTranscriptSpec(t testing.TB) *apitypes.EinoWorkflowSpec {
	t.Helper()
	var public apitypes.EinoWorkflowSpec
	if err := json.Unmarshal([]byte(`{
		"graph": {
			"name": "audio-transcript",
			"compile": {"node_trigger_mode": "any_predecessor"},
			"state": {"fields": [{"name": "answer", "type": "string", "merge": "replace"}]},
			"nodes": [{
				"id": "answer",
				"type": "chat_model",
				"inputs": {"messages": {"from": "input.messages"}},
				"outputs": {"text": "answer"},
				"model": "audio-llm"
			}],
			"edges": [{"from": "start", "to": "answer"}, {"from": "answer", "to": "end"}],
			"branches": [],
			"outputs": [{"node": "answer", "field": "answer", "name": "assistant", "mime_type": "text/plain", "primary": true}]
		}
	}`), &public); err != nil {
		t.Fatalf("decode audio_transcript fixture: %v", err)
	}
	return &public
}

func testAudioParts(mimeType string, payloads ...[]byte) []schema.MessageInputPart {
	parts := make([]schema.MessageInputPart, 0, len(payloads))
	for _, payload := range payloads {
		data := base64.StdEncoding.EncodeToString(payload)
		parts = append(parts, schema.MessageInputPart{
			Type: schema.ChatMessagePartTypeAudioURL,
			Audio: &schema.MessageInputAudio{MessagePartCommon: schema.MessagePartCommon{
				Base64Data: &data, MIMEType: mimeType,
			}},
		})
	}
	return parts
}
