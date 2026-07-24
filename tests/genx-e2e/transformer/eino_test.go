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
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
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
	generator := &genx.OpenAIGenerator{Client: &client, Model: "gpt-4o-mini", TextOnly: true}
	resolver := &einoE2EResolver{models: map[string]model.BaseChatModel{
		"primary": &genxChatModel{generator: generator, instruction: "Include the exact token EINO_PRIMARY."},
		"peer":    &genxChatModel{generator: generator, instruction: "Include the exact token EINO_PEER."},
	}}
	transformer, err := einotransformer.New(t.Context(), einotransformer.Config{
		Agent:      einotransformer.AgentConfig{ID: "eino-e2e", Name: "Eino E2E"},
		Components: resolver,
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
	if primaryDataChunks == 0 || !strings.Contains(primary.String(), "EINO_PRIMARY") {
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
	_ ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	builder := &genx.ModelContextBuilder{}
	builder.PromptText("eino-e2e-alias", chat.instruction)
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
			builder.ModelText("", message.Content)
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
			if text, ok := chunk.Part.(genx.Text); ok && !chunk.IsEndOfStream() {
				if writer.Send(schema.AssistantMessage(string(text), nil), nil) {
					return
				}
			}
		}
	}()
	return reader, nil
}
