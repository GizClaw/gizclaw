package peergenx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	dashscope "github.com/GizClaw/dashscope-realtime-go"
	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/minimax-go"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"google.golang.org/genai"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
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
				if cfg.Agent {
					return b.buildVolcRealtimeAgent(cfg)
				}
				return b.buildVolcRealtime(cfg)
			case string(apitypes.ModelProviderKindDashscopeTenant):
				if cfg.Agent {
					return b.buildDashScopeRealtimeAgent(cfg)
				}
				return nil, fmt.Errorf("%w: dashscope realtime is only available through an Agent runtime", ErrUnsupported)
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

func (b DefaultBuilder) buildVolcRealtimeAgent(cfg TransformerConfig) (genx.Transformer, error) {
	if cfg.Tenant.Volc == nil || cfg.Model == nil {
		return nil, fmt.Errorf("%w: volc tenant and model are required", ErrInvalid)
	}
	var providerData apitypes.VolcTenantModelProviderData
	if cfg.Model.ProviderData != nil {
		var err error
		providerData, err = cfg.Model.ProviderData.AsVolcTenantModelProviderData()
		if err != nil {
			return nil, fmt.Errorf("%w: decode volc realtime model provider_data: %w", ErrInvalid, err)
		}
	}
	body, err := cfg.Credential.Body.AsVolcCredentialBody()
	if err != nil {
		return nil, err
	}
	apiKey := firstString(body.SpeechApiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: credential %q missing speech_api_key for doubao realtime", ErrInvalid, cfg.Credential.Name)
	}
	appID := firstString(body.SpeechAppId)
	if appID == "" {
		return nil, fmt.Errorf("%w: credential %q missing speech_app_id for doubao realtime", ErrInvalid, cfg.Credential.Name)
	}
	data := mergeParams(nil, cfg.Params)
	resourceID := firstString(mapString(data, "resource_id"), providerData.ResourceId, doubaospeech.ResourceRealtime)
	client := doubaospeech.NewClient(appID,
		doubaospeech.WithResourceID(resourceID),
		doubaospeech.WithAPIKey(apiKey),
	)
	opts := []transformers.DoubaoRealtimeDuplexOption{
		transformers.WithDoubaoRealtimeDuplexModel(doubaospeech.RealtimeDuplexModelDefault),
	}
	if value := mapString(data, "instructions", "system_role"); value != "" {
		opts = append(opts, transformers.WithDoubaoRealtimeDuplexInstructions(value))
	}
	if value := mapString(data, "dialog_id", "session_id"); value != "" {
		opts = append(opts, transformers.WithDoubaoRealtimeDuplexSessionID(value))
	}
	if value := mapString(data, "output_voice", "voice", "speaker"); value != "" {
		opts = append(opts, transformers.WithDoubaoRealtimeDuplexSpeaker(value))
	}
	if value := mapString(data, "output_format", "format"); value != "" {
		opts = append(opts, transformers.WithDoubaoRealtimeDuplexFormat(value))
	}
	if value, ok := mapInt(data, "output_sample_rate", "sample_rate"); ok {
		opts = append(opts, transformers.WithDoubaoRealtimeDuplexSampleRate(value))
	}
	if value, ok := mapInt(data, "output_speed", "speech_rate", "speed"); ok {
		opts = append(opts, transformers.WithDoubaoRealtimeDuplexOutputSpeed(value))
	}
	if value, ok := mapInt(data, "output_loudness", "loudness_rate", "loudness"); ok {
		opts = append(opts, transformers.WithDoubaoRealtimeDuplexOutputLoudness(value))
	}
	if value := mapString(data, "input_format"); value != "" {
		opts = append(opts, transformers.WithDoubaoRealtimeDuplexInputFormat(value))
	}
	if value, ok := mapInt(data, "input_sample_rate"); ok {
		opts = append(opts, transformers.WithDoubaoRealtimeDuplexInputSampleRate(value))
	}
	if value, ok := mapInt(data, "input_channels"); ok {
		opts = append(opts, transformers.WithDoubaoRealtimeDuplexInputChannels(value))
	}
	if value, ok := mapBool(data, "input_transcode"); ok {
		opts = append(opts, transformers.WithDoubaoRealtimeDuplexInputTranscode(value))
	}
	extension, err := doubaoRealtimeDuplexExtension(data)
	if err != nil {
		return nil, err
	}
	if extension != nil {
		opts = append(opts, transformers.WithDoubaoRealtimeDuplexExtension(extension))
	}
	return transformers.NewDoubaoRealtimeDuplexRealtime(client, opts...), nil
}

func (b DefaultBuilder) buildDashScopeRealtimeAgent(cfg TransformerConfig) (genx.Transformer, error) {
	if cfg.Tenant.DashScope == nil || cfg.Model == nil {
		return nil, fmt.Errorf("%w: dashscope tenant and model are required", ErrInvalid)
	}
	var providerData apitypes.DashScopeTenantModelProviderData
	if cfg.Model.ProviderData != nil {
		var err error
		providerData, err = cfg.Model.ProviderData.AsDashScopeTenantModelProviderData()
		if err != nil {
			return nil, fmt.Errorf("%w: decode dashscope realtime model provider_data: %w", ErrInvalid, err)
		}
	}
	body, err := cfg.Credential.Body.AsDashScopeCredentialBody()
	if err != nil {
		return nil, err
	}
	apiKey := firstString(body.ApiKey, body.Token)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: credential %q missing api_key for dashscope realtime", ErrInvalid, cfg.Credential.Name)
	}
	clientOpts := []dashscope.Option{}
	if baseURL := firstString(cfg.Tenant.DashScope.BaseUrl, body.BaseUrl); baseURL != "" {
		clientOpts = append(clientOpts, dashscope.WithBaseURL(baseURL))
	}
	if b.HTTPClient != nil {
		clientOpts = append(clientOpts, dashscope.WithHTTPClient(b.HTTPClient))
	}
	data := mergeParams(nil, cfg.Params)
	modelName := firstString(mapString(data, "upstream_model", "model"), providerData.UpstreamModel, string(cfg.Model.Id))
	opts := []transformers.DashScopeRealtimeOption{transformers.WithDashScopeRealtimeModel(modelName)}
	if value := mapString(data, "instructions", "system_prompt"); value != "" {
		opts = append(opts, transformers.WithDashScopeRealtimeInstructions(value))
	}
	if value := mapString(data, "output_voice", "voice"); value != "" {
		opts = append(opts, transformers.WithDashScopeRealtimeVoice(value))
	}
	if value := mapString(data, "input_format"); value != "" {
		opts = append(opts, transformers.WithDashScopeRealtimeInputAudioFormat(value))
	}
	if value := mapString(data, "output_format"); value != "" {
		opts = append(opts, transformers.WithDashScopeRealtimeOutputAudioFormat(value))
	}
	return transformers.NewDashScopeRealtime(dashscope.NewClient(apiKey, clientOpts...), opts...), nil
}

func doubaoRealtimeDuplexExtension(data map[string]any) (*doubaospeech.RealtimeDuplexExtension, error) {
	extension, err := doubaoRealtimeExtension(data)
	if err != nil || extension == nil {
		return nil, err
	}
	raw, err := json.Marshal(extension)
	if err != nil {
		return nil, fmt.Errorf("%w: encode doubao realtime duplex extension: %w", ErrInvalid, err)
	}
	var duplex doubaospeech.RealtimeDuplexExtension
	if err := json.Unmarshal(raw, &duplex); err != nil {
		return nil, fmt.Errorf("%w: decode doubao realtime duplex extension: %w", ErrInvalid, err)
	}
	return &duplex, nil
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
		return nil, fmt.Errorf("%w: credential %q missing ark_api_key for volc ark", ErrInvalid, cfg.Credential.Name)
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
	credentialBody, err := cfg.Credential.Body.AsVolcCredentialBody()
	if err != nil {
		return nil, err
	}
	apiKey := firstString(credentialBody.SpeechApiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: credential %q missing speech_api_key for doubao asr", ErrInvalid, cfg.Credential.Name)
	}
	appID := firstString(credentialBody.SpeechAppId)
	if appID == "" {
		return nil, fmt.Errorf("%w: credential %q missing speech_app_id for doubao asr", ErrInvalid, cfg.Credential.Name)
	}
	clientOpts = append(clientOpts, doubaospeech.WithAPIKey(apiKey))
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
	var providerData apitypes.VolcTenantModelProviderData
	if cfg.Model.ProviderData != nil {
		var err error
		providerData, err = cfg.Model.ProviderData.AsVolcTenantModelProviderData()
		if err != nil {
			return nil, fmt.Errorf("%w: decode volc realtime model provider_data: %w", ErrInvalid, err)
		}
	}
	credentialBody, err := cfg.Credential.Body.AsVolcCredentialBody()
	if err != nil {
		return nil, err
	}
	data := mergeParams(nil, cfg.Params)
	clientOpts := []doubaospeech.Option{doubaospeech.WithResourceID(doubaospeech.ResourceRealtime)}
	if resourceID := firstString(mapString(data, "resource_id"), providerData.ResourceId); resourceID != "" {
		clientOpts[0] = doubaospeech.WithResourceID(resourceID)
	}
	apiKey := firstString(credentialBody.SpeechApiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: credential %q missing speech_api_key for doubao realtime", ErrInvalid, cfg.Credential.Name)
	}
	appID := firstString(credentialBody.SpeechAppId)
	if appID == "" {
		return nil, fmt.Errorf("%w: credential %q missing speech_app_id for doubao realtime", ErrInvalid, cfg.Credential.Name)
	}
	clientOpts = append(clientOpts, doubaospeech.WithAPIKey(apiKey))

	modelName := firstString(mapString(data, "upstream_model", "model"), providerData.UpstreamModel)
	if modelName == "" {
		return nil, fmt.Errorf("%w: model %q missing upstream_model for doubao realtime", ErrInvalid, cfg.Model.Id)
	}
	mode := transformers.DoubaoRealtimeModePushToTalk
	if value := mapString(data, "mode", "input_mode", "input"); value != "" {
		parsed, err := doubaoRealtimeMode(value)
		if err != nil {
			return nil, err
		}
		mode = parsed
	}

	client := doubaospeech.NewClient(appID, clientOpts...)
	opts := []transformers.DoubaoRealtimeOption{
		transformers.WithDoubaoRealtimeModel(modelName),
		transformers.WithDoubaoRealtimeMode(mode),
	}
	if value := mapString(data, "instructions", "system_role"); value != "" {
		opts = append(opts, transformers.WithDoubaoRealtimeSystemRole(value))
	}
	if value := mapString(data, "dialog_id"); value != "" {
		opts = append(opts, transformers.WithDoubaoRealtimeDialogID(value))
	}
	extension, err := doubaoRealtimeExtension(data)
	if err != nil {
		return nil, err
	}
	if asrExtra := doubaoRealtimeASRExtra(extension); asrExtra != nil {
		opts = append(opts, transformers.WithDoubaoRealtimeASRExtra(*asrExtra))
	}
	if ttsExtra := doubaoRealtimeTTSExtra(extension); ttsExtra != nil {
		opts = append(opts, transformers.WithDoubaoRealtimeTTSExtra(*ttsExtra))
	}
	dialogExtra := doubaoRealtimeDialogExtra(extension)
	if dialogExtra != nil {
		opts = append(opts, transformers.WithDoubaoRealtimeDialogExtra(*dialogExtra))
		if doubaoRealtimeWebsearchEnabled(dialogExtra) {
			searchAPIKey := firstString(credentialBody.SearchApiKey)
			if searchAPIKey == "" {
				return nil, fmt.Errorf("%w: credential %q missing search_api_key for doubao realtime web search", ErrInvalid, cfg.Credential.Name)
			}
			opts = append(opts, transformers.WithDoubaoRealtimeSearchAPIKey(searchAPIKey))
		}
	}
	if value := mapString(data, "output_voice", "voice", "speaker"); value != "" {
		opts = append(opts, transformers.WithDoubaoRealtimeSpeaker(value))
	}
	if value := mapString(data, "output_format", "format"); value != "" {
		opts = append(opts, transformers.WithDoubaoRealtimeFormat(value))
	}
	if value, ok := mapInt(data, "output_sample_rate", "sample_rate"); ok {
		opts = append(opts, transformers.WithDoubaoRealtimeSampleRate(value))
	}
	if value, ok := mapInt(data, "output_speed", "speech_rate", "speed"); ok {
		opts = append(opts, transformers.WithDoubaoRealtimeSpeechRate(value))
	}
	if value, ok := mapInt(data, "output_loudness", "loudness_rate", "loudness"); ok {
		opts = append(opts, transformers.WithDoubaoRealtimeLoudnessRate(value))
	}
	if value := mapString(data, "input_format"); value != "" {
		opts = append(opts, transformers.WithDoubaoRealtimeInputFormat(value))
	}
	if value, ok := mapInt(data, "input_sample_rate"); ok {
		opts = append(opts, transformers.WithDoubaoRealtimeInputSampleRate(value))
	}
	if value, ok := mapInt(data, "input_channels"); ok {
		opts = append(opts, transformers.WithDoubaoRealtimeInputChannels(value))
	}
	if value, ok := mapBool(data, "input_transcode"); ok {
		opts = append(opts, transformers.WithDoubaoRealtimeInputTranscode(value))
	}
	if value := mapString(data, "bot_name"); value != "" {
		opts = append(opts, transformers.WithDoubaoRealtimeBotName(value))
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
	return transformers.NewDoubaoRealtime(client, opts...), nil
}

func doubaoRealtimeExtension(data map[string]any) (*apitypes.DoubaoRealtimeExtension, error) {
	raw, ok := data["extension"]
	if !ok || raw == nil {
		return nil, nil
	}
	var extension apitypes.DoubaoRealtimeExtension
	switch typed := raw.(type) {
	case apitypes.DoubaoRealtimeExtension:
		extension = typed
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil, nil
		}
		if err := json.Unmarshal([]byte(typed), &extension); err != nil {
			return nil, fmt.Errorf("%w: decode doubao realtime extension: %w", ErrInvalid, err)
		}
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return nil, fmt.Errorf("%w: encode doubao realtime extension: %w", ErrInvalid, err)
		}
		if err := json.Unmarshal(data, &extension); err != nil {
			return nil, fmt.Errorf("%w: decode doubao realtime extension: %w", ErrInvalid, err)
		}
	}
	return &extension, nil
}

func doubaoRealtimeASRExtra(extension *apitypes.DoubaoRealtimeExtension) *doubaospeech.RealtimeASRExtra {
	if extension == nil || extension.Asr == nil || extension.Asr.Extra == nil {
		return nil
	}
	extra := extension.Asr.Extra
	out := &doubaospeech.RealtimeASRExtra{
		BoostingTableID:       firstString(extra.BoostingTableId),
		BoostingTableName:     firstString(extra.BoostingTableName),
		RegexCorrectTableID:   firstString(extra.RegexCorrectTableId),
		RegexCorrectTableName: firstString(extra.RegexCorrectTableName),
	}
	if extra.EndSmoothWindowMs != nil {
		out.EndSmoothWindowMS = *extra.EndSmoothWindowMs
	}
	if extra.EnableCustomVad != nil {
		value := *extra.EnableCustomVad
		out.EnableCustomVAD = &value
	}
	if extra.EnableAsrTwopass != nil {
		value := *extra.EnableAsrTwopass
		out.EnableASRTwopass = &value
	}
	if extra.Context != nil {
		out.Context = &doubaospeech.RealtimeASRContext{}
		if extra.Context.Hotwords != nil {
			for _, hotword := range *extra.Context.Hotwords {
				out.Context.Hotwords = append(out.Context.Hotwords, doubaospeech.RealtimeHotword{Word: hotword.Word})
			}
		}
		if extra.Context.CorrectWords != nil {
			out.Context.CorrectWords = make(map[string]string, len(*extra.Context.CorrectWords))
			for key, value := range *extra.Context.CorrectWords {
				out.Context.CorrectWords[key] = value
			}
		}
	}
	return out
}

func doubaoRealtimeTTSExtra(extension *apitypes.DoubaoRealtimeExtension) *doubaospeech.RealtimeTTSExtra {
	if extension == nil || extension.Tts == nil || extension.Tts.Extra == nil {
		return nil
	}
	extra := extension.Tts.Extra
	out := &doubaospeech.RealtimeTTSExtra{
		ExplicitDialect: firstString(extra.ExplicitDialect),
		TTS20Model:      firstString(extra.Tts20Model),
	}
	if extra.AigcMetadata != nil {
		out.AIGCMetadata = &doubaospeech.RealtimeAIGCMetadata{
			ContentProducer:   firstString(extra.AigcMetadata.ContentProducer),
			ProduceID:         firstString(extra.AigcMetadata.ProduceId),
			ContentPropagator: firstString(extra.AigcMetadata.ContentPropagator),
			PropagateID:       firstString(extra.AigcMetadata.PropagateId),
		}
		if extra.AigcMetadata.Enable != nil {
			value := *extra.AigcMetadata.Enable
			out.AIGCMetadata.Enable = &value
		}
	}
	return out
}

func doubaoRealtimeDialogExtra(extension *apitypes.DoubaoRealtimeExtension) *doubaospeech.RealtimeDialogExtra {
	if extension == nil || extension.Dialog == nil || extension.Dialog.Extra == nil {
		return nil
	}
	extra := extension.Dialog.Extra
	out := &doubaospeech.RealtimeDialogExtra{
		AuditResponse:                firstString(extra.AuditResponse),
		VolcWebsearchBotID:           firstString(extra.VolcWebsearchBotId),
		VolcWebsearchNoResultMessage: firstString(extra.VolcWebsearchNoResultMessage),
	}
	if extra.VolcWebsearchType != nil {
		out.VolcWebsearchType = string(*extra.VolcWebsearchType)
	}
	if extra.EnableVolcWebsearch != nil {
		value := *extra.EnableVolcWebsearch
		out.EnableVolcWebsearch = &value
	}
	if extra.EnableMusic != nil {
		value := *extra.EnableMusic
		out.EnableMusic = &value
	}
	if extra.EnableLoudnessNorm != nil {
		value := *extra.EnableLoudnessNorm
		out.EnableLoudnessNorm = &value
	}
	if extra.VolcWebsearchResultCount != nil {
		out.VolcWebsearchResultCount = *extra.VolcWebsearchResultCount
	}
	if extra.StrictAudit != nil {
		value := *extra.StrictAudit
		out.StrictAudit = &value
	}
	if extra.EnableConversationTruncate != nil {
		value := *extra.EnableConversationTruncate
		out.EnableConversationTruncate = &value
	}
	if extra.EnableUserQueryExit != nil {
		value := *extra.EnableUserQueryExit
		out.EnableUserQueryExit = &value
	}
	return out
}

func doubaoRealtimeWebsearchEnabled(extra *doubaospeech.RealtimeDialogExtra) bool {
	return extra != nil && extra.EnableVolcWebsearch != nil && *extra.EnableVolcWebsearch
}

func (b DefaultBuilder) buildVolcASTTranslate(cfg TransformerConfig) (genx.Transformer, error) {
	if cfg.Tenant.Volc == nil || cfg.Model == nil {
		return nil, fmt.Errorf("%w: volc tenant and model are required", ErrInvalid)
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
	apiKey := firstString(credentialBody.SpeechApiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: credential %q missing speech_api_key for doubao ast translate", ErrInvalid, cfg.Credential.Name)
	}
	appID := firstString(credentialBody.SpeechAppId)
	if appID == "" {
		return nil, fmt.Errorf("%w: credential %q missing speech_app_id for doubao ast translate", ErrInvalid, cfg.Credential.Name)
	}
	clientOpts = append(clientOpts, doubaospeech.WithAPIKey(apiKey))

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
	if value, ok := mapBool(data, "realtime_pacing", "realtimePacing"); ok {
		opts = append(opts, transformers.WithDoubaoASTTranslateRealtimePacing(value))
	}
	if value := mapString(data, "input", "input_mode"); value != "" {
		inputMode, err := doubaoASTTranslateInputMode(value)
		if err != nil {
			return nil, err
		}
		opts = append(opts, transformers.WithDoubaoASTTranslateInputMode(inputMode))
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

func doubaoASTTranslateInputMode(value string) (transformers.DoubaoASTTranslateInputMode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "push-to-talk", "push_to_talk", "ptt", "default":
		return transformers.DoubaoASTTranslateInputModePushToTalk, nil
	case "realtime", "real-time", "real_time":
		return transformers.DoubaoASTTranslateInputModeRealtime, nil
	default:
		return "", fmt.Errorf("%w: doubao ast translate input mode %q", ErrUnsupported, value)
	}
}

func (b DefaultBuilder) buildVolcTTS(cfg TransformerConfig) (genx.Transformer, error) {
	if cfg.Tenant.Volc == nil || cfg.Voice == nil {
		return nil, fmt.Errorf("%w: volc tenant and voice are required", ErrInvalid)
	}
	credentialBody, err := cfg.Credential.Body.AsVolcCredentialBody()
	if err != nil {
		return nil, err
	}
	apiKey := firstString(credentialBody.SpeechApiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: credential %q missing speech_api_key for doubao tts", ErrInvalid, cfg.Credential.Name)
	}
	appID := firstString(credentialBody.SpeechAppId)
	if appID == "" {
		return nil, fmt.Errorf("%w: credential %q missing speech_app_id for doubao tts", ErrInvalid, cfg.Credential.Name)
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
	client := doubaospeech.NewClient(appID, doubaospeech.WithAPIKey(apiKey))
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
	case "push-to-talk", "push_to_talk", "ptt", "default":
		return transformers.DoubaoRealtimeModePushToTalk, nil
	case "realtime", "real-time", "real_time":
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
