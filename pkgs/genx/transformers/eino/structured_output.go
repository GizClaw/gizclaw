package eino

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/google/jsonschema-go/jsonschema"
)

func (output *StructuredOutput) resolve() (*jsonschema.Resolved, error) {
	if len(output.Name) == 0 || len(output.Name) > 64 {
		return nil, fmt.Errorf("name must contain 1 to 64 ASCII letters, digits, underscores or hyphens")
	}
	for _, char := range output.Name {
		if char != '_' && char != '-' && !(char >= 'a' && char <= 'z') &&
			!(char >= 'A' && char <= 'Z') && !(char >= '0' && char <= '9') {
			return nil, fmt.Errorf("name must contain only ASCII letters, digits, underscores or hyphens")
		}
	}
	if output.Schema == nil || output.Schema.Type != "object" {
		return nil, fmt.Errorf("schema must declare an object")
	}
	return output.Schema.Resolve(nil)
}

func (chatModel *streamingChatModel) generateStructured(
	ctx context.Context,
	state *runState,
	input []*schema.Message,
	options ...model.Option,
) (*schema.Message, error) {
	component := chatModel.component.(StructuredChatModel)
	message, err := component.GenerateStructured(ctx, cloneMessages(input),
		*chatModel.node.ChatModel.StructuredOutput,
		append(slices.Clone(chatModel.options), options...)...)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if message == nil || message.Role != schema.Assistant || len(message.ToolCalls) != 0 ||
		message.ToolCallID != "" || len(message.MultiContent) != 0 || len(message.UserInputMultiContent) != 0 || len(message.AssistantGenMultiContent) != 0 {
		return nil, fmt.Errorf("eino: structured output requires one assistant JSON object")
	}
	if !utf8.ValidString(message.Content) || len(message.Content) > chatModel.maxReplyBytes {
		return nil, fmt.Errorf("eino: structured output is invalid UTF-8 or exceeds MaxOutputBytes")
	}
	value, err := decodeJSONObject(message.Content)
	if err != nil {
		return nil, fmt.Errorf("eino: structured output: %w", err)
	}
	if err := chatModel.structuredSchema.Validate(value); err != nil {
		return nil, fmt.Errorf("eino: structured output does not match schema: %w", err)
	}
	var content strings.Builder
	if err := chatModel.publishText(state, chatModel.node.Outputs["text"], &content, message.Content); err != nil {
		return nil, err
	}
	return message, nil
}
