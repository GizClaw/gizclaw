package flowcraft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"

	flowllm "github.com/GizClaw/flowcraft/sdk/llm"
	flowmodel "github.com/GizClaw/flowcraft/sdk/model"
	"github.com/GizClaw/gizclaw-go/pkgs/buffer"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/google/jsonschema-go/jsonschema"
)

// providerSafeEmptyUserText matches the compatibility behavior previously
// supplied by Claw. It keeps an initiative turn logically empty in Flowcraft
// history and memory while satisfying providers that reject empty user input.
const providerSafeEmptyUserText = "\u200b"

type modelResolver struct{ generator genx.Generator }

// ResolveLLM adapts one RuntimeProfile model alias to Flowcraft's LLM
// interface. It uses the same GenX generator path as Graph LLM nodes.
func ResolveLLM(generator genx.Generator, alias string) (flowllm.LLM, error) {
	if generator == nil {
		return nil, fmt.Errorf("flowcraft: Generator is required")
	}
	return (&modelResolver{generator: generator}).Resolve(context.Background(), alias)
}

func (r *modelResolver) Resolve(_ context.Context, alias string) (flowllm.LLM, error) {
	alias = strings.TrimSpace(alias)
	if alias == "" || strings.Contains(alias, "/") {
		return nil, fmt.Errorf("flowcraft: invalid model alias %q", alias)
	}
	return &genXLLM{generator: r.generator, pattern: "model/" + alias}, nil
}

func (*modelResolver) InvalidateCache(...flowllm.InvalidateOption) {}

type genXLLM struct {
	generator genx.Generator
	pattern   string
}

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
	options := flowllm.ApplyOptions(opts...)
	modelContext, err := buildModelContext(messages, options)
	if err != nil {
		return nil, err
	}
	if options.JSONSchema != nil {
		tool, err := structuredOutputTool(options.JSONSchema)
		if err != nil {
			return nil, err
		}
		usage, call, err := l.generator.Invoke(ctx, l.pattern, modelContext, tool)
		if err != nil {
			return nil, err
		}
		if call == nil {
			return nil, fmt.Errorf("flowcraft: structured model output is empty")
		}
		return &genXStructuredStream{
			content: call.Arguments,
			usage: flowmodel.Usage{
				InputTokens:       usage.PromptTokenCount,
				CachedInputTokens: usage.CachedContentTokenCount,
				OutputTokens:      usage.GeneratedTokenCount,
			},
		}, nil
	}
	stream, err := l.generator.GenerateStream(ctx, l.pattern, modelContext)
	if err != nil {
		return nil, err
	}
	return &genXStream{stream: stream}, nil
}

func buildModelContext(messages []flowmodel.Message, options *flowllm.GenerateOptions) (genx.ModelContext, error) {
	builder := &genx.ModelContextBuilder{Params: &genx.ModelParams{}}
	if options != nil {
		if len(options.Tools) != 0 || options.ToolChoice != nil {
			return nil, fmt.Errorf("flowcraft: tool calls are outside this Transformer")
		}
		if options.MaxTokens != nil {
			builder.Params.MaxTokens = int(*options.MaxTokens)
		}
		if options.Temperature != nil {
			builder.Params.Temperature = float32(*options.Temperature)
		}
		if options.TopP != nil {
			builder.Params.TopP = float32(*options.TopP)
		}
		if options.TopK != nil {
			builder.Params.TopK = float32(*options.TopK)
		}
		if options.FrequencyPenalty != nil {
			builder.Params.FrequencyPenalty = float32(*options.FrequencyPenalty)
		}
		if options.PresencePenalty != nil {
			builder.Params.PresencePenalty = float32(*options.PresencePenalty)
		}
		if options.Thinking != nil {
			builder.Params.Thinking = &genx.ThinkingParams{Enabled: options.Thinking}
		}
		builder.Params.ExtraFields = cloneAnyMap(options.Extra)
		if len(options.StopWords) != 0 {
			return nil, fmt.Errorf("flowcraft: stop words are not represented by genx.ModelParams")
		}
		if options.ImageGen != nil {
			return nil, fmt.Errorf("flowcraft: image generation is outside this text Transformer")
		}
		if options.JSONSchema == nil && options.JSONMode != nil && *options.JSONMode {
			builder.PromptText("flowcraft_json_mode", "Return exactly one valid JSON value without Markdown fences or explanatory text.")
		}
	}
	for _, message := range messages {
		emptyUser := message.Role == flowmodel.RoleUser && strings.TrimSpace(message.Content()) == ""
		wroteUserText := false
		for _, part := range message.Parts {
			if part.Type == flowmodel.PartData && part.Data != nil && part.Data.MimeType == "application/vnd.genx.interruption+json" {
				continue
			}
			if part.Type != flowmodel.PartText {
				return nil, fmt.Errorf("flowcraft: unsupported model message part %q", part.Type)
			}
			switch message.Role {
			case flowmodel.RoleSystem:
				builder.PromptText("system", part.Text)
			case flowmodel.RoleUser:
				text := part.Text
				if emptyUser && !wroteUserText {
					text = providerSafeEmptyUserText
				}
				builder.UserText("", text)
				wroteUserText = true
			case flowmodel.RoleAssistant:
				builder.ModelText("", part.Text)
			default:
				return nil, fmt.Errorf("flowcraft: unsupported model message role %q", message.Role)
			}
		}
		if emptyUser && !wroteUserText {
			builder.UserText("", providerSafeEmptyUserText)
		}
	}
	return builder.Build(), nil
}

func structuredOutputTool(param *flowllm.JSONSchemaParam) (*genx.FuncTool, error) {
	if param == nil {
		return nil, fmt.Errorf("flowcraft: structured output schema is required")
	}
	name := strings.TrimSpace(param.Name)
	if name == "" {
		name = "flowcraft_structured_output"
	}
	description := strings.TrimSpace(param.Description)
	if description == "" {
		description = "Flowcraft structured model output"
	}
	schema, err := convertJSONSchema(param.Schema)
	if err != nil {
		return nil, err
	}
	return &genx.FuncTool{
		Name:        name,
		Description: description,
		Argument:    schema,
	}, nil
}

func convertJSONSchema(source any) (*jsonschema.Schema, error) {
	if schema, ok := source.(*jsonschema.Schema); ok && schema != nil {
		return schema, nil
	}
	encoded, err := json.Marshal(source)
	if err != nil {
		return nil, fmt.Errorf("flowcraft: encode structured output schema: %w", err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(encoded, &schema); err != nil {
		return nil, fmt.Errorf("flowcraft: decode structured output schema: %w", err)
	}
	return &schema, nil
}

func cloneAnyMap(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]any, len(source))
	maps.Copy(result, source)
	return result
}

type genXStream struct {
	stream     genx.Stream
	current    flowmodel.StreamChunk
	content    strings.Builder
	err        error
	pendingErr error
	usage      flowmodel.Usage
}

type genXStructuredStream struct {
	content string
	usage   flowmodel.Usage
	current flowmodel.StreamChunk
	emitted bool
}

func (s *genXStructuredStream) Next() bool {
	if s == nil || s.emitted {
		return false
	}
	s.emitted = true
	s.current = flowmodel.StreamChunk{Role: flowmodel.RoleAssistant, Content: s.content}
	return true
}

func (s *genXStructuredStream) Current() flowmodel.StreamChunk { return s.current }
func (*genXStructuredStream) Err() error                       { return nil }
func (*genXStructuredStream) Close() error                     { return nil }
func (s *genXStructuredStream) Message() flowmodel.Message {
	return flowmodel.NewTextMessage(flowmodel.RoleAssistant, s.content)
}
func (s *genXStructuredStream) Usage() flowmodel.Usage { return s.usage }

func (s *genXStream) Next() bool {
	if s.err != nil || s.stream == nil {
		return false
	}
	if s.pendingErr != nil {
		s.err = s.pendingErr
		s.pendingErr = nil
		return false
	}
	for {
		chunk, err := s.stream.Next()
		if err != nil {
			var state *genx.State
			switch {
			case errors.As(err, &state) && state.Status() == genx.StatusDone:
				s.usage = flowmodel.Usage{InputTokens: state.Usage().PromptTokenCount, CachedInputTokens: state.Usage().CachedContentTokenCount, OutputTokens: state.Usage().GeneratedTokenCount}
			case errors.Is(err, io.EOF), errors.Is(err, buffer.ErrIteratorDone):
			default:
				s.err = err
			}
			return false
		}
		if chunk == nil {
			continue
		}
		endOfStream := chunk.IsEndOfStream()
		var terminalErr error
		if endOfStream && chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
			terminalErr = errors.New(chunk.Ctrl.Error)
		}
		if chunk.ToolCall != nil {
			s.err = fmt.Errorf("flowcraft: tool calls are outside this Transformer")
			return false
		}
		text, ok := chunk.Part.(genx.Text)
		if chunk.Part != nil && !ok {
			s.err = fmt.Errorf("flowcraft: model returned non-text part %T", chunk.Part)
			return false
		}
		if !ok || text == "" {
			if terminalErr != nil {
				s.err = terminalErr
				return false
			}
			continue
		}
		s.current = flowmodel.StreamChunk{Role: flowmodel.RoleAssistant, Content: string(text)}
		s.content.WriteString(string(text))
		s.pendingErr = terminalErr
		return true
	}
}

func (s *genXStream) Current() flowmodel.StreamChunk { return s.current }
func (s *genXStream) Err() error                     { return s.err }
func (s *genXStream) Close() error {
	if s == nil || s.stream == nil {
		return nil
	}
	return s.stream.Close()
}
func (s *genXStream) Message() flowmodel.Message {
	return flowmodel.NewTextMessage(flowmodel.RoleAssistant, s.content.String())
}
func (s *genXStream) Usage() flowmodel.Usage { return s.usage }

var _ flowllm.LLMResolver = (*modelResolver)(nil)
var _ flowllm.LLM = (*genXLLM)(nil)
var _ flowllm.StreamMessage = (*genXStream)(nil)
var _ flowllm.StreamMessage = (*genXStructuredStream)(nil)
