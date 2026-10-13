package eino

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/cloudwego/eino/schema"
)

type conversationHistory struct {
	config *HistoryConfig
	mu     sync.Mutex
	live   []*schema.Message
}

func (history *conversationHistory) load(ctx context.Context, excludeTurnID ...string) ([]*schema.Message, error) {
	if history.config == nil || history.config.Store == nil {
		history.mu.Lock()
		defer history.mu.Unlock()
		messages := history.live
		limit := 50
		if history.config != nil {
			limit = history.config.Limit
		}
		if len(messages) > limit {
			messages = messages[len(messages)-limit:]
		}
		return cloneMessages(messages), nil
	}
	query := memory.HistoryQuery{ConversationID: history.config.Scope, Limit: history.config.Limit, Descending: true}
	if len(excludeTurnID) != 0 {
		query.ExcludeTurnID = excludeTurnID[0]
	}
	page, err := history.config.Store.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("eino: load History: %w", err)
	}
	if len(page.Entries) > query.Limit {
		return nil, fmt.Errorf("eino: History reader exceeded the query limit")
	}
	messages := make([]*schema.Message, 0, len(page.Entries))
	for _, entry := range page.Entries {
		if entry.ConversationID != query.ConversationID {
			return nil, fmt.Errorf("eino: History reader returned another conversation")
		}
		if strings.TrimSpace(entry.Text) == "" {
			continue
		}
		switch entry.Role {
		case memory.RoleUser:
			messages = append(messages, schema.UserMessage(entry.Text))
		case memory.RoleAssistant:
			messages = append(messages, schema.AssistantMessage(entry.Text, nil))
		default:
			return nil, fmt.Errorf("eino: unsupported History role %q", entry.Role)
		}
	}
	slices.Reverse(messages)
	return messages, nil
}

func (history *conversationHistory) append(_ context.Context, messages []*schema.Message, _ bool) error {
	// A supplied reader is authoritative. Never create a second durable history.
	if history.config != nil && history.config.Store != nil {
		return nil
	}
	history.mu.Lock()
	defer history.mu.Unlock()
	history.live = append(history.live, cloneMessages(messages)...)
	limit := 50
	if history.config != nil {
		limit = history.config.Limit
	}
	if len(history.live) > limit {
		history.live = append([]*schema.Message(nil), history.live[len(history.live)-limit:]...)
	}
	return nil
}
