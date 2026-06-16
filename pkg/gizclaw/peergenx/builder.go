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
	defaultTTSAudioFormat     = "mp3"
	defaultTTSAudioSampleRate = 16000
	defaultMiniMaxBaseURL     = "https://api.minimax.io"
)

func (b DefaultBuilder) BuildGenerator(ctx context.Context, cfg GeneratorConfig) (genx.Generator, error) {
	switch cfg.Tenant.Kind {
	case string(apitypes.ModelProviderKindOpenaiTenant):
		return b.buildOpenAIGenerator(cfg)
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
	apiKey := credentialString(cfg.Credential, "api_key", "token")
	if apiKey == "" {
		return nil, fmt.Errorf("%w: credential %q missing api_key", ErrInvalid, cfg.Credential.Name)
	}
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL := firstString(cfg.Tenant.OpenAI.BaseUrl, credentialBodyString(cfg.Credential.Body, "base_url")); baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	if b.HTTPClient != nil {
		opts = append(opts, option.WithHTTPClient(b.HTTPClient))
	}
	client := openai.NewClient(opts...)

	var providerData apitypes.OpenAITenantModelProviderData
	if err := decodeProviderData(cfg.Model.ProviderData, string(apitypes.ModelProviderKindOpenaiTenant), &providerData); err != nil {
		return nil, err
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
	}, nil
}

func (b DefaultBuilder) buildGeminiGenerator(ctx context.Context, cfg GeneratorConfig) (genx.Generator, error) {
	if cfg.Tenant.Gemini == nil {
		return nil, fmt.Errorf("%w: gemini tenant is required", ErrInvalid)
	}
	apiKey := credentialString(cfg.Credential, "api_key", "token")
	if apiKey == "" {
		return nil, fmt.Errorf("%w: credential %q missing api_key", ErrInvalid, cfg.Credential.Name)
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		return nil, err
	}
	var providerData apitypes.GeminiTenantModelProviderData
	if err := decodeProviderData(cfg.Model.ProviderData, string(apitypes.ModelProviderKindGeminiTenant), &providerData); err != nil {
		return nil, err
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
	var data map[string]any
	if err := decodeProviderData(cfg.Model.ProviderData, string(apitypes.VoiceProviderKindVolcTenant), &data); err != nil {
		return nil, err
	}
	clientOpts := []doubaospeech.Option{}
	resourceID := mapString(data, "resource_id")
	if resourceID == "" {
		resourceID = doubaospeech.ResourceASRStream
	}
	clientOpts = append(clientOpts, doubaospeech.WithResourceID(resourceID))
	if mapString(data, "auth_mode", "auth") == "x-api-key" {
		apiKey := credentialString(cfg.Credential, "api_key", "x_api_key")
		if apiKey == "" {
			return nil, fmt.Errorf("%w: credential %q missing api_key for x-api-key auth", ErrInvalid, cfg.Credential.Name)
		}
		clientOpts = append(clientOpts, doubaospeech.WithAPIKey(apiKey))
	} else if accessKey := credentialString(cfg.Credential, "sauc_access_key", "api_key", "access_token", "token", "bearer_token"); accessKey != "" {
		clientOpts = append(clientOpts, doubaospeech.WithV2APIKey(accessKey, cfg.Tenant.Volc.AppId))
	} else if accessKey := credentialString(cfg.Credential, "access_key", "access_key_id"); accessKey != "" {
		clientOpts = append(clientOpts, doubaospeech.WithV2APIKey(accessKey, cfg.Tenant.Volc.AppId))
	} else {
		return nil, fmt.Errorf("%w: credential %q missing speech api_key/token/access_token", ErrInvalid, cfg.Credential.Name)
	}
	opts := []transformers.DoubaoASRSAUCOption{}
	opts = append(opts, transformers.WithDoubaoASRSAUCResourceID(resourceID))
	client := doubaospeech.NewClient(cfg.Tenant.Volc.AppId, clientOpts...)
	return transformers.NewDoubaoASRSAUC(client, opts...), nil
}

func (b DefaultBuilder) buildVolcRealtime(cfg TransformerConfig) (genx.Transformer, error) {
	if cfg.Tenant.Volc == nil || cfg.Model == nil {
		return nil, fmt.Errorf("%w: volc tenant and model are required", ErrInvalid)
	}
	data := mergeParams(nil, cfg.Params)
	clientOpts := []doubaospeech.Option{doubaospeech.WithResourceID(doubaospeech.ResourceRealtime)}
	if resourceID := mapString(data, "resource_id"); resourceID != "" {
		clientOpts[0] = doubaospeech.WithResourceID(resourceID)
	}
	switch mapString(data, "auth_mode", "auth") {
	case "x-api-key", "api_key", "":
		apiKey := credentialString(cfg.Credential, "api_key", "x_api_key")
		if apiKey != "" {
			clientOpts = append(clientOpts, doubaospeech.WithAPIKey(apiKey))
			break
		}
		token := credentialString(cfg.Credential, "token", "bearer_token", "access_token")
		if token != "" {
			clientOpts = append(clientOpts, doubaospeech.WithBearerToken(token))
			break
		}
		return nil, fmt.Errorf("%w: credential %q missing api_key or token for doubao realtime", ErrInvalid, cfg.Credential.Name)
	case "v2", "realtime-api-key":
		accessKey := credentialString(cfg.Credential, "access_key", "access_key_id", "token", "bearer_token")
		if accessKey == "" {
			return nil, fmt.Errorf("%w: credential %q missing access key for doubao realtime", ErrInvalid, cfg.Credential.Name)
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
	client := doubaospeech.NewClient(cfg.Tenant.Volc.AppId, clientOpts...)
	return transformers.NewDoubaoRealtime(client, opts...), nil
}

func (b DefaultBuilder) buildVolcTTS(cfg TransformerConfig) (genx.Transformer, error) {
	if cfg.Tenant.Volc == nil || cfg.Voice == nil {
		return nil, fmt.Errorf("%w: volc tenant and voice are required", ErrInvalid)
	}
	token := credentialString(cfg.Credential, "token", "api_key", "bearer_token")
	if token == "" {
		return nil, fmt.Errorf("%w: credential %q missing token", ErrInvalid, cfg.Credential.Name)
	}
	data := voiceProviderData(*cfg.Voice, string(apitypes.VoiceProviderKindVolcTenant))
	voiceID := mapString(data, "voice_id")
	if voiceID == "" {
		return nil, fmt.Errorf("%w: voice %q missing voice_id", ErrInvalid, cfg.Voice.Id)
	}
	opts := []transformers.DoubaoTTSSeedV2Option{
		transformers.WithDoubaoTTSSeedV2Format(defaultTTSAudioFormat),
		transformers.WithDoubaoTTSSeedV2SampleRate(defaultTTSAudioSampleRate),
	}
	if value := mapString(data, "resource_id"); value != "" {
		opts = append(opts, transformers.WithDoubaoTTSSeedV2ResourceID(value))
	}
	client := doubaospeech.NewClient(cfg.Tenant.Volc.AppId, doubaospeech.WithBearerToken(token))
	return transformers.NewDoubaoTTSSeedV2(client, voiceID, opts...), nil
}

func (b DefaultBuilder) buildMiniMaxTTS(cfg TransformerConfig) (genx.Transformer, error) {
	if cfg.Tenant.MiniMax == nil || cfg.Voice == nil {
		return nil, fmt.Errorf("%w: minimax tenant and voice are required", ErrInvalid)
	}
	apiKey := credentialString(cfg.Credential, "api_key", "token")
	if apiKey == "" {
		return nil, fmt.Errorf("%w: credential %q missing api_key", ErrInvalid, cfg.Credential.Name)
	}
	data := voiceProviderData(*cfg.Voice, string(apitypes.VoiceProviderKindMinimaxTenant))
	voiceID := mapString(data, "voice_id")
	if voiceID == "" {
		return nil, fmt.Errorf("%w: voice %q missing voice_id", ErrInvalid, cfg.Voice.Id)
	}
	clientConfig := minimax.Config{
		APIKey:  apiKey,
		BaseURL: firstString(cfg.Tenant.MiniMax.BaseUrl, credentialBodyString(cfg.Credential.Body, "base_url"), defaultMiniMaxBaseURL),
	}
	client, err := minimax.NewClient(clientConfig)
	if err != nil {
		return nil, err
	}
	opts := []transformers.MinimaxTTSOption{
		transformers.WithMinimaxTTSFormat(defaultTTSAudioFormat),
		transformers.WithMinimaxTTSSampleRate(defaultTTSAudioSampleRate),
	}
	if model := mapString(data, "model"); model != "" {
		opts = append(opts, transformers.WithMinimaxTTSModel(model))
	}
	if format := mapString(data, "format"); format != "" {
		opts = append(opts, transformers.WithMinimaxTTSFormat(format))
	}
	if sampleRate, ok := mapInt(data, "sample_rate"); ok {
		opts = append(opts, transformers.WithMinimaxTTSSampleRate(sampleRate))
	}
	return transformers.NewMinimaxTTS(client, voiceID, opts...), nil
}

func decodeProviderData[T any](providerData *apitypes.ModelProviderData, kind string, out *T) error {
	if out == nil || providerData == nil {
		return nil
	}
	value, ok := (*providerData)[kind]
	if !ok || value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("%w: encode provider_data[%s]: %w", ErrInvalid, kind, err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%w: decode provider_data[%s]: %w", ErrInvalid, kind, err)
	}
	return nil
}

func voiceProviderData(voice apitypes.Voice, kind string) map[string]any {
	if voice.ProviderData == nil {
		return nil
	}
	value := (*voice.ProviderData)[kind]
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			out[key] = value
		}
		return out
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return nil
		}
		var out map[string]any
		if err := json.Unmarshal(data, &out); err != nil {
			return nil
		}
		return out
	}
}

func credentialString(credential apitypes.Credential, keys ...string) string {
	return credentialBodyString(credential.Body, keys...)
}

func credentialBodyString(body apitypes.CredentialBody, keys ...string) string {
	for _, key := range keys {
		if value := mapString(map[string]any(body), key); value != "" {
			return value
		}
	}
	return ""
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
