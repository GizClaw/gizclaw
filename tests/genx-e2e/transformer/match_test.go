//go:build gizclaw_genx_e2e

package transformer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	flowgraph "github.com/GizClaw/flowcraft/sdk/graph"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	genxmatch "github.com/GizClaw/gizclaw-go/pkgs/genx/match"
	einotransformer "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/eino"
	flowcrafttransformer "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/flowcraft"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

func TestMatchNodesProviderFreeParity(t *testing.T) {
	flowcraftOutput := runMatchTransformer(t, newFlowcraftMatchTransformer(t))
	einoOutput := runMatchTransformer(t, newEinoMatchTransformer(t))
	var flowcraftMatches, einoMatches any
	if err := json.Unmarshal([]byte(flowcraftOutput), &flowcraftMatches); err != nil {
		t.Fatalf("decode Flowcraft Match output %q: %v", flowcraftOutput, err)
	}
	if err := json.Unmarshal([]byte(einoOutput), &einoMatches); err != nil {
		t.Fatalf("decode Eino Match output %q: %v", einoOutput, err)
	}
	if !reflect.DeepEqual(flowcraftMatches, einoMatches) {
		t.Fatalf("completed Match values differ:\nFlowcraft: %#v\nEino: %#v", flowcraftMatches, einoMatches)
	}
	want := []any{map[string]any{
		"rule": "play_music",
		"args": map[string]any{"title": map[string]any{
			"value": "卡农",
			"var": map[string]any{
				"label": "歌曲名",
				"type":  "string",
			},
			"has_value": true,
		}},
		"raw_text": "",
	}}
	if !reflect.DeepEqual(flowcraftMatches, want) {
		t.Fatalf("completed Match value = %#v, want %#v", flowcraftMatches, want)
	}
}

func TestMatchNodesOpenAICompatibleMusicDirectChat(t *testing.T) {
	loadGenXE2EEnv(t)
	tests := []struct {
		name         string
		apiKeyNames  []string
		baseURLNames []string
		modelNames   []string
		transformer  func(*testing.T, *genx.OpenAIGenerator) genx.Transformer
	}{
		{
			name:         "flowcraft",
			apiKeyNames:  []string{flowcraftAPIKeyEnv, "GIZCLAW_E2E_OPENAI_API_KEY", "OPENAI_API_KEY"},
			baseURLNames: []string{"GIZCLAW_GENX_E2E_FLOWCRAFT_OPENAI_BASE_URL", "GIZCLAW_GENX_E2E_OPENAI_BASE_URL", "OPENAI_BASE_URL"},
			modelNames:   []string{"GIZCLAW_GENX_E2E_FLOWCRAFT_OPENAI_MODEL", "GIZCLAW_GENX_E2E_OPENAI_MODEL", "OPENAI_MODEL"},
			transformer: func(t *testing.T, generator *genx.OpenAIGenerator) genx.Transformer {
				return newFlowcraftMatchTransformerWithGenerator(t, generator)
			},
		},
		{
			name:         "eino",
			apiKeyNames:  []string{einoAPIKeyEnv, "GIZCLAW_E2E_OPENAI_API_KEY", "OPENAI_API_KEY"},
			baseURLNames: []string{"GIZCLAW_GENX_E2E_EINO_OPENAI_BASE_URL", "GIZCLAW_GENX_E2E_OPENAI_BASE_URL", "OPENAI_BASE_URL"},
			modelNames:   []string{"GIZCLAW_GENX_E2E_EINO_OPENAI_MODEL", "GIZCLAW_GENX_E2E_OPENAI_MODEL", "OPENAI_MODEL"},
			transformer: func(t *testing.T, generator *genx.OpenAIGenerator) genx.Transformer {
				return newEinoMatchTransformerWithModel(t, &genxChatModel{generator: generator})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			apiKey := firstEnv(test.apiKeyNames...)
			baseURL := firstEnv(test.baseURLNames...)
			modelName := firstEnv(test.modelNames...)
			if apiKey == "" || baseURL == "" || modelName == "" {
				t.Skipf(
					"set %s, %s, and %s in tests/genx-e2e/.env",
					test.apiKeyNames[0],
					test.baseURLNames[0],
					test.modelNames[0],
				)
			}
			client := openai.NewClient(
				option.WithAPIKey(apiKey),
				option.WithBaseURL(strings.TrimRight(baseURL, "/")),
			)
			generator := &genx.OpenAIGenerator{
				Client: &client, Model: modelName, TextOnly: true,
			}
			output := runMatchTransformer(t, test.transformer(t, generator))
			assertMusicDirectMatch(t, output)
			t.Logf("input=%q match=%s", "我想听卡农", output)
		})
	}
}

func newFlowcraftMatchTransformer(t *testing.T) genx.Transformer {
	t.Helper()
	return newFlowcraftMatchTransformerWithGenerator(t, &providerFreeMatchGenerator{})
}

func newFlowcraftMatchTransformerWithGenerator(
	t *testing.T,
	generator genx.Generator,
) genx.Transformer {
	t.Helper()
	transformer, err := flowcrafttransformer.New(flowcrafttransformer.Config{
		ID: "flowcraft-match",
		Graph: flowgraph.GraphDefinition{
			Name:  "match",
			Entry: "match",
			Nodes: []flowgraph.NodeDefinition{
				{
					ID: "match", Type: "match",
					Config: map[string]any{
						"model": "router", "input": "input", "output": "matches",
						"rules": matchRules(),
					},
				},
				{
					ID: "emit", Type: "script",
					Config: map[string]any{"source": `
host.emit("token", {content: JSON.stringify(board.getVar("matches"))});
`},
				},
			},
			Edges: []flowgraph.EdgeDefinition{{From: "match", To: "emit"}},
		},
		PublishNodes: []string{"emit"},
		Models:       generator,
	})
	if err != nil {
		t.Fatalf("flowcraft.New() error = %v", err)
	}
	return transformer
}

func newEinoMatchTransformer(t *testing.T) genx.Transformer {
	t.Helper()
	return newEinoMatchTransformerWithModel(t, &providerFreeMatchChatModel{})
}

func newEinoMatchTransformerWithModel(
	t *testing.T,
	chatModel model.BaseChatModel,
) genx.Transformer {
	t.Helper()
	transformer, err := einotransformer.New(t.Context(), einotransformer.Config{
		Agent:      einotransformer.AgentConfig{ID: "eino-match"},
		Components: &matchE2EResolver{chatModel: chatModel},
		Limits:     einotransformer.Limits{MaxOutputBytes: 1 << 20},
		Graph: einotransformer.GraphDefinition{
			Name: "match",
			State: einotransformer.StateDefinition{Fields: []einotransformer.StateField{
				{Name: "matches", Type: einotransformer.StateList, Merge: einotransformer.MergeReplace},
				{Name: "answer", Type: einotransformer.StateString, Merge: einotransformer.MergeReplace},
			}},
			Nodes: []einotransformer.NodeDefinition{
				{
					ID: "match",
					Inputs: map[string]einotransformer.Binding{
						"text": {From: "input.text"},
					},
					Outputs: map[string]string{"matches": "matches"},
					Match: &einotransformer.MatchNode{
						Model: "router", Rules: matchRules(),
					},
				},
				{
					ID: "emit",
					Inputs: map[string]einotransformer.Binding{
						"matches": {From: "matches"},
					},
					Outputs: map[string]string{"answer": "answer"},
					Script: &einotransformer.ScriptNode{
						Language: einotransformer.ScriptStarlark,
						Source: `
def run(input):
    item = input["matches"][0]
    title = item["args"]["title"]
    value = (
        "[{\"rule\":\"" + item["rule"] +
        "\",\"args\":{\"title\":{\"value\":\"" + title["value"] +
        "\",\"var\":{\"label\":\"" + title["var"]["label"] +
        "\",\"type\":\"" + title["var"]["type"] +
        "\"},\"has_value\":true}},\"raw_text\":\"" + item["raw_text"] + "\"}]"
    )
    return {"answer": value}
`,
						Limits: einotransformer.ScriptLimits{
							MaxExecutionSteps: 1_000,
							Timeout:           time.Second,
							MaxInputBytes:     1 << 10,
							MaxOutputBytes:    1 << 10,
						},
					},
				},
			},
			Edges: []einotransformer.EdgeDefinition{
				{From: "start", To: "match"},
				{From: "match", To: "emit"},
				{From: "emit", To: "end"},
			},
			Outputs: []einotransformer.OutputDefinition{{
				Node: "emit", Field: "answer", Name: "answer",
				MIMEType: "text/plain", Primary: true,
			}},
		},
	})
	if err != nil {
		t.Fatalf("eino.New() error = %v", err)
	}
	return transformer
}

func assertMusicDirectMatch(t *testing.T, output string) {
	t.Helper()
	var matches []struct {
		Rule string `json:"rule"`
		Args map[string]struct {
			Value    any  `json:"value"`
			HasValue bool `json:"has_value"`
		} `json:"args"`
		RawText string `json:"raw_text"`
	}
	if err := json.Unmarshal([]byte(output), &matches); err != nil {
		t.Fatalf("decode Match chat output %q: %v", output, err)
	}
	if len(matches) != 1 {
		t.Fatalf("Match chat output = %#v, want one result", matches)
	}
	title, ok := matches[0].Args["title"]
	if matches[0].Rule != "play_music" || !ok || !title.HasValue ||
		title.Value != "卡农" || matches[0].RawText != "" {
		t.Fatalf("Match chat output = %#v, want play_music title 卡农", matches)
	}
}

func matchRules() []*genxmatch.Rule {
	return []*genxmatch.Rule{{
		Name: "play_music",
		Vars: map[string]genxmatch.Var{
			"title": {Label: "歌曲名", Type: "string"},
		},
		Patterns: []genxmatch.Pattern{{Input: "我想听[title]"}},
	}}
}

func runMatchTransformer(t *testing.T, transformer genx.Transformer) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
	if err := input.Add(
		genx.NewBeginOfStream("match-input"),
		&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("我想听卡农")},
		genx.NewTextEndOfStream(),
	); err != nil {
		t.Fatalf("build input: %v", err)
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatalf("finish input: %v", err)
	}
	output, err := transformer.Transform(ctx, input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer output.Close()
	var text strings.Builder
	for {
		chunk, nextErr := output.Next()
		if errors.Is(nextErr, io.EOF) {
			return text.String()
		}
		if nextErr != nil {
			t.Fatalf("read output: %v", nextErr)
		}
		if chunk != nil && chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
			t.Fatalf("output error: %s", chunk.Ctrl.Error)
		}
		if value, ok := chunk.Part.(genx.Text); ok && !chunk.IsEndOfStream() {
			text.WriteString(string(value))
		}
	}
}

type providerFreeMatchGenerator struct{}

func (*providerFreeMatchGenerator) GenerateStream(
	_ context.Context,
	_ string,
	_ genx.ModelContext,
) (genx.Stream, error) {
	stream := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 2)
	if err := stream.Add(&genx.MessageChunk{
		Role: genx.RoleModel, Part: genx.Text("play_music: title=卡农\n"),
	}); err != nil {
		return nil, err
	}
	if err := stream.Done(genx.Usage{}); err != nil {
		return nil, err
	}
	return stream.Stream(), nil
}

func (*providerFreeMatchGenerator) Invoke(
	context.Context,
	string,
	genx.ModelContext,
	*genx.FuncTool,
) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("Invoke must not be used")
}

type matchE2EResolver struct {
	chatModel model.BaseChatModel
}

func (resolver *matchE2EResolver) ResolveChatModel(
	context.Context,
	string,
) (model.BaseChatModel, error) {
	return resolver.chatModel, nil
}

func (*matchE2EResolver) ResolveRetriever(
	context.Context,
	string,
) (retriever.Retriever, error) {
	return nil, errors.New("retriever is not configured")
}

type providerFreeMatchChatModel struct{}

func (*providerFreeMatchChatModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.Message, error) {
	reader, err := (&providerFreeMatchChatModel{}).Stream(ctx, input, options...)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return reader.Recv()
}

func (*providerFreeMatchChatModel) Stream(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{
		schema.AssistantMessage("play_music: title=卡农\n", nil),
	}), nil
}
