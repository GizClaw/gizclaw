package genx

import (
	"context"
	"encoding/json"
	"testing"
)

func TestToolConversationSnapshotsRemainIsolated(t *testing.T) {
	input := ToolConversation{CurrentUser: "set brightness", Messages: []ToolConversationMessage{{Role: "user", Content: "30%", Arguments: json.RawMessage(`{"level":30}`)}}}
	first := WithToolConversation(t.Context(), input)
	input.Messages[0].Content = "changed"
	input.Messages[0].Arguments[0] = '!'
	second := WithToolConversation(t.Context(), ToolConversation{CurrentUser: "cancel"})
	value, ok := ToolConversationFromContext(first)
	if !ok || value.Messages[0].Content != "30%" || !json.Valid(value.Messages[0].Arguments) {
		t.Fatalf("input mutation crossed snapshot: %#v", value)
	}
	value.Messages[0].Content = "changed again"
	again, _ := ToolConversationFromContext(first)
	other, _ := ToolConversationFromContext(second)
	if again.Messages[0].Content != "30%" || other.CurrentUser != "cancel" {
		t.Fatal("conversation snapshots crossed invocations")
	}
	if _, ok := ToolConversationFromContext(context.Background()); ok {
		t.Fatal("absent conversation was invented")
	}
}
