//go:build gizclaw_genx_e2e

package transformer

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	einotransformer "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/eino"
	"github.com/cloudwego/eino/components/model"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const einoChatVerificationToken = "EINO_CHAT_HISTORY_OK_7F3A"

func TestEinoWorkflowOpenAICompatibleConversation(t *testing.T) {
	loadGenXE2EEnv(t)
	apiKey := firstEnv(einoAPIKeyEnv, "GIZCLAW_E2E_OPENAI_API_KEY", "OPENAI_API_KEY")
	if apiKey == "" {
		t.Skipf("set %s in tests/genx-e2e/.env", einoAPIKeyEnv)
	}
	client := openai.NewClient(option.WithAPIKey(apiKey))
	generator := &genx.OpenAIGenerator{
		Client: &client, Model: "gpt-4o-mini", TextOnly: true, SupportToolCalls: true,
	}
	var toolCalls atomic.Int32
	tool, err := genx.NewFuncTool[struct{}](
		"eino_chat_token",
		"Returns the private verification token for the current conversation.",
		genx.InvokeFunc[struct{}](func(context.Context, *genx.FuncCall, struct{}) (any, error) {
			toolCalls.Add(1)
			return map[string]string{"token": einoChatVerificationToken}, nil
		}),
	)
	if err != nil {
		t.Fatalf("create Eino chat tool: %v", err)
	}
	toolkit, err := genx.NewToolkit(tool)
	if err != nil {
		t.Fatalf("create Eino chat Toolkit: %v", err)
	}
	resolver := &einoE2EResolver{models: map[string]model.BaseChatModel{
		"chat": &genxChatModel{
			generator: generator,
			instruction: "You are verifying a two-turn Eino conversation. " +
				"When the user asks you to fetch the private token, call eino_chat_token exactly once and include its exact token in your reply. " +
				"When the user later asks you to repeat the token from the prior reply, use conversation history and do not call any tool. " +
				"Keep every reply to one short sentence.",
		},
	}}
	transformer, err := einotransformer.New(t.Context(), einotransformer.Config{
		Agent: einotransformer.AgentConfig{
			ID: "eino-chat-e2e", Name: "Eino Chat E2E", ContextID: "eino-chat-e2e",
		},
		Components: resolver,
		Toolkit:    toolkit,
		Limits:     einotransformer.Limits{MaxOutputBytes: 1 << 20},
		Graph: einotransformer.GraphDefinition{
			Name: "chat-workflow",
			State: einotransformer.StateDefinition{Fields: []einotransformer.StateField{
				{Name: "answer", Type: einotransformer.StateString, Merge: einotransformer.MergeReplace},
			}},
			Nodes: []einotransformer.NodeDefinition{{
				ID: "chat",
				Inputs: map[string]einotransformer.Binding{
					"messages": {From: "input.messages"},
				},
				Outputs:   map[string]string{"text": "answer"},
				ChatModel: &einotransformer.ChatModelNode{Model: "chat"},
			}},
			Edges: []einotransformer.EdgeDefinition{
				{From: "start", To: "chat"},
				{From: "chat", To: "end"},
			},
			Outputs: []einotransformer.OutputDefinition{{
				Node: "chat", Field: "answer", Name: "assistant",
				MIMEType: "text/plain", Primary: true,
			}},
		},
	})
	if err != nil {
		t.Fatalf("eino.New() chat workflow failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	output, err := transformer.Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() chat workflow failed: %v", err)
	}
	defer output.Close()

	pushEinoChatTurn(t, ctx, input, "eino-chat-turn-1",
		"Fetch the private token with the tool and tell me the exact token.")
	first := readEinoChatTurn(t, output)
	if !strings.Contains(first.text, einoChatVerificationToken) {
		t.Fatalf("first response = %q, want token %q", first.text, einoChatVerificationToken)
	}
	if got := toolCalls.Load(); got != 1 {
		t.Fatalf("tool calls after first turn = %d, want 1", got)
	}

	pushEinoChatTurn(t, ctx, input, "eino-chat-turn-2",
		"Without calling any tool, repeat the exact private token from your previous reply.")
	second := readEinoChatTurn(t, output)
	if !strings.Contains(second.text, einoChatVerificationToken) {
		t.Fatalf("second response = %q, want history token %q", second.text, einoChatVerificationToken)
	}
	if got := toolCalls.Load(); got != 1 {
		t.Fatalf("tool calls after history turn = %d, want 1", got)
	}
	if first.streamID == second.streamID {
		t.Fatalf("Eino chat reused output StreamID %q across turns", first.streamID)
	}

	if err := input.Close(); err != nil {
		t.Fatalf("close Eino chat input: %v", err)
	}
	if _, err := output.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("read Eino chat completion: %v, want EOF", err)
	}
	t.Logf(
		"first_stream=%s first_response=%q second_stream=%s second_response=%q tool_calls=%d",
		first.streamID, first.text, second.streamID, second.text, toolCalls.Load(),
	)
}

func pushEinoChatTurn(
	t *testing.T,
	ctx context.Context,
	input *genx.RealtimeStream,
	streamID string,
	text string,
) {
	t.Helper()
	for _, chunk := range []*genx.MessageChunk{
		genx.NewBeginOfStream(streamID),
		{Role: genx.RoleUser, Part: genx.Text(text)},
		genx.NewTextEndOfStream(),
	} {
		if err := input.Push(ctx, chunk); err != nil {
			t.Fatalf("push Eino chat turn %q: %v", streamID, err)
		}
	}
}

type einoChatTurn struct {
	text     string
	streamID string
}

func readEinoChatTurn(t *testing.T, output genx.Stream) einoChatTurn {
	t.Helper()
	var response strings.Builder
	var streamID string
	seenBOS := false
	for {
		chunk, err := output.Next()
		if err != nil {
			t.Fatalf("read Eino chat turn: %v", err)
		}
		if chunk == nil {
			t.Fatal("read Eino chat turn: nil chunk")
		}
		if chunk.ToolCall != nil {
			t.Fatalf("Eino chat leaked internal ToolCall: %#v", chunk.ToolCall)
		}
		if chunk.Ctrl != nil {
			if chunk.Ctrl.Error != "" {
				t.Fatalf("Eino chat output error: %s", chunk.Ctrl.Error)
			}
			if chunk.Name != "assistant" {
				t.Fatalf("Eino chat output route = %q, want assistant", chunk.Name)
			}
			if streamID == "" {
				streamID = chunk.Ctrl.StreamID
			}
			if chunk.Ctrl.StreamID != streamID {
				t.Fatalf("Eino chat output changed StreamID from %q to %q", streamID, chunk.Ctrl.StreamID)
			}
			seenBOS = seenBOS || chunk.IsBeginOfStream()
			if chunk.IsEndOfStream() {
				if !seenBOS || streamID == "" {
					t.Fatalf("incomplete Eino chat lifecycle: stream_id=%q BOS=%v", streamID, seenBOS)
				}
				return einoChatTurn{text: response.String(), streamID: streamID}
			}
		}
		if text, ok := chunk.Part.(genx.Text); ok {
			response.WriteString(string(text))
		}
	}
}
