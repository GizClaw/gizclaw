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
		name string
		// asrModel and node say which audio input paths the Workflow declares.
		asrModel bool
		node     bool
		// textModel binds the audio_transcript node to a text-only Model.
		textModel bool
		selected  *apitypes.AudioInputPath
		realtime  bool
		// want is the reported path; empty means a text-only Agent.
		want    apitypes.AudioInputPath
		wantErr string
	}{
		{name: "both declared default", asrModel: true, node: true, want: asr},
		{name: "both declared model preferred", asrModel: true, node: true, selected: &model, want: model},
		{name: "both declared asr preferred", asrModel: true, node: true, selected: &asr, want: asr},
		{name: "model preferred falls back for a text-only Model", asrModel: true, node: true, textModel: true, selected: &model, want: asr},
		{name: "model preferred keeps asr for realtime", asrModel: true, node: true, selected: &model, realtime: true, want: asr},
		{name: "model preferred without node falls back", asrModel: true, selected: &model, want: asr},
		{name: "asr only default", asrModel: true, want: asr},
		{name: "node only default", node: true, want: model},
		{name: "node only asr preferred falls back", node: true, selected: &asr, want: model},
		{name: "node only text-only Model", node: true, textModel: true, wantErr: "does not accept audio input"},
		{name: "node only realtime", node: true, realtime: true, wantErr: "realtime input requires voice_adapter.asr_model"},
		{name: "text only default"},
		{name: "text only ignores a preference", selected: &model},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			spec := einoFactorySpec(t)
			spec.Workflow.Spec.Eino = audioTranscriptSpec(t)
			if !test.node {
				spec.Workflow.Spec.Eino = audioTranscriptSpecWithout(t)
			}
			if test.asrModel {
				alias := "asr"
				spec.Workflow.Spec.Eino.VoiceAdapter = &apitypes.VoiceAdapter{AsrModel: &alias}
			}
			spec.AudioInput = test.selected
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
				if heard != "audio" || asrBuilds != 0 || transcript.String() != "model heard" || reply.String() != "reply" {
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

func TestWithoutAudioTranscriptLeavesTheSourceGraph(t *testing.T) {
	t.Parallel()
	source := genxeino.GraphDefinition{Nodes: []genxeino.NodeDefinition{
		{ID: "prompt", Prompt: &genxeino.PromptNode{}},
		{ID: "answer", ChatModel: &genxeino.ChatModelNode{Model: "audio-llm", AudioTranscript: true}},
	}}
	stripped := withoutAudioTranscript(source)
	if genxeino.AcceptsAudioInput(genxeino.Config{Graph: stripped}) {
		t.Fatal("stripped Graph still starts audio turns")
	}
	if stripped.Nodes[1].ChatModel.Model != "audio-llm" {
		t.Fatalf("stripped chat_model = %#v", stripped.Nodes[1].ChatModel)
	}
	if !genxeino.AcceptsAudioInput(genxeino.Config{Graph: source}) {
		t.Fatal("withoutAudioTranscript modified the source Graph")
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
	textModel bool
}

func (m audioPathModels) GetModel(_ context.Context, request adminhttp.GetModelRequestObject) (adminhttp.GetModelResponseObject, error) {
	model := apitypes.Model{
		Id: request.Id, Kind: apitypes.ModelKindLlm,
		Provider: apitypes.ModelProvider{Kind: apitypes.ModelProviderKindVolcTenant, Id: "volc-main"},
	}
	data := apitypes.VolcTenantModelProviderData{
		ApiMode: apitypes.VolcTenantModelProviderDataApiModeChatCompletions, SupportTextOnly: &m.textModel,
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

// audioTranscriptSpecWithout returns the audio_transcript fixture with the
// flag removed from its chat_model node.
func audioTranscriptSpecWithout(t testing.TB) *apitypes.EinoWorkflowSpec {
	t.Helper()
	public := audioTranscriptSpec(t)
	node, err := public.Graph.Nodes[0].AsEinoChatModelNode()
	if err != nil {
		t.Fatalf("decode chat_model node: %v", err)
	}
	node.AudioTranscript = nil
	if err := public.Graph.Nodes[0].FromEinoChatModelNode(node); err != nil {
		t.Fatalf("encode chat_model node: %v", err)
	}
	return public
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
				"model": "audio-llm",
				"audio_transcript": true
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
