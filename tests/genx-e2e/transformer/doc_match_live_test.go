//go:build gizclaw_genx_e2e

package transformer

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

func TestMatchNodesOpenAICompatibleMusicDirectChat(t *testing.T) {
	loadGenXE2EEnv(t)
	tests := []struct {
		name        string
		apiKeyName  string
		transformer func(*testing.T, *genx.OpenAIGenerator) genx.Transformer
	}{
		{
			name:       "flowcraft",
			apiKeyName: flowcraftAPIKeyEnv,
			transformer: func(t *testing.T, generator *genx.OpenAIGenerator) genx.Transformer {
				return newFlowcraftMatchTransformerWithGenerator(t, generator)
			},
		},
		{
			name:       "eino",
			apiKeyName: einoAPIKeyEnv,
			transformer: func(t *testing.T, generator *genx.OpenAIGenerator) genx.Transformer {
				return newEinoMatchTransformerWithModel(t, &genxChatModel{generator: generator})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			apiKey := firstEnv(test.apiKeyName)
			if apiKey == "" {
				t.Fatalf("set %s in tests/genx-e2e/.env", test.apiKeyName)
			}
			client := openai.NewClient(
				option.WithAPIKey(apiKey),
				option.WithBaseURL("https://api.openai.com/v1"),
			)
			generator := &genx.OpenAIGenerator{
				Client: &client, Model: "gpt-4o-mini", TextOnly: true,
			}
			output := runMatchTransformer(t, test.transformer(t, generator))
			assertMusicDirectMatch(t, output)
			t.Logf("input=%q match=%s", "我想听卡农", output)
		})
	}
}
