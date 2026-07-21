package flowcraft

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"

	flowllm "github.com/GizClaw/flowcraft/sdk/llm"
	flowmodel "github.com/GizClaw/flowcraft/sdk/model"
	"github.com/GizClaw/gizclaw-go/pkgs/buffer"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type modelResolver struct{ generator genx.Generator }

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
	modelContext, err := buildModelContext(messages, flowllm.ApplyOptions(opts...))
	if err != nil {
		return nil, err
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
		if len(options.StopWords) != 0 || options.JSONSchema != nil || options.JSONMode != nil && *options.JSONMode {
			return nil, fmt.Errorf("flowcraft: stop words and structured output are not represented by genx.ModelParams")
		}
		if options.ImageGen != nil {
			return nil, fmt.Errorf("flowcraft: image generation is outside this text Transformer")
		}
	}
	for _, message := range messages {
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
				builder.UserText("", part.Text)
			case flowmodel.RoleAssistant:
				builder.ModelText("", part.Text)
			default:
				return nil, fmt.Errorf("flowcraft: unsupported model message role %q", message.Role)
			}
		}
	}
	return builder.Build(), nil
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
