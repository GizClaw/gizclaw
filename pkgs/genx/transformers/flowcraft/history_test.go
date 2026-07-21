package flowcraft

import (
	"context"
	"fmt"
	"testing"

	flowmodel "github.com/GizClaw/flowcraft/sdk/model"
)

func TestInvocationLocalHistoryUsesWindow(t *testing.T) {
	t.Parallel()
	history := &conversationHistory{}
	messages := make([]flowmodel.Message, 0, historyWindow+10)
	for index := range historyWindow + 10 {
		messages = append(messages, flowmodel.NewTextMessage(flowmodel.RoleUser, fmt.Sprintf("message-%d", index)))
	}
	if err := history.append(context.Background(), messages, false); err != nil {
		t.Fatalf("append() error = %v", err)
	}
	if len(history.live) != historyWindow {
		t.Fatalf("retained History = %d messages, want %d", len(history.live), historyWindow)
	}
	messages, err := history.load(context.Background())
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if len(messages) != historyWindow || messages[0].Content() != "message-10" {
		t.Fatalf("window = %d messages starting at %q", len(messages), messages[0].Content())
	}
}
