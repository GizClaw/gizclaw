package flowcraft

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GizClaw/flowcraft/sdk/graph"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// promptEchoGenerator replies with every system prompt the model received.
type promptEchoGenerator struct{}

func (*promptEchoGenerator) GenerateStream(_ context.Context, _ string, modelContext genx.ModelContext) (genx.Stream, error) {
	var prompts []string
	for prompt := range modelContext.Prompts() {
		prompts = append(prompts, prompt.Text)
	}
	return responseStream(modelContext, strings.Join(prompts, "|")), nil
}

func (*promptEchoGenerator) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("not supported")
}

// A Board input referenced as ${board.name} in an LLM system_prompt is
// resolved on every turn; this is how a Workflow places host values such as
// the Workspace safety fence.
func TestBoardInputResolvesInSystemPrompt(t *testing.T) {
	t.Parallel()
	for _, fence := range []string{"Stay child safe.", ""} {
		transformer, err := New(Config{
			ID: "fence", Name: "Fence", Models: &promptEchoGenerator{},
			BoardInputs: func(context.Context) (map[string]any, error) {
				return map[string]any{"safety_fence": fence}, nil
			},
			Graph: graph.GraphDefinition{
				Name: "fence", Entry: "answer",
				Nodes: []graph.NodeDefinition{{
					ID: "answer", Type: "llm",
					Config: map[string]any{"model": "chat", "system_prompt": "[${board.safety_fence}] You are Momo."},
				}},
				Edges: []graph.EdgeDefinition{{From: "answer", To: graph.END}},
			},
			PublishNodes: []string{"answer"},
		})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		output, err := transformer.Transform(t.Context(), textInput("hi"))
		if err != nil {
			t.Fatalf("Transform() error = %v", err)
		}
		if got, want := joinedText(drain(t, output)), "["+fence+"] You are Momo."; got != want {
			t.Fatalf("system prompt = %q, want %q", got, want)
		}
	}
}
