package peergenx

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func speechRateVolcCredential() apitypes.Credential {
	return apitypes.Credential{Id: "volc-token", Body: testVolcCredentialBodyFromStrings(map[string]string{"speech_app_id": "app", "speech_api_key": "tok"})}
}

func speechRateVolcModel(t *testing.T, kind apitypes.ModelKind) *apitypes.Model {
	return &apitypes.Model{
		Id: "model", Kind: kind,
		ProviderData: mustVolcModelProviderData(t, apitypes.VolcTenantModelProviderData{UpstreamModel: new("O")}),
	}
}

func speechRateDashScopeConfig(t *testing.T, params map[string]any) TransformerConfig {
	t.Helper()
	var providerData apitypes.ModelProviderData
	if err := providerData.FromDashScopeTenantModelProviderData(apitypes.DashScopeTenantModelProviderData{UpstreamModel: new("qwen-omni-turbo-realtime")}); err != nil {
		t.Fatal(err)
	}
	var body apitypes.CredentialBody
	if err := body.FromDashScopeCredentialBody(apitypes.DashScopeCredentialBody{ApiKey: new("sk")}); err != nil {
		t.Fatal(err)
	}
	return TransformerConfig{
		Model:      &apitypes.Model{Id: "omni", Kind: apitypes.ModelKindRealtime, ProviderData: providerData},
		Tenant:     Tenant{Kind: string(apitypes.ModelProviderKindDashscopeTenant), DashScope: &apitypes.DashScopeTenant{Id: "main", CredentialId: "dashscope-key"}},
		Credential: apitypes.Credential{Id: "dashscope-key", Body: body},
		Params:     params,
	}
}

func transformerFloatField(t *testing.T, tf genx.Transformer, fieldName string) float64 {
	t.Helper()
	field := reflect.Indirect(reflect.ValueOf(tf)).FieldByName(fieldName)
	if !field.IsValid() || field.Kind() != reflect.Float64 {
		t.Fatalf("transformer %T missing float64 field %q", tf, fieldName)
	}
	return field.Float()
}

func TestDefaultBuilderMapsSpeechRatePercentToEveryProvider(t *testing.T) {
	rate := map[string]any{SpeechRatePercentParam: 70}
	volcTenant := Tenant{Kind: "volc-tenant", Volc: &apitypes.VolcTenant{Id: "main", CredentialId: "volc-token"}}

	volcTTS, err := (DefaultBuilder{}).BuildTransformer(context.Background(), TransformerConfig{
		Voice:  &apitypes.Voice{Id: "volc-voice", ProviderData: mustVolcVoiceProviderData(t, apitypes.VolcTenantVoiceProviderData{VoiceId: new("voice-id")})},
		Tenant: volcTenant, Credential: speechRateVolcCredential(), Params: rate,
	})
	if err != nil {
		t.Fatalf("volc TTS error = %v", err)
	}
	if got := transformerFloatField(t, volcTTS, "speedRatio"); got != 0.7 {
		t.Fatalf("volc TTS speedRatio = %v, want 0.7", got)
	}

	minimax, err := (DefaultBuilder{}).BuildTransformer(context.Background(), TransformerConfig{
		Voice: &apitypes.Voice{Id: "minimax-voice", ProviderData: mustMiniMaxVoiceProviderData(t, apitypes.MiniMaxTenantVoiceProviderData{
			VoiceId: new("voice-id"), Model: new("speech-2.6-hd"),
		})},
		Tenant:     Tenant{Kind: "minimax-tenant", MiniMax: &apitypes.MiniMaxTenant{Id: "main", CredentialId: "minimax-key"}},
		Credential: apitypes.Credential{Id: "minimax-key", Body: testMiniMaxCredentialBody("sk-test")},
		Params:     rate,
	})
	if err != nil {
		t.Fatalf("minimax TTS error = %v", err)
	}
	if got := transformerFloatField(t, minimax, "speed"); got != 0.7 {
		t.Fatalf("minimax speed = %v, want 0.7", got)
	}

	// The Workspace rate overrides the static provider-specific rate.
	realtime, err := (DefaultBuilder{}).BuildTransformer(context.Background(), TransformerConfig{
		Model: speechRateVolcModel(t, apitypes.ModelKindRealtime), Tenant: volcTenant, Credential: speechRateVolcCredential(),
		Params: map[string]any{"output_speed": 20, SpeechRatePercentParam: 70},
	})
	if err != nil {
		t.Fatalf("volc realtime error = %v", err)
	}
	if got := transformerNestedIntPointerField(t, realtime, "speechRate"); got != -30 {
		t.Fatalf("volc realtime speechRate = %d, want -30", got)
	}

	duplex, err := (DefaultBuilder{}).BuildTransformer(context.Background(), TransformerConfig{
		Model: speechRateVolcModel(t, apitypes.ModelKindRealtimeDuplex), Tenant: volcTenant, Credential: speechRateVolcCredential(),
		Params: map[string]any{"output_speed": 20, SpeechRatePercentParam: 150},
	})
	if err != nil {
		t.Fatalf("volc realtime duplex error = %v", err)
	}
	if got := transformerNestedIntPointerField(t, duplex, "outputSpeed"); got != 50 {
		t.Fatalf("volc realtime duplex outputSpeed = %d, want 50", got)
	}

	ast, err := (DefaultBuilder{}).BuildTransformer(context.Background(), TransformerConfig{
		Model: speechRateVolcModel(t, apitypes.ModelKindTranslation), Tenant: volcTenant, Credential: speechRateVolcCredential(),
		Params: map[string]any{"speech_rate": 10, SpeechRatePercentParam: 50, "lang_pair": "zh/en"},
	})
	if err != nil {
		t.Fatalf("volc AST error = %v", err)
	}
	if got := transformerIntField(t, ast, "speechRate"); got != -50 {
		t.Fatalf("volc AST speechRate = %d, want -50", got)
	}

	dashScope, err := (DefaultBuilder{}).BuildTransformer(context.Background(), speechRateDashScopeConfig(t, rate))
	if err != nil {
		t.Fatalf("dashscope realtime error = %v", err)
	}
	if got := transformerIntField(t, dashScope, "speechRatePercent"); got != 70 {
		t.Fatalf("dashscope speechRatePercent = %d, want 70", got)
	}
}

func TestDefaultBuilderKeepsProviderRatesWithoutSpeechRatePercent(t *testing.T) {
	volcTenant := Tenant{Kind: "volc-tenant", Volc: &apitypes.VolcTenant{Id: "main", CredentialId: "volc-token"}}
	duplex, err := (DefaultBuilder{}).BuildTransformer(context.Background(), TransformerConfig{
		Model: speechRateVolcModel(t, apitypes.ModelKindRealtimeDuplex), Tenant: volcTenant, Credential: speechRateVolcCredential(),
		Params: map[string]any{"output_speed": 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := transformerNestedIntPointerField(t, duplex, "outputSpeed"); got != 20 {
		t.Fatalf("duplex outputSpeed = %d, want static 20", got)
	}
	volcTTS, err := (DefaultBuilder{}).BuildTransformer(context.Background(), TransformerConfig{
		Voice:  &apitypes.Voice{Id: "volc-voice", ProviderData: mustVolcVoiceProviderData(t, apitypes.VolcTenantVoiceProviderData{VoiceId: new("voice-id")})},
		Tenant: volcTenant, Credential: speechRateVolcCredential(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := transformerFloatField(t, volcTTS, "speedRatio"); got != 1 {
		t.Fatalf("volc TTS speedRatio = %v, want default 1", got)
	}
	dashScope, err := (DefaultBuilder{}).BuildTransformer(context.Background(), speechRateDashScopeConfig(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if got := transformerIntField(t, dashScope, "speechRatePercent"); got != 0 {
		t.Fatalf("dashscope speechRatePercent = %d, want 0", got)
	}
}

func TestDefaultBuilderRejectsInvalidSpeechRatePercent(t *testing.T) {
	for _, value := range []any{49, 201, "fast"} {
		_, err := (DefaultBuilder{}).BuildTransformer(context.Background(), speechRateDashScopeConfig(t, map[string]any{SpeechRatePercentParam: value}))
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("speech rate %v error = %v, want ErrInvalid", value, err)
		}
	}
}

func TestWithSpeechRatePercent(t *testing.T) {
	if got := WithSpeechRatePercent("voice/narrator", nil); got != "voice/narrator" {
		t.Fatalf("nil rate pattern = %q", got)
	}
	got := WithSpeechRatePercent(WithSegmentVoiceFormat("voice/narrator"), new(80))
	base, params, err := splitPatternParams(got)
	if err != nil {
		t.Fatal(err)
	}
	if base != "voice/narrator" || params[SpeechRatePercentParam] != 80 || params["format"] != SegmentVoiceFormat {
		t.Fatalf("pattern %q parsed as %q %#v", got, base, params)
	}
}
