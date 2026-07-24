//go:build gizclaw_genx_e2e

package transformer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	einotransformer "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/eino"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const einoAPIKeyEnv = "GIZCLAW_GENX_E2E_EINO_OPENAI_API_KEY"

func TestEinoTransformerOpenAICompatibleGraph(t *testing.T) {
	loadGenXE2EEnv(t)
	apiKey := firstEnv(einoAPIKeyEnv, "GIZCLAW_E2E_OPENAI_API_KEY", "OPENAI_API_KEY")
	if apiKey == "" {
		t.Skipf("set %s in tests/genx-e2e/.env", einoAPIKeyEnv)
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
	resolver := &einoE2EResolver{models: map[string]model.BaseChatModel{
		"primary": &genxChatModel{
			generator:   generator,
			instruction: "You must call eino_token exactly once. Then include its exact returned token in one short sentence.",
		},
		"peer": &genxChatModel{generator: generator, instruction: "Include the exact token EINO_PEER."},
	}}
	transformer, err := einotransformer.New(t.Context(), einotransformer.Config{
		Agent:      einotransformer.AgentConfig{ID: "eino-e2e", Name: "Eino E2E"},
		Components: resolver,
		Toolkit:    toolkit,
		Limits:     einotransformer.Limits{MaxOutputBytes: 1 << 20},
		Graph: einotransformer.GraphDefinition{
			Name: "provider-graph",
			Compile: einotransformer.GraphCompileConfig{
				NodeTriggerMode: einotransformer.NodeTriggerAllPredecessor,
				FanIn: map[string]einotransformer.FanInConfig{
					"join": {StreamMergeWithSourceEOF: true},
				},
			},
			State: einotransformer.StateDefinition{Fields: []einotransformer.StateField{
				{Name: "intent", Type: einotransformer.StateString, Merge: einotransformer.MergeReplace},
				{Name: "messages", Type: einotransformer.StateMessages, Merge: einotransformer.MergeReplace},
				{Name: "primary", Type: einotransformer.StateString, Merge: einotransformer.MergeReplace},
				{Name: "peer", Type: einotransformer.StateString, Merge: einotransformer.MergeReplace},
				{Name: "joined", Type: einotransformer.StateString, Merge: einotransformer.MergeReplace},
			}},
			Nodes: []einotransformer.NodeDefinition{
				{
					ID: "classify", Inputs: map[string]einotransformer.Binding{
						"value": {From: "input.text"},
					},
					Outputs: map[string]string{"value": "intent"},
					Transform: &einotransformer.TransformNode{
						Operation: einotransformer.TransformSelect,
					},
				},
				{
					ID: "prompt", Inputs: map[string]einotransformer.Binding{
						"text": {From: "input.text"},
					},
					Outputs: map[string]string{"messages": "messages"},
					Prompt: &einotransformer.PromptNode{
						Format: einotransformer.PromptFString,
						Messages: []einotransformer.PromptMessage{
							{Role: einotransformer.PromptSystem, Template: "Reply with one short sentence."},
							{Role: einotransformer.PromptUser, Template: "{text}"},
						},
					},
				},
				{
					ID: "primary", Inputs: map[string]einotransformer.Binding{
						"messages": {From: "messages"},
					},
					Outputs:   map[string]string{"text": "primary"},
					ChatModel: &einotransformer.ChatModelNode{Model: "primary"},
				},
				{
					ID: "peer", Inputs: map[string]einotransformer.Binding{
						"messages": {From: "messages"},
					},
					Outputs:   map[string]string{"text": "peer"},
					ChatModel: &einotransformer.ChatModelNode{Model: "peer"},
				},
				{
					ID: "join", Inputs: map[string]einotransformer.Binding{
						"primary": {From: "primary"}, "peer": {From: "peer"},
					},
					Outputs: map[string]string{"text": "joined"},
					Transform: &einotransformer.TransformNode{
						Operation: einotransformer.TransformConcatText,
						Order:     []string{"primary", "peer"}, Separator: "\n",
					},
				},
			},
			Edges: []einotransformer.EdgeDefinition{
				{From: "start", To: "classify"}, {From: "classify", To: "prompt"},
				{From: "primary", To: "join"}, {From: "peer", To: "join"}, {From: "join", To: "end"},
			},
			Branches: []einotransformer.BranchDefinition{{
				From: "prompt", Mode: einotransformer.BranchAllMatch,
				Routes: []einotransformer.BranchRoute{
					{
						When: einotransformer.Predicate{
							Field: "intent", Op: einotransformer.PredicateExists,
						},
						To: "primary",
					},
					{
						When: einotransformer.Predicate{
							Field: "intent", Op: einotransformer.PredicateExists,
						},
						To: "peer",
					},
				},
				Default: "primary",
			}},
			Outputs: []einotransformer.OutputDefinition{
				{
					Node: "primary", Field: "primary", Name: "assistant",
					MIMEType: "text/plain", Primary: true,
				},
				{
					Node: "join", Field: "joined", Name: "joined",
					MIMEType: "text/plain",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("eino.New() failed: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	output, err := transformer.Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() failed: %v", err)
	}
	streamID := "eino-e2e-input"
	for _, chunk := range []*genx.MessageChunk{
		genx.NewBeginOfStream(streamID),
		{Role: genx.RoleUser, Part: genx.Text("Confirm that the Eino graph is running.")},
		genx.NewTextEndOfStream(),
	} {
		if err := input.Push(ctx, chunk); err != nil {
			t.Fatalf("push Eino input: %v", err)
		}
	}
	if err := input.Close(); err != nil {
		t.Fatalf("close Eino input: %v", err)
	}
	var primary strings.Builder
	var primaryDataChunks int
	for {
		chunk, nextErr := output.Next()
		if nextErr != nil {
			if errors.Is(nextErr, io.EOF) {
				break
			}
			t.Fatalf("read Eino output: %v", nextErr)
		}
		if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
			t.Fatalf("Eino output error: %s", chunk.Ctrl.Error)
		}
		if text, ok := chunk.Part.(genx.Text); ok && chunk.Name == "assistant" && !chunk.IsEndOfStream() {
			primary.WriteString(string(text))
			primaryDataChunks++
		}
	}
	if primaryDataChunks == 0 || !strings.Contains(primary.String(), "EINO_TOOL_OK") {
		t.Fatalf("streamed primary response = %q in %d chunks", primary.String(), primaryDataChunks)
	}
	t.Logf("streamed_primary_chunks=%d response=%q", primaryDataChunks, primary.String())
}

type einoE2EResolver struct {
	models map[string]model.BaseChatModel
}

func (resolver *einoE2EResolver) ResolveChatModel(
	_ context.Context,
	name string,
) (model.BaseChatModel, error) {
	component := resolver.models[name]
	if component == nil {
		return nil, errors.New("unknown model alias")
	}
	return component, nil
}

func (*einoE2EResolver) ResolveRetriever(
	context.Context,
	string,
) (retriever.Retriever, error) {
	return nil, errors.New("retriever is not configured")
}

type genxChatModel struct {
	generator   *genx.OpenAIGenerator
	instruction string
}

func (chat *genxChatModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.Message, error) {
	reader, err := chat.Stream(ctx, input, options...)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	var chunks []*schema.Message
	for {
		chunk, recvErr := reader.Recv()
		if errors.Is(recvErr, io.EOF) {
			return schema.ConcatMessages(chunks)
		}
		if recvErr != nil {
			return nil, recvErr
		}
		chunks = append(chunks, chunk)
	}
}

func (chat *genxChatModel) Stream(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	builder := &genx.ModelContextBuilder{}
	builder.PromptText("eino-e2e-alias", chat.instruction)
	modelOptions := model.GetCommonOptions(nil, options...)
	for _, tool := range modelOptions.Tools {
		if tool == nil {
			continue
		}
		params, err := tool.ParamsOneOf.ToJSONSchema()
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		var converted jsonschema.Schema
		if err := json.Unmarshal(encoded, &converted); err != nil {
			return nil, err
		}
		builder.AddTool(&genx.FuncTool{
			Name: tool.Name, Description: tool.Desc, Argument: &converted,
		})
	}
	for _, message := range input {
		if message == nil {
			continue
		}
		switch message.Role {
		case schema.System:
			builder.PromptText("eino-e2e-prompt", message.Content)
		case schema.User:
			builder.UserText("", message.Content)
		case schema.Assistant:
			if message.Content != "" {
				builder.ModelText("", message.Content)
			}
			for _, call := range message.ToolCalls {
				builder.AddMessage(&genx.Message{
					Role: genx.RoleModel,
					Payload: &genx.ToolCall{
						ID: call.ID,
						FuncCall: &genx.FuncCall{
							Name: call.Function.Name, Arguments: call.Function.Arguments,
						},
					},
				})
			}
		case schema.Tool:
			builder.AddMessage(&genx.Message{
				Role: genx.RoleTool,
				Payload: &genx.ToolResult{
					ID: message.ToolCallID, Result: message.Content,
				},
			})
		default:
			return nil, errors.New("unsupported Eino message role")
		}
	}
	stream, err := chat.generator.GenerateStream(ctx, "", builder.Build())
	if err != nil {
		return nil, err
	}
	reader, writer := schema.Pipe[*schema.Message](8)
	go func() {
		defer writer.Close()
		defer stream.Close()
		for {
			chunk, nextErr := stream.Next()
			if nextErr != nil {
				if !errors.Is(nextErr, io.EOF) {
					writer.Send(nil, nextErr)
				}
				return
			}
			if chunk != nil && chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
				writer.Send(nil, errors.New(chunk.Ctrl.Error))
				return
			}
			message := &schema.Message{Role: schema.Assistant}
			if text, ok := chunk.Part.(genx.Text); ok && !chunk.IsEndOfStream() {
				message.Content = string(text)
			}
			if chunk.ToolCall != nil && chunk.ToolCall.FuncCall != nil {
				message.ToolCalls = []schema.ToolCall{{
					ID: chunk.ToolCall.ID, Type: "function",
					Function: schema.FunctionCall{
						Name: chunk.ToolCall.FuncCall.Name, Arguments: chunk.ToolCall.FuncCall.Arguments,
					},
				}}
			}
			if message.Content == "" && len(message.ToolCalls) == 0 {
				continue
			}
			if writer.Send(message, nil) {
				return
			}
		}
	}()
	return reader, nil
}
