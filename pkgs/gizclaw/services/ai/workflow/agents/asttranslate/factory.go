package asttranslate

import (
	"context"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	genxast "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/asttranslate"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
)

const Type = "ast-translate"

// Factory converts GizClaw workflow and workspace configuration into the
// reusable GenX AST Translate Transformer. Stream execution belongs to GenX.
type Factory struct {
	Transformer genx.TransformerMux
}

func (f Factory) NewAgent(_ context.Context, spec agenthost.Spec) (agenthost.Agent, error) {
	config, err := resolveConfig(spec)
	if err != nil {
		return nil, err
	}
	config.Transformer = f.Transformer
	transformer, err := genxast.New(config)
	if err != nil {
		return nil, err
	}
	return agenthost.NewTransformerAgent(transformer), nil
}

func resolveConfig(spec agenthost.Spec) (genxast.Config, error) {
	workflowSpec := spec.Workflow.Spec.AstTranslate
	if workflowSpec == nil {
		return genxast.Config{}, fmt.Errorf("asttranslate: workflow spec.ast_translate is required")
	}
	model := strings.TrimSpace(workflowSpec.TranslationModel)
	params := workflowParams(*workflowSpec)
	externalVoice := astTranslateTTSVoice(params)
	if spec.Workspace.Parameters != nil {
		typed, err := spec.Workspace.Parameters.AsASTTranslateWorkspaceParameters()
		if err != nil {
			return genxast.Config{}, fmt.Errorf("asttranslate: decode workspace parameters: %w", err)
		}
		model = firstNonEmpty(ptrString(typed.TranslationModel), model)
		params = mergeWorkspaceParams(params, typed)
		externalVoice = firstNonEmpty(astTranslateWorkspaceTTSVoice(typed), externalVoice)
	}
	return genxast.Config{
		Model:         model,
		Params:        params,
		ExternalVoice: externalVoice,
	}, nil
}

func workflowParams(ast apitypes.ASTTranslateWorkflowSpec) map[string]any {
	params := make(map[string]any)
	if ast.Mode != nil {
		setParam(params, "mode", string(*ast.Mode))
	}
	if ast.Voice != nil {
		mergeASTTranslateVoice(params, *ast.Voice)
	}
	setParam(params, "enable_source_language_detect", ast.EnableSourceLanguageDetect)
	setParam(params, "denoise", ast.Denoise)
	setParam(params, "resource_id", ast.ResourceId)
	return params
}

func mergeWorkspaceParams(params map[string]any, typed apitypes.ASTTranslateWorkspaceParameters) map[string]any {
	if params == nil {
		params = make(map[string]any)
	}
	if typed.Mode != nil {
		setParam(params, "mode", string(*typed.Mode))
	}
	if typed.Input != nil {
		setParam(params, "input", string(*typed.Input))
	}
	if typed.LangPair != nil {
		setParam(params, "lang_pair", *typed.LangPair)
	}
	if typed.Voice != nil {
		mergeASTTranslateVoice(params, *typed.Voice)
	}
	if typed.EnableSourceLanguageDetect != nil {
		setParam(params, "enable_source_language_detect", *typed.EnableSourceLanguageDetect)
	}
	if typed.Denoise != nil {
		setParam(params, "denoise", *typed.Denoise)
	}
	return params
}

func mergeASTTranslateVoice(params map[string]any, value apitypes.ASTTranslateVoiceParameters) {
	if speaker, err := value.AsASTTranslateInternalSpeakerParameters(); err == nil && strings.TrimSpace(speaker.SpeakerId) != "" {
		params["speaker_id"] = speaker.SpeakerId
		setParam(params, "is_custom_speaker", speaker.IsCustomSpeaker)
		setParam(params, "tts_resource_id", speaker.TtsResourceId)
		setParam(params, "speech_rate", speaker.SpeechRate)
		return
	}
	voice, err := value.AsASTTranslateExternalVoiceParameters()
	if err == nil && strings.TrimSpace(voice.TtsVoice) != "" {
		params["tts_voice"] = voice.TtsVoice
	}
}

func astTranslateTTSVoice(params map[string]any) string {
	value, _ := params["tts_voice"].(string)
	return strings.TrimSpace(value)
}

func astTranslateWorkspaceTTSVoice(typed apitypes.ASTTranslateWorkspaceParameters) string {
	if typed.Voice == nil {
		return ""
	}
	voice, err := typed.Voice.AsASTTranslateExternalVoiceParameters()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(voice.TtsVoice)
}

func setParam(params map[string]any, key string, value any) {
	if value != nil {
		params[key] = value
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func ptrString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
