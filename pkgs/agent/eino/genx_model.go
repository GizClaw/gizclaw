package eino

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/google/jsonschema-go/jsonschema"
)

// GenXChatModel adapts a resolved GenX model pattern to Eino's immutable
// ToolCallingChatModel boundary.
type GenXChatModel struct {
	Generator genx.Generator
	Pattern   string
	tools     []*schema.ToolInfo
}

// NewGenXChatModel constructs an immutable Eino adapter for one resolved GenX
// generator pattern.
func NewGenXChatModel(generator genx.Generator, pattern string) (*GenXChatModel, error) {
	pattern = strings.TrimSpace(pattern)
	if generator == nil || pattern == "" {
		return nil, fmt.Errorf("agent/eino: GenX generator and pattern are required")
	}
	return &GenXChatModel{Generator: generator, Pattern: pattern}, nil
}

func (m *GenXChatModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	cloned, err := cloneToolInfos(tools)
	if err != nil {
		return nil, err
	}
	owned := *m
	owned.tools = cloned
	return &owned, nil
}

func cloneToolInfos(tools []*schema.ToolInfo) ([]*schema.ToolInfo, error) {
	cloned := make([]*schema.ToolInfo, len(tools))
	for i, info := range tools {
		if info == nil {
			continue
		}
		data, err := json.Marshal(info)
		if err != nil {
			return nil, fmt.Errorf("agent/eino: clone tool %d: %w", i, err)
		}
		cloned[i] = new(schema.ToolInfo)
		if err := json.Unmarshal(data, cloned[i]); err != nil {
			return nil, fmt.Errorf("agent/eino: clone tool %d: %w", i, err)
		}
	}
	return cloned, nil
}

func (m *GenXChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	stream, err := m.Stream(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	return schema.ConcatMessageStream(stream)
}

func (m *GenXChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if m == nil || m.Generator == nil {
		return nil, fmt.Errorf("agent/eino: GenX model is nil")
	}
	options := model.GetCommonOptions(&model.Options{Tools: slices.Clone(m.tools)}, opts...)
	modelContext, err := einoModelContext(input, options)
	if err != nil {
		return nil, err
	}
	inner, err := m.Generator.GenerateStream(ctx, m.Pattern, modelContext)
	if err != nil {
		return nil, err
	}
	reader, writer := schema.Pipe[*schema.Message](8)
	go func() {
		defer writer.Close()
		defer inner.Close()
		for {
			chunk, err := inner.Next()
			if commonagent.IsStreamEnd(err) {
				return
			}
			if err != nil {
				writer.Send(nil, err)
				return
			}
			if chunk != nil && chunk.IsEndOfStream() && chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
				writer.Send(nil, errors.New(chunk.Ctrl.Error))
				return
			}
			message := einoMessageChunk(chunk)
			if message != nil && writer.Send(message, nil) {
				return
			}
		}
	}()
	return reader, nil
}

func einoModelContext(input []*schema.Message, options *model.Options) (genx.ModelContext, error) {
	builder := &genx.ModelContextBuilder{Params: &genx.ModelParams{}}
	if options != nil {
		if options.Temperature != nil {
			builder.Params.Temperature = *options.Temperature
		}
		if options.TopP != nil {
			builder.Params.TopP = *options.TopP
		}
		if options.MaxTokens != nil {
			builder.Params.MaxTokens = *options.MaxTokens
		}
		for _, info := range options.Tools {
			tool, err := genXTool(info)
			if err != nil {
				return nil, err
			}
			builder.AddTool(tool)
		}
	}
	for _, message := range input {
		if message == nil {
			continue
		}
		switch message.Role {
		case schema.System:
			builder.PromptText(message.Name, message.Content)
		case schema.User:
			builder.UserText(message.Name, message.Content)
		case schema.Assistant:
			if message.Content != "" {
				builder.ModelText(message.Name, message.Content)
			}
			for _, call := range message.ToolCalls {
				builder.AddMessage(&genx.Message{Role: genx.RoleModel, Payload: &genx.ToolCall{
					ID: call.ID, FuncCall: &genx.FuncCall{Name: call.Function.Name, Arguments: call.Function.Arguments},
				}})
			}
		case schema.Tool:
			builder.AddMessage(&genx.Message{Role: genx.RoleTool, Payload: &genx.ToolResult{ID: message.ToolCallID, Result: message.Content}})
		}
	}
	return builder.Build(), nil
}

func genXTool(info *schema.ToolInfo) (*genx.FuncTool, error) {
	if info == nil || info.Name == "" {
		return nil, fmt.Errorf("agent/eino: Eino tool name is required")
	}
	argument := &jsonschema.Schema{Type: "object"}
	if info.ParamsOneOf != nil {
		einoSchema, err := info.ParamsOneOf.ToJSONSchema()
		if err != nil {
			return nil, fmt.Errorf("agent/eino: render Eino tool %q schema: %w", info.Name, err)
		}
		data, err := json.Marshal(einoSchema)
		if err != nil {
			return nil, fmt.Errorf("agent/eino: encode Eino tool %q schema: %w", info.Name, err)
		}
		if err := json.Unmarshal(data, argument); err != nil {
			return nil, fmt.Errorf("agent/eino: convert Eino tool %q schema: %w", info.Name, err)
		}
	}
	return &genx.FuncTool{Name: info.Name, Description: info.Desc, Argument: argument}, nil
}

func einoMessageChunk(chunk *genx.MessageChunk) *schema.Message {
	if chunk == nil || chunk.IsEndOfStream() {
		return nil
	}
	message := &schema.Message{Role: schema.Assistant}
	if text, ok := chunk.Part.(genx.Text); ok {
		message.Content = string(text)
	}
	if chunk.ToolCall != nil && chunk.ToolCall.FuncCall != nil {
		message.ToolCalls = []schema.ToolCall{{
			ID: chunk.ToolCall.ID, Type: "function",
			Function: schema.FunctionCall{Name: chunk.ToolCall.FuncCall.Name, Arguments: chunk.ToolCall.FuncCall.Arguments},
		}}
	}
	if message.Content == "" && len(message.ToolCalls) == 0 {
		return nil
	}
	return message
}

var _ model.ToolCallingChatModel = (*GenXChatModel)(nil)
