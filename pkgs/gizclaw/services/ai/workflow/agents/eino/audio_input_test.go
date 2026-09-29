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
	"testing"

	"github.com/cloudwego/eino/schema"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	genxeino "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/eino"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/einoconfig"
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

func TestFactoryAudioTranscriptRules(t *testing.T) {
	t.Parallel()
	spec := einoFactorySpec(t)
	spec.Workflow.Spec.Eino = audioTranscriptSpec(t)
	spec.Workspace.Parameters = einoWorkspaceParameters(t, apitypes.WorkspaceInputModeRealtime)
	if _, err := (Factory{GenX: &peergenx.Service{}}).NewAgent(t.Context(), spec); err == nil ||
		!strings.Contains(err.Error(), "requires push-to-talk input") {
		t.Fatalf("NewAgent(realtime audio_transcript) error = %v", err)
	}

	asr := "asr"
	withASR := audioTranscriptSpec(t)
	withASR.VoiceAdapter = &apitypes.VoiceAdapter{AsrModel: &asr}
	if err := einoconfig.Validate(*withASR); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("Validate(asr_model with audio_transcript) error = %v", err)
	}
	voice := "speech.voice"
	ttsOnly := audioTranscriptSpec(t)
	ttsOnly.VoiceAdapter = &apitypes.VoiceAdapter{DefaultVoice: &voice}
	if err := einoconfig.Validate(*ttsOnly); err != nil {
		t.Fatalf("Validate(TTS-only audio_transcript) error = %v", err)
	}
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
