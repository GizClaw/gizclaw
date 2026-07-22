package peergenx

import (
	"context"
	"testing"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaoast"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestDefaultBuilderBuildsVolcASTTranslateTransformer(t *testing.T) {
	body := apitypes.CredentialBody{}
	if err := body.FromVolcCredentialBody(apitypes.VolcCredentialBody{
		SpeechAppId:  testStringPtr("speech-app-id"),
		SpeechApiKey: testStringPtr("speech-api-key"),
	}); err != nil {
		t.Fatalf("FromVolcCredentialBody() error = %v", err)
	}
	transformer, err := (DefaultBuilder{}).BuildTransformer(context.Background(), TransformerConfig{
		Model: &apitypes.Model{
			Id:           "ast-model",
			Kind:         apitypes.ModelKindTranslation,
			ProviderData: mustVolcModelProviderData(t, apitypes.VolcTenantModelProviderData{}),
		},
		Tenant: Tenant{Kind: string(apitypes.ModelProviderKindVolcTenant), Volc: &apitypes.VolcTenant{}},
		Credential: apitypes.Credential{
			Name: "volc",
			Body: body,
		},
		Params: map[string]any{
			"mode":            string(doubaospeech.ASTTranslateModeS2T),
			"input":           "push-to-talk",
			"lang_pair":       "auto",
			"realtime_pacing": false,
		},
	})
	if err != nil {
		t.Fatalf("BuildTransformer() error = %v", err)
	}
	if _, ok := transformer.(*doubaoast.Transformer); !ok {
		t.Fatalf("transformer = %T, want *doubaoast.Transformer", transformer)
	}
	if got := transformerStringField(t, transformer, "inputMode"); got != "push-to-talk" {
		t.Fatalf("AST translate inputMode = %q, want push-to-talk", got)
	}
	if transformerBoolField(t, transformer, "realtimePacing") {
		t.Fatal("AST translate realtimePacing = true, want false")
	}
}

func TestVolcASTTranslateLanguagesFromPairRejectsZhenForms(t *testing.T) {
	for _, pair := range []string{"zhen", "zhen/zhen", "zh/zhen", "zhen/en"} {
		if _, _, _, err := volcASTTranslateLanguagesFromPair(pair); err == nil {
			t.Fatalf("volcASTTranslateLanguagesFromPair(%q) succeeded, want error", pair)
		}
	}
	source, target, auto, err := volcASTTranslateLanguagesFromPair("auto")
	if err != nil {
		t.Fatalf("volcASTTranslateLanguagesFromPair(auto) error = %v", err)
	}
	if source != "zhen" || target != "zhen" || !auto {
		t.Fatalf("volcASTTranslateLanguagesFromPair(auto) = %q, %q, %v", source, target, auto)
	}
	source, target, auto, err = volcASTTranslateLanguagesFromPair("en/zh")
	if err != nil {
		t.Fatalf("volcASTTranslateLanguagesFromPair(en/zh) error = %v", err)
	}
	if source != "en" || target != "zh" || auto {
		t.Fatalf("volcASTTranslateLanguagesFromPair(en/zh) = %q, %q, %v", source, target, auto)
	}
	source, target, auto, err = volcASTTranslateLanguagesFromPair("zh/jp")
	if err != nil {
		t.Fatalf("volcASTTranslateLanguagesFromPair(zh/jp) error = %v", err)
	}
	if source != "zh" || target != "ja" || auto {
		t.Fatalf("volcASTTranslateLanguagesFromPair(zh/jp) = %q, %q, %v", source, target, auto)
	}
}
