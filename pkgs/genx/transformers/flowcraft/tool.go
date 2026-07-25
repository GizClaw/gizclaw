package flowcraft

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	flowmodel "github.com/GizClaw/flowcraft/sdk/model"
	"github.com/GizClaw/gizclaw-go/pkgs/buffer"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/toolrun"
)

type genXToolStream struct {
	ctx       context.Context
	generator genx.Generator
	pattern   string
	state     *toolrun.State
	builder   *genx.ModelContextBuilder
	invoker   genx.ToolInvoker
	stream    genx.Stream

	current      flowmodel.StreamChunk
	content      strings.Builder
	roundContent strings.Builder
	calls        []genx.ToolCall
	err          error
	pendingErr   error
	usage        flowmodel.Usage
}

func newGenXToolStream(
	ctx context.Context,
	generator genx.Generator,
	pattern string,
	modelContext genx.ModelContext,
	invoker genx.ToolInvoker,
) (*genXToolStream, error) {
	builder := cloneModelContext(modelContext)
	resolved, err := modelContextWithTools(ctx, builder, invoker)
	if err != nil {
		return nil, err
	}
	stream, err := generator.GenerateStream(ctx, pattern, resolved)
	if err != nil {
		return nil, err
	}
	state := toolrun.FromContext(ctx)
	if state == nil {
		state = toolrun.New(invoker, 0)
	}
	return &genXToolStream{
		ctx: ctx, generator: generator, pattern: pattern,
		state: state, builder: builder, invoker: invoker, stream: stream,
	}, nil
}

func (s *genXToolStream) Next() bool {
	if s == nil || s.err != nil || s.stream == nil {
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
			terminal, usage, terminalErr := genXToolStreamEnd(err)
			if !terminal {
				s.err = terminalErr
				return false
			}
			s.addUsage(usage)
			if len(s.calls) == 0 {
				return false
			}
			if err := s.continueAfterTools(); err != nil {
				s.err = err
				return false
			}
			continue
		}
		if chunk == nil {
			continue
		}
		terminalErr := error(nil)
		if chunk.IsEndOfStream() && chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
			terminalErr = errors.New(chunk.Ctrl.Error)
		}
		if chunk.ToolCall != nil {
			if terminalErr != nil {
				s.err = terminalErr
				return false
			}
			s.calls = append(s.calls, cloneToolCall(*chunk.ToolCall))
			continue
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
		s.roundContent.WriteString(string(text))
		s.pendingErr = terminalErr
		return true
	}
}

func (s *genXToolStream) continueAfterTools() error {
	if err := s.stream.Close(); err != nil {
		return err
	}
	if text := s.roundContent.String(); text != "" {
		s.builder.ModelText("", text)
	}
	for _, call := range s.calls {
		if call.FuncCall == nil {
			return fmt.Errorf("flowcraft: ToolCall %q has no function", call.ID)
		}
		s.builder.AddMessage(&genx.Message{
			Role: genx.RoleModel,
			Payload: &genx.ToolCall{
				ID: call.ID,
				FuncCall: &genx.FuncCall{
					Name: call.FuncCall.Name, Arguments: call.FuncCall.Arguments,
				},
			},
		})
		result, err := s.state.Invoke(s.ctx, call)
		if err != nil {
			return err
		}
		s.builder.AddMessage(&genx.Message{
			Role: genx.RoleTool, Payload: &genx.ToolResult{ID: result.ID, Result: result.Result},
		})
	}
	resolved, err := modelContextWithTools(s.ctx, s.builder, s.invoker)
	if err != nil {
		return err
	}
	stream, err := s.generator.GenerateStream(s.ctx, s.pattern, resolved)
	if err != nil {
		return err
	}
	s.stream = stream
	s.calls = s.calls[:0]
	s.roundContent.Reset()
	return nil
}

func (s *genXToolStream) addUsage(usage genx.Usage) {
	s.usage.InputTokens += usage.PromptTokenCount
	s.usage.CachedInputTokens += usage.CachedContentTokenCount
	s.usage.OutputTokens += usage.GeneratedTokenCount
}

func genXToolStreamEnd(err error) (bool, genx.Usage, error) {
	var state *genx.State
	switch {
	case errors.As(err, &state) && state.Status() == genx.StatusDone:
		return true, state.Usage(), nil
	case errors.Is(err, io.EOF), errors.Is(err, buffer.ErrIteratorDone):
		return true, genx.Usage{}, nil
	default:
		return false, genx.Usage{}, err
	}
}

func cloneModelContext(source genx.ModelContext) *genx.ModelContextBuilder {
	builder := &genx.ModelContextBuilder{}
	for prompt := range source.Prompts() {
		builder.Prompts = append(builder.Prompts, prompt)
	}
	for message := range source.Messages() {
		builder.Messages = append(builder.Messages, message)
	}
	for cot := range source.CoTs() {
		builder.CoTs = append(builder.CoTs, cot)
	}
	for tool := range source.Tools() {
		builder.Tools = append(builder.Tools, tool)
	}
	builder.Params = source.Params()
	return builder
}

func modelContextWithTools(
	ctx context.Context,
	source *genx.ModelContextBuilder,
	invoker genx.ToolInvoker,
) (genx.ModelContext, error) {
	builder := cloneModelContext(source.Build())
	if invoker == nil {
		return builder.Build(), nil
	}
	definitions, err := toolrun.ResolveTools(ctx, invoker)
	if err != nil {
		return nil, fmt.Errorf("flowcraft: %w", err)
	}
	for _, definition := range definitions {
		builder.AddTool(&genx.FuncTool{
			Name:        definition.Name,
			Description: definition.Description,
			Argument:    definition.Argument,
		})
	}
	return builder.Build(), nil
}

func cloneToolCall(source genx.ToolCall) genx.ToolCall {
	result := genx.ToolCall{ID: source.ID}
	if source.FuncCall != nil {
		result.FuncCall = &genx.FuncCall{Name: source.FuncCall.Name, Arguments: source.FuncCall.Arguments}
	}
	return result
}

func (s *genXToolStream) Current() flowmodel.StreamChunk { return s.current }
func (s *genXToolStream) Err() error                     { return s.err }
func (s *genXToolStream) Message() flowmodel.Message {
	return flowmodel.NewTextMessage(flowmodel.RoleAssistant, s.content.String())
}
func (s *genXToolStream) Usage() flowmodel.Usage { return s.usage }
func (s *genXToolStream) Close() error {
	if s == nil || s.stream == nil {
		return nil
	}
	return s.stream.Close()
}
