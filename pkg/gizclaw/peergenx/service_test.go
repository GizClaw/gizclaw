package peergenx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/acl"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

func TestGeneratorAuthorizesBeforeReadingModel(t *testing.T) {
	ctx := context.Background()
	events := []string{}
	svc := New(Service{
		Peer:            newTestPeer(),
		Authorizer:      &recordingAuthorizer{events: &events},
		Models:          fakeModels{events: &events},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
		Builder:         fakeBuilder{events: &events},
	})

	stream, err := svc.Generator().GenerateStream(ctx, "model/chat", nil)
	if err != nil {
		t.Fatalf("GenerateStream() error = %v", err)
	}
	if stream == nil {
		t.Fatal("GenerateStream() stream = nil")
	}

	want := []string{
		"auth:model:chat:model.read",
		"get:model:chat",
		"auth:model:chat:model.use",
		"get:tenant:openai:main",
		"auth:credential:openai-key:credential.read",
		"auth:credential:openai-key:credential.use",
		"get:credential:openai-key",
		"build:generator:chat",
		"call:generator:model/chat",
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

func TestGeneratorDeniedReadDoesNotReadModel(t *testing.T) {
	ctx := context.Background()
	events := []string{}
	svc := New(Service{
		Peer:       newTestPeer(),
		Authorizer: &recordingAuthorizer{events: &events, deny: "auth:model:chat:model.read"},
		Models:     fakeModels{events: &events},
		Builder:    fakeBuilder{events: &events},
	})

	_, err := svc.Generator().GenerateStream(ctx, "model/chat", nil)
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("GenerateStream() error = %v, want %v", err, ErrDenied)
	}
	want := []string{"auth:model:chat:model.read"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

func TestTransformerVoiceAuthorizesBeforeReadingVoiceAndCredential(t *testing.T) {
	ctx := context.Background()
	events := []string{}
	svc := New(Service{
		Peer:            newTestPeer(),
		Authorizer:      &recordingAuthorizer{events: &events},
		Voices:          fakeVoices{events: &events},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
		Builder:         fakeBuilder{events: &events},
	})

	stream, err := svc.Transformer().Transform(ctx, "voice/cancan", fakeStream{})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if stream == nil {
		t.Fatal("Transform() stream = nil")
	}

	want := []string{
		"auth:voice:cancan:voice.read",
		"get:voice:cancan",
		"auth:voice:cancan:voice.use",
		"get:tenant:volc:main",
		"auth:credential:volc-token:credential.read",
		"auth:credential:volc-token:credential.use",
		"get:credential:volc-token",
		"build:transformer:voice:cancan",
		"call:transformer:voice/cancan",
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

func TestTransformerModelASRUsesVolcTenant(t *testing.T) {
	ctx := context.Background()
	events := []string{}
	svc := New(Service{
		Peer:            newTestPeer(),
		Authorizer:      &recordingAuthorizer{events: &events},
		Models:          fakeModels{events: &events, modelKind: apitypes.ModelKindAsr, providerKind: "volc-tenant"},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
		Builder:         fakeBuilder{events: &events},
	})

	if _, err := svc.Transformer().Transform(ctx, "model/asr", fakeStream{}); err != nil {
		t.Fatalf("Transform() error = %v", err)
	}

	want := []string{
		"auth:model:asr:model.read",
		"get:model:asr",
		"auth:model:asr:model.use",
		"get:tenant:volc:main",
		"auth:credential:volc-token:credential.read",
		"auth:credential:volc-token:credential.use",
		"get:credential:volc-token",
		"build:transformer:model:asr",
		"call:transformer:model/asr",
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

func TestTransformerModelRealtimeUsesVolcTenant(t *testing.T) {
	ctx := context.Background()
	events := []string{}
	svc := New(Service{
		Peer:            newTestPeer(),
		Authorizer:      &recordingAuthorizer{events: &events},
		Models:          fakeModels{events: &events, modelKind: apitypes.ModelKindRealtime, providerKind: "volc-tenant"},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
		Builder:         fakeBuilder{events: &events},
	})

	if _, err := svc.Transformer().Transform(ctx, "model/realtime", fakeStream{}); err != nil {
		t.Fatalf("Transform() error = %v", err)
	}

	want := []string{
		"auth:model:realtime:model.read",
		"get:model:realtime",
		"auth:model:realtime:model.use",
		"get:tenant:volc:main",
		"auth:credential:volc-token:credential.read",
		"auth:credential:volc-token:credential.use",
		"get:credential:volc-token",
		"build:transformer:model:realtime",
		"call:transformer:model/realtime",
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

func TestTransformerVoiceSupportsMiniMaxTenant(t *testing.T) {
	ctx := context.Background()
	events := []string{}
	svc := New(Service{
		Peer:            newTestPeer(),
		Authorizer:      &recordingAuthorizer{events: &events},
		Voices:          fakeVoices{events: &events, providerKind: apitypes.VoiceProviderKindMinimaxTenant},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
		Builder:         fakeBuilder{events: &events},
	})

	if _, err := svc.Transformer().Transform(ctx, "voice/minimax", fakeStream{}); err != nil {
		t.Fatalf("Transform() error = %v", err)
	}

	want := []string{
		"auth:voice:minimax:voice.read",
		"get:voice:minimax",
		"auth:voice:minimax:voice.use",
		"get:tenant:minimax:main",
		"auth:credential:minimax-key:credential.read",
		"auth:credential:minimax-key:credential.use",
		"get:credential:minimax-key",
		"build:transformer:voice:minimax",
		"call:transformer:voice/minimax",
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

func TestTransformerDeniedVoiceUseDoesNotReadCredential(t *testing.T) {
	ctx := context.Background()
	events := []string{}
	svc := New(Service{
		Peer:            newTestPeer(),
		Authorizer:      &recordingAuthorizer{events: &events, deny: "auth:voice:cancan:voice.use"},
		Voices:          fakeVoices{events: &events},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
		Builder:         fakeBuilder{events: &events},
	})

	_, err := svc.Transformer().Transform(ctx, "voice/cancan", fakeStream{})
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("Transform() error = %v, want %v", err, ErrDenied)
	}
	want := []string{
		"auth:voice:cancan:voice.read",
		"get:voice:cancan",
		"auth:voice:cancan:voice.use",
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

func TestDefaultBuilderBuildsOpenAIGenerator(t *testing.T) {
	trueValue := true
	upstream := "gpt-test"
	gen, err := (DefaultBuilder{}).BuildGenerator(context.Background(), GeneratorConfig{
		Model: apitypes.Model{
			Id:   "chat",
			Kind: apitypes.ModelKindLlm,
			Capabilities: &apitypes.ModelCapabilities{
				JsonOutput: &trueValue,
				ToolCalls:  &trueValue,
				TextOnly:   &trueValue,
				SystemRole: &trueValue,
			},
			ProviderData: &apitypes.ModelProviderData{
				"openai-tenant": map[string]any{"upstream_model": upstream},
			},
		},
		Tenant: Tenant{
			Kind:   "openai-tenant",
			OpenAI: &apitypes.OpenAITenant{Name: "main", CredentialName: "openai-key"},
		},
		Credential: apitypes.Credential{
			Name: "openai-key",
			Body: apitypes.CredentialBody{"api_key": "sk-test"},
		},
	})
	if err != nil {
		t.Fatalf("BuildGenerator() error = %v", err)
	}
	openaiGen, ok := gen.(*genx.OpenAIGenerator)
	if !ok {
		t.Fatalf("BuildGenerator() = %T, want *genx.OpenAIGenerator", gen)
	}
	if openaiGen.Model != upstream || !openaiGen.SupportJSONOutput || !openaiGen.SupportToolCalls || !openaiGen.TextOnly || openaiGen.PromptRole != genx.PromptRoleSystem {
		t.Fatalf("OpenAIGenerator = %#v", openaiGen)
	}
}

func TestDefaultBuilderBuildsVolcASRTransformer(t *testing.T) {
	tf, err := (DefaultBuilder{}).BuildTransformer(context.Background(), TransformerConfig{
		Model: &apitypes.Model{
			Id:   "asr",
			Kind: apitypes.ModelKindAsr,
			ProviderData: &apitypes.ModelProviderData{
				"volc-tenant": map[string]any{
					"resource_id": "volc.bigasr.sauc.duration",
					"format":      "pcm",
					"sample_rate": 24000,
					"channels":    2,
					"bits":        8,
					"language":    "en-US",
					"result_type": "full",
				},
			},
		},
		Tenant: Tenant{
			Kind: "volc-tenant",
			Volc: &apitypes.VolcTenant{
				Name:           "main",
				AppId:          "app-id",
				CredentialName: "volc-token",
			},
		},
		Credential: apitypes.Credential{
			Name: "volc-token",
			Body: apitypes.CredentialBody{"token": "tok"},
		},
	})
	if err != nil {
		t.Fatalf("BuildTransformer() error = %v", err)
	}
	if tf == nil {
		t.Fatal("BuildTransformer() transformer = nil")
	}
	if got := transformerStringField(t, tf, "format"); got != "ogg_opus" {
		t.Fatalf("ASR format = %q, want ogg_opus", got)
	}
	if got := transformerIntField(t, tf, "sampleRate"); got != 16000 {
		t.Fatalf("ASR sampleRate = %d, want 16000", got)
	}
	if got := transformerIntField(t, tf, "channels"); got != 1 {
		t.Fatalf("ASR channels = %d, want 1", got)
	}
	if got := transformerIntField(t, tf, "bits"); got != 16 {
		t.Fatalf("ASR bits = %d, want 16", got)
	}
	if got := transformerStringField(t, tf, "language"); got != "zh-CN" {
		t.Fatalf("ASR language = %q, want zh-CN", got)
	}
	if got := transformerStringField(t, tf, "resultType"); got != "single" {
		t.Fatalf("ASR resultType = %q, want single", got)
	}
	if got := transformerStringField(t, tf, "resourceID"); got != "volc.bigasr.sauc.duration" {
		t.Fatalf("ASR resourceID = %q, want volc.bigasr.sauc.duration", got)
	}
}

func TestDefaultBuilderBuildsVolcRealtimeTransformer(t *testing.T) {
	tf, err := (DefaultBuilder{}).BuildTransformer(context.Background(), TransformerConfig{
		Model: &apitypes.Model{
			Id:   "dialog",
			Kind: apitypes.ModelKindRealtime,
			ProviderData: &apitypes.ModelProviderData{
				"volc-tenant": map[string]any{
					"upstream_model": "O",
					"speaker":        "model-speaker",
					"format":         "pcm_s16le",
					"sample_rate":    16000,
					"vad_window_ms":  180,
				},
			},
		},
		Tenant: Tenant{
			Kind: "volc-tenant",
			Volc: &apitypes.VolcTenant{
				Name:           "main",
				AppId:          "app-id",
				CredentialName: "volc-token",
			},
		},
		Credential: apitypes.Credential{
			Name: "volc-token",
			Body: apitypes.CredentialBody{"api_key": "runtime-key"},
		},
		Params: map[string]any{
			"upstream_model": "SC",
			"speaker":        "speaker-id",
			"vad_window_ms":  220,
		},
	})
	if err != nil {
		t.Fatalf("BuildTransformer() error = %v", err)
	}
	if got := transformerStringField(t, tf, "model"); got != "SC" {
		t.Fatalf("realtime model = %q, want SC", got)
	}
	if got := transformerStringField(t, tf, "speaker"); got != "speaker-id" {
		t.Fatalf("realtime speaker = %q, want speaker-id", got)
	}
	if got := transformerIntField(t, tf, "vadWindowMs"); got != 220 {
		t.Fatalf("realtime vadWindowMs = %d, want 220", got)
	}
}

func TestDefaultBuilderBuildsVolcRealtimeTransformerFromWorkflowParams(t *testing.T) {
	tf, err := (DefaultBuilder{}).BuildTransformer(context.Background(), TransformerConfig{
		Model: &apitypes.Model{
			Id:   "dialog",
			Kind: apitypes.ModelKindRealtime,
		},
		Tenant: Tenant{
			Kind: "volc-tenant",
			Volc: &apitypes.VolcTenant{
				Name:           "main",
				AppId:          "app-id",
				CredentialName: "volc-token",
			},
		},
		Credential: apitypes.Credential{
			Name: "volc-token",
			Body: apitypes.CredentialBody{"access_key": "realtime-key"},
		},
		Params: map[string]any{
			"auth_mode":          "v2",
			"resource_id":        "volc.speech.dialog",
			"upstream_model":     "O",
			"speaker":            "workflow-speaker",
			"format":             "ogg_opus",
			"sample_rate":        24000,
			"channels":           1,
			"input_format":       "speech_opus",
			"input_sample_rate":  16000,
			"input_channels":     1,
			"input_transcode":    true,
			"bot_name":           "豆包",
			"system_role":        "简短回答。",
			"speaking_style":     "自然",
			"character_manifest": "友好",
			"vad_window_ms":      200,
		},
	})
	if err != nil {
		t.Fatalf("BuildTransformer() error = %v", err)
	}
	if got := transformerNestedStringField(t, tf, "client", "config", "resourceID"); got != "volc.speech.dialog" {
		t.Fatalf("realtime resourceID = %q, want volc.speech.dialog", got)
	}
	if got := transformerStringField(t, tf, "model"); got != "O" {
		t.Fatalf("realtime model = %q, want O", got)
	}
	if got := transformerStringField(t, tf, "speaker"); got != "workflow-speaker" {
		t.Fatalf("realtime speaker = %q, want workflow-speaker", got)
	}
	if got := transformerStringField(t, tf, "format"); got != "ogg_opus" {
		t.Fatalf("realtime format = %q, want ogg_opus", got)
	}
	if got := transformerIntField(t, tf, "sampleRate"); got != 24000 {
		t.Fatalf("realtime sampleRate = %d, want 24000", got)
	}
	if got := transformerIntField(t, tf, "channels"); got != 1 {
		t.Fatalf("realtime channels = %d, want 1", got)
	}
	if got := transformerStringField(t, tf, "inputFormat"); got != "speech_opus" {
		t.Fatalf("realtime inputFormat = %q, want speech_opus", got)
	}
	if got := transformerIntField(t, tf, "inputSampleRate"); got != 16000 {
		t.Fatalf("realtime inputSampleRate = %d, want 16000", got)
	}
	if got := transformerIntField(t, tf, "inputChannels"); got != 1 {
		t.Fatalf("realtime inputChannels = %d, want 1", got)
	}
	if !transformerBoolField(t, tf, "inputTranscode") {
		t.Fatal("realtime inputTranscode = false, want true")
	}
	if got := transformerStringField(t, tf, "botName"); got != "豆包" {
		t.Fatalf("realtime botName = %q, want 豆包", got)
	}
	if got := transformerStringField(t, tf, "systemRole"); got != "简短回答。" {
		t.Fatalf("realtime systemRole = %q, want 简短回答。", got)
	}
	if got := transformerStringField(t, tf, "speakingStyle"); got != "自然" {
		t.Fatalf("realtime speakingStyle = %q, want 自然", got)
	}
	if got := transformerStringField(t, tf, "characterManifest"); got != "友好" {
		t.Fatalf("realtime characterManifest = %q, want 友好", got)
	}
	if got := transformerIntField(t, tf, "vadWindowMs"); got != 200 {
		t.Fatalf("realtime vadWindowMs = %d, want 200", got)
	}
}

func TestDefaultBuilderBuildsVolcASRTransformerPrefersSaucAccessToken(t *testing.T) {
	tf, err := (DefaultBuilder{}).BuildTransformer(context.Background(), TransformerConfig{
		Model: &apitypes.Model{
			Id:   "asr",
			Kind: apitypes.ModelKindAsr,
			ProviderData: &apitypes.ModelProviderData{
				"volc-tenant": map[string]any{"resource_id": "volc.bigasr.sauc.duration"},
			},
		},
		Tenant: Tenant{
			Kind: "volc-tenant",
			Volc: &apitypes.VolcTenant{
				Name:           "main",
				AppId:          "app-id",
				CredentialName: "volc-token",
			},
		},
		Credential: apitypes.Credential{
			Name: "volc-token",
			Body: apitypes.CredentialBody{
				"access_key_id": "volc-ak",
				"bearer_token":  "old-sauc-token",
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildTransformer() error = %v", err)
	}
	if got := transformerNestedStringField(t, tf, "client", "config", "accessKey"); got != "old-sauc-token" {
		t.Fatalf("ASR access key = %q, want old SAUC token", got)
	}
	if got := transformerNestedStringField(t, tf, "client", "config", "resourceID"); got != "volc.bigasr.sauc.duration" {
		t.Fatalf("ASR resource ID = %q, want volc.bigasr.sauc.duration", got)
	}
}

func TestDefaultBuilderBuildsVolcASRTransformerTreatsAPIKeyAsSpeechAccessKey(t *testing.T) {
	tf, err := (DefaultBuilder{}).BuildTransformer(context.Background(), TransformerConfig{
		Model: &apitypes.Model{
			Id:   "asr",
			Kind: apitypes.ModelKindAsr,
			ProviderData: &apitypes.ModelProviderData{
				"volc-tenant": map[string]any{"resource_id": "volc.bigasr.sauc.duration"},
			},
		},
		Tenant: Tenant{
			Kind: "volc-tenant",
			Volc: &apitypes.VolcTenant{Name: "main", AppId: "app-id"},
		},
		Credential: apitypes.Credential{
			Name: "volc-token",
			Body: apitypes.CredentialBody{"api_key": "speech-runtime-key"},
		},
	})
	if err != nil {
		t.Fatalf("BuildTransformer() error = %v", err)
	}
	if got := transformerNestedStringField(t, tf, "client", "config", "accessKey"); got != "speech-runtime-key" {
		t.Fatalf("ASR access key = %q, want speech runtime key", got)
	}
	if got := transformerNestedStringField(t, tf, "client", "config", "apiKey"); got != "" {
		t.Fatalf("ASR x-api-key = %q, want empty by default", got)
	}
}

func TestDefaultBuilderBuildsVolcASRTransformerSupportsXAPIKeyMode(t *testing.T) {
	tf, err := (DefaultBuilder{}).BuildTransformer(context.Background(), TransformerConfig{
		Model: &apitypes.Model{
			Id:   "asr",
			Kind: apitypes.ModelKindAsr,
			ProviderData: &apitypes.ModelProviderData{
				"volc-tenant": map[string]any{"auth_mode": "x-api-key"},
			},
		},
		Tenant: Tenant{
			Kind: "volc-tenant",
			Volc: &apitypes.VolcTenant{Name: "main", AppId: "app-id"},
		},
		Credential: apitypes.Credential{
			Name: "volc-token",
			Body: apitypes.CredentialBody{"api_key": "real-x-api-key"},
		},
	})
	if err != nil {
		t.Fatalf("BuildTransformer() error = %v", err)
	}
	if got := transformerNestedStringField(t, tf, "client", "config", "apiKey"); got != "real-x-api-key" {
		t.Fatalf("ASR x-api-key = %q, want real x-api-key", got)
	}
	if got := transformerNestedStringField(t, tf, "client", "config", "accessKey"); got != "" {
		t.Fatalf("ASR access key = %q, want empty in x-api-key mode", got)
	}
}

func TestDefaultBuilderBuildsGeminiGenerator(t *testing.T) {
	upstream := "gemini-test"
	gen, err := (DefaultBuilder{}).BuildGenerator(context.Background(), GeneratorConfig{
		Model: apitypes.Model{
			Id:   "gemini",
			Kind: apitypes.ModelKindLlm,
			ProviderData: &apitypes.ModelProviderData{
				"gemini-tenant": map[string]any{"upstream_model": upstream},
			},
		},
		Tenant: Tenant{
			Kind:   "gemini-tenant",
			Gemini: &apitypes.GeminiTenant{Name: "main", CredentialName: "gemini-key"},
		},
		Credential: apitypes.Credential{
			Name: "gemini-key",
			Body: apitypes.CredentialBody{"api_key": "gemini-token"},
		},
	})
	if err != nil {
		t.Fatalf("BuildGenerator() error = %v", err)
	}
	geminiGen, ok := gen.(*genx.GeminiGenerator)
	if !ok {
		t.Fatalf("BuildGenerator() = %T, want *genx.GeminiGenerator", gen)
	}
	if geminiGen.Model != upstream {
		t.Fatalf("GeminiGenerator.Model = %q, want %q", geminiGen.Model, upstream)
	}
}

func TestDefaultBuilderBuildsVoiceTransformers(t *testing.T) {
	baseURL := "https://minimax.example"
	for _, tc := range []struct {
		name           string
		cfg            TransformerConfig
		wantFormat     string
		wantSampleRate int
		wantResourceID string
		wantModel      string
		wantBaseURL    string
	}{
		{
			name: "volc",
			cfg: TransformerConfig{
				Voice: &apitypes.Voice{
					Id: "volc-voice",
					ProviderData: &apitypes.VoiceProviderData{
						"volc-tenant": map[string]any{
							"voice_id":    "voice-id",
							"resource_id": "seed-icl-2.0",
							"format":      "ogg_opus",
							"sample_rate": 24000,
						},
					},
				},
				Tenant: Tenant{
					Kind: "volc-tenant",
					Volc: &apitypes.VolcTenant{Name: "main", AppId: "app-id", CredentialName: "volc-token"},
				},
				Credential: apitypes.Credential{Name: "volc-token", Body: apitypes.CredentialBody{"token": "tok"}},
			},
			wantFormat:     defaultTTSAudioFormat,
			wantSampleRate: defaultTTSAudioSampleRate,
			wantResourceID: "seed-icl-2.0",
		},
		{
			name: "minimax",
			cfg: TransformerConfig{
				Voice: &apitypes.Voice{
					Id: "minimax-voice",
					ProviderData: &apitypes.VoiceProviderData{
						"minimax-tenant": struct {
							VoiceID string `json:"voice_id"`
							Model   string `json:"model"`
						}{VoiceID: "voice-id", Model: "speech-02-hd"},
					},
				},
				Tenant: Tenant{
					Kind:    "minimax-tenant",
					MiniMax: &apitypes.MiniMaxTenant{Name: "main", CredentialName: "minimax-key", BaseUrl: &baseURL},
				},
				Credential: apitypes.Credential{Name: "minimax-key", Body: apitypes.CredentialBody{"api_key": "sk-test"}},
			},
			wantFormat:     defaultTTSAudioFormat,
			wantSampleRate: defaultTTSAudioSampleRate,
			wantModel:      "speech-02-hd",
			wantBaseURL:    baseURL,
		},
		{
			name: "minimax default base url",
			cfg: TransformerConfig{
				Voice: &apitypes.Voice{
					Id: "minimax-voice",
					ProviderData: &apitypes.VoiceProviderData{
						"minimax-tenant": map[string]string{"voice_id": "voice-id"},
					},
				},
				Tenant: Tenant{
					Kind:    "minimax-tenant",
					MiniMax: &apitypes.MiniMaxTenant{Name: "main", CredentialName: "minimax-key"},
				},
				Credential: apitypes.Credential{Name: "minimax-key", Body: apitypes.CredentialBody{"api_key": "sk-test"}},
			},
			wantFormat:     defaultTTSAudioFormat,
			wantSampleRate: defaultTTSAudioSampleRate,
			wantBaseURL:    defaultMiniMaxBaseURL,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tf, err := (DefaultBuilder{}).BuildTransformer(context.Background(), tc.cfg)
			if err != nil {
				t.Fatalf("BuildTransformer() error = %v", err)
			}
			if tf == nil {
				t.Fatal("BuildTransformer() transformer = nil")
			}
			if got := transformerStringField(t, tf, "format"); got != tc.wantFormat {
				t.Fatalf("transformer format = %q, want %q", got, tc.wantFormat)
			}
			if got := transformerIntField(t, tf, "sampleRate"); got != tc.wantSampleRate {
				t.Fatalf("transformer sampleRate = %d, want %d", got, tc.wantSampleRate)
			}
			if tc.wantModel != "" {
				if got := transformerStringField(t, tf, "model"); got != tc.wantModel {
					t.Fatalf("transformer model = %q, want %q", got, tc.wantModel)
				}
			}
			if tc.wantResourceID != "" {
				if got := transformerStringField(t, tf, "resourceID"); got != tc.wantResourceID {
					t.Fatalf("transformer resourceID = %q, want %q", got, tc.wantResourceID)
				}
			}
			if tc.wantBaseURL != "" {
				if got := transformerNestedStringField(t, tf, "client", "transport", "baseURL"); got != tc.wantBaseURL {
					t.Fatalf("transformer minimax baseURL = %q, want %q", got, tc.wantBaseURL)
				}
			}
		})
	}
}

func TestDefaultBuilderRejectsInvalidGeneratorConfigs(t *testing.T) {
	tests := []struct {
		name string
		cfg  GeneratorConfig
	}{
		{
			name: "unsupported provider",
			cfg: GeneratorConfig{
				Tenant: Tenant{Kind: "unknown"},
			},
		},
		{
			name: "openai tenant missing",
			cfg: GeneratorConfig{
				Tenant: Tenant{Kind: string(apitypes.ModelProviderKindOpenaiTenant)},
			},
		},
		{
			name: "openai credential missing api key",
			cfg: GeneratorConfig{
				Model: apitypes.Model{Id: "chat"},
				Tenant: Tenant{
					Kind:   string(apitypes.ModelProviderKindOpenaiTenant),
					OpenAI: &apitypes.OpenAITenant{Name: "main"},
				},
			},
		},
		{
			name: "openai upstream missing",
			cfg: GeneratorConfig{
				Tenant: Tenant{
					Kind:   string(apitypes.ModelProviderKindOpenaiTenant),
					OpenAI: &apitypes.OpenAITenant{Name: "main"},
				},
				Credential: apitypes.Credential{Body: apitypes.CredentialBody{"api_key": "sk-test"}},
			},
		},
		{
			name: "gemini tenant missing",
			cfg: GeneratorConfig{
				Tenant: Tenant{Kind: string(apitypes.ModelProviderKindGeminiTenant)},
			},
		},
		{
			name: "gemini credential missing api key",
			cfg: GeneratorConfig{
				Tenant: Tenant{
					Kind:   string(apitypes.ModelProviderKindGeminiTenant),
					Gemini: &apitypes.GeminiTenant{Name: "main"},
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := (DefaultBuilder{}).BuildGenerator(context.Background(), tc.cfg); err == nil {
				t.Fatal("BuildGenerator() error = nil")
			}
		})
	}
}

func TestDefaultBuilderRejectsInvalidTransformerConfigs(t *testing.T) {
	tests := []struct {
		name string
		cfg  TransformerConfig
	}{
		{
			name: "empty config",
			cfg:  TransformerConfig{},
		},
		{
			name: "unsupported voice provider",
			cfg: TransformerConfig{
				Voice:  &apitypes.Voice{Id: "voice"},
				Tenant: Tenant{Kind: "unknown"},
			},
		},
		{
			name: "unsupported model kind",
			cfg: TransformerConfig{
				Model:  &apitypes.Model{Id: "chat", Kind: apitypes.ModelKindLlm},
				Tenant: Tenant{Kind: string(apitypes.VoiceProviderKindVolcTenant)},
			},
		},
		{
			name: "unsupported model provider",
			cfg: TransformerConfig{
				Model:  &apitypes.Model{Id: "asr", Kind: apitypes.ModelKindAsr},
				Tenant: Tenant{Kind: "unknown"},
			},
		},
		{
			name: "volc asr missing tenant",
			cfg: TransformerConfig{
				Model:  &apitypes.Model{Id: "asr", Kind: apitypes.ModelKindAsr},
				Tenant: Tenant{Kind: string(apitypes.VoiceProviderKindVolcTenant)},
			},
		},
		{
			name: "volc asr missing token",
			cfg: TransformerConfig{
				Model: &apitypes.Model{Id: "asr", Kind: apitypes.ModelKindAsr},
				Tenant: Tenant{
					Kind: string(apitypes.VoiceProviderKindVolcTenant),
					Volc: &apitypes.VolcTenant{Name: "main", AppId: "app-id"},
				},
			},
		},
		{
			name: "volc tts missing voice id",
			cfg: TransformerConfig{
				Voice: &apitypes.Voice{Id: "voice"},
				Tenant: Tenant{
					Kind: string(apitypes.VoiceProviderKindVolcTenant),
					Volc: &apitypes.VolcTenant{Name: "main", AppId: "app-id"},
				},
				Credential: apitypes.Credential{Body: apitypes.CredentialBody{"token": "tok"}},
			},
		},
		{
			name: "minimax missing credential",
			cfg: TransformerConfig{
				Voice: &apitypes.Voice{
					Id: "voice",
					ProviderData: &apitypes.VoiceProviderData{
						"minimax-tenant": map[string]any{"voice_id": "voice-id"},
					},
				},
				Tenant: Tenant{
					Kind:    string(apitypes.VoiceProviderKindMinimaxTenant),
					MiniMax: &apitypes.MiniMaxTenant{Name: "main"},
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := (DefaultBuilder{}).BuildTransformer(context.Background(), tc.cfg); err == nil {
				t.Fatal("BuildTransformer() error = nil")
			}
		})
	}
}

func transformerStringField(t *testing.T, tf genx.Transformer, fieldName string) string {
	t.Helper()
	value := reflect.Indirect(reflect.ValueOf(tf))
	field := value.FieldByName(fieldName)
	if !field.IsValid() || field.Kind() != reflect.String {
		t.Fatalf("transformer %T missing string field %q", tf, fieldName)
	}
	return field.String()
}

func transformerIntField(t *testing.T, tf genx.Transformer, fieldName string) int {
	t.Helper()
	value := reflect.Indirect(reflect.ValueOf(tf))
	field := value.FieldByName(fieldName)
	if !field.IsValid() || field.Kind() != reflect.Int {
		t.Fatalf("transformer %T missing int field %q", tf, fieldName)
	}
	return int(field.Int())
}

func transformerBoolField(t *testing.T, tf genx.Transformer, fieldName string) bool {
	t.Helper()
	value := reflect.Indirect(reflect.ValueOf(tf))
	field := value.FieldByName(fieldName)
	if !field.IsValid() || field.Kind() != reflect.Bool {
		t.Fatalf("transformer %T missing bool field %q", tf, fieldName)
	}
	return field.Bool()
}

func transformerNestedStringField(t *testing.T, tf genx.Transformer, fieldNames ...string) string {
	t.Helper()
	value := reflect.ValueOf(tf)
	for _, fieldName := range fieldNames {
		value = reflect.Indirect(value)
		field := value.FieldByName(fieldName)
		if !field.IsValid() {
			t.Fatalf("transformer %T missing field %q", tf, fieldName)
		}
		value = field
	}
	value = reflect.Indirect(value)
	if value.Kind() != reflect.String {
		t.Fatalf("transformer %T field path %v is %s, want string", tf, fieldNames, value.Kind())
	}
	return value.String()
}

func TestGeneratorInvokeUsesResolvedModel(t *testing.T) {
	ctx := context.Background()
	events := []string{}
	svc := New(Service{
		Peer:            newTestPeer(),
		Authorizer:      &recordingAuthorizer{events: &events},
		Models:          fakeModels{events: &events},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
		Builder:         fakeBuilder{events: &events},
	})

	if _, _, err := svc.Generator().Invoke(ctx, "model/chat", nil, nil); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}

	want := []string{
		"auth:model:chat:model.read",
		"get:model:chat",
		"auth:model:chat:model.use",
		"get:tenant:openai:main",
		"auth:credential:openai-key:credential.read",
		"auth:credential:openai-key:credential.use",
		"get:credential:openai-key",
		"build:generator:chat",
		"call:invoke:model/chat",
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

func TestResolveGeneratorSupportsAdditionalTenantKinds(t *testing.T) {
	for _, tc := range []struct {
		name         string
		providerKind string
		wantKind     string
	}{
		{name: "gemini", providerKind: string(apitypes.ModelProviderKindGeminiTenant), wantKind: string(apitypes.ModelProviderKindGeminiTenant)},
		{name: "dashscope", providerKind: string(apitypes.ModelProviderKindDashscopeTenant), wantKind: string(apitypes.ModelProviderKindDashscopeTenant)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := []string{}
			svc := New(Service{
				Peer:            newTestPeer(),
				Authorizer:      &recordingAuthorizer{events: &events},
				Models:          fakeModels{events: &events, providerKind: tc.providerKind},
				Credentials:     fakeCredentials{events: &events},
				ProviderTenants: fakeTenants{events: &events},
			})

			cfg, err := svc.ResolveGenerator(context.Background(), "model/chat")
			if err != nil {
				t.Fatalf("ResolveGenerator() error = %v", err)
			}
			if cfg.Tenant.Kind != tc.wantKind {
				t.Fatalf("Tenant.Kind = %q, want %q", cfg.Tenant.Kind, tc.wantKind)
			}
		})
	}
}

func TestBuilderHelpersHandleJSONNumberAndInvalidVoiceData(t *testing.T) {
	number := json.Number("42")
	if got, ok := mapInt(map[string]any{"n": number}, "n"); !ok || got != 42 {
		t.Fatalf("mapInt(json.Number) = %d, %v; want 42, true", got, ok)
	}
	if got := voiceProviderData(apitypes.Voice{ProviderData: &apitypes.VoiceProviderData{
		"bad": make(chan int),
	}}, "bad"); got != nil {
		t.Fatalf("voiceProviderData() = %#v, want nil", got)
	}
	if got, ok := parsePattern(" voice/cancan ", "voice"); !ok || got != "cancan" {
		t.Fatalf("parsePattern() = %q, %v; want cancan, true", got, ok)
	}
	if got, ok := parsePattern("voice/ ", "voice"); ok || got != "" {
		t.Fatalf("parsePattern(empty id) = %q, %v; want empty, false", got, ok)
	}
	if got, ok := parsePattern("model/realtime?vad_window_ms=200", "model"); !ok || got != "realtime" {
		t.Fatalf("parsePattern(query) = %q, %v; want realtime, true", got, ok)
	}
	base, params, err := splitPatternParams("model/realtime?input_transcode=true&vad_window_ms=200&system_role=%E7%AE%80%E7%9F%AD")
	if err != nil {
		t.Fatalf("splitPatternParams() error = %v", err)
	}
	if base != "model/realtime" {
		t.Fatalf("splitPatternParams base = %q, want model/realtime", base)
	}
	if got, ok := params["input_transcode"].(bool); !ok || !got {
		t.Fatalf("input_transcode param = %#v", params["input_transcode"])
	}
	if got, ok := params["vad_window_ms"].(int); !ok || got != 200 {
		t.Fatalf("vad_window_ms param = %#v", params["vad_window_ms"])
	}
	if got, ok := params["system_role"].(string); !ok || got != "简短" {
		t.Fatalf("system_role param = %#v", params["system_role"])
	}
	if !isDenied(fmt.Errorf("wrapped: %w", ErrDenied)) {
		t.Fatal("isDenied(wrapped ErrDenied) = false, want true")
	}
	if isDenied(ErrInvalid) {
		t.Fatal("isDenied(ErrInvalid) = true, want false")
	}
}

func TestGenXNilReceiversReturnNotConfigured(t *testing.T) {
	if _, err := (*Generator)(nil).GenerateStream(context.Background(), "model/chat", nil); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("nil Generator.GenerateStream() error = %v, want %v", err, ErrNotConfigured)
	}
	if _, _, err := (*Generator)(nil).Invoke(context.Background(), "model/chat", nil, nil); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("nil Generator.Invoke() error = %v, want %v", err, ErrNotConfigured)
	}
	if _, err := (*Transformer)(nil).Transform(context.Background(), "voice/cancan", fakeStream{}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("nil Transformer.Transform() error = %v, want %v", err, ErrNotConfigured)
	}
}

func TestResolverGettersHandleNotFoundAndUnexpectedResponses(t *testing.T) {
	ctx := context.Background()
	t.Run("model", func(t *testing.T) {
		svc := &Service{Models: responseModels{response: adminservice.GetModel404JSONResponse{}}}
		if _, err := svc.getModel(ctx, "missing"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("getModel(404) error = %v, want %v", err, ErrNotFound)
		}
		svc.Models = responseModels{response: adminservice.GetModel500JSONResponse{}}
		if _, err := svc.getModel(ctx, "bad"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("getModel(500) error = %v, want %v", err, ErrInvalid)
		}
	})
	t.Run("voice", func(t *testing.T) {
		svc := &Service{Voices: responseVoices{response: adminservice.GetVoice404JSONResponse{}}}
		if _, err := svc.getVoice(ctx, "missing"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("getVoice(404) error = %v, want %v", err, ErrNotFound)
		}
		svc.Voices = responseVoices{response: adminservice.GetVoice500JSONResponse{}}
		if _, err := svc.getVoice(ctx, "bad"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("getVoice(500) error = %v, want %v", err, ErrInvalid)
		}
	})
	t.Run("credential", func(t *testing.T) {
		svc := &Service{Credentials: responseCredentials{response: adminservice.GetCredential404JSONResponse{}}}
		if _, err := svc.getCredential(ctx, "missing"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("getCredential(404) error = %v, want %v", err, ErrNotFound)
		}
		svc.Credentials = responseCredentials{response: adminservice.GetCredential500JSONResponse{}}
		if _, err := svc.getCredential(ctx, "bad"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("getCredential(500) error = %v, want %v", err, ErrInvalid)
		}
	})
	t.Run("tenants", func(t *testing.T) {
		svc := &Service{ProviderTenants: responseTenants{
			openai:    adminservice.GetOpenAITenant404JSONResponse{},
			gemini:    adminservice.GetGeminiTenant404JSONResponse{},
			dashscope: adminservice.GetDashScopeTenant404JSONResponse{},
			minimax:   adminservice.GetMiniMaxTenant404JSONResponse{},
			volc:      adminservice.GetVolcTenant404JSONResponse{},
		}}
		for name, call := range map[string]func() error{
			"openai": func() error {
				_, err := svc.getOpenAITenant(ctx, "missing")
				return err
			},
			"gemini": func() error {
				_, err := svc.getGeminiTenant(ctx, "missing")
				return err
			},
			"dashscope": func() error {
				_, err := svc.getDashScopeTenant(ctx, "missing")
				return err
			},
			"minimax": func() error {
				_, err := svc.getMiniMaxTenant(ctx, "missing")
				return err
			},
			"volc": func() error {
				_, err := svc.getVolcTenant(ctx, "missing")
				return err
			},
		} {
			if err := call(); !errors.Is(err, ErrNotFound) {
				t.Fatalf("%s tenant 404 error = %v, want %v", name, err, ErrNotFound)
			}
		}

		svc.ProviderTenants = responseTenants{
			openai:    adminservice.GetOpenAITenant500JSONResponse{},
			gemini:    adminservice.GetGeminiTenant500JSONResponse{},
			dashscope: adminservice.GetDashScopeTenant500JSONResponse{},
			minimax:   adminservice.GetMiniMaxTenant500JSONResponse{},
			volc:      adminservice.GetVolcTenant500JSONResponse{},
		}
		for name, call := range map[string]func() error{
			"openai": func() error {
				_, err := svc.getOpenAITenant(ctx, "bad")
				return err
			},
			"gemini": func() error {
				_, err := svc.getGeminiTenant(ctx, "bad")
				return err
			},
			"dashscope": func() error {
				_, err := svc.getDashScopeTenant(ctx, "bad")
				return err
			},
			"minimax": func() error {
				_, err := svc.getMiniMaxTenant(ctx, "bad")
				return err
			},
			"volc": func() error {
				_, err := svc.getVolcTenant(ctx, "bad")
				return err
			},
		} {
			if err := call(); !errors.Is(err, ErrInvalid) {
				t.Fatalf("%s tenant unexpected response error = %v, want %v", name, err, ErrInvalid)
			}
		}
	})
}

type testPeer [32]byte

func (p testPeer) PublicKey() giznet.PublicKey {
	var pk giznet.PublicKey
	copy(pk[:], p[:])
	return pk
}

func newTestPeer() testPeer {
	var p testPeer
	p[0] = 1
	return p
}

type recordingAuthorizer struct {
	events *[]string
	deny   string
}

func (a *recordingAuthorizer) Authorize(_ context.Context, request acl.AuthorizeRequest) error {
	event := "auth:" + string(request.Resource.Kind) + ":" + request.Resource.Id + ":" + string(request.Permission)
	*a.events = append(*a.events, event)
	if a.deny == event {
		return acl.ErrDenied
	}
	return nil
}

type fakeModels struct {
	events       *[]string
	modelKind    apitypes.ModelKind
	providerKind string
}

func (f fakeModels) GetModel(_ context.Context, request adminservice.GetModelRequestObject) (adminservice.GetModelResponseObject, error) {
	*f.events = append(*f.events, "get:model:"+request.Id)
	kind := f.modelKind
	if kind == "" {
		kind = apitypes.ModelKindLlm
	}
	providerKind := f.providerKind
	if providerKind == "" {
		providerKind = string(apitypes.ModelProviderKindOpenaiTenant)
	}
	return adminservice.GetModel200JSONResponse(apitypes.Model{
		Id:   request.Id,
		Kind: kind,
		Provider: apitypes.ModelProvider{
			Kind: apitypes.ModelProviderKind(providerKind),
			Name: "main",
		},
	}), nil
}

type fakeVoices struct {
	events       *[]string
	providerKind apitypes.VoiceProviderKind
}

func (f fakeVoices) GetVoice(_ context.Context, request adminservice.GetVoiceRequestObject) (adminservice.GetVoiceResponseObject, error) {
	*f.events = append(*f.events, "get:voice:"+request.Id)
	providerKind := f.providerKind
	if providerKind == "" {
		providerKind = apitypes.VoiceProviderKindVolcTenant
	}
	providerData := apitypes.VoiceProviderData{
		string(providerKind): map[string]any{"voice_id": "voice-id"},
	}
	return adminservice.GetVoice200JSONResponse(apitypes.Voice{
		Id: request.Id,
		Provider: apitypes.VoiceProvider{
			Kind: providerKind,
			Name: "main",
		},
		ProviderData: &providerData,
	}), nil
}

type fakeCredentials struct {
	events *[]string
}

func (f fakeCredentials) GetCredential(_ context.Context, request adminservice.GetCredentialRequestObject) (adminservice.GetCredentialResponseObject, error) {
	*f.events = append(*f.events, "get:credential:"+request.Name)
	return adminservice.GetCredential200JSONResponse(apitypes.Credential{
		Name: request.Name,
		Body: apitypes.CredentialBody{
			"api_key": "sk-test",
			"token":   "tok-test",
		},
	}), nil
}

type fakeTenants struct {
	events *[]string
}

func (f fakeTenants) GetOpenAITenant(_ context.Context, request adminservice.GetOpenAITenantRequestObject) (adminservice.GetOpenAITenantResponseObject, error) {
	*f.events = append(*f.events, "get:tenant:openai:"+request.Name)
	return adminservice.GetOpenAITenant200JSONResponse(apitypes.OpenAITenant{Name: request.Name, CredentialName: "openai-key"}), nil
}

func (f fakeTenants) GetGeminiTenant(_ context.Context, request adminservice.GetGeminiTenantRequestObject) (adminservice.GetGeminiTenantResponseObject, error) {
	*f.events = append(*f.events, "get:tenant:gemini:"+request.Name)
	return adminservice.GetGeminiTenant200JSONResponse(apitypes.GeminiTenant{Name: request.Name, CredentialName: "gemini-key"}), nil
}

func (f fakeTenants) GetDashScopeTenant(_ context.Context, request adminservice.GetDashScopeTenantRequestObject) (adminservice.GetDashScopeTenantResponseObject, error) {
	*f.events = append(*f.events, "get:tenant:dashscope:"+request.Name)
	return adminservice.GetDashScopeTenant200JSONResponse(apitypes.DashScopeTenant{Name: request.Name, CredentialName: "dashscope-key"}), nil
}

func (f fakeTenants) GetMiniMaxTenant(_ context.Context, request adminservice.GetMiniMaxTenantRequestObject) (adminservice.GetMiniMaxTenantResponseObject, error) {
	*f.events = append(*f.events, "get:tenant:minimax:"+request.Name)
	return adminservice.GetMiniMaxTenant200JSONResponse(apitypes.MiniMaxTenant{Name: request.Name, CredentialName: "minimax-key"}), nil
}

func (f fakeTenants) GetVolcTenant(_ context.Context, request adminservice.GetVolcTenantRequestObject) (adminservice.GetVolcTenantResponseObject, error) {
	*f.events = append(*f.events, "get:tenant:volc:"+request.Name)
	return adminservice.GetVolcTenant200JSONResponse(apitypes.VolcTenant{Name: request.Name, AppId: "app-id", CredentialName: "volc-token"}), nil
}

type responseModels struct {
	response adminservice.GetModelResponseObject
}

func (f responseModels) GetModel(context.Context, adminservice.GetModelRequestObject) (adminservice.GetModelResponseObject, error) {
	return f.response, nil
}

type responseVoices struct {
	response adminservice.GetVoiceResponseObject
}

func (f responseVoices) GetVoice(context.Context, adminservice.GetVoiceRequestObject) (adminservice.GetVoiceResponseObject, error) {
	return f.response, nil
}

type responseCredentials struct {
	response adminservice.GetCredentialResponseObject
}

func (f responseCredentials) GetCredential(context.Context, adminservice.GetCredentialRequestObject) (adminservice.GetCredentialResponseObject, error) {
	return f.response, nil
}

type responseTenants struct {
	openai    adminservice.GetOpenAITenantResponseObject
	gemini    adminservice.GetGeminiTenantResponseObject
	dashscope adminservice.GetDashScopeTenantResponseObject
	minimax   adminservice.GetMiniMaxTenantResponseObject
	volc      adminservice.GetVolcTenantResponseObject
}

func (f responseTenants) GetOpenAITenant(context.Context, adminservice.GetOpenAITenantRequestObject) (adminservice.GetOpenAITenantResponseObject, error) {
	return f.openai, nil
}

func (f responseTenants) GetGeminiTenant(context.Context, adminservice.GetGeminiTenantRequestObject) (adminservice.GetGeminiTenantResponseObject, error) {
	return f.gemini, nil
}

func (f responseTenants) GetDashScopeTenant(context.Context, adminservice.GetDashScopeTenantRequestObject) (adminservice.GetDashScopeTenantResponseObject, error) {
	return f.dashscope, nil
}

func (f responseTenants) GetMiniMaxTenant(context.Context, adminservice.GetMiniMaxTenantRequestObject) (adminservice.GetMiniMaxTenantResponseObject, error) {
	return f.minimax, nil
}

func (f responseTenants) GetVolcTenant(context.Context, adminservice.GetVolcTenantRequestObject) (adminservice.GetVolcTenantResponseObject, error) {
	return f.volc, nil
}

type fakeBuilder struct {
	events *[]string
}

func (b fakeBuilder) BuildGenerator(_ context.Context, cfg GeneratorConfig) (genx.Generator, error) {
	*b.events = append(*b.events, "build:generator:"+cfg.Model.Id)
	return fakeGenerator{events: b.events}, nil
}

func (b fakeBuilder) BuildTransformer(_ context.Context, cfg TransformerConfig) (genx.Transformer, error) {
	if cfg.Model != nil {
		*b.events = append(*b.events, "build:transformer:model:"+cfg.Model.Id)
	} else if cfg.Voice != nil {
		*b.events = append(*b.events, "build:transformer:voice:"+cfg.Voice.Id)
	}
	return fakeTransformer{events: b.events}, nil
}

type fakeGenerator struct {
	events *[]string
}

func (g fakeGenerator) GenerateStream(_ context.Context, pattern string, _ genx.ModelContext) (genx.Stream, error) {
	*g.events = append(*g.events, "call:generator:"+pattern)
	return fakeStream{}, nil
}

func (g fakeGenerator) Invoke(_ context.Context, pattern string, _ genx.ModelContext, _ *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	*g.events = append(*g.events, "call:invoke:"+pattern)
	return genx.Usage{}, nil, nil
}

type fakeTransformer struct {
	events *[]string
}

func (t fakeTransformer) Transform(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	*t.events = append(*t.events, "call:transformer:"+pattern)
	return input, nil
}

type fakeStream struct{}

func (fakeStream) Next() (*genx.MessageChunk, error) {
	return nil, errors.New("unused")
}

func (fakeStream) Close() error {
	return nil
}

func (fakeStream) CloseWithError(error) error {
	return nil
}
