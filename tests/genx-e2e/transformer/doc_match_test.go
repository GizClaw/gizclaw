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
	if err := input.Add(completeTextRoute(genx.RoleUser, "", "", "match-input", "我想听卡农")...); err != nil {
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
	tracker := newRouteLifecycleTracker()
	for {
		chunk, nextErr := output.Next()
		if errors.Is(nextErr, io.EOF) {
			tracker.assertComplete(t)
			return text.String()
		}
		if nextErr != nil {
			t.Fatalf("read output: %v", nextErr)
		}
		if chunk != nil && chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
			t.Fatalf("output error: %s", chunk.Ctrl.Error)
		}
		observeRouteLifecycle(t, tracker, chunk)
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
