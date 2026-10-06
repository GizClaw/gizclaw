package genx

import (
	"context"
	"encoding/json"
	"slices"
)

// ToolConversation contains the actual input and continuation visible when a
// model proposes a ToolCall. Provider call IDs are deliberately absent.
type ToolConversation struct {
	CurrentUser string `json:"current_user"`
	// ContinuationStart is the first message produced by the current model
	// invocation. Earlier messages are input/history, not current execution proof.
	ContinuationStart int                       `json:"continuation_start"`
	Messages          []ToolConversationMessage `json:"messages"`
}

// ToolConversationMessage is a provider-neutral text/control observation.
// Arguments and results are untrusted data, not authorization instructions.
type ToolConversationMessage struct {
	Role      string          `json:"role"`
	Content   string          `json:"content,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type toolConversationKey struct{}

// WithToolConversation attaches an owned snapshot for the current invocation.
func WithToolConversation(ctx context.Context, conversation ToolConversation) context.Context {
	return context.WithValue(ctx, toolConversationKey{}, cloneToolConversation(conversation))
}

// ToolConversationFromContext returns an owned copy of the current snapshot.
func ToolConversationFromContext(ctx context.Context) (ToolConversation, bool) {
	if ctx == nil {
		return ToolConversation{}, false
	}
	value, ok := ctx.Value(toolConversationKey{}).(ToolConversation)
	return cloneToolConversation(value), ok
}

func cloneToolConversation(value ToolConversation) ToolConversation {
	value.Messages = slices.Clone(value.Messages)
	for index := range value.Messages {
		value.Messages[index].Arguments = slices.Clone(value.Messages[index].Arguments)
	}
	return value
}
