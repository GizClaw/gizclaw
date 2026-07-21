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
	for index := range historyWindow + 10 {
		history.live = append(history.live, flowmodel.NewTextMessage(flowmodel.RoleUser, fmt.Sprintf("message-%d", index)))
	}
	messages, err := history.load(context.Background())
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if len(messages) != historyWindow || messages[0].Content() != "message-10" {
		t.Fatalf("window = %d messages starting at %q", len(messages), messages[0].Content())
	}
}
