package flowcraft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	flowllm "github.com/GizClaw/flowcraft/sdk/llm"
	flowmodel "github.com/GizClaw/flowcraft/sdk/model"
	"github.com/GizClaw/gizclaw-go/pkgs/buffer"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/google/jsonschema-go/jsonschema"
)

// GenXModel binds one Flowcraft model reference to a resolved GenX generator.
type GenXModel struct {
	Generator genx.Generator
	Pattern   string
}

// GenXResolver adapts resolved GenX generators to Flowcraft's lower-level LLM
// primitive without giving Flowcraft ownership of credentials or resources.
type GenXResolver struct {
	mu     sync.RWMutex
	models map[string]GenXModel
}

// NewGenXResolver constructs an immutable mapping from Flowcraft model names
// to resolved GenX generators.
func NewGenXResolver(models map[string]GenXModel) (*GenXResolver, error) {
	owned := make(map[string]GenXModel, len(models))
	for name, model := range models {
		if name == "" || model.Generator == nil || model.Pattern == "" {
			return nil, fmt.Errorf("agent/flowcraft: model name, generator, and pattern are required")
		}
		owned[name] = model
	}
	return &GenXResolver{models: owned}, nil
}

// Resolve returns the lower-level Flowcraft LLM adapter for a configured model name.
func (r *GenXResolver) Resolve(_ context.Context, name string) (flowllm.LLM, error) {
	r.mu.RLock()
	model, ok := r.models[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("agent/flowcraft: model %q is not resolved", name)
	}
	return &genXLLM{model: model}, nil
}

// InvalidateCache implements Flowcraft's resolver contract. GenX models are
// already resolved and immutable, so there is no local cache to invalidate.
func (r *GenXResolver) InvalidateCache(...flowllm.InvalidateOption) {}

type genXLLM struct{ model GenXModel }

func (l *genXLLM) Generate(ctx context.Context, messages []flowmodel.Message, opts ...flowllm.GenerateOption) (flowmodel.Message, flowmodel.TokenUsage, error) {
	stream, err := l.GenerateStream(ctx, messages, opts...)
	if err != nil {
		return flowmodel.Message{}, flowmodel.TokenUsage{}, err
	}
	defer stream.Close()
	for stream.Next() {
	}
	if err := stream.Err(); err != nil {
		return flowmodel.Message{}, flowmodel.TokenUsage{}, err
	}
	usage := stream.Usage()
	return stream.Message(), flowmodel.TokenUsage{
		InputTokens:       usage.InputTokens,
		CachedInputTokens: usage.CachedInputTokens,
		OutputTokens:      usage.OutputTokens,
		TotalTokens:       usage.InputTokens + usage.OutputTokens,
	}, nil
}

func (l *genXLLM) GenerateStream(ctx context.Context, messages []flowmodel.Message, opts ...flowllm.GenerateOption) (flowllm.StreamMessage, error) {
	modelContext, err := genXModelContext(messages, flowllm.ApplyOptions(opts...))
	if err != nil {
		return nil, err
	}
	stream, err := l.model.Generator.GenerateStream(ctx, l.model.Pattern, modelContext)
	if err != nil {
		return nil, err
	}
	return &genXStreamMessage{stream: stream}, nil
}

func genXModelContext(messages []flowmodel.Message, options *flowllm.GenerateOptions) (genx.ModelContext, error) {
	builder := &genx.ModelContextBuilder{Params: &genx.ModelParams{}}
	if options != nil {
		if options.MaxTokens != nil {
			builder.Params.MaxTokens = int(*options.MaxTokens)
		}
		if options.Temperature != nil {
			builder.Params.Temperature = float32(*options.Temperature)
		}
		if options.TopP != nil {
			builder.Params.TopP = float32(*options.TopP)
		}
		for _, definition := range options.Tools {
			schemaData, err := json.Marshal(definition.InputSchema)
			if err != nil {
				return nil, fmt.Errorf("agent/flowcraft: encode tool schema %q: %w", definition.Name, err)
			}
			var schema jsonschema.Schema
			if err := json.Unmarshal(schemaData, &schema); err != nil {
				return nil, fmt.Errorf("agent/flowcraft: decode tool schema %q: %w", definition.Name, err)
			}
			builder.AddTool(&genx.FuncTool{Name: definition.Name, Description: definition.Description, Argument: &schema})
		}
	}
	for _, message := range messages {
		for _, part := range message.Parts {
			switch part.Type {
			case flowmodel.PartText:
				switch message.Role {
				case flowmodel.RoleSystem:
					builder.PromptText("system", part.Text)
				case flowmodel.RoleUser:
					builder.UserText("", part.Text)
				case flowmodel.RoleAssistant:
					builder.ModelText("", part.Text)
				}
			case flowmodel.PartToolCall:
				if part.ToolCall != nil {
					builder.AddMessage(&genx.Message{Role: genx.RoleModel, Payload: &genx.ToolCall{
						ID:       part.ToolCall.ID,
						FuncCall: &genx.FuncCall{Name: part.ToolCall.Name, Arguments: part.ToolCall.Arguments},
					}})
				}
			case flowmodel.PartToolResult:
				if part.ToolResult != nil {
					builder.AddMessage(&genx.Message{Role: genx.RoleTool, Payload: &genx.ToolResult{
						ID: part.ToolResult.ToolCallID, Result: part.ToolResult.Content,
					}})
				}
			}
		}
	}
	return builder.Build(), nil
}

type genXStreamMessage struct {
	stream  genx.Stream
	current flowmodel.StreamChunk
	content string
	calls   []flowmodel.ToolCall
	err     error
	usage   flowmodel.Usage
}

func (s *genXStreamMessage) Next() bool {
	if s.err != nil || s.stream == nil {
		return false
	}
	for {
		chunk, err := s.stream.Next()
		if err != nil {
			var state *genx.State
			switch {
			case errors.As(err, &state) && state.Status() == genx.StatusDone:
				s.usage = flowmodel.Usage{
					InputTokens:       state.Usage().PromptTokenCount,
					CachedInputTokens: state.Usage().CachedContentTokenCount,
					OutputTokens:      state.Usage().GeneratedTokenCount,
				}
			case !errors.Is(err, io.EOF) && !errors.Is(err, buffer.ErrIteratorDone):
				s.err = err
			}
			return false
		}
		if chunk == nil {
			continue
		}
		if chunk.IsEndOfStream() {
			if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
				s.err = errors.New(chunk.Ctrl.Error)
				return false
			}
			continue
		}
		s.current = flowmodel.StreamChunk{Role: flowmodel.RoleAssistant}
		if text, ok := chunk.Part.(genx.Text); ok {
			s.current.Content = string(text)
			s.content += string(text)
		}
		if chunk.ToolCall != nil && chunk.ToolCall.FuncCall != nil {
			call := flowmodel.ToolCall{ID: chunk.ToolCall.ID, Name: chunk.ToolCall.FuncCall.Name, Arguments: chunk.ToolCall.FuncCall.Arguments}
			s.current.ToolCalls = []flowmodel.ToolCall{call}
			s.calls = append(s.calls, call)
		}
		if s.current.Content == "" && len(s.current.ToolCalls) == 0 {
			continue
		}
		return true
	}
}

func (s *genXStreamMessage) Current() flowmodel.StreamChunk { return s.current }
func (s *genXStreamMessage) Err() error                     { return s.err }
func (s *genXStreamMessage) Close() error {
	if s == nil || s.stream == nil {
		return nil
	}
	return s.stream.Close()
}
func (s *genXStreamMessage) Message() flowmodel.Message {
	parts := make([]flowmodel.Part, 0, 1+len(s.calls))
	if s.content != "" {
		parts = append(parts, flowmodel.Part{Type: flowmodel.PartText, Text: s.content})
	}
	for _, call := range s.calls {
		owned := call
		parts = append(parts, flowmodel.Part{Type: flowmodel.PartToolCall, ToolCall: &owned})
	}
	return flowmodel.Message{Role: flowmodel.RoleAssistant, Parts: parts}
}
func (s *genXStreamMessage) Usage() flowmodel.Usage { return s.usage }

var _ flowllm.LLMResolver = (*GenXResolver)(nil)
var _ flowllm.LLM = (*genXLLM)(nil)
var _ flowllm.StreamMessage = (*genXStreamMessage)(nil)
