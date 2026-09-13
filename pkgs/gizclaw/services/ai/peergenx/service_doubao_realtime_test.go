package peergenx

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaorealtime"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestDefaultBuilderBuildsVolcRealtimeTextOutput(t *testing.T) {
	for _, tc := range []struct {
		output string
		want   bool
	}{
		{output: "", want: false},
		{output: "audio", want: false},
		{output: "text", want: true},
	} {
		transformer, err := (DefaultBuilder{}).BuildTransformer(context.Background(), volcRealtimeTestConfig(t, tc.output))
		if err != nil {
			t.Fatalf("BuildTransformer(output=%q) error = %v", tc.output, err)
		}
		if _, ok := transformer.(*doubaorealtime.Transformer); !ok {
			t.Fatalf("transformer = %T, want *doubaorealtime.Transformer", transformer)
		}
		if got := transformerBoolField(t, transformer, "textOutput"); got != tc.want {
			t.Fatalf("output=%q textOutput = %v, want %v", tc.output, got, tc.want)
		}
	}
}

func TestDefaultBuilderRejectsUnsupportedVolcRealtimeOutput(t *testing.T) {
	_, err := (DefaultBuilder{}).BuildTransformer(context.Background(), volcRealtimeTestConfig(t, "video"))
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("BuildTransformer(output=video) error = %v, want ErrUnsupported", err)
	}
}

func volcRealtimeTestConfig(t *testing.T, output string) TransformerConfig {
	t.Helper()
	body := apitypes.CredentialBody{}
	if err := body.FromVolcCredentialBody(apitypes.VolcCredentialBody{
		SpeechAppId:  new("speech-app-id"),
		SpeechApiKey: new("speech-api-key"),
	}); err != nil {
		t.Fatalf("FromVolcCredentialBody() error = %v", err)
	}
	params := map[string]any{}
	if output != "" {
		params["output"] = output
	}
	return TransformerConfig{
		Model: &apitypes.Model{
			Id:           "realtime-model",
			Kind:         apitypes.ModelKindRealtime,
			ProviderData: mustVolcModelProviderData(t, apitypes.VolcTenantModelProviderData{UpstreamModel: new("1.2.1.1")}),
		},
		Tenant:     Tenant{Kind: string(apitypes.ModelProviderKindVolcTenant), Volc: &apitypes.VolcTenant{}},
		Credential: apitypes.Credential{Id: "volc", Body: body},
		Params:     params,
	}
}
