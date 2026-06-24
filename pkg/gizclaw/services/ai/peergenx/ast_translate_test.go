package peergenx

import (
	"context"
	"testing"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkg/genx/transformers"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func TestDefaultBuilderBuildsVolcASTTranslateTransformer(t *testing.T) {
	body := apitypes.CredentialBody{}
	if err := body.FromVolcCredentialBody(apitypes.VolcCredentialBody{
		AppId:       testStringPtr("app-id"),
		SpeechToken: testStringPtr("speech-token"),
	}); err != nil {
		t.Fatalf("FromVolcCredentialBody() error = %v", err)
	}
	transformer, err := (DefaultBuilder{}).BuildTransformer(context.Background(), TransformerConfig{
		Model: &apitypes.Model{
			Id:   "ast-model",
			Kind: apitypes.ModelKindTranslation,
		},
		Tenant: Tenant{Kind: string(apitypes.ModelProviderKindVolcTenant), Volc: &apitypes.VolcTenant{}},
		Credential: apitypes.Credential{
			Name: "volc",
			Body: body,
		},
		Params: map[string]any{
			"mode":      string(doubaospeech.ASTTranslateModeS2T),
			"lang_pair": "auto",
		},
	})
	if err != nil {
		t.Fatalf("BuildTransformer() error = %v", err)
	}
	if _, ok := transformer.(*transformers.DoubaoASTTranslate); !ok {
		t.Fatalf("transformer = %T, want *DoubaoASTTranslate", transformer)
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
