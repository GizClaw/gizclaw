package asttranslate

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
)

func TestFactoryMergesWorkflowAndWorkspaceParams(t *testing.T) {
	langPair := "zh/ja"
	mode := apitypes.ASTTranslateModeS2s
	input := apitypes.WorkspaceInputModePushToTalk
	var voice apitypes.ASTTranslateVoiceParameters
	if err := voice.FromASTTranslateInternalSpeakerParameters(apitypes.ASTTranslateInternalSpeakerParameters{SpeakerId: "workspace-speaker"}); err != nil {
		t.Fatalf("FromASTTranslateInternalSpeakerParameters() error = %v", err)
	}
	var workspaceParams apitypes.WorkspaceParameters
	if err := workspaceParams.FromASTTranslateWorkspaceParameters(apitypes.ASTTranslateWorkspaceParameters{
		LangPair: &langPair, Mode: &mode, Input: &input, Voice: &voice,
	}); err != nil {
		t.Fatalf("FromASTTranslateWorkspaceParameters() error = %v", err)
	}
	agent, err := (Factory{Transformer: recordingTransformer{}}).NewAgent(context.Background(), agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "demo", Parameters: &workspaceParams},
		Workflow:  astWorkflow("ast-model", nil),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	stream, err := agent.Transform(context.Background(), emptyStream{})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer stream.Close()
	chunk, err := stream.Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	got := string(chunk.Part.(genx.Text))
	for _, want := range []string{
		"model/ast-model?", "source_language=zh", "target_language=ja", "mode=s2s", "input=push-to-talk", "speaker_id=workspace-speaker",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("pattern = %q, missing %q", got, want)
		}
	}
}

func TestResolveConfigExternalVoice(t *testing.T) {
	var voice apitypes.ASTTranslateVoiceParameters
	if err := voice.FromASTTranslateExternalVoiceParameters(apitypes.ASTTranslateExternalVoiceParameters{TtsVoice: "volc-tenant:e2e-volc-tenant:voice-a"}); err != nil {
		t.Fatalf("FromASTTranslateExternalVoiceParameters() error = %v", err)
	}
	langPair := "auto"
	var workspaceParams apitypes.WorkspaceParameters
	if err := workspaceParams.FromASTTranslateWorkspaceParameters(apitypes.ASTTranslateWorkspaceParameters{LangPair: &langPair, Voice: &voice}); err != nil {
		t.Fatalf("FromASTTranslateWorkspaceParameters() error = %v", err)
	}
	config, err := resolveConfig(agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "demo", Parameters: &workspaceParams},
		Workflow:  astWorkflow("ast-model", nil),
	})
	if err != nil {
		t.Fatalf("resolveConfig() error = %v", err)
	}
	if config.ExternalVoice != "volc-tenant:e2e-volc-tenant:voice-a" || config.Model != "ast-model" || config.Params["lang_pair"] != "auto" {
		t.Fatalf("resolveConfig() = %#v", config)
	}
}

func TestMergeWorkspaceParamsInternalSpeaker(t *testing.T) {
	customSpeaker := true
	speechRate := 15
	var voice apitypes.ASTTranslateVoiceParameters
	if err := voice.FromASTTranslateInternalSpeakerParameters(apitypes.ASTTranslateInternalSpeakerParameters{
		SpeakerId: "speaker-a", IsCustomSpeaker: &customSpeaker, TtsResourceId: stringPtr("tts-resource"), SpeechRate: &speechRate,
	}); err != nil {
		t.Fatalf("FromASTTranslateInternalSpeakerParameters() error = %v", err)
	}
	params := mergeWorkspaceParams(nil, apitypes.ASTTranslateWorkspaceParameters{LangPair: stringPtr("en/jp"), Voice: &voice, Denoise: &customSpeaker})
	for _, want := range []string{"lang_pair", "speaker_id", "is_custom_speaker", "tts_resource_id", "speech_rate", "denoise"} {
		if _, ok := params[want]; !ok {
			t.Fatalf("params = %#v, missing %q", params, want)
		}
	}
}

func TestFactoryErrors(t *testing.T) {
	if _, err := (Factory{}).NewAgent(context.Background(), agenthost.Spec{}); err == nil {
		t.Fatal("NewAgent() without AST spec succeeded, want error")
	}
	if _, err := (Factory{}).NewAgent(context.Background(), agenthost.Spec{Workflow: astWorkflow("model-a", nil)}); err == nil {
		t.Fatal("NewAgent() without transformer succeeded, want error")
	}
}

type recordingTransformer struct{}

func (recordingTransformer) Transform(_ context.Context, pattern string, _ genx.Stream) (genx.Stream, error) {
	builder := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 2)
	_ = builder.Add(&genx.MessageChunk{Part: genx.Text(pattern)})
	_ = builder.Done(genx.Usage{})
	return builder.Stream(), nil
}

type emptyStream struct{}

func (emptyStream) Next() (*genx.MessageChunk, error) { return nil, io.EOF }
func (emptyStream) Close() error                      { return nil }
func (emptyStream) CloseWithError(error) error        { return nil }

func astWorkflow(model string, voice *apitypes.ASTTranslateVoiceParameters) apitypes.Workflow {
	return apitypes.Workflow{Name: "ast", Spec: apitypes.WorkflowSpec{
		Driver: apitypes.WorkflowDriverAstTranslate,
		AstTranslate: &apitypes.ASTTranslateWorkflowSpec{
			TranslationModel: model,
			Voice:            voice,
		},
	}}
}

func stringPtr(value string) *string { return &value }
