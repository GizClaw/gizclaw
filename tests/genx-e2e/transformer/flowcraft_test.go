//go:build gizclaw_genx_e2e

package transformer

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	flowgraph "github.com/GizClaw/flowcraft/sdk/graph"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	flowcrafttransformer "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/flowcraft"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const flowcraftAPIKeyEnv = "GIZCLAW_GENX_E2E_FLOWCRAFT_OPENAI_API_KEY"

func TestFlowcraftTransformerOpenAICompatibleModel(t *testing.T) {
	loadGenXE2EEnv(t)
	apiKey := firstEnv(flowcraftAPIKeyEnv, "GIZCLAW_E2E_OPENAI_API_KEY", "OPENAI_API_KEY")
	if apiKey == "" {
		t.Skipf("set %s in tests/genx-e2e/.env", flowcraftAPIKeyEnv)
	}
	client := openai.NewClient(option.WithAPIKey(apiKey))
	generator := &genx.OpenAIGenerator{Client: &client, Model: "gpt-4o-mini", TextOnly: true}
	tool, err := genx.NewFuncTool[struct{}](
		"flowcraft_token",
		"Returns the required Flowcraft verification token.",
		genx.InvokeFunc[struct{}](func(context.Context, *genx.FuncCall, struct{}) (any, error) {
			return map[string]string{"token": "FLOWCRAFT_TOOL_OK"}, nil
		}),
	)
	if err != nil {
		t.Fatalf("create Flowcraft tool: %v", err)
	}
	toolkit, err := genx.NewToolkit(tool)
	if err != nil {
		t.Fatalf("create Flowcraft Toolkit: %v", err)
	}
	transformer, err := flowcrafttransformer.New(flowcrafttransformer.Config{
		ID: "flowcraft-e2e", Name: "Flowcraft E2E", Models: generator, Toolkit: toolkit,
		Graph: flowgraph.GraphDefinition{Name: "chat", Entry: "chat", Nodes: []flowgraph.NodeDefinition{{
			ID: "chat", Type: "llm", Config: map[string]any{
				"model":         "chat",
				"system_prompt": "You must call flowcraft_token exactly once. Then reply with one short sentence containing the exact token returned by the tool.",
			},
		}}},
		PublishNodes: []string{"chat"},
	})
	if err != nil {
		t.Fatalf("flowcraft.New() failed: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	output, err := transformer.Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() failed: %v", err)
	}
	streamID := "flowcraft-e2e-input"
	for _, chunk := range []*genx.MessageChunk{
		genx.NewBeginOfStream(streamID),
		{Role: genx.RoleUser, Part: genx.Text("Confirm that the Flowcraft graph is running.")},
		genx.NewTextEndOfStream(),
	} {
		if err := input.Push(ctx, chunk); err != nil {
			t.Fatalf("push Flowcraft input: %v", err)
		}
	}
	if err := input.Close(); err != nil {
		t.Fatalf("close Flowcraft input: %v", err)
	}
	var response strings.Builder
	var outputStreamID string
	seenBOS, seenEOS := false, false
	for {
		chunk, nextErr := output.Next()
		if nextErr != nil {
			if errors.Is(nextErr, io.EOF) {
				break
			}
			t.Fatalf("read Flowcraft output: %v", nextErr)
		}
		if chunk.Ctrl != nil {
			if chunk.Ctrl.Error != "" {
				t.Fatalf("Flowcraft output error: %s", chunk.Ctrl.Error)
			}
			if outputStreamID == "" {
				outputStreamID = chunk.Ctrl.StreamID
			}
			if chunk.Ctrl.StreamID != outputStreamID {
				t.Fatalf("Flowcraft output changed StreamID from %q to %q", outputStreamID, chunk.Ctrl.StreamID)
			}
			seenBOS = seenBOS || chunk.IsBeginOfStream()
			seenEOS = seenEOS || chunk.IsEndOfStream()
		}
		if text, ok := chunk.Part.(genx.Text); ok && !chunk.IsEndOfStream() {
			response.WriteString(string(text))
		}
	}
	if outputStreamID == "" || !seenBOS || !seenEOS {
		t.Fatalf("incomplete lifecycle: stream_id=%q BOS=%v EOS=%v", outputStreamID, seenBOS, seenEOS)
	}
	if !strings.Contains(response.String(), "FLOWCRAFT_TOOL_OK") {
		t.Fatalf("response = %q, want FLOWCRAFT_TOOL_OK", response.String())
	}
	t.Logf("stream_id=%s response=%q", outputStreamID, response.String())
}
