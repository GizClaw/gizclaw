package peergenx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/minimax-go"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"google.golang.org/genai"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/genx/transformers"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

type DefaultBuilder struct {
	HTTPClient *http.Client
}

const (
	defaultVolcTTSAudioFormat    = "ogg_opus"
	defaultMiniMaxTTSAudioFormat = "mp3"
	defaultTTSAudioSampleRate    = 16000
	defaultMiniMaxBaseURL        = "https://api.minimax.io"
	defaultVolcArkBaseURL        = "https://ark.cn-beijing.volces.com/api/v3"
)

func (b DefaultBuilder) BuildGenerator(ctx context.Context, cfg GeneratorConfig) (genx.Generator, error) {
	switch cfg.Tenant.Kind {
	case string(apitypes.ModelProviderKindOpenaiTenant):
		return b.buildOpenAIGenerator(cfg)
	case string(apitypes.ModelProviderKindVolcTenant):
		return b.buildVolcArkGenerator(cfg)
	case string(apitypes.ModelProviderKindGeminiTenant):
		return b.buildGeminiGenerator(ctx, cfg)
	default:
		return nil, fmt.Errorf("%w: generator provider %q", ErrUnsupported, cfg.Tenant.Kind)
	}
}

func (b DefaultBuilder) BuildTransformer(_ context.Context, cfg TransformerConfig) (genx.Transformer, error) {
	if cfg.Voice != nil {
		switch cfg.Tenant.Kind {
		case string(apitypes.VoiceProviderKindVolcTenant):
			return b.buildVolcTTS(cfg)
		case string(apitypes.VoiceProviderKindMinimaxTenant):
			return b.buildMiniMaxTTS(cfg)
		default:
			return nil, fmt.Errorf("%w: voice transformer provider %q", ErrUnsupported, cfg.Tenant.Kind)
		}
	}
	if cfg.Model != nil {
		switch cfg.Model.Kind {
		case apitypes.ModelKindAsr:
			switch cfg.Tenant.Kind {
			case string(apitypes.VoiceProviderKindVolcTenant):
				return b.buildVolcASR(cfg)
			default:
				return nil, fmt.Errorf("%w: model transformer provider %q", ErrUnsupported, cfg.Tenant.Kind)
			}
		case apitypes.ModelKindRealtime:
			switch cfg.Tenant.Kind {
			case string(apitypes.VoiceProviderKindVolcTenant):
				return b.buildVolcRealtime(cfg)
			default:
				return nil, fmt.Errorf("%w: realtime transformer provider %q", ErrUnsupported, cfg.Tenant.Kind)
			}
		case apitypes.ModelKindTranslation:
			switch cfg.Tenant.Kind {
			case string(apitypes.VoiceProviderKindVolcTenant):
				return b.buildVolcASTTranslate(cfg)
			default:
				return nil, fmt.Errorf("%w: translation transformer provider %q", ErrUnsupported, cfg.Tenant.Kind)
			}
		default:
			return nil, fmt.Errorf("%w: model transformer kind %q", ErrUnsupported, cfg.Model.Kind)
		}
	}
	return nil, fmt.Errorf("%w: transformer config has no model or voice", ErrInvalid)
}

func (b DefaultBuilder) buildOpenAIGenerator(cfg GeneratorConfig) (genx.Generator, error) {
	if cfg.Tenant.OpenAI == nil {
		return nil, fmt.Errorf("%w: openai tenant is required", ErrInvalid)
	}
	body, err := cfg.Credential.Body.AsOpenAICredentialBody()
	if err != nil {
		return nil, err
	}
	apiKey := firstString(body.ApiKey, body.Token)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: credential %q missing api_key", ErrInvalid, cfg.Credential.Name)
	}
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL := firstString(cfg.Tenant.OpenAI.BaseUrl, body.BaseUrl); baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	if b.HTTPClient != nil {
		opts = append(opts, option.WithHTTPClient(b.HTTPClient))
	}
	client := openai.NewClient(opts...)

	var providerData apitypes.OpenAITenantModelProviderData
	if cfg.Model.ProviderData != nil {
		providerData, err = cfg.Model.ProviderData.AsOpenAITenantModelProviderData()
		if err != nil {
			return nil, fmt.Errorf("%w: decode openai model provider_data: %w", ErrInvalid, err)
		}
	}
	modelName := firstString(providerData.UpstreamModel, string(cfg.Model.Id))
	if modelName == "" {
		return nil, fmt.Errorf("%w: model %q missing upstream model", ErrInvalid, cfg.Model.Id)
	}
	caps := cfg.Model.Capabilities
	return &genx.OpenAIGenerator{
		Client:            &client,
		Model:             modelName,
		SupportJSONOutput: boolValue(providerData.SupportJsonOutput, capabilityBool(caps, "json")),
		SupportToolCalls:  boolValue(providerData.SupportToolCalls, capabilityBool(caps, "tools")),
		TextOnly:          boolValue(providerData.SupportTextOnly, capabilityBool(caps, "text")),
		PromptRole:        openAIPromptRole(providerData.UseSystemRole, capabilityBool(caps, "system")),
		ExtraFields:       openAIThinkingExtraFields(providerData),
	}, nil
}

func (b DefaultBuilder) buildVolcArkGenerator(cfg GeneratorConfig) (genx.Generator, error) {
	if cfg.Tenant.Volc == nil {
		return nil, fmt.Errorf("%w: volc tenant is required", ErrInvalid)
	}
	body, err := cfg.Credential.Body.AsVolcCredentialBody()
	if err != nil {
		return nil, err
	}
	apiKey := firstString(body.ArkApiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: credential %q missing ark_api_key for ark", ErrInvalid, cfg.Credential.Name)
	}
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	baseURL := firstString(cfg.Tenant.Volc.Endpoint, defaultVolcArkBaseURL)
	opts = append(opts, option.WithBaseURL(baseURL))
	if b.HTTPClient != nil {
		opts = append(opts, option.WithHTTPClient(b.HTTPClient))
	}
	client := openai.NewClient(opts...)

	var providerData apitypes.VolcTenantModelProviderData
	if cfg.Model.ProviderData != nil {
		providerData, err = cfg.Model.ProviderData.AsVolcTenantModelProviderData()
		if err != nil {
			return nil, fmt.Errorf("%w: decode volc model provider_data: %w", ErrInvalid, err)
		}
	}
	openAIData := openAIProviderDataFromVolc(providerData)
	modelName := firstString(providerData.UpstreamModel, string(cfg.Model.Id))
	if modelName == "" {
		return nil, fmt.Errorf("%w: model %q missing upstream model", ErrInvalid, cfg.Model.Id)
	}
	caps := cfg.Model.Capabilities
	return &genx.OpenAIGenerator{
		Client:            &client,
		Model:             modelName,
		SupportJSONOutput: boolValue(providerData.SupportJsonOutput, capabilityBool(caps, "json")),
		SupportToolCalls:  boolValue(providerData.SupportToolCalls, capabilityBool(caps, "tools")),
		TextOnly:          boolValue(providerData.SupportTextOnly, capabilityBool(caps, "text")),
		PromptRole:        openAIPromptRole(providerData.UseSystemRole, capabilityBool(caps, "system")),
		ExtraFields:       openAIThinkingExtraFields(openAIData),
	}, nil
}

func (b DefaultBuilder) buildGeminiGenerator(ctx context.Context, cfg GeneratorConfig) (genx.Generator, error) {
	if cfg.Tenant.Gemini == nil {
		return nil, fmt.Errorf("%w: gemini tenant is required", ErrInvalid)
	}
	body, err := cfg.Credential.Body.AsGeminiCredentialBody()
	if err != nil {
		return nil, err
	}
	apiKey := firstString(body.ApiKey, body.Token)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: credential %q missing api_key", ErrInvalid, cfg.Credential.Name)
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		return nil, err
	}
	var providerData apitypes.GeminiTenantModelProviderData
	if cfg.Model.ProviderData != nil {
		providerData, err = cfg.Model.ProviderData.AsGeminiTenantModelProviderData()
		if err != nil {
			return nil, fmt.Errorf("%w: decode gemini model provider_data: %w", ErrInvalid, err)
		}
	}
	modelName := firstString(providerData.UpstreamModel, string(cfg.Model.Id))
	if modelName == "" {
		return nil, fmt.Errorf("%w: model %q missing upstream model", ErrInvalid, cfg.Model.Id)
	}
	return &genx.GeminiGenerator{
		Client: client,
		Model:  modelName,
	}, nil
}

func (b DefaultBuilder) buildVolcASR(cfg TransformerConfig) (genx.Transformer, error) {
	if cfg.Tenant.Volc == nil || cfg.Model == nil {
		return nil, fmt.Errorf("%w: volc tenant and model are required", ErrInvalid)
	}
	var providerData apitypes.VolcTenantModelProviderData
	if cfg.Model.ProviderData != nil {
		var err error
		providerData, err = cfg.Model.ProviderData.AsVolcTenantModelProviderData()
		if err != nil {
			return nil, fmt.Errorf("%w: decode volc model provider_data: %w", ErrInvalid, err)
		}
	}
	clientOpts := []doubaospeech.Option{}
	resourceID := firstString(providerData.ResourceId)
	if resourceID == "" {
		resourceID = doubaospeech.ResourceASRStream
	}
	clientOpts = append(clientOpts, doubaospeech.WithResourceID(resourceID))
	appID, err := volcCredentialAppID(cfg.Credential)
	if err != nil {
		return nil, err
	}
	credentialBody, err := cfg.Credential.Body.AsVolcCredentialBody()
	if err != nil {
		return nil, err
	}
	if firstString(providerData.AuthMode) == "x-api-key" {
		apiKey := firstString(credentialBody.ArkApiKey)
		if apiKey == "" {
			return nil, fmt.Errorf("%w: credential %q missing ark_api_key for x-api-key auth", ErrInvalid, cfg.Credential.Name)
		}
		clientOpts = append(clientOpts, doubaospeech.WithAPIKey(apiKey))
	} else if accessKey := firstString(credentialBody.SpeechToken); accessKey != "" {
		clientOpts = append(clientOpts, doubaospeech.WithV2APIKey(accessKey, appID))
	} else {
		return nil, fmt.Errorf("%w: credential %q missing speech_token", ErrInvalid, cfg.Credential.Name)
	}
	data := mergeParams(nil, cfg.Params)
	opts := []transformers.DoubaoASRSAUCOption{}
	opts = append(opts, transformers.WithDoubaoASRSAUCResourceID(resourceID))
	if value := mapString(data, "format", "audio_format"); value != "" {
		opts = append(opts, transformers.WithDoubaoASRSAUCFormat(value))
	}
	if value, ok := mapInt(data, "sample_rate", "sampleRate", "rate"); ok {
		opts = append(opts, transformers.WithDoubaoASRSAUCSampleRate(value))
	}
	if value, ok := mapInt(data, "channels", "channel"); ok {
		opts = append(opts, transformers.WithDoubaoASRSAUCChannels(value))
	}
	if value, ok := mapInt(data, "bits"); ok {
		opts = append(opts, transformers.WithDoubaoASRSAUCBits(value))
	}
	if value := mapString(data, "language", "lang"); value != "" {
		opts = append(opts, transformers.WithDoubaoASRSAUCLanguage(value))
	}
	if value := mapString(data, "result_type", "resultType"); value != "" {
		opts = append(opts, transformers.WithDoubaoASRSAUCResultType(value))
	}
	if value, ok := mapBool(data, "emit_interim", "emitInterim", "interim"); ok {
		opts = append(opts, transformers.WithDoubaoASRSAUCEmitInterim(value))
	}
	if value, ok := mapBool(data, "realtime_pacing", "realtimePacing"); ok {
		opts = append(opts, transformers.WithDoubaoASRSAUCRealtimePacing(value))
	}
	client := doubaospeech.NewClient(appID, clientOpts...)
	return transformers.NewDoubaoASRSAUC(client, opts...), nil
}

func (b DefaultBuilder) buildVolcRealtime(cfg TransformerConfig) (genx.Transformer, error) {
	if cfg.Tenant.Volc == nil || cfg.Model == nil {
		return nil, fmt.Errorf("%w: volc tenant and model are required", ErrInvalid)
	}
	appID, err := volcCredentialAppID(cfg.Credential)
	if err != nil {
		return nil, err
	}
	credentialBody, err := cfg.Credential.Body.AsVolcCredentialBody()
	if err != nil {
		return nil, err
	}
	data := mergeParams(nil, cfg.Params)
	clientOpts := []doubaospeech.Option{doubaospeech.WithResourceID(doubaospeech.ResourceRealtime)}
	if resourceID := mapString(data, "resource_id"); resourceID != "" {
		clientOpts[0] = doubaospeech.WithResourceID(resourceID)
	}
	switch mapString(data, "auth_mode", "auth") {
	case "x-api-key", "api_key":
		apiKey := firstString(credentialBody.SpeechToken)
		if apiKey != "" {
			clientOpts = append(clientOpts, doubaospeech.WithAPIKey(apiKey))
			break
		}
		return nil, fmt.Errorf("%w: credential %q missing speech_token for doubao realtime x-api-key auth", ErrInvalid, cfg.Credential.Name)
	case "", "v2", "realtime-api-key", "access_key", "speech-api-key", "speech_api_key":
		accessKey := firstString(credentialBody.SpeechToken)
		if accessKey == "" {
			return nil, fmt.Errorf("%w: credential %q missing speech_token for doubao realtime", ErrInvalid, cfg.Credential.Name)
		}
		clientOpts = append(clientOpts, doubaospeech.WithRealtimeAPIKey(accessKey, doubaospeech.AppKeyRealtime))
	default:
		return nil, fmt.Errorf("%w: doubao realtime auth_mode %q", ErrUnsupported, mapString(data, "auth_mode", "auth"))
	}

	opts := []transformers.DoubaoRealtimeOption{}
	if value := mapString(data, "speaker", "voice"); value != "" {
		opts = append(opts, transformers.WithDoubaoRealtimeSpeaker(value))
	}
	if value := mapString(data, "bot_name"); value != "" {
		opts = append(opts, transformers.WithDoubaoRealtimeBotName(value))
	}
	if value := mapString(data, "system_role", "system_prompt"); value != "" {
		opts = append(opts, transformers.WithDoubaoRealtimeSystemRole(value))
	}
	if value, ok := mapInt(data, "vad_window_ms"); ok {
		opts = append(opts, transformers.WithDoubaoRealtimeVADWindow(value))
	}
	if value := mapString(data, "speaking_style"); value != "" {
		opts = append(opts, transformers.WithDoubaoRealtimeSpeakingStyle(value))
	}
	if value := mapString(data, "character_manifest"); value != "" {
		opts = append(opts, transformers.WithDoubaoRealtimeCharacterManifest(value))
	}
	if value := mapString(data, "upstream_model", "model"); value != "" {
		opts = append(opts, transformers.WithDoubaoRealtimeModel(value))
	}
	if value := mapString(data, "mode", "input_mode", "input"); value != "" {
		mode, err := doubaoRealtimeMode(value)
		if err != nil {
			return nil, err
		}
		opts = append(opts, transformers.WithDoubaoRealtimeMode(mode))
	}
	client := doubaospeech.NewClient(appID, clientOpts...)
	return transformers.NewDoubaoRealtime(client, opts...), nil
}

func (b DefaultBuilder) buildVolcASTTranslate(cfg TransformerConfig) (genx.Transformer, error) {
	if cfg.Tenant.Volc == nil || cfg.Model == nil {
		return nil, fmt.Errorf("%w: volc tenant and model are required", ErrInvalid)
	}
	appID, err := volcCredentialAppID(cfg.Credential)
	if err != nil {
		return nil, err
	}
	credentialBody, err := cfg.Credential.Body.AsVolcCredentialBody()
	if err != nil {
		return nil, err
	}
	var providerData apitypes.VolcTenantModelProviderData
	if cfg.Model.ProviderData != nil {
		providerData, err = cfg.Model.ProviderData.AsVolcTenantModelProviderData()
		if err != nil {
			return nil, fmt.Errorf("%w: decode volc model provider_data: %w", ErrInvalid, err)
		}
	}
	data := mergeParams(nil, cfg.Params)
	if err := normalizeVolcASTTranslateLanguagePair(data); err != nil {
		return nil, err
	}
	resourceID := firstString(mapString(data, "resource_id"), providerData.ResourceId, doubaospeech.ResourceASTTranslate)
	clientOpts := []doubaospeech.Option{doubaospeech.WithResourceID(resourceID)}
	switch mapString(data, "auth_mode", "auth") {
	case "x-api-key", "api_key":
		apiKey := firstString(credentialBody.ArkApiKey)
		if apiKey == "" {
			return nil, fmt.Errorf("%w: credential %q missing ark_api_key for doubao ast translate x-api-key auth", ErrInvalid, cfg.Credential.Name)
		}
		clientOpts = append(clientOpts, doubaospeech.WithAPIKey(apiKey))
	case "", "v2", "access_key":
		accessKey := firstString(credentialBody.SpeechToken, credentialBody.OpenapiAccessKeyId)
		if accessKey == "" {
			return nil, fmt.Errorf("%w: credential %q missing speech_token or openapi_access_key_id for doubao ast translate", ErrInvalid, cfg.Credential.Name)
		}
		clientOpts = append(clientOpts, doubaospeech.WithV2APIKey(accessKey, ""))
	default:
		return nil, fmt.Errorf("%w: doubao ast translate auth_mode %q", ErrUnsupported, mapString(data, "auth_mode", "auth"))
	}

	opts := []transformers.DoubaoASTTranslateOption{
		transformers.WithDoubaoASTTranslateResourceID(resourceID),
	}
	if value := mapString(data, "mode"); value != "" {
		mode, err := doubaoASTTranslateMode(value)
		if err != nil {
			return nil, err
		}
		opts = append(opts, transformers.WithDoubaoASTTranslateMode(mode))
	}
	if value := mapString(data, "source_language", "source"); value != "" {
		opts = append(opts, transformers.WithDoubaoASTTranslateSourceLanguage(value))
	}
	if value := mapString(data, "target_language", "target"); value != "" {
		opts = append(opts, transformers.WithDoubaoASTTranslateTargetLanguage(value))
	}
	if value := mapString(data, "speaker_id", "speaker"); value != "" {
		opts = append(opts, transformers.WithDoubaoASTTranslateSpeakerID(value))
	}
	if value, ok := mapBool(data, "is_custom_speaker", "custom_speaker"); ok {
		opts = append(opts, transformers.WithDoubaoASTTranslateCustomSpeaker(value))
	}
	if value := mapString(data, "tts_resource_id"); value != "" {
		opts = append(opts, transformers.WithDoubaoASTTranslateTTSResourceID(value))
	}
	if value, ok := mapInt(data, "speech_rate"); ok {
		opts = append(opts, transformers.WithDoubaoASTTranslateSpeechRate(value))
	}
	if value, ok := mapBool(data, "enable_source_language_detect", "source_language_detect"); ok {
		opts = append(opts, transformers.WithDoubaoASTTranslateSourceLanguageDetect(value))
	}
	if value, ok := mapBool(data, "denoise"); ok {
		opts = append(opts, transformers.WithDoubaoASTTranslateDenoise(value))
	}
	client := doubaospeech.NewClient(appID, clientOpts...)
	return transformers.NewDoubaoASTTranslate(client, opts...), nil
}

func normalizeVolcASTTranslateLanguagePair(data map[string]any) error {
	if data == nil {
		return nil
	}
	pair := mapString(data, "lang_pair", "language_pair")
	source, target, auto, err := volcASTTranslateLanguagesFromPair(pair)
	if err != nil {
		return fmt.Errorf("%w: doubao ast translate lang_pair %q: %w", ErrInvalid, pair, err)
	}
	if source != "" && target != "" {
		data["source_language"] = source
		data["target_language"] = target
		delete(data, "lang_pair")
		delete(data, "language_pair")
	}
	if auto {
		data["enable_source_language_detect"] = true
	}
	return nil
}

func volcASTTranslateLanguagesFromPair(pair string) (source string, target string, auto bool, err error) {
	pair = strings.ToLower(strings.TrimSpace(pair))
	switch pair {
	case "":
		return "", "", false, nil
	case "auto":
		return "zhen", "zhen", true, nil
	}
	parts := strings.Split(pair, "/")
	if len(parts) != 2 {
		return "", "", false, fmt.Errorf("expected source/target or auto")
	}
	source = normalizeVolcASTTranslateLanguageCode(parts[0])
	target = normalizeVolcASTTranslateLanguageCode(parts[1])
	if source == "" || target == "" {
		return "", "", false, fmt.Errorf("source and target must be non-empty")
	}
	if source == "zhen" || target == "zhen" {
		return "", "", false, fmt.Errorf("zhen is only available through auto")
	}
	return source, target, false, nil
}

func normalizeVolcASTTranslateLanguageCode(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "jp":
		return "ja"
	default:
		return strings.ToLower(strings.TrimSpace(language))
	}
}

func doubaoASTTranslateMode(mode string) (doubaospeech.ASTTranslateMode, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "s2t", "speech-to-text", "speech_to_text":
		return doubaospeech.ASTTranslateModeS2T, nil
	case "s2s", "speech-to-speech", "speech_to_speech":
		return doubaospeech.ASTTranslateModeS2S, nil
	default:
		return "", fmt.Errorf("%w: doubao ast translate mode %q", ErrUnsupported, mode)
	}
}

func (b DefaultBuilder) buildVolcTTS(cfg TransformerConfig) (genx.Transformer, error) {
	if cfg.Tenant.Volc == nil || cfg.Voice == nil {
		return nil, fmt.Errorf("%w: volc tenant and voice are required", ErrInvalid)
	}
	appID, err := volcCredentialAppID(cfg.Credential)
	if err != nil {
		return nil, err
	}
	credentialBody, err := cfg.Credential.Body.AsVolcCredentialBody()
	if err != nil {
		return nil, err
	}
	token := firstString(credentialBody.SpeechToken)
	if token == "" {
		return nil, fmt.Errorf("%w: credential %q missing speech_token", ErrInvalid, cfg.Credential.Name)
	}
	var providerData apitypes.VolcTenantVoiceProviderData
	if cfg.Voice.ProviderData != nil {
		providerData, err = cfg.Voice.ProviderData.AsVolcTenantVoiceProviderData()
		if err != nil {
			return nil, fmt.Errorf("%w: decode volc voice provider_data: %w", ErrInvalid, err)
		}
	}
	voiceID := firstString(providerData.VoiceId)
	if voiceID == "" {
		return nil, fmt.Errorf("%w: voice %q missing voice_id", ErrInvalid, cfg.Voice.Id)
	}
	opts := []transformers.DoubaoTTSSeedV2Option{
		transformers.WithDoubaoTTSSeedV2Format(defaultVolcTTSAudioFormat),
		transformers.WithDoubaoTTSSeedV2SampleRate(defaultTTSAudioSampleRate),
	}
	if value := firstString(providerData.ResourceId); value != "" {
		opts = append(opts, transformers.WithDoubaoTTSSeedV2ResourceID(value))
	}
	client := doubaospeech.NewClient(appID, doubaospeech.WithBearerToken(token))
	return transformers.NewDoubaoTTSSeedV2(client, voiceID, opts...), nil
}

func (b DefaultBuilder) buildMiniMaxTTS(cfg TransformerConfig) (genx.Transformer, error) {
	if cfg.Tenant.MiniMax == nil || cfg.Voice == nil {
		return nil, fmt.Errorf("%w: minimax tenant and voice are required", ErrInvalid)
	}
	body, err := cfg.Credential.Body.AsMiniMaxCredentialBody()
	if err != nil {
		return nil, err
	}
	apiKey := firstString(body.ApiKey, body.Token)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: credential %q missing api_key", ErrInvalid, cfg.Credential.Name)
	}
	var providerData apitypes.MiniMaxTenantVoiceProviderData
	if cfg.Voice.ProviderData != nil {
		providerData, err = cfg.Voice.ProviderData.AsMiniMaxTenantVoiceProviderData()
		if err != nil {
			return nil, fmt.Errorf("%w: decode minimax voice provider_data: %w", ErrInvalid, err)
		}
	}
	voiceID := firstString(providerData.VoiceId)
	if voiceID == "" {
		return nil, fmt.Errorf("%w: voice %q missing voice_id", ErrInvalid, cfg.Voice.Id)
	}
	clientConfig := minimax.Config{
		APIKey:  apiKey,
		BaseURL: firstString(cfg.Tenant.MiniMax.BaseUrl, body.BaseUrl, defaultMiniMaxBaseURL),
	}
	client, err := minimax.NewClient(clientConfig)
	if err != nil {
		return nil, err
	}
	opts := []transformers.MinimaxTTSOption{
		transformers.WithMinimaxTTSFormat(defaultMiniMaxTTSAudioFormat),
		transformers.WithMinimaxTTSSampleRate(defaultTTSAudioSampleRate),
	}
	if model := firstString(providerData.Model); model != "" {
		opts = append(opts, transformers.WithMinimaxTTSModel(model))
	}
	if format := firstString(providerData.Format); format != "" {
		opts = append(opts, transformers.WithMinimaxTTSFormat(format))
	}
	if providerData.SampleRate != nil {
		opts = append(opts, transformers.WithMinimaxTTSSampleRate(*providerData.SampleRate))
	}
	return transformers.NewMinimaxTTS(client, voiceID, opts...), nil
}

func volcCredentialAppID(credential apitypes.Credential) (string, error) {
	body, err := credential.Body.AsVolcCredentialBody()
	if err != nil {
		return "", err
	}
	appID := firstString(body.AppId)
	if appID == "" {
		return "", fmt.Errorf("%w: credential %q missing app_id", ErrInvalid, credential.Name)
	}
	return appID, nil
}

func firstString(values ...any) string {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case *string:
			if typed != nil && strings.TrimSpace(*typed) != "" {
				return strings.TrimSpace(*typed)
			}
		}
	}
	return ""
}

func doubaoRealtimeMode(value string) (transformers.DoubaoRealtimeMode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "push-to-talk", "push_to_talk", "ptt":
		return transformers.DoubaoRealtimeModePushToTalk, nil
	case "realtime", "real-time", "real_time", "default":
		return transformers.DoubaoRealtimeModeRealtime, nil
	case "text":
		return transformers.DoubaoRealtimeModeText, nil
	default:
		return "", fmt.Errorf("%w: doubao realtime mode %q", ErrUnsupported, value)
	}
}

func mapString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		switch value := values[key].(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		case fmt.Stringer:
			if text := strings.TrimSpace(value.String()); text != "" {
				return text
			}
		}
	}
	return ""
}

func mapInt(values map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		switch value := values[key].(type) {
		case int:
			return value, true
		case int32:
			return int(value), true
		case int64:
			return int(value), true
		case float64:
			return int(value), true
		case json.Number:
			n, err := value.Int64()
			return int(n), err == nil
		}
	}
	return 0, false
}

func mergeParams(base, overrides map[string]any) map[string]any {
	if len(base) == 0 && len(overrides) == 0 {
		return nil
	}
	out := make(map[string]any, len(base)+len(overrides))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range overrides {
		out[key] = value
	}
	return out
}

func mapBool(values map[string]any, keys ...string) (bool, bool) {
	for _, key := range keys {
		switch value := values[key].(type) {
		case bool:
			return value, true
		case string:
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "true", "1", "yes", "y", "on":
				return true, true
			case "false", "0", "no", "n", "off":
				return false, true
			}
		}
	}
	return false, false
}

func boolValue(values ...*bool) bool {
	for _, value := range values {
		if value != nil {
			return *value
		}
	}
	return false
}

func capabilityBool(caps *apitypes.ModelCapabilities, name string) *bool {
	if caps == nil {
		return nil
	}
	switch name {
	case "json":
		return caps.JsonOutput
	case "tools":
		return caps.ToolCalls
	case "text":
		return caps.TextOnly
	case "system":
		return caps.SystemRole
	default:
		return nil
	}
}

func openAIPromptRole(values ...*bool) genx.PromptRole {
	if boolValue(values...) {
		return genx.PromptRoleSystem
	}
	return ""
}

func openAIThinkingExtraFields(data apitypes.OpenAITenantModelProviderData) map[string]any {
	param := firstString(data.ThinkingParam, data.ThinkingLevelParam)
	level := firstString(data.DefaultThinkingLevel)
	if param == "" || level == "" {
		return nil
	}
	out := map[string]any{}
	setNestedExtraField(out, param, openAIThinkingValue(param, level))
	return out
}

func openAIProviderDataFromVolc(data apitypes.VolcTenantModelProviderData) apitypes.OpenAITenantModelProviderData {
	return apitypes.OpenAITenantModelProviderData{
		DefaultThinkingLevel: data.DefaultThinkingLevel,
		SupportJsonOutput:    data.SupportJsonOutput,
		SupportTextOnly:      data.SupportTextOnly,
		SupportThinking:      data.SupportThinking,
		SupportToolCalls:     data.SupportToolCalls,
		ThinkingLevelParam:   data.ThinkingLevelParam,
		ThinkingLevels:       data.ThinkingLevels,
		ThinkingParam:        data.ThinkingParam,
		UpstreamModel:        data.UpstreamModel,
		UseSystemRole:        data.UseSystemRole,
	}
}

func openAIThinkingValue(param, level string) any {
	if strings.EqualFold(strings.TrimSpace(param), "enable_thinking") {
		return !isDisabledThinkingLevel(level)
	}
	return level
}

func isDisabledThinkingLevel(level string) bool {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "disabled", "disable", "off", "false", "0", "none", "no":
		return true
	default:
		return false
	}
}

func setNestedExtraField(out map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return
	}
	current := out
	for _, raw := range parts[:len(parts)-1] {
		part := strings.TrimSpace(raw)
		if part == "" {
			return
		}
		next, _ := current[part].(map[string]any)
		if next == nil {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
	last := strings.TrimSpace(parts[len(parts)-1])
	if last != "" {
		current[last] = value
	}
}
