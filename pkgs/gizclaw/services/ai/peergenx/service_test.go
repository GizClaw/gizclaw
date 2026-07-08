package peergenx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
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
		"auth:model:chat:read",
		"get:model:chat",
		"auth:model:chat:use",
		"get:tenant:openai:main",
		"auth:credential:openai-key:read",
		"auth:credential:openai-key:use",
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
		Authorizer: &recordingAuthorizer{events: &events, deny: "auth:model:chat:read"},
		Models:     fakeModels{events: &events},
		Builder:    fakeBuilder{events: &events},
	})

	_, err := svc.Generator().GenerateStream(ctx, "model/chat", nil)
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("GenerateStream() error = %v, want %v", err, ErrDenied)
	}
	want := []string{"auth:model:chat:read"}
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
		"auth:voice:cancan:read",
		"get:voice:cancan",
		"auth:voice:cancan:use",
		"get:tenant:volc:main",
		"auth:credential:volc-token:read",
		"auth:credential:volc-token:use",
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
		"auth:model:asr:read",
		"get:model:asr",
		"auth:model:asr:use",
		"get:tenant:volc:main",
		"auth:credential:volc-token:read",
		"auth:credential:volc-token:use",
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
		"auth:model:realtime:read",
		"get:model:realtime",
		"auth:model:realtime:use",
		"get:tenant:volc:main",
		"auth:credential:volc-token:read",
		"auth:credential:volc-token:use",
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
		"auth:voice:minimax:read",
		"get:voice:minimax",
		"auth:voice:minimax:use",
		"get:tenant:minimax:main",
		"auth:credential:minimax-key:read",
		"auth:credential:minimax-key:use",
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
		Authorizer:      &recordingAuthorizer{events: &events, deny: "auth:voice:cancan:use"},
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
		"auth:voice:cancan:read",
		"get:voice:cancan",
		"auth:voice:cancan:use",
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
			ProviderData: mustOpenAIModelProviderData(t, apitypes.OpenAITenantModelProviderData{
				UpstreamModel:        &upstream,
				ThinkingParam:        stringPtr("thinking.type"),
				DefaultThinkingLevel: stringPtr("disabled"),
			}),
		},
		Tenant: Tenant{
			Kind:   "openai-tenant",
			OpenAI: &apitypes.OpenAITenant{Name: "main", CredentialName: "openai-key"},
		},
		Credential: apitypes.Credential{
			Name: "openai-key",
			Body: testOpenAICredentialBody("sk-test"),
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
	thinking, ok := openaiGen.ExtraFields["thinking"].(map[string]any)
	if !ok || thinking["type"] != "disabled" {
		t.Fatalf("OpenAIGenerator ExtraFields = %#v, want thinking.type=disabled", openaiGen.ExtraFields)
	}
}

func TestDefaultBuilderBuildsVolcArkGenerator(t *testing.T) {
	trueValue := true
	upstream := "doubao-test"
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
			ProviderData: mustVolcModelProviderData(t, apitypes.VolcTenantModelProviderData{
				UpstreamModel:        &upstream,
				ThinkingParam:        stringPtr("thinking.type"),
				DefaultThinkingLevel: stringPtr("disabled"),
			}),
		},
		Tenant: Tenant{
			Kind: "volc-tenant",
			Volc: &apitypes.VolcTenant{
				Name:           "main",
				CredentialName: "volc-key",
			},
		},
		Credential: apitypes.Credential{
			Name: "volc-key",
			Body: testVolcCredentialBodyFromStrings(map[string]string{"app_id": "app", "api_key": "ark-test"}),
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
	thinking, ok := openaiGen.ExtraFields["thinking"].(map[string]any)
	if !ok || thinking["type"] != "disabled" {
		t.Fatalf("OpenAIGenerator ExtraFields = %#v, want thinking.type=disabled", openaiGen.ExtraFields)
	}
}

func TestDefaultBuilderBuildsVolcASRTransformer(t *testing.T) {
	tf, err := (DefaultBuilder{}).BuildTransformer(context.Background(), TransformerConfig{
		Model: &apitypes.Model{
			Id:   "asr",
			Kind: apitypes.ModelKindAsr,
			ProviderData: mustVolcModelProviderData(t, apitypes.VolcTenantModelProviderData{
				ResourceId: stringPtr("volc.bigasr.sauc.duration"),
			}),
		},
		Tenant: Tenant{
			Kind: "volc-tenant",
			Volc: &apitypes.VolcTenant{
				Name:           "main",
				CredentialName: "volc-token",
			},
		},
		Credential: apitypes.Credential{
			Name: "volc-token",
			Body: testVolcCredentialBodyFromStrings(map[string]string{"app_id": "app", "api_key": "tok"}),
		},
	})
	if err != nil {
		t.Fatalf("BuildTransformer() error = %v", err)
	}
	if tf == nil {
		t.Fatal("BuildTransformer() transformer = nil")
	}
	if got := transformerStringField(t, tf, "format"); got != "pcm" {
		t.Fatalf("ASR format = %q, want pcm", got)
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
	if got := transformerNestedStringField(t, tf, "client", "config", "apiKey"); got != "tok" {
		t.Fatalf("ASR api key = %q, want tok", got)
	}
}

func TestDefaultBuilderBuildsVolcASRTransformerFromParams(t *testing.T) {
	tf, err := (DefaultBuilder{}).BuildTransformer(context.Background(), TransformerConfig{
		Model: &apitypes.Model{
			Id:   "asr",
			Kind: apitypes.ModelKindAsr,
			ProviderData: mustVolcModelProviderData(t, apitypes.VolcTenantModelProviderData{
				ResourceId: stringPtr("volc.bigasr.sauc.duration"),
			}),
		},
		Tenant: Tenant{
			Kind: "volc-tenant",
			Volc: &apitypes.VolcTenant{
				Name:           "main",
				CredentialName: "volc-token",
			},
		},
		Credential: apitypes.Credential{
			Name: "volc-token",
			Body: testVolcCredentialBodyFromStrings(map[string]string{"app_id": "app", "api_key": "tok"}),
		},
		Params: map[string]any{
			"format":          "pcm",
			"sample_rate":     24000,
			"channels":        1,
			"bits":            16,
			"language":        "ja-JP",
			"result_type":     "full",
			"emit_interim":    true,
			"realtime_pacing": false,
		},
	})
	if err != nil {
		t.Fatalf("BuildTransformer() error = %v", err)
	}
	if got := transformerStringField(t, tf, "format"); got != "pcm" {
		t.Fatalf("ASR format = %q, want pcm", got)
	}
	if got := transformerIntField(t, tf, "sampleRate"); got != 24000 {
		t.Fatalf("ASR sampleRate = %d, want 24000", got)
	}
	if got := transformerStringField(t, tf, "language"); got != "ja-JP" {
		t.Fatalf("ASR language = %q, want ja-JP", got)
	}
	if got := transformerStringField(t, tf, "resultType"); got != "full" {
		t.Fatalf("ASR resultType = %q, want full", got)
	}
	if !transformerBoolField(t, tf, "emitInterim") {
		t.Fatal("ASR emitInterim = false, want true")
	}
	if transformerBoolField(t, tf, "realtimePacing") {
		t.Fatal("ASR realtimePacing = true, want false")
	}
}

func TestDefaultBuilderBuildsVolcRealtimeTransformer(t *testing.T) {
	tf, err := (DefaultBuilder{}).BuildTransformer(context.Background(), TransformerConfig{
		Model: &apitypes.Model{
			Id:   "dialog",
			Kind: apitypes.ModelKindRealtime,
			ProviderData: mustVolcModelProviderData(t, apitypes.VolcTenantModelProviderData{
				UpstreamModel: stringPtr("O"),
			}),
		},
		Tenant: Tenant{
			Kind: "volc-tenant",
			Volc: &apitypes.VolcTenant{
				Name:           "main",
				CredentialName: "volc-token",
			},
		},
		Credential: apitypes.Credential{
			Name: "volc-token",
			Body: testVolcCredentialBodyFromStrings(map[string]string{"app_id": "app", "api_key": "runtime-key"}),
		},
		Params: map[string]any{
			"output_voice": "speaker-id",
		},
	})
	if err != nil {
		t.Fatalf("BuildTransformer() error = %v", err)
	}
	if _, ok := tf.(*transformers.DoubaoRealtime); !ok {
		t.Fatalf("transformer = %T, want *transformers.DoubaoRealtime", tf)
	}
	if got := transformerStringField(t, tf, "model"); got != "O" {
		t.Fatalf("realtime model = %q, want O", got)
	}
	if got := transformerStringField(t, tf, "speaker"); got != "speaker-id" {
		t.Fatalf("realtime speaker = %q, want speaker-id", got)
	}
	if got := transformerStringField(t, tf, "mode"); got != "push_to_talk" {
		t.Fatalf("realtime mode = %q, want push_to_talk", got)
	}
	if got := transformerNestedStringField(t, tf, "client", "config", "apiKey"); got != "runtime-key" {
		t.Fatalf("realtime api key = %q, want runtime-key", got)
	}
}

func TestDefaultBuilderBuildsVolcRealtimeTransformerUsesAPIKey(t *testing.T) {
	tf, err := (DefaultBuilder{}).BuildTransformer(context.Background(), TransformerConfig{
		Model: &apitypes.Model{
			Id:   "dialog",
			Kind: apitypes.ModelKindRealtime,
			ProviderData: mustVolcModelProviderData(t, apitypes.VolcTenantModelProviderData{
				ResourceId:    stringPtr("volc.speech.dialog"),
				UpstreamModel: stringPtr("O"),
			}),
		},
		Tenant: Tenant{
			Kind: "volc-tenant",
			Volc: &apitypes.VolcTenant{Name: "main"},
		},
		Credential: apitypes.Credential{
			Name: "volc-token",
			Body: testVolcCredentialBodyFromStrings(map[string]string{"app_id": "app", "api_key": "speech-runtime-key"}),
		},
	})
	if err != nil {
		t.Fatalf("BuildTransformer() error = %v", err)
	}
	if got := transformerNestedStringField(t, tf, "client", "config", "apiKey"); got != "speech-runtime-key" {
		t.Fatalf("realtime api key = %q, want speech-runtime-key", got)
	}
}

func TestDefaultBuilderBuildsVolcRealtimeTransformerFromWorkflowParams(t *testing.T) {
	tf, err := (DefaultBuilder{}).BuildTransformer(context.Background(), TransformerConfig{
		Model: &apitypes.Model{
			Id:   "dialog",
			Kind: apitypes.ModelKindRealtime,
			ProviderData: mustVolcModelProviderData(t, apitypes.VolcTenantModelProviderData{
				ResourceId:    stringPtr("volc.speech.dialog"),
				UpstreamModel: stringPtr("O"),
			}),
		},
		Tenant: Tenant{
			Kind: "volc-tenant",
			Volc: &apitypes.VolcTenant{
				Name:           "main",
				CredentialName: "volc-token",
			},
		},
		Credential: apitypes.Credential{
			Name: "volc-token",
			Body: testVolcCredentialBodyFromStrings(map[string]string{
				"app_id":         "app",
				"api_key":        "realtime-key",
				"search_api_key": "search-key",
			}),
		},
		Params: map[string]any{
			"instructions":       "简短回答。",
			"output_voice":       "workflow-speaker",
			"output_format":      "ogg_opus",
			"output_sample_rate": 24000,
			"input_format":       "speech_opus",
			"input_sample_rate":  16000,
			"input_channels":     1,
			"input_transcode":    true,
			"output_speed":       12,
			"output_loudness":    6,
			"dialog_id":          "workspace-dialog-id",
			"mode":               "realtime",
			"extension":          `{"asr":{"extra":{"end_smooth_window_ms":800,"enable_custom_vad":true,"enable_asr_twopass":true,"context":{"hotwords":[{"word":"GizClaw"}],"correct_words":{"吉斯克劳":"GizClaw"}}}},"tts":{"extra":{"explicit_dialect":"sichuan","tts_2.0_model":"expressive","aigc_metadata":{"enable":true,"content_producer":"gizclaw","produce_id":"produce-1"}}},"dialog":{"extra":{"strict_audit":false,"enable_volc_websearch":true,"volc_websearch_type":"web","volc_websearch_result_count":3,"volc_websearch_no_result_message":"没有找到相关搜索结果。","enable_conversation_truncate":true,"enable_user_query_exit":true}}}`,
		},
	})
	if err != nil {
		t.Fatalf("BuildTransformer() error = %v", err)
	}
	if _, ok := tf.(*transformers.DoubaoRealtime); !ok {
		t.Fatalf("transformer = %T, want *transformers.DoubaoRealtime", tf)
	}
	if got := transformerNestedStringField(t, tf, "client", "config", "resourceID"); got != "volc.speech.dialog" {
		t.Fatalf("realtime resourceID = %q, want volc.speech.dialog", got)
	}
	if got := transformerNestedStringField(t, tf, "client", "config", "apiKey"); got != "realtime-key" {
		t.Fatalf("realtime api key = %q, want realtime-key", got)
	}
	if got := transformerStringField(t, tf, "model"); got != "O" {
		t.Fatalf("realtime model = %q, want O", got)
	}
	if got := transformerStringField(t, tf, "systemRole"); got != "简短回答。" {
		t.Fatalf("realtime systemRole = %q, want 简短回答。", got)
	}
	if got := transformerStringField(t, tf, "dialogID"); got != "workspace-dialog-id" {
		t.Fatalf("realtime dialogID = %q, want workspace-dialog-id", got)
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
	if got := transformerStringField(t, tf, "mode"); got != "realtime" {
		t.Fatalf("realtime mode = %q, want realtime", got)
	}
	if got := transformerNestedIntPointerField(t, tf, "speechRate"); got != 12 {
		t.Fatalf("realtime speechRate = %d, want 12", got)
	}
	if got := transformerNestedIntPointerField(t, tf, "loudnessRate"); got != 6 {
		t.Fatalf("realtime loudnessRate = %d, want 6", got)
	}
	if got := transformerNestedIntField(t, tf, "asrExtra", "EndSmoothWindowMS"); got != 800 {
		t.Fatalf("realtime asrExtra.EndSmoothWindowMS = %d, want 800", got)
	}
	if !transformerNestedBoolPointerField(t, tf, "asrExtra", "EnableCustomVAD") {
		t.Fatal("realtime asrExtra.EnableCustomVAD = false, want true")
	}
	if !transformerNestedBoolPointerField(t, tf, "asrExtra", "EnableASRTwopass") {
		t.Fatal("realtime asrExtra.EnableASRTwopass = false, want true")
	}
	if got := transformerNestedStringField(t, tf, "ttsExtra", "ExplicitDialect"); got != "sichuan" {
		t.Fatalf("realtime ttsExtra.ExplicitDialect = %q, want sichuan", got)
	}
	if got := transformerNestedStringField(t, tf, "ttsExtra", "TTS20Model"); got != "expressive" {
		t.Fatalf("realtime ttsExtra.TTS20Model = %q, want expressive", got)
	}
	if !transformerNestedBoolPointerField(t, tf, "ttsExtra", "AIGCMetadata", "Enable") {
		t.Fatal("realtime ttsExtra.AIGCMetadata.Enable = false, want true")
	}
	if !transformerNestedBoolPointerField(t, tf, "dialogExtra", "EnableVolcWebsearch") {
		t.Fatal("realtime dialogExtra.EnableVolcWebsearch = false, want true")
	}
	if transformerNestedBoolPointerField(t, tf, "dialogExtra", "StrictAudit") {
		t.Fatal("realtime dialogExtra.StrictAudit = true, want false")
	}
	if !transformerNestedBoolPointerField(t, tf, "dialogExtra", "EnableConversationTruncate") {
		t.Fatal("realtime dialogExtra.EnableConversationTruncate = false, want true")
	}
	if !transformerNestedBoolPointerField(t, tf, "dialogExtra", "EnableUserQueryExit") {
		t.Fatal("realtime dialogExtra.EnableUserQueryExit = false, want true")
	}
	if got := transformerNestedStringField(t, tf, "dialogExtra", "VolcWebsearchType"); got != "web" {
		t.Fatalf("realtime dialogExtra.VolcWebsearchType = %q, want web", got)
	}
	if got := transformerNestedStringField(t, tf, "dialogExtra", "VolcWebsearchAPIKey"); got != "search-key" {
		t.Fatalf("realtime dialogExtra.VolcWebsearchAPIKey = %q, want search-key", got)
	}
	if got := transformerNestedIntField(t, tf, "dialogExtra", "VolcWebsearchResultCount"); got != 3 {
		t.Fatalf("realtime dialogExtra.VolcWebsearchResultCount = %d, want 3", got)
	}
}

func TestDefaultBuilderRejectsUnsupportedVolcRealtimeMode(t *testing.T) {
	_, err := (DefaultBuilder{}).BuildTransformer(context.Background(), TransformerConfig{
		Model: &apitypes.Model{
			Id:   "dialog",
			Kind: apitypes.ModelKindRealtime,
			ProviderData: mustVolcModelProviderData(t, apitypes.VolcTenantModelProviderData{
				UpstreamModel: stringPtr("O"),
			}),
		},
		Tenant: Tenant{
			Kind: "volc-tenant",
			Volc: &apitypes.VolcTenant{
				Name:           "main",
				CredentialName: "volc-token",
			},
		},
		Credential: apitypes.Credential{
			Name: "volc-token",
			Body: testVolcCredentialBodyFromStrings(map[string]string{"app_id": "app", "api_key": "realtime-key"}),
		},
		Params: map[string]any{"mode": "bad"},
	})
	if err == nil || !strings.Contains(err.Error(), `doubao realtime mode "bad"`) {
		t.Fatalf("BuildTransformer() error = %v, want unsupported mode", err)
	}
}

func TestDefaultBuilderRejectsVolcRealtimeMissingUpstreamModel(t *testing.T) {
	_, err := (DefaultBuilder{}).BuildTransformer(context.Background(), TransformerConfig{
		Model: &apitypes.Model{
			Id:   "dialog",
			Kind: apitypes.ModelKindRealtime,
			ProviderData: mustVolcModelProviderData(t, apitypes.VolcTenantModelProviderData{
				ResourceId: stringPtr("volc.speech.dialog"),
			}),
		},
		Tenant: Tenant{
			Kind: "volc-tenant",
			Volc: &apitypes.VolcTenant{
				Name:           "main",
				CredentialName: "volc-token",
			},
		},
		Credential: apitypes.Credential{
			Name: "volc-token",
			Body: testVolcCredentialBodyFromStrings(map[string]string{"app_id": "app", "api_key": "realtime-key"}),
		},
	})
	if err == nil || !strings.Contains(err.Error(), `missing upstream_model for doubao realtime`) {
		t.Fatalf("BuildTransformer() error = %v, want missing upstream_model", err)
	}
}

func TestDefaultBuilderBuildsGeminiGenerator(t *testing.T) {
	upstream := "gemini-test"
	gen, err := (DefaultBuilder{}).BuildGenerator(context.Background(), GeneratorConfig{
		Model: apitypes.Model{
			Id:   "gemini",
			Kind: apitypes.ModelKindLlm,
			ProviderData: mustGeminiModelProviderData(t, apitypes.GeminiTenantModelProviderData{
				UpstreamModel: &upstream,
			}),
		},
		Tenant: Tenant{
			Kind:   "gemini-tenant",
			Gemini: &apitypes.GeminiTenant{Name: "main", CredentialName: "gemini-key"},
		},
		Credential: apitypes.Credential{
			Name: "gemini-key",
			Body: testGeminiCredentialBody("gemini-token"),
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
					ProviderData: mustVolcVoiceProviderData(t, apitypes.VolcTenantVoiceProviderData{
						VoiceId:    stringPtr("voice-id"),
						ResourceId: stringPtr("seed-icl-2.0"),
					}),
				},
				Tenant: Tenant{
					Kind: "volc-tenant",
					Volc: &apitypes.VolcTenant{Name: "main", CredentialName: "volc-token"},
				},
				Credential: apitypes.Credential{Name: "volc-token", Body: testVolcCredentialBodyFromStrings(map[string]string{"app_id": "app", "api_key": "tok"})},
			},
			wantFormat:     defaultVolcTTSAudioFormat,
			wantSampleRate: defaultTTSAudioSampleRate,
			wantResourceID: "seed-icl-2.0",
		},
		{
			name: "minimax",
			cfg: TransformerConfig{
				Voice: &apitypes.Voice{
					Id: "minimax-voice",
					ProviderData: mustMiniMaxVoiceProviderData(t, apitypes.MiniMaxTenantVoiceProviderData{
						VoiceId: stringPtr("voice-id"),
						Model:   stringPtr("speech-02-hd"),
					}),
				},
				Tenant: Tenant{
					Kind:    "minimax-tenant",
					MiniMax: &apitypes.MiniMaxTenant{Name: "main", CredentialName: "minimax-key", BaseUrl: &baseURL},
				},
				Credential: apitypes.Credential{Name: "minimax-key", Body: testMiniMaxCredentialBody("sk-test")},
			},
			wantFormat:     defaultMiniMaxTTSAudioFormat,
			wantSampleRate: defaultTTSAudioSampleRate,
			wantModel:      "speech-02-hd",
			wantBaseURL:    baseURL,
		},
		{
			name: "minimax default base url",
			cfg: TransformerConfig{
				Voice: &apitypes.Voice{
					Id: "minimax-voice",
					ProviderData: mustMiniMaxVoiceProviderData(t, apitypes.MiniMaxTenantVoiceProviderData{
						VoiceId: stringPtr("voice-id"),
					}),
				},
				Tenant: Tenant{
					Kind:    "minimax-tenant",
					MiniMax: &apitypes.MiniMaxTenant{Name: "main", CredentialName: "minimax-key"},
				},
				Credential: apitypes.Credential{Name: "minimax-key", Body: testMiniMaxCredentialBody("sk-test")},
			},
			wantFormat:     defaultMiniMaxTTSAudioFormat,
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
				Credential: apitypes.Credential{Body: testOpenAICredentialBody("sk-test")},
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
			name: "volc asr missing api key",
			cfg: TransformerConfig{
				Model: &apitypes.Model{Id: "asr", Kind: apitypes.ModelKindAsr},
				Tenant: Tenant{
					Kind: string(apitypes.VoiceProviderKindVolcTenant),
					Volc: &apitypes.VolcTenant{Name: "main"},
				},
			},
		},
		{
			name: "volc tts missing voice id",
			cfg: TransformerConfig{
				Voice: &apitypes.Voice{Id: "voice"},
				Tenant: Tenant{
					Kind: string(apitypes.VoiceProviderKindVolcTenant),
					Volc: &apitypes.VolcTenant{Name: "main"},
				},
				Credential: apitypes.Credential{Body: testVolcCredentialBodyFromStrings(map[string]string{"app_id": "app", "api_key": "tok"})},
			},
		},
		{
			name: "minimax missing credential",
			cfg: TransformerConfig{
				Voice: &apitypes.Voice{
					Id: "voice",
					ProviderData: mustMiniMaxVoiceProviderData(t, apitypes.MiniMaxTenantVoiceProviderData{
						VoiceId: stringPtr("voice-id"),
					}),
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

func transformerNestedIntField(t *testing.T, tf genx.Transformer, fieldNames ...string) int {
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
	if value.Kind() != reflect.Int {
		t.Fatalf("transformer %T field path %v is %s, want int", tf, fieldNames, value.Kind())
	}
	return int(value.Int())
}

func transformerNestedIntPointerField(t *testing.T, tf genx.Transformer, fieldNames ...string) int {
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
	if value.Kind() != reflect.Int {
		t.Fatalf("transformer %T field path %v is %s, want int", tf, fieldNames, value.Kind())
	}
	return int(value.Int())
}

func transformerNestedBoolPointerField(t *testing.T, tf genx.Transformer, fieldNames ...string) bool {
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
	if value.Kind() != reflect.Bool {
		t.Fatalf("transformer %T field path %v is %s, want bool", tf, fieldNames, value.Kind())
	}
	return value.Bool()
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
		"auth:model:chat:read",
		"get:model:chat",
		"auth:model:chat:use",
		"get:tenant:openai:main",
		"auth:credential:openai-key:read",
		"auth:credential:openai-key:use",
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

func TestListAccessibleGeneratorConfigsEnumeratesAuthorizedLLMs(t *testing.T) {
	ctx := context.Background()
	events := []string{}
	svc := New(Service{
		Peer:       newTestPeer(),
		Authorizer: &recordingAuthorizer{events: &events, deny: "auth:model:denied:read"},
		Models: fakeModels{events: &events, listItems: []apitypes.Model{
			testModel("chat", apitypes.ModelKindLlm),
			testModel("asr", apitypes.ModelKindAsr),
			testModel("denied", apitypes.ModelKindLlm),
		}},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
	})

	got, err := svc.ListAccessibleGeneratorConfigs(ctx)
	if err != nil {
		t.Fatalf("ListAccessibleGeneratorConfigs() error = %v", err)
	}
	if len(got) != 1 || got[0].Model.Id != "chat" || got[0].Pattern != "model/chat" {
		t.Fatalf("ListAccessibleGeneratorConfigs() = %#v, want only chat", got)
	}
	want := []string{
		"list:models",
		"auth:model:chat:read",
		"auth:model:chat:use",
		"get:tenant:openai:main",
		"auth:credential:openai-key:read",
		"auth:credential:openai-key:use",
		"get:credential:openai-key",
		"auth:model:denied:read",
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

func TestListAccessibleGeneratorConfigsErrorsAndUseDenial(t *testing.T) {
	ctx := context.Background()
	if _, err := (*Service)(nil).ListAccessibleGeneratorConfigs(ctx); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("nil ListAccessibleGeneratorConfigs() error = %v, want %v", err, ErrNotConfigured)
	}
	if _, err := (&Service{}).ListAccessibleGeneratorConfigs(ctx); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("missing models ListAccessibleGeneratorConfigs() error = %v, want %v", err, ErrNotConfigured)
	}
	if _, err := (&Service{Models: responseModels{}}).ListAccessibleGeneratorConfigs(ctx); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("missing lister ListAccessibleGeneratorConfigs() error = %v, want %v", err, ErrNotConfigured)
	}
	svc := &Service{Models: responseModelLister{response: adminhttp.ListModels500JSONResponse{}}}
	if _, err := svc.ListAccessibleGeneratorConfigs(ctx); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad list response error = %v, want %v", err, ErrInvalid)
	}

	events := []string{}
	deniedUse := New(Service{
		Peer:       newTestPeer(),
		Authorizer: &recordingAuthorizer{events: &events, deny: "auth:model:denied:use"},
		Models: fakeModels{events: &events, listItems: []apitypes.Model{
			testModel("denied", apitypes.ModelKindLlm),
		}},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
	})
	got, err := deniedUse.ListAccessibleGeneratorConfigs(ctx)
	if err != nil {
		t.Fatalf("denied use ListAccessibleGeneratorConfigs() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("denied use configs = %#v, want empty", got)
	}
	want := []string{
		"list:models",
		"auth:model:denied:read",
		"auth:model:denied:use",
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("denied use events = %#v, want %#v", events, want)
	}
}

func TestBuilderHelpersHandleJSONNumberAndInvalidVoiceData(t *testing.T) {
	number := json.Number("42")
	if got, ok := mapInt(map[string]any{"n": number}, "n"); !ok || got != 42 {
		t.Fatalf("mapInt(json.Number) = %d, %v; want 42, true", got, ok)
	}
	if got, ok := parsePattern(" voice/cancan ", "voice"); !ok || got != "cancan" {
		t.Fatalf("parsePattern() = %q, %v; want cancan, true", got, ok)
	}
	if got, ok := parsePattern("voice/ ", "voice"); ok || got != "" {
		t.Fatalf("parsePattern(empty id) = %q, %v; want empty, false", got, ok)
	}
	if got, ok := parsePattern("model/realtime?output_sample_rate=24000", "model"); !ok || got != "realtime" {
		t.Fatalf("parsePattern(query) = %q, %v; want realtime, true", got, ok)
	}
	base, params, err := splitPatternParams("model/realtime?input_transcode=true&output_sample_rate=24000&instructions=%E7%AE%80%E7%9F%AD")
	if err != nil {
		t.Fatalf("splitPatternParams() error = %v", err)
	}
	if base != "model/realtime" {
		t.Fatalf("splitPatternParams base = %q, want model/realtime", base)
	}
	if got, ok := params["input_transcode"].(bool); !ok || !got {
		t.Fatalf("input_transcode param = %#v", params["input_transcode"])
	}
	if got, ok := params["output_sample_rate"].(int); !ok || got != 24000 {
		t.Fatalf("output_sample_rate param = %#v", params["output_sample_rate"])
	}
	if got, ok := params["instructions"].(string); !ok || got != "简短" {
		t.Fatalf("instructions param = %#v", params["instructions"])
	}
	if !isDenied(fmt.Errorf("wrapped: %w", ErrDenied)) {
		t.Fatal("isDenied(wrapped ErrDenied) = false, want true")
	}
	if isDenied(ErrInvalid) {
		t.Fatal("isDenied(ErrInvalid) = true, want false")
	}
}

func TestBuilderBooleanHelperBranches(t *testing.T) {
	if got, ok := mapBool(map[string]any{"a": "yes"}, "missing", "a"); !ok || !got {
		t.Fatalf("mapBool(yes) = %t, %t; want true, true", got, ok)
	}
	if got, ok := mapBool(map[string]any{"a": "off"}, "a"); !ok || got {
		t.Fatalf("mapBool(off) = %t, %t; want false, true", got, ok)
	}
	if got, ok := mapBool(map[string]any{"a": "maybe"}); ok || got {
		t.Fatalf("mapBool(maybe) = %t, %t; want false, false", got, ok)
	}
	if boolValue(nil, boolPtr(true)) != true || boolValue(nil) != false {
		t.Fatal("boolValue() returned unexpected result")
	}
	caps := &apitypes.ModelCapabilities{
		JsonOutput: boolPtr(true),
		ToolCalls:  boolPtr(false),
		TextOnly:   boolPtr(true),
		SystemRole: boolPtr(true),
	}
	for _, name := range []string{"json", "tools", "text", "system"} {
		if capabilityBool(caps, name) == nil {
			t.Fatalf("capabilityBool(%s) = nil", name)
		}
	}
	if capabilityBool(caps, "unknown") != nil || capabilityBool(nil, "json") != nil {
		t.Fatal("capabilityBool unknown/nil returned non-nil")
	}
	if openAIPromptRole(boolPtr(true)) != genx.PromptRoleSystem || openAIPromptRole(boolPtr(false)) != "" {
		t.Fatal("openAIPromptRole() returned unexpected result")
	}
	if got := openAIThinkingValue("enable_thinking", "off"); got != false {
		t.Fatalf("openAIThinkingValue(enable_thinking, off) = %#v, want false", got)
	}
	for _, value := range []string{"disabled", "disable", "off", "false", "0", "none", "no"} {
		if !isDisabledThinkingLevel(value) {
			t.Fatalf("isDisabledThinkingLevel(%q) = false, want true", value)
		}
	}
	if isDisabledThinkingLevel("auto") {
		t.Fatal("isDisabledThinkingLevel(auto) = true, want false")
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
		svc := &Service{Models: responseModels{response: adminhttp.GetModel404JSONResponse{}}}
		if _, err := svc.getModel(ctx, "missing"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("getModel(404) error = %v, want %v", err, ErrNotFound)
		}
		svc.Models = responseModels{response: adminhttp.GetModel500JSONResponse{}}
		if _, err := svc.getModel(ctx, "bad"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("getModel(500) error = %v, want %v", err, ErrInvalid)
		}
	})
	t.Run("voice", func(t *testing.T) {
		svc := &Service{Voices: responseVoices{response: adminhttp.GetVoice404JSONResponse{}}}
		if _, err := svc.getVoice(ctx, "missing"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("getVoice(404) error = %v, want %v", err, ErrNotFound)
		}
		svc.Voices = responseVoices{response: adminhttp.GetVoice500JSONResponse{}}
		if _, err := svc.getVoice(ctx, "bad"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("getVoice(500) error = %v, want %v", err, ErrInvalid)
		}
	})
	t.Run("credential", func(t *testing.T) {
		svc := &Service{Credentials: responseCredentials{response: adminhttp.GetCredential404JSONResponse{}}}
		if _, err := svc.getCredential(ctx, "missing"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("getCredential(404) error = %v, want %v", err, ErrNotFound)
		}
		svc.Credentials = responseCredentials{response: adminhttp.GetCredential500JSONResponse{}}
		if _, err := svc.getCredential(ctx, "bad"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("getCredential(500) error = %v, want %v", err, ErrInvalid)
		}
	})
	t.Run("tenants", func(t *testing.T) {
		svc := &Service{ProviderTenants: responseTenants{
			openai:    adminhttp.GetOpenAITenant404JSONResponse{},
			gemini:    adminhttp.GetGeminiTenant404JSONResponse{},
			dashscope: adminhttp.GetDashScopeTenant404JSONResponse{},
			minimax:   adminhttp.GetMiniMaxTenant404JSONResponse{},
			volc:      adminhttp.GetVolcTenant404JSONResponse{},
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
			openai:    adminhttp.GetOpenAITenant500JSONResponse{},
			gemini:    adminhttp.GetGeminiTenant500JSONResponse{},
			dashscope: adminhttp.GetDashScopeTenant500JSONResponse{},
			minimax:   adminhttp.GetMiniMaxTenant500JSONResponse{},
			volc:      adminhttp.GetVolcTenant500JSONResponse{},
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
	listItems    []apitypes.Model
}

func (f fakeModels) GetModel(_ context.Context, request adminhttp.GetModelRequestObject) (adminhttp.GetModelResponseObject, error) {
	*f.events = append(*f.events, "get:model:"+request.Id)
	return adminhttp.GetModel200JSONResponse(f.model(request.Id)), nil
}

func (f fakeModels) ListModels(_ context.Context, request adminhttp.ListModelsRequestObject) (adminhttp.ListModelsResponseObject, error) {
	*f.events = append(*f.events, "list:models")
	if request.Params.Cursor != nil && *request.Params.Cursor != "" {
		return adminhttp.ListModels200JSONResponse(adminhttp.ModelList{}), nil
	}
	items := f.listItems
	if items == nil {
		items = []apitypes.Model{f.model("chat")}
	}
	return adminhttp.ListModels200JSONResponse(adminhttp.ModelList{Items: items}), nil
}

func (f fakeModels) model(id string) apitypes.Model {
	kind := f.modelKind
	if kind == "" {
		kind = apitypes.ModelKindLlm
	}
	providerKind := f.providerKind
	if providerKind == "" {
		providerKind = string(apitypes.ModelProviderKindOpenaiTenant)
	}
	return apitypes.Model{
		Id:   id,
		Kind: kind,
		Provider: apitypes.ModelProvider{
			Kind: apitypes.ModelProviderKind(providerKind),
			Name: "main",
		},
	}
}

func testModel(id string, kind apitypes.ModelKind) apitypes.Model {
	return apitypes.Model{
		Id:   id,
		Kind: kind,
		Provider: apitypes.ModelProvider{
			Kind: apitypes.ModelProviderKindOpenaiTenant,
			Name: "main",
		},
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func stringPtr(value string) *string {
	return &value
}

func intPtr(value int) *int {
	return &value
}

func mustOpenAIModelProviderData(t *testing.T, data apitypes.OpenAITenantModelProviderData) *apitypes.ModelProviderData {
	t.Helper()
	out := apitypes.ModelProviderData{}
	if err := out.FromOpenAITenantModelProviderData(data); err != nil {
		t.Fatalf("FromOpenAITenantModelProviderData() error = %v", err)
	}
	return &out
}

func mustVolcModelProviderData(t *testing.T, data apitypes.VolcTenantModelProviderData) *apitypes.ModelProviderData {
	t.Helper()
	out := apitypes.ModelProviderData{}
	if err := out.FromVolcTenantModelProviderData(data); err != nil {
		t.Fatalf("FromVolcTenantModelProviderData() error = %v", err)
	}
	return &out
}

func mustGeminiModelProviderData(t *testing.T, data apitypes.GeminiTenantModelProviderData) *apitypes.ModelProviderData {
	t.Helper()
	out := apitypes.ModelProviderData{}
	if err := out.FromGeminiTenantModelProviderData(data); err != nil {
		t.Fatalf("FromGeminiTenantModelProviderData() error = %v", err)
	}
	return &out
}

func mustVolcVoiceProviderData(t *testing.T, data apitypes.VolcTenantVoiceProviderData) *apitypes.VoiceProviderData {
	t.Helper()
	out := apitypes.VoiceProviderData{}
	if err := out.FromVolcTenantVoiceProviderData(data); err != nil {
		t.Fatalf("FromVolcTenantVoiceProviderData() error = %v", err)
	}
	return &out
}

func mustMiniMaxVoiceProviderData(t *testing.T, data apitypes.MiniMaxTenantVoiceProviderData) *apitypes.VoiceProviderData {
	t.Helper()
	out := apitypes.VoiceProviderData{}
	if err := out.FromMiniMaxTenantVoiceProviderData(data); err != nil {
		t.Fatalf("FromMiniMaxTenantVoiceProviderData() error = %v", err)
	}
	return &out
}

type fakeVoices struct {
	events       *[]string
	providerKind apitypes.VoiceProviderKind
}

func (f fakeVoices) GetVoice(_ context.Context, request adminhttp.GetVoiceRequestObject) (adminhttp.GetVoiceResponseObject, error) {
	*f.events = append(*f.events, "get:voice:"+request.Id)
	providerKind := f.providerKind
	if providerKind == "" {
		providerKind = apitypes.VoiceProviderKindVolcTenant
	}
	providerData := apitypes.VoiceProviderData{}
	voiceID := "voice-id"
	var err error
	switch providerKind {
	case apitypes.VoiceProviderKindMinimaxTenant:
		err = providerData.FromMiniMaxTenantVoiceProviderData(apitypes.MiniMaxTenantVoiceProviderData{VoiceId: &voiceID})
	default:
		err = providerData.FromVolcTenantVoiceProviderData(apitypes.VolcTenantVoiceProviderData{VoiceId: &voiceID})
	}
	if err != nil {
		panic(err)
	}
	return adminhttp.GetVoice200JSONResponse(apitypes.Voice{
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

func (f fakeCredentials) GetCredential(_ context.Context, request adminhttp.GetCredentialRequestObject) (adminhttp.GetCredentialResponseObject, error) {
	*f.events = append(*f.events, "get:credential:"+request.Name)
	return adminhttp.GetCredential200JSONResponse(apitypes.Credential{
		Name: request.Name,
		Body: testVolcCredentialBodyFromStrings(map[string]string{"app_id": "app", "api_key": "sk-test"}),
	}), nil
}

type fakeTenants struct {
	events *[]string
}

func (f fakeTenants) GetOpenAITenant(_ context.Context, request adminhttp.GetOpenAITenantRequestObject) (adminhttp.GetOpenAITenantResponseObject, error) {
	*f.events = append(*f.events, "get:tenant:openai:"+request.Name)
	return adminhttp.GetOpenAITenant200JSONResponse(apitypes.OpenAITenant{Name: request.Name, CredentialName: "openai-key"}), nil
}

func (f fakeTenants) GetGeminiTenant(_ context.Context, request adminhttp.GetGeminiTenantRequestObject) (adminhttp.GetGeminiTenantResponseObject, error) {
	*f.events = append(*f.events, "get:tenant:gemini:"+request.Name)
	return adminhttp.GetGeminiTenant200JSONResponse(apitypes.GeminiTenant{Name: request.Name, CredentialName: "gemini-key"}), nil
}

func (f fakeTenants) GetDashScopeTenant(_ context.Context, request adminhttp.GetDashScopeTenantRequestObject) (adminhttp.GetDashScopeTenantResponseObject, error) {
	*f.events = append(*f.events, "get:tenant:dashscope:"+request.Name)
	return adminhttp.GetDashScopeTenant200JSONResponse(apitypes.DashScopeTenant{Name: request.Name, CredentialName: "dashscope-key"}), nil
}

func (f fakeTenants) GetMiniMaxTenant(_ context.Context, request adminhttp.GetMiniMaxTenantRequestObject) (adminhttp.GetMiniMaxTenantResponseObject, error) {
	*f.events = append(*f.events, "get:tenant:minimax:"+request.Name)
	return adminhttp.GetMiniMaxTenant200JSONResponse(apitypes.MiniMaxTenant{Name: request.Name, CredentialName: "minimax-key"}), nil
}

func (f fakeTenants) GetVolcTenant(_ context.Context, request adminhttp.GetVolcTenantRequestObject) (adminhttp.GetVolcTenantResponseObject, error) {
	*f.events = append(*f.events, "get:tenant:volc:"+request.Name)
	return adminhttp.GetVolcTenant200JSONResponse(apitypes.VolcTenant{Name: request.Name, CredentialName: "volc-token"}), nil
}

type responseModels struct {
	response adminhttp.GetModelResponseObject
}

func (f responseModels) GetModel(context.Context, adminhttp.GetModelRequestObject) (adminhttp.GetModelResponseObject, error) {
	return f.response, nil
}

type responseModelLister struct {
	response adminhttp.ListModelsResponseObject
}

func (f responseModelLister) GetModel(context.Context, adminhttp.GetModelRequestObject) (adminhttp.GetModelResponseObject, error) {
	return adminhttp.GetModel404JSONResponse{}, nil
}

func (f responseModelLister) ListModels(context.Context, adminhttp.ListModelsRequestObject) (adminhttp.ListModelsResponseObject, error) {
	return f.response, nil
}

type responseVoices struct {
	response adminhttp.GetVoiceResponseObject
}

func (f responseVoices) GetVoice(context.Context, adminhttp.GetVoiceRequestObject) (adminhttp.GetVoiceResponseObject, error) {
	return f.response, nil
}

type responseCredentials struct {
	response adminhttp.GetCredentialResponseObject
}

func (f responseCredentials) GetCredential(context.Context, adminhttp.GetCredentialRequestObject) (adminhttp.GetCredentialResponseObject, error) {
	return f.response, nil
}

type responseTenants struct {
	openai    adminhttp.GetOpenAITenantResponseObject
	gemini    adminhttp.GetGeminiTenantResponseObject
	dashscope adminhttp.GetDashScopeTenantResponseObject
	minimax   adminhttp.GetMiniMaxTenantResponseObject
	volc      adminhttp.GetVolcTenantResponseObject
}

func (f responseTenants) GetOpenAITenant(context.Context, adminhttp.GetOpenAITenantRequestObject) (adminhttp.GetOpenAITenantResponseObject, error) {
	return f.openai, nil
}

func (f responseTenants) GetGeminiTenant(context.Context, adminhttp.GetGeminiTenantRequestObject) (adminhttp.GetGeminiTenantResponseObject, error) {
	return f.gemini, nil
}

func (f responseTenants) GetDashScopeTenant(context.Context, adminhttp.GetDashScopeTenantRequestObject) (adminhttp.GetDashScopeTenantResponseObject, error) {
	return f.dashscope, nil
}

func (f responseTenants) GetMiniMaxTenant(context.Context, adminhttp.GetMiniMaxTenantRequestObject) (adminhttp.GetMiniMaxTenantResponseObject, error) {
	return f.minimax, nil
}

func (f responseTenants) GetVolcTenant(context.Context, adminhttp.GetVolcTenantRequestObject) (adminhttp.GetVolcTenantResponseObject, error) {
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
