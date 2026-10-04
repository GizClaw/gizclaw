//go:build gizclaw_genx_e2e

package transformer

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	einotransformer "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/eino"
	"github.com/cloudwego/eino/components/model"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

func TestEinoTransformerToolContinuation(t *testing.T) {
	loadGenXE2EEnv(t)
	apiKey := firstEnv(einoAPIKeyEnv)
	if apiKey == "" {
		t.Fatalf("set %s in tests/genx-e2e/.env", einoAPIKeyEnv)
	}
	client := openai.NewClient(option.WithAPIKey(apiKey))
	generator := &genx.OpenAIGenerator{
		Client: &client, Model: "gpt-4o-mini", TextOnly: true, SupportToolCalls: true,
	}
	tool, err := genx.NewFuncTool[struct{}](
		"eino_token",
		"Returns the required Eino verification token.",
		genx.InvokeFunc[struct{}](func(context.Context, *genx.FuncCall, struct{}) (any, error) {
			return map[string]string{"token": "EINO_TOOL_OK"}, nil
		}),
	)
	if err != nil {
		t.Fatalf("create Eino tool: %v", err)
	}
	toolkit, err := genx.NewToolkit(tool)
	if err != nil {
		t.Fatalf("create Eino Toolkit: %v", err)
	}
	transformer, err := einotransformer.New(t.Context(), einotransformer.Config{
		Agent: einotransformer.AgentConfig{ID: "tool-continuation", Name: "Tool Continuation"}, Components: &einoE2EResolver{models: map[string]model.BaseChatModel{"chat": &genxChatModel{generator: generator}}}, ToolInvoker: toolkit,
		Graph: einotransformer.GraphDefinition{Name: "tool-continuation", Compile: einotransformer.GraphCompileConfig{NodeTriggerMode: einotransformer.NodeTriggerAnyPredecessor},
			State: einotransformer.StateDefinition{Fields: []einotransformer.StateField{{Name: "messages", Type: einotransformer.StateMessages, Merge: einotransformer.MergeReplace}, {Name: "answer", Type: einotransformer.StateString, Merge: einotransformer.MergeReplace}}},
			Nodes: []einotransformer.NodeDefinition{
				{ID: "prompt", Inputs: map[string]einotransformer.Binding{"text": {From: "input.text"}}, Outputs: map[string]string{"messages": "messages"}, Prompt: &einotransformer.PromptNode{Format: einotransformer.PromptFString, Messages: []einotransformer.PromptMessage{{Role: einotransformer.PromptSystem, Template: "You must call eino_token exactly once. Then reply with one short sentence containing the exact token returned by the tool."}, {Role: einotransformer.PromptUser, Template: "{text}"}}}},
				{ID: "chat", Inputs: map[string]einotransformer.Binding{"messages": {From: "messages"}}, Outputs: map[string]string{"text": "answer"}, ChatModel: &einotransformer.ChatModelNode{Model: "chat"}}},
			Edges: []einotransformer.EdgeDefinition{{From: "start", To: "prompt"}, {From: "prompt", To: "chat"}, {From: "chat", To: "end"}}, Outputs: []einotransformer.OutputDefinition{{Node: "chat", Field: "answer", Name: "assistant", MIMEType: "text/plain", Primary: true}}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = transformer.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	output, err := transformer.Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() failed: %v", err)
	}
	streamID := "eino-e2e-input"
	for _, chunk := range completeTextRoute(
		genx.RoleUser, "", "", streamID, "Confirm that the Eino graph is running.",
	) {
		if err := input.Push(ctx, chunk); err != nil {
			t.Fatalf("push Eino input: %v", err)
		}
	}
	if err := input.Close(); err != nil {
		t.Fatalf("close Eino input: %v", err)
	}
	var response strings.Builder
	var outputStreamID string
	tracker := newRouteLifecycleTracker()
	for {
		chunk, nextErr := output.Next()
		if nextErr != nil {
			if errors.Is(nextErr, io.EOF) {
				break
			}
			t.Fatalf("read Eino output: %v", nextErr)
		}
		observeRouteLifecycle(t, tracker, chunk)
		if chunk.Ctrl != nil {
			if chunk.Ctrl.Error != "" {
				t.Fatalf("Eino output error: %s", chunk.Ctrl.Error)
			}
			if outputStreamID == "" {
				outputStreamID = chunk.Ctrl.StreamID
			}
			if chunk.Ctrl.StreamID != outputStreamID {
				t.Fatalf("Eino output changed StreamID from %q to %q", outputStreamID, chunk.Ctrl.StreamID)
			}
		}
		if text, ok := chunk.Part.(genx.Text); ok && !chunk.IsEndOfStream() {
			response.WriteString(string(text))
		}
	}
	tracker.assertComplete(t)
	if outputStreamID == "" || len(tracker.routes) != 1 {
		t.Fatalf("Eino routes = %#v, want one generated text route", tracker.routes)
	}
	outputRoute := tracker.route(outputStreamID, "text/plain")
	if outputRoute == nil || outputRoute.dataChunks == 0 {
		t.Fatalf("Eino route = %#v, want BOS/data/EOS", outputRoute)
	}
	if !strings.Contains(response.String(), "EINO_TOOL_OK") {
		t.Fatalf("response = %q, want EINO_TOOL_OK", response.String())
	}
	t.Logf("stream_id=%s response=%q", outputStreamID, response.String())
}
